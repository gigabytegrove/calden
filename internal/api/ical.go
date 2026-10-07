package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	calical "github.com/gigabytegrove/calden/internal/ical"
	"github.com/gigabytegrove/calden/internal/recurrence"
)

const maxICalendarUploadBytes int64 = 20 << 20

var filenameCleaner = regexp.MustCompile(`[^0-9A-Za-z._-]+`)

func (s *server) exportCalendarICS(w http.ResponseWriter, r *http.Request) {
	calendarID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "Invalid calendar")
		return
	}
	if !s.canViewCalendar(r, calendarID) {
		writeError(w, 403, "You cannot export this calendar")
		return
	}
	var calendarName string
	if err := s.db.QueryRow(r.Context(), `SELECT name FROM calendars WHERE id=$1`, calendarID).Scan(&calendarName); err != nil {
		writeError(w, 404, "Calendar not found")
		return
	}
	loc := s.householdLocation(r.Context())

	rows, err := s.db.Query(r.Context(), `SELECT
		e.id,e.external_uid,e.title,e.notes,e.location,e.starts_at,e.ends_at,e.all_day,
		cat.name,er.frequency,er.interval_value,er.weekdays,er.until_at,er.occurrence_count
		FROM events e
		LEFT JOIN categories cat ON cat.id=e.category_id
		LEFT JOIN event_recurrence er ON er.event_id=e.id
		WHERE e.calendar_id=$1 AND e.recurrence_parent_id IS NULL AND e.status<>'cancelled'
		ORDER BY e.starts_at`, calendarID)
	if err != nil {
		writeError(w, 500, "Could not export calendar")
		return
	}
	defer rows.Close()

	export := calical.Calendar{Name: calendarName}
	for rows.Next() {
		var id uuid.UUID
		var externalUID *string
		var title, notes, location string
		var startAt, endAt time.Time
		var allDay bool
		var category *string
		var frequency *string
		var interval *int
		var weekdaysRaw []byte
		var until *time.Time
		var count *int
		if rows.Scan(&id, &externalUID, &title, &notes, &location, &startAt, &endAt, &allDay,
			&category, &frequency, &interval, &weekdaysRaw, &until, &count) != nil {
			continue
		}
		uid := id.String() + "@calden"
		if externalUID != nil && strings.TrimSpace(*externalUID) != "" {
			uid = *externalUID
		}
		var rule *recurrence.Rule
		if frequency != nil && interval != nil {
			weekdays := []int{}
			_ = json.Unmarshal(weekdaysRaw, &weekdays)
			rule = recurrence.Normalize(&recurrence.Rule{
				Frequency: *frequency, Interval: *interval, Weekdays: weekdays,
				Until: until, OccurrenceCount: count,
			}, startAt)
		}
		base := calical.Event{
			UID: uid, Summary: title, Description: notes, Location: location,
			Start: startAt, End: endAt, AllDay: allDay, Recurrence: rule,
		}
		if category != nil {
			base.Category = *category
		}

		if rule != nil {
			exRows, exErr := s.db.Query(r.Context(), `SELECT original_start,cancelled,replacement_event_id
				FROM event_occurrence_exceptions WHERE event_id=$1 ORDER BY original_start`, id)
			if exErr == nil {
				for exRows.Next() {
					var original time.Time
					var cancelled bool
					var replacementID *uuid.UUID
					if exRows.Scan(&original, &cancelled, &replacementID) != nil {
						continue
					}
					if cancelled || replacementID == nil {
						base.ExDates = append(base.ExDates, original)
						continue
					}
					var override calical.Event
					override.UID = uid
					override.RecurrenceID = &original
					var overrideCategory *string
					if err := s.db.QueryRow(r.Context(), `SELECT e.title,e.notes,e.location,e.starts_at,e.ends_at,e.all_day,cat.name
						FROM events e LEFT JOIN categories cat ON cat.id=e.category_id WHERE e.id=$1`, *replacementID).
						Scan(&override.Summary, &override.Description, &override.Location, &override.Start, &override.End, &override.AllDay, &overrideCategory); err == nil {
						if overrideCategory != nil {
							override.Category = *overrideCategory
						}
						export.Events = append(export.Events, override)
					}
				}
				exRows.Close()
			}
		}
		export.Events = append(export.Events, base)
	}
	if err := rows.Err(); err != nil {
		writeError(w, 500, "Could not finish exporting calendar")
		return
	}

	// Parent events are appended after their overrides above. Sort so parents appear first for broad client compatibility.
	ordered := make([]calical.Event, 0, len(export.Events))
	overrides := make([]calical.Event, 0)
	for _, event := range export.Events {
		if event.RecurrenceID == nil {
			ordered = append(ordered, event)
		} else {
			overrides = append(overrides, event)
		}
	}
	export.Events = append(ordered, overrides...)

	filename := filenameCleaner.ReplaceAllString(strings.TrimSpace(calendarName), "-")
	filename = strings.Trim(filename, "-._")
	if filename == "" {
		filename = "calden-calendar"
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.ics"`, filename))
	w.Header().Set("Cache-Control", "no-store")
	if err := calical.Write(w, export, loc); err != nil {
		return
	}
	s.audit(r, "export", "calendar", &calendarID, "Exported calendar "+calendarName, map[string]any{"format": "ics"})
}

