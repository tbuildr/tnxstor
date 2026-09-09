package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"tnxstor/internal/operations"
	"tnxstor/internal/util"
)

// animTick drives harmonica spring updates at 60fps.
type animTick struct{}

func animTickCmd() tea.Cmd {
	return tea.Tick(time.Second/60, func(time.Time) tea.Msg { return animTick{} })
}

type statusLoadedMsg struct {
	st  operations.StatusInfo
	err error
}

type opDoneMsg struct {
	res runResult
	op  operation
}

type opErrMsg struct {
	err error
	op  operation
}

// beginOp transitions to the running screen and starts the operation.
func beginOp(m model, op operation) (model, tea.Cmd) {
	m.op = op
	m.running = m.metaFor(op).title
	m.screen = screenRunning
	ctx, cancel := context.WithCancel(context.Background())
	m.opCtx, m.opCancel = ctx, cancel
	return m, tea.Batch(m.spin.Tick, m.runOp(op))
}

// dryRunResult renders a dry-run outcome: the plan is shown, the command is
// never started. Applied to every operation, read-only ones included. The
// title stays plain; the view adds the [DRY RUN] badge.
func dryRunResult(title string, p operations.Plan) runResult {
	return runResult{title: title, summary: p.String(), ok: true, dryRun: true}
}

