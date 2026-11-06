package cli

import (
	"bytes"
	"testing"
)

func TestUsageExitsZeroPath(t *testing.T) {
	var out, errb bytes.Buffer
	s := Stdio{Out: &out, Err: &errb}
	if code := Main([]string{"--help"}, s); code != ExitSuccess {
		t.Fatalf("got %d want %d", code, ExitSuccess)
	}
	if !bytes.Contains(out.Bytes(), []byte("tnxs — Nix store maintenance")) {
		t.Fatalf("usage text missing: %q", out.String())
	}
}

func TestUnknownCommandExitCode(t *testing.T) {
	var out, errb bytes.Buffer
	s := Stdio{Out: &out, Err: &errb}
	if code := Main([]string{"bogus"}, s); code != ExitUsage {
		t.Fatalf("got %d want %d", code, ExitUsage)
	}
}

func TestDryRunFlagBeforeCommand(t *testing.T) {
	var out, errb bytes.Buffer
	s := Stdio{Out: &out, Err: &errb}
	// Uses a fake-friendly path only to verify flag parsing does not error.
	// gc --dry-run with nix-store missing must exit with dependency code 3
	// without printing a usage error (code 2).
	code := Main([]string{"--dry-run", "gc"}, s)
	if code == ExitUsage {
		t.Fatalf("dry-run flag parsing failed: %q %q", out.String(), errb.String())
	}
}
