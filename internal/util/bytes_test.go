package util

import "testing"

func TestBytesFormat(t *testing.T) {
	var gib float64 = 1024 * 1024 * 1024
	cases := []struct {
		in   uint64
		want string
	}{
		{0, "0 B"},
		{7, "7 B"},
		{512, "512 B"},
		{1024, "1.00 KiB"},
		{1024 * 1024, "1.00 MiB"},
		{5 * 1024 * 1024, "5.00 MiB"},
		{uint64(4.72 * gib), "4.72 GiB"},
	}
	for _, c := range cases {
		if got := Bytes(c.in); got != c.want {
			t.Errorf("Bytes(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseBytes(t *testing.T) {
	cases := []struct {
		in   string
		want uint64
		ok   bool
	}{
		{"512K", 512 * 1024, true},
		{"1.5MiB", 1572864, true},
		{"123", 123, true},
		{"2.5 G", 2684354560, true},
		{"6.4 GiB", 6871947673, true}, // approx, see below
		{"bogus", 0, false},
		{"", 0, false},
		{"--", 0, false},
	}
	for _, c := range cases {
		got, ok := ParseBytes(c.in)
		if ok != c.ok {
			t.Errorf("ParseBytes(%q) ok = %v, want %v", c.in, ok, c.ok)
			continue
		}
		if !ok {
			continue
		}
		if c.in == "6.4 GiB" {
			if got < 6871947673-100 || got > 6871947673+100 {
				t.Errorf("ParseBytes(%q) = %d, want ~6871947673", c.in, got)
			}
			continue
		}
		if got != c.want {
			t.Errorf("ParseBytes(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}
