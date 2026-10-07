package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gigabytegrove/calden/internal/recurrence"
)

type occurrenceChange struct {
	OriginalStart time.Time  `json:"original_start"`
	Event         eventInput `json:"event"`
}

type occurrenceTarget struct {
	OriginalStart time.Time `json:"original_start"`
}

func (s *server) updateOccurrence(w http.ResponseWriter, r *http.Request) {
	parentID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "Invalid recurring event")
		return
	}

	var in occurrenceChange
	if decode(r, &in) != nil || in.OriginalStart.IsZero() {
		writeError(w, 400, "Choose the occurrence to change")
		return
	}
	in.Event.Recurrence = nil
	if msg := validateEventInput(in.Event); msg != "" {
		writeError(w, 400, msg)
		return
	}

	parentCalendar, exists, err := s.validSeriesOccurrence(r.Context(), parentID, in.OriginalStart)
	if err != nil {
		writeError(w, 500, "Could not check the recurring event")
		return
	}
	if !exists {
		writeError(w, 404, "That occurrence is not part of this recurring event")
		return
	}
	if !s.canEditCalendar(r, parentCalendar) || !s.canEditCalendar(r, in.Event.CalendarID) {
		writeError(w, 403, "You cannot change this occurrence")
		return
	}

	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "Could not change occurrence")
		return
	}
	defer tx.Rollback(r.Context())

	var replacementID uuid.UUID
	err = tx.QueryRow(r.Context(), `SELECT replacement_event_id
		FROM event_occurrence_exceptions
		WHERE event_id=$1 AND original_start=$2 AND replacement_event_id IS NOT NULL`,
		parentID, in.OriginalStart).Scan(&replacementID)
	if err != nil && err != pgx.ErrNoRows {
		writeError(w, 500, "Could not load occurrence override")
		return
	}

	if replacementID == uuid.Nil {
		err = tx.QueryRow(r.Context(), `INSERT INTO events(
				calendar_id,category_id,title,notes,location,starts_at,ends_at,all_day,created_by,
				recurrence_parent_id,recurrence_original_start
			) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
			RETURNING id`,
			in.Event.CalendarID, in.Event.CategoryID, cleanText(in.Event.Title, 200), cleanText(in.Event.Notes, 5000),
			cleanText(in.Event.Location, 500), in.Event.StartsAt, in.Event.EndsAt, in.Event.AllDay,
			currentActor(r).ID, parentID, in.OriginalStart).Scan(&replacementID)
		if err != nil {
			writeError(w, 500, "Could not create occurrence override")
			return
		}
	} else {
		if _, err = tx.Exec(r.Context(), `UPDATE events
			SET calendar_id=$2,category_id=$3,title=$4,notes=$5,location=$6,starts_at=$7,ends_at=$8,all_day=$9,status='confirmed',updated_at=now()
			WHERE id=$1`,
			replacementID, in.Event.CalendarID, in.Event.CategoryID, cleanText(in.Event.Title, 200), cleanText(in.Event.Notes, 5000),
			cleanText(in.Event.Location, 500), in.Event.StartsAt, in.Event.EndsAt, in.Event.AllDay); err != nil {
			writeError(w, 500, "Could not update occurrence override")
			return
		}
	}

	if _, err = tx.Exec(r.Context(), `DELETE FROM event_assignees WHERE event_id=$1`, replacementID); err != nil {
		writeError(w, 500, "Could not update occurrence people")
		return
	}
	for _, userID := range in.Event.AssigneeIDs {
		if _, err = tx.Exec(r.Context(), `INSERT INTO event_assignees(event_id,user_id) VALUES($1,$2)`, replacementID, userID); err != nil {
			writeError(w, 400, "One of the selected people is invalid")
			return
		}
	}
	if err = s.saveBillDetails(r.Context(), tx, replacementID, in.Event.CalendarID, in.Event); err != nil {
		writeError(w, 400, "Could not save bill details")
		return
	}

	if _, err = tx.Exec(r.Context(), `DELETE FROM reminders WHERE event_id=$1`, replacementID); err != nil {
		writeError(w, 500, "Could not update occurrence reminders")
		return
	}
	for _, rm := range in.Event.Reminders {
		if _, err = tx.Exec(r.Context(), `INSERT INTO reminders(event_id,kind,provider,minutes_before,destination)
			VALUES($1,$2,$3,$4,NULLIF($5,''))`,
			replacementID, rm.Kind, rm.Provider, rm.MinutesBefore, cleanText(rm.Destination, 200)); err != nil {
			writeError(w, 400, "Could not save occurrence reminder")
			return
		}
	}

	if _, err = tx.Exec(r.Context(), `INSERT INTO event_occurrence_exceptions(
			event_id,original_start,cancelled,replacement_event_id,updated_at
		) VALUES($1,$2,false,$3,now())
		ON CONFLICT(event_id,original_start) DO UPDATE
		SET cancelled=false,replacement_event_id=$3,updated_at=now()`,
		parentID, in.OriginalStart, replacementID); err != nil {
		writeError(w, 500, "Could not save occurrence override")
		return
	}

	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "Could not save occurrence")
		return
	}
	s.audit(r, "update_occurrence", "event", &parentID, "Changed one recurring occurrence", map[string]any{
		"original_start": in.OriginalStart, "replacement_event_id": replacementID, "new_start": in.Event.StartsAt,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"id": replacementID, "series_id": parentID, "original_start": in.OriginalStart,
	})
}

