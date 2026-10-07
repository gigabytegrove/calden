package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gigabytegrove/calden/internal/recurrence"
)

func (s *server) createEventNotifications(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	calendarID uuid.UUID,
	eventTitle string,
	assigneeIDs []uuid.UUID,
) error {
	recipients := map[uuid.UUID]bool{}
	kind := "event_assigned"
	message := "A new event was assigned to you."

	if len(assigneeIDs) > 0 {
		for _, userID := range assigneeIDs {
			if userID == uuid.Nil || recipients[userID] {
				continue
			}
			var active bool
			if err := tx.QueryRow(ctx, `SELECT active FROM users WHERE id=$1`, userID).Scan(&active); err != nil {
				return err
			}
			if active {
				recipients[userID] = true
			}
		}
	} else {
		kind = "event_family"
		message = "A new event was added for everyone."
		rows, err := tx.Query(ctx, `SELECT u.id
			FROM users u
			LEFT JOIN calendar_permissions p
			  ON p.user_id=u.id AND p.calendar_id=$1
			WHERE u.active=true
			  AND (u.role='admin' OR COALESCE(p.can_view,false)=true)
			ORDER BY u.id`, calendarID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var userID uuid.UUID
			if err := rows.Scan(&userID); err != nil {
				return err
			}
			recipients[userID] = true
		}
		if err := rows.Err(); err != nil {
			return err
		}
	}

	title := cleanText(strings.TrimSpace(eventTitle), 200)
	for userID := range recipients {
		if _, err := tx.Exec(ctx, `INSERT INTO notifications(user_id,event_id,kind,title,message)
			VALUES($1,$2,$3,$4,$5)`, userID, eventID, kind, title, message); err != nil {
			return err
		}
	}
	return nil
}

func (s *server) ensureBillReviewNotifications(ctx context.Context, userID uuid.UUID, role string) {
	timezone := "UTC"
	_ = s.db.QueryRow(ctx, `SELECT value FROM app_settings WHERE key='timezone'`).Scan(&timezone)
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	windowFrom := now.AddDate(0, 0, -3)
	windowTo := now.AddDate(0, 0, 1)

	rows, err := s.db.Query(ctx, `SELECT
		e.id,e.title,e.starts_at,e.ends_at,e.all_day,
		er.frequency,er.interval_value,er.weekdays,er.until_at,er.occurrence_count,COALESCE(er.raw_rule,'')
		FROM events e
		JOIN calendars c ON c.id=e.calendar_id
		LEFT JOIN calendar_permissions p ON p.calendar_id=c.id AND p.user_id=$1
		LEFT JOIN bill_event_details b ON b.event_id=e.id
		LEFT JOIN event_recurrence er ON er.event_id=e.id
		WHERE e.recurrence_parent_id IS NULL
		  AND c.calendar_type='bill_pay'
		  AND ($2='admin' OR COALESCE(p.can_view,false)=true)
		  AND (b.payer_user_id IS NULL OR b.payer_user_id=$1)
		  AND e.starts_at < $4
		  AND (er.event_id IS NOT NULL OR e.ends_at > $3)
		  AND (er.until_at IS NULL OR er.until_at >= $3)
		ORDER BY e.starts_at`, userID, role, windowFrom, windowTo)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var eventID uuid.UUID
		var title string
		var startsAt, endsAt time.Time
		var allDay bool
		var frequency *string
		var interval *int
		var weekdaysRaw []byte
		var until *time.Time
		var count *int
		var rawRule string
		if rows.Scan(
			&eventID, &title, &startsAt, &endsAt, &allDay,
			&frequency, &interval, &weekdaysRaw, &until, &count, &rawRule,
		) != nil {
			continue
		}

		var rule *recurrence.Rule
		if frequency != nil && interval != nil {
			weekdays := []int{}
			if len(weekdaysRaw) > 0 {
				_ = json.Unmarshal(weekdaysRaw, &weekdays)
			}
			rule = recurrence.Normalize(&recurrence.Rule{
				Frequency: *frequency, Interval: *interval, Weekdays: weekdays,
				Until: until, OccurrenceCount: count, Raw: rawRule,
			}, startsAt)
		}

		for _, occurrence := range recurrence.Expand(startsAt, endsAt, rule, windowFrom, windowTo, 32) {
			var excepted bool
			if rule != nil {
				_ = s.db.QueryRow(ctx, `SELECT EXISTS(
					SELECT 1 FROM event_occurrence_exceptions
					WHERE event_id=$1 AND original_start=$2
				)`, eventID, occurrence.Start).Scan(&excepted)
			}
			if excepted {
				continue
			}

			occurrenceLocal := occurrence.Start.In(loc)
			promptAt := occurrenceLocal
			if allDay {
				promptAt = time.Date(
					occurrenceLocal.Year(), occurrenceLocal.Month(), occurrenceLocal.Day(),
					20, 0, 0, 0, loc,
				)
			}
			if now.Before(promptAt) {
				continue
			}

			details := s.eventBillPayment(ctx, eventID, occurrence.Start)
			if details.Status == "cleared" || details.Status == "no_balance" {
				continue
			}
			message := "Confirm the autopay came out and record the payment."
			if details.Status == "allocated" {
				message = "Funds are allocated. Confirm the autopay came out and record the payment."
			} else if details.Status == "partial" {
				message = "A partial payment is recorded. Confirm the remaining payment activity."
			} else if details.Status == "paid" {
				message = "Payment is recorded. Confirm it cleared the bill-pay account."
			}
			_, _ = s.db.Exec(ctx, `INSERT INTO notifications(
					user_id,event_id,kind,title,message,occurrence_start
				) VALUES($1,$2,'bill_review',$3,$4,$5)
				ON CONFLICT DO NOTHING`,
				userID, eventID, cleanText(strings.TrimSpace(title), 200), message, occurrence.Start)
		}
	}
}

