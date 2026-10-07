package ical

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/gigabytegrove/calden/internal/recurrence"
)

func TestRoundTripRecurringEventWithException(t *testing.T) {
	start := time.Date(2026, 10, 12, 14, 30, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	count := 6
	rule := &recurrence.Rule{
		Frequency:       "weekly",
		Interval:        1,
		Weekdays:        []int{1, 3},
		OccurrenceCount: &count,
	}
	ex := start.AddDate(0, 0, 2)
	overrideStart := ex.Add(2 * time.Hour)
	cal := Calendar{
		Name: "Family",
		Events: []Event{
			{
				UID:         "abc-123@calden",
				Summary:     "Therapy, check-in",
				Description: "Bring notes\nSecond line",
				Location:    "Clinic; Building A",
				Category:    "Medical",
				Start:       start,
				End:         end,
				Recurrence:  rule,
				ExDates:     []time.Time{ex},
			},
			{
				UID:          "abc-123@calden",
				Summary:      "Therapy moved",
				Location:     "Clinic",
				Category:     "Medical",
				Start:        overrideStart,
				End:          overrideStart.Add(time.Hour),
				RecurrenceID: &ex,
			},
		},
	}
	var buf bytes.Buffer
	if err := Write(&buf, cal, time.UTC); err != nil {
		t.Fatal(err)
	}
	raw := buf.String()
	for _, want := range []string{
		"BEGIN:VCALENDAR",
		"RRULE:FREQ=WEEKLY;BYDAY=MO,WE;COUNT=6",
		"EXDATE:20261014T143000Z",
		"RECURRENCE-ID:20261014T143000Z",
		"CATEGORIES:Medical",
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("export missing %q:\n%s", want, raw)
		}
	}

	parsed, err := Parse(strings.NewReader(raw), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Name != "Family" {
		t.Fatalf("calendar name mismatch: %q", parsed.Name)
	}
	if len(parsed.Events) != 2 {
		t.Fatalf("expected 2 VEVENTs, got %d", len(parsed.Events))
	}
	parent := parsed.Events[0]
	if parent.Summary != "Therapy, check-in" || parent.Description != "Bring notes\nSecond line" {
		t.Fatalf("escaped text did not round-trip: %#v", parent)
	}
	if parent.Recurrence == nil || parent.Recurrence.Frequency != "weekly" || len(parent.Recurrence.Weekdays) != 2 {
		t.Fatalf("recurrence did not round-trip: %#v", parent.Recurrence)
	}
	if len(parent.ExDates) != 1 || !parent.ExDates[0].Equal(ex) {
		t.Fatalf("exception date mismatch: %#v", parent.ExDates)
	}
	override := parsed.Events[1]
	if override.RecurrenceID == nil || !override.RecurrenceID.Equal(ex) {
		t.Fatalf("recurrence id mismatch: %#v", override.RecurrenceID)
	}
}

func TestAllDayRoundTripUsesExclusiveICalEnd(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 10, 20, 0, 0, 0, 0, loc)
	end := time.Date(2026, 10, 20, 23, 59, 59, 0, loc)
	var buf bytes.Buffer
	if err := Write(&buf, Calendar{Events: []Event{{
		UID: "all-day@calden", Summary: "Birthday", Start: start, End: end, AllDay: true,
	}}}, loc); err != nil {
		t.Fatal(err)
	}
	raw := buf.String()
	if !strings.Contains(raw, "DTSTART;VALUE=DATE:20261020") || !strings.Contains(raw, "DTEND;VALUE=DATE:20261021") {
		t.Fatalf("unexpected all-day export:\n%s", raw)
	}
	parsed, err := Parse(strings.NewReader(raw), loc)
	if err != nil {
		t.Fatal(err)
	}
	got := parsed.Events[0]
	if !got.AllDay || got.Start.In(loc).Day() != 20 || got.End.In(loc).Day() != 20 {
		t.Fatalf("all-day event did not round-trip: %#v", got)
	}
}

func TestParseTZIDAndFoldedText(t *testing.T) {
	raw := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:tz@test\r\nDTSTART;TZID=America/New_York:20261020T090000\r\nDTEND;TZID=America/New_York:20261020T100000\r\nSUMMARY:A long family event that has a folded\r\n  summary\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	cal, err := Parse(strings.NewReader(raw), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if len(cal.Events) != 1 {
		t.Fatalf("expected event")
	}
	event := cal.Events[0]
	if event.Summary != "A long family event that has a folded summary" {
		t.Fatalf("folding mismatch: %q", event.Summary)
	}
	if event.Start.UTC().Hour() != 13 {
		t.Fatalf("TZID not applied: %v", event.Start)
	}
}
