package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	calical "github.com/gigabytegrove/calden/internal/ical"
)

const (
	maxGoogleCalendarUploadBytes   int64 = 100 << 20
	maxGoogleCalendarExpandedBytes int64 = 250 << 20
	maxGoogleCalendarFiles               = 100
	googleMappingCreate                  = "__create__"
	googleMappingSkip                    = "__skip__"
)

var googleCalendarPalette = []string{
	"#2563EB", "#16A34A", "#7C3AED", "#DB2777",
	"#EA580C", "#0891B2", "#4F46E5", "#65A30D",
	"#C2410C", "#0F766E", "#9333EA", "#475569",
}

type googleCalendarBundle struct {
	ExternalID                 string
	Name                       string
	Calendar                   calical.Calendar
	SuppressedDuplicateUIDs    map[string]string
	SuppressedDuplicateEvents  int
}

type googleCalendarImportItem struct {
	CalendarID      uuid.UUID `json:"calendar_id"`
	Name            string    `json:"name"`
	CreatedCalendar bool      `json:"created_calendar"`
	Created         int       `json:"created"`
	Updated         int       `json:"updated"`
	Exceptions      int       `json:"exceptions"`
	Skipped         int       `json:"skipped"`
}

type googleCalendarCandidate struct {
	ID          uuid.UUID
	Name        string
	Description string
}

type googleCalendarPreviewItem struct {
	ExternalID            string     `json:"external_id"`
	Name                  string     `json:"name"`
	EventCount            int        `json:"event_count"`
	DuplicateEventsSuppressed int     `json:"duplicate_events_suppressed"`
	SuggestedCalendarID   *uuid.UUID `json:"suggested_calendar_id"`
	SuggestedCalendarName string     `json:"suggested_calendar_name,omitempty"`
	MatchScore            int        `json:"match_score"`
	MatchReason           string     `json:"match_reason"`
	PreviousCalendarID    *uuid.UUID `json:"previous_calendar_id,omitempty"`
	PreviousCalendarName  string     `json:"previous_calendar_name,omitempty"`
	PreviousAutoCreated   bool       `json:"previous_auto_created"`
}

func (s *server) previewGoogleCalendarExport(w http.ResponseWriter, r *http.Request) {
	bundles, err := s.googleBundlesFromRequest(w, r)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}

	candidates, err := s.googleCalendarCandidates(r.Context())
	if err != nil {
		writeError(w, 500, "Could not load existing CalDen calendars")
		return
	}

	items := make([]googleCalendarPreviewItem, 0, len(bundles))
	for _, bundle := range bundles {
		item, err := s.previewGoogleCalendarBundle(r.Context(), bundle, candidates)
		if err != nil {
			writeError(w, 500, "Could not match Google calendars")
			return
		}
		items = append(items, item)
	}

	duplicateSuppressed := 0
	for _, bundle := range bundles {
		duplicateSuppressed += bundle.SuppressedDuplicateEvents
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"calendars":                   items,
		"calendar_count":              len(items),
		"duplicate_events_suppressed": duplicateSuppressed,
	})
}

