package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/gigabytegrove/calden/internal/recurrence"
)

func (s *server) me(w http.ResponseWriter, r *http.Request) {
	a := currentActor(r)
	var out struct {
		ID          uuid.UUID `json:"id"`
		Username    string    `json:"username"`
		DisplayName string    `json:"display_name"`
		Role        string    `json:"role"`
		Initials    string    `json:"initials"`
		AvatarURL   *string   `json:"avatar_url"`
	}
	if err := s.db.QueryRow(r.Context(), `SELECT id,username,display_name,role,initials,avatar_url FROM users WHERE id=$1`, a.ID).
		Scan(&out.ID, &out.Username, &out.DisplayName, &out.Role, &out.Initials, &out.AvatarURL); err != nil {
		writeError(w, 500, "Could not load your account")
		return
	}
	writeJSON(w, 200, out)
}

func (s *server) listUsers(w http.ResponseWriter, r *http.Request) {
	query := `SELECT id,username,display_name,role,initials,avatar_url,active FROM users WHERE active=true ORDER BY display_name`
	if currentActor(r).Role == "admin" {
		query = `SELECT id,username,display_name,role,initials,avatar_url,active FROM users ORDER BY active DESC,display_name`
	}
	rows, err := s.db.Query(r.Context(), query)
	if err != nil {
		writeError(w, 500, "Could not load people")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id uuid.UUID
		var username, name, role, ini string
		var avatar *string
		var active bool
		if rows.Scan(&id, &username, &name, &role, &ini, &avatar, &active) == nil {
			out = append(out, map[string]any{
				"id": id, "username": username, "display_name": name, "role": role,
				"initials": ini, "avatar_url": avatar, "active": active,
			})
		}
	}
	writeJSON(w, 200, out)
}
func (s *server) createUser(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
		Role        string `json:"role"`
	}
	if decode(r, &in) != nil || strings.TrimSpace(in.DisplayName) == "" {
		writeError(w, 400, "Check the account details")
		return
	}
	if err := validateLogin(in.Username, in.Password); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	in.Role = strings.ToLower(strings.TrimSpace(in.Role))
	if in.Role == "" {
		in.Role = "member"
	}
	if in.Role != "member" && in.Role != "restricted" && in.Role != "admin" {
		writeError(w, 400, "Choose a valid account type")
		return
	}
	hash, err := hashPassword(in.Password)
	if err != nil {
		writeError(w, 500, "Could not create account")
		return
	}
	var id uuid.UUID
	err = s.db.QueryRow(r.Context(), `INSERT INTO users(username,display_name,password_hash,role,initials) VALUES(lower($1),$2,$3,$4,$5) RETURNING id`,
		strings.TrimSpace(in.Username), cleanText(in.DisplayName, 100), hash, in.Role, initials(in.DisplayName)).Scan(&id)
	if err != nil {
		writeError(w, 409, "That username is already in use")
		return
	}
	writeJSON(w, 201, map[string]any{"id": id})
}

func hashPassword(password string) (string, error) { return bcryptHash(password) }

func (s *server) listCalendars(w http.ResponseWriter, r *http.Request) {
	a := currentActor(r)
	rows, err := s.db.Query(r.Context(), `SELECT c.id,c.name,c.color,c.icon,c.description,
		COALESCE(p.can_edit,false),COALESCE(p.can_delete,false)
		FROM calendars c
		LEFT JOIN calendar_permissions p ON p.calendar_id=c.id AND p.user_id=$1
		WHERE $2='admin' OR COALESCE(p.can_view,false)=true ORDER BY c.name`, a.ID, a.Role)
	if err != nil {
		writeError(w, 500, "Could not load calendars")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id uuid.UUID
		var name, color, icon, description string
		var canEdit, canDelete bool
		if rows.Scan(&id, &name, &color, &icon, &description, &canEdit, &canDelete) == nil {
			if a.Role == "admin" {
				canEdit = true
				canDelete = true
			}
			out = append(out, map[string]any{"id": id, "name": name, "color": color, "icon": icon, "description": description, "can_edit": canEdit, "can_delete": canDelete})
		}
	}
	writeJSON(w, 200, out)
}