func (s *server) importCalendarICS(w http.ResponseWriter, r *http.Request) {
	calendarID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "Invalid calendar")
		return
	}
	if !s.canEditCalendar(r, calendarID) {
		writeError(w, 403, "You cannot import events into this calendar")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxICalendarUploadBytes+(2<<20))
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeError(w, 400, "Could not read the iCalendar upload")
		return
	}
	file, header, err := r.FormFile("calendar")
	if err != nil {
		writeError(w, 400, "Choose an iCalendar (.ics) file")
		return
	}
	defer file.Close()
	if header.Size <= 0 || header.Size > maxICalendarUploadBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "iCalendar file is too large")
		return
	}

	parsed, err := calical.Parse(file, s.householdLocation(r.Context()))
	if err != nil {
		writeError(w, 400, "Could not import iCalendar file: "+err.Error())
		return
	}
	if len(parsed.Events) == 0 {
		writeError(w, 400, "The iCalendar file does not contain any events")
		return
	}

	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "Could not begin calendar import")
		return
	}
	defer tx.Rollback(r.Context())

	type group struct {
		parent    *calical.Event
		overrides []calical.Event
	}
	groups := map[string]*group{}
	order := []string{}
	for i := range parsed.Events {
		event := parsed.Events[i]
		uid := strings.TrimSpace(event.UID)
		if uid == "" {
			uid = "import-" + uuid.NewString() + "@calden"
			event.UID = uid
		}
		g := groups[uid]
		if g == nil {
			g = &group{}
			groups[uid] = g
			order = append(order, uid)
		}
		if event.RecurrenceID == nil {
			if g.parent == nil {
				copy := event
				g.parent = &copy
			}
		} else {
			g.overrides = append(g.overrides, event)
		}
	}

	created, updated, exceptions, skipped := 0, 0, 0, 0
	for _, uid := range order {
		g := groups[uid]
		if g.parent == nil {
			skipped += len(g.overrides)
			continue
		}
		parent := *g.parent
		if parent.Cancelled {
			skipped++
			continue
		}
		categoryID, err := s.resolveImportCategory(r.Context(), tx, parent.Category, currentActor(r))
		if err != nil {
			writeError(w, 500, "Could not resolve imported category")
			return
		}

		var eventID uuid.UUID
		err = tx.QueryRow(r.Context(), `SELECT id FROM events
			WHERE calendar_id=$1 AND external_uid=$2 AND recurrence_parent_id IS NULL`, calendarID, uid).Scan(&eventID)
		exists := err == nil
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			writeError(w, 500, "Could not check existing imported event")
			return
		}
		if !exists {
			err = tx.QueryRow(r.Context(), `INSERT INTO events(
				calendar_id,category_id,external_uid,title,notes,location,starts_at,ends_at,all_day,created_by
			) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`,
				calendarID, categoryID, uid, cleanText(parent.Summary, 200), cleanText(parent.Description, 5000),
				cleanText(parent.Location, 500), parent.Start, parent.End, parent.AllDay, currentActor(r).ID).Scan(&eventID)
			if err != nil {
				writeError(w, 500, "Could not create imported event")
				return
			}
			created++
		} else {
			if _, err = tx.Exec(r.Context(), `UPDATE events SET category_id=$2,title=$3,notes=$4,location=$5,
				starts_at=$6,ends_at=$7,all_day=$8,status='confirmed',updated_at=now()
				WHERE id=$1`, eventID, categoryID, cleanText(parent.Summary, 200), cleanText(parent.Description, 5000),
				cleanText(parent.Location, 500), parent.Start, parent.End, parent.AllDay); err != nil {
				writeError(w, 500, "Could not update imported event")
				return
			}
			updated++
		}

		if _, err = tx.Exec(r.Context(), `DELETE FROM event_recurrence WHERE event_id=$1`, eventID); err != nil {
			writeError(w, 500, "Could not update imported recurrence")
			return
		}
		if parent.Recurrence != nil {
			rule := recurrence.Normalize(parent.Recurrence, parent.Start)
			weekdays, _ := json.Marshal(rule.Weekdays)
			if _, err = tx.Exec(r.Context(), `INSERT INTO event_recurrence(
				event_id,frequency,interval_value,weekdays,until_at,occurrence_count
			) VALUES($1,$2,$3,$4,$5,$6)`,
				eventID, rule.Frequency, rule.Interval, weekdays, rule.Until, rule.OccurrenceCount); err != nil {
				writeError(w, 500, "Could not save imported recurrence")
				return
			}
		}

		for _, ex := range parent.ExDates {
			if _, err = tx.Exec(r.Context(), `INSERT INTO event_occurrence_exceptions(
				event_id,original_start,cancelled,replacement_event_id,updated_at
			) VALUES($1,$2,true,NULL,now())
			ON CONFLICT(event_id,original_start) DO UPDATE SET cancelled=true,replacement_event_id=NULL,updated_at=now()`,
				eventID, ex); err != nil {
				writeError(w, 500, "Could not save imported exception")
				return
			}
			exceptions++
		}

		for _, override := range g.overrides {
			if override.RecurrenceID == nil {
				continue
			}
			if override.Cancelled {
				if _, err = tx.Exec(r.Context(), `INSERT INTO event_occurrence_exceptions(
					event_id,original_start,cancelled,replacement_event_id,updated_at
				) VALUES($1,$2,true,NULL,now())
				ON CONFLICT(event_id,original_start) DO UPDATE SET cancelled=true,replacement_event_id=NULL,updated_at=now()`,
					eventID, *override.RecurrenceID); err != nil {
					writeError(w, 500, "Could not save cancelled imported occurrence")
					return
				}
				exceptions++
				continue
			}

			overrideCategoryID, err := s.resolveImportCategory(r.Context(), tx, override.Category, currentActor(r))
			if err != nil {
				writeError(w, 500, "Could not resolve imported occurrence category")
				return
			}
			if overrideCategoryID == nil {
				overrideCategoryID = categoryID
			}
			var replacementID uuid.UUID
			err = tx.QueryRow(r.Context(), `SELECT replacement_event_id FROM event_occurrence_exceptions
				WHERE event_id=$1 AND original_start=$2 AND replacement_event_id IS NOT NULL`,
				eventID, *override.RecurrenceID).Scan(&replacementID)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				writeError(w, 500, "Could not load imported occurrence override")
				return
			}
			if replacementID == uuid.Nil {
				err = tx.QueryRow(r.Context(), `INSERT INTO events(
					calendar_id,category_id,title,notes,location,starts_at,ends_at,all_day,created_by,
					recurrence_parent_id,recurrence_original_start
				) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`,
					calendarID, overrideCategoryID, cleanText(override.Summary, 200), cleanText(override.Description, 5000),
					cleanText(override.Location, 500), override.Start, override.End, override.AllDay, currentActor(r).ID,
					eventID, *override.RecurrenceID).Scan(&replacementID)
			} else {
				_, err = tx.Exec(r.Context(), `UPDATE events SET calendar_id=$2,category_id=$3,title=$4,notes=$5,location=$6,
					starts_at=$7,ends_at=$8,all_day=$9,status='confirmed',updated_at=now() WHERE id=$1`,
					replacementID, calendarID, overrideCategoryID, cleanText(override.Summary, 200),
					cleanText(override.Description, 5000), cleanText(override.Location, 500),
					override.Start, override.End, override.AllDay)
			}
			if err != nil {
				writeError(w, 500, "Could not save imported occurrence override")
				return
			}
			if _, err = tx.Exec(r.Context(), `INSERT INTO event_occurrence_exceptions(
				event_id,original_start,cancelled,replacement_event_id,updated_at
			) VALUES($1,$2,false,$3,now())
			ON CONFLICT(event_id,original_start) DO UPDATE
			SET cancelled=false,replacement_event_id=$3,updated_at=now()`,
				eventID, *override.RecurrenceID, replacementID); err != nil {
				writeError(w, 500, "Could not link imported occurrence override")
				return
			}
			exceptions++
		}
	}

	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "Could not finish calendar import")
		return
	}
	s.audit(r, "import", "calendar", &calendarID, "Imported iCalendar file "+filepath.Base(header.Filename), map[string]any{
		"created": created, "updated": updated, "exceptions": exceptions, "skipped": skipped,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"created": created, "updated": updated, "exceptions": exceptions, "skipped": skipped,
	})
}

