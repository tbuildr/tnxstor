package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"tnxstor/internal/operations"
	"tnxstor/internal/util"
)

// clamp truncates s to n display columns with an ellipsis.
func clamp(s string, n int) string {
	if n <= 0 {
		return ""
	}
	w := 0
	var b strings.Builder
	for _, r := range s {
		rw := 1
		if r > 0x1100 { // rough wide-char guard; good enough for our labels
			rw = 1
		}
		if w+rw > n-1 {
			b.WriteRune('…')
			return b.String()
		}
		b.WriteRune(r)
		w += rw
	}
	return b.String()
}

// View renders the current screen.
func (m model) View() string {
	width := 70
	if m.width > 12 {
		width = m.width - 4
	}
	height := 32
	if m.height > 14 {
		height = m.height - 2
	}

	var body string
	switch m.screen {
	case screenStatus:
		body = m.viewStatus()
	case screenConfirm:
		body = m.viewConfirm()
	case screenRunning:
		body = m.viewRunning()
	case screenResult:
		body = m.viewResult()
	case screenCleanup:
		body = m.viewCleanup()
	case screenHelp:
		body = m.viewHelp()
	default:
		body = m.viewMenu(width - 6)
	}

	header := m.viewHeader(width)
	footer := m.viewFooter(width)

	content := lipgloss.JoinVertical(lipgloss.Left, header, "", body, "", footer)
	// Height leaves one terminal row free: a frame that exactly fills the
	// screen can push the cursor past the bottom row during repaints, which
	// scrolls the screen and shows up as interleaved blank lines.
	box := styleFrame.
		Width(width).
		Height(height - 1).
		MaxHeight(m.height - 1).
		Render(content)
	return box
}

// contextLabel names the current screen in the header's top-right corner.
// On the menu it follows the section the cursor is in.
func (m model) contextLabel() string {
	switch m.screen {
	case screenMenu:
		if m.cursor >= 0 && m.cursor < len(m.items) {
			if s := m.items[m.cursor].section; s != "" {
				return s
			}
			return "Overview"
		}
		return "Overview"
	case screenStatus:
		return "Status"
	case screenConfirm:
		return "Confirm"
	case screenRunning:
		return "Running"
	case screenResult:
		return "Result"
	case screenCleanup:
		return "Clean up"
	case screenHelp:
		return "Help"
	}
	return "Overview"
}

func (m model) viewHeader(width int) string {
	title := styleBrand.Render("⚡ tnxstor") + " " + styleVersion.Render("v"+Version)
	context := styleDim.Render(m.contextLabel())
	if m.dryRun {
		context = styleDryRun.Render("[DRY RUN]") + " " + context
	}
	gap := width - 2 - lipgloss.Width(title) - lipgloss.Width(context)
	if gap < 1 {
		gap = 1
	}
	line := title + strings.Repeat(" ", gap) + context
	divider := styleDivider.Render("─" + strings.Repeat("─", maxInt(1, width-4)) + "─")
	return line + "\n" + divider
}

func (m model) viewFooter(width int) string {
	var pairs [][2]string
	switch m.screen {
	case screenResult:
		pairs = [][2]string{{"↑↓", "scroll"}, {"Enter", expandLabel(m)}, {"Esc", "back"}, {"q", "quit"}}
	case screenRunning:
		pairs = [][2]string{{"Esc", "cancel"}, {"q", "quit"}}
	case screenConfirm:
		pairs = [][2]string{{"↑↓", "choose"}, {"y", "confirm"}, {"n/Esc", "cancel"}}
	case screenCleanup:
		pairs = [][2]string{{"Space", "toggle"}, {"Enter", "run"}, {"d", "dry-run"}, {"Esc", "back"}}
	case screenHelp:
		pairs = [][2]string{{"Esc", "back"}, {"q", "quit"}}
	case screenStatus:
		pairs = [][2]string{{"r", "refresh"}, {"Esc", "back"}, {"q", "quit"}}
	default:
		pairs = [][2]string{{"↑↓", "navigate"}, {"Enter", "select"}, {"d", "dry-run"}, {"r", "refresh"}, {"?", "help"}, {"q", "quit"}}
	}
	var parts []string
	for _, p := range pairs {
		parts = append(parts, styleKey.Render(p[0])+" "+styleFooter.Render(p[1]))
	}
	return strings.Join(parts, "   ")
}

