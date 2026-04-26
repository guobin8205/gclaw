package cron

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Expr represents a parsed 5-field cron expression (minute hour dom month dow).
type Expr struct {
	Minutes     field
	Hours       field
	DaysOfMonth field
	Months      field
	DaysOfWeek  field
}

type field struct {
	all      bool          // *
	exact    []int         // exact values
	step     int           // step interval (*/5 = every 5)
	stepBase int           // base for step (0 unless specified)
}

// Parse parses a 5-field cron expression.
// Format: minute hour day-of-month month day-of-week
func Parse(expr string) (*Expr, error) {
	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return nil, fmt.Errorf("cron expression must have 5 fields, got %d: %q", len(parts), expr)
	}

	e := &Expr{}
	var err error
	e.Minutes, err = parseField(parts[0], 0, 59)
	if err != nil {
		return nil, fmt.Errorf("minutes field: %w", err)
	}
	e.Hours, err = parseField(parts[1], 0, 23)
	if err != nil {
		return nil, fmt.Errorf("hours field: %w", err)
	}
	e.DaysOfMonth, err = parseField(parts[2], 1, 31)
	if err != nil {
		return nil, fmt.Errorf("day-of-month field: %w", err)
	}
	e.Months, err = parseField(parts[3], 1, 12)
	if err != nil {
		return nil, fmt.Errorf("months field: %w", err)
	}
	e.DaysOfWeek, err = parseField(parts[4], 0, 6)
	if err != nil {
		return nil, fmt.Errorf("day-of-week field: %w", err)
	}
	return e, nil
}

func parseField(s string, min, max int) (field, error) {
	s = strings.TrimSpace(s)
	if s == "*" {
		return field{all: true}, nil
	}

	// Step: */5 or 1/5
	if strings.Contains(s, "/") {
		parts := strings.SplitN(s, "/", 2)

		var base int
		if parts[0] == "*" {
			base = min
		} else {
			var err error
			base, err = strconv.Atoi(parts[0])
			if err != nil {
				return field{}, fmt.Errorf("invalid step base: %q", parts[0])
			}
		}

		step, err := strconv.Atoi(parts[1])
		if err != nil || step < 1 {
			return field{}, fmt.Errorf("invalid step: %q", parts[1])
		}

		return field{step: step, stepBase: base}, nil
	}

	// Comma-separated list (each item may be a value or range)
	if strings.Contains(s, ",") {
		var exact []int
		for _, part := range strings.Split(s, ",") {
			vals, err := parseValueOrRange(strings.TrimSpace(part), min, max)
			if err != nil {
				return field{}, err
			}
			exact = append(exact, vals...)
		}
		return field{exact: exact}, nil
	}

	// Range: 1-5
	if strings.Contains(s, "-") {
		return parseRange(s, min, max)
	}

	// Single value
	v, err := strconv.Atoi(s)
	if err != nil {
		return field{}, fmt.Errorf("invalid value: %q", s)
	}
	if v < min || v > max {
		return field{}, fmt.Errorf("value %d out of range [%d, %d]", v, min, max)
	}
	return field{exact: []int{v}}, nil
}

func parseValueOrRange(s string, min, max int) ([]int, error) {
	if strings.Contains(s, "-") {
		f, err := parseRange(s, min, max)
		if err != nil {
			return nil, err
		}
		return f.exact, nil
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return nil, fmt.Errorf("invalid value: %q", s)
	}
	if v < min || v > max {
		return nil, fmt.Errorf("value %d out of range [%d, %d]", v, min, max)
	}
	return []int{v}, nil
}

func parseRange(s string, min, max int) (field, error) {
	parts := strings.SplitN(s, "-", 2)
	lo, err := strconv.Atoi(parts[0])
	if err != nil {
		return field{}, fmt.Errorf("invalid range start: %q", parts[0])
	}
	hi, err := strconv.Atoi(parts[1])
	if err != nil {
		return field{}, fmt.Errorf("invalid range end: %q", parts[1])
	}
	if lo < min || hi > max || lo > hi {
		return field{}, fmt.Errorf("range %d-%d out of bounds [%d, %d]", lo, hi, min, max)
	}
	var vals []int
	for i := lo; i <= hi; i++ {
		vals = append(vals, i)
	}
	return field{exact: vals}, nil
}

// matches checks if a time value matches a field.
func (f field) matches(v int) bool {
	if f.all {
		return true
	}
	if f.step > 0 {
		return (v-f.stepBase)%f.step == 0 && v >= f.stepBase
	}
	for _, exact := range f.exact {
		if exact == v {
			return true
		}
	}
	return false
}

// next returns the next matching value >= start, optionally wrapping.
func (f field) next(start int, max int) (int, bool) {
	if f.all {
		return start, true
	}
	if f.step > 0 {
		if start < f.stepBase {
			return f.stepBase, true
		}
		next := start + (f.step - (start-f.stepBase)%f.step)
		if next > max {
			return 0, false
		}
		return next, true
	}
	for _, exact := range f.exact {
		if exact >= start {
			return exact, true
		}
	}
	return 0, false
}

// first returns the smallest matching value.
func (f field) first() int {
	if f.all {
		return 0
	}
	if f.step > 0 {
		return f.stepBase
	}
	min := f.exact[0]
	for _, v := range f.exact {
		if v < min {
			min = v
		}
	}
	return min
}

// nextRun computes the next time matching the cron expression after 'after'.
func nextRun(expr string, after time.Time, loc *time.Location) time.Time {
	e, err := Parse(expr)
	if err != nil {
		return time.Time{}
	}

	t := after.In(loc).Add(time.Minute).Truncate(time.Minute)

	// Search up to 4 years ahead
	deadline := after.Add(4 * 365 * 24 * time.Hour)
	for t.Before(deadline) {
		if !e.Months.matches(int(t.Month())) {
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, loc)
			continue
		}

		if !e.DaysOfMonth.matches(t.Day()) || !e.DaysOfWeek.matches(int(t.Weekday())) {
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, loc)
			continue
		}

		if !e.Hours.matches(t.Hour()) {
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, loc)
			continue
		}

		if e.Minutes.matches(t.Minute()) {
			return t
		}

		t = t.Add(time.Minute)
	}

	return time.Time{}
}
