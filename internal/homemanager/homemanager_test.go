package homemanager

import (
	"testing"
	"time"
)

const sampleGenerations = `No 128  2026-09-09 09:41:12   (current)   /nix/store/aaaa-home-manager-generation
No 127  2026-09-08 22:17:03   /nix/store/bbbb-home-manager-generation
No 126  2026-09-07 18:32:44   /nix/store/cccc-home-manager-generation
No 100  2026-08-01 12:01:00   /nix/store/dddd-home-manager-generation
`

func TestParseGenerations(t *testing.T) {
	gens := ParseGenerations(sampleGenerations)
	if len(gens) != 4 {
		t.Fatalf("got %d generations, want 4", len(gens))
	}
	if gens[0].Number != 128 || !gens[0].Current {
		t.Errorf("first generation wrong: %+v", gens[0])
	}
	if gens[1].Current {
		t.Error("generation 127 must not be current")
	}
	if gens[3].Date.Month() != time.August || gens[3].Date.Day() != 1 {
		t.Errorf("date parse failed: %v", gens[3].Date)
	}
}

func TestParseGenerationsGarbage(t *testing.T) {
	if got := ParseGenerations("total nonsense\n"); len(got) != 0 {
		t.Errorf("expected 0 generations, got %d", len(got))
	}
}

func TestParseGenerationsLegacyFormat(t *testing.T) {
	// Older home-manager versions omit the "No" prefix.
	out := `128   2026-09-09 09:41:12   /nix/store/aaaa-home-manager-generation
127   2026-09-08 22:17:03   /nix/store/bbbb-home-manager-generation
`
	gens := ParseGenerations(out)
	if len(gens) != 2 {
		t.Fatalf("got %d, want 2", len(gens))
	}
	if gens[0].Number != 128 || gens[1].Number != 127 {
		t.Errorf("numbers wrong: %+v", gens)
	}
}

func TestParseGenerationsGenerationPrefix(t *testing.T) {
	out := `Generation 3  2025-01-01 10:00:00  (current)  /nix/store/xxxx-home-manager-generation
Generation 2  2024-01-01 10:00:00  /nix/store/yyyy-home-manager-generation
`
	gens := ParseGenerations(out)
	if len(gens) != 2 {
		t.Fatalf("got %d, want 2: %+v", len(gens), gens)
	}
	if !gens[0].Current || gens[0].Number != 3 {
		t.Errorf("current/prefix parse wrong: %+v", gens[0])
	}
}

func TestParseGenerationsIgnoresHeadersAndHashes(t *testing.T) {
	// Header lines and store hashes must not become generations.
	out := `CURRENT   DATE                    GENERATION
No 42    2026-01-01 00:00:00   /nix/store/12345678901234567890123456789012-hm-42
`
	gens := ParseGenerations(out)
	if len(gens) != 1 || gens[0].Number != 42 {
		t.Errorf("got %+v, want one generation 42", gens)
	}
}

func TestGNUDatePeriod(t *testing.T) {
	p, err := GNUDatePeriod("30d")
	if err != nil {
		t.Fatal(err)
	}
	if p != "-30 days" {
		t.Errorf("got %q, want %q", p, "-30 days")
	}
	if _, err := GNUDatePeriod("nope"); err == nil {
		t.Error("expected error")
	}
}

func TestEligible(t *testing.T) {
	gens := ParseGenerations(sampleGenerations)
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	// Generations older than 7 days.
	el := Eligible(gens, "7d", now)
	if len(el) != 1 || el[0].Number != 100 {
		t.Errorf("eligible = %+v, want only generation 100", el)
	}
	// A very old generation matches a long period; the current generation is
	// never eligible no matter the period.
	gens2 := ParseGenerations(`No 128  2026-09-09 09:41:12   (current)   /nix/store/aaaa-home-manager-generation
No 50   2020-01-01 00:00:00   /nix/store/old-home-manager-generation
`)
	el = Eligible(gens2, "1y", now)
	if len(el) != 1 || el[0].Number != 50 {
		t.Errorf("eligible = %+v, want only generation 50", el)
	}
}
