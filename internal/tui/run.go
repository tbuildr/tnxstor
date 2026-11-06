package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Run starts the interactive TUI. It should only be called when an
// interactive terminal is available.
func Run() error {
	p := tea.NewProgram(newModel(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