func (s *server) createCalendar(w http.ResponseWriter, r *http.Request) {
	var raw struct {
		Name        string      `json:"name"`
		Color       string      `json:"color"`
		Icon        string      `json:"icon"`
		Description string      `json:"description"`
		VisibleTo   []uuid.UUID `json:"visible_to"`
		EditableBy  []uuid.UUID `json:"editable_by"`
	}
	if decode(r, &raw) != nil {
		writeError(w, 400, "Check the calendar details")
		return
	}
	if strings.TrimSpace(raw.Name) == "" || !validColor(raw.Color) {
		writeError(w, 400, "Calendar name and color are required")
		return
	}
	if raw.Icon == "" {
		raw.Icon = "calendar"
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "Could not create calendar")
		return
	}
	defer tx.Rollback(r.Context())
	var id uuid.UUID
	if err = tx.QueryRow(r.Context(), `INSERT INTO calendars(name,color,icon,description,created_by) VALUES($1,$2,$3,$4,$5) RETURNING id`,
		cleanText(raw.Name, 100), raw.Color, cleanText(raw.Icon, 40), cleanText(raw.Description, 500), currentActor(r).ID).Scan(&id); err != nil {
		writeError(w, 500, "Could not create calendar")
		return
	}
	edit := map[uuid.UUID]bool{}
	for _, u := range raw.EditableBy {
		edit[u] = true
	}
	seen := map[uuid.UUID]bool{}
	for _, u := range append(raw.VisibleTo, raw.EditableBy...) {
		if seen[u] {
			continue
		}
		seen[u] = true
		_, err = tx.Exec(r.Context(), `INSERT INTO calendar_permissions(calendar_id,user_id,can_view,can_edit,can_delete) VALUES($1,$2,true,$3,$3)`, id, u, edit[u])
		if err != nil {
			writeError(w, 400, "One of the selected people is invalid")
			return
		}
	}
	if !seen[currentActor(r).ID] {
		_, err = tx.Exec(r.Context(), `INSERT INTO calendar_permissions(calendar_id,user_id,can_view,can_edit,can_delete) VALUES($1,$2,true,true,true)`, id, currentActor(r).ID)
		if err != nil {
			writeError(w, 500, "Could not grant calendar access")
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "Could not save calendar")
		return
	}
	writeJSON(w, 201, map[string]any{"id": id})
}

