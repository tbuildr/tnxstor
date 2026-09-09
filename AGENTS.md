# AGENTS.md

## Project: tnxstor

Build a Linux CLI/TUI application called **tnxstor** for safely managing and cleaning a local Nix store.

The compiled executable must be named:

```text
tnxs
```

The primary target environment is:

- Fedora / Fedora Atomic / Bazzite
- Standalone Nix installation
- Home Manager
- devenv
- Fish/bash/zsh
- Linux x86_64 initially

The application must **not assume NixOS**.

The user specifically wants to manage Nix storage from Fedora/Bazzite where Nix, Home Manager and devenv are used independently of the host operating system.

---

# 1. Core goals

tnxstor should provide two interfaces:

1. An interactive Bubble Tea TUI.
2. A conventional CLI with flags/subcommands suitable for scripting.

Every destructive operation must support:

```text
--dry-run
```

Dry-run mode must show the commands/actions that would be performed without executing destructive operations.

The TUI and CLI should ultimately use the same internal operation layer rather than implementing separate logic.

---

# 2. Required Go libraries

Use these Charm libraries:

- Bubble Tea
- Bubbles
- Lip Gloss
- Harmonica

Primary dependencies:

```text
github.com/charmbracelet/bubbletea
github.com/charmbracelet/bubbles
github.com/charmbracelet/lipgloss
github.com/charmbracelet/harmonica
```

Use Harmonica for tasteful animation/physics where it improves the TUI, particularly:

- selection movement
- panels
- progress/status transitions
- expandable result sections

Do not add animation merely for decoration.

Keep the interface fast and terminal-friendly.

---

# 3. Project structure

Use a conventional Go project layout.

Suggested structure:

```text
tnxstor/
├── AGENTS.md
├── README.md
├── LICENSE
├── go.mod
├── go.sum
├── cmd/
│   └── tnxs/
│       └── main.go
├── internal/
│   ├── cli/
│   ├── config/
│   ├── nix/
│   ├── devenv/
│   ├── homemanager/
│   ├── operations/
│   ├── runner/
│   ├── system/
│   ├── tui/
│   │   ├── model.go
│   │   ├── update.go
│   │   ├── view.go
│   │   ├── styles.go
│   │   └── components/
│   └── util/
├── docs/
└── tests/
```

Keep the architecture modular.

The TUI must not directly execute shell commands.

The CLI must not contain Nix-specific implementation details.

Use interfaces around command execution so the code can be tested without modifying a real Nix store.

---

# 4. Important safety principle

tnxstor is a **cleanup tool**, therefore safety is more important than convenience.

Never manually delete files from:

```text
/nix/store
```

Never implement cleanup by recursively removing store paths.

Always use the appropriate Nix/devenv commands.

Nix determines reachability through GC roots. Unreachable paths can be collected using Nix's garbage collector. `nix-store --gc --print-dead` provides a preview of dead paths and `nix-store --gc --print-roots` shows roots keeping paths alive.

Reference:

```text
nix-store --gc --print-dead
nix-store --gc --print-roots
nix-store --gc
```

Do not attempt to reproduce Nix's garbage collector logic inside Go.

tnxstor should orchestrate the official tools.

---

# 5. Operations

The application must expose the following operations.

## 5.1 devenv garbage collection

Operation ID:

```text
devenv-gc
```

Execute:

```bash
devenv gc
```

Purpose:

Clean obsolete devenv GC roots and then allow Nix to reclaim store paths no longer referenced by active devenv environments.

This must be a distinct operation from normal Nix garbage collection.

Do not manually delete files under devenv's GC directory.

The official devenv behaviour is that it maintains timestamped GC roots and `devenv gc` cleans obsolete roots before performing Nix garbage collection.

---

# 6. Home Manager generation management

Operation:

```text
hm-generations
```

The application should first detect whether Home Manager is installed.

Check:

```bash
command -v home-manager
```

Display:

```bash
home-manager generations
```

The TUI should show generations in a readable table/list.

For example:

```text
Home Manager generations

CURRENT   DATE                    GENERATION
  ✓       2026-09-09 09:41        128
          2026-09-08 22:17        127
          2026-09-07 18:32        126
          2026-08-31 12:01        125
```