func (s *server) importGoogleCalendarExport(w http.ResponseWriter, r *http.Request) {
	bundles, err := s.googleBundlesFromRequest(w, r)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}

	mapping := map[string]string{}
	mappingProvided := false
	if raw := strings.TrimSpace(r.FormValue("calendar_mapping")); raw != "" {
		if err := json.Unmarshal([]byte(raw), &mapping); err != nil {
			writeError(w, 400, "Calendar mapping is invalid. Review the Google calendars again.")
			return
		}
		mappingProvided = true
	}

	reuseByName := !strings.EqualFold(strings.TrimSpace(r.FormValue("reuse_by_name")), "false")
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "Could not begin Google Calendar import")
		return
	}
	defer tx.Rollback(r.Context())

	importActor := currentActor(r)
	items := make([]googleCalendarImportItem, 0, len(bundles))
	totalCreated, totalUpdated, totalExceptions, totalSkipped := 0, 0, 0, 0
	createdCalendars, reusedCalendars, skippedCalendars, cleanedCalendars := 0, 0, 0, 0
	duplicateEventsSuppressed, duplicateEventsReconciled := 0, 0
	processedSources := map[string]bool{}
	warnings := []string{}
	for _, bundle := range bundles {
		duplicateEventsSuppressed += bundle.SuppressedDuplicateEvents
	}

	for i, bundle := range bundles {
		choice := ""
		if mappingProvided {
			var ok bool
			choice, ok = mapping[bundle.ExternalID]
			if !ok {
				writeError(w, 400, "Every Google calendar must be mapped, skipped, or set to create a new calendar")
				return
			}
			choice = strings.TrimSpace(choice)
			if choice == googleMappingSkip {
				skippedCalendars++
				continue
			}
		}

		calendarID, createdCalendar, previousCalendarID, err := s.resolveGoogleImportCalendar(
			r.Context(), tx, bundle.ExternalID, bundle.Name, choice, mappingProvided, reuseByName, importActor, i,
		)
		if err != nil {
			writeError(w, 400, "Could not prepare imported calendar "+bundle.Name+": "+err.Error())
			return
		}

		result, err := s.importParsedCalendar(r.Context(), tx, calendarID, bundle.Calendar, importActor)
		if err != nil {
			writeError(w, 500, "Could not import "+bundle.Name+": "+err.Error())
			return
		}
		processedSources[bundle.ExternalID] = true

		if previousCalendarID != nil && *previousCalendarID != calendarID {
			cleaned, reason, cleanupErr := s.cleanupOldGoogleCalendar(r.Context(), tx, *previousCalendarID)
			if cleanupErr != nil {
				writeError(w, 500, "Imported "+bundle.Name+" but could not safely reconcile its old calendar")
				return
			}
			if cleaned {
				cleanedCalendars++
			} else if reason != "" {
				warnings = append(warnings, reason)
			}
		}

		if createdCalendar {
			createdCalendars++
		} else {
			reusedCalendars++
		}
		totalCreated += result.Created
		totalUpdated += result.Updated
		totalExceptions += result.Exceptions
		totalSkipped += result.Skipped
		items = append(items, googleCalendarImportItem{
			CalendarID: calendarID, Name: bundle.Name, CreatedCalendar: createdCalendar,
			Created: result.Created, Updated: result.Updated,
			Exceptions: result.Exceptions, Skipped: result.Skipped,
		})
	}

	duplicateEventsReconciled, err = s.reconcileGoogleDuplicateImports(r.Context(), tx, bundles, processedSources)
	if err != nil {
		writeError(w, 500, "Could not reconcile duplicate Google events")
		return
	}

	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "Could not finish Google Calendar import")
		return
	}

	s.audit(r, "import", "integration", nil, "Imported Google Calendar export", map[string]any{
		"calendars": len(items), "created_calendars": createdCalendars, "reused_calendars": reusedCalendars,
		"skipped_calendars": skippedCalendars, "cleaned_calendars": cleanedCalendars,
		"events_created": totalCreated, "events_updated": totalUpdated, "exceptions": totalExceptions, "skipped": totalSkipped,
		"duplicate_events_suppressed": duplicateEventsSuppressed, "duplicate_events_reconciled": duplicateEventsReconciled,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"calendars": items, "calendar_count": len(items),
		"created_calendars": createdCalendars, "reused_calendars": reusedCalendars,
		"skipped_calendars": skippedCalendars, "cleaned_calendars": cleanedCalendars,
		"created": totalCreated, "updated": totalUpdated,
		"exceptions": totalExceptions, "skipped": totalSkipped,
		"duplicate_events_suppressed": duplicateEventsSuppressed,
		"duplicate_events_reconciled": duplicateEventsReconciled,
		"warnings": warnings,
	})
}