func (s *server) setCalendarPermissions(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "Invalid calendar")
		return
	}
	var raw struct {
		VisibleTo  []uuid.UUID `json:"visible_to"`
		EditableBy []uuid.UUID `json:"editable_by"`
	}
	if decode(r, &raw) != nil {
		writeError(w, 400, "Check the people selected")
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "Could not update access")
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), "DELETE FROM calendar_permissions WHERE calendar_id=$1", id); err != nil {
		writeError(w, 500, "Could not update access")
		return
	}
	edit := map[uuid.UUID]bool{}
	for _, u := range raw.EditableBy {
		edit[u] = true
	}
	seen := map[uuid.UUID]bool{}
	for _, u := range append(raw.VisibleTo, raw.EditableBy...) {
		if seen[u] {
			continue
		}
		seen[u] = true
		if _, err = tx.Exec(r.Context(), `INSERT INTO calendar_permissions(calendar_id,user_id,can_view,can_edit,can_delete) VALUES($1,$2,true,$3,$3)`, id, u, edit[u]); err != nil {
			writeError(w, 400, "One of the selected people is invalid")
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "Could not update access")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) listEvents(w http.ResponseWriter, r *http.Request) {
	a := currentActor(r)
	from := time.Now().AddDate(0, -1, 0)
	to := time.Now().AddDate(0, 3, 0)
	if v := r.URL.Query().Get("from"); v != "" {
		if t, e := time.Parse(time.RFC3339, v); e == nil {
			from = t
		}
	}
	if v := r.URL.Query().Get("to"); v != "" {
		if t, e := time.Parse(time.RFC3339, v); e == nil {
			to = t
		}
	}
	if !to.After(from) || to.Sub(from) > 370*24*time.Hour {
		writeError(w, 400, "Choose a calendar range of one year or less")
		return
	}

	rows, err := s.db.Query(r.Context(), `SELECT e.id,e.calendar_id,e.title,e.notes,e.location,e.starts_at,e.ends_at,e.all_day,e.status,
		c.name,c.color,er.frequency,er.interval_value,er.weekdays,er.until_at,er.occurrence_count
		FROM events e
		JOIN calendars c ON c.id=e.calendar_id
		LEFT JOIN calendar_permissions p ON p.calendar_id=c.id AND p.user_id=$1
		LEFT JOIN event_recurrence er ON er.event_id=e.id
		WHERE e.recurrence_parent_id IS NULL
		  AND ($2='admin' OR COALESCE(p.can_view,false)=true)
		  AND (
			(er.event_id IS NULL AND e.starts_at < $4 AND e.ends_at >= $3)
			OR
			(er.event_id IS NOT NULL AND e.starts_at < $4 AND (er.until_at IS NULL OR er.until_at >= $3))
		  )
		ORDER BY e.starts_at`, a.ID, a.Role, from, to)
	if err != nil {
		writeError(w, 500, "Could not load events")
		return
	}
	defer rows.Close()

	out := []map[string]any{}
	for rows.Next() {
		var id, calID uuid.UUID
		var title, notes, location, status, calName, color string
		var startAt, endAt time.Time
		var allDay bool
		var frequency *string
		var interval *int
		var weekdaysRaw []byte
		var until *time.Time
		var count *int

		if rows.Scan(&id, &calID, &title, &notes, &location, &startAt, &endAt, &allDay, &status, &calName, &color,
			&frequency, &interval, &weekdaysRaw, &until, &count) != nil {
			continue
		}

		assignees := s.eventAssignees(r, id)
		reminders := s.eventReminders(r, id)

		var rule *recurrence.Rule
		if frequency != nil && interval != nil {
			weekdays := []int{}
			if len(weekdaysRaw) > 0 {
				_ = json.Unmarshal(weekdaysRaw, &weekdays)
			}
			rule = recurrence.Normalize(&recurrence.Rule{
				Frequency: *frequency, Interval: *interval, Weekdays: weekdays,
				Until: until, OccurrenceCount: count,
			}, startAt)
		}

		exceptions := map[time.Time]bool{}
		if rule != nil {
			exRows, exErr := s.db.Query(r.Context(), `SELECT original_start
				FROM event_occurrence_exceptions WHERE event_id=$1`, id)
			if exErr == nil {
				for exRows.Next() {
					var original time.Time
					if exRows.Scan(&original) == nil {
						exceptions[original] = true
					}
				}
				exRows.Close()
			}
		}

		occurrences := recurrence.Expand(startAt, endAt, rule, from, to, 1500)
		for _, occurrence := range occurrences {
			if exceptions[occurrence.Start] {
				continue
			}
			out = append(out, map[string]any{
				"id": id, "series_id": id, "calendar_id": calID,
				"title": title, "notes": notes, "location": location,
				"starts_at": occurrence.Start, "ends_at": occurrence.End,
				"series_starts_at": startAt, "series_ends_at": endAt,
				"all_day": allDay, "status": status, "calendar_name": calName, "color": color,
				"assignees": assignees, "reminders": reminders, "recurrence": rule,
				"is_recurring": rule != nil, "is_occurrence_override": false,
				"occurrence_index": occurrence.Index, "occurrence_start": occurrence.Start,
			})
		}
	}
	if err := rows.Err(); err != nil {
		writeError(w, 500, "Could not finish loading events")
		return
	}

	overrideRows, err := s.db.Query(r.Context(), `SELECT
		parent.id,parent.calendar_id,parent.title,parent.notes,parent.location,parent.starts_at,parent.ends_at,parent.all_day,parent.status,
		pc.name,pc.color,
		er.frequency,er.interval_value,er.weekdays,er.until_at,er.occurrence_count,
		replacement.id,replacement.calendar_id,replacement.title,replacement.notes,replacement.location,
		replacement.starts_at,replacement.ends_at,replacement.all_day,replacement.status,
		rc.name,rc.color,replacement.recurrence_original_start
		FROM events replacement
		JOIN events parent ON parent.id=replacement.recurrence_parent_id
		JOIN event_recurrence er ON er.event_id=parent.id
		JOIN calendars pc ON pc.id=parent.calendar_id
		JOIN calendars rc ON rc.id=replacement.calendar_id
		LEFT JOIN calendar_permissions rp ON rp.calendar_id=rc.id AND rp.user_id=$1
		WHERE replacement.recurrence_parent_id IS NOT NULL
		  AND ($2='admin' OR COALESCE(rp.can_view,false)=true)
		  AND replacement.starts_at < $4
		  AND replacement.ends_at >= $3
		ORDER BY replacement.starts_at`, a.ID, a.Role, from, to)
	if err != nil {
		writeError(w, 500, "Could not load changed recurring occurrences")
		return
	}
	defer overrideRows.Close()

	for overrideRows.Next() {
		var parentID, parentCalID, replacementID, replacementCalID uuid.UUID
		var parentTitle, parentNotes, parentLocation, parentStatus, parentCalName, parentColor string
		var replacementTitle, replacementNotes, replacementLocation, replacementStatus, replacementCalName, replacementColor string
		var parentStart, parentEnd, replacementStart, replacementEnd, originalStart time.Time
		var parentAllDay, replacementAllDay bool
		var frequency string
		var interval int
		var weekdaysRaw []byte
		var until *time.Time
		var count *int

		if overrideRows.Scan(
			&parentID, &parentCalID, &parentTitle, &parentNotes, &parentLocation, &parentStart, &parentEnd, &parentAllDay, &parentStatus,
			&parentCalName, &parentColor,
			&frequency, &interval, &weekdaysRaw, &until, &count,
			&replacementID, &replacementCalID, &replacementTitle, &replacementNotes, &replacementLocation,
			&replacementStart, &replacementEnd, &replacementAllDay, &replacementStatus,
			&replacementCalName, &replacementColor, &originalStart,
		) != nil {
			continue
		}

		weekdays := []int{}
		if len(weekdaysRaw) > 0 {
			_ = json.Unmarshal(weekdaysRaw, &weekdays)
		}
		rule := recurrence.Normalize(&recurrence.Rule{
			Frequency: frequency, Interval: interval, Weekdays: weekdays,
			Until: until, OccurrenceCount: count,
		}, parentStart)

		out = append(out, map[string]any{
			"id": parentID, "series_id": parentID, "replacement_event_id": replacementID,
			"calendar_id": replacementCalID, "title": replacementTitle, "notes": replacementNotes, "location": replacementLocation,
			"starts_at": replacementStart, "ends_at": replacementEnd,
			"series_starts_at": parentStart, "series_ends_at": parentEnd,
			"all_day": replacementAllDay, "status": replacementStatus,
			"calendar_name": replacementCalName, "color": replacementColor,
			"assignees": s.eventAssignees(r, replacementID), "reminders": s.eventReminders(r, replacementID),
			"recurrence": rule, "is_recurring": true, "is_occurrence_override": true,
			"occurrence_start": originalStart,
			"series_calendar_id": parentCalID, "series_title": parentTitle, "series_notes": parentNotes,
			"series_location": parentLocation, "series_all_day": parentAllDay, "series_status": parentStatus,
			"series_calendar_name": parentCalName, "series_color": parentColor,
		})
	}
	if err := overrideRows.Err(); err != nil {
		writeError(w, 500, "Could not finish loading changed recurring occurrences")
		return
	}

	sort.SliceStable(out, func(i, j int) bool {
		left, _ := out[i]["starts_at"].(time.Time)
		right, _ := out[j]["starts_at"].(time.Time)
		return left.Before(right)
	})
	writeJSON(w, 200, out)
}

