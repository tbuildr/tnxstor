// Package operations is the single internal operation layer shared by the
// CLI and the TUI. The TUI never executes shell commands directly and the
// CLI contains no Nix-specific implementation details.
//
// Dry-run is a higher-level application concern: read-only discovery
// commands still execute (so eligible generations can be listed), while
// destructive commands are never started. Nothing is faked in dry-run mode.
package operations

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"tnxstor/internal/devenv"
	"tnxstor/internal/homemanager"
	"tnxstor/internal/nix"
	"tnxstor/internal/runner"
	"tnxstor/internal/util"
)

// Sentinel errors mapped to documented exit codes by the CLI.
var (
	// ErrMissingDependency indicates a required tool is unavailable (exit 3).
	ErrMissingDependency = errors.New("required dependency missing")
	// ErrCancelled indicates the user declined confirmation (exit 4).
	ErrCancelled = errors.New("user cancelled")
)

// Options carries global options shared by CLI and TUI.
type Options struct {
	DryRun  bool
	Yes     bool
	Verbose bool
	Quiet   bool
	NoColor bool
}

// Executor runs operations through a runner.
type Executor struct {
	R    runner.Runner
	Opts Options
	// Confirm is used by interactive frontends before destructive actions.
	// When nil, confirmations must have been handled by the caller.
	Confirm func(title, command string) bool
}

// Verbosef logs a debug line when verbose mode is enabled.
func (e *Executor) Verbosef(format string, args ...any) {
	if e.Opts.Verbose && !e.Opts.Quiet {
		fmt.Printf("DEBUG "+format+"\n", args...)
	}
}

// Plan describes a destructive command that would be executed.
type Plan struct {
	Title    string
	Commands []string
	Notes    []string
}

func (p Plan) String() string {
	var b strings.Builder
	b.WriteString("Would execute:\n")
	for _, c := range p.Commands {
		b.WriteString("\n  " + c + "\n")
	}
	for _, n := range p.Notes {
		b.WriteString("\n" + n + "\n")
	}
	b.WriteString("\nNo changes have been made.")
	return b.String()
}

func require(r runner.Runner, tools ...string) error {
	for _, t := range tools {
		if _, ok := r.LookPath(t); !ok {
			return fmt.Errorf("%w: %s is not installed or not available in PATH", ErrMissingDependency, t)
		}
	}
	return nil
}

// dryRun builds a Plan without executing anything.
func dryRun(title string, cmds ...string) Plan {
	return Plan{Title: title, Commands: cmds}
}

// ---- Nix GC ------------------------------------------------------------

// GCOutcome is the result of a garbage collection.
type GCOutcome struct {
	DryRun     bool
	Plan       Plan
	FreedBytes uint64
	FreedKnown bool
	Raw        string
}

// GC runs `nix-store --gc` (operation id: gc).
func (e *Executor) GC(ctx context.Context) (GCOutcome, error) {
	if err := require(e.R, "nix-store"); err != nil {
		return GCOutcome{}, err
	}
	if e.Opts.DryRun {
		return GCOutcome{DryRun: true, Plan: dryRun("Garbage collection", "nix-store --gc")}, nil
	}
	e.Verbosef("operation=gc")
	res, err := nix.GC(ctx, e.R)
	out := GCOutcome{FreedBytes: res.FreedBytes, FreedKnown: res.FreedKnown, Raw: res.Raw}
	return out, err
}

// GCOldOutcome is the result of aggressive GC.
type GCOldOutcome struct {
	DryRun     bool
	Plan       Plan
	FreedBytes uint64
	FreedKnown bool
	Raw        string
}

