package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"tnxstor/internal/operations"
	"tnxstor/internal/runner"
)

func TestExpandOutputNoInterleavedBlankLines(t *testing.T) {
	m := newModel()
	tm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = tm.(model)
	raw := "line1\nline2\nline3\nline4\nline5"
	tm, _ = m.Update(opDoneMsg{op: opDead, res: runResult{
		title: "Dead store paths", summary: "5 paths", raw: raw, ok: true,
	}})
	m = tm.(model)
	m.expanded = true
	// Pump animation frames so the panel spring settles.
	for i := 0; i < 90; i++ {
		tm, _ = m.Update(animTick{})
		m = tm.(model)
	}
	tm, _ = m.Update(animTick{})
	m = tm.(model)
	view := m.View()

	// Inside the panel there must be no double blank lines.
	if strings.Contains(view, "\n\n\n") {
		t.Errorf("found interleaved blank lines:\n%q", view)
	}
	// All five lines must appear in order without blank lines between them.
	i1 := strings.Index(view, "line1")
	i5 := strings.Index(view, "line5")
	if i1 < 0 || i5 < 0 {
		t.Fatalf("content missing from view")
	}
	seg := view[i1 : i5+5]
	if strings.Contains(seg, "\n\n") {
		t.Errorf("blank line between content lines:\n%q", seg)
	}
}

func TestExpandOutputLongLinesNoBlankLines(t *testing.T) {
	m := newModel()
	tm, _ := m.Update(tea.WindowSizeMsg{Width: 94, Height: 40})
	m = tm.(model)
	long := strings.Repeat("a", 120)
	raw := strings.Join([]string{long, long, long, long, long}, "\n")
	tm, _ = m.Update(opDoneMsg{op: opRoots, res: runResult{
		title: "GC roots", summary: "5 roots", raw: raw, ok: true,
	}})
	m = tm.(model)
	m.expanded = true
	for i := 0; i < 90; i++ {
		tm, _ = m.Update(animTick{})
		m = tm.(model)
	}
	tm, _ = m.Update(animTick{})
	m = tm.(model)
	view := m.View()
	i1 := strings.Index(view, "aaaa")
	i5 := strings.LastIndex(view, "aaaa")
	if i1 < 0 {
		t.Fatalf("content missing from view")
	}
	seg := view[i1:i5]
	if strings.Contains(seg, "\n\n") {
		t.Errorf("blank line between wrapped content lines:\n%q", seg)
	}
}