func (s *server) eventAssignees(r *http.Request, eventID uuid.UUID) []map[string]any {
	out := []map[string]any{}
	rows, err := s.db.Query(r.Context(), `SELECT u.id,u.display_name,u.initials,u.avatar_url
		FROM event_assignees ea JOIN users u ON u.id=ea.user_id
		WHERE ea.event_id=$1 AND u.active=true ORDER BY u.display_name`, eventID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var userID uuid.UUID
		var name, initials string
		var avatar *string
		if rows.Scan(&userID, &name, &initials, &avatar) == nil {
			out = append(out, map[string]any{
				"id": userID, "display_name": name, "initials": initials, "avatar_url": avatar,
			})
		}
	}
	return out
}

func (s *server) eventReminders(r *http.Request, eventID uuid.UUID) []map[string]any {
	out := []map[string]any{}
	rows, err := s.db.Query(r.Context(), `SELECT kind,provider,minutes_before,destination
		FROM reminders WHERE event_id=$1 AND enabled=true ORDER BY minutes_before DESC`, eventID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var kind, provider string
		var minutes int
		var destination *string
		if rows.Scan(&kind, &provider, &minutes, &destination) == nil {
			out = append(out, map[string]any{
				"kind": kind, "provider": provider, "minutes_before": minutes, "destination": destination,
			})
		}
	}
	return out
}