func (m model) runOp(op operation) tea.Cmd {
	ctx := m.opCtx
	ex := m.ex
	switch op {
	case opDead:
		return func() tea.Msg {
			out, err := ex.Dead(ctx, 0)
			if err != nil {
				return opErrMsg{err, op}
			}
			if out.DryRun {
				return opDoneMsg{dryRunResult("Dead store paths preview", out.Plan), op}
			}
			var b strings.Builder
			fmt.Fprintf(&b, "%d paths\n", out.Count)
			if out.BytesKnown && out.TotalBytes > 0 {
				fmt.Fprintf(&b, "Estimated reclaimable: %s", util.Bytes(out.TotalBytes))
			}
			return opDoneMsg{runResult{title: "Dead store paths", summary: b.String(), raw: out.Raw, ok: true}, op}
		}
	case opRoots:
		return func() tea.Msg {
			out, err := ex.Roots(ctx)
			if err != nil {
				return opErrMsg{err, op}
			}
			if out.DryRun {
				return opDoneMsg{dryRunResult("GC roots listing", out.Plan), op}
			}
			var b strings.Builder
			for _, r := range out.Roots {
				fmt.Fprintf(&b, "%-12s %s -> %s\n", r.Class, r.Link, r.Target)
			}
			return opDoneMsg{runResult{title: "GC roots", summary: fmt.Sprintf("%d roots", len(out.Roots)), raw: b.String(), ok: true}, op}
		}
	case opLive:
		return func() tea.Msg {
			out, err := ex.Live(ctx)
			if err != nil {
				return opErrMsg{err, op}
			}
			if out.DryRun {
				return opDoneMsg{dryRunResult("Live store paths listing", out.Plan), op}
			}
			return opDoneMsg{runResult{title: "Live store paths", summary: "Live paths as reported by Nix", raw: out.Raw, ok: true}, op}
		}
	case opGC:
		return func() tea.Msg {
			out, err := ex.GC(ctx)
			if err != nil {
				return opErrMsg{err, op}
			}
			if out.DryRun {
				return opDoneMsg{dryRunResult("Garbage collection", out.Plan), op}
			}
			sum := "Complete."
			if out.FreedKnown {
				sum = fmt.Sprintf("Freed: %s", util.Bytes(out.FreedBytes))
			}
			return opDoneMsg{runResult{title: "Garbage Collection Complete", summary: sum, raw: out.Raw, ok: true}, op}
		}
	case opGCOld:
		return func() tea.Msg {
			out, err := ex.GCOld(ctx)
			if err != nil {
				return opErrMsg{err, op}
			}
			if out.DryRun {
				return opDoneMsg{dryRunResult("Aggressive GC", out.Plan), op}
			}
			sum := "Complete."
			if out.FreedKnown {
				sum = fmt.Sprintf("Freed: %s", util.Bytes(out.FreedBytes))
			}
			return opDoneMsg{runResult{title: "Aggressive GC Complete", summary: sum, raw: out.Raw, ok: true}, op}
		}
	case opOptimise:
		return func() tea.Msg {
			out, err := ex.Optimise(ctx)
			if err != nil {
				return opErrMsg{err, op}
			}
			if out.DryRun {
				return opDoneMsg{dryRunResult("Nix store optimisation", out.Plan), op}
			}
			var sum string
			if out.Known {
				sum = fmt.Sprintf("Before: %s\nAfter:  %s\nSaved:  %s",
					util.Bytes(out.Before), util.Bytes(out.After), util.Bytes(out.Saved))
			} else {
				sum = "Complete."
			}
			return opDoneMsg{runResult{title: "Store Optimisation Complete", summary: sum, raw: out.Raw, ok: true}, op}
		}
	case opDevenvGC:
		return func() tea.Msg {
			out, err := ex.DevenvGC(ctx)
			if err != nil {
				return opErrMsg{err, op}
			}
			if out.DryRun {
				return opDoneMsg{dryRunResult("devenv garbage collection", out.Plan), op}
			}
			return opDoneMsg{runResult{title: "devenv GC Complete", summary: "Complete.", raw: out.Raw, ok: true}, op}
		}
	case opDevenvUpdate:
		return func() tea.Msg {
			out, err := ex.DevenvUpdate(ctx)
			if err != nil {
				return opErrMsg{err, op}
			}
			if out.DryRun {
				return opDoneMsg{dryRunResult("devenv environment update", out.Plan), op}
			}
			return opDoneMsg{runResult{title: "devenv Update Complete", summary: "Inputs and lock files refreshed.", raw: out.Raw, ok: true}, op}
		}
	case opDevenvInfo:
		return func() tea.Msg {
			out, err := ex.DevenvInfo(ctx)
			if err != nil {
				return opErrMsg{err, op}
			}
			if out.DryRun {
				return opDoneMsg{dryRunResult("devenv environment info", out.Plan), op}
			}
			return opDoneMsg{runResult{title: "devenv Environment Info", summary: "Current project information", raw: out.Raw, ok: true}, op}
		}
	case opHMGenerations:
		return func() tea.Msg {
			out, err := ex.HMGenerations(ctx)
			if err != nil {
				return opErrMsg{err, op}
			}
			if out.DryRun {
				return opDoneMsg{dryRunResult("Home Manager generations listing", out.Plan), op}
			}
			var b strings.Builder
			fmt.Fprintf(&b, "CURRENT  DATE                 GENERATION  PATH\n")
			for _, g := range out.Generations {
				mark := "  "
				if g.Current {
					mark = "✓ "
				}
				fmt.Fprintf(&b, "%s%-20s %-10d %s\n", mark, g.Date.Format("2006-01-02 15:04"), g.Number, g.Path)
			}
			return opDoneMsg{runResult{title: "Home Manager generations", summary: fmt.Sprintf("%d generations", len(out.Generations)), raw: b.String(), ok: true}, op}
		}
	case opHMExpire:
		period := m.hmExpirePeriod
		return func() tea.Msg {
			out, err := ex.HMExpire(ctx, period)
			if err != nil {
				return opErrMsg{err, op}
			}
			if out.DryRun {
				var b strings.Builder
				fmt.Fprintf(&b, "Would expire generations older than %s.\n", util.HumanPeriod(period))
				for _, g := range out.Eligible {
					fmt.Fprintf(&b, "  %d\n", g.Number)
				}
				b.WriteString("\n" + out.Plan.String())
				return opDoneMsg{runResult{title: "Expire Home Manager generations",
					summary: b.String(), raw: out.Raw, ok: true, dryRun: true}, op}
			}
			return opDoneMsg{runResult{title: "Home Manager expiry Complete", summary: "Complete.", raw: out.Raw, ok: true}, op}
		}
	case opClean:
		return func() tea.Msg {
			plan := operations.CleanPlan{
				DevenvGC:    m.cleanChecks["devenv-gc"],
				HMExpire:    m.cleanChecks["hm-expire"],
				HMOlderThan: m.hmExpirePeriod,
				GC:          m.cleanChecks["gc"],
				Aggressive:  m.cleanChecks["gc-old"],
				Optimise:    m.cleanChecks["optimise"],
			}
			out, err := ex.Clean(ctx, plan)
			if err != nil {
				return opErrMsg{err, op}
			}
			if out.DryRun {
				return opDoneMsg{dryRunResult("Cleanup plan", out.Plan), op}
			}
			var raw strings.Builder
			for _, sr := range out.Steps {
				fmt.Fprintf(&raw, "=== %s (%s)\n%s\n", sr.Step.Title, sr.Step.Command, sr.Raw)
			}
			return opDoneMsg{runResult{title: "Cleanup Complete", summary: fmt.Sprintf("%d steps executed", len(out.Steps)), raw: raw.String(), ok: true}, op}
		}
	}
	return nil
}