func (m model) viewMenu(width int) string {
	rows := m.itemRows()
	selectedRow := int(m.selY + 0.5)
	var b strings.Builder
	lastSection := ""
	for i, item := range m.items {
		if item.section != lastSection {
			if item.section != "" {
				if strings.TrimSpace(b.String()) != "" {
					b.WriteString("\n")
				}
				b.WriteString(styleSection.Render(item.section) + "\n")
				b.WriteString(styleDivider.Render(strings.Repeat("─", maxInt(4, width/2))) + "\n")
				lastSection = item.section
			}
		}
		// Exactly one highlight, at the animated bar position. Settled it
		// coincides with the cursor; while moving it glides to the target.
		if rows[i] == selectedRow {
			line := styleBar.Render("▌") + styleItemSelected.Render(item.meta.title)
			if d := strings.TrimSpace(item.meta.desc); d != "" {
				line += "  " + styleDesc.Render(clamp("— "+d, width))
			}
			b.WriteString(line + "\n")
		} else {
			b.WriteString(styleItem.Render(item.meta.title) + "\n")
		}
	}
	if m.err != nil {
		b.WriteString("\n" + styleErr.Render("✗ "+m.err.Error()) + "\n")
	}
	return b.String()
}

func (m model) viewStatus() string {
	if m.status == nil {
		return styleDim.Render("Loading status…") + "\n"
	}
	st := m.status
	yn := func(v bool) string {
		if v {
			return styleOk.Render("✓ yes")
		}
		return styleDim.Render("✗ no")
	}
	section := func(title string, rows [][2]string) string {
		var b strings.Builder
		b.WriteString(styleSection.Render(title) + "\n")
		b.WriteString(styleDivider.Render("─────────────────────") + "\n")
		for _, r := range rows {
			b.WriteString(fmt.Sprintf("%-14s %s\n", styleDim.Render(r[0]), r[1]))
		}
		b.WriteString("\n")
		return b.String()
	}
	var b strings.Builder
	b.WriteString(section("Nix", [][2]string{
		{"Version", orDashT(st.NixVersion)},
		{"Store", st.StorePath},
		{"Store size", storeSizeStr(st)},
		{"Daemon", yn(st.Daemon) + " " + styleDim.Render(st.DaemonNote)},
		{"NIX_REMOTE", orDashT(st.NixRemote)},
	}))
	b.WriteString(section("Host", [][2]string{
		{"OS", orDashT(st.OS.OSPretty)},
	}))
	hmRows := [][2]string{{"Installed", yn(st.Deps.HomeManager)}}
	if st.Deps.HomeManager && st.HMHasData {
		hmRows = append(hmRows,
			[2]string{"Generation", strconv.Itoa(st.HMCurrent)},
			[2]string{"Generations", strconv.Itoa(st.HMCount)})
	}
	b.WriteString(section("Home Manager", hmRows))
	b.WriteString(section("devenv", [][2]string{
		{"Installed", yn(st.Deps.Devenv)},
		{"GC roots", yn(st.DevenvRoot)},
	}))
	if m.err != nil {
		b.WriteString(styleErr.Render("✗ "+m.err.Error()) + "\n")
	}
	return b.String()
}

func storeSizeStr(st *operations.StatusInfo) string {
	if st.StoreKnown {
		return styleOk.Render(util.Bytes(st.StoreSize))
	}
	return styleDim.Render("-")
}

func orDashT(v string) string {
	if strings.TrimSpace(v) == "" {
		return "-"
	}
	return v
}

func (m model) viewConfirm() string {
	var b strings.Builder
	b.WriteString(styleHeader.Render(m.opMeta.title) + "\n\n")
	b.WriteString(styleItem.Render(m.opMeta.desc) + "\n")
	if m.confirmNote != "" {
		var warn strings.Builder
		for _, line := range strings.Split(m.confirmNote, "\n") {
			warn.WriteString(styleWarn.Render("⚠ "+line) + "\n")
		}
		b.WriteString("\n" + styleWarnBox.Render(strings.TrimRight(warn.String(), "\n")) + "\n")
	}
	if m.dryRun {
		b.WriteString("\n" + styleDryRun.Render("[DRY RUN]") + styleDim.Render("  Nothing will be executed.") + "\n")
	}
	b.WriteString(styleCommandBox.Render(styleDim.Render("command")+"\n"+styleCode.Render(m.confirmCmd)) + "\n\n")
	yes := styleItem.Render("Yes")
	no := styleItem.Render("No")
	if m.confirmIdx == 0 {
		yes = styleBar.Render("▌") + styleItemSelected.Render("Yes")
	} else {
		no = styleBar.Render("▌") + styleItemSelected.Render("No")
	}
	b.WriteString(yes + "\n" + no + "\n")
	return b.String()
}

func (m model) viewRunning() string {
	var b strings.Builder
	b.WriteString(styleHeader.Render(m.running) + "\n\n")
	if m.dryRun {
		b.WriteString(styleDryRun.Render("[DRY RUN]") + "\n\n")
	}
	b.WriteString("  " + m.spin.View() + " working…" + "\n\n")
	b.WriteString(styleDim.Render("Press Esc to cancel") + "\n")
	return b.String()
}