// TestResultFitsTerminal guards against layout overflow: a frame taller
// than the terminal makes bubbletea scroll mid-render, which shows up as
// interleaved blank lines on the user's screen.
func TestResultFitsTerminal(t *testing.T) {
	sizes := [][2]int{{80, 24}, {70, 20}, {60, 16}, {100, 40}, {120, 50}}
	for _, sz := range sizes {
		for _, expand := range []bool{false, true} {
			m := newModel()
			tm, _ := m.Update(tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
			m = tm.(model)
			raw := strings.Repeat("/nix/store/xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx-env\n", 200)
			tm, _ = m.Update(opDoneMsg{op: opDead, res: runResult{
				title: "Dead store paths", summary: "200 paths", raw: raw, ok: true,
			}})
			m = tm.(model)
			m.expanded = expand
			for i := 0; i < 120; i++ {
				tm, _ = m.Update(animTick{})
				m = tm.(model)
			}
			view := m.View()
			got := strings.Count(view, "\n") + 1
			if got > sz[1] {
				t.Errorf("terminal %dx%d expand=%v: view has %d lines, terminal has %d",
					sz[0], sz[1], expand, got, sz[1])
			}
			if strings.Contains(view, "\n\n\n") {
				t.Errorf("terminal %dx%d expand=%v: interleaved blank lines", sz[0], sz[1], expand)
			}
		}
	}
}

func TestMenuSectionsHaveSpacing(t *testing.T) {
	m := newModel()
	m.selY = float64(m.rowOfItem(m.cursor))
	view := m.viewMenu(60)
	if !strings.Contains(view, "\n\nHome Manager") {
		t.Errorf("expected blank line before Home Manager section:\n%s", view)
	}
	if !strings.Contains(view, "\n\ndevenv") {
		t.Errorf("expected blank line before devenv section:\n%s", view)
	}
}

func TestStatusIsStandaloneTopMenuItem(t *testing.T) {
	m := newModel()
	if m.items[0].meta.op != opNone || m.items[0].section != "" {
		t.Errorf("first menu item must be standalone Status, got %+v", m.items[0])
	}
	if m.items[1].section != "Nix Store" {
		t.Errorf("second item should start Nix Store section, got %+v", m.items[1])
	}
	view := m.viewMenu(60)
	if strings.Contains(view, "\nNix Store\n") && !strings.Contains(view, "\n\nNix Store") {
		t.Errorf("expected blank line between Status and Nix Store header:\n%s", view)
	}
}

// TestRunOpDryRunNeverStartsCommands verifies the TUI dry-run guarantee:
// with dry-run enabled, EVERY operation — read-only ones included — shows
// a plan and starts no command.
func TestRunOpDryRunNeverStartsCommands(t *testing.T) {
	f := runner.NewFake()
	f.ScriptCmd("nix-store --gc --print-dead", "/nix/store/x\n")
	f.ScriptCmd("home-manager generations", "No 1  2026-01-01 00:00:00  /nix/store/g\n")
	m := newModel()
	m.ex = &operations.Executor{R: f, Opts: operations.Options{DryRun: true}}
	m.dryRun = true

	ops := []operation{opDead, opRoots, opLive, opGC, opGCOld, opOptimise,
		opDevenvGC, opDevenvUpdate, opDevenvInfo, opHMGenerations}

	for _, op := range ops {
		cmd := m.runOp(op)
		if cmd == nil {
			t.Fatalf("no cmd for op %d", op)
		}
		msg := cmd()
		done, ok := msg.(opDoneMsg)
		if !ok {
			t.Fatalf("op %d: expected opDoneMsg, got %T", op, msg)
		}
		if !done.res.dryRun {
			t.Errorf("op %d: result must be flagged dry-run, got %+v", op, done.res)
		}
		if strings.Contains(done.res.title, "DRY RUN") {
			t.Errorf("op %d: badge belongs in the view, not the title %q", op, done.res.title)
		}
		if !strings.Contains(done.res.summary, "Would execute") {
			t.Errorf("op %d: summary must show the plan, got %q", op, done.res.summary)
		}
	}
	if len(f.Calls) != 0 {
		t.Errorf("dry-run started commands: %v", f.Calls)
	}
}

// TestMenuSingleHighlightAfterMovement ensures there is always exactly one
// highlighted menu item: the selection bar must follow the cursor once the
// animation frames are pumped (regression: frozen selY highlighted both
// Status and the cursor item).
func TestMenuSingleHighlightAfterMovement(t *testing.T) {
	m := newModel()
	tm, _ := m.Update(tea.WindowSizeMsg{Width: 90, Height: 30})
	m = tm.(model)
	// Move down twice (Status -> dead -> Garbage collect).
	for i := 0; i < 2; i++ {
		tm, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = tm.(model)
	}
	// One animation frame, as the realtime loop would deliver.
	tm, _ = m.Update(animTick{})
	m = tm.(model)
	view := m.viewMenu(70)
	if got := strings.Count(view, "▌"); got != 1 {
		t.Fatalf("expected exactly 1 highlight, got %d:\n%s", got, view)
	}
	// The bar may still be gliding mid-flight after one frame.
	// Pump frames until the spring settles: the bar must land on the cursor.
	for i := 0; i < 120; i++ {
		tm, _ = m.Update(animTick{})
		m = tm.(model)
	}
	view = m.viewMenu(70)
	if got := strings.Count(view, "▌"); got != 1 {
		t.Errorf("after settle: expected 1 highlight, got %d", got)
	}
	for _, l := range strings.Split(view, "\n") {
		if strings.Contains(l, "▌") && !strings.Contains(l, "Garbage collect") {
			t.Errorf("after settle: highlight on wrong item: %q", l)
		}
	}
}

// TestMenuSingleHighlightOnFreshStart covers a fresh TUI launch: no result
// screen has ever run the animation chain, yet exactly one item highlights.
func TestMenuSingleHighlightOnFreshStart(t *testing.T) {
	m := newModel()
	view := m.viewMenu(70)
	if got := strings.Count(view, "▌"); got != 1 {
		t.Errorf("fresh start: expected 1 highlight (Status), got %d:\n%s", got, view)
	}
}
