package recurrence

import (
	"testing"
	"time"
)

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestDailyCount(t *testing.T) {
	start := mustTime(t, "2026-10-01T09:00:00Z")
	end := start.Add(time.Hour)
	count := 3
	rule := &Rule{Frequency: "daily", Interval: 1, OccurrenceCount: &count}
	got := Expand(start, end, rule, start.Add(-time.Hour), start.AddDate(0, 0, 10), 100)
	if len(got) != 3 {
		t.Fatalf("expected 3 occurrences, got %d", len(got))
	}
	if got[2].Start.Day() != 3 {
		t.Fatalf("expected third occurrence on Oct 3, got %v", got[2].Start)
	}
}

func TestWeeklyMultipleDays(t *testing.T) {
	start := mustTime(t, "2026-10-05T08:30:00Z") // Monday
	end := start.Add(30 * time.Minute)
	rule := &Rule{Frequency: "weekly", Interval: 1, Weekdays: []int{1, 3, 5}}
	got := Expand(start, end, rule, start, start.AddDate(0, 0, 8), 100)
	if len(got) != 4 {
		t.Fatalf("expected Mon/Wed/Fri/Mon, got %d occurrences: %#v", len(got), got)
	}
	want := []time.Weekday{time.Monday, time.Wednesday, time.Friday, time.Monday}
	for i := range want {
		if got[i].Start.Weekday() != want[i] {
			t.Fatalf("occurrence %d: expected %s, got %s", i, want[i], got[i].Start.Weekday())
		}
	}
}

func TestMonthlyClampsMonthEnd(t *testing.T) {
	start := mustTime(t, "2026-01-31T10:00:00Z")
	end := start.Add(time.Hour)
	count := 3
	rule := &Rule{Frequency: "monthly", Interval: 1, OccurrenceCount: &count}
	got := Expand(start, end, rule, start, start.AddDate(0, 4, 0), 100)
	if len(got) != 3 {
		t.Fatalf("expected 3 occurrences, got %d", len(got))
	}
	if got[1].Start.Month() != time.February || got[1].Start.Day() != 28 {
		t.Fatalf("expected Feb 28, got %v", got[1].Start)
	}
	if got[2].Start.Month() != time.March || got[2].Start.Day() != 31 {
		t.Fatalf("expected Mar 31, got %v", got[2].Start)
	}
}

func TestYearlyLeapDayClamps(t *testing.T) {
	start := mustTime(t, "2024-02-29T12:00:00Z")
	end := start.Add(time.Hour)
	count := 3
	rule := &Rule{Frequency: "yearly", Interval: 1, OccurrenceCount: &count}
	got := Expand(start, end, rule, start, start.AddDate(4, 0, 0), 100)
	if len(got) != 3 {
		t.Fatalf("expected 3 occurrences, got %d", len(got))
	}
	if got[1].Start.Month() != time.February || got[1].Start.Day() != 28 {
		t.Fatalf("expected 2025-02-28, got %v", got[1].Start)
	}
}

func TestUntilStopsSeries(t *testing.T) {
	start := mustTime(t, "2026-10-01T09:00:00Z")
	end := start.Add(time.Hour)
	until := mustTime(t, "2026-10-03T09:00:00Z")
	rule := &Rule{Frequency: "daily", Interval: 1, Until: &until}
	got := Expand(start, end, rule, start, start.AddDate(0, 0, 10), 100)
	if len(got) != 3 {
		t.Fatalf("expected 3 occurrences through inclusive end date, got %d", len(got))
	}
}