The current generation must never be selected for deletion.

---

# 7. Expire Home Manager generations

Operation:

```text
hm-expire
```

Support an age parameter.

Examples:

```bash
tnxs hm-expire --older-than 30d
tnxs hm-expire --older-than 14d
tnxs hm-expire --older-than 7d
```

Internally use the appropriate Home Manager generation expiration mechanism.

Preferred command:

```bash
home-manager expire-generations "<period>"
```

Example:

```bash
home-manager expire-generations "-30 days"
```

The application should not invent its own interpretation of Home Manager generations.

Before execution:

1. Discover generations.
2. Determine which generations qualify.
3. Display them.
4. Ask for confirmation in interactive mode.
5. Execute only after confirmation.

In dry-run mode, do not execute.

---

# 8. Standard Nix garbage collection

Operation:

```text
gc
```

Execute:

```bash
nix-store --gc
```

This is the normal garbage collection operation.

It should remove Nix store paths that are no longer reachable from GC roots.

The TUI should show:

```text
Running Nix garbage collection...
```

Capture stdout/stderr.

Parse the final freed-byte information if possible.

Display a human-readable result:

```text
Nix GC complete

Freed: 4.72 GiB
```

Do not assume a particular Nix output format is permanently stable.

If parsing fails, display the raw command output rather than failing the operation.

---

# 9. Preview dead store paths

Operation:

```text
dead
```

Execute:

```bash
nix-store --gc --print-dead
```

This must never modify the store.

Parse the result where practical.

Display:

- number of dead paths
- approximate total size where available
- optionally the paths themselves
- command output

Example:

```text
Dead store paths

137 paths
Estimated reclaimable: 6.4 GiB

/nix/store/...
/nix/store/...
...
```

Provide an expandable "Show paths" view in the TUI.

For large stores, do not load an unbounded number of paths into memory unnecessarily.

---

# 10. Show GC roots

Operation:

```text
roots
```

Execute:

```bash
nix-store --gc --print-roots
```

Display the GC roots.

The TUI should allow scrolling.

Where practical, classify roots:

```text
Home Manager
devenv
User profiles
Manual roots
Other
```

Do not assume every root can be perfectly classified.

Unknown roots should be displayed as:

```text
Other
```

This operation is diagnostic only.

It must never modify anything.

---

# 11. Show live store paths

Operation:

```text
live
```

Execute:

```bash
nix-store --gc --print-live
```

This is diagnostic only.

Do not attempt to manually calculate reachability.

---

# 12. Aggressive garbage collection

Operation:

```text
gc-old
```

Equivalent in spirit to:

```bash
nix-collect-garbage -d
```

This removes old profile generations before performing garbage collection.

This operation is potentially destructive from a rollback perspective.

Therefore:

### CLI

Require explicit confirmation unless:

```text
--yes
```

is supplied.

### TUI

Display a warning:

```text
This removes old Nix profile generations.

You may lose rollback points.

Continue?
```

Do not hide this behind the normal "GC" option.

The application should make the distinction obvious.

---

# 13. Nix store optimisation

Operation:

```text
optimise
```

Execute:

```bash
nix-store --optimise
```

Do not confuse optimisation with garbage collection.

Optimisation deduplicates identical files in the Nix store by hard-linking identical content.

Display:

```text
Optimising Nix store...

Before: 18.7 GiB
After:  14.2 GiB
Saved:   4.5 GiB
```

Where accurate before/after measurements can be obtained.

If they cannot, display the Nix command's output.

This operation can be expensive and should have a progress/running state in the TUI.

---

# 14. Store information

Provide a read-only overview screen.

Operation:

```text
status
```

Show:

```text
Nix version
Nix store path
Nix installation mode
NIX_REMOTE
Nix daemon availability
Home Manager availability
devenv availability
Nix store size
```

Useful commands include:

```bash
nix --version
nix store info
command -v home-manager
command -v devenv
```

Also inspect:

```bash
echo "$NIX_REMOTE"
```

Do not expose secrets from environment variables.

---

# 15. Detect multi-user/daemon Nix

