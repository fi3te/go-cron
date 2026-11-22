package cron

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Field bounds for the standard 5-field CRON format.
const (
	minuteMin, minuteMax   = 0, 59
	hourMin, hourMax       = 0, 23
	dayMin, dayMax         = 1, 31
	monthMin, monthMax     = 1, 12
	weekdayMin, weekdayMax = 0, 7
)

// cronSchedule holds the parsed CRON fields as dense boolean lookup tables.
// Using fixed-size arrays instead of maps avoids hashing overhead during the
// hot scheduling loop and makes the "restricted" state trivially known.
type cronSchedule struct {
	minutes     [minuteMax - minuteMin + 1]bool
	hours       [hourMax - hourMin + 1]bool
	daysOfMonth [dayMax - dayMin + 1]bool
	months      [monthMax - monthMin + 1]bool
	daysOfWeek  [weekdayMax - weekdayMin + 1]bool

	// domRestricted and dowRestricted track whether the corresponding field
	// is a wildcard. Precomputing them avoids repeated length checks.
	domRestricted bool
	dowRestricted bool
}

// parseCronExpression parses and validates a 5-field CRON expression.
func parseCronExpression(expr string) (cronSchedule, error) {
	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return cronSchedule{}, fmt.Errorf("invalid CRON expression %q: expected 5 fields, got %d", expr, len(parts))
	}

	minutes, err := parseCronField(parts[0], minuteMin, minuteMax)
	if err != nil {
		return cronSchedule{}, fmt.Errorf("invalid minute field %q: %w", parts[0], err)
	}
	hours, err := parseCronField(parts[1], hourMin, hourMax)
	if err != nil {
		return cronSchedule{}, fmt.Errorf("invalid hour field %q: %w", parts[1], err)
	}
	daysOfMonth, err := parseCronField(parts[2], dayMin, dayMax)
	if err != nil {
		return cronSchedule{}, fmt.Errorf("invalid day-of-month field %q: %w", parts[2], err)
	}
	months, err := parseCronField(parts[3], monthMin, monthMax)
	if err != nil {
		return cronSchedule{}, fmt.Errorf("invalid month field %q: %w", parts[3], err)
	}
	daysOfWeek, err := parseCronField(parts[4], weekdayMin, weekdayMax)
	if err != nil {
		return cronSchedule{}, fmt.Errorf("invalid day-of-week field %q: %w", parts[4], err)
	}

	var schedule cronSchedule
	setField(new(schedule.minutes[:]), minutes, minuteMin)
	setField(new(schedule.hours[:]), hours, hourMin)
	setField(new(schedule.daysOfMonth[:]), daysOfMonth, dayMin)
	setField(new(schedule.months[:]), months, monthMin)
	setField(new(schedule.daysOfWeek[:]), daysOfWeek, weekdayMin)
	schedule.domRestricted = len(daysOfMonth) < dayMax
	schedule.dowRestricted = len(daysOfWeek) < weekdayMax

	return schedule, nil
}

// setField maps a parsed set of values into the dense boolean array of a
// cronSchedule field, where the value v sets the flag at index v-min.
func setField(dst *[]bool, values map[int]bool, min int) {
	for v := range values {
		if idx := v - min; idx >= 0 && idx < len(*dst) {
			(*dst)[idx] = true
		}
	}
}

// parseCronField parses a single CRON field into a set of valid values.
// It supports wildcards (*), step values (*/n and a-b/n), ranges (a-b),
// and comma-separated lists of any of the previous forms.
func parseCronField(field string, min, max int) (map[int]bool, error) {
	valid := make(map[int]bool)

	if field == "*" {
		return fullRange(min, max), nil
	}

	for _, part := range strings.Split(field, ",") {
		values, err := parseCronPart(part, min, max)
		if err != nil {
			return nil, err
		}
		for v := range values {
			valid[v] = true
		}
	}
	return valid, nil
}

