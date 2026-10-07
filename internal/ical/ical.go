package ical

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/gigabytegrove/calden/internal/recurrence"
)

type Event struct {
	UID          string
	Summary      string
	Description  string
	Location     string
	Category     string
	Start        time.Time
	End          time.Time
	AllDay       bool
	Recurrence   *recurrence.Rule
	ExDates      []time.Time
	RecurrenceID *time.Time
	Cancelled    bool
	CreatedAt    time.Time
	ModifiedAt   time.Time
	StampAt      time.Time
	Sequence     int
}

type Calendar struct {
	Name   string
	Events []Event
}

func Write(w io.Writer, calendar Calendar, loc *time.Location) error {
	if loc == nil {
		loc = time.UTC
	}
	lines := []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//Gigabyte Grove//CalDen//EN",
		"CALSCALE:GREGORIAN",
		"METHOD:PUBLISH",
	}
	if strings.TrimSpace(calendar.Name) != "" {
		lines = append(lines, "X-WR-CALNAME:"+escapeText(calendar.Name))
	}
	now := time.Now().UTC()
	for _, event := range calendar.Events {
		lines = append(lines, eventLines(event, loc, now)...)
	}
	lines = append(lines, "END:VCALENDAR")
	for _, line := range lines {
		for _, folded := range foldLine(line) {
			if _, err := io.WriteString(w, folded+"\r\n"); err != nil {
				return err
			}
		}
	}
	return nil
}

func eventLines(event Event, loc *time.Location, stamp time.Time) []string {
	lines := []string{"BEGIN:VEVENT"}
	uid := strings.TrimSpace(event.UID)
	if uid == "" {
		uid = fmt.Sprintf("calden-%d@local", event.Start.UnixNano())
	}
	lines = append(lines, "UID:"+escapeText(uid))
	lines = append(lines, "DTSTAMP:"+formatDateTime(stamp))
	if event.RecurrenceID != nil {
		lines = append(lines, formatPropertyTime("RECURRENCE-ID", *event.RecurrenceID, event.AllDay, loc))
	}
	lines = append(lines, formatPropertyTime("DTSTART", event.Start, event.AllDay, loc))
	lines = append(lines, formatPropertyEnd("DTEND", event.Start, event.End, event.AllDay, loc))
	if event.Cancelled {
		lines = append(lines, "STATUS:CANCELLED")
	}
	if event.Summary != "" {
		lines = append(lines, "SUMMARY:"+escapeText(event.Summary))
	}
	if event.Description != "" {
		lines = append(lines, "DESCRIPTION:"+escapeText(event.Description))
	}
	if event.Location != "" {
		lines = append(lines, "LOCATION:"+escapeText(event.Location))
	}
	if event.Category != "" {
		lines = append(lines, "CATEGORIES:"+escapeText(event.Category))
	}
	if event.Recurrence != nil {
		lines = append(lines, "RRULE:"+formatRule(*event.Recurrence))
	}
	for _, ex := range event.ExDates {
		lines = append(lines, formatPropertyTime("EXDATE", ex, event.AllDay, loc))
	}
	lines = append(lines, "END:VEVENT")
	return lines
}

func formatPropertyTime(name string, value time.Time, allDay bool, loc *time.Location) string {
	if allDay {
		return name + ";VALUE=DATE:" + value.In(loc).Format("20060102")
	}
	return name + ":" + formatDateTime(value)
}

func formatPropertyEnd(name string, start, end time.Time, allDay bool, loc *time.Location) string {
	if !allDay {
		return name + ":" + formatDateTime(end)
	}
	startDate := start.In(loc)
	endDate := end.In(loc)
	exclusive := time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)
	minimum := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)
	if exclusive.Before(minimum) {
		exclusive = minimum
	}
	return name + ";VALUE=DATE:" + exclusive.Format("20060102")
}

func formatDateTime(value time.Time) string {
	return value.UTC().Format("20060102T150405Z")
}

func formatRule(rule recurrence.Rule) string {
	if raw := strings.TrimSpace(strings.TrimPrefix(rule.Raw, "RRULE:")); raw != "" {
		return raw
	}
	freq := strings.ToUpper(rule.Frequency)
	parts := []string{"FREQ=" + freq}
	if rule.Interval > 1 {
		parts = append(parts, "INTERVAL="+strconv.Itoa(rule.Interval))
	}
	if rule.Frequency == "weekly" && len(rule.Weekdays) > 0 {
		days := make([]string, 0, len(rule.Weekdays))
		for _, day := range rule.Weekdays {
			if code := weekdayCode(day); code != "" {
				days = append(days, code)
			}
		}
		if len(days) > 0 {
			parts = append(parts, "BYDAY="+strings.Join(days, ","))
		}
	}
	if rule.Until != nil {
		parts = append(parts, "UNTIL="+formatDateTime(*rule.Until))
	}
	if rule.OccurrenceCount != nil {
		parts = append(parts, "COUNT="+strconv.Itoa(*rule.OccurrenceCount))
	}
	return strings.Join(parts, ";")
}

