// Package homemanager manages Home Manager generations via the official
// `home-manager` CLI. tnxstor never deletes generations itself.
package homemanager

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"tnxstor/internal/runner"
	"tnxstor/internal/util"
)

// Generation is a single Home Manager generation.
type Generation struct {
	Number  int
	Date    time.Time
	DateRaw string
	Path    string
	Current bool
}

// Available reports whether home-manager is installed.
func Available(r runner.Runner) bool {
	_, ok := r.LookPath("home-manager")
	return ok
}

// List runs `home-manager generations` and parses the output. Output formats
// vary slightly between versions; parsing is best-effort and raw output is
// always available to callers on failure.
func List(ctx context.Context, r runner.Runner) ([]Generation, string, error) {
	res := r.Run(ctx, "home-manager", "generations")
	if !res.OK() {
		return nil, res.Stdout + res.Stderr, res.Error()
	}
	gens := ParseGenerations(res.Stdout)
	return gens, res.Stdout, nil
}

var (
	// Dates like "2026-09-09 09:41:12", "2026-09-09T09:41:12", "2026-09-09 09:41"
	// or a bare "2026-09-09". Home Manager output formats vary between
	// versions, so parsing is deliberately tolerant.
	dateRe = regexp.MustCompile(`(\d{4}-\d{2}-\d{2})(?:[ T](\d{2}:\d{2})(?::(\d{2}))?)?`)
	// Store paths appearing in generation lines.
	pathRe = regexp.MustCompile(`/nix/store/[^\s]+`)
	// Explicit "No 128" / "Generation 128" style prefixes.
	noRe = regexp.MustCompile(`(?i)(?:no|gen(eration)?)\s+(\d+)`)
	// Current-generation markers.
	currentRe = regexp.MustCompile(`(?i)current`)
)

// ParseGenerations parses the text output of `home-manager generations`.
// It accepts the various layouts different home-manager versions print:
//
//	No 128  2026-09-09 09:41:12   (current)   /nix/store/...
//	128     2026-09-09 09:41:12   /nix/store/...
//	Generation 128  2026-09-09 09:41  (current)  /nix/store/...
func ParseGenerations(out string) []Generation {
	var gens []Generation
	seen := map[int]bool{}
	for _, line := range strings.Split(out, "\n") {
		dateM := dateRe.FindStringSubmatch(line)
		path := pathRe.FindString(line)
		if dateM == nil && path == "" {
			// Headers, blank lines and noise carry no date or path.
			continue
		}
		num := generationNumber(line)
		if num < 0 || seen[num] {
			continue
		}
		seen[num] = true
		g := Generation{
			Number:  num,
			Path:    path,
			Current: currentRe.MatchString(line),
		}
		if dateM != nil {
			g.DateRaw = strings.TrimSpace(dateM[1] + " " + dateM[2])
			for _, layout := range []string{
				"2006-01-02 15:04:05", "2006-01-02T15:04:05",
				"2006-01-02 15:04", "2006-01-02T15:04", "2006-01-02",
			} {
				if t, err := time.Parse(layout, g.DateRaw); err == nil {
					g.Date = t
					break
				}
			}
			if g.Date.IsZero() {
				if t, err := time.Parse("2006-01-02", dateM[1]); err == nil {
					g.Date = t
				}
			}
		}
		gens = append(gens, g)
	}
	// Newest first, matching home-manager's own ordering.
	for i := 1; i < len(gens); i++ {
		for j := i; j > 0 && gens[j].Number > gens[j-1].Number; j-- {
			gens[j], gens[j-1] = gens[j-1], gens[j]
		}
	}
	return gens
}

// generationNumber extracts the generation number from a line. It prefers
// explicit "No N"/"Generation N" prefixes and falls back to the first
// all-digit field short enough to be a generation number (never a store
// hash fragment).
func generationNumber(line string) int {
	if m := noRe.FindStringSubmatch(line); m != nil {
		if n, err := strconv.Atoi(m[2]); err == nil {
			return n
		}
	}
	for _, f := range strings.FieldsFunc(line, func(r rune) bool {
		return r == ' ' || r == '\t'
	}) {
		if len(f) <= 6 && isDigits(f) {
			n, err := strconv.Atoi(f)
			if err == nil {
				return n
			}
		}
	}
	return -1
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Expire runs `home-manager expire-generations "<period>"` with the GNU-date
// style period (e.g. "-30 days"). Destructive: removes generations.
func Expire(ctx context.Context, r runner.Runner, period string) (string, error) {
	res := r.Run(ctx, "home-manager", "expire-generations", period)
	if !res.OK() {
		return res.Stdout + res.Stderr, res.Error()
	}
	return res.Stdout + res.Stderr, nil
}

// GNUDatePeriod converts a tnxstor period like "30d" into the GNU date
// expression Home Manager expects: "-30 days".
func GNUDatePeriod(p string) (string, error) {
	days, err := util.ParseDays(p)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("-%d days", days), nil
}

// Eligible returns generations older than the given period. The current
// generation is never eligible.
func Eligible(gens []Generation, period string, now time.Time) []Generation {
	cutoff, err := util.TimeBefore(period, now)
	if err != nil {
		return nil
	}
	var out []Generation
	for _, g := range gens {
		if g.Current {
			continue
		}
		if !g.Date.IsZero() && g.Date.Before(cutoff) {
			out = append(out, g)
		}
	}
	return out
}