// parseCronPart parses a single CRON expression component into a set of values.
func parseCronPart(part string, min, max int) (map[int]bool, error) {
	// Handle optional step values: */n, a-b/n, or value/n.
	step := 1
	if idx := strings.Index(part, "/"); idx >= 0 {
		s, err := strconv.Atoi(part[idx+1:])
		if err != nil || s < 1 {
			return nil, fmt.Errorf("invalid step value %q", part[idx+1:])
		}
		step = s
		part = part[:idx]
	}

	var start, end int
	switch {
	case part == "*":
		start, end = min, max
	case strings.Contains(part, "-"):
		bounds := strings.SplitN(part, "-", 2)
		var err error
		start, err = strconv.Atoi(bounds[0])
		if err != nil {
			return nil, fmt.Errorf("invalid range start %q", bounds[0])
		}
		end, err = strconv.Atoi(bounds[1])
		if err != nil {
			return nil, fmt.Errorf("invalid range end %q", bounds[1])
		}
	default:
		v, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid value %q", part)
		}
		start, end = v, v
	}

	if start < min || end < start || end > max {
		return nil, fmt.Errorf("value %d-%d out of range [%d, %d]", start, end, min, max)
	}

	values := make(map[int]bool)
	for v := start; v <= end; v += step {
		values[v] = true
	}
	return values, nil
}

// fullRange returns a set containing every integer from min to max inclusive.
func fullRange(min, max int) map[int]bool {
	values := make(map[int]bool, max-min+1)
	for i := min; i <= max; i++ {
		values[i] = true
	}
	return values
}

// nextRunTime computes the next run time strictly after the given reference
// time. Instead of stepping minute-by-minute, it skips directly to the next
// matching month, then day, then the earliest matching hour:minute.
func (s *cronSchedule) nextRunTime(now time.Time) (time.Time, error) {
	// Start at the next minute boundary so we never return "now".
	current := now.Truncate(time.Minute).Add(time.Minute)

	// Iterate over candidate days. The loop counter bounds the search to a
	// year of wall-clock days; advancing more than a day at a time is fine.
	for i := 0; i < 366; i++ {
		// Skip months that can never match by jumping straight to their end.
		if !s.months[int(current.Month())-monthMin] {
			current = time.Date(current.Year(), current.Month()+1, 1, 0, 0, 0, 0, current.Location())
			continue
		}
		// Skip days that do not match the day-of-month / day-of-week rules.
		if !s.dayMatches(current) {
			current = time.Date(current.Year(), current.Month(), current.Day()+1, 0, 0, 0, 0, current.Location())
			continue
		}
		// The current day matches; find the earliest matching hour:minute.
		if earliest, ok := s.firstMatchingTime(current); ok {
			return earliest, nil
		}
		// No matching time remains today, so continue with tomorrow.
		current = time.Date(current.Year(), current.Month(), current.Day()+1, 0, 0, 0, 0, current.Location())
	}

	return time.Time{}, fmt.Errorf("could not find next run time within a year for expression")
}

// firstMatchingTime returns the earliest time on the same calendar day as t
// whose hour and minute both match the schedule and is at or after t.
func (s *cronSchedule) firstMatchingTime(t time.Time) (time.Time, bool) {
	startHour := t.Hour()
	startMinute := t.Minute()

	for h := startHour; h <= hourMax; h++ {
		if !s.hours[h-hourMin] {
			continue
		}
		// For the first candidate hour the minute must be at or after the
		// start; for later hours any matching minute is acceptable.
		mStart := minuteMin
		if h == startHour {
			mStart = startMinute
		}
		for m := mStart; m <= minuteMax; m++ {
			if s.minutes[m-minuteMin] {
				return time.Date(t.Year(), t.Month(), t.Day(), h, m, 0, 0, t.Location()), true
			}
		}
	}
	return time.Time{}, false
}

// dayMatches reports whether the given date satisfies the day constraints,
// applying standard CRON OR semantics when both fields are restricted.
func (s *cronSchedule) dayMatches(t time.Time) bool {
	domOk := s.daysOfMonth[t.Day()-dayMin]
	weekday := t.Weekday()
	var dowOk bool
	if weekday == time.Sunday {
		dowOk = s.daysOfWeek[weekdayMin] || s.daysOfWeek[weekdayMax]
	} else {
		dowOk = s.daysOfWeek[int(weekday)-weekdayMin]
	}

	switch {
	case s.domRestricted && s.dowRestricted:
		return domOk || dowOk
	case s.domRestricted:
		return domOk
	case s.dowRestricted:
		return dowOk
	default:
		return true
	}
}

// matchesCron reports whether the given time matches the parsed CRON schedule.
// It mirrors dayMatches but always checks the full set of fields.
func matchesCron(t time.Time, s cronSchedule) bool {
	if !s.minutes[t.Minute()-minuteMin] {
		return false
	}
	if !s.hours[t.Hour()-hourMin] {
		return false
	}
	if !s.months[int(t.Month())-monthMin] {
		return false
	}
	return s.dayMatches(t)
}
