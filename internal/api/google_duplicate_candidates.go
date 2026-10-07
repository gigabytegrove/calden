package api

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type googlePotentialDuplicateEvent struct {
	EventID      uuid.UUID
	CalendarID   uuid.UUID
	CalendarName string
	Title        string
	ExternalUID  string
	Start        time.Time
	End          time.Time
	AllDay       bool
	Frequency    string
	Interval     int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type googlePotentialDuplicateGroup struct {
	Key                 string
	Title               string
	CalendarID          uuid.UUID
	CalendarName        string
	Frequency           string
	Interval            int
	AllDay              bool
	Events              []googlePotentialDuplicateEvent
	RecommendedKeepID   uuid.UUID
}

func normalizeImportedEventTitle(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var out strings.Builder
	space := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			out.WriteRune(r)
			space = false
		case r >= '0' && r <= '9':
			out.WriteRune(r)
			space = false
		default:
			if !space {
				out.WriteByte(' ')
				space = true
			}
		}
	}
	return strings.Join(strings.Fields(out.String()), " ")
}

func potentialDuplicateSignature(event googlePotentialDuplicateEvent) string {
	duration := event.End.Sub(event.Start).Round(time.Second)
	return fmt.Sprintf("%s|%t|%s|%d|%s",
		normalizeImportedEventTitle(event.Title),
		event.AllDay,
		strings.ToLower(strings.TrimSpace(event.Frequency)),
		event.Interval,
		duration,
	)
}

func importedEventLooksRelated(events []googlePotentialDuplicateEvent) bool {
	if len(events) < 2 {
		return false
	}
	recurring := strings.TrimSpace(events[0].Frequency) != ""
	for i := 1; i < len(events); i++ {
		if (strings.TrimSpace(events[i].Frequency) != "") != recurring {
			return false
		}
	}
	if recurring {
		// Same imported title/calendar/recurrence shape is suspicious enough to
		// show to the administrator, but never safe enough to auto-delete.
		return true
	}
	// Non-recurring events are only candidates when their start times are close.
	minStart, maxStart := events[0].Start, events[0].Start
	for _, event := range events[1:] {
		if event.Start.Before(minStart) {
			minStart = event.Start
		}
		if event.Start.After(maxStart) {
			maxStart = event.Start
		}
	}
	return maxStart.Sub(minStart) <= 14*24*time.Hour
}

func newestImportedEvent(events []googlePotentialDuplicateEvent) uuid.UUID {
	if len(events) == 0 {
		return uuid.Nil
	}
	best := events[0]
	for _, event := range events[1:] {
		if event.CreatedAt.After(best.CreatedAt) ||
			(event.CreatedAt.Equal(best.CreatedAt) && event.UpdatedAt.After(best.UpdatedAt)) {
			best = event
		}
	}
	return best.EventID
}

