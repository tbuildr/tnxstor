// Package nix orchestrates the official nix-store / nix tools. tnxstor never
// deletes anything from /nix/store itself; reachability and deletion decisions
// are delegated to Nix's garbage collector.
package nix

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"tnxstor/internal/runner"
	"tnxstor/internal/util"
)

// DeadPaths is the parsed result of `nix-store --gc --print-dead`.
type DeadPaths struct {
	Paths      []string
	TotalBytes uint64
	BytesKnown bool
	Raw        string
}

// GCResult is the parsed result of `nix-store --gc`.
type GCResult struct {
	FreedBytes uint64
	FreedKnown bool
	Raw        string
}

// Roots is the parsed result of `nix-store --gc --print-roots`.
type Root struct {
	Link   string
	Target string
	Class  string // Home Manager, devenv, User profiles, Manual roots, Other
}

// OptimiseResult captures `nix-store --optimise` output.
type OptimiseResult struct {
	Raw string
}

// ---- command wrappers -------------------------------------------------

// GC runs `nix-store --gc`. Destructive: collects unreachable store paths.
func GC(ctx context.Context, r runner.Runner) (GCResult, error) {
	res := r.Run(ctx, "nix-store", "--gc")
	if !res.OK() {
		return GCResult{Raw: res.Stdout + res.Stderr}, res.Error()
	}
	freed, known := ParseGCFreed(res.Stdout + res.Stderr)
	return GCResult{FreedBytes: freed, FreedKnown: known, Raw: res.Stdout + res.Stderr}, nil
}

// GCOld runs `nix-collect-garbage -d`. Destructive: also removes old profile
// generations, so rollback points are lost.
func GCOld(ctx context.Context, r runner.Runner) (GCResult, error) {
	res := r.Run(ctx, "nix-collect-garbage", "-d")
	if !res.OK() {
		return GCResult{Raw: res.Stdout + res.Stderr}, res.Error()
	}
	freed, known := ParseGCFreed(res.Stdout + res.Stderr)
	return GCResult{FreedBytes: freed, FreedKnown: known, Raw: res.Stdout + res.Stderr}, nil
}

// DeadPaths runs `nix-store --gc --print-dead`. Read-only preview.
func Dead(ctx context.Context, r runner.Runner, limit int) (DeadPaths, error) {
	res := r.Run(ctx, "nix-store", "--gc", "--print-dead")
	out := res.Stdout
	if !res.OK() {
		return DeadPaths{Raw: out + res.Stderr}, res.Error()
	}
	paths, total, known := ParseDeadPaths(out)
	if limit > 0 && len(paths) > limit {
		paths = paths[:limit]
	}
	return DeadPaths{Paths: paths, TotalBytes: total, BytesKnown: known, Raw: out}, nil
}

// LivePaths runs `nix-store --gc --print-live`. Diagnostic only.
func LivePaths(ctx context.Context, r runner.Runner) (string, error) {
	res := r.Run(ctx, "nix-store", "--gc", "--print-live")
	if !res.OK() {
		return res.Stdout + res.Stderr, res.Error()
	}
	return res.Stdout, nil
}

// RootsList runs `nix-store --gc --print-roots`. Diagnostic only.
func RootsList(ctx context.Context, r runner.Runner) ([]Root, string, error) {
	res := r.Run(ctx, "nix-store", "--gc", "--print-roots")
	out := res.Stdout
	if !res.OK() {
		return nil, out + res.Stderr, res.Error()
	}
	roots := ParseRoots(out)
	for i := range roots {
		roots[i].Class = ClassifyRoot(roots[i].Link)
	}
	return roots, out, nil
}

// Optimise runs `nix-store --optimise`. Deduplicates identical files by
// hard-linking; can be expensive.
func Optimise(ctx context.Context, r runner.Runner) (OptimiseResult, error) {
	res := r.Run(ctx, "nix-store", "--optimise")
	if !res.OK() {
		return OptimiseResult{Raw: res.Stdout + res.Stderr}, res.Error()
	}
	return OptimiseResult{Raw: res.Stdout + res.Stderr}, nil
}

// StorePath discovers the store directory from Nix output and falls back to
// /nix/store. It only inspects read-only command output.
func StorePath(ctx context.Context, r runner.Runner) string {
	// The store path is the prefix of any printed store path.
	if res := r.Run(ctx, "nix-store", "--gc", "--print-dead"); res.OK() && len(res.Stdout) > 0 {
		line := strings.TrimSpace(strings.SplitN(res.Stdout, "\n", 2)[0])
		if strings.HasPrefix(line, "/") {
			if i := strings.Index(line, "/store/"); i > 0 {
				return line[:i+len("/store")]
			}
		}
	}
	return "/nix/store"
}