func (s *server) createEvent(w http.ResponseWriter, r *http.Request) {
	var in eventInput
	if decode(r, &in) != nil {
		writeError(w, 400, "Check the event details")
		return
	}
	if msg := validateEventInput(in); msg != "" {
		writeError(w, 400, msg)
		return
	}
	a := currentActor(r)
	if !s.canEditCalendar(r, in.CalendarID) {
		writeError(w, 403, "You cannot add events to this calendar")
		return
	}

	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "Could not create event")
		return
	}
	defer tx.Rollback(r.Context())

	var id uuid.UUID
	err = tx.QueryRow(r.Context(), `INSERT INTO events(calendar_id,title,notes,location,starts_at,ends_at,all_day,created_by)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,
		in.CalendarID, cleanText(in.Title, 200), cleanText(in.Notes, 5000), cleanText(in.Location, 500),
		in.StartsAt, in.EndsAt, in.AllDay, a.ID).Scan(&id)
	if err != nil {
		writeError(w, 400, "Could not create event")
		return
	}

	for _, uid := range in.AssigneeIDs {
		if _, err = tx.Exec(r.Context(), `INSERT INTO event_assignees(event_id,user_id) VALUES($1,$2)`, id, uid); err != nil {
			writeError(w, 400, "One of the selected people is invalid")
			return
		}
	}

	if in.Recurrence != nil {
		rule := recurrence.Normalize(in.Recurrence, in.StartsAt)
		weekdays, _ := json.Marshal(rule.Weekdays)
		if _, err = tx.Exec(r.Context(), `INSERT INTO event_recurrence(event_id,frequency,interval_value,weekdays,until_at,occurrence_count)
			VALUES($1,$2,$3,$4,$5,$6)`, id, rule.Frequency, rule.Interval, weekdays, rule.Until, rule.OccurrenceCount); err != nil {
			writeError(w, 400, "Could not save repeat settings")
			return
		}
	}

	for _, rm := range in.Reminders {
		if _, err = tx.Exec(r.Context(), `INSERT INTO reminders(event_id,kind,provider,minutes_before,destination)
			VALUES($1,$2,$3,$4,NULLIF($5,''))`, id, rm.Kind, rm.Provider, rm.MinutesBefore, cleanText(rm.Destination, 200)); err != nil {
			writeError(w, 400, "Could not save reminder")
			return
		}
	}

	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "Could not save event")
		return
	}
	writeJSON(w, 201, map[string]any{"id": id})
}

func validColor(v string) bool {
	if len(v) != 7 || v[0] != '#' {
		return false
	}
	for _, c := range v[1:] {
		if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return false
		}
	}
	return true
}
