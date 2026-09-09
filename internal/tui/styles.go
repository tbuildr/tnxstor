package tui

import "github.com/charmbracelet/lipgloss"

// Palette: restrained, terminal-friendly colours with a blue accent.
var (
	colAccent = lipgloss.Color("4") // blue
	colOk     = lipgloss.Color("2") // green
	colWarn   = lipgloss.Color("3") // yellow
	colDanger = lipgloss.Color("1") // red
	colDim    = lipgloss.Color("8")
	colText   = lipgloss.Color("15")
	colBar    = lipgloss.Color("6") // cyan highlight bar
)

var (
	styleFrame = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colAccent)

	styleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(colText)

	styleBrand = lipgloss.NewStyle().
			Bold(true).
			Foreground(colWarn)

	styleVersion = lipgloss.NewStyle().
			Foreground(colDim)

	styleDryRun = lipgloss.NewStyle().
			Bold(true).
			Foreground(colWarn).
			Background(lipgloss.Color("8")).
			Padding(0, 1)

	styleSection = lipgloss.NewStyle().
			Bold(true).
			Foreground(colAccent)

	styleDivider = lipgloss.NewStyle().
			Foreground(colDim)

	styleItem = lipgloss.NewStyle().
			PaddingLeft(2).
			Foreground(colText)

	styleItemSelected = lipgloss.NewStyle().
				Bold(true).
				PaddingLeft(2).
				Foreground(colText)

	styleBar = lipgloss.NewStyle().
			Bold(true).
			Foreground(colBar)

	styleDesc = lipgloss.NewStyle().
			Foreground(colDim)

	styleFooter = lipgloss.NewStyle().
			Foreground(colDim)

	styleKey = lipgloss.NewStyle().
			Bold(true).
			Foreground(colAccent)

	styleCheck = lipgloss.NewStyle().
			Bold(true).
			Foreground(colOk)

	styleErr = lipgloss.NewStyle().
			Bold(true).
			Foreground(colDanger)

	styleOk = lipgloss.NewStyle().
		Bold(true).
		Foreground(colOk)

	styleWarn = lipgloss.NewStyle().
			Bold(true).
			Foreground(colWarn)

	styleDim = lipgloss.NewStyle().
			Foreground(colDim)

	styleCode = lipgloss.NewStyle().
			Foreground(colAccent)

	// Boxes
	styleSummaryBox = lipgloss.NewStyle().
			Border(lipgloss.ThickBorder(), false, false, false, true).
			BorderForeground(colOk).
			Padding(0, 1)

	styleSummaryBoxErr = styleSummaryBox.BorderForeground(colDanger)

	styleSummaryBoxDry = styleSummaryBox.BorderForeground(colWarn)

	styleCommandBox = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(colAccent).
			Padding(0, 1).
			MarginTop(1)

	styleWarnBox = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(colWarn).
			Padding(0, 1)

	styleOutputBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colDim).
			Padding(0, 1)
)