The application must detect whether Nix uses the daemon.

Potentially inspect:

```bash
NIX_REMOTE
```

and:

```bash
nix show-config
```

The application must not assume the user has permission to directly modify `/nix/store`.

If a command requires elevated privileges, report that clearly.

Do not automatically invoke:

```bash
sudo
```

unless explicitly designed and documented as an optional future feature.

The first version should preferably execute commands as the invoking user and let Nix handle its configured permissions.

---

# 16. CLI design

The binary is:

```text
tnxs
```

Provide:

```bash
tnxs --help
```

Suggested interface:

```text
tnxs
tnxs tui

tnxs status

tnxs devenv-gc

tnxs hm-generations
tnxs hm-expire --older-than 30d

tnxs dead
tnxs roots
tnxs live

tnxs gc
tnxs gc-old

tnxs optimise
```

Global options:

```text
--dry-run
--yes
--verbose
--quiet
--no-color
```

Potential global flags should be available consistently.

---

# 17. Dry-run behaviour

`--dry-run` is a major feature.

Examples:

```bash
tnxs gc --dry-run
tnxs devenv-gc --dry-run
tnxs hm-expire --older-than 30d --dry-run
tnxs gc-old --dry-run
tnxs optimise --dry-run
```

Dry-run must show exactly what would be executed.

Example:

```text
DRY RUN

Would execute:

  devenv gc

No changes have been made.
```

For Home Manager:

```text
DRY RUN

Would expire Home Manager generations older than 30 days.

Eligible generations:

  121
  122
  123
  124

Would execute:

  home-manager expire-generations "-30 days"

No changes have been made.
```

For Nix GC:

```text
DRY RUN

Would execute:

  nix-store --gc

No changes have been made.
```

For optimise:

```text
DRY RUN

Would execute:

  nix-store --optimise

No changes have been made.
```

Do not fake results in dry-run mode.

For example, do not report a predicted number of freed bytes unless explicitly calculated from a read-only command.

---

# 18. TUI

Running:

```bash
tnxs
```

with no arguments should launch the interactive TUI.

Alternatively:

```bash
tnxs tui
```

should launch it explicitly.

Use Bubble Tea.

Suggested main screen:

```text
┌──────────────────────────────────────────────────────────────┐
│ tnxstor                                      Nix Store        │
├──────────────────────────────────────────────────────────────┤
│                                                              │
│  Nix Store                                                   │
│                                                              │
│  > Status                                                    │
│    Preview dead paths                                        │
│    Garbage collect                                          │
│    Aggressive GC                                             │
│    Optimise store                                            │
│    Show GC roots                                             │
│    Show live paths                                           │
│                                                              │
│  Home Manager                                                │
│                                                              │
│    List generations                                          │
│    Expire old generations                                    │
│                                                              │
│  devenv                                                      │
│                                                              │
│    Garbage collect                                           │
│                                                              │
├──────────────────────────────────────────────────────────────┤
│ ↑↓ navigate   Enter select   q quit   ? help                │
└──────────────────────────────────────────────────────────────┘
```

Use Lip Gloss for styling.

Keep the design clean and restrained.

Do not copy the exact appearance of another Charm application.

---

# 19. TUI operation flow

For every potentially destructive action:

```text
Select operation
       ↓
Show description
       ↓
Show command(s)
       ↓
Show detected impact where possible
       ↓
Confirm
       ↓
Execute
       ↓
Show output
       ↓
Show result
```

For example:

```text
Garbage Collection

This will remove Nix store paths that are no longer
reachable from a GC root.

Command:

  nix-store --gc

Continue?

  > Yes
    No
```

After execution:

```text
Garbage Collection Complete

Freed: 3.84 GiB

Press Enter to continue.
```

---

# 20. TUI keyboard controls

Support:

```text
↑ / k       Move up
↓ / j       Move down
Enter       Select
Esc         Back
q           Quit
?           Help
r           Refresh
d           Toggle dry-run
y           Confirm
n           Cancel
```

Do not make `d` unexpectedly execute anything.

A dry-run state should be visually obvious.

Example:

```text
[DRY RUN]
```

in the header.

---

