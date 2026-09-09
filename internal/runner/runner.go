// Package runner provides an abstraction over command execution so that
// operations can be tested without touching a real Nix store.
package runner

import (
	"bytes"
	"context"
	"fmt"
	"os"
	osexec "os/exec"
	"strings"
	"time"
)

// Result captures the outcome of an executed command.
type Result struct {
	Command  string
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
}

// OK reports whether the command completed successfully.
func (r Result) OK() bool { return r.Err == nil && r.ExitCode == 0 }

// Output returns the combined command line of the executed command.
func (r Result) Output() string { return r.Command }

// Error builds a human readable error including stderr when present.
func (r Result) Error() error {
	if r.Err == nil && r.ExitCode == 0 {
		return nil
	}
	if r.Err != nil {
		if _, ok := r.Err.(*exitError); ok || strings.TrimSpace(r.Stderr) == "" {
			return r.Err
		}
	}
	msg := strings.TrimSpace(r.Stderr)
	if msg == "" {
		msg = strings.TrimSpace(r.Stdout)
	}
	if msg == "" {
		msg = fmt.Sprintf("exit status %d", r.ExitCode)
	}
	return fmt.Errorf("%s failed (exit %d): %s", r.Command, r.ExitCode, msg)
}

type exitError struct {
	code int
}

func (e *exitError) Error() string { return fmt.Sprintf("exit status %d", e.code) }

// Runner executes external commands. Implementations must never invoke a
// shell ("sh -c"); arguments are always passed separately.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) Result
	LookPath(name string) (string, bool)
}

// ExecRunner executes commands on the host system.
type ExecRunner struct {
	// Verbose, when non-nil, receives debug lines for each executed command.
	Verbose func(format string, args ...any)
}

// NewExec returns an ExecRunner.
func NewExec() *ExecRunner { return &ExecRunner{} }

// Run executes the command using exec.CommandContext with separate arguments.
func (e *ExecRunner) Run(ctx context.Context, name string, args ...string) Result {
	cmdStr := name
	if len(args) > 0 {
		cmdStr = name + " " + strings.Join(args, " ")
	}
	if e.Verbose != nil {
		e.Verbose("command=%s", cmdStr)
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	cmd := osexec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Stdin = os.Stdin

	err := cmd.Start()
	if err != nil {
		return Result{Command: cmdStr, ExitCode: 127, Err: err, Stderr: err.Error()}
	}
	err = cmd.Wait()
	res := Result{Command: cmdStr, Stdout: stdout.String(), Stderr: stderr.String()}
	if err != nil {
		if ee, ok := err.(*osexec.ExitError); ok {
			res.ExitCode = ee.ExitCode()
			res.Err = &exitError{code: ee.ExitCode()}
		} else if ctx.Err() != nil {
			res.Err = fmt.Errorf("cancelled: %w", ctx.Err())
		} else {
			res.Err = err
		}
	}
	if e.Verbose != nil {
		e.Verbose("exit_code=%d", res.ExitCode)
	}
	return res
}

// LookPath resolves an executable in PATH.
func (e *ExecRunner) LookPath(name string) (string, bool) {
	p, err := osexec.LookPath(name)
	return p, err == nil
}

// FakeRunner is a scriptable runner for tests. It never executes anything on
// the host; scripted outputs are matched first against the full command
// line, then against the binary name.
type FakeRunner struct {
	// Scripts maps a binary name (e.g. "nix-store") to the Result to return.
	Scripts map[string]Result
	// ScriptCmds maps a full command line (e.g. "nix-store --gc") to the
	// Result to return. It takes precedence over Scripts.
	ScriptCmds map[string]Result
	// Missing contains binaries LookPath should report as absent.
	Missing map[string]bool
	// Calls records every invocation in order.
	Calls []Call
}

// Call records a single invocation.
type Call struct {
	Name string
	Args []string
}

// CommandString renders the full command line for a Call.
func (c Call) CommandString() string {
	if len(c.Args) == 0 {
		return c.Name
	}
	return c.Name + " " + strings.Join(c.Args, " ")
}

// NewFake creates an empty FakeRunner.
func NewFake() *FakeRunner {
	return &FakeRunner{Scripts: map[string]Result{}, ScriptCmds: map[string]Result{}, Missing: map[string]bool{}}
}

// Script registers the result for a binary name.
func (f *FakeRunner) Script(name string, r Result) { f.Scripts[name] = r }

// ScriptOut registers a successful result with the given stdout.
func (f *FakeRunner) ScriptOut(name, stdout string) {
	f.Scripts[name] = Result{Command: name, Stdout: stdout, ExitCode: 0}
}

// ScriptCmd registers a successful result for a full command line.
func (f *FakeRunner) ScriptCmd(cmd, stdout string) {
	f.ScriptCmds[cmd] = Result{Command: cmd, Stdout: stdout, ExitCode: 0}
}

// ScriptCmdErr registers a failing result for a full command line.
func (f *FakeRunner) ScriptCmdErr(cmd, stderr string, code int) {
	f.ScriptCmds[cmd] = Result{Command: cmd, Stderr: stderr, ExitCode: code, Err: &exitError{code: code}}
}

// ScriptErr registers a failing result with the given stderr and exit code.
func (f *FakeRunner) ScriptErr(name, stderr string, code int) {
	f.Scripts[name] = Result{Command: name, Stderr: stderr, ExitCode: code, Err: &exitError{code: code}}
}

// Run returns the scripted result, if any.
func (f *FakeRunner) Run(_ context.Context, name string, args ...string) Result {
	call := Call{Name: name, Args: args}
	f.Calls = append(f.Calls, call)
	if r, ok := f.ScriptCmds[call.CommandString()]; ok {
		return r
	}
	if r, ok := f.Scripts[name]; ok {
		return r
	}
	return Result{Command: call.CommandString(), ExitCode: 127, Err: &exitError{code: 127}}
}

// LookPath reports scripted availability.
func (f *FakeRunner) LookPath(name string) (string, bool) {
	if f.Missing[name] {
		return "", false
	}
	return "/usr/bin/" + name, true
}

// HasRun reports whether the given command line was invoked.
func (f *FakeRunner) HasRun(cmd string) bool {
	for _, c := range f.Calls {
		if c.CommandString() == cmd {
			return true
		}
	}
	return false
}
