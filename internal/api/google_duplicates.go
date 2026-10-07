package api

import (
	"context"
	"net/http"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type googleDuplicateEventRow struct {
	EventID      uuid.UUID
	CalendarID   uuid.UUID
	Title        string
	CalendarName string
	Mapped       bool
}

type googleDuplicateGroup struct {
	UID    string
	Events []googleDuplicateEventRow
}

func (s *server) googleDuplicateGroups(ctx context.Context, tx pgx.Tx) ([]googleDuplicateGroup, error) {
	rows, err := tx.Query(ctx, `SELECT e.external_uid,e.id,e.calendar_id,e.title,c.name,
		EXISTS(
			SELECT 1 FROM calendar_import_sources cis
			WHERE cis.provider='google' AND cis.calendar_id=e.calendar_id
		)
		FROM events e
		JOIN calendars c ON c.id=e.calendar_id
		JOIN (
			SELECT de.external_uid
			FROM events de
			JOIN calendars dc ON dc.id=de.calendar_id
			WHERE de.external_uid IS NOT NULL
			  AND de.recurrence_parent_id IS NULL
			  AND (
				dc.description='Imported from Google Calendar'
				OR EXISTS (
					SELECT 1 FROM calendar_import_sources cis
					WHERE cis.provider='google' AND cis.calendar_id=de.calendar_id
				)
			  )
			GROUP BY de.external_uid
			HAVING count(*) > 1
		) d ON d.external_uid=e.external_uid
		WHERE e.recurrence_parent_id IS NULL
		  AND (
			c.description='Imported from Google Calendar'
			OR EXISTS (
				SELECT 1 FROM calendar_import_sources cis
				WHERE cis.provider='google' AND cis.calendar_id=e.calendar_id
			)
		  )
		ORDER BY e.external_uid,lower(c.name),e.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	groups := []googleDuplicateGroup{}
	var current *googleDuplicateGroup
	for rows.Next() {
		var uid string
		var row googleDuplicateEventRow
		if err := rows.Scan(&uid, &row.EventID, &row.CalendarID, &row.Title, &row.CalendarName, &row.Mapped); err != nil {
			return nil, err
		}
		if current == nil || current.UID != uid {
			groups = append(groups, googleDuplicateGroup{UID: uid})
			current = &groups[len(groups)-1]
		}
		current.Events = append(current.Events, row)
	}
	return groups, rows.Err()
}

func safeGoogleDuplicateOwner(group googleDuplicateGroup) (*googleDuplicateEventRow, bool) {
	var owner *googleDuplicateEventRow
	for i := range group.Events {
		event := &group.Events[i]
		if !event.Mapped {
			continue
		}
		if owner != nil {
			return nil, false
		}
		copy := *event
		owner = &copy
	}
	return owner, owner != nil
}

func googleDuplicateSummary(groups []googleDuplicateGroup) map[string]any {
	safeGroups := 0
	exportNeeded := 0
	copies := 0
	examples := []map[string]any{}
	for _, group := range groups {
		copies += len(group.Events) - 1
		if _, safe := safeGoogleDuplicateOwner(group); safe {
			safeGroups++
		} else {
			exportNeeded++
		}
		if len(examples) < 8 {
			calendars := []string{}
			seen := map[string]bool{}
			for _, event := range group.Events {
				if !seen[event.CalendarName] {
					calendars = append(calendars, event.CalendarName)
					seen[event.CalendarName] = true
				}
			}
			sort.Strings(calendars)
			title := ""
			if len(group.Events) > 0 {
				title = group.Events[0].Title
			}
			examples = append(examples, map[string]any{
				"uid": group.UID, "title": title, "copies": len(group.Events),
				"calendars": calendars,
			})
		}
	}
	return map[string]any{
		"duplicate_groups": len(groups),
		"extra_copies":     copies,
		"safe_groups":      safeGroups,
		"export_needed":    exportNeeded,
		"examples":         examples,
	}
}

func (s *server) scanGoogleDuplicates(w http.ResponseWriter, r *http.Request) {
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "Could not scan imported events")
		return
	}
	defer tx.Rollback(r.Context())
	groups, err := s.googleDuplicateGroups(r.Context(), tx)
	if err != nil {
		writeError(w, 500, "Could not scan imported events")
		return
	}
	writeJSON(w, 200, googleDuplicateSummary(groups))
}

func (s *server) mergeImportedEventRows(ctx context.Context, tx pgx.Tx, ownerEvent, loserEvent, ownerCalendar uuid.UUID) error {
	if ownerEvent == loserEvent {
		return nil
	}
	if _, err := tx.Exec(ctx, `INSERT INTO event_assignees(event_id,user_id)
		SELECT $1,user_id FROM event_assignees WHERE event_id=$2
		ON CONFLICT(event_id,user_id) DO NOTHING`, ownerEvent, loserEvent); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO reminders(event_id,kind,provider,minutes_before,destination,enabled)
		SELECT $1,r.kind,r.provider,r.minutes_before,r.destination,r.enabled
		FROM reminders r
		WHERE r.event_id=$2
		  AND NOT EXISTS (
			SELECT 1 FROM reminders existing
			WHERE existing.event_id=$1
			  AND existing.kind=r.kind
			  AND existing.provider=r.provider
			  AND existing.minutes_before=r.minutes_before
			  AND COALESCE(existing.destination,'')=COALESCE(r.destination,'')
			  AND existing.enabled=r.enabled
		  )`, ownerEvent, loserEvent); err != nil {
		return err
	}

	var ownerCalendarType string
	if err := tx.QueryRow(ctx, `SELECT calendar_type FROM calendars WHERE id=$1`, ownerCalendar).Scan(&ownerCalendarType); err != nil {
		return err
	}
	if ownerCalendarType == "bill_pay" {
		if _, err := tx.Exec(ctx, `INSERT INTO bill_event_details(event_id,amount_due,amount_is_estimate,payer_user_id,updated_at)
			SELECT $1,amount_due,amount_is_estimate,payer_user_id,updated_at
			FROM bill_event_details WHERE event_id=$2
			ON CONFLICT(event_id) DO NOTHING`, ownerEvent, loserEvent); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO bill_payments(
				event_id,occurrence_start,paid_by_user_id,amount_paid,paid_on,cleared_on,settles_bill,paid_at,updated_at
			)
			SELECT $1,p.occurrence_start,p.paid_by_user_id,p.amount_paid,p.paid_on,p.cleared_on,p.settles_bill,p.paid_at,p.updated_at
			FROM bill_payments p
			WHERE p.event_id=$2
			  AND NOT EXISTS (
				SELECT 1 FROM bill_payments existing
				WHERE existing.event_id=$1
				  AND existing.occurrence_start=p.occurrence_start
				  AND existing.paid_by_user_id IS NOT DISTINCT FROM p.paid_by_user_id
				  AND existing.amount_paid IS NOT DISTINCT FROM p.amount_paid
				  AND existing.paid_on=p.paid_on
				  AND existing.cleared_on IS NOT DISTINCT FROM p.cleared_on
				  AND existing.settles_bill=p.settles_bill
			  )`, ownerEvent, loserEvent); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO bill_no_balance_occurrences(
				event_id,occurrence_start,marked_by_user_id,marked_at
			)
			SELECT $1,occurrence_start,marked_by_user_id,marked_at
			FROM bill_no_balance_occurrences WHERE event_id=$2
			ON CONFLICT(event_id,occurrence_start) DO NOTHING`, ownerEvent, loserEvent); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO bill_allocations(
				event_id,occurrence_start,amount_allocated,allocated_on,allocated_by_user_id,allocated_at,updated_at
			)
			SELECT $1,occurrence_start,amount_allocated,allocated_on,allocated_by_user_id,allocated_at,updated_at
			FROM bill_allocations WHERE event_id=$2
			ON CONFLICT(event_id,occurrence_start) DO NOTHING`, ownerEvent, loserEvent); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE notifications SET event_id=$1 WHERE event_id=$2`, ownerEvent, loserEvent); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `DELETE FROM events WHERE id=$1`, loserEvent)
	return err
}