// GCOld runs `nix-collect-garbage -d` (operation id: gc-old). It removes old
// profile generations before GC; rollback points may be lost.
func (e *Executor) GCOld(ctx context.Context) (GCOldOutcome, error) {
	if err := require(e.R, "nix-collect-garbage"); err != nil {
		return GCOldOutcome{}, err
	}
	if e.Opts.DryRun {
		p := dryRun("Aggressive garbage collection", "nix-collect-garbage -d")
		p.Notes = append(p.Notes,
			"This removes old Nix profile generations.",
			"You may lose rollback points.")
		return GCOldOutcome{DryRun: true, Plan: p}, nil
	}
	e.Verbosef("operation=gc-old")
	res, err := nix.GCOld(ctx, e.R)
	out := GCOldOutcome{FreedBytes: res.FreedBytes, FreedKnown: res.FreedKnown, Raw: res.Raw}
	return out, err
}

// ---- Preview / diagnostics ----------------------------------------------

// DeadOutcome is the parsed result of `nix-store --gc --print-dead`.
type DeadOutcome struct {
	DryRun     bool
	Plan       Plan
	Count      int
	TotalBytes uint64
	BytesKnown bool
	Paths      []string
	Raw        string
}

// Dead previews dead store paths (operation id: dead). Read-only, but it
// still honours --dry-run: the command is not started and a plan is
// returned instead, so nothing executes while dry-run is enabled.
func (e *Executor) Dead(ctx context.Context, limit int) (DeadOutcome, error) {
	if err := require(e.R, "nix-store"); err != nil {
		return DeadOutcome{}, err
	}
	if e.Opts.DryRun {
		p := dryRun("Dead store paths preview", "nix-store --gc --print-dead")
		p.Notes = append(p.Notes, "This is a read-only preview; it would not change the store.")
		return DeadOutcome{DryRun: true, Plan: p}, nil
	}
	res, err := nix.Dead(ctx, e.R, limit)
	out := DeadOutcome{Raw: res.Raw}
	if err != nil {
		return out, err
	}
	// Count all dead paths even when the path list was truncated.
	all, _, _ := nix.ParseDeadPaths(res.Raw)
	out.Count = len(all)
	out.Paths = res.Paths
	out.TotalBytes = res.TotalBytes
	out.BytesKnown = res.BytesKnown
	return out, nil
}

// RootsOutcome is the parsed result of `nix-store --gc --print-roots`.
type RootsOutcome struct {
	DryRun bool
	Plan   Plan
	Roots  []nix.Root
	Raw    string
}

// Roots lists GC roots (operation id: roots). Diagnostic only; honours
// --dry-run like every other operation.
func (e *Executor) Roots(ctx context.Context) (RootsOutcome, error) {
	if err := require(e.R, "nix-store"); err != nil {
		return RootsOutcome{}, err
	}
	if e.Opts.DryRun {
		p := dryRun("GC roots listing", "nix-store --gc --print-roots")
		p.Notes = append(p.Notes, "This is a read-only diagnostic.")
		return RootsOutcome{DryRun: true, Plan: p}, nil
	}
	roots, raw, err := nix.RootsList(ctx, e.R)
	return RootsOutcome{Roots: roots, Raw: raw}, err
}

// LiveOutcome is the result of `nix-store --gc --print-live`.
type LiveOutcome struct {
	DryRun bool
	Plan   Plan
	Raw    string
}

// Live lists live store paths (operation id: live). Diagnostic only.
func (e *Executor) Live(ctx context.Context) (LiveOutcome, error) {
	if err := require(e.R, "nix-store"); err != nil {
		return LiveOutcome{}, err
	}
	if e.Opts.DryRun {
		p := dryRun("Live store paths listing", "nix-store --gc --print-live")
		p.Notes = append(p.Notes, "This is a read-only diagnostic.")
		return LiveOutcome{DryRun: true, Plan: p}, nil
	}
	raw, err := nix.LivePaths(ctx, e.R)
	return LiveOutcome{Raw: raw}, err
}

// ---- Optimise -----------------------------------------------------------

// OptimiseOutcome is the result of `nix-store --optimise`.
type OptimiseOutcome struct {
	DryRun bool
	Plan   Plan
	Before uint64
	After  uint64
	Saved  uint64
	Known  bool
	Raw    string
}

