package tui

import (
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

// Update handles all messages.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// Output panel width keeps room for the frame (2) plus the output
		// box border/padding (4) so no rendered line ever exceeds the
		// terminal and gets soft-wrapped.
		m.panel.SetWidth(m.width - 10)
		return m, nil

	case animTick:
		// Always keep the animation chain alive; it pauses rendering work
		// on screens that do not need it but must never die, otherwise the
		// selection bar freezes and the menu highlights two items.
		if m.screen == screenMenu || m.screen == screenResult {
			return m, m.tickAnim()
		}
		return m, animTickCmd()

	case spinner.TickMsg:
		if m.screen == screenRunning {
			var cmd tea.Cmd
			m.spin, cmd = m.spin.Update(msg)
			return m, cmd
		}
		return m, nil

	case statusLoadedMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.err = nil
			st := msg.st
			m.status = &st
		}
		return m, nil

	case opDoneMsg:
		if m.opCancel != nil {
			m.opCancel()
			m.opCancel = nil
		}
		m.screen = screenResult
		m.prev = screenMenu
		m.expanded = false
		m.panelH, m.panelV = 6, 0
		res := msg.res
		m.result = &res
		m.panel.SetContent(res.raw)
		return m, animTickCmd()

	case opErrMsg:
		if m.opCancel != nil {
			m.opCancel()
			m.opCancel = nil
		}
		m.screen = screenResult
		m.expanded = false
		m.panelH, m.panelV = 6, 0
		m.result = &runResult{title: "Operation failed", err: msg.err, raw: ""}
		return m, animTickCmd()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	// Global keys: quit from any screen.
	switch key {
	case "ctrl+c":
		if m.opCancel != nil {
			m.opCancel()
		}
		return m, tea.Quit
	case "q":
		if m.opCancel != nil {
			m.opCancel()
		}
		return m, tea.Quit
	}
	switch m.screen {
	case screenMenu:
		return m.menuKey(key)
	case screenStatus:
		switch key {
		case "esc", "backspace":
			m.screen = screenMenu
			return m, nil
		case "r":
			return m, m.loadStatus()
		}
	case screenGenerations, screenResult:
		return m.resultKey(key)
	case screenConfirm:
		return m.confirmKey(key)
	case screenRunning:
		switch key {
		case "esc":
			if m.opCancel != nil {
				m.opCancel()
			}
			m.screen = screenMenu
			m.opCancel = nil
			return m, nil
		}
	case screenCleanup:
		return m.cleanupKey(key)
	case screenHelp:
		switch key {
		case "esc", "?", "backspace":
			m.screen = m.prev
			if m.screen == screenHelp {
				m.screen = screenMenu
			}
			return m, nil
		}
	}
	return m, nil
}

func (m model) menuKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
	case "g":
		m.cursor = 0
	case "G":
		m.cursor = len(m.items) - 1
	case "d":
		m.dryRun = !m.dryRun
	case "?":
		m.prev = screenMenu
		m.screen = screenHelp
	case "r":
		return m, m.loadStatus()
	case "c":
		m.screen = screenCleanup
		m.cleanCursor = 0
	case "enter":
		item := m.items[m.cursor]
		return m.selectOperation(item.meta)
	}
	return m, nil
}

// selectOperation routes to confirmation (destructive) or immediate run.
// With dry-run enabled the confirmation screen is skipped: nothing will
// execute anyway and the plan is presented in the same uniform dry-run
// result style as every other task.
func (m model) selectOperation(meta opMeta) (tea.Model, tea.Cmd) {
	m.ex.Opts.DryRun = m.dryRun
	switch meta.op {
	case opNone:
		m.screen = screenStatus
		return m, m.loadStatus()
	case opDead, opRoots, opLive, opHMGenerations, opDevenvInfo, opDevenvUpdate:
		return beginOp(m, meta.op)
	case opHMExpire:
		if m.dryRun {
			return beginOp(m, opHMExpire)
		}
		m.opMeta = meta
		m.confirmCmd = `home-manager expire-generations "` + "-30 days" + `"`
		m.confirmNote = ""
		m.confirmIdx = 1
		m.screen = screenConfirm
		return m, nil
	case opClean:
		m.screen = screenCleanup
		m.cleanCursor = 0
		return m, nil
	}
	if m.dryRun {
		return beginOp(m, meta.op)
	}
	m.opMeta = meta
	m.confirmCmd = meta.command
	m.confirmNote = meta.warning
	m.confirmIdx = 1
	m.screen = screenConfirm
	return m, nil
}

func (m model) confirmKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "n":
		m.screen = screenMenu
		return m, nil
	case "up", "k", "down", "j", "tab", "left", "right":
		m.confirmIdx = 1 - m.confirmIdx
	case "y":
		m.confirmIdx = 0
		return m.runConfirmed()
	case "enter":
		return m.runConfirmed()
	}
	return m, nil
}

func (m model) runConfirmed() (tea.Model, tea.Cmd) {
	if m.confirmIdx != 0 {
		m.screen = screenMenu
		return m, nil
	}
	// Sync the current dry-run toggle into the executor; in dry-run the
	// operations layer returns a plan instead of executing.
	m.ex.Opts.DryRun = m.dryRun
	if m.opMeta.op == opHMExpire {
		return beginOp(m, opHMExpire)
	}
	return beginOp(m, m.opMeta.op)
}

func (m model) resultKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "backspace":
		m.screen = screenMenu
		m.result = nil
		return m, nil
	case "enter":
		// Toggle expandable output panel.
		m.expanded = !m.expanded
		return m, animTickCmd()
	case "up", "k":
		m.panel.LineUp(1)
	case "down", "j":
		m.panel.LineDown(1)
	case "pgup":
		m.panel.HalfPageUp()
	case "pgdown":
		m.panel.HalfPageDown()
	}
	return m, nil
}

func (m model) cleanupKey(key string) (tea.Model, tea.Cmd) {
	ids := cleanupIDs()
	switch key {
	case "esc":
		m.screen = screenMenu
		return m, nil
	case "up", "k":
		if m.cleanCursor > 0 {
			m.cleanCursor--
		}
	case "down", "j":
		if m.cleanCursor < len(ids)-1 {
			m.cleanCursor++
		}
	case " ":
		id := ids[m.cleanCursor]
		m.cleanChecks[id] = !m.cleanChecks[id]
	case "d":
		m.dryRun = !m.dryRun
	case "enter":
		m.ex.Opts.DryRun = m.dryRun
		return beginOp(m, opClean)
	}
	return m, nil
}

func cleanupIDs() []string {
	return []string{"devenv-gc", "hm-expire", "gc", "gc-old", "optimise"}
}

var _ spinner.Model
