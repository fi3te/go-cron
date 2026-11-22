package cron

import (
	"testing"
	"time"
)

// mapsEqual is a helper to compare maps of int->bool.
func mapsEqual(a, b map[int]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func sliceTo1BasedMap[T any](values []T) map[int]T {
	result := make(map[int]T)

	for i, value := range values {
		result[i+1] = value
	}

	return result
}

func sliceTo0BasedMap[T any](values []T) map[int]T {
	result := make(map[int]T)

	for i, value := range values {
		result[i] = value
	}

	return result
}

func TestParseCronExpressionValid(t *testing.T) {
	expr := "0 9 * * 1-5"
	schedule, err := parseCronExpression(expr)
	if err != nil {
		t.Fatalf("parseCronExpression(%q) returned error: %v", expr, err)
	}

	var expectedMinutes [60]bool
	expectedMinutes[0] = true
	if schedule.minutes != expectedMinutes {
		t.Errorf("minutes = %v, want {0}", schedule.minutes)
	}

	var expectedHours [24]bool
	expectedHours[9] = true
	if schedule.hours != expectedHours {
		t.Errorf("hours = %v, want {9}", schedule.hours)
	}

	if !mapsEqual(sliceTo1BasedMap(schedule.daysOfMonth[:]), fullRange(1, 31)) {
		t.Errorf("dayOfMonth = %v, want full range", schedule.daysOfMonth)
	}
	if !mapsEqual(sliceTo1BasedMap(schedule.months[:]), fullRange(1, 12)) {
		t.Errorf("months = %v, want full range", schedule.months)
	}
	if schedule.daysOfWeek != [8]bool{false, true, true, true, true, true, false, false} {
		t.Errorf("daysOfWeek = %v, want Mon-Fri", schedule.daysOfWeek)
	}
}

func TestParseCronExpressionWildcard(t *testing.T) {
	schedule, err := parseCronExpression("* * * * *")
	if err != nil {
		t.Fatalf("parseCronExpression returned error: %v", err)
	}
	if !mapsEqual(sliceTo0BasedMap(schedule.minutes[:]), fullRange(minuteMin, minuteMax)) {
		t.Errorf("minutes should use wildcard, got %v", schedule.minutes)
	}
	if !mapsEqual(sliceTo0BasedMap(schedule.hours[:]), fullRange(hourMin, hourMax)) {
		t.Errorf("hours should use wildcard, got %v", schedule.hours)
	}
	if !mapsEqual(sliceTo1BasedMap(schedule.daysOfMonth[:]), fullRange(dayMin, dayMax)) {
		t.Errorf("days of month should use wildcard, got %v", schedule.daysOfMonth)
	}
	if !mapsEqual(sliceTo1BasedMap(schedule.months[:]), fullRange(monthMin, monthMax)) {
		t.Errorf("months should use wildcard, got %v", schedule.months)
	}
	if !mapsEqual(sliceTo0BasedMap(schedule.daysOfWeek[:]), fullRange(weekdayMin, weekdayMax)) {
		t.Errorf("days of week should use wildcard, got %v", schedule.daysOfWeek)
	}
	if schedule.domRestricted == true {
		t.Errorf("domRestricted should be false")
	}
	if schedule.dowRestricted == true {
		t.Errorf("dowRestricted should be false")
	}
}

func TestParseCronExpressionInvalid(t *testing.T) {
	tests := []struct {
		name string
		expr string
	}{
		{"empty", ""},
		{"too few fields", "* * * *"},
		{"too many fields", "* * * * * *"},
		{"bad minute", "61 * * * *"},
		{"bad hour", "* 24 * * *"},
		{"bad day of month", "* * 0 * *"},
		{"bad month", "* * * 13 *"},
		{"bad day of week", "* * * * 8"},
		{"garbage", "foo bar baz qux quux"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseCronExpression(tt.expr); err == nil {
				t.Errorf("parseCronExpression(%q) expected error, got nil", tt.expr)
			}
		})
	}
}