func (s *server) listNotifications(w http.ResponseWriter, r *http.Request) {
	a := currentActor(r)
	s.ensureBillReviewNotifications(r.Context(), a.ID, a.Role)
	limit := 100
	rows, err := s.db.Query(r.Context(), `SELECT n.id,n.event_id,n.kind,n.title,n.message,n.read_at,n.created_at,n.occurrence_start,
		e.starts_at,e.ends_at,e.all_day,c.id,c.name,c.color
		FROM notifications n
		LEFT JOIN events e ON e.id=n.event_id
		LEFT JOIN calendars c ON c.id=e.calendar_id
		WHERE n.user_id=$1
		ORDER BY n.created_at DESC
		LIMIT $2`, a.ID, limit)
	if err != nil {
		writeError(w, 500, "Could not load notifications")
		return
	}
	defer rows.Close()

	items := []map[string]any{}
	for rows.Next() {
		var id uuid.UUID
		var eventID *uuid.UUID
		var kind, title, message string
		var readAt *time.Time
		var createdAt time.Time
		var occurrenceStart *time.Time
		var startsAt, endsAt *time.Time
		var allDay *bool
		var calendarID *uuid.UUID
		var calendarName, calendarColor *string
		if err := rows.Scan(
			&id, &eventID, &kind, &title, &message, &readAt, &createdAt, &occurrenceStart,
			&startsAt, &endsAt, &allDay, &calendarID, &calendarName, &calendarColor,
		); err != nil {
			continue
		}
		items = append(items, map[string]any{
			"id": id, "event_id": eventID, "kind": kind, "title": title, "message": message,
			"read_at": readAt, "created_at": createdAt, "occurrence_start": occurrenceStart,
			"starts_at": startsAt, "ends_at": endsAt, "all_day": allDay,
			"calendar_id": calendarID, "calendar_name": calendarName, "calendar_color": calendarColor,
		})
	}
	if err := rows.Err(); err != nil {
		writeError(w, 500, "Could not finish loading notifications")
		return
	}
	var unread int
	if err := s.db.QueryRow(r.Context(), `SELECT count(*) FROM notifications WHERE user_id=$1 AND read_at IS NULL`, a.ID).Scan(&unread); err != nil {
		writeError(w, 500, "Could not count unread notifications")
		return
	}
	writeJSON(w, 200, map[string]any{"items": items, "unread": unread})
}

func (s *server) markNotificationRead(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "Invalid notification")
		return
	}
	tag, err := s.db.Exec(r.Context(), `UPDATE notifications
		SET read_at=COALESCE(read_at,now())
		WHERE id=$1 AND user_id=$2`, id, currentActor(r).ID)
	if err != nil {
		writeError(w, 500, "Could not update notification")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 404, "Notification not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) markAllNotificationsRead(w http.ResponseWriter, r *http.Request) {
	if _, err := s.db.Exec(r.Context(), `UPDATE notifications
		SET read_at=COALESCE(read_at,now())
		WHERE user_id=$1 AND read_at IS NULL`, currentActor(r).ID); err != nil {
		writeError(w, 500, "Could not update notifications")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
