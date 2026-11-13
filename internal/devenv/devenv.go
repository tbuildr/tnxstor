// Package devenv wraps the official `devenv gc`, `devenv update` and
// `devenv info` commands and discovers devenv projects on disk. devenv
// maintains timestamped GC roots itself; `devenv gc` cleans obsolete roots
// and then performs Nix garbage collection. tnxstor never touches devenv's
// GC directory directly.
package devenv

import (
	"context"

	"tnxstor/internal/runner"
)

// Available reports whether devenv is installed.
func Available(r runner.Runner) bool {
	_, ok := r.LookPath("devenv")
	return ok
}

// GC runs `devenv gc` in dir. This is distinct from plain Nix GC.
func GC(ctx context.Context, r runner.Runner, dir string) (string, error) {
	res := r.RunDir(ctx, dir, "devenv", "gc")
	if !res.OK() {
		return res.Stdout + res.Stderr, res.Error()
	}
	return res.Stdout + res.Stderr, nil
}

// Update runs `devenv update` in dir, refreshing the environment's inputs
// and lock files.
func Update(ctx context.Context, r runner.Runner, dir string) (string, error) {
	res := r.RunDir(ctx, dir, "devenv", "update")
	if !res.OK() {
		return res.Stdout + res.Stderr, res.Error()
	}
	return res.Stdout + res.Stderr, nil
}

// Info runs `devenv info` in dir, a read-only overview of the environment.
func Info(ctx context.Context, r runner.Runner, dir string) (string, error) {
	res := r.RunDir(ctx, dir, "devenv", "info")
	if !res.OK() {
		return res.Stdout + res.Stderr, res.Error()
	}
	return res.Stdout + res.Stderr, nil
}
