package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

func (s *server) listNotifications(w http.ResponseWriter, r *http.Request) {
	a := currentActor(r)
	limit := 100
	rows, err := s.db.Query(r.Context(), `SELECT n.id,n.event_id,n.kind,n.title,n.message,n.read_at,n.created_at,
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
		var startsAt, endsAt *time.Time
		var allDay *bool
		var calendarID *uuid.UUID
		var calendarName, calendarColor *string
		if err := rows.Scan(
			&id, &eventID, &kind, &title, &message, &readAt, &createdAt,
			&startsAt, &endsAt, &allDay, &calendarID, &calendarName, &calendarColor,
		); err != nil {
			continue
		}
		items = append(items, map[string]any{
			"id": id, "event_id": eventID, "kind": kind, "title": title, "message": message,
			"read_at": readAt, "created_at": createdAt,
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