func (s *server) reconcileGoogleDuplicateImports(
	ctx context.Context,
	tx pgx.Tx,
	bundles []googleCalendarBundle,
	processedSources map[string]bool,
) (int, error) {
	reconciled := 0
	calendarForSource := map[string]uuid.UUID{}
	loadCalendar := func(source string) (uuid.UUID, error) {
		if id, ok := calendarForSource[source]; ok {
			return id, nil
		}
		var id uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT calendar_id FROM calendar_import_sources
			WHERE provider='google' AND external_id=$1`, source).Scan(&id); err != nil {
			return uuid.Nil, err
		}
		calendarForSource[source] = id
		return id, nil
	}

	for _, loserBundle := range bundles {
		if !processedSources[loserBundle.ExternalID] || len(loserBundle.SuppressedDuplicateUIDs) == 0 {
			continue
		}
		loserCalendar, err := loadCalendar(loserBundle.ExternalID)
		if err != nil {
			return reconciled, err
		}
		for uid, ownerSource := range loserBundle.SuppressedDuplicateUIDs {
			if !processedSources[ownerSource] {
				continue
			}
			ownerCalendar, err := loadCalendar(ownerSource)
			if err != nil {
				return reconciled, err
			}
			if ownerCalendar == loserCalendar {
				continue
			}

			var loserEvent, ownerEvent uuid.UUID
			err = tx.QueryRow(ctx, `SELECT id FROM events
				WHERE calendar_id=$1 AND external_uid=$2 AND recurrence_parent_id IS NULL`,
				loserCalendar, uid).Scan(&loserEvent)
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			if err != nil {
				return reconciled, err
			}
			err = tx.QueryRow(ctx, `SELECT id FROM events
				WHERE calendar_id=$1 AND external_uid=$2 AND recurrence_parent_id IS NULL`,
				ownerCalendar, uid).Scan(&ownerEvent)
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			if err != nil {
				return reconciled, err
			}

			if _, err = tx.Exec(ctx, `INSERT INTO event_assignees(event_id,user_id)
				SELECT $1,user_id FROM event_assignees WHERE event_id=$2
				ON CONFLICT(event_id,user_id) DO NOTHING`, ownerEvent, loserEvent); err != nil {
				return reconciled, err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO reminders(event_id,kind,provider,minutes_before,destination,enabled)
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
				return reconciled, err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO bill_event_details(event_id,amount_due,amount_is_estimate,payer_user_id,updated_at)
				SELECT $1,amount_due,amount_is_estimate,payer_user_id,updated_at
				FROM bill_event_details WHERE event_id=$2
				ON CONFLICT(event_id) DO NOTHING`, ownerEvent, loserEvent); err != nil {
				return reconciled, err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO bill_payments(event_id,occurrence_start,paid_by_user_id,amount_paid,paid_at,updated_at)
				SELECT $1,occurrence_start,paid_by_user_id,amount_paid,paid_at,updated_at
				FROM bill_payments WHERE event_id=$2
				ON CONFLICT(event_id,occurrence_start) DO NOTHING`, ownerEvent, loserEvent); err != nil {
				return reconciled, err
			}
			if _, err = tx.Exec(ctx, `UPDATE notifications SET event_id=$1 WHERE event_id=$2`, ownerEvent, loserEvent); err != nil {
				return reconciled, err
			}
			if _, err = tx.Exec(ctx, `DELETE FROM events WHERE id=$1`, loserEvent); err != nil {
				return reconciled, err
			}
			reconciled++
		}
	}
	return reconciled, nil
}

func (s *server) googleBundlesFromRequest(w http.ResponseWriter, r *http.Request) ([]googleCalendarBundle, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxGoogleCalendarUploadBytes+(4<<20))
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		return nil, errors.New("Could not read the Google Calendar export")
	}

	file, header, err := r.FormFile("archive")
	if err != nil {
		return nil, errors.New("Choose the Google Calendar export ZIP or an .ics file")
	}
	defer file.Close()
	if header.Size <= 0 || header.Size > maxGoogleCalendarUploadBytes {
		return nil, errors.New("Google Calendar export is too large")
	}

	raw, err := io.ReadAll(io.LimitReader(file, maxGoogleCalendarUploadBytes+1))
	if err != nil || int64(len(raw)) > maxGoogleCalendarUploadBytes {
		return nil, errors.New("Google Calendar export is too large")
	}

	bundles, err := parseGoogleCalendarUpload(header.Filename, raw, s.householdLocation(r.Context()))
	if err != nil {
		return nil, err
	}
	if len(bundles) == 0 {
		return nil, errors.New("The export does not contain any iCalendar files")
	}
	return bundles, nil
}

func parseGoogleCalendarUpload(filename string, raw []byte, loc *time.Location) ([]googleCalendarBundle, error) {
	if zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw))); err == nil {
		bundles := make([]googleCalendarBundle, 0)
		var expanded int64
		for _, entry := range zr.File {
			if entry.FileInfo().IsDir() || !strings.EqualFold(filepath.Ext(entry.Name), ".ics") {
				continue
			}
			if len(bundles) >= maxGoogleCalendarFiles {
				return nil, fmt.Errorf("Google Calendar export contains more than %d calendars", maxGoogleCalendarFiles)
			}
			if entry.UncompressedSize64 > uint64(maxICalendarUploadBytes) {
				return nil, fmt.Errorf("Calendar %q is too large to import", filepath.Base(entry.Name))
			}
			expanded += int64(entry.UncompressedSize64)
			if expanded > maxGoogleCalendarExpandedBytes {
				return nil, errors.New("Expanded Google Calendar export is too large")
			}

			rc, err := entry.Open()
			if err != nil {
				return nil, fmt.Errorf("Could not read %q", filepath.Base(entry.Name))
			}
			data, readErr := io.ReadAll(io.LimitReader(rc, maxICalendarUploadBytes+1))
			rc.Close()
			if readErr != nil || int64(len(data)) > maxICalendarUploadBytes {
				return nil, fmt.Errorf("Could not read %q", filepath.Base(entry.Name))
			}
			parsed, err := calical.Parse(bytes.NewReader(data), loc)
			if err != nil {
				return nil, fmt.Errorf("Could not import %q: %w", filepath.Base(entry.Name), err)
			}
			name := googleCalendarDisplayName(parsed.Name, entry.Name)
			bundles = append(bundles, googleCalendarBundle{
				ExternalID: strings.ToLower(filepath.ToSlash(entry.Name)),
				Name:       name,
				Calendar:   parsed,
			})
		}
		if len(bundles) == 0 {
			return nil, errors.New("The ZIP does not contain any .ics calendars")
		}
		return dedupeGoogleCalendarBundles(bundles), nil
	}

	if !strings.EqualFold(filepath.Ext(filename), ".ics") && !bytes.Contains(raw, []byte("BEGIN:VCALENDAR")) {
		return nil, errors.New("Choose the ZIP downloaded from Google Calendar, or an .ics calendar file")
	}
	parsed, err := calical.Parse(bytes.NewReader(raw), loc)
	if err != nil {
		return nil, fmt.Errorf("Could not import iCalendar file: %w", err)
	}
	return []googleCalendarBundle{{
		ExternalID: strings.ToLower(filepath.Base(filename)),
		Name:       googleCalendarDisplayName(parsed.Name, filename),
		Calendar:   parsed,
	}}, nil
}

type googleEventRevision struct {
	Modified time.Time
	Stamp    time.Time
	Created  time.Time
	Sequence int
}

func googleRevisionForEvent(event calical.Event) googleEventRevision {
	return googleEventRevision{
		Modified: event.ModifiedAt,
		Stamp:    event.StampAt,
		Created:  event.CreatedAt,
		Sequence: event.Sequence,
	}
}

func compareGoogleEventRevision(left, right googleEventRevision) int {
	compareTime := func(a, b time.Time) int {
		if a.After(b) {
			return 1
		}
		if a.Before(b) {
			return -1
		}
		return 0
	}
	if result := compareTime(left.Modified, right.Modified); result != 0 {
		return result
	}
	if left.Sequence > right.Sequence {
		return 1
	}
	if left.Sequence < right.Sequence {
		return -1
	}
	if result := compareTime(left.Stamp, right.Stamp); result != 0 {
		return result
	}
	return compareTime(left.Created, right.Created)
}

func dedupeGoogleCalendarBundles(bundles []googleCalendarBundle) []googleCalendarBundle {
	type owner struct {
		bundle   int
		revision googleEventRevision
	}
	uidBundles := map[string]map[int]googleEventRevision{}

	for bundleIndex := range bundles {
		perBundle := map[string]googleEventRevision{}
		for _, event := range bundles[bundleIndex].Calendar.Events {
			uid := strings.TrimSpace(event.UID)
			if uid == "" {
				continue
			}
			revision := googleRevisionForEvent(event)
			if current, ok := perBundle[uid]; !ok || compareGoogleEventRevision(revision, current) > 0 {
				perBundle[uid] = revision
			}
		}
		for uid, revision := range perBundle {
			if uidBundles[uid] == nil {
				uidBundles[uid] = map[int]googleEventRevision{}
			}
			uidBundles[uid][bundleIndex] = revision
		}
	}

	owners := map[string]owner{}
	for uid, revisions := range uidBundles {
		if len(revisions) < 2 {
			continue
		}
		best := owner{bundle: -1}
		ambiguous := false
		for bundleIndex, revision := range revisions {
			if best.bundle < 0 {
				best = owner{bundle: bundleIndex, revision: revision}
				continue
			}
			switch compareGoogleEventRevision(revision, best.revision) {
			case 1:
				best = owner{bundle: bundleIndex, revision: revision}
				ambiguous = false
			case 0:
				ambiguous = true
			}
		}
		// If Google gives identical revision metadata in more than one calendar,
		// keep both rather than guessing which calendar currently owns the event.
		if !ambiguous && best.bundle >= 0 {
			owners[uid] = best
		}
	}

	for bundleIndex := range bundles {
		filtered := make([]calical.Event, 0, len(bundles[bundleIndex].Calendar.Events))
		for _, event := range bundles[bundleIndex].Calendar.Events {
			uid := strings.TrimSpace(event.UID)
			owner, duplicated := owners[uid]
			if duplicated && owner.bundle != bundleIndex {
				if bundles[bundleIndex].SuppressedDuplicateUIDs == nil {
					bundles[bundleIndex].SuppressedDuplicateUIDs = map[string]string{}
				}
				bundles[bundleIndex].SuppressedDuplicateUIDs[uid] = bundles[owner.bundle].ExternalID
				bundles[bundleIndex].SuppressedDuplicateEvents++
				continue
			}
			filtered = append(filtered, event)
		}
		bundles[bundleIndex].Calendar.Events = filtered
	}
	return bundles
}

func googleCalendarDisplayName(calendarName, filename string) string {
	name := strings.TrimSpace(calendarName)
	filenameLabel := googleCalendarFilenameLabel(filename)

	// Google's generated Birthdays export can report the account email as
	// X-WR-CALNAME even though the filename identifies it as Birthdays.
	// Prefer that semantic filename label so it can be matched correctly.
	if looksLikeCalendarEmail(name) && filenameLabel != "" && !looksLikeCalendarEmail(filenameLabel) {
		name = filenameLabel
	}
	if name == "" {
		name = filenameLabel
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Imported Google Calendar"
	}
	return cleanText(name, 100)
}

func googleCalendarFilenameLabel(filename string) string {
	base := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	base = strings.TrimSpace(base)
	if base == "" {
		return ""
	}
	if underscore := strings.Index(base, "_"); underscore > 0 {
		prefix := strings.TrimSpace(base[:underscore])
		suffix := strings.TrimSpace(base[underscore+1:])
		if prefix != "" && strings.Contains(suffix, "@") {
			return prefix
		}
	}
	return base
}

func looksLikeCalendarEmail(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && !strings.ContainsAny(value, " \t") && strings.Count(value, "@") == 1
}

func normalizeGoogleCalendarName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var cleaned strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cleaned.WriteRune(r)
		} else {
			cleaned.WriteByte(' ')
		}
	}
	ignored := map[string]bool{"calendar": true, "calendars": true, "event": true, "events": true, "schedule": true, "schedules": true}
	aliases := map[string]string{
		"bill": "bills", "billing": "bills", "payment": "bills", "payments": "bills", "pay": "bills",
		"birthday": "birthdays", "bday": "birthdays",
		"appointment": "appointments", "appt": "appointments",
	}
	seen := map[string]bool{}
	tokens := []string{}
	for _, token := range strings.Fields(cleaned.String()) {
		if ignored[token] {
			continue
		}
		if alias := aliases[token]; alias != "" {
			token = alias
		}
		if token == "" || seen[token] {
			continue
		}
		seen[token] = true
		tokens = append(tokens, token)
	}
	sort.Strings(tokens)
	return strings.Join(tokens, " ")
}

func googleCalendarMatchScore(importName, existingName string) (int, string) {
	leftRaw := strings.ToLower(strings.TrimSpace(importName))
	rightRaw := strings.ToLower(strings.TrimSpace(existingName))
	if leftRaw != "" && leftRaw == rightRaw {
		return 100, "Exact existing calendar name"
	}
	left := normalizeGoogleCalendarName(importName)
	right := normalizeGoogleCalendarName(existingName)
	if left == "" || right == "" {
		return 0, ""
	}
	if left == right {
		return 96, "Same calendar name after removing generic words"
	}
	if (strings.Contains(left, right) || strings.Contains(right, left)) && minInt(len(left), len(right)) >= 4 {
		return 88, "Very similar calendar name"
	}

	leftTokens := strings.Fields(left)
	rightTokens := strings.Fields(right)
	rightSet := map[string]bool{}
	for _, token := range rightTokens {
		rightSet[token] = true
	}
	common := 0
	for _, token := range leftTokens {
		if rightSet[token] {
			common++
		}
	}
	union := len(leftTokens) + len(rightTokens) - common
	if union == 0 {
		return 0, ""
	}
	score := common * 100 / union
	if score >= 67 {
		return 80 + (score-67)*8/33, "Similar calendar name"
	}
	return 0, ""
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (s *server) googleCalendarCandidates(ctx context.Context) ([]googleCalendarCandidate, error) {
	rows, err := s.db.Query(ctx, `SELECT id,name,description FROM calendars ORDER BY created_at,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	candidates := []googleCalendarCandidate{}
	for rows.Next() {
		var candidate googleCalendarCandidate
		if err := rows.Scan(&candidate.ID, &candidate.Name, &candidate.Description); err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	return candidates, rows.Err()
}

func chooseGoogleCalendarCandidate(name string, candidates []googleCalendarCandidate) (*googleCalendarCandidate, int, string, bool) {
	bestScore := 0
	bestIndex := -1
	bestReason := ""
	ambiguous := false
	for i := range candidates {
		candidate := &candidates[i]
		if candidate.Description == "Imported from Google Calendar" {
			continue
		}
		score, reason := googleCalendarMatchScore(name, candidate.Name)
		if score > bestScore {
			bestScore = score
			bestIndex = i
			bestReason = reason
			ambiguous = false
		} else if score > 0 && score == bestScore {
			ambiguous = true
		}
	}
	if bestIndex < 0 {
		return nil, 0, "", false
	}
	return &candidates[bestIndex], bestScore, bestReason, ambiguous
}

func (s *server) previewGoogleCalendarBundle(
	ctx context.Context,
	bundle googleCalendarBundle,
	candidates []googleCalendarCandidate,
) (googleCalendarPreviewItem, error) {
	item := googleCalendarPreviewItem{
		ExternalID:                bundle.ExternalID,
		Name:                      bundle.Name,
		EventCount:                len(bundle.Calendar.Events),
		DuplicateEventsSuppressed: bundle.SuppressedDuplicateEvents,
		MatchReason:               "No confident match. Review before creating a new calendar.",
	}

	var previousID uuid.UUID
	var previousName, previousDescription string
	err := s.db.QueryRow(ctx, `SELECT c.id,c.name,c.description
		FROM calendar_import_sources cis
		JOIN calendars c ON c.id=cis.calendar_id
		WHERE cis.provider='google' AND cis.external_id=$1`, bundle.ExternalID).
		Scan(&previousID, &previousName, &previousDescription)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return item, err
	}
	if err == nil {
		item.PreviousCalendarID = &previousID
		item.PreviousCalendarName = previousName
		item.PreviousAutoCreated = previousDescription == "Imported from Google Calendar"
		if !item.PreviousAutoCreated {
			item.SuggestedCalendarID = &previousID
			item.SuggestedCalendarName = previousName
			item.MatchScore = 100
			item.MatchReason = "Previously mapped to this CalDen calendar"
			return item, nil
		}
	}

	best, bestScore, bestReason, ambiguous := chooseGoogleCalendarCandidate(bundle.Name, candidates)
	if ambiguous && bestScore >= 80 {
		item.MatchScore = bestScore
		item.MatchReason = "Multiple existing calendars are equally plausible matches. Choose the destination manually."
		if item.PreviousAutoCreated {
			item.MatchReason += " The previous import-created duplicate will not be reused automatically."
		}
		return item, nil
	}
	if best != nil && bestScore >= 80 {
		item.SuggestedCalendarID = &best.ID
		item.SuggestedCalendarName = best.Name
		item.MatchScore = bestScore
		if item.PreviousAutoCreated {
			item.MatchReason = bestReason + "; remapping can clean up the prior imported duplicate"
		} else {
			item.MatchReason = bestReason
		}
		return item, nil
	}

	if item.PreviousAutoCreated {
		item.MatchScore = 75
		item.MatchReason = "A previous import created a separate CalDen calendar. Choose an existing destination, create a new one deliberately, or skip it."
	}
	return item, nil
}

func (s *server) resolveGoogleImportCalendar(
	ctx context.Context,
	tx pgx.Tx,
	externalID, name, choice string,
	mappingProvided, reuseByName bool,
	importActor actor,
	paletteIndex int,
) (uuid.UUID, bool, *uuid.UUID, error) {
	var previousCalendarID *uuid.UUID
	var existingMapping uuid.UUID
	err := tx.QueryRow(ctx, `SELECT calendar_id FROM calendar_import_sources
		WHERE provider='google' AND external_id=$1`, externalID).Scan(&existingMapping)
	if err == nil {
		copy := existingMapping
		previousCalendarID = &copy
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil, err
	}

	var calendarID uuid.UUID
	createNew := false
	if mappingProvided {
		switch choice {
		case googleMappingCreate:
			createNew = true
		case "", googleMappingSkip:
			return uuid.Nil, false, previousCalendarID, errors.New("calendar mapping is incomplete")
		default:
			calendarID, err = uuid.Parse(choice)
			if err != nil {
				return uuid.Nil, false, previousCalendarID, errors.New("selected CalDen calendar is invalid")
			}
			var exists bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM calendars WHERE id=$1)`, calendarID).Scan(&exists); err != nil {
				return uuid.Nil, false, previousCalendarID, err
			}
			if !exists {
				return uuid.Nil, false, previousCalendarID, errors.New("selected CalDen calendar no longer exists")
			}
		}
	} else if existingMapping != uuid.Nil {
		calendarID = existingMapping
	} else if reuseByName {
		calendarID, err = s.bestGoogleCalendarMatchTx(ctx, tx, name)
		if err != nil {
			return uuid.Nil, false, previousCalendarID, err
		}
	}

	created := false
	if createNew || calendarID == uuid.Nil {
		color := googleCalendarPalette[paletteIndex%len(googleCalendarPalette)]
		err = tx.QueryRow(ctx, `INSERT INTO calendars(name,color,icon,description,created_by)
			VALUES($1,$2,'calendar','Imported from Google Calendar',$3)
			RETURNING id`, name, color, importActor.ID).Scan(&calendarID)
		if err != nil {
			return uuid.Nil, false, previousCalendarID, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO calendar_permissions(calendar_id,user_id,can_view,can_edit,can_delete)
			VALUES($1,$2,true,true,true)
			ON CONFLICT(calendar_id,user_id) DO UPDATE
			SET can_view=true,can_edit=true,can_delete=true`, calendarID, importActor.ID); err != nil {
			return uuid.Nil, false, previousCalendarID, err
		}
		created = true
	}

	_, err = tx.Exec(ctx, `INSERT INTO calendar_import_sources(provider,external_id,calendar_id,display_name,last_imported_at)
		VALUES('google',$1,$2,$3,now())
		ON CONFLICT(provider,external_id) DO UPDATE
		SET calendar_id=EXCLUDED.calendar_id,display_name=EXCLUDED.display_name,last_imported_at=now(),updated_at=now()`,
		externalID, calendarID, name)
	if err != nil {
		return uuid.Nil, false, previousCalendarID, err
	}
	return calendarID, created, previousCalendarID, nil
}