# 21. Command execution abstraction

Create an abstraction similar to:

```go
type Runner interface {
    Run(ctx context.Context, name string, args ...string) Result
}
```

Where `Result` contains at least:

```go
type Result struct {
    Command  string
    Stdout   string
    Stderr   string
    ExitCode int
    Err      error
}
```

Also provide:

```go
type DryRunRunner struct {
    // ...
}
```

However, dry-run should preferably be a higher-level application concern so that read-only discovery commands can still execute when useful.

For example:

```bash
tnxs hm-expire --older-than 30d --dry-run
```

may legitimately execute:

```bash
home-manager generations
```

to determine what would be affected, while refusing to execute:

```bash
home-manager expire-generations "-30 days"
```

Document this distinction clearly.

---

# 22. Never use shell=True style execution

Do not construct commands such as:

```bash
sh -c "..."
```

for normal operations.

Prefer:

```go
exec.CommandContext(ctx, "nix-store", "--gc")
```

Arguments must be passed separately.

This is especially important for:

```text
--older-than
```

and any future user-supplied values.

---

# 23. Context and cancellation

Every external process must use:

```go
exec.CommandContext
```

or equivalent.

The TUI must allow cancellation of long-running operations where practical.

For example:

```text
Running nix-store --optimise...

[████████░░░░░░░░░░░░]

Press Esc to cancel
```

If the underlying command does not provide useful progress, don't fabricate a percentage.

Use an indeterminate spinner/progress indicator instead.

---

# 24. Output handling

Do not discard command output.

Capture stdout and stderr separately.

The TUI should have an output viewport using Bubbles.

Support:

- scrolling
- text wrapping
- copying where practical
- clear distinction between stdout/stderr

CLI mode should behave like a conventional Unix command.

Do not force TUI formatting into non-interactive CLI output.

---

# 25. Non-interactive detection

Detect whether stdout is a TTY.

If stdin/stdout are not interactive:

```bash
tnxs gc
```

should behave as a normal CLI command.

It must not launch Bubble Tea.

The explicit:

```bash
tnxs tui
```

may fail with a helpful error when no interactive terminal exists.

---

# 26. Exit codes

Use sensible exit codes.

At minimum:

```text
0   success
1   operation failed
2   invalid command/arguments
3   required dependency missing
4   user cancelled
```

Document these.

---

# 27. Dependency detection

Before executing an operation, verify required tools.

Examples:

```text
nix-store
nix-collect-garbage
nix
home-manager
devenv
```

If missing:

```text
devenv is not installed or not available in PATH.

Operation cannot continue.
```

Do not silently skip the operation.

---

# 28. Nix command preference

Prefer modern commands where appropriate, but retain compatibility with the user's installed Nix version.

Do not unnecessarily depend on experimental Nix CLI features.

For garbage collection specifically, the initial implementation should use:

```bash
nix-store --gc
nix-store --gc --print-dead
nix-store --gc --print-roots
nix-store --gc --print-live
nix-store --optimise
```

because these commands provide the precise low-level operations needed.

---

# 29. Do not treat NixOS specially

tnxstor is not a NixOS management tool.

Do not add:

```text
nixos-rebuild
```

or NixOS system-generation management.

The host operating system should be detected and displayed only for informational purposes.

For example:

```text
Host OS: Fedora Atomic
Nix: standalone
Home Manager: detected
devenv: detected
```

---

# 30. Storage measurements

Provide a utility for measuring `/nix/store`.

Possible approach:

```bash
du -sh /nix/store
```

but do not assume `/nix/store` is writable.

The store location should ideally be obtained from Nix rather than hard-coded where practical.

Do not scan the entire store repeatedly for every screen refresh.

Cache measurements until the user explicitly refreshes.

---

# 31. TUI status dashboard

The status screen should ideally show:

```text
tnxstor

Nix
────────────────────────────────────
Version       2.x.x
Store         /nix/store
Store size    21.4 GiB
Daemon        enabled

Home Manager
────────────────────────────────────
Installed     yes
Generation    128
Generations   17

devenv
────────────────────────────────────
Installed     yes
GC roots      detected

Potential cleanup
────────────────────────────────────
Dead paths    183
```

