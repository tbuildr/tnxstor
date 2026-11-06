package util

import (
	"testing"
	"time"
)

func TestParseDays(t *testing.T) {
	cases := []struct {
		in   string
		want int
		ok   bool
	}{
		{"30d", 30, true},
		{"14d", 14, true},
		{"7d", 7, true},
		{"12w", 84, true},
		{"6m", 180, true},
		{"1y", 365, true},
		{"30", 0, false},
		{"d", 0, false},
		{"", 0, false},
		{"-30d", 0, false},
	}
	for _, c := range cases {
		got, err := ParseDays(c.in)
		if (err == nil) != c.ok {
			t.Errorf("ParseDays(%q) err = %v, want ok=%v", c.in, err, c.ok)
			continue
		}
		if c.ok && got != c.want {
			t.Errorf("ParseDays(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestTimeBefore(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	cut, err := TimeBefore("30d", now)
	if err != nil {
		t.Fatal(err)
	}
	want := now.AddDate(0, 0, -30)
	if !cut.Equal(want) {
		t.Errorf("TimeBefore(30d) = %v, want %v", cut, want)
	}
	if _, err := TimeBefore("banana", now); err == nil {
		t.Error("expected error for invalid period")
	}
}