func (s *server) bestGoogleCalendarMatchTx(ctx context.Context, tx pgx.Tx, name string) (uuid.UUID, error) {
	rows, err := tx.Query(ctx, `SELECT c.id,c.name
		FROM calendars c
		WHERE c.description <> 'Imported from Google Calendar'
		   OR c.description IS NULL
		ORDER BY c.created_at,c.id`)
	if err != nil {
		return uuid.Nil, err
	}
	defer rows.Close()

	candidates := []googleCalendarCandidate{}
	for rows.Next() {
		var candidate googleCalendarCandidate
		if err := rows.Scan(&candidate.ID, &candidate.Name); err != nil {
			return uuid.Nil, err
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return uuid.Nil, err
	}
	best, bestScore, _, ambiguous := chooseGoogleCalendarCandidate(name, candidates)
	if best == nil || bestScore < 80 || ambiguous {
		return uuid.Nil, nil
	}
	return best.ID, nil
}

func (s *server) cleanupOldGoogleCalendar(ctx context.Context, tx pgx.Tx, calendarID uuid.UUID) (bool, string, error) {
	var name, description string
	err := tx.QueryRow(ctx, `SELECT name,description FROM calendars WHERE id=$1`, calendarID).Scan(&name, &description)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	if description != "Imported from Google Calendar" {
		return false, "", nil
	}

	var remainingMappings int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM calendar_import_sources
		WHERE provider='google' AND calendar_id=$1`, calendarID).Scan(&remainingMappings); err != nil {
		return false, "", err
	}
	if remainingMappings > 0 {
		return false, fmt.Sprintf("Kept old imported calendar %q because another Google calendar still uses it.", name), nil
	}

	var manualEvents int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM events
		WHERE calendar_id=$1 AND recurrence_parent_id IS NULL AND external_uid IS NULL`, calendarID).Scan(&manualEvents); err != nil {
		return false, "", err
	}
	if manualEvents > 0 {
		return false, fmt.Sprintf("Kept old imported calendar %q because it now contains manually created CalDen events.", name), nil
	}

	if _, err := tx.Exec(ctx, `DELETE FROM calendars WHERE id=$1`, calendarID); err != nil {
		return false, "", err
	}
	return true, "", nil
}