// Optimise deduplicates identical store files (operation id: optimise).
func (e *Executor) Optimise(ctx context.Context) (OptimiseOutcome, error) {
	if err := require(e.R, "nix-store"); err != nil {
		return OptimiseOutcome{}, err
	}
	if e.Opts.DryRun {
		return OptimiseOutcome{DryRun: true, Plan: dryRun("Nix store optimisation", "nix-store --optimise")}, nil
	}
	e.Verbosef("operation=optimise")
	store := nix.StorePath(ctx, e.R)
	before, beforeOK := nix.StoreSize(ctx, e.R, store)
	res, err := nix.Optimise(ctx, e.R)
	out := OptimiseOutcome{Raw: res.Raw, Before: before, Known: beforeOK}
	if err != nil {
		return out, err
	}
	after, afterOK := nix.StoreSize(ctx, e.R, store)
	if beforeOK && afterOK && after < before {
		out.After = after
		out.Saved = before - after
		out.Known = true
	} else {
		out.Known = false
	}
	return out, nil
}

// ---- devenv -------------------------------------------------------------

// DevenvGCOutcome is the result of `devenv gc`.
type DevenvGCOutcome struct {
	DryRun bool
	Plan   Plan
	Raw    string
}

// DevenvGC runs `devenv gc` (operation id: devenv-gc).
func (e *Executor) DevenvGC(ctx context.Context) (DevenvGCOutcome, error) {
	if err := require(e.R, "devenv"); err != nil {
		return DevenvGCOutcome{}, err
	}
	if e.Opts.DryRun {
		return DevenvGCOutcome{DryRun: true, Plan: dryRun("devenv garbage collection", "devenv gc")}, nil
	}
	e.Verbosef("operation=devenv-gc")
	raw, err := devenv.GC(ctx, e.R)
	return DevenvGCOutcome{Raw: raw}, err
}

// DevenvUpdateOutcome is the result of `devenv update`.
type DevenvUpdateOutcome struct {
	DryRun bool
	Plan   Plan
	Raw    string
}

// DevenvUpdate runs `devenv update` (operation id: devenv-update). It
// refreshes the devenv project's inputs and lock files, so it honours
// --dry-run like the destructive operations.
func (e *Executor) DevenvUpdate(ctx context.Context) (DevenvUpdateOutcome, error) {
	if err := require(e.R, "devenv"); err != nil {
		return DevenvUpdateOutcome{}, err
	}
	if e.Opts.DryRun {
		return DevenvUpdateOutcome{DryRun: true, Plan: dryRun("devenv environment update", "devenv update")}, nil
	}
	e.Verbosef("operation=devenv-update")
	raw, err := devenv.Update(ctx, e.R)
	return DevenvUpdateOutcome{Raw: raw}, err
}

// DevenvInfoOutcome is the result of `devenv info`.
type DevenvInfoOutcome struct {
	DryRun bool
	Plan   Plan
	Raw    string
}

// DevenvInfo runs `devenv info` (operation id: devenv-info). Read-only, but
// it honours --dry-run like every other operation.
func (e *Executor) DevenvInfo(ctx context.Context) (DevenvInfoOutcome, error) {
	if err := require(e.R, "devenv"); err != nil {
		return DevenvInfoOutcome{}, err
	}
	if e.Opts.DryRun {
		p := dryRun("devenv environment info", "devenv info")
		p.Notes = append(p.Notes, "This is a read-only diagnostic.")
		return DevenvInfoOutcome{DryRun: true, Plan: p}, nil
	}
	raw, err := devenv.Info(ctx, e.R)
	return DevenvInfoOutcome{Raw: raw}, err
}

// ---- Home Manager ---------------------------------------------------------

// HMGenerationsOutcome lists Home Manager generations.
type HMGenerationsOutcome struct {
	DryRun      bool
	Plan        Plan
	Generations []homemanager.Generation
	Raw         string
}