// ---- parsers ------------------------------------------------------------

// ParseGCFreed extracts the freed byte count from `nix-store --gc` output.
// The exact wording may vary between Nix versions; we scan for the last
// "<number> <unit> freed" style phrase. If nothing can be parsed the caller
// should display raw output instead.
func ParseGCFreed(out string) (uint64, bool) {
	words := strings.FieldsFunc(out, func(c rune) bool {
		return c == '\n' || c == ','
	})
	// Try line-based first: look for "<n> <unit> freed"
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(strings.TrimSpace(line))
		if len(f) >= 3 && f[len(f)-1] == "freed" {
			if n, ok := util.ParseBytes(f[len(f)-3] + " " + f[len(f)-2]); ok {
				return n, true
			}
		}
	}
	_ = words
	return 0, false
}

// ParseDeadPaths parses `nix-store --gc --print-dead` output. The final block
// contains "<n> store paths deleted, <size> freed" only for --gc; for
// --print-dead Nix prints the dead paths followed by a blank line and the
// "Nix would delete..." summary line in some versions. We treat every line
// starting with '/' that looks like a store path as a dead path and try to
// pick a total size from a summary line if present.
func ParseDeadPaths(out string) ([]string, uint64, bool) {
	var paths []string
	var total uint64
	known := false
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if !strings.HasPrefix(t, "/") {
			// Possible summary line, e.g.:
			// "137 store paths deleted, 6.4 GiB freed" (gc) or
			// "Nix would delete: 137 paths, 6.4 GiB"
			if n, ok := parseSummarySize(t); ok {
				total, known = n, true
			}
			continue
		}
		paths = append(paths, t)
	}
	return paths, total, known
}

func parseSummarySize(line string) (uint64, bool) {
	f := strings.Fields(line)
	for i := 0; i < len(f)-1; i++ {
		if f[i+1] == "freed" || f[i+1] == "reclaimable" {
			if n, ok := util.ParseBytes(f[i]); ok {
				return n, true
			}
		}
	}
	return 0, false
}

// ParseRoots parses `nix-store --gc --print-roots` which emits lines like:
//
//	/nix/var/nix/profiles/per-user/root/profile -> /nix/store/...
func ParseRoots(out string) []Root {
	var roots []Root
	seen := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "->")
		if len(parts) != 2 {
			continue
		}
		link := strings.TrimSpace(parts[0])
		target := strings.TrimSpace(parts[1])
		if link == "" || seen[link] {
			continue
		}
		seen[link] = true
		roots = append(roots, Root{Link: link, Target: target})
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i].Link < roots[j].Link })
	return roots
}

// ClassifyRoot maps a GC root link path to a human category. Unknown roots
// are classified as "Other"; classification is best effort.
func ClassifyRoot(link string) string {
	l := link
	switch {
	case strings.Contains(l, "/home-manager") ||
		strings.Contains(l, "/profiles/home-manager") ||
		strings.Contains(l, "/.home-manager-build") ||
		(strings.Contains(l, "/.local/state/nix/profiles") && strings.Contains(l, "home")):
		return "Home Manager"
	case strings.Contains(l, "devenv"):
		return "devenv"
	case strings.Contains(l, "/profiles/per-user/") ||
		strings.Contains(l, "/nix/var/nix/profiles") ||
		strings.Contains(l, "/.local/state/nix/profiles") ||
		strings.Contains(l, "/.nix-profile"):
		return "User profiles"
	case strings.Contains(l, "/result") || strings.Contains(l, "auto/gc"):
		return "Manual roots"
	default:
		return "Other"
	}
}

// StoreSize measures the store directory using du (read-only, may be slow).
func StoreSize(ctx context.Context, r runner.Runner, store string) (uint64, bool) {
	res := r.Run(ctx, "du", "-sb", store)
	if !res.OK() {
		return 0, false
	}
	f := strings.Fields(strings.TrimSpace(res.Stdout))
	if len(f) == 0 {
		return 0, false
	}
	var n uint64
	if _, err := fmt.Sscanf(f[0], "%d", &n); err == nil {
		return n, true
	}
	return 0, false
}

// CountLines counts non-empty lines (helper for path counts).
func CountLines(s string) int {
	n := 0
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			n++
		}
	}
	return n
}