The last item may require executing:

```bash
nix-store --gc --print-dead
```

Do not automatically run expensive scans every time the UI renders.

---

# 32. Interactive cleanup workflow

Add a convenient:

```text
Clean up
```

workflow.

It should present:

```text
Cleanup

[ ] devenv garbage collection
[ ] Expire Home Manager generations older than 30 days
[ ] Nix garbage collection
[ ] Optimise Nix store
```

Allow the user to select multiple operations.

Default selections should be conservative.

Recommended defaults:

```text
[x] devenv gc
[x] Home Manager generations > 30 days
[x] nix-store --gc
[ ] nix-collect-garbage -d
[ ] nix-store --optimise
```

Before running:

```text
Cleanup plan

1. devenv gc
2. home-manager expire-generations "-30 days"
3. nix-store --gc

Continue?
```

This is one of the key UX features of the application.

---

# 33. Operation ordering

When running the cleanup workflow, use this order:

```text
1. devenv gc
2. Home Manager generation cleanup
3. Nix garbage collection
4. optional Nix store optimisation
```

Reasoning:

- devenv maintains GC roots.
- Home Manager generations can themselves retain store paths.
- Nix GC should happen after those roots have been cleaned.
- optimisation is separate from garbage collection and should generally happen last.

Do not automatically perform aggressive GC unless explicitly selected.

---

# 34. Dry-run cleanup workflow

For:

```bash
tnxs clean --dry-run
```

display something similar to:

```text
Cleanup plan

1. devenv gc
2. expire Home Manager generations older than 30 days
3. nix-store --gc

No destructive operations will be executed.

Commands:

  devenv gc
  home-manager expire-generations "-30 days"
  nix-store --gc
```

If `devenv gc` itself is difficult to predict because it performs both root maintenance and GC, clearly label it:

```text
Would execute:
  devenv gc
```

Do not claim an exact amount of space will be freed unless it can be safely determined.

---

# 35. CLI clean command

Provide:

```bash
tnxs clean
```

Options:

```text
--dry-run
--yes
--hm-older-than 30d
--devenv
--home-manager
--gc
--optimise
--aggressive
```

Examples:

```bash
tnxs clean --dry-run

tnxs clean --yes

tnxs clean --hm-older-than 14d --yes

tnxs clean --devenv --gc --yes

tnxs clean --home-manager --gc --optimise --yes
```

Default:

```text
devenv GC
Home Manager old generations
normal Nix GC
```

Do not include aggressive GC by default.

---

# 36. Logging

Provide optional verbose logging:

```bash
tnxs --verbose gc
```

Use structured internal logging rather than scattering `fmt.Println` throughout the code.

Normal CLI output should remain concise.

Verbose mode can display:

```text
DEBUG command=nix-store args=["--gc"]
DEBUG exit_code=0
```

Never log:

- secrets
- environment variables indiscriminately
- credentials
- tokens

---

# 37. Configuration

The first version should work without configuration.

Optional configuration can later be added.

Potential future location:

```text
~/.config/tnxstor/config.toml
```

Do not implement configuration merely for the sake of having it.

The initial application should have sensible defaults.

---

# 38. Testing

Testing is important because this application executes destructive system commands.

Implement unit tests around:

- command construction
- argument validation
- dry-run behaviour
- Home Manager generation parsing
- Nix GC output parsing
- byte-size parsing
- command result handling
- operation ordering
- dependency detection
- exit codes

Use fake/mock runners.

Tests must never execute destructive Nix commands against the developer's real store.

---

# 39. Integration testing

Where practical, provide an integration-test mode using a fake command directory.

For example:

```text
tests/fixtures/bin/nix-store
tests/fixtures/bin/home-manager
tests/fixtures/bin/devenv
```

The fake executables can emit representative outputs.

This permits testing the complete operation pipeline without touching `/nix/store`.

---

# 40. Security requirements

Do not:

- run commands through `sh -c`
- interpolate arbitrary user input into shell strings
- automatically use sudo
- modify `/nix/store` directly
- delete GC roots manually
- delete Home Manager generations without confirmation in interactive mode
- hide destructive operations behind innocuous names