// HMGenerations lists generations (operation id: hm-generations). Read-only,
// but it honours --dry-run like every other operation.
func (e *Executor) HMGenerations(ctx context.Context) (HMGenerationsOutcome, error) {
	if err := require(e.R, "home-manager"); err != nil {
		return HMGenerationsOutcome{}, err
	}
	if e.Opts.DryRun {
		p := dryRun("Home Manager generations listing", "home-manager generations")
		p.Notes = append(p.Notes, "This is a read-only listing.")
		return HMGenerationsOutcome{DryRun: true, Plan: p}, nil
	}
	gens, raw, err := homemanager.List(ctx, e.R)
	return HMGenerationsOutcome{Generations: gens, Raw: raw}, err
}

// HMExpireOutcome is the result of expiring old generations.
type HMExpireOutcome struct {
	DryRun    bool
	Plan      Plan
	Period    string
	GNUPeriod string
	Eligible  []homemanager.Generation
	Raw       string
}

// HMExpire expires Home Manager generations older than the given period
// (operation id: hm-expire). In dry-run the listing still runs so eligible
// generations can be displayed, but expiry is never started. When eligible
// generations exist and --yes was not given, the executor's Confirm hook is
// consulted before expiring.
func (e *Executor) HMExpire(ctx context.Context, period string) (HMExpireOutcome, error) {
	if err := require(e.R, "home-manager"); err != nil {
		return HMExpireOutcome{}, err
	}
	gnu, err := homemanager.GNUDatePeriod(period)
	if err != nil {
		return HMExpireOutcome{}, fmt.Errorf("%w: %v", ErrCancelled, err)
	}
	out := HMExpireOutcome{Period: period, GNUPeriod: gnu}
	if gens, raw, err := homemanager.List(ctx, e.R); err == nil {
		out.Eligible = homemanager.Eligible(gens, period, time.Now())
		out.Raw = raw
	} else if !e.Opts.DryRun {
		return out, err
	}
	if e.Opts.DryRun {
		out.DryRun = true
		out.Plan = dryRun("Expire Home Manager generations older than "+util.HumanPeriod(period),
			"home-manager expire-generations \""+gnu+"\"")
		return out, nil
	}
	if len(out.Eligible) > 0 && !e.Opts.Yes && e.Confirm != nil {
		ok := e.Confirm("Expire Home Manager generations older than "+util.HumanPeriod(period),
			"home-manager expire-generations \""+gnu+"\"")
		if !ok {
			return out, ErrCancelled
		}
	}
	e.Verbosef("operation=hm-expire period=%s", gnu)
	raw, err := homemanager.Expire(ctx, e.R, gnu)
	out.Raw = raw
	return out, err
}

// ---- Clean workflow -------------------------------------------------------

// CleanPlan selects which operations the cleanup workflow runs. Defaults are
// conservative: aggressive GC and optimisation are never enabled implicitly.
type CleanPlan struct {
	DevenvGC    bool
	HMExpire    bool
	HMOlderThan string
	GC          bool
	Optimise    bool
	Aggressive  bool
}

// DefaultCleanPlan returns the recommended defaults per AGENTS.md:
// devenv gc, HM generations older than 30 days, normal GC.
func DefaultCleanPlan() CleanPlan {
	return CleanPlan{DevenvGC: true, HMExpire: true, HMOlderThan: "30d", GC: true}
}

// CleanStep is one ordered step of the cleanup workflow.
type CleanStep struct {
	ID      string // operation id
	Title   string
	Command string
}

