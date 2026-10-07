package api

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/gigabytegrove/calden/internal/recurrence"
)

func TestParseGoogleCalendarUploadZIP(t *testing.T) {
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)

	files := map[string]string{
		"Takeout/Calendar/Personal.ics": "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nX-WR-CALNAME:Personal\r\nBEGIN:VEVENT\r\nUID:one@test\r\nDTSTART:20261102T140000Z\r\nDTEND:20261102T150000Z\r\nSUMMARY:Personal event\r\nRRULE:FREQ=MONTHLY;COUNT=4;BYDAY=1MO\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n",
		"Takeout/Calendar/Family.ics": "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nX-WR-CALNAME:Family Imported\r\nBEGIN:VEVENT\r\nUID:two@test\r\nDTSTART;VALUE=DATE:20261115\r\nDTEND;VALUE=DATE:20261116\r\nSUMMARY:Family day\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n",
	}
	for name, content := range files {
		entry, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	bundles, err := parseGoogleCalendarUpload("takeout.zip", archive.Bytes(), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundles) != 2 {
		t.Fatalf("expected two calendars, got %d", len(bundles))
	}
	byName := map[string]googleCalendarBundle{}
	for _, bundle := range bundles {
		byName[bundle.Name] = bundle
		if !strings.Contains(bundle.ExternalID, "takeout/calendar/") {
			t.Fatalf("expected stable archive path id, got %q", bundle.ExternalID)
		}
	}
	personal, ok := byName["Personal"]
	if !ok || len(personal.Calendar.Events) != 1 {
		t.Fatalf("personal calendar missing: %#v", byName)
	}
	rule := personal.Calendar.Events[0].Recurrence
	if rule == nil || rule.Raw != "FREQ=MONTHLY;COUNT=4;BYDAY=1MO" {
		t.Fatalf("advanced recurrence was not preserved: %#v", rule)
	}
	got := recurrence.Expand(
		personal.Calendar.Events[0].Start,
		personal.Calendar.Events[0].End,
		rule,
		time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2027, 4, 1, 0, 0, 0, 0, time.UTC),
		20,
	)
	if len(got) != 4 {
		t.Fatalf("expected four imported recurrence dates, got %d: %#v", len(got), got)
	}
	if _, ok := byName["Family Imported"]; !ok {
		t.Fatalf("second calendar name was not preserved: %#v", byName)
	}
}

func TestParseGoogleCalendarUploadRejectsNonCalendarFile(t *testing.T) {
	_, err := parseGoogleCalendarUpload("notes.txt", []byte("not a calendar"), time.UTC)
	if err == nil {
		t.Fatal("expected non-calendar upload to be rejected")
	}
}


func TestGoogleCalendarMatchScore(t *testing.T) {
	tests := []struct {
		importName   string
		existingName string
		minScore     int
	}{
		{importName: "Family Calendar", existingName: "Family", minScore: 90},
		{importName: "Bill Pay Calendar", existingName: "Bills", minScore: 90},
		{importName: "Birthdays", existingName: "Birthday Calendar", minScore: 90},
		{importName: "Appointments Calendar", existingName: "Appointments", minScore: 90},
	}
	for _, tc := range tests {
		score, _ := googleCalendarMatchScore(tc.importName, tc.existingName)
		if score < tc.minScore {
			t.Fatalf("expected %q to match %q with score >= %d, got %d", tc.importName, tc.existingName, tc.minScore, score)
		}
	}

	if score, _ := googleCalendarMatchScore("Work", "Birthdays"); score != 0 {
		t.Fatalf("unrelated calendars should not match, got %d", score)
	}
}
