// Package devenv wraps the official `devenv gc` command. devenv maintains
// timestamped GC roots itself; `devenv gc` cleans obsolete roots and then
// performs Nix garbage collection. tnxstor never touches devenv's GC
// directory directly.
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

// GC runs `devenv gc`. This is distinct from plain Nix GC.
func GC(ctx context.Context, r runner.Runner) (string, error) {
	res := r.Run(ctx, "devenv", "gc")
	if !res.OK() {
		return res.Stdout + res.Stderr, res.Error()
	}
	return res.Stdout + res.Stderr, nil
}

// Update runs `devenv update`, refreshing the devenv environment's inputs
// and lock files for the project in the current directory.
func Update(ctx context.Context, r runner.Runner) (string, error) {
	res := r.Run(ctx, "devenv", "update")
	if !res.OK() {
		return res.Stdout + res.Stderr, res.Error()
	}
	return res.Stdout + res.Stderr, nil
}

// Info runs `devenv info`, a read-only overview of the environment in the
// current directory.
func Info(ctx context.Context, r runner.Runner) (string, error) {
	res := r.Run(ctx, "devenv", "info")
	if !res.OK() {
		return res.Stdout + res.Stderr, res.Error()
	}
	return res.Stdout + res.Stderr, nil
}
