package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/gigabytegrove/calden/internal/recurrence"
)

type eventInput struct {
	CalendarID  uuid.UUID   `json:"calendar_id"`
	CategoryID  *uuid.UUID  `json:"category_id,omitempty"`
	Title       string      `json:"title"`
	Notes       string      `json:"notes"`
	Location    string      `json:"location"`
	StartsAt    time.Time   `json:"starts_at"`
	EndsAt      time.Time   `json:"ends_at"`
	AllDay      bool        `json:"all_day"`
	AssigneeIDs []uuid.UUID `json:"assignee_ids"`
	Recurrence *recurrence.Rule `json:"recurrence,omitempty"`
	Reminders   []struct {
		Kind          string `json:"kind"`
		Provider      string `json:"provider"`
		MinutesBefore int    `json:"minutes_before"`
		Destination   string `json:"destination"`
	} `json:"reminders"`
}

func (s *server) canEditCalendar(r *http.Request, calendarID uuid.UUID) bool {
	a := currentActor(r)
	if a.Role == "admin" {
		return true
	}
	var allowed bool
	_ = s.db.QueryRow(r.Context(), `SELECT COALESCE(can_edit,false) FROM calendar_permissions WHERE calendar_id=$1 AND user_id=$2`, calendarID, a.ID).Scan(&allowed)
	return allowed
}

func validateEventInput(in eventInput) string {
	if strings.TrimSpace(in.Title) == "" {
		return "Event name is required"
	}
	if in.CalendarID == uuid.Nil {
		return "Choose a calendar"
	}
	if in.StartsAt.IsZero() || in.EndsAt.IsZero() || in.EndsAt.Before(in.StartsAt) {
		return "Check the event date and time"
	}
	if in.Recurrence != nil {
		in.Recurrence = recurrence.Normalize(in.Recurrence, in.StartsAt)
		if err := recurrence.Validate(in.Recurrence); err != nil {
			return err.Error()
		}
		if in.Recurrence.Until != nil && in.Recurrence.Until.Before(in.StartsAt) {
			return "Repeat end date must be after the event starts"
		}
	}
	for _, rm := range in.Reminders {
		if rm.Kind != "personal" && rm.Kind != "system" {
			return "Check reminder settings"
		}
		if rm.Provider != "android" && rm.Provider != "monita" {
			return "Check reminder settings"
		}
		if rm.MinutesBefore < 0 {
			return "Check reminder settings"
		}
	}
	return ""
}

func (s *server) updateEvent(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "Invalid event")
		return
	}
	var in eventInput
	if decode(r, &in) != nil {
		writeError(w, 400, "Check the event details")
		return
	}
	if msg := validateEventInput(in); msg != "" {
		writeError(w, 400, msg)
		return
	}

	var oldCalendar uuid.UUID
	if err = s.db.QueryRow(r.Context(), "SELECT calendar_id FROM events WHERE id=$1", id).Scan(&oldCalendar); err != nil {
		writeError(w, 404, "Event not found")
		return
	}
	if !s.canEditCalendar(r, oldCalendar) || !s.canEditCalendar(r, in.CalendarID) {
		writeError(w, 403, "You cannot change this event")
		return
	}

	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "Could not update event")
		return
	}
	defer tx.Rollback(r.Context())

	_, err = tx.Exec(r.Context(), `UPDATE events SET calendar_id=$2,category_id=$3,title=$4,notes=$5,location=$6,starts_at=$7,ends_at=$8,all_day=$9,updated_at=now() WHERE id=$1`,
		id, in.CalendarID, in.CategoryID, cleanText(in.Title, 200), cleanText(in.Notes, 5000), cleanText(in.Location, 500), in.StartsAt, in.EndsAt, in.AllDay)
	if err != nil {
		writeError(w, 400, "Could not update event")
		return
	}

	if _, err = tx.Exec(r.Context(), "DELETE FROM event_assignees WHERE event_id=$1", id); err != nil {
		writeError(w, 500, "Could not update people")
		return
	}
	for _, uid := range in.AssigneeIDs {
		if _, err = tx.Exec(r.Context(), `INSERT INTO event_assignees(event_id,user_id) VALUES($1,$2)`, id, uid); err != nil {
			writeError(w, 400, "One of the selected people is invalid")
			return
		}
	}

	if _, err = tx.Exec(r.Context(), "DELETE FROM event_recurrence WHERE event_id=$1", id); err != nil {
		writeError(w, 500, "Could not update repeat settings")
		return
	}
	if in.Recurrence != nil {
		rule := recurrence.Normalize(in.Recurrence, in.StartsAt)
		weekdays, _ := json.Marshal(rule.Weekdays)
		if _, err = tx.Exec(r.Context(), `INSERT INTO event_recurrence(event_id,frequency,interval_value,weekdays,until_at,occurrence_count,raw_rule)
			VALUES($1,$2,$3,$4,$5,$6,$7)`, id, rule.Frequency, rule.Interval, weekdays, rule.Until, rule.OccurrenceCount, rule.Raw); err != nil {
			writeError(w, 400, "Could not save repeat settings")
			return
		}
	}

	if _, err = tx.Exec(r.Context(), "DELETE FROM reminders WHERE event_id=$1", id); err != nil {
		writeError(w, 500, "Could not update reminders")
		return
	}
	for _, rm := range in.Reminders {
		if _, err = tx.Exec(r.Context(), `INSERT INTO reminders(event_id,kind,provider,minutes_before,destination) VALUES($1,$2,$3,$4,NULLIF($5,''))`,
			id, rm.Kind, rm.Provider, rm.MinutesBefore, cleanText(rm.Destination, 200)); err != nil {
			writeError(w, 400, "Could not save reminder")
			return
		}
	}

	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "Could not save event")
		return
	}
	s.audit(r, "update", "event", &id, "Updated event "+cleanText(in.Title, 200), map[string]any{
		"calendar_id": in.CalendarID, "category_id": in.CategoryID, "starts_at": in.StartsAt, "recurring": in.Recurrence != nil,
	})
	writeJSON(w, 200, map[string]any{"id": id})
}