func Parse(r io.Reader, defaultLoc *time.Location) (Calendar, error) {
	if defaultLoc == nil {
		defaultLoc = time.UTC
	}
	lines, err := unfold(r)
	if err != nil {
		return Calendar{}, err
	}
	var cal Calendar
	var current *Event
	for _, line := range lines {
		if line == "BEGIN:VEVENT" {
			if current != nil {
				return Calendar{}, errors.New("nested VEVENT is not supported")
			}
			current = &Event{}
			continue
		}
		if line == "END:VEVENT" {
			if current == nil {
				continue
			}
			if current.Start.IsZero() {
				return Calendar{}, errors.New("event is missing DTSTART")
			}
			if current.End.IsZero() {
				if current.AllDay {
					current.End = current.Start.AddDate(0, 0, 1)
				} else {
					current.End = current.Start.Add(time.Hour)
				}
			}
			if current.AllDay {
				// RFC 5545 all-day DTEND is exclusive. CalDen stores an inclusive end.
				current.End = current.End.Add(-time.Nanosecond)
			}
			if !current.End.After(current.Start) {
				current.End = current.Start.Add(time.Hour)
			}
			cal.Events = append(cal.Events, *current)
			current = nil
			continue
		}
		name, params, value, ok := parseProperty(line)
		if !ok {
			continue
		}
		if current == nil {
			if name == "X-WR-CALNAME" {
				cal.Name = unescapeText(value)
			}
			continue
		}
		switch name {
		case "UID":
			current.UID = unescapeText(value)
		case "SUMMARY":
			current.Summary = unescapeText(value)
		case "DESCRIPTION":
			current.Description = unescapeText(value)
		case "LOCATION":
			current.Location = unescapeText(value)
		case "CATEGORIES":
			parts := splitEscaped(value, ',')
			if len(parts) > 0 {
				current.Category = unescapeText(parts[0])
			}
		case "STATUS":
			current.Cancelled = strings.EqualFold(strings.TrimSpace(value), "CANCELLED")
		case "CREATED":
			if t, _, err := parseTimeValue(value, params, defaultLoc); err == nil {
				current.CreatedAt = t
			}
		case "LAST-MODIFIED":
			if t, _, err := parseTimeValue(value, params, defaultLoc); err == nil {
				current.ModifiedAt = t
			}
		case "DTSTAMP":
			if t, _, err := parseTimeValue(value, params, defaultLoc); err == nil {
				current.StampAt = t
			}
		case "SEQUENCE":
			if n, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
				current.Sequence = n
			}
		case "DTSTART":
			t, allDay, err := parseTimeValue(value, params, defaultLoc)
			if err != nil {
				return Calendar{}, fmt.Errorf("invalid DTSTART: %w", err)
			}
			current.Start = t
			current.AllDay = allDay
		case "DTEND":
			t, _, err := parseTimeValue(value, params, defaultLoc)
			if err != nil {
				return Calendar{}, fmt.Errorf("invalid DTEND: %w", err)
			}
			current.End = t
		case "RECURRENCE-ID":
			t, _, err := parseTimeValue(value, params, defaultLoc)
			if err != nil {
				return Calendar{}, fmt.Errorf("invalid RECURRENCE-ID: %w", err)
			}
			current.RecurrenceID = &t
		case "EXDATE":
			values := splitEscaped(value, ',')
			for _, raw := range values {
				t, _, err := parseTimeValue(raw, params, defaultLoc)
				if err == nil {
					current.ExDates = append(current.ExDates, t)
				}
			}
		case "RRULE":
			rule, err := parseRule(value, current.Start, defaultLoc)
			if err != nil {
				return Calendar{}, err
			}
			current.Recurrence = rule
		}
	}
	if current != nil {
		return Calendar{}, errors.New("unterminated VEVENT")
	}
	return cal, nil
}

func unfold(r io.Reader) ([]string, error) {
	scanner := bufio.NewScanner(io.LimitReader(r, 32<<20))
	scanner.Buffer(make([]byte, 64*1024), 2<<20)
	lines := []string{}
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) && len(lines) > 0 {
			lines[len(lines)-1] += line[1:]
		} else {
			lines = append(lines, line)
		}
	}
	return lines, scanner.Err()
}

