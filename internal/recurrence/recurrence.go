package recurrence

import (
	"errors"
	"sort"
	"time"
)

type Rule struct {
	Frequency       string     `json:"frequency"`
	Interval        int        `json:"interval"`
	Weekdays        []int      `json:"weekdays,omitempty"`
	Until           *time.Time `json:"until,omitempty"`
	OccurrenceCount *int       `json:"occurrence_count,omitempty"`
}

type Occurrence struct {
	Start time.Time
	End   time.Time
	Index int
}

func Validate(rule *Rule) error {
	if rule == nil {
		return nil
	}
	switch rule.Frequency {
	case "daily", "weekly", "monthly", "yearly":
	default:
		return errors.New("repeat type must be daily, weekly, monthly, or yearly")
	}
	if rule.Interval < 1 || rule.Interval > 365 {
		return errors.New("repeat interval must be between 1 and 365")
	}
	if rule.Until != nil && rule.OccurrenceCount != nil {
		return errors.New("choose either a repeat end date or a number of occurrences")
	}
	if rule.OccurrenceCount != nil && (*rule.OccurrenceCount < 1 || *rule.OccurrenceCount > 10000) {
		return errors.New("repeat count must be between 1 and 10000")
	}
	seen := map[int]bool{}
	for _, day := range rule.Weekdays {
		if day < 0 || day > 6 {
			return errors.New("repeat weekday is invalid")
		}
		if seen[day] {
			return errors.New("repeat weekdays must not contain duplicates")
		}
		seen[day] = true
	}
	if rule.Frequency != "weekly" && len(rule.Weekdays) > 0 {
		return errors.New("weekdays can only be selected for weekly repeats")
	}
	return nil
}

func Normalize(rule *Rule, start time.Time) *Rule {
	if rule == nil {
		return nil
	}
	out := *rule
	if out.Interval < 1 {
		out.Interval = 1
	}
	if out.Frequency == "weekly" {
		if len(out.Weekdays) == 0 {
			out.Weekdays = []int{int(start.Weekday())}
		}
		sort.Ints(out.Weekdays)
	} else {
		out.Weekdays = nil
	}
	return &out
}

func Expand(start, end time.Time, rule *Rule, from, to time.Time, max int) []Occurrence {
	if max <= 0 {
		max = 10000
	}
	if !end.After(start) || !to.After(from) {
		return nil
	}
	duration := end.Sub(start)
	if rule == nil {
		if start.Before(to) && end.After(from) {
			return []Occurrence{{Start: start, End: end, Index: 1}}
		}
		return nil
	}
	rule = Normalize(rule, start)
	if Validate(rule) != nil {
		return nil
	}

	switch rule.Frequency {
	case "daily":
		return expandDaily(start, duration, rule, from, to, max)
	case "weekly":
		return expandWeekly(start, duration, rule, from, to, max)
	case "monthly":
		return expandMonthly(start, duration, rule, from, to, max)
	case "yearly":
		return expandYearly(start, duration, rule, from, to, max)
	default:
		return nil
	}
}

func accepted(candidate time.Time, index int, rule *Rule) bool {
	if candidate.Before(time.Time{}) {
		return false
	}
	if rule.Until != nil && candidate.After(*rule.Until) {
		return false
	}
	if rule.OccurrenceCount != nil && index > *rule.OccurrenceCount {
		return false
	}
	return true
}

func overlaps(start time.Time, duration time.Duration, from, to time.Time) bool {
	return start.Before(to) && start.Add(duration).After(from)
}

func appendIfInRange(out []Occurrence, start time.Time, duration time.Duration, index int, from, to time.Time, max int) []Occurrence {
	if len(out) >= max {
		return out
	}
	if overlaps(start, duration, from, to) {
		return append(out, Occurrence{Start: start, End: start.Add(duration), Index: index})
	}
	return out
}

func expandDaily(start time.Time, duration time.Duration, rule *Rule, from, to time.Time, max int) []Occurrence {
	out := []Occurrence{}
	index := 1
	for candidate := start; candidate.Before(to) && index <= 10000; candidate = candidate.AddDate(0, 0, rule.Interval) {
		if !accepted(candidate, index, rule) {
			break
		}
		out = appendIfInRange(out, candidate, duration, index, from, to, max)
		if len(out) >= max {
			break
		}
		index++
	}
	return out
}

func expandWeekly(start time.Time, duration time.Duration, rule *Rule, from, to time.Time, max int) []Occurrence {
	out := []Occurrence{}
	wanted := map[time.Weekday]bool{}
	for _, d := range rule.Weekdays {
		wanted[time.Weekday(d)] = true
	}
	anchorWeek := weekStart(start)
	index := 0
	day := dayAtTime(start, start)
	limit := 0
	for day.Before(to) && limit < 70000 {
		limit++
		weeks := int(day.Sub(anchorWeek).Hours() / (24 * 7))
		if weeks >= 0 && weeks%rule.Interval == 0 && wanted[day.Weekday()] && !day.Before(start) {
			index++
			if !accepted(day, index, rule) {
				break
			}
			out = appendIfInRange(out, day, duration, index, from, to, max)
			if len(out) >= max {
				break
			}
		}
		day = day.AddDate(0, 0, 1)
	}
	return out
}

func expandMonthly(start time.Time, duration time.Duration, rule *Rule, from, to time.Time, max int) []Occurrence {
	out := []Occurrence{}
	index := 1
	year, month, day := start.Date()
	for step := 0; step < 10000; step++ {
		candidate := dateClamped(year, month, day, start)
		if step > 0 {
			candidate = addMonthsClamped(start, step*rule.Interval)
		}
		if !candidate.Before(to) {
			break
		}
		if !accepted(candidate, index, rule) {
			break
		}
		out = appendIfInRange(out, candidate, duration, index, from, to, max)
		if len(out) >= max {
			break
		}
		index++
	}
	return out
}

func expandYearly(start time.Time, duration time.Duration, rule *Rule, from, to time.Time, max int) []Occurrence {
	out := []Occurrence{}
	index := 1
	_, month, day := start.Date()
	for step := 0; step < 10000; step++ {
		year := start.Year() + step*rule.Interval
		candidate := dateClamped(year, month, day, start)
		if !candidate.Before(to) {
			break
		}
		if !accepted(candidate, index, rule) {
			break
		}
		out = appendIfInRange(out, candidate, duration, index, from, to, max)
		if len(out) >= max {
			break
		}
		index++
	}
	return out
}

func weekStart(t time.Time) time.Time {
	y, m, d := t.Date()
	base := time.Date(y, m, d, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
	return base.AddDate(0, 0, -int(base.Weekday()))
}

func dayAtTime(day, source time.Time) time.Time {
	y, m, d := day.Date()
	return time.Date(y, m, d, source.Hour(), source.Minute(), source.Second(), source.Nanosecond(), source.Location())
}

func addMonthsClamped(start time.Time, months int) time.Time {
	y, m, d := start.Date()
	total := int(m) - 1 + months
	y += total / 12
	total %= 12
	if total < 0 {
		total += 12
		y--
	}
	m = time.Month(total + 1)
	return dateClamped(y, m, d, start)
}

func dateClamped(year int, month time.Month, day int, source time.Time) time.Time {
	last := time.Date(year, month+1, 0, source.Hour(), source.Minute(), source.Second(), source.Nanosecond(), source.Location()).Day()
	if day > last {
		day = last
	}
	return time.Date(year, month, day, source.Hour(), source.Minute(), source.Second(), source.Nanosecond(), source.Location())
}
