package util

import (
	"fmt"
	"regexp"
	"time"
)

var ageRe = regexp.MustCompile(`^(\d+)([dwmhy])$`)

// DurationUnit converts a human period such as "30d", "14d", "7d", "2w",
// "6m", "1y" into (days, nil). Home Manager's expire-generations accepts a
// date expression; we canonicalise to days for presentation while passing
// a GNU-date style period through.
func ParseDays(s string) (int, error) {
	m := ageRe.FindStringSubmatch(s)
	if m == nil {
		return 0, fmt.Errorf("invalid period %q: use forms like 30d, 12w, 6m, 1y", s)
	}
	n := 0
	for _, c := range m[1] {
		n = n*10 + int(c-'0')
	}
	switch m[2] {
	case "d":
		return n, nil
	case "w":
		return n * 7, nil
	case "m":
		return n * 30, nil
	case "h":
		if n < 24 {
			return 0, fmt.Errorf("period %q is shorter than one day", s)
		}
		return n / 24, nil
	case "y":
		return n * 365, nil
	}
	return 0, fmt.Errorf("invalid period %q", s)
}

// HumanPeriod renders a period like "30d" for display.
func HumanPeriod(s string) string {
	days, err := ParseDays(s)
	if err != nil {
		return s
	}
	return fmt.Sprintf("%d days", days)
}

// TimeBefore returns the time cutoff for the given period string.
func TimeBefore(s string, now time.Time) (time.Time, error) {
	m := ageRe.FindStringSubmatch(s)
	if m == nil {
		return time.Time{}, fmt.Errorf("invalid period %q", s)
	}
	n := 0
	for _, c := range m[1] {
		n = n*10 + int(c-'0')
	}
	switch m[2] {
	case "d":
		return now.AddDate(0, 0, -n), nil
	case "w":
		return now.AddDate(0, 0, -n*7), nil
	case "m":
		return now.AddDate(0, -n, 0), nil
	case "h":
		return now.Add(-time.Duration(n) * time.Hour), nil
	case "y":
		return now.AddDate(-n, 0, 0), nil
	}
	return time.Time{}, fmt.Errorf("invalid period %q", s)
}