// loadStatus fetches the status dashboard data through the executor.
func (m model) loadStatus() tea.Cmd {
	ex := m.ex
	return func() tea.Msg {
		st, err := ex.Status(context.Background())
		return statusLoadedMsg{st, err}
	}
}

func (m model) metaFor(op operation) opMeta {
	for _, it := range m.items {
		if it.meta.op == op {
			return it.meta
		}
	}
	switch op {
	case opClean:
		return opMeta{opClean, "Clean up", "Guided multi-step cleanup", "", true, ""}
	case opHMExpire:
		return opMeta{opHMExpire, "Expire old generations", "", `home-manager expire-generations "-30 days"`, true, ""}
	}
	return opMeta{}
}

// tickAnim advances the harmonica springs one frame. The menu selection bar
// glides in row space; the result panel expands/collapses with its own spring.
func (m *model) tickAnim() tea.Cmd {
	m.selY, m.selV = m.spring.Update(m.selY, m.selV, float64(m.rowOfItem(m.cursor)))
	target := minInt(6, m.panelMax())
	if m.expanded {
		target = m.panelMax()
	}
	m.panelH, m.panelV = m.panelSpring.Update(m.panelH, m.panelV, float64(target))
	m.panel.SetHeight(int(m.panelH + 0.5))
	return animTickCmd()
}

// panelMax is the largest viewport height (in content rows) that still keeps
// the whole result screen inside the terminal. It reserves room for the
// header, title, summary box, output label, footer and, when expanded, the
// output box border. Overflowing the terminal causes scroll artifacts
// (apparent blank lines), so this is a hard layout budget.
func (m model) panelMax() int {
	k := 0
	if m.result != nil && strings.TrimSpace(m.result.summary) != "" {
		k = strings.Count(m.result.summary, "\n") + 1
	}
	if m.result != nil && m.result.err != nil {
		k++ // error box line
	}
	h := m.height - 13 - k
	if h < 3 {
		h = 3
	}
	return h
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// itemRows maps each menu item to its display row. Status sits alone at the
// top with no section header; a new named section adds three rows: a blank
// spacer, the section header and its divider. It is shared by the menu view
// and the selection animation so they stay in sync.
func (m model) itemRows() []int {
	rows := make([]int, len(m.items))
	disp := 0
	last := ""
	for i, it := range m.items {
		if it.section != last {
			if i > 0 {
				disp++ // blank spacer before sections
			}
			disp += 2 // section header + divider
			last = it.section
		}
		rows[i] = disp
		disp++
	}
	return rows
}

func (m model) rowOfItem(i int) int {
	rows := m.itemRows()
	if i < 0 || i >= len(rows) {
		return 0
	}
	return rows[i]
}
