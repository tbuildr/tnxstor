// Package cli implements the command-line interface. All Nix-specific work
// is delegated to the internal operations layer.
package cli

import (
	"fmt"
	"io"
	"os"
)

// Exit codes (AGENTS.md §26).
const (
	ExitSuccess    = 0 // success
	ExitFailure    = 1 // operation failed
	ExitUsage      = 2 // invalid command/arguments
	ExitDependency = 3 // required dependency missing
	ExitCancelled  = 4 // user cancelled
)

// ExitError carries an exit code to main.
type ExitError struct {
	Code    int
	Message string
}

func (e *ExitError) Error() string { return e.Message }

// Stdio bundles the CLI's output streams.
type Stdio struct {
	Out io.Writer
	Err io.Writer
}

// DefaultStdio wires the standard streams.
func DefaultStdio() Stdio { return Stdio{Out: os.Stdout, Err: os.Stderr} }

func isTTY(f *os.File) bool {
	st, err := f.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice != 0
}

// Interactive reports whether both stdin and stdout are attached to a TTY.
func Interactive() bool { return isTTY(os.Stdin) && isTTY(os.Stdout) }

func failf(s Stdio, code int, format string, args ...any) error {
	fmt.Fprintf(s.Err, "tnxs: "+format+"\n", args...)
	return &ExitError{Code: code}
}