func (s *server) deleteOccurrence(w http.ResponseWriter, r *http.Request) {
	parentID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "Invalid recurring event")
		return
	}
	var in occurrenceTarget
	if decode(r, &in) != nil || in.OriginalStart.IsZero() {
		writeError(w, 400, "Choose the occurrence to delete")
		return
	}

	parentCalendar, exists, err := s.validSeriesOccurrence(r.Context(), parentID, in.OriginalStart)
	if err != nil {
		writeError(w, 500, "Could not check the recurring event")
		return
	}
	if !exists {
		writeError(w, 404, "That occurrence is not part of this recurring event")
		return
	}
	if !s.canEditCalendar(r, parentCalendar) {
		writeError(w, 403, "You cannot delete this occurrence")
		return
	}

	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "Could not delete occurrence")
		return
	}
	defer tx.Rollback(r.Context())

	var replacementID *uuid.UUID
	_ = tx.QueryRow(r.Context(), `SELECT replacement_event_id
		FROM event_occurrence_exceptions WHERE event_id=$1 AND original_start=$2`,
		parentID, in.OriginalStart).Scan(&replacementID)
	if replacementID != nil && *replacementID != uuid.Nil {
		if _, err = tx.Exec(r.Context(), `DELETE FROM events WHERE id=$1`, *replacementID); err != nil {
			writeError(w, 500, "Could not remove occurrence override")
			return
		}
	}
	if _, err = tx.Exec(r.Context(), `DELETE FROM bill_payments WHERE event_id=$1 AND occurrence_start=$2`,
		parentID, in.OriginalStart); err != nil {
		writeError(w, 500, "Could not clear bill payment status")
		return
	}

	if _, err = tx.Exec(r.Context(), `INSERT INTO event_occurrence_exceptions(
			event_id,original_start,cancelled,replacement_event_id,updated_at
		) VALUES($1,$2,true,NULL,now())
		ON CONFLICT(event_id,original_start) DO UPDATE
		SET cancelled=true,replacement_event_id=NULL,updated_at=now()`,
		parentID, in.OriginalStart); err != nil {
		writeError(w, 500, "Could not cancel occurrence")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "Could not cancel occurrence")
		return
	}
	s.audit(r, "delete_occurrence", "event", &parentID, "Deleted one recurring occurrence", map[string]any{"original_start": in.OriginalStart})
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) restoreOccurrence(w http.ResponseWriter, r *http.Request) {
	parentID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "Invalid recurring event")
		return
	}
	var in occurrenceTarget
	if decode(r, &in) != nil || in.OriginalStart.IsZero() {
		writeError(w, 400, "Choose the occurrence to restore")
		return
	}

	parentCalendar, exists, err := s.validSeriesOccurrence(r.Context(), parentID, in.OriginalStart)
	if err != nil {
		writeError(w, 500, "Could not check the recurring event")
		return
	}
	if !exists {
		writeError(w, 404, "That occurrence is not part of this recurring event")
		return
	}
	if !s.canEditCalendar(r, parentCalendar) {
		writeError(w, 403, "You cannot restore this occurrence")
		return
	}

	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "Could not restore occurrence")
		return
	}
	defer tx.Rollback(r.Context())

	var replacementID *uuid.UUID
	_ = tx.QueryRow(r.Context(), `SELECT replacement_event_id
		FROM event_occurrence_exceptions WHERE event_id=$1 AND original_start=$2`,
		parentID, in.OriginalStart).Scan(&replacementID)
	if replacementID != nil && *replacementID != uuid.Nil {
		if _, err = tx.Exec(r.Context(), `DELETE FROM events WHERE id=$1`, *replacementID); err != nil {
			writeError(w, 500, "Could not remove occurrence override")
			return
		}
	}
	if _, err = tx.Exec(r.Context(), `DELETE FROM event_occurrence_exceptions
		WHERE event_id=$1 AND original_start=$2`, parentID, in.OriginalStart); err != nil {
		writeError(w, 500, "Could not restore occurrence")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "Could not restore occurrence")
		return
	}
	s.audit(r, "restore_occurrence", "event", &parentID, "Restored recurring occurrence", map[string]any{"original_start": in.OriginalStart})
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) validSeriesOccurrence(ctx context.Context, parentID uuid.UUID, originalStart time.Time) (uuid.UUID, bool, error) {
	var calendarID uuid.UUID
	var seriesStart, seriesEnd time.Time
	var frequency string
	var interval int
	var weekdaysRaw []byte
	var until *time.Time
	var count *int
	var rawRule string
	err := s.db.QueryRow(ctx, `SELECT e.calendar_id,e.starts_at,e.ends_at,er.frequency,er.interval_value,er.weekdays,er.until_at,er.occurrence_count,COALESCE(er.raw_rule,'')
		FROM events e JOIN event_recurrence er ON er.event_id=e.id WHERE e.id=$1`, parentID).
		Scan(&calendarID, &seriesStart, &seriesEnd, &frequency, &interval, &weekdaysRaw, &until, &count, &rawRule)
	if err == pgx.ErrNoRows {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, err
	}

	weekdays := []int{}
	if len(weekdaysRaw) > 0 {
		_ = json.Unmarshal(weekdaysRaw, &weekdays)
	}
	rule := recurrence.Normalize(&recurrence.Rule{
		Frequency: frequency, Interval: interval, Weekdays: weekdays,
		Until: until, OccurrenceCount: count, Raw: rawRule,
	}, seriesStart)
	windowFrom := originalStart.Add(-time.Second)
	windowTo := originalStart.Add(seriesEnd.Sub(seriesStart)).Add(time.Second)
	for _, occurrence := range recurrence.Expand(seriesStart, seriesEnd, rule, windowFrom, windowTo, 10) {
		if occurrence.Start.Equal(originalStart) {
			return calendarID, true, nil
		}
	}
	return calendarID, false, nil
}
