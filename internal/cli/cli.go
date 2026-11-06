package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"tnxstor/internal/homemanager"
	"tnxstor/internal/operations"
	"tnxstor/internal/runner"
	"tnxstor/internal/tui"
	"tnxstor/internal/util"
)

// GlobalFlags are the options accepted by every subcommand.
type GlobalFlags struct {
	DryRun  bool
	Yes     bool
	Verbose bool
	Quiet   bool
	NoColor bool
	Version bool
}

// Version is the tnxs release version.
const Version = "0.1.0"

// Usage prints the top-level help.
func Usage(s Stdio) {
	fmt.Fprint(s.Out, `tnxs — Nix store maintenance for standalone Nix systems

Usage:
  tnxs <command> [options]

Commands:
  tui             Launch interactive interface
  status          Show Nix/Home Manager/devenv status
  clean           Run a configurable cleanup
  gc              Garbage collect unreachable store paths
  gc-old          Remove old generations and garbage collect
  dead            Show unreachable store paths
  live            Show live store paths
  roots           Show garbage collection roots
  optimise        Optimise/deduplicate the Nix store

  devenv-gc       Clean devenv GC roots
  devenv-update   Update devenv inputs and lock files
  devenv-info     Show devenv environment info (read-only)
  hm-generations  Show Home Manager generations
  hm-expire       Expire old Home Manager generations

Global options:
  --dry-run       Show actions without performing destructive operations
  --yes           Skip confirmation prompts
  --verbose       Enable verbose output
  --quiet         Suppress informational output
  --no-color      Disable colour
  --version       Show version
  -h, --help      Show help

Exit codes:
  0 success  1 operation failed  2 invalid arguments
  3 dependency missing  4 user cancelled
`)
}