func (s *server) deleteEvent(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "Invalid event")
		return
	}
	var calendarID uuid.UUID
	if err = s.db.QueryRow(r.Context(), "SELECT calendar_id FROM events WHERE id=$1", id).Scan(&calendarID); err != nil {
		writeError(w, 404, "Event not found")
		return
	}
	if !s.canEditCalendar(r, calendarID) {
		writeError(w, 403, "You cannot delete this event")
		return
	}
	if _, err = s.db.Exec(r.Context(), "DELETE FROM events WHERE id=$1", id); err != nil {
		writeError(w, 500, "Could not delete event")
		return
	}
	s.audit(r, "delete", "event", &id, "Deleted event", nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) updateCalendar(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "Invalid calendar")
		return
	}
	var in struct {
		Name        string `json:"name"`
		Color       string `json:"color"`
		Icon        string `json:"icon"`
		Description string `json:"description"`
	}
	if decode(r, &in) != nil || strings.TrimSpace(in.Name) == "" || !validColor(in.Color) {
		writeError(w, 400, "Check the calendar details")
		return
	}
	if in.Icon == "" {
		in.Icon = "calendar"
	}
	tag, err := s.db.Exec(r.Context(), `UPDATE calendars SET name=$2,color=$3,icon=$4,description=$5,updated_at=now() WHERE id=$1`,
		id, cleanText(in.Name, 100), in.Color, cleanText(in.Icon, 40), cleanText(in.Description, 500))
	if err != nil {
		writeError(w, 500, "Could not update calendar")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 404, "Calendar not found")
		return
	}
	s.audit(r, "update", "calendar", &id, "Updated calendar "+cleanText(in.Name, 100), map[string]any{"color": in.Color})
	writeJSON(w, 200, map[string]any{"id": id})
}

func (s *server) deleteCalendar(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "Invalid calendar")
		return
	}
	var events int
	if err = s.db.QueryRow(r.Context(), "SELECT count(*) FROM events WHERE calendar_id=$1", id).Scan(&events); err != nil {
		writeError(w, 500, "Could not check calendar")
		return
	}
	if events > 0 {
		writeError(w, 409, "Move or delete this calendar's events first")
		return
	}
	tag, err := s.db.Exec(r.Context(), "DELETE FROM calendars WHERE id=$1", id)
	if err != nil {
		writeError(w, 500, "Could not delete calendar")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 404, "Calendar not found")
		return
	}
	s.audit(r, "delete", "calendar", &id, "Deleted calendar", nil)
	w.WriteHeader(http.StatusNoContent)
}