func (s *server) canViewCalendar(r *http.Request, calendarID uuid.UUID) bool {
	actor := currentActor(r)
	if actor.Role == "admin" {
		return true
	}
	var allowed bool
	_ = s.db.QueryRow(r.Context(), `SELECT COALESCE(can_view,false) FROM calendar_permissions
		WHERE calendar_id=$1 AND user_id=$2`, calendarID, actor.ID).Scan(&allowed)
	return allowed
}

func (s *server) householdLocation(ctx context.Context) *time.Location {
	var name string
	if err := s.db.QueryRow(ctx, `SELECT value FROM app_settings WHERE key='timezone'`).Scan(&name); err == nil {
		if loc, err := time.LoadLocation(name); err == nil {
			return loc
		}
	}
	return time.UTC
}

func (s *server) resolveImportCategory(ctx context.Context, tx pgx.Tx, name string, actor actor) (*uuid.UUID, error) {
	name = cleanText(strings.TrimSpace(name), 100)
	if name == "" {
		return nil, nil
	}
	var id uuid.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM categories WHERE active=true AND lower(name)=lower($1) LIMIT 1`, name).Scan(&id)
	if err == nil {
		return &id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if actor.Role != "admin" {
		return nil, nil
	}
	err = tx.QueryRow(ctx, `INSERT INTO categories(name,color,icon,description,created_by)
		VALUES($1,'#64748B','tag','Created from iCalendar import',$2)
		ON CONFLICT DO NOTHING RETURNING id`, name, actor.ID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := tx.QueryRow(ctx, `SELECT id FROM categories WHERE active=true AND lower(name)=lower($1) LIMIT 1`, name).Scan(&id); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	return &id, nil
}
