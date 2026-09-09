// Package components holds reusable TUI widgets for tnxstor.
package components

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
)

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// OutputPanel is a scrollable, resizable output viewer (Bubbles viewport)
// used to display command output with scrolling and wrapping.
type OutputPanel struct {
	vp viewport.Model
}

// NewOutputPanel creates a panel with the given width.
func NewOutputPanel(width int) OutputPanel {
	vp := viewport.New(maxInt(20, width), 6)
	vp.SetContent("")
	return OutputPanel{vp: vp}
}

// SetContent replaces the panel contents.
func (o *OutputPanel) SetContent(s string) {
	o.vp.SetContent(strings.TrimRight(s, "\n"))
	o.vp.GotoTop()
}

// SetWidth resizes the panel.
func (o *OutputPanel) SetWidth(w int) {
	o.vp.Width = maxInt(20, w)
}

// SetHeight resizes the visible height.
func (o *OutputPanel) SetHeight(h int) {
	o.vp.Height = maxInt(3, h)
}

// LineUp scrolls up n lines.
func (o *OutputPanel) LineUp(n int) { o.vp.LineUp(n) }

// LineDown scrolls down n lines.
func (o *OutputPanel) LineDown(n int) { o.vp.LineDown(n) }

// HalfPageUp scrolls up half a page.
func (o *OutputPanel) HalfPageUp() { o.vp.HalfPageUp() }

// HalfPageDown scrolls down half a page.
func (o *OutputPanel) HalfPageDown() { o.vp.HalfPageDown() }

// View renders the panel.
func (o OutputPanel) View() string { return o.vp.View() }
