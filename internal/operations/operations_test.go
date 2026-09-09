package operations

import (
	"context"
	"errors"
	"testing"
	"time"

	"tnxstor/internal/runner"
)

const fakeHMGenerations = `No 128  2026-09-09 09:41:12   (current)   /nix/store/aaaa-home-manager-generation
No 127  2026-09-08 22:17:03   /nix/store/bbbb-home-manager-generation
No 100  2026-08-01 12:01:00   /nix/store/dddd-home-manager-generation
`

func newEx(f *runner.FakeRunner, dryRun bool) *Executor {
	return &Executor{R: f, Opts: Options{DryRun: dryRun}}
}

func TestDiagnosticOpsHonourDryRun(t *testing.T) {
	f := runner.NewFake()
	ex := newEx(f, true)
	// All diagnostics should produce plans without running anything.
	_, e1 := ex.Dead(context.Background(), 0)
	_, e2 := ex.Roots(context.Background())
	_, e3 := ex.Live(context.Background())
	_, e4 := ex.HMGenerations(context.Background())
	_, e5 := ex.DevenvInfo(context.Background())
	for i, e := range []error{e1, e2, e3, e4, e5} {
		if e != nil {
			t.Errorf("op %d: %v", i, e)
		}
	}
	if len(f.Calls) != 0 {
		t.Errorf("dry-run must not run any diagnostic command, got calls %v", f.Calls)
	}
}

func TestDryRunPlansMatchCommands(t *testing.T) {
	f := runner.NewFake()
	ex := newEx(f, true)
	d, _ := ex.Dead(context.Background(), 0)
	r, _ := ex.Roots(context.Background())
	l, _ := ex.Live(context.Background())
	h, _ := ex.HMGenerations(context.Background())
	i, _ := ex.DevenvInfo(context.Background())
	for _, tc := range []struct {
		p    Plan
		want string
	}{
		{d.Plan, "nix-store --gc --print-dead"},
		{r.Plan, "nix-store --gc --print-roots"},
		{l.Plan, "nix-store --gc --print-live"},
		{h.Plan, "home-manager generations"},
		{i.Plan, "devenv info"},
	} {
		if len(tc.p.Commands) != 1 || tc.p.Commands[0] != tc.want {
			t.Errorf("plan = %v, want [%s]", tc.p.Commands, tc.want)
		}
		if !contains(tc.p.String(), "No changes have been made") {
			t.Errorf("plan must state no changes: %q", tc.p.String())
		}
	}
}