func (s *server) repairSafeGoogleDuplicates(w http.ResponseWriter, r *http.Request) {
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "Could not start duplicate repair")
		return
	}
	defer tx.Rollback(r.Context())

	groups, err := s.googleDuplicateGroups(r.Context(), tx)
	if err != nil {
		writeError(w, 500, "Could not scan imported events")
		return
	}
	repaired := 0
	for _, group := range groups {
		owner, safe := safeGoogleDuplicateOwner(group)
		if !safe {
			continue
		}
		for _, duplicate := range group.Events {
			if duplicate.EventID == owner.EventID {
				continue
			}
			if err := s.mergeImportedEventRows(r.Context(), tx, owner.EventID, duplicate.EventID, owner.CalendarID); err != nil {
				writeError(w, 500, "Could not repair imported duplicates")
				return
			}
			repaired++
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "Could not finish duplicate repair")
		return
	}
	s.audit(r, "repair", "integration", nil, "Repaired safe Google Calendar duplicates", map[string]any{"removed_copies": repaired})

	checkTx, err := s.db.Begin(r.Context())
	if err != nil {
		writeJSON(w, 200, map[string]any{"removed_copies": repaired})
		return
	}
	defer checkTx.Rollback(r.Context())
	remaining, _ := s.googleDuplicateGroups(r.Context(), checkTx)
	out := googleDuplicateSummary(remaining)
	out["removed_copies"] = repaired
	writeJSON(w, 200, out)
}

