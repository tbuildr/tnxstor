package tui

import (
	"context"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/harmonica"
	"github.com/charmbracelet/lipgloss"

	"tnxstor/internal/operations"
	"tnxstor/internal/runner"
	"tnxstor/internal/tui/components"
)

type screen int

const (
	screenMenu screen = iota
	screenStatus
	screenConfirm
	screenRunning
	screenResult
	screenGenerations
	screenCleanup
	screenHelp
)

// operation identifies which operation a confirm/run/result screen is about.
type operation int

const (
	opNone operation = iota
	opDead
	opRoots
	opLive
	opGC
	opGCOld
	opOptimise
	opDevenvGC
	opDevenvUpdate
	opDevenvInfo
	opHMGenerations
	opHMExpire
	opClean
)

// opMeta describes an operation for menus and confirmation screens.
type opMeta struct {
	op          operation
	title       string
	desc        string
	command     string
	destructive bool
	warning     string
}

var menuNix = []opMeta{
	{opNone, "Status", "Nix / Home Manager / devenv overview", "", false, ""},
	{opDead, "Preview dead paths", "List store paths Nix considers unreachable (read-only)", "nix-store --gc --print-dead", false, ""},
	{opGC, "Garbage collect", "Remove store paths no longer reachable from a GC root", "nix-store --gc", true, ""},
	{opGCOld, "Aggressive GC", "Remove old profile generations, then garbage collect", "nix-collect-garbage -d", true,
		"This removes old Nix profile generations.\nYou may lose rollback points."},
	{opOptimise, "Optimise store", "Deduplicate identical files in the store via hard links", "nix-store --optimise", true, ""},
	{opRoots, "Show GC roots", "List the roots keeping store paths alive (read-only)", "nix-store --gc --print-roots", false, ""},
	{opLive, "Show live paths", "List store paths reachable from GC roots (read-only)", "nix-store --gc --print-live", false, ""},
}

var menuHM = []opMeta{
	{opHMGenerations, "List generations", "Show Home Manager generations (read-only)", "home-manager generations", false, ""},
	{opHMExpire, "Expire old generations", "Expire Home Manager generations older than 30 days", `home-manager expire-generations "-30 days"`, true, ""},
}

var menuDevenv = []opMeta{
	{opDevenvGC, "Garbage collect", "Clean obsolete devenv GC roots, then Nix GC", "devenv gc", true, ""},
	{opDevenvUpdate, "Update environment", "Refresh devenv inputs and lock files (current project)", "devenv update", true, ""},
	{opDevenvInfo, "Environment info", "Show information about the current devenv project (read-only)", "devenv info", false, ""},
}

type menuItem struct {
	meta    opMeta
	section string
}

// allMenuItems builds the menu: Status stands alone at the top, outside
// the Nix Store group.
func allMenuItems() []menuItem {
	var items []menuItem
	items = append(items, menuItem{menuNix[0], ""}) // Status, standalone
	for _, m := range menuNix[1:] {
		items = append(items, menuItem{m, "Nix Store"})
	}
	for _, m := range menuHM {
		items = append(items, menuItem{m, "Home Manager"})
	}
	for _, m := range menuDevenv {
		items = append(items, menuItem{m, "devenv"})
	}
	return items
}

// runResult carries everything a result screen needs.
type runResult struct {
	title   string
	summary string // parsed, human-readable summary (may be multi-line)
	raw     string // raw command output for the viewport
	ok      bool
	dryRun  bool // true when the outcome is a dry-run plan
	err     error
}

type model struct {
	ex             *operations.Executor
	opCtx          context.Context
	opCancel       context.CancelFunc
	items          []menuItem
	cursor         int
	screen         screen
	prev           screen
	dryRun         bool
	width          int
	height         int
	err            error
	status         *operations.StatusInfo
	op             operation
	opMeta         opMeta
	hmExpirePeriod string
	// confirmation
	confirmIdx  int // 0 = yes, 1 = no
	confirmCmd  string
	confirmNote string
	// running
	spin    spinner.Model
	running string
	result  *runResult
	panel   components.OutputPanel
	// cleanup workflow
	cleanChecks map[string]bool
	cleanCursor int
	// animation
	selY, selV  float64
	spring      harmonica.Spring
	panelH      float64
	panelV      float64
	panelSpring harmonica.Spring
	expanded    bool
}

func newModel() model {
	r := runner.NewExec()
	ex := &operations.Executor{R: r}
	sp := spinner.New(spinner.WithSpinner(spinner.Line))
	sp.Style = lipgloss.NewStyle().Foreground(colAccent)
	clean := map[string]bool{
		"devenv-gc": true,
		"hm-expire": true,
		"gc":        true,
		"gc-old":    false,
		"optimise":  false,
	}
	return model{
		ex:             ex,
		opCtx:          context.Background(),
		items:          allMenuItems(),
		dryRun:         false,
		spin:           sp,
		panel:          components.NewOutputPanel(60),
		cleanChecks:    clean,
		spring:         harmonica.NewSpring(harmonica.FPS(60), 220.0, 22.0),
		panelSpring:    harmonica.NewSpring(harmonica.FPS(60), 180.0, 20.0),
		hmExpirePeriod: "30d",
	}
}

// Version is shown in the TUI header.
const Version = "0.1.0"

// Init starts the status dashboard load and the animation tick loop.
func (m model) Init() tea.Cmd {
	return tea.Batch(m.loadStatus(), animTickCmd())
}
