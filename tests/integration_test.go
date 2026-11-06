// Package tests contains integration tests that run the compiled tnxs
// binary against fake executables. These tests never touch a real Nix store.
package tests

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const fixturesBin = "/workspace/tests/fixtures/bin"

func buildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "tnxs")
	cmd := exec.Command("go", "build", "-o", bin, "tnxstor/cmd/tnxs")
	cmd.Dir = ".."
	var out bytes.Buffer
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("build tnxs: %v\n%s", err, out.String())
	}
	return bin
}

func runTNXS(t *testing.T, bin string, args ...string) (string, string, int) {
	t.Helper()
	log := filepath.Join(t.TempDir(), "calls.log")
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "PATH="+fixturesBin+":"+os.Getenv("PATH"), "TNXS_TEST_LOG="+log, "HOME=/home/tester")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	code := 0
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			t.Fatalf("run tnxs: %v", err)
		}
	}
	calls := ""
	if data, err := os.ReadFile(log); err == nil {
		calls = string(data)
	}
	return stdout.String(), stderr.String() + calls, code
}

func TestStatus(t *testing.T) {
	bin := buildBinary(t)
	stdout, _, code := runTNXS(t, bin, "status")
	if code != 0 {
		t.Fatalf("exit = %d, stderr: %s", code, stdout)
	}
	for _, want := range []string{"nix (Nix) 2.19.2", "/nix/store", "Home Manager"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("status output missing %q:\n%s", want, stdout)
		}
	}
}

func TestDeadCommand(t *testing.T) {
	bin := buildBinary(t)
	stdout, _, code := runTNXS(t, bin, "dead")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(stdout, "3 paths") {
		t.Errorf("dead output should count 3 paths:\n%s", stdout)
	}
}

func TestGCDryRunDoesNotExecute(t *testing.T) {
	bin := buildBinary(t)
	stdout, stderr, code := runTNXS(t, bin, "gc", "--dry-run")
	if code != 0 {
		t.Fatalf("exit = %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "DRY RUN") || !strings.Contains(stdout, "nix-store --gc") {
		t.Errorf("dry-run output wrong:\n%s", stdout)
	}
	if strings.Contains(stderr, "nix-store --gc\n") && strings.Contains(stderr, "executed") {
		t.Error("gc must not execute during dry-run")
	}
}

func TestOptimiseDryRun(t *testing.T) {
	bin := buildBinary(t)
	stdout, _, code := runTNXS(t, bin, "optimise", "--dry-run")
	if code != 0 {
		t.Fatal(code)
	}
	if !strings.Contains(stdout, "nix-store --optimise") || !strings.Contains(stdout, "No changes have been made") {
		t.Errorf("optimise dry-run output wrong:\n%s", stdout)
	}
}

func TestHMExpireDryRunListsEligible(t *testing.T) {
	bin := buildBinary(t)
	stdout, _, code := runTNXS(t, bin, "hm-expire", "--older-than", "30d", "--dry-run")
	if code != 0 {
		t.Fatal(code)
	}
	if !strings.Contains(stdout, "100") || !strings.Contains(stdout, "DRY RUN") {
		t.Errorf("hm-expire dry-run should list generation 100:\n%s", stdout)
	}
}

func TestHMDryRunDoesNotExpire(t *testing.T) {
	bin := buildBinary(t)
	_, stderr, _ := runTNXS(t, bin, "hm-expire", "--older-than", "30d", "--dry-run")
	if strings.Contains(stderr, "expire-generations") {
		t.Error("expire-generations must not be called during dry-run")
	}
}

func TestGCOldDryRunExplainsRollback(t *testing.T) {
	bin := buildBinary(t)
	stdout, _, code := runTNXS(t, bin, "gc-old", "--dry-run")
	if code != 0 {
		t.Fatal(code)
	}
	if !strings.Contains(stdout, "rollback") || !strings.Contains(stdout, "nix-collect-garbage -d") {
		t.Errorf("gc-old dry-run must warn about rollback points:\n%s", stdout)
	}
}

func TestDevenvGCDryRun(t *testing.T) {
	bin := buildBinary(t)
	stdout, _, code := runTNXS(t, bin, "devenv-gc", "--dry-run")
	if code != 0 {
		t.Fatal(code)
	}
	if !strings.Contains(stdout, "devenv gc") {
		t.Errorf("devenv-gc dry-run output wrong:\n%s", stdout)
	}
}

func TestDevenvUpdateDryRun(t *testing.T) {
	bin := buildBinary(t)
	stdout, _, code := runTNXS(t, bin, "devenv-update", "--dry-run")
	if code != 0 {
		t.Fatal(code)
	}
	if !strings.Contains(stdout, "devenv update") || !strings.Contains(stdout, "No changes have been made") {
		t.Errorf("devenv-update dry-run output wrong:\n%s", stdout)
	}
}

func TestDevenvInfo(t *testing.T) {
	bin := buildBinary(t)
	stdout, _, code := runTNXS(t, bin, "devenv-info")
	if code != 0 {
		t.Fatal(code)
	}
	if !strings.Contains(stdout, "test-project") {
		t.Errorf("devenv-info output wrong:\n%s", stdout)
	}
}

func TestCleanDryRunShowsPlan(t *testing.T) {
	bin := buildBinary(t)
	stdout, _, code := runTNXS(t, bin, "clean", "--dry-run")
	if code != 0 {
		t.Fatal(code)
	}
	for _, want := range []string{"Cleanup plan", "devenv gc", "expire", "nix-store --gc"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("clean dry-run missing %q:\n%s", want, stdout)
		}
	}
}

func TestCleanYesExecutesInOrder(t *testing.T) {
	bin := buildBinary(t)
	_, stderr, code := runTNXS(t, bin, "clean", "--yes")
	if code != 0 {
		t.Fatalf("exit = %d, stderr: %s", code, stderr)
	}
	calls := extractCalls(stderr)
	wantOrder := []string{"devenv gc", "home-manager generations", "home-manager expire-generations -30 days", "nix-store --gc"}
	pos := 0
	for _, w := range wantOrder {
		found := false
		for ; pos < len(calls); pos++ {
			if calls[pos] == w {
				found = true
				pos++
				break
			}
		}
		if !found {
			t.Errorf("call %q not found in order:\n%s", w, strings.Join(calls, "\n"))
			break
		}
	}
}

func extractCalls(stderr string) []string {
	var out []string
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func TestGCOldNonInteractiveRequiresYes(t *testing.T) {
	bin := buildBinary(t)
	_, _, code := runTNXS(t, bin, "gc-old")
	if code != 4 {
		t.Errorf("gc-old without --yes non-interactively should exit 4 (cancelled), got %d", code)
	}
}

func TestUnknownCommandExitCode(t *testing.T) {
	bin := buildBinary(t)
	_, _, code := runTNXS(t, bin, "nonsense")
	if code != 2 {
		t.Errorf("unknown command should exit 2, got %d", code)
	}
}