func googleUIDOwnerSources(bundles []googleCalendarBundle) (map[string]string, map[string]bool) {
	owners := map[string]string{}
	ambiguous := map[string]bool{}
	for _, bundle := range bundles {
		for _, event := range bundle.Calendar.Events {
			uid := strings.TrimSpace(event.UID)
			if uid == "" {
				continue
			}
			if existing := owners[uid]; existing != "" && existing != bundle.ExternalID {
				ambiguous[uid] = true
			} else if !ambiguous[uid] {
				owners[uid] = bundle.ExternalID
			}
		}
		for uid, ownerSource := range bundle.SuppressedDuplicateUIDs {
			if ambiguous[uid] {
				continue
			}
			if existing := owners[uid]; existing != "" && existing != ownerSource {
				ambiguous[uid] = true
				delete(owners, uid)
				continue
			}
			owners[uid] = ownerSource
		}
	}
	return owners, ambiguous
}

func (s *server) exportRepairableGoogleDuplicates(
	ctx context.Context,
	tx pgx.Tx,
	bundles []googleCalendarBundle,
	groups []googleDuplicateGroup,
	doRepair bool,
) (int, int, error) {
	owners, ambiguous := googleUIDOwnerSources(bundles)
	sourceCalendars := map[string]uuid.UUID{}
	rows, err := tx.Query(ctx, `SELECT external_id,calendar_id
		FROM calendar_import_sources WHERE provider='google'`)
	if err != nil {
		return 0, 0, err
	}
	for rows.Next() {
		var source string
		var calendarID uuid.UUID
		if err := rows.Scan(&source, &calendarID); err != nil {
			rows.Close()
			return 0, 0, err
		}
		sourceCalendars[source] = calendarID
	}
	rows.Close()

	repairable := 0
	removed := 0
	for _, group := range groups {
		if ambiguous[group.UID] {
			continue
		}
		source := owners[group.UID]
		ownerCalendar := sourceCalendars[source]
		if source == "" || ownerCalendar == uuid.Nil {
			continue
		}
		var owner *googleDuplicateEventRow
		for i := range group.Events {
			if group.Events[i].CalendarID == ownerCalendar {
				copy := group.Events[i]
				owner = &copy
				break
			}
		}
		if owner == nil {
			continue
		}
		repairable++
		if !doRepair {
			continue
		}
		for _, duplicate := range group.Events {
			if duplicate.EventID == owner.EventID {
				continue
			}
			if err := s.mergeImportedEventRows(ctx, tx, owner.EventID, duplicate.EventID, owner.CalendarID); err != nil {
				return repairable, removed, err
			}
			removed++
		}
	}
	return repairable, removed, nil
}

func (s *server) previewGoogleDuplicateRepair(w http.ResponseWriter, r *http.Request) {
	bundles, err := s.googleBundlesFromRequest(w, r)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "Could not scan imported events")
		return
	}
	defer tx.Rollback(r.Context())
	groups, err := s.googleDuplicateGroups(r.Context(), tx)
	if err != nil {
		writeError(w, 500, "Could not scan imported events")
		return
	}
	repairable, _, err := s.exportRepairableGoogleDuplicates(r.Context(), tx, bundles, groups, false)
	if err != nil {
		writeError(w, 500, "Could not compare the export with existing imported events")
		return
	}
	out := googleDuplicateSummary(groups)
	out["export_repairable_groups"] = repairable
	writeJSON(w, 200, out)
}

func (s *server) repairGoogleDuplicatesWithExport(w http.ResponseWriter, r *http.Request) {
	bundles, err := s.googleBundlesFromRequest(w, r)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "Could not start duplicate repair")
		return
	}
	defer tx.Rollback(r.Context())
	groups, err := s.googleDuplicateGroups(r.Context(), tx)
	if err != nil {
		writeError(w, 500, "Could not scan imported events")
		return
	}

	_, removed, err := s.exportRepairableGoogleDuplicates(r.Context(), tx, bundles, groups, true)
	if err != nil {
		writeError(w, 500, "Could not repair imported duplicates")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "Could not finish duplicate repair")
		return
	}
	s.audit(r, "repair", "integration", nil, "Repaired Google Calendar duplicates using export revision data", map[string]any{"removed_copies": removed})

	checkTx, err := s.db.Begin(r.Context())
	if err != nil {
		writeJSON(w, 200, map[string]any{"removed_copies": removed})
		return
	}
	defer checkTx.Rollback(r.Context())
	remaining, _ := s.googleDuplicateGroups(r.Context(), checkTx)
	out := googleDuplicateSummary(remaining)
	out["removed_copies"] = removed
	writeJSON(w, 200, out)
}