// Steps returns the cleanup steps in the mandated order:
// devenv gc → Home Manager expiry → Nix GC → (optional) aggressive GC → optimise.
func (p CleanPlan) Steps() []CleanStep {
	var steps []CleanStep
	if p.DevenvGC {
		steps = append(steps, CleanStep{ID: "devenv-gc", Title: "devenv garbage collection", Command: "devenv gc"})
	}
	if p.HMExpire {
		period := p.HMOlderThan
		if period == "" {
			period = "30d"
		}
		days, err := util.ParseDays(period)
		if err != nil {
			days = 30
			period = "30d"
		}
		steps = append(steps, CleanStep{
			ID:      "hm-expire",
			Title:   "Expire Home Manager generations older than " + util.HumanPeriod(period),
			Command: fmt.Sprintf("home-manager expire-generations \"-%d days\"", days),
		})
	}
	if p.GC {
		steps = append(steps, CleanStep{ID: "gc", Title: "Nix garbage collection", Command: "nix-store --gc"})
	}
	if p.Aggressive {
		steps = append(steps, CleanStep{ID: "gc-old", Title: "Aggressive GC (removes old profile generations)", Command: "nix-collect-garbage -d"})
	}
	if p.Optimise {
		steps = append(steps, CleanStep{ID: "optimise", Title: "Nix store optimisation", Command: "nix-store --optimise"})
	}
	return steps
}

// RequiredTools lists the tools needed for the plan (dependency pre-check).
func (p CleanPlan) RequiredTools() []string {
	var tools []string
	seen := map[string]bool{}
	add := func(t string) {
		if !seen[t] {
			seen[t] = true
			tools = append(tools, t)
		}
	}
	if p.DevenvGC {
		add("devenv")
	}
	if p.HMExpire {
		add("home-manager")
	}
	if p.GC {
		add("nix-store")
	}
	if p.Aggressive {
		add("nix-collect-garbage")
	}
	if p.Optimise {
		add("nix-store")
	}
	return tools
}

// StepResult records the outcome of one cleanup step.
type StepResult struct {
	Step   CleanStep
	DryRun bool
	Plan   Plan
	Err    error
	Raw    string
	Freed  uint64
	Known  bool
}

// CleanOutcome aggregates step results in execution order.
type CleanOutcome struct {
	DryRun bool
	Plan   Plan
	Steps  []StepResult
	Failed bool
}

// Clean executes the cleanup workflow (operation id: clean). Ordering follows
// AGENTS.md §33. In dry-run mode every step is replaced by its plan.
func (e *Executor) Clean(ctx context.Context, plan CleanPlan) (CleanOutcome, error) {
	steps := plan.Steps()
	out := CleanOutcome{DryRun: e.Opts.DryRun}
	if e.Opts.DryRun {
		var cmds []string
		for _, s := range steps {
			cmds = append(cmds, s.Command)
		}
		out.Plan = Plan{Title: "Cleanup plan", Commands: cmds}
		return out, nil
	}
	if err := require(e.R, plan.RequiredTools()...); err != nil {
		return out, err
	}
	for _, s := range steps {
		sr := StepResult{Step: s}
		var err error
		switch s.ID {
		case "devenv-gc":
			var o DevenvGCOutcome
			o, err = e.DevenvGC(ctx)
			sr.Raw = o.Raw
		case "hm-expire":
			period := plan.HMOlderThan
			if period == "" {
				period = "30d"
			}
			var o HMExpireOutcome
			o, err = e.HMExpire(ctx, period)
			sr.Raw = o.Raw
		case "gc":
			var o GCOutcome
			o, err = e.GC(ctx)
			sr.Freed, sr.Known, sr.Raw = o.FreedBytes, o.FreedKnown, o.Raw
		case "gc-old":
			var o GCOldOutcome
			o, err = e.GCOld(ctx)
			sr.Freed, sr.Known, sr.Raw = o.FreedBytes, o.FreedKnown, o.Raw
		case "optimise":
			var o OptimiseOutcome
			o, err = e.Optimise(ctx)
			sr.Raw = o.Raw
		}
		sr.Err = err
		if err != nil {
			out.Failed = true
			out.Steps = append(out.Steps, sr)
			return out, err
		}
		out.Steps = append(out.Steps, sr)
	}
	return out, nil
}