func TestGCDryRunNeverExecutes(t *testing.T) {
	f := runner.NewFake()
	f.ScriptCmd("nix-store --gc", "42 store paths deleted, 1.00 GiB freed")
	ex := newEx(f, true)
	out, err := ex.GC(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !out.DryRun {
		t.Error("expected dry-run outcome")
	}
	if out.Plan.Commands[0] != "nix-store --gc" {
		t.Errorf("plan command = %q", out.Plan.Commands[0])
	}
	for _, c := range f.Calls {
		if c.CommandString() == "nix-store --gc" {
			t.Fatal("destructive command must not run in dry-run")
		}
	}
}

func TestGCExecutesAndParses(t *testing.T) {
	f := runner.NewFake()
	f.ScriptCmd("nix-store --gc", "42 store paths deleted, 1.00 GiB freed")
	ex := newEx(f, false)
	out, err := ex.GC(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !out.FreedKnown {
		t.Error("expected freed bytes to be parsed")
	}
	if out.FreedBytes != 1024*1024*1024 {
		t.Errorf("freed = %d", out.FreedBytes)
	}
}

func TestGCOldDryRunExplainsGenerations(t *testing.T) {
	f := runner.NewFake()
	ex := newEx(f, true)
	out, err := ex.GCOld(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	joined := out.Plan.String()
	if !contains(joined, "nix-collect-garbage -d") || !contains(joined, "rollback") {
		t.Errorf("dry-run plan must explain generation removal: %q", joined)
	}
	for _, c := range f.Calls {
		if c.Name == "nix-collect-garbage" {
			t.Fatal("aggressive GC must not run in dry-run")
		}
	}
}

func TestDevenvGCDryRun(t *testing.T) {
	f := runner.NewFake()
	ex := newEx(f, true)
	out, err := ex.DevenvGC(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if out.Plan.Commands[0] != "devenv gc" {
		t.Errorf("plan = %q", out.Plan.Commands)
	}
	for _, c := range f.Calls {
		if c.Name == "devenv" {
			t.Fatal("devenv gc must not run in dry-run")
		}
	}
}

func TestDevenvUpdateAndInfo(t *testing.T) {
	f := runner.NewFake()
	f.ScriptCmd("devenv update", "updated")
	f.ScriptCmd("devenv info", "Name: test")
	ex := newEx(f, false)

	info, err := ex.DevenvInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Raw != "Name: test" {
		t.Errorf("info raw = %q", info.Raw)
	}

	upd, err := ex.DevenvUpdate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if upd.Raw != "updated" {
		t.Errorf("update raw = %q", upd.Raw)
	}
	if !f.HasRun("devenv update") || !f.HasRun("devenv info") {
		t.Error("devenv update/info should have run")
	}
}

func TestDevenvUpdateDryRun(t *testing.T) {
	f := runner.NewFake()
	f.ScriptCmd("devenv info", "Name: test")
	ex := newEx(f, true)
	out, err := ex.DevenvUpdate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if out.Plan.Commands[0] != "devenv update" {
		t.Errorf("plan = %q", out.Plan.Commands)
	}
	for _, c := range f.Calls {
		if c.CommandString() == "devenv update" {
			t.Fatal("devenv update must not run in dry-run")
		}
	}
	// devenv info is read-only and always allowed.
	if _, err := ex.DevenvInfo(context.Background()); err != nil {
		t.Errorf("devenv info should run even in dry-run: %v", err)
	}
	if !f.HasRun("devenv info") {
		t.Error("devenv info should have been called")
	}
}

func TestHMExpireDryRunListsButDoesNotExpire(t *testing.T) {
	f := runner.NewFake()
	f.ScriptCmd("home-manager generations", fakeHMGenerations)
	ex := newEx(f, true)
	out, err := ex.HMExpire(context.Background(), "30d")
	if err != nil {
		t.Fatal(err)
	}
	if !out.DryRun {
		t.Error("expected dry-run")
	}
	// Discovery (read-only) is allowed and expected.
	if !f.HasRun("home-manager generations") {
		t.Error("read-only discovery should run in dry-run")
	}
	if len(out.Eligible) != 1 || out.Eligible[0].Number != 100 {
		t.Errorf("eligible = %+v, want generation 100", out.Eligible)
	}
	for _, c := range f.Calls {
		if contains(c.CommandString(), "expire-generations") {
			t.Fatal("expire-generations must not run in dry-run")
		}
	}
}

func TestHMExpireInvalidPeriod(t *testing.T) {
	f := runner.NewFake()
	ex := newEx(f, false)
	_, err := ex.HMExpire(context.Background(), "bogus")
	if !errors.Is(err, ErrCancelled) {
		t.Errorf("expected ErrCancelled, got %v", err)
	}
}

func TestDependencyMissing(t *testing.T) {
	f := runner.NewFake()
	f.Missing["nix-store"] = true
	ex := newEx(f, false)
	_, err := ex.GC(context.Background())
	if !errors.Is(err, ErrMissingDependency) {
		t.Errorf("expected ErrMissingDependency, got %v", err)
	}
}

func TestCleanOrdering(t *testing.T) {
	plan := CleanPlan{DevenvGC: true, HMExpire: true, HMOlderThan: "30d", GC: true, Optimise: true, Aggressive: true}
	steps := plan.Steps()
	want := []string{"devenv-gc", "hm-expire", "gc", "gc-old", "optimise"}
	if len(steps) != len(want) {
		t.Fatalf("got %d steps, want %d", len(steps), len(want))
	}
	for i, id := range want {
		if steps[i].ID != id {
			t.Errorf("step %d = %q, want %q", i, steps[i].ID, id)
		}
	}
}

func TestCleanDryRunPlanOnly(t *testing.T) {
	f := runner.NewFake()
	ex := newEx(f, true)
	out, err := ex.Clean(context.Background(), DefaultCleanPlan())
	if err != nil {
		t.Fatal(err)
	}
	if !out.DryRun {
		t.Error("expected dry-run")
	}
	if len(out.Plan.Commands) != 3 {
		t.Errorf("default plan should have 3 commands, got %v", out.Plan.Commands)
	}
	if len(f.Calls) != 0 {
		t.Errorf("no commands should run during clean --dry-run, got %v", f.Calls)
	}
}

func TestCleanExecutionOrder(t *testing.T) {
	f := runner.NewFake()
	f.ScriptCmd("devenv gc", "cleaned")
	f.ScriptCmd("home-manager generations", fakeHMGenerations)
	f.ScriptCmd("home-manager expire-generations -30 days", "expired")
	f.ScriptCmd("nix-store --gc", "1 store path deleted, 1.00 MiB freed")
	ex := newEx(f, false)
	ex.Opts.Yes = true
	out, err := ex.Clean(context.Background(), DefaultCleanPlan())
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range f.Calls {
		got = append(got, c.CommandString())
	}
	// Order: devenv gc, hm generations (discovery), hm expire, nix-store gc.
	if len(got) != 4 {
		t.Fatalf("calls = %v", got)
	}
	if got[0] != "devenv gc" || got[1] != "home-manager generations" || got[2] != "home-manager expire-generations -30 days" || got[3] != "nix-store --gc" {
		t.Errorf("ordering wrong: %v", got)
	}
	if len(out.Steps) != 3 {
		t.Errorf("steps = %d", len(out.Steps))
	}
}

func TestOptimiseDryRun(t *testing.T) {
	f := runner.NewFake()
	ex := newEx(f, true)
	out, err := ex.Optimise(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if out.Plan.Commands[0] != "nix-store --optimise" {
		t.Errorf("plan = %q", out.Plan.Commands)
	}
	for _, c := range f.Calls {
		if contains(c.CommandString(), "--optimise") {
			t.Fatal("optimise must not run in dry-run")
		}
	}
}

func TestConfirmDeclined(t *testing.T) {
	f := runner.NewFake()
	f.ScriptCmd("home-manager generations", fakeHMGenerations)
	ex := newEx(f, false)
	ex.Confirm = func(string, string) bool { return false }
	_, err := ex.HMExpire(context.Background(), "30d")
	if !errors.Is(err, ErrCancelled) {
		t.Errorf("expected ErrCancelled, got %v", err)
	}
	for _, c := range f.Calls {
		if contains(c.CommandString(), "expire-generations") {
			t.Fatal("expiry must not run after declined confirmation")
		}
	}
}

func TestDeadParseCounts(t *testing.T) {
	f := runner.NewFake()
	f.ScriptCmd("nix-store --gc --print-dead", "/nix/store/aaa-env\n/nix/store/bbb-env\n")
	ex := newEx(f, false)
	out, err := ex.Dead(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if out.Count != 2 {
		t.Errorf("count = %d, want 2", out.Count)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

var _ = time.Now