Use least privilege.

The application should normally run as the regular user.

---

# 41. CLI help

`tnxs --help` should be excellent.

Example conceptual output:

```text
tnxs — Nix store maintenance for standalone Nix systems

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
  hm-generations  Show Home Manager generations
  hm-expire       Expire old Home Manager generations

Global options:
  --dry-run       Show actions without performing destructive operations
  --yes           Skip confirmation prompts
  --verbose       Enable verbose output
  --no-color      Disable colour
  -h, --help      Show help
```

---

# 42. README

Create a README explaining:

- what tnxstor is
- why it exists
- supported environments
- installation
- CLI usage
- TUI usage
- dry-run mode
- safety model
- Home Manager handling
- devenv handling
- Nix GC behaviour
- optimisation
- development
- testing

Explicitly state that:

```text
tnxstor does not delete files directly from /nix/store.
It delegates store lifecycle management to Nix.
```

---

# 43. Build

The main build should produce:

```text
tnxs
```

For example:

```bash
go build -o tnxs ./cmd/tnxs
```

Also provide:

```bash
go test ./...
```

and ideally:

```bash
go vet ./...
```

Use a Makefile or Justfile only if it genuinely improves developer ergonomics.

---

# 44. Cross compilation

Initially target:

```text
linux/amd64
```

Optionally support:

```text
linux/arm64
```

Do not add unnecessary platform-specific code.

The application should gracefully report unsupported platforms rather than making dangerous assumptions.

---

# 45. Shell completion

If practical, add shell completion for:

```text
bash
fish
zsh
```

This is particularly useful for:

```text
tnxs clean --hm-older-than ...
```

Do not make shell completion a blocker for the initial implementation.

---

# 46. Implementation order

Build the project incrementally.

### Phase 1

Create:

- Go module
- CLI
- runner abstraction
- `status`
- `gc`
- `dead`
- `roots`
- `live`
- `optimise`

### Phase 2

Add:

- Home Manager detection
- generation listing
- generation expiry
- devenv GC

### Phase 3

Add:

- `clean`
- dry-run
- confirmation handling
- operation plans

### Phase 4

Build the Bubble Tea TUI.

### Phase 5

Add:

- animations
- polished styles
- progress states
- help screen
- responsive layout

### Phase 6

Add comprehensive tests and integration fixtures.

### Phase 7

Polish README, CLI help, error messages and packaging.

---

# 47. Definition of done

The project is complete when all of the following work:

```bash
tnxs
```

launches the TUI.

```bash
tnxs status
```

shows useful system information.

```bash
tnxs dead
```

shows what Nix considers dead.

```bash
tnxs roots
```

shows GC roots.

```bash
tnxs live
```

shows live paths.

```bash
tnxs gc --dry-run
```

does not modify anything.

```bash
tnxs gc
```

performs normal Nix GC.

```bash
tnxs gc-old --dry-run
```

clearly explains that old generations would be removed.

```bash
tnxs devenv-gc
```

runs devenv GC.

```bash
tnxs hm-generations
```

shows Home Manager generations.

```bash
tnxs hm-expire --older-than 30d --dry-run
```

shows which generations would be expired without deleting them.

```bash
tnxs optimise --dry-run
```

shows the optimisation command without running it.

```bash
tnxs clean --dry-run
```

shows a complete cleanup plan.

```bash
tnxs clean --yes
```

performs the configured safe cleanup without interactive confirmation.

All destructive commands must be tested through mocked/fake runners.

No test may accidentally modify the developer's real Nix store.

---

# 48. Design philosophy

tnxstor should feel like a **native terminal utility**, not a GUI forced into a terminal.

Priorities:

1. Safety
2. Correctness
3. Transparency
4. Good CLI ergonomics
5. Excellent TUI
6. Speed
7. Visual polish

The user should always understand:

```text
What is being inspected?
What is going to happen?
Which command will execute?
Is anything destructive?
How much might be reclaimed?
What happened afterwards?
```

When uncertain, prefer showing information rather than taking action.

Do not invent Nix behaviour.

Delegate store reachability and deletion decisions to Nix itself.