func (s *server) googlePotentialDuplicateGroups(ctx context.Context, tx pgx.Tx) ([]googlePotentialDuplicateGroup, error) {
	rows, err := tx.Query(ctx, `SELECT
		e.id,e.calendar_id,c.name,e.title,e.external_uid,e.starts_at,e.ends_at,e.all_day,
		COALESCE(er.frequency,''),COALESCE(er.interval_value,0),
		e.created_at,e.updated_at
		FROM events e
		JOIN calendars c ON c.id=e.calendar_id
		LEFT JOIN event_recurrence er ON er.event_id=e.id
		WHERE e.external_uid IS NOT NULL
		  AND e.recurrence_parent_id IS NULL
		  AND (
			c.description='Imported from Google Calendar'
			OR EXISTS (
				SELECT 1 FROM calendar_import_sources cis
				WHERE cis.provider='google' AND cis.calendar_id=e.calendar_id
			)
		  )
		ORDER BY lower(c.name),lower(e.title),e.starts_at,e.created_at,e.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	grouped := map[string][]googlePotentialDuplicateEvent{}
	for rows.Next() {
		var event googlePotentialDuplicateEvent
		if err := rows.Scan(
			&event.EventID,&event.CalendarID,&event.CalendarName,&event.Title,&event.ExternalUID,
			&event.Start,&event.End,&event.AllDay,&event.Frequency,&event.Interval,
			&event.CreatedAt,&event.UpdatedAt,
		); err != nil {
			return nil, err
		}
		key := potentialDuplicateSignature(event)
		grouped[key] = append(grouped[key], event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	groups := []googlePotentialDuplicateGroup{}
	for key, events := range grouped {
		if !importedEventLooksRelated(events) {
			continue
		}
		sort.Slice(events, func(i, j int) bool {
			if events[i].Start.Equal(events[j].Start) {
				return events[i].CreatedAt.Before(events[j].CreatedAt)
			}
			return events[i].Start.Before(events[j].Start)
		})
		calendarName := events[0].CalendarName
		calendarID := events[0].CalendarID
		for _, event := range events[1:] {
			if event.CalendarID != calendarID {
				calendarID = uuid.Nil
				calendarName = "Multiple calendars"
				break
			}
		}
		group := googlePotentialDuplicateGroup{
			Key: key,
			Title: events[0].Title,
			CalendarID: calendarID,
			CalendarName: calendarName,
			Frequency: strings.ToLower(strings.TrimSpace(events[0].Frequency)),
			Interval: events[0].Interval,
			AllDay: events[0].AllDay,
			Events: events,
			RecommendedKeepID: newestImportedEvent(events),
		}
		groups = append(groups, group)
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].CalendarName == groups[j].CalendarName {
			return strings.ToLower(groups[i].Title) < strings.ToLower(groups[j].Title)
		}
		return strings.ToLower(groups[i].CalendarName) < strings.ToLower(groups[j].CalendarName)
	})
	return groups, nil
}

func googlePotentialDuplicateGroupJSON(group googlePotentialDuplicateGroup) map[string]any {
	events := make([]map[string]any, 0, len(group.Events))
	for _, event := range group.Events {
		events = append(events, map[string]any{
			"id": event.EventID,
			"calendar_id": event.CalendarID,
			"calendar_name": event.CalendarName,
			"title": event.Title,
			"external_uid": event.ExternalUID,
			"starts_at": event.Start,
			"ends_at": event.End,
			"all_day": event.AllDay,
			"frequency": event.Frequency,
			"interval": event.Interval,
			"created_at": event.CreatedAt,
			"updated_at": event.UpdatedAt,
			"recommended": event.EventID == group.RecommendedKeepID,
		})
	}
	return map[string]any{
		"key": group.Key,
		"title": group.Title,
		"calendar_id": group.CalendarID,
		"calendar_name": group.CalendarName,
		"frequency": group.Frequency,
		"interval": group.Interval,
		"all_day": group.AllDay,
		"recommended_keep_event_id": group.RecommendedKeepID,
		"events": events,
	}
}

func (s *server) listGooglePotentialDuplicates(w http.ResponseWriter, r *http.Request) {
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "Could not scan CalDen events")
		return
	}
	defer tx.Rollback(r.Context())
	groups, err := s.googlePotentialDuplicateGroups(r.Context(), tx)
	if err != nil {
		writeError(w, 500, "Could not scan CalDen events")
		return
	}
	items := make([]map[string]any, 0, len(groups))
	extra := 0
	for _, group := range groups {
		items = append(items, googlePotentialDuplicateGroupJSON(group))
		extra += len(group.Events) - 1
	}
	writeJSON(w, 200, map[string]any{
		"groups": items,
		"group_count": len(items),
		"extra_copies": extra,
	})
}

func samePotentialDuplicateGroup(group googlePotentialDuplicateGroup, ids map[uuid.UUID]bool) bool {
	if len(group.Events) != len(ids) {
		return false
	}
	for _, event := range group.Events {
		if !ids[event.EventID] {
			return false
		}
	}
	return true
}

func (s *server) resolveGooglePotentialDuplicates(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Resolutions []struct {
			KeepEventID   uuid.UUID   `json:"keep_event_id"`
			RemoveEventIDs []uuid.UUID `json:"remove_event_ids"`
		} `json:"resolutions"`
	}
	if decode(r, &in) != nil || len(in.Resolutions) == 0 {
		writeError(w, 400, "Choose at least one duplicate group to repair")
		return
	}

	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "Could not start duplicate repair")
		return
	}
	defer tx.Rollback(r.Context())
	groups, err := s.googlePotentialDuplicateGroups(r.Context(), tx)
	if err != nil {
		writeError(w, 500, "Could not rescan duplicate candidates")
		return
	}

	removed := 0
	for _, resolution := range in.Resolutions {
		if resolution.KeepEventID == uuid.Nil || len(resolution.RemoveEventIDs) == 0 {
			writeError(w, 400, "Each repair must choose one copy to keep and at least one to remove")
			return
		}
		ids := map[uuid.UUID]bool{resolution.KeepEventID:true}
		for _, id := range resolution.RemoveEventIDs {
			if id == uuid.Nil || id == resolution.KeepEventID || ids[id] {
				writeError(w, 400, "Duplicate repair selection is invalid")
				return
			}
			ids[id] = true
		}

		var matched *googlePotentialDuplicateGroup
		for i := range groups {
			if samePotentialDuplicateGroup(groups[i], ids) {
				matched = &groups[i]
				break
			}
		}
		if matched == nil {
			writeError(w, 409, "Those events are no longer the same duplicate group. Scan again before repairing.")
			return
		}

		var owner *googlePotentialDuplicateEvent
		for i := range matched.Events {
			if matched.Events[i].EventID == resolution.KeepEventID {
				owner = &matched.Events[i]
				break
			}
		}
		if owner == nil {
			writeError(w, 400, "The event selected to keep is not in this duplicate group")
			return
		}
		for _, removeID := range resolution.RemoveEventIDs {
			if err := s.mergeImportedEventRows(r.Context(), tx, owner.EventID, removeID, owner.CalendarID); err != nil {
				writeError(w, 500, "Could not merge duplicate event data")
				return
			}
			removed++
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "Could not finish duplicate repair")
		return
	}
	s.audit(r, "repair", "integration", nil, "Resolved Google-imported duplicate events selected by administrator", map[string]any{
		"groups": len(in.Resolutions), "removed_copies": removed,
	})
	writeJSON(w, 200, map[string]any{
		"groups_repaired": len(in.Resolutions),
		"removed_copies": removed,
	})
}