func TestParseCronFieldWildcard(t *testing.T) {
	values, err := parseCronField("*", 0, 59)
	if err != nil {
		t.Fatalf("parseCronField returned error: %v", err)
	}
	if len(values) != 60 {
		t.Errorf("expected 60 values, got %d", len(values))
	}
}

func TestParseCronFieldSingleValue(t *testing.T) {
	values, err := parseCronField("15", 0, 59)
	if err != nil {
		t.Fatalf("parseCronField returned error: %v", err)
	}
	if !mapsEqual(values, map[int]bool{15: true}) {
		t.Errorf("values = %v, want {15}", values)
	}
}

func TestParseCronFieldStep(t *testing.T) {
	values, err := parseCronField("*/15", 0, 59)
	if err != nil {
		t.Fatalf("parseCronField returned error: %v", err)
	}
	want := map[int]bool{0: true, 15: true, 30: true, 45: true}
	if !mapsEqual(values, want) {
		t.Errorf("values = %v, want %v", values, want)
	}
}

func TestParseCronFieldRange(t *testing.T) {
	values, err := parseCronField("9-17", 0, 23)
	if err != nil {
		t.Fatalf("parseCronField returned error: %v", err)
	}
	want := map[int]bool{}
	for i := 9; i <= 17; i++ {
		want[i] = true
	}
	if !mapsEqual(values, want) {
		t.Errorf("values = %v, want %v", values, want)
	}
}

func TestParseCronFieldRangeWithStep(t *testing.T) {
	values, err := parseCronField("0-30/10", 0, 59)
	if err != nil {
		t.Fatalf("parseCronField returned error: %v", err)
	}
	want := map[int]bool{0: true, 10: true, 20: true, 30: true}
	if !mapsEqual(values, want) {
		t.Errorf("values = %v, want %v", values, want)
	}
}

func TestParseCronFieldCommaSeparated(t *testing.T) {
	values, err := parseCronField("0,15,30,45", 0, 59)
	if err != nil {
		t.Fatalf("parseCronField returned error: %v", err)
	}
	want := map[int]bool{0: true, 15: true, 30: true, 45: true}
	if !mapsEqual(values, want) {
		t.Errorf("values = %v, want %v", values, want)
	}
}

func TestParseCronFieldInvalid(t *testing.T) {
	tests := []struct {
		name  string
		field string
		min   int
		max   int
	}{
		{"non-numeric", "abc", 0, 59},
		{"above max", "60", 0, 59},
		{"below min", "-1", 0, 59},
		{"reversed range", "20-10", 0, 59},
		{"step zero", "*/0", 0, 59},
		{"step negative", "*/-5", 0, 59},
		{"bad step", "*/x", 0, 59},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseCronField(tt.field, tt.min, tt.max); err == nil {
				t.Errorf("parseCronField(%q) expected error, got nil", tt.field)
			}
		})
	}
}

func TestMatchesCron(t *testing.T) {
	schedule, err := parseCronExpression("0 9 * * 1-5")
	if err != nil {
		t.Fatalf("parseCronExpression returned error: %v", err)
	}

	// Mon-Fri at 9:00 should match.
	monday := time.Date(2026, time.May, 11, 9, 0, 0, 0, time.UTC) // Monday
	if !matchesCron(monday, schedule) {
		t.Error("expected Monday 09:00 to match")
	}

	// Different hour should not match.
	offHour := time.Date(2026, time.May, 11, 10, 0, 0, 0, time.UTC)
	if matchesCron(offHour, schedule) {
		t.Error("expected 10:00 not to match")
	}

	// Different minute should not match.
	offMinute := time.Date(2026, time.May, 11, 9, 30, 0, 0, time.UTC)
	if matchesCron(offMinute, schedule) {
		t.Error("expected 09:30 not to match")
	}

	// Weekend should not match.
	saturday := time.Date(2026, time.May, 16, 9, 0, 0, 0, time.UTC) // Saturday
	if matchesCron(saturday, schedule) {
		t.Error("expected Saturday 09:00 not to match")
	}
}