// Main is the CLI entry point. It returns the process exit code.
func Main(args []string, s Stdio) int {
	if len(args) == 0 {
		if !Interactive() {
			Usage(s)
			return ExitSuccess
		}
		return runTUI(s)
	}

	cmd := args[0]
	switch cmd {
	case "-h", "--help", "help":
		Usage(s)
		return ExitSuccess
	}

	g := GlobalFlags{}
	rest := make([]string, 0, len(args))
	for _, a := range args {
		switch a {
		case "--dry-run":
			g.DryRun = true
		case "--yes", "-y":
			g.Yes = true
		case "--verbose":
			g.Verbose = true
		case "--quiet", "-q":
			g.Quiet = true
		case "--no-color":
			g.NoColor = true
		case "--version", "-v":
			g.Version = true
		default:
			rest = append(rest, a)
		}
	}
	if len(rest) == 0 {
		if g.Version {
			fmt.Fprintf(s.Out, "tnxs %s\n", Version)
			return ExitSuccess
		}
		Usage(s)
		return ExitUsage
	}
	cmd = rest[0]
	if g.Version {
		fmt.Fprintf(s.Out, "tnxs %s\n", Version)
		return ExitSuccess
	}

	r := runner.NewExec()
	if g.Verbose {
		r.Verbose = func(f string, a ...any) { fmt.Fprintf(s.Err, "DEBUG "+f+"\n", a...) }
	}
	ex := &operations.Executor{R: r, Opts: operations.Options{
		DryRun: g.DryRun, Yes: g.Yes, Verbose: g.Verbose, Quiet: g.Quiet, NoColor: g.NoColor,
	}}
	ex.Confirm = func(title, command string) bool {
		return confirm(s, title, command, g.Yes) == nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub := rest[1:]
	var err error
	switch cmd {
	case "tui":
		return runTUI(s)
	case "status":
		err = cmdStatus(ctx, ex, s)
	case "dead":
		err = cmdDead(ctx, ex, s)
	case "roots":
		err = cmdRoots(ctx, ex, s)
	case "live":
		err = cmdLive(ctx, ex, s)
	case "gc":
		err = cmdGC(ctx, ex, g, s)
	case "gc-old":
		err = cmdGCOld(ctx, ex, g, s)
	case "optimise":
		err = cmdOptimise(ctx, ex, g, s)
	case "devenv-gc", "devenv_gc":
		err = cmdDevenvGC(ctx, ex, g, s)
	case "devenv-update":
		err = cmdDevenvUpdate(ctx, ex, g, s)
	case "devenv-info":
		err = cmdDevenvInfo(ctx, ex, s)
	case "hm-generations", "hm-generations-list":
		err = cmdHMGenerations(ctx, ex, s)
	case "hm-expire":
		err = cmdHMExpire(ctx, ex, g, sub, s)
	case "clean":
		err = cmdClean(ctx, ex, g, sub, s)
	default:
		return failf(s, ExitUsage, "unknown command %q (see tnxs --help)", cmd).(*ExitError).Code
	}
	if err == nil {
		return ExitSuccess
	}
	var ee *ExitError
	if errors.As(err, &ee) {
		if ee.Message != "" {
			fmt.Fprintln(s.Err, ee.Message)
		}
		return ee.Code
	}
	if errors.Is(err, operations.ErrMissingDependency) {
		fmt.Fprintln(s.Err, "tnxs:", err)
		fmt.Fprintln(s.Err, "Operation cannot continue.")
		return ExitDependency
	}
	if errors.Is(err, operations.ErrCancelled) {
		fmt.Fprintln(s.Err, "tnxs: cancelled:", errors.Unwrap(err))
		return ExitCancelled
	}
	fmt.Fprintln(s.Err, "tnxs: error:", err)
	return ExitFailure
}

func runTUI(s Stdio) int {
	if !Interactive() {
		return failf(s, ExitUsage, "no interactive terminal available; use explicit subcommands or run tnxs inside a TTY").(*ExitError).Code
	}
	if err := tui.Run(); err != nil {
		return failf(s, ExitFailure, "TUI: %v", err).(*ExitError).Code
	}
	return ExitSuccess
}

// confirm asks the user before a destructive operation in interactive mode.
func confirm(s Stdio, title, command string, yes bool) error {
	if yes {
		return nil
	}
	if !Interactive() {
		return &ExitError{Code: ExitCancelled, Message: fmt.Sprintf(
			"refusing to run %q non-interactively without --yes", command)}
	}
	fmt.Fprintf(s.Out, "%s\n\nThis will execute:\n\n  %s\n\nContinue? [y/N] ", title, command)
	rd := bufio.NewReader(os.Stdin)
	line, _ := rd.ReadString('\n')
	ans := strings.ToLower(strings.TrimSpace(line))
	if ans != "y" && ans != "yes" {
		return &ExitError{Code: ExitCancelled, Message: "aborted by user"}
	}
	return nil
}

// ---- commands -----------------------------------------------------------

func cmdStatus(ctx context.Context, ex *operations.Executor, s Stdio) error {
	st, err := ex.Status(ctx)
	if err != nil {
		return err
	}
	yn := func(b bool) string {
		if b {
			return "yes"
		}
		return "no"
	}
	daemon := yn(st.Daemon)
	if st.DaemonNote != "" {
		daemon += " (" + st.DaemonNote + ")"
	}
	fmt.Fprintln(s.Out, "tnxstor status")
	fmt.Fprintln(s.Out)
	fmt.Fprintf(s.Out, "Host OS       %s\n", orDash(st.OS.OSPretty))
	fmt.Fprintf(s.Out, "Nix version   %s\n", orDash(st.NixVersion))
	fmt.Fprintf(s.Out, "Store         %s\n", st.StorePath)
	if st.StoreKnown {
		fmt.Fprintf(s.Out, "Store size    %s\n", util.Bytes(st.StoreSize))
	} else {
		fmt.Fprintf(s.Out, "Store size    %s\n", "-")
	}
	fmt.Fprintf(s.Out, "NIX_REMOTE    %s\n", orDash(st.NixRemote))
	fmt.Fprintf(s.Out, "Daemon        %s\n", daemon)
	fmt.Fprintln(s.Out)
	fmt.Fprintf(s.Out, "Home Manager  %s\n", yn(st.Deps.HomeManager))
	if st.Deps.HomeManager && st.HMHasData {
		fmt.Fprintf(s.Out, "  Generation  %d\n", st.HMCurrent)
		fmt.Fprintf(s.Out, "  Generations %d\n", st.HMCount)
	}
	fmt.Fprintf(s.Out, "devenv        %s\n", yn(st.Deps.Devenv))
	if st.Deps.Devenv {
		fmt.Fprintf(s.Out, "  GC roots    %s\n", yn(st.DevenvRoot))
	}
	return nil
}

func orDash(v string) string {
	if strings.TrimSpace(v) == "" {
		return "-"
	}
	return v
}

func cmdDead(ctx context.Context, ex *operations.Executor, s Stdio) error {
	out, err := ex.Dead(ctx, 0)
	if err != nil {
		return err
	}
	if out.DryRun {
		printDryRun(s, out.Plan)
		return nil
	}
	fmt.Fprintln(s.Out, "Dead store paths")
	fmt.Fprintln(s.Out)
	fmt.Fprintf(s.Out, "%d paths\n", out.Count)
	if out.BytesKnown && out.TotalBytes > 0 {
		fmt.Fprintf(s.Out, "Estimated reclaimable: %s\n", util.Bytes(out.TotalBytes))
	}
	fmt.Fprintln(s.Out)
	if out.Count > 0 && len(out.Paths) > 0 {
		for _, p := range out.Paths {
			fmt.Fprintln(s.Out, p)
		}
	} else {
		fmt.Fprint(s.Out, out.Raw)
	}
	return nil
}

func cmdRoots(ctx context.Context, ex *operations.Executor, s Stdio) error {
	out, err := ex.Roots(ctx)
	if err != nil {
		return err
	}
	if out.DryRun {
		printDryRun(s, out.Plan)
		return nil
	}
	if len(out.Roots) == 0 {
		fmt.Fprint(s.Out, out.Raw)
		return nil
	}
	fmt.Fprintf(s.Out, "GC roots (%d)\n\n", len(out.Roots))
	for _, r := range out.Roots {
		fmt.Fprintf(s.Out, "  %-12s %s -> %s\n", r.Class, r.Link, r.Target)
	}
	return nil
}

func cmdLive(ctx context.Context, ex *operations.Executor, s Stdio) error {
	out, err := ex.Live(ctx)
	if err != nil {
		return err
	}
	if out.DryRun {
		printDryRun(s, out.Plan)
		return nil
	}
	fmt.Fprint(s.Out, out.Raw)
	return nil
}

func cmdGC(ctx context.Context, ex *operations.Executor, g GlobalFlags, s Stdio) error {
	if ex.Opts.DryRun {
		out, err := ex.GC(ctx)
		if err != nil {
			return err
		}
		printDryRun(s, out.Plan)
		return nil
	}
	if !ex.Opts.Quiet {
		fmt.Fprintln(s.Out, "Running Nix garbage collection...")
	}
	out, err := ex.GC(ctx)
	if err != nil {
		return err
	}
	printGCResult(s, "Nix GC complete", out.FreedBytes, out.FreedKnown, out.Raw)
	return nil
}

func cmdGCOld(ctx context.Context, ex *operations.Executor, g GlobalFlags, s Stdio) error {
	if ex.Opts.DryRun {
		out, err := ex.GCOld(ctx)
		if err != nil {
			return err
		}
		printDryRun(s, out.Plan)
		return nil
	}
	if err := confirm(s, "Aggressive garbage collection", "nix-collect-garbage -d", ex.Opts.Yes); err != nil {
		return err
	}
	if !ex.Opts.Quiet {
		fmt.Fprintln(s.Out, "Removing old profile generations and collecting garbage...")
	}
	out, err := ex.GCOld(ctx)
	if err != nil {
		return err
	}
	printGCResult(s, "Aggressive GC complete", out.FreedBytes, out.FreedKnown, out.Raw)
	return nil
}

func cmdOptimise(ctx context.Context, ex *operations.Executor, g GlobalFlags, s Stdio) error {
	if ex.Opts.DryRun {
		out, err := ex.Optimise(ctx)
		if err != nil {
			return err
		}
		printDryRun(s, out.Plan)
		return nil
	}
	if !ex.Opts.Quiet {
		fmt.Fprintln(s.Out, "Optimising Nix store...")
	}
	out, err := ex.Optimise(ctx)
	if err != nil {
		return err
	}
	if out.Known {
		fmt.Fprintln(s.Out)
		fmt.Fprintf(s.Out, "Before: %s\n", util.Bytes(out.Before))
		fmt.Fprintf(s.Out, "After:  %s\n", util.Bytes(out.After))
		fmt.Fprintf(s.Out, "Saved:  %s\n", util.Bytes(out.Saved))
	} else if !ex.Opts.Quiet {
		fmt.Fprintln(s.Out)
		fmt.Fprint(s.Out, strings.TrimRight(out.Raw, "\n")+"\n")
	}
	return nil
}

func cmdDevenvGC(ctx context.Context, ex *operations.Executor, g GlobalFlags, s Stdio) error {
	if ex.Opts.DryRun {
		out, err := ex.DevenvGC(ctx)
		if err != nil {
			return err
		}
		printDryRun(s, out.Plan)
		return nil
	}
	if !ex.Opts.Quiet {
		fmt.Fprintln(s.Out, "Running devenv garbage collection...")
	}
	out, err := ex.DevenvGC(ctx)
	if err != nil {
		return err
	}
	if !ex.Opts.Quiet && strings.TrimSpace(out.Raw) != "" {
		fmt.Fprintln(s.Out, strings.TrimRight(out.Raw, "\n"))
	}
	return nil
}

func cmdDevenvUpdate(ctx context.Context, ex *operations.Executor, g GlobalFlags, s Stdio) error {
	if ex.Opts.DryRun {
		out, err := ex.DevenvUpdate(ctx)
		if err != nil {
			return err
		}
		printDryRun(s, out.Plan)
		return nil
	}
	if !ex.Opts.Quiet {
		fmt.Fprintln(s.Out, "Running devenv update...")
	}
	out, err := ex.DevenvUpdate(ctx)
	if err != nil {
		return err
	}
	if !ex.Opts.Quiet && strings.TrimSpace(out.Raw) != "" {
		fmt.Fprintln(s.Out, strings.TrimRight(out.Raw, "\n"))
	}
	if !ex.Opts.Quiet {
		fmt.Fprintln(s.Out, "devenv update complete.")
	}
	return nil
}

func cmdDevenvInfo(ctx context.Context, ex *operations.Executor, s Stdio) error {
	out, err := ex.DevenvInfo(ctx)
	if err != nil {
		return err
	}
	if out.DryRun {
		printDryRun(s, out.Plan)
		return nil
	}
	fmt.Fprint(s.Out, strings.TrimRight(out.Raw, "\n")+"\n")
	return nil
}

func cmdHMGenerations(ctx context.Context, ex *operations.Executor, s Stdio) error {
	out, err := ex.HMGenerations(ctx)
	if err != nil {
		return err
	}
	if out.DryRun {
		printDryRun(s, out.Plan)
		return nil
	}
	if len(out.Generations) == 0 {
		// Parsing could not recover a table; show raw output so the user
		// always sees what home-manager reported.
		fmt.Fprintln(s.Out, "Home Manager generations")
		fmt.Fprintln(s.Out)
		fmt.Fprint(s.Out, strings.TrimRight(out.Raw, "\n")+"\n")
		return nil
	}
	fmt.Fprintln(s.Out, "Home Manager generations")
	fmt.Fprintln(s.Out)
	fmt.Fprintf(s.Out, "  %-8s %-20s %-12s %s\n", "CURRENT", "DATE", "GENERATION", "PATH")
	for _, g := range out.Generations {
		mark := " "
		if g.Current {
			mark = "✓"
		}
		fmt.Fprintf(s.Out, "%s %-8s %-20s %-12d %s\n", mark, "", fmtDate(g), g.Number, g.Path)
	}
	return nil
}

func fmtDate(g homemanager.Generation) string {
	if !g.Date.IsZero() {
		return g.Date.Format("2006-01-02 15:04")
	}
	return g.DateRaw
}

func cmdHMExpire(ctx context.Context, ex *operations.Executor, g GlobalFlags, args []string, s Stdio) error {
	fs := flag.NewFlagSet("hm-expire", flag.ContinueOnError)
	fs.SetOutput(s.Err)
	older := fs.String("older-than", "30d", "expire generations older than this period (e.g. 30d, 12w, 6m)")
	if err := fs.Parse(args); err != nil {
		return &ExitError{Code: ExitUsage}
	}
	if fs.NArg() > 0 {
		return failf(s, ExitUsage, "unexpected argument %q", fs.Arg(0))
	}
	out, err := ex.HMExpire(ctx, *older)
	if err != nil {
		return err
	}
	if out.DryRun {
		fmt.Fprintln(s.Out, "DRY RUN")
		fmt.Fprintln(s.Out)
		fmt.Fprintf(s.Out, "Would expire Home Manager generations older than %s.\n\n", util.HumanPeriod(out.Period))
		fmt.Fprintln(s.Out, "Eligible generations:")
		if len(out.Eligible) == 0 {
			fmt.Fprintln(s.Out, "  (none)")
		}
		for _, gen := range out.Eligible {
			fmt.Fprintf(s.Out, "  %d\n", gen.Number)
		}
		fmt.Fprintln(s.Out)
		fmt.Fprintln(s.Out, out.Plan.String())
		return nil
	}
	if !ex.Opts.Quiet && strings.TrimSpace(out.Raw) != "" {
		fmt.Fprint(s.Out, strings.TrimRight(out.Raw, "\n")+"\n")
	}
	if !ex.Opts.Quiet {
		fmt.Fprintln(s.Out, "Home Manager generation expiry complete.")
	}
	return nil
}

func printDryRun(s Stdio, p operations.Plan) {
	fmt.Fprintln(s.Out, "DRY RUN")
	if p.Title != "" {
		fmt.Fprintln(s.Out)
		fmt.Fprintln(s.Out, p.Title)
	}
	fmt.Fprintln(s.Out)
	fmt.Fprintln(s.Out, p.String())
}

// cleanFlags mirrors `tnxs clean` options. By default the plan is devenv GC,
// Home Manager generations older than --hm-older-than, and normal GC.
func cmdClean(ctx context.Context, ex *operations.Executor, g GlobalFlags, args []string, s Stdio) error {
	fs := flag.NewFlagSet("clean", flag.ContinueOnError)
	fs.SetOutput(s.Err)
	hmOlder := fs.String("hm-older-than", "30d", "expire Home Manager generations older than this period")
	devenv := fs.Bool("devenv", false, "include devenv GC")
	homeManager := fs.Bool("home-manager", false, "include Home Manager generation expiry")
	gc := fs.Bool("gc", false, "include Nix garbage collection")
	optimise := fs.Bool("optimise", false, "include Nix store optimisation")
	aggressive := fs.Bool("aggressive", false, "include aggressive GC (removes old generations)")
	if err := fs.Parse(args); err != nil {
		return &ExitError{Code: ExitUsage}
	}
	if fs.NArg() > 0 {
		return failf(s, ExitUsage, "unexpected argument %q", fs.Arg(0))
	}

	plan := operations.CleanPlan{HMOlderThan: *hmOlder}
	if *devenv || *homeManager || *gc || *optimise || *aggressive {
		// Explicit selection: only the requested steps run.
		plan.DevenvGC = *devenv
		plan.HMExpire = *homeManager
		plan.GC = *gc
		plan.Optimise = *optimise
		plan.Aggressive = *aggressive
	} else {
		plan = operations.DefaultCleanPlan()
	}

	steps := plan.Steps()
	if len(steps) == 0 {
		return failf(s, ExitUsage, "clean plan is empty")
	}

	if !ex.Opts.DryRun {
		// Confirm the whole plan up-front; individual steps must not
		// prompt again during execution.
		var cmds []string
		for _, st := range steps {
			cmds = append(cmds, st.Command)
		}
		title := "Cleanup plan"
		for i, st := range steps {
			title += fmt.Sprintf("\n%d. %s", i+1, st.Title)
		}
		if err := confirm(s, title, strings.Join(cmds, "\n  "), ex.Opts.Yes); err != nil {
			return err
		}
		ex.Confirm = func(string, string) bool { return true }
	}

	out, err := ex.Clean(ctx, plan)
	if err != nil && !errors.Is(err, operations.ErrMissingDependency) && !ex.Opts.DryRun {
		// Print partial progress before surfacing the failure.
		printCleanSteps(s, out, ex)
		return err
	}
	if err != nil {
		return err
	}

	if ex.Opts.DryRun {
		fmt.Fprintln(s.Out, "DRY RUN")
		fmt.Fprintln(s.Out)
		fmt.Fprintln(s.Out, "Cleanup plan")
		for i, st := range steps {
			fmt.Fprintf(s.Out, "%d. %s\n", i+1, st.Title)
		}
		fmt.Fprintln(s.Out)
		fmt.Fprintln(s.Out, "No destructive operations will be executed.")
		fmt.Fprintln(s.Out)
		fmt.Fprintln(s.Out, "Commands:")
		for _, st := range steps {
			fmt.Fprintf(s.Out, "\n  %s\n", st.Command)
		}
		return nil
	}
	printCleanSteps(s, out, ex)
	return nil
}

func printCleanSteps(s Stdio, out operations.CleanOutcome, ex *operations.Executor) {
	for _, sr := range out.Steps {
		fmt.Fprintf(s.Out, "\n[%s]\n", sr.Step.Title)
		fmt.Fprintf(s.Out, "Running: %s\n", sr.Step.Command)
		if sr.Err != nil {
			fmt.Fprintf(s.Out, "FAILED: %v\n", sr.Err)
			continue
		}
		if sr.Known && sr.Freed > 0 {
			fmt.Fprintf(s.Out, "Freed: %s\n", util.Bytes(sr.Freed))
		} else if strings.TrimSpace(sr.Raw) != "" {
			fmt.Fprint(s.Out, strings.TrimRight(sr.Raw, "\n")+"\n")
		}
		fmt.Fprintln(s.Out, "Done.")
	}
}

func printGCResult(s Stdio, title string, freed uint64, known bool, raw string) {
	fmt.Fprintln(s.Out)
	fmt.Fprintln(s.Out, title)
	if known && freed > 0 {
		fmt.Fprintf(s.Out, "\nFreed: %s\n", util.Bytes(freed))
	} else if strings.TrimSpace(raw) != "" {
		fmt.Fprintln(s.Out)
		fmt.Fprint(s.Out, strings.TrimRight(raw, "\n")+"\n")
	}
}