func (m model) viewResult() string {
	if m.result == nil {
		return ""
	}
	r := m.result
	var b strings.Builder
	switch {
	case r.err != nil:
		b.WriteString(styleErr.Render("✗ Operation failed") + "\n\n")
		b.WriteString(styleSummaryBoxErr.Render(styleErr.Render(r.err.Error())) + "\n")
	case r.dryRun:
		// Dry-run outcomes are presented like the [DRY RUN] header badge,
		// consistently for every task.
		b.WriteString(styleDryRun.Render("[DRY RUN]") + "  " + styleHeader.Render(r.title) + "\n\n")
		if strings.TrimSpace(r.summary) != "" {
			b.WriteString(styleSummaryBoxDry.Render(r.summary) + "\n")
		}
	default:
		b.WriteString(styleOk.Render("✓ "+r.title) + "\n\n")
		if strings.TrimSpace(r.summary) != "" {
			b.WriteString(styleSummaryBox.Render(r.summary) + "\n")
		}
	}
	m.panel.SetHeight(maxInt(3, int(m.panelH)))
	panel := m.panel.View()
	if m.expanded {
		panel = styleOutputBox.Render(panel)
	}
	lines := 0
	if r.raw != "" {
		lines = strings.Count(strings.TrimRight(r.raw, "\n"), "\n") + 1
	}
	b.WriteString("\n" + styleDim.Render(fmt.Sprintf("output · %d lines · Enter to %s", lines, expandLabel(m))) + "\n")
	b.WriteString(panel)
	return strings.TrimRight(b.String(), "\n")
}

func expandLabel(m model) string {
	if m.expanded {
		return "collapse"
	}
	return "expand"
}

var cleanTitles = map[string]string{
	"devenv-gc": "devenv garbage collection",
	"hm-expire": "Expire Home Manager generations older than 30 days",
	"gc":        "Nix garbage collection",
	"gc-old":    "Aggressive GC (nix-collect-garbage -d)",
	"optimise":  "Optimise Nix store",
}

func (m model) viewCleanup() string {
	var b strings.Builder
	b.WriteString(styleHeader.Render("Clean up") + "\n")
	b.WriteString(styleDesc.Render("Select the operations to run. Defaults are conservative.") + "\n\n")
	for i, id := range cleanupIDs() {
		box := styleDim.Render("[ ]")
		if m.cleanChecks[id] {
			box = styleCheck.Render("[x]")
		}
		line := box + " " + cleanTitles[id]
		if i == m.cleanCursor {
			b.WriteString(styleBar.Render("▌") + styleItemSelected.Render(line) + "\n")
		} else {
			b.WriteString(styleItem.Render(line) + "\n")
		}
	}
	b.WriteString("\n")
	steps := currentPlan(m).Steps()
	if len(steps) > 0 {
		b.WriteString(styleDim.Render("plan") + "\n")
		var pb strings.Builder
		for i, s := range steps {
			fmt.Fprintf(&pb, "%d. %s\n", i+1, s.Command)
		}
		b.WriteString(styleCommandBox.Render(strings.TrimRight(pb.String(), "\n")) + "\n")
	} else {
		b.WriteString(styleWarn.Render("⚠ No operations selected.") + "\n")
	}
	if m.dryRun {
		b.WriteString("\n" + styleDryRun.Render("[DRY RUN]") + "\n")
	}
	return b.String()
}

func currentPlan(m model) operations.CleanPlan {
	return operations.CleanPlan{
		DevenvGC:    m.cleanChecks["devenv-gc"],
		HMExpire:    m.cleanChecks["hm-expire"],
		HMOlderThan: m.hmExpirePeriod,
		GC:          m.cleanChecks["gc"],
		Aggressive:  m.cleanChecks["gc-old"],
		Optimise:    m.cleanChecks["optimise"],
	}
}

func (m model) viewHelp() string {
	var b strings.Builder
	b.WriteString(styleHeader.Render("Help") + "\n\n")
	rows := [][2]string{
		{"↑ / k", "Move up"},
		{"↓ / j", "Move down"},
		{"Enter", "Select / expand output"},
		{"Esc", "Back"},
		{"q", "Quit"},
		{"?", "Help"},
		{"r", "Refresh"},
		{"d", "Toggle dry-run"},
		{"y", "Confirm"},
		{"n", "Cancel"},
		{"c", "Clean up workflow (menu)"},
	}
	for _, r := range rows {
		b.WriteString(fmt.Sprintf("  %-8s %s\n", styleKey.Render(r[0]), styleItem.Render(r[1])))
	}
	b.WriteString("\n" + styleDim.Render("Destructive operations always ask for confirmation.") + "\n")
	b.WriteString(styleDim.Render("Nothing is ever deleted directly from /nix/store; Nix decides.") + "\n")
	return b.String()
}
