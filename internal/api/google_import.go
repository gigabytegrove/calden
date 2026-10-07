package api

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	calical "github.com/gigabytegrove/calden/internal/ical"
)

const (
	maxGoogleCalendarUploadBytes int64 = 100 << 20
	maxGoogleCalendarExpandedBytes int64 = 250 << 20
	maxGoogleCalendarFiles = 100
)

var googleCalendarPalette = []string{
	"#2563EB", "#16A34A", "#7C3AED", "#DB2777",
	"#EA580C", "#0891B2", "#4F46E5", "#65A30D",
	"#C2410C", "#0F766E", "#9333EA", "#475569",
}

type googleCalendarBundle struct {
	ExternalID string
	Name       string
	Calendar   calical.Calendar
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

func (s *server) importGoogleCalendarExport(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxGoogleCalendarUploadBytes+(4<<20))
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		writeError(w, 400, "Could not read the Google Calendar export")
		return
	}

	file, header, err := r.FormFile("archive")
	if err != nil {
		writeError(w, 400, "Choose the Google Calendar export ZIP or an .ics file")
		return
	}
	defer file.Close()
	if header.Size <= 0 || header.Size > maxGoogleCalendarUploadBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "Google Calendar export is too large")
		return
	}

	raw, err := io.ReadAll(io.LimitReader(file, maxGoogleCalendarUploadBytes+1))
	if err != nil || int64(len(raw)) > maxGoogleCalendarUploadBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "Google Calendar export is too large")
		return
	}

	bundles, err := parseGoogleCalendarUpload(header.Filename, raw, s.householdLocation(r.Context()))
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if len(bundles) == 0 {
		writeError(w, 400, "The export does not contain any iCalendar files")
		return
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
	createdCalendars, reusedCalendars := 0, 0

	for i, bundle := range bundles {
		calendarID, createdCalendar, err := s.resolveGoogleImportCalendar(
			r.Context(), tx, bundle.ExternalID, bundle.Name, reuseByName, importActor, i,
		)
		if err != nil {
			writeError(w, 500, "Could not prepare imported calendar "+bundle.Name)
			return
		}

		result, err := s.importParsedCalendar(r.Context(), tx, calendarID, bundle.Calendar, importActor)
		if err != nil {
			writeError(w, 500, "Could not import "+bundle.Name+": "+err.Error())
			return
		}
		if _, err = tx.Exec(r.Context(), `UPDATE calendar_import_sources
			SET display_name=$3,last_imported_at=now(),updated_at=now()
			WHERE provider='google' AND external_id=$1 AND calendar_id=$2`,
			bundle.ExternalID, calendarID, bundle.Name); err != nil {
			writeError(w, 500, "Could not finish Google Calendar import")
			return
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

	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "Could not finish Google Calendar import")
		return
	}

	s.audit(r, "import", "integration", nil, "Imported Google Calendar export", map[string]any{
		"calendars": len(items), "created_calendars": createdCalendars, "reused_calendars": reusedCalendars,
		"events_created": totalCreated, "events_updated": totalUpdated, "exceptions": totalExceptions, "skipped": totalSkipped,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"calendars": items, "calendar_count": len(items),
		"created_calendars": createdCalendars, "reused_calendars": reusedCalendars,
		"created": totalCreated, "updated": totalUpdated,
		"exceptions": totalExceptions, "skipped": totalSkipped,
	})
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
				Name: name,
				Calendar: parsed,
			})
		}
		if len(bundles) == 0 {
			return nil, errors.New("The ZIP does not contain any .ics calendars")
		}
		return bundles, nil
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
		Name: googleCalendarDisplayName(parsed.Name, filename),
		Calendar: parsed,
	}}, nil
}

func googleCalendarDisplayName(calendarName, filename string) string {
	name := strings.TrimSpace(calendarName)
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Imported Google Calendar"
	}
	return cleanText(name, 100)
}

func (s *server) resolveGoogleImportCalendar(
	ctx context.Context,
	tx pgx.Tx,
	externalID, name string,
	reuseByName bool,
	importActor actor,
	paletteIndex int,
) (uuid.UUID, bool, error) {
	var calendarID uuid.UUID
	err := tx.QueryRow(ctx, `SELECT calendar_id FROM calendar_import_sources
		WHERE provider='google' AND external_id=$1`, externalID).Scan(&calendarID)
	if err == nil {
		return calendarID, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, err
	}

	if reuseByName {
		err = tx.QueryRow(ctx, `SELECT c.id FROM calendars c
			WHERE lower(c.name)=lower($1)
			  AND NOT EXISTS (
				SELECT 1 FROM calendar_import_sources cis
				WHERE cis.calendar_id=c.id AND cis.provider='google'
			  )
			ORDER BY c.created_at
			LIMIT 1`, name).Scan(&calendarID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, false, err
		}
	}

	created := false
	if calendarID == uuid.Nil {
		color := googleCalendarPalette[paletteIndex%len(googleCalendarPalette)]
		err = tx.QueryRow(ctx, `INSERT INTO calendars(name,color,icon,description,created_by)
			VALUES($1,$2,'calendar','Imported from Google Calendar',$3)
			RETURNING id`, name, color, importActor.ID).Scan(&calendarID)
		if err != nil {
			return uuid.Nil, false, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO calendar_permissions(calendar_id,user_id,can_view,can_edit,can_delete)
			VALUES($1,$2,true,true,true)
			ON CONFLICT(calendar_id,user_id) DO UPDATE
			SET can_view=true,can_edit=true,can_delete=true`, calendarID, importActor.ID); err != nil {
			return uuid.Nil, false, err
		}
		created = true
	}

	_, err = tx.Exec(ctx, `INSERT INTO calendar_import_sources(provider,external_id,calendar_id,display_name,last_imported_at)
		VALUES('google',$1,$2,$3,now())
		ON CONFLICT(provider,external_id) DO UPDATE
		SET calendar_id=EXCLUDED.calendar_id,display_name=EXCLUDED.display_name,last_imported_at=now(),updated_at=now()`,
		externalID, calendarID, name)
	if err != nil {
		return uuid.Nil, false, err
	}
	return calendarID, created, nil
}