func TestMatchesCron_DomOrDowSemantics(t *testing.T) {
	// 13th of every month OR every Friday (Vixie cron OR semantics).
	schedule, err := parseCronExpression("0 0 13 * 5")
	if err != nil {
		t.Fatalf("parseCronExpression returned error: %v", err)
	}

	// A Friday (not the 13th) matches via day-of-week.
	friday := time.Date(2026, time.February, 13, 0, 0, 0, 0, time.UTC) // Friday the 13th
	if !matchesCron(friday, schedule) {
		t.Error("expected Friday 13th to match")
	}

	// Sunday the 15th (15 Feb 2026 is a Sunday) - not 13th, not Friday.
	sunday15 := time.Date(2026, time.February, 15, 0, 0, 0, 0, time.UTC)
	if matchesCron(sunday15, schedule) {
		t.Error("expected non-13th, non-Friday date not to match")
	}
}

func TestNextRunTime(t *testing.T) {
	cs := NewCronScheduler("* * * * *")
	s, err := cs.getSchedule()
	if err != nil {
		t.Fatalf("getSchedule returned error: %v", err)
	}
	now := time.Now()
	next, err := s.nextRunTime(now)
	if err != nil {
		t.Fatalf("nextRunTime returned error: %v", err)
	}
	if !next.After(now) {
		t.Errorf("next run %v should be after now %v", next, now)
	}
	if next.Sub(now) > 2*time.Minute {
		t.Errorf("next run %v is too far in the future for a per-minute schedule", next)
	}
}

func TestNextRunTimeHourly(t *testing.T) {
	cs := NewCronScheduler("0 * * * *")
	s, err := cs.getSchedule()
	if err != nil {
		t.Fatalf("getSchedule returned error: %v", err)
	}
	now := time.Now()
	next, err := s.nextRunTime(now)
	if err != nil {
		t.Fatalf("nextRunTime returned error: %v", err)
	}
	if next.Minute() != 0 {
		t.Errorf("next run should be at minute 0, got %d", next.Minute())
	}
}

func TestNextRunTimeSomeMinutesAfterTheLastRunOfTheDay(t *testing.T) {
	cs := NewCronScheduler("1 2 * * *")
	s, err := cs.getSchedule()
	if err != nil {
		t.Fatalf("getSchedule returned error: %v", err)
	}
	now := time.Date(2026, time.February, 11, 2, 15, 0, 0, time.UTC)
	next, err := s.nextRunTime(now)
	if err != nil {
		t.Fatalf("nextRunTime returned error: %v", err)
	}
	if next.Minute() != 1 {
		t.Errorf("next run should be at minute 1, got %d", next.Minute())
	}
	if next.Hour() != 2 {
		t.Errorf("next run should be at hour 2, got %d", next.Hour())
	}
	year, month, day := next.Date()
	if day != 12 {
		t.Errorf("next run should be at day 12, got %d", day)
	}
	if month != 2 {
		t.Errorf("next run should be at month 2, got %d", month)
	}
	if year != 2026 {
		t.Errorf("next run should be at year 2026, got %d", year)
	}
}

func TestNextRunTimeSomeHoursAfterTheLastRunOfTheDay(t *testing.T) {
	cs := NewCronScheduler("10 2 * * *")
	s, err := cs.getSchedule()
	if err != nil {
		t.Fatalf("getSchedule returned error: %v", err)
	}
	now := time.Date(2026, time.February, 11, 4, 5, 0, 0, time.UTC)
	next, err := s.nextRunTime(now)
	if err != nil {
		t.Fatalf("nextRunTime returned error: %v", err)
	}
	if next.Minute() != 10 {
		t.Errorf("next run should be at minute 10, got %d", next.Minute())
	}
	if next.Hour() != 2 {
		t.Errorf("next run should be at hour 2, got %d", next.Hour())
	}
	year, month, day := next.Date()
	if day != 12 {
		t.Errorf("next run should be at day 12, got %d", day)
	}
	if month != 2 {
		t.Errorf("next run should be at month 2, got %d", month)
	}
	if year != 2026 {
		t.Errorf("next run should be at year 2026, got %d", year)
	}
}