func parseProperty(line string) (string, map[string]string, string, bool) {
	before, value, ok := strings.Cut(line, ":")
	if !ok {
		return "", nil, "", false
	}
	parts := strings.Split(before, ";")
	name := strings.ToUpper(strings.TrimSpace(parts[0]))
	params := map[string]string{}
	for _, part := range parts[1:] {
		key, val, ok := strings.Cut(part, "=")
		if ok {
			params[strings.ToUpper(strings.TrimSpace(key))] = strings.Trim(strings.TrimSpace(val), "\"")
		}
	}
	return name, params, value, true
}

func parseTimeValue(value string, params map[string]string, defaultLoc *time.Location) (time.Time, bool, error) {
	value = strings.TrimSpace(value)
	allDay := strings.EqualFold(params["VALUE"], "DATE") || len(value) == 8
	loc := defaultLoc
	if tzid := strings.TrimSpace(params["TZID"]); tzid != "" {
		if loaded, err := time.LoadLocation(tzid); err == nil {
			loc = loaded
		}
	}
	if allDay {
		t, err := time.ParseInLocation("20060102", value, loc)
		return t, true, err
	}
	if strings.HasSuffix(value, "Z") {
		t, err := time.Parse("20060102T150405Z", value)
		return t, false, err
	}
	for _, layout := range []string{"20060102T150405", "20060102T1504"} {
		if t, err := time.ParseInLocation(layout, value, loc); err == nil {
			return t, false, nil
		}
	}
	return time.Time{}, false, fmt.Errorf("unsupported date-time %q", value)
}

func parseRule(value string, start time.Time, loc *time.Location) (*recurrence.Rule, error) {
	rule := &recurrence.Rule{Interval: 1, Raw: strings.TrimSpace(value)}
	for _, part := range strings.Split(value, ";") {
		key, val, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		switch strings.ToUpper(strings.TrimSpace(key)) {
		case "FREQ":
			rule.Frequency = strings.ToLower(strings.TrimSpace(val))
		case "INTERVAL":
			n, err := strconv.Atoi(strings.TrimSpace(val))
			if err != nil {
				return nil, errors.New("invalid RRULE interval")
			}
			rule.Interval = n
		case "COUNT":
			n, err := strconv.Atoi(strings.TrimSpace(val))
			if err != nil {
				return nil, errors.New("invalid RRULE count")
			}
			rule.OccurrenceCount = &n
		case "UNTIL":
			t, _, err := parseTimeValue(strings.TrimSpace(val), nil, loc)
			if err != nil {
				return nil, errors.New("invalid RRULE end date")
			}
			rule.Until = &t
		case "BYDAY":
			for _, day := range strings.Split(val, ",") {
				if parsed := weekdayNumber(strings.ToUpper(strings.TrimSpace(day))); parsed >= 0 {
					rule.Weekdays = append(rule.Weekdays, parsed)
				}
			}
		}
	}
	rule = recurrence.Normalize(rule, start)
	if err := recurrence.Validate(rule); err != nil {
		return nil, fmt.Errorf("unsupported RRULE: %w", err)
	}
	return rule, nil
}

func escapeText(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	value = strings.ReplaceAll(value, "\n", "\\n")
	value = strings.ReplaceAll(value, ",", "\\,")
	value = strings.ReplaceAll(value, ";", "\\;")
	return value
}

func unescapeText(value string) string {
	var b strings.Builder
	escaped := false
	for _, r := range value {
		if escaped {
			switch r {
			case 'n', 'N':
				b.WriteRune('\n')
			default:
				b.WriteRune(r)
			}
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
		} else {
			b.WriteRune(r)
		}
	}
	if escaped {
		b.WriteRune('\\')
	}
	return b.String()
}

func splitEscaped(value string, separator rune) []string {
	out := []string{}
	var b strings.Builder
	escaped := false
	for _, r := range value {
		if escaped {
			b.WriteRune('\\')
			b.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if r == separator {
			out = append(out, b.String())
			b.Reset()
		} else {
			b.WriteRune(r)
		}
	}
	out = append(out, b.String())
	return out
}

func foldLine(line string) []string {
	runes := []rune(line)
	if len(runes) <= 72 {
		return []string{line}
	}
	out := []string{}
	first := true
	for len(runes) > 0 {
		n := 72
		if !first {
			n = 71
		}
		if len(runes) < n {
			n = len(runes)
		}
		part := string(runes[:n])
		runes = runes[n:]
		if !first {
			part = " " + part
		}
		out = append(out, part)
		first = false
	}
	return out
}

func weekdayCode(day int) string {
	if day < 0 || day > 6 {
		return ""
	}
	return []string{"SU", "MO", "TU", "WE", "TH", "FR", "SA"}[day]
}

func weekdayNumber(code string) int {
	switch code {
	case "SU":
		return 0
	case "MO":
		return 1
	case "TU":
		return 2
	case "WE":
		return 3
	case "TH":
		return 4
	case "FR":
		return 5
	case "SA":
		return 6
	default:
		return -1
	}
}
