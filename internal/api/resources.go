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
	s.audit(r, "create", "user", &id, "Added "+cleanText(in.DisplayName, 100), map[string]any{"role": in.Role})
	writeJSON(w, 201, map[string]any{"id": id})
}

func hashPassword(password string) (string, error) { return bcryptHash(password) }

func (s *server) listCalendars(w http.ResponseWriter, r *http.Request) {
	a := currentActor(r)
	rows, err := s.db.Query(r.Context(), `SELECT c.id,c.name,c.color,c.icon,c.description,c.calendar_type,
		COALESCE(p.can_edit,false),COALESCE(p.can_delete,false),
		(SELECT count(*) FROM events e WHERE e.calendar_id=c.id AND e.recurrence_parent_id IS NULL)
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
		var name, color, icon, description, calendarType string
		var canEdit, canDelete bool
		var eventCount int
		if rows.Scan(&id, &name, &color, &icon, &description, &calendarType, &canEdit, &canDelete, &eventCount) == nil {
			if a.Role == "admin" {
				canEdit = true
				canDelete = true
			}
			out = append(out, map[string]any{"id": id, "name": name, "color": color, "icon": icon, "description": description, "calendar_type": calendarType, "can_edit": canEdit, "can_delete": canDelete, "event_count": eventCount})
		}
	}
	writeJSON(w, 200, out)
}

func (s *server) createCalendar(w http.ResponseWriter, r *http.Request) {
	var raw struct {
		Name        string      `json:"name"`
		Color       string      `json:"color"`
		Icon        string      `json:"icon"`
		Description  string      `json:"description"`
		CalendarType string      `json:"calendar_type"`
		VisibleTo    []uuid.UUID `json:"visible_to"`
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
	if raw.CalendarType == "" {
		raw.CalendarType = "standard"
	}
	if raw.CalendarType != "standard" && raw.CalendarType != "bill_pay" {
		writeError(w, 400, "Choose a valid calendar type")
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "Could not create calendar")
		return
	}
	defer tx.Rollback(r.Context())
	var id uuid.UUID
	if err = tx.QueryRow(r.Context(), `INSERT INTO calendars(name,color,icon,description,calendar_type,created_by) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`,
		cleanText(raw.Name, 100), raw.Color, cleanText(raw.Icon, 40), cleanText(raw.Description, 500), raw.CalendarType, currentActor(r).ID).Scan(&id); err != nil {
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
	s.audit(r, "create", "calendar", &id, "Created calendar "+cleanText(raw.Name, 100), map[string]any{"color": raw.Color})
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
	s.audit(r, "permissions", "calendar", &id, "Updated calendar access", map[string]any{"visible_count": len(raw.VisibleTo), "editor_count": len(raw.EditableBy)})
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

	rows, err := s.db.Query(r.Context(), `SELECT e.id,e.calendar_id,e.category_id,e.title,e.notes,e.location,e.starts_at,e.ends_at,e.all_day,e.status,e.request_confirmation,
		c.name,c.color,cat.name,cat.color,er.frequency,er.interval_value,er.weekdays,er.until_at,er.occurrence_count,COALESCE(er.raw_rule,'')
		FROM events e
		JOIN calendars c ON c.id=e.calendar_id
		LEFT JOIN categories cat ON cat.id=e.category_id
		LEFT JOIN calendar_permissions p ON p.calendar_id=c.id AND p.user_id=$1
		LEFT JOIN event_recurrence er ON er.event_id=e.id
		WHERE e.recurrence_parent_id IS NULL
		  AND ($2='admin' OR COALESCE(p.can_view,false)=true)
		  AND (
			(er.event_id IS NULL AND e.starts_at < $4 AND e.ends_at > $3)
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
		var categoryID *uuid.UUID
		var title, notes, location, status, calName, calendarColor string
		var categoryName, categoryColor *string
		var startAt, endAt time.Time
		var allDay bool
		var requestConfirmation bool
		var frequency *string
		var interval *int
		var weekdaysRaw []byte
		var until *time.Time
		var count *int
		var rawRule string

		if rows.Scan(&id, &calID, &categoryID, &title, &notes, &location, &startAt, &endAt, &allDay, &status, &requestConfirmation,
			&calName, &calendarColor, &categoryName, &categoryColor,
			&frequency, &interval, &weekdaysRaw, &until, &count, &rawRule) != nil {
			continue
		}

		assignees := s.eventAssignees(r, id)
		reminders := s.eventReminders(r, id)
		billDetails := s.eventBillDetails(r.Context(), id)

		var rule *recurrence.Rule
		if frequency != nil && interval != nil {
			weekdays := []int{}
			if len(weekdaysRaw) > 0 {
				_ = json.Unmarshal(weekdaysRaw, &weekdays)
			}
			rule = recurrence.Normalize(&recurrence.Rule{
				Frequency: *frequency, Interval: *interval, Weekdays: weekdays,
				Until: until, OccurrenceCount: count, Raw: rawRule,
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
			displayColor := calendarColor
			item := map[string]any{
				"id": id, "series_id": id, "calendar_id": calID, "category_id": categoryID,
				"category_name": categoryName, "category_color": categoryColor, "calendar_color": calendarColor,
				"title": title, "notes": notes, "location": location,
				"starts_at": occurrence.Start, "ends_at": occurrence.End,
				"series_starts_at": startAt, "series_ends_at": endAt,
				"all_day": allDay, "status": status, "request_confirmation": requestConfirmation, "calendar_name": calName, "color": displayColor,
				"assignees": assignees, "reminders": reminders, "recurrence": rule,
				"is_recurring": rule != nil, "is_occurrence_override": false,
				"occurrence_index": occurrence.Index, "occurrence_start": occurrence.Start,
			}
			addBillFields(item, "", billDetails)
			addBillPaymentFields(item, s.eventBillPayment(r.Context(), id, occurrence.Start))
			out = append(out, item)
		}
	}
	if err := rows.Err(); err != nil {
		writeError(w, 500, "Could not finish loading events")
		return
	}

	overrideRows, err := s.db.Query(r.Context(), `SELECT
		parent.id,parent.calendar_id,parent.category_id,parent.title,parent.notes,parent.location,parent.starts_at,parent.ends_at,parent.all_day,parent.status,parent.request_confirmation,
		pc.name,pc.color,pcat.name,pcat.color,
		er.frequency,er.interval_value,er.weekdays,er.until_at,er.occurrence_count,COALESCE(er.raw_rule,''),
		replacement.id,replacement.calendar_id,replacement.category_id,replacement.title,replacement.notes,replacement.location,
		replacement.starts_at,replacement.ends_at,replacement.all_day,replacement.status,replacement.request_confirmation,
		rc.name,rc.color,rcat.name,rcat.color,replacement.recurrence_original_start
		FROM events replacement
		JOIN events parent ON parent.id=replacement.recurrence_parent_id
		JOIN event_recurrence er ON er.event_id=parent.id
		JOIN calendars pc ON pc.id=parent.calendar_id
		LEFT JOIN categories pcat ON pcat.id=parent.category_id
		JOIN calendars rc ON rc.id=replacement.calendar_id
		LEFT JOIN categories rcat ON rcat.id=replacement.category_id
		LEFT JOIN calendar_permissions rp ON rp.calendar_id=rc.id AND rp.user_id=$1
		WHERE replacement.recurrence_parent_id IS NOT NULL
		  AND ($2='admin' OR COALESCE(rp.can_view,false)=true)
		  AND replacement.starts_at < $4
		  AND replacement.ends_at > $3
		ORDER BY replacement.starts_at`, a.ID, a.Role, from, to)
	if err != nil {
		writeError(w, 500, "Could not load changed recurring occurrences")
		return
	}
	defer overrideRows.Close()

	for overrideRows.Next() {
		var parentID, parentCalID, replacementID, replacementCalID uuid.UUID
		var parentCategoryID, replacementCategoryID *uuid.UUID
		var parentTitle, parentNotes, parentLocation, parentStatus, parentCalName, parentCalendarColor string
		var parentCategoryName, parentCategoryColor *string
		var replacementTitle, replacementNotes, replacementLocation, replacementStatus, replacementCalName, replacementCalendarColor string
		var replacementCategoryName, replacementCategoryColor *string
		var parentStart, parentEnd, replacementStart, replacementEnd, originalStart time.Time
		var parentAllDay, replacementAllDay, parentRequestConfirmation, replacementRequestConfirmation bool
		var frequency string
		var interval int
		var weekdaysRaw []byte
		var until *time.Time
		var count *int
		var rawRule string

		if overrideRows.Scan(
			&parentID, &parentCalID, &parentCategoryID, &parentTitle, &parentNotes, &parentLocation, &parentStart, &parentEnd, &parentAllDay, &parentStatus, &parentRequestConfirmation,
			&parentCalName, &parentCalendarColor, &parentCategoryName, &parentCategoryColor,
			&frequency, &interval, &weekdaysRaw, &until, &count, &rawRule,
			&replacementID, &replacementCalID, &replacementCategoryID, &replacementTitle, &replacementNotes, &replacementLocation,
			&replacementStart, &replacementEnd, &replacementAllDay, &replacementStatus, &replacementRequestConfirmation,
			&replacementCalName, &replacementCalendarColor, &replacementCategoryName, &replacementCategoryColor, &originalStart,
		) != nil {
			continue
		}

		weekdays := []int{}
		if len(weekdaysRaw) > 0 {
			_ = json.Unmarshal(weekdaysRaw, &weekdays)
		}
		rule := recurrence.Normalize(&recurrence.Rule{
			Frequency: frequency, Interval: interval, Weekdays: weekdays,
			Until: until, OccurrenceCount: count, Raw: rawRule,
		}, parentStart)

		replacementDisplayColor := replacementCalendarColor
		parentDisplayColor := parentCalendarColor
		item := map[string]any{
			"id": parentID, "series_id": parentID, "replacement_event_id": replacementID,
			"calendar_id": replacementCalID, "category_id": replacementCategoryID,
			"category_name": replacementCategoryName, "category_color": replacementCategoryColor, "calendar_color": replacementCalendarColor,
			"title": replacementTitle, "notes": replacementNotes, "location": replacementLocation,
			"starts_at": replacementStart, "ends_at": replacementEnd,
			"series_starts_at": parentStart, "series_ends_at": parentEnd,
			"all_day": replacementAllDay, "status": replacementStatus, "request_confirmation": replacementRequestConfirmation,
			"calendar_name": replacementCalName, "color": replacementDisplayColor,
			"assignees": s.eventAssignees(r, replacementID), "reminders": s.eventReminders(r, replacementID),
			"recurrence": rule, "is_recurring": true, "is_occurrence_override": true,
			"occurrence_start": originalStart,
			"series_calendar_id": parentCalID, "series_category_id": parentCategoryID,
			"series_category_name": parentCategoryName, "series_category_color": parentCategoryColor,
			"series_title": parentTitle, "series_notes": parentNotes,
			"series_location": parentLocation, "series_all_day": parentAllDay, "series_status": parentStatus, "series_request_confirmation": parentRequestConfirmation,
			"series_calendar_name": parentCalName, "series_color": parentDisplayColor,
			"series_assignees": s.eventAssignees(r, parentID), "series_reminders": s.eventReminders(r, parentID),
		}
		addBillFields(item, "", s.eventBillDetails(r.Context(), replacementID))
		addBillFields(item, "series_", s.eventBillDetails(r.Context(), parentID))
		addBillPaymentFields(item, s.eventBillPayment(r.Context(), parentID, originalStart))
		out = append(out, item)
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
	rows, err := s.db.Query(r.Context(), `SELECT kind,provider,minutes_before,destination,recipient_user_id
		FROM reminders WHERE event_id=$1 AND enabled=true ORDER BY minutes_before DESC`, eventID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var kind, provider string
		var minutes int
		var destination *string
		var recipient *uuid.UUID
		if rows.Scan(&kind, &provider, &minutes, &destination, &recipient) == nil {
			out = append(out, map[string]any{
				"kind": kind, "provider": provider, "minutes_before": minutes, "destination": destination, "recipient_user_id": recipient,
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
	err = tx.QueryRow(r.Context(), `INSERT INTO events(calendar_id,category_id,title,notes,location,starts_at,ends_at,all_day,created_by,request_confirmation)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,COALESCE($10,false)) RETURNING id`,
		in.CalendarID, in.CategoryID, cleanText(in.Title, 200), cleanText(in.Notes, 5000), cleanText(in.Location, 500),
		in.StartsAt, in.EndsAt, in.AllDay, a.ID, in.RequestConfirmation).Scan(&id)
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
	if err = s.saveBillDetails(r.Context(), tx, id, in.CalendarID, in); err != nil {
		writeError(w, 400, "Could not save bill details")
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

	for _, rm := range in.Reminders {
		if _, err = tx.Exec(r.Context(), `INSERT INTO reminders(event_id,kind,provider,minutes_before,destination,recipient_user_id)
			VALUES($1,$2,$3,$4,NULLIF($5,''),$6)`, id, rm.Kind, rm.Provider, rm.MinutesBefore, cleanText(rm.Destination, 200), rm.RecipientUserID); err != nil {
			writeError(w, 400, "Could not save reminder")
			return
		}
	}

	if err = s.createEventNotifications(r.Context(), tx, id, in.CalendarID, in.Title, in.AssigneeIDs); err != nil {
		writeError(w, 500, "Could not create event notifications")
		return
	}

	if in.RequestConfirmation != nil && *in.RequestConfirmation {
		for _, person := range in.AssigneeIDs {
			// No self-confirmation: only appointments assigned by someone else.
			if person == a.ID { continue }
			_, err = tx.Exec(r.Context(), `INSERT INTO notifications(user_id,event_id,kind,title,message,occurrence_start)
				SELECT u.id,$2,'event_confirmation_request',$3,'Please confirm this appointment or request a change.',$4
				FROM users u LEFT JOIN calendar_permissions p ON p.user_id=u.id AND p.calendar_id=$5
				WHERE u.id=$1 AND u.active AND (u.role='admin' OR COALESCE(p.can_view,false))`,
				person, id, cleanText(in.Title, 200), in.StartsAt, in.CalendarID)
			if err != nil { writeError(w, 500, "Could not create confirmation request"); return }
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "Could not save event")
		return
	}
	s.audit(r, "create", "event", &id, "Created event "+cleanText(in.Title, 200), map[string]any{
		"calendar_id": in.CalendarID, "category_id": in.CategoryID, "starts_at": in.StartsAt, "recurring": in.Recurrence != nil,
	})
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
