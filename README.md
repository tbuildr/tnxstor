# tnxstor

**tnxstor** is a Linux CLI/TUI application for safely managing and cleaning a
local Nix store on systems where Nix is installed independently of the host
OS — for example Fedora, Fedora Atomic or Bazzite with standalone Nix, Home
Manager and devenv.

The compiled executable is called **`tnxs`**.

> **Safety model**
>
> tnxstor does not delete files directly from `/nix/store`.
> It delegates store lifecycle management to Nix.
>
> tnxstor never runs commands through a shell (`sh -c`), never invokes
> `sudo` automatically, and never invents Nix behaviour. Every destructive
> operation supports `--dry-run`.

---

## Why tnxstor exists

On non-NixOS systems it is easy to accumulate large amounts of dead store
paths, obsolete devenv GC roots and stale Home Manager generations. Nix
already provides precise, safe tooling for reclaiming this space — GC roots
determine reachability, and the Nix garbage collector only removes
unreachable paths. tnxstor orchestrates those official tools in one place,
with previews, confirmations and a dry-run mode, instead of asking you to
remember the low-level incantations.

---

## Supported environments

- Fedora / Fedora Atomic / Bazzite (and other Linux x86_64 hosts)
- Standalone Nix installation (single- or multi-user/daemon)
- Home Manager (standalone)
- devenv
- bash / zsh / fish (tnxstor does not depend on your shell)

tnxstor is **not** a NixOS tool: it never touches `nixos-rebuild` or system
generations. The host OS is detected and shown for information only.

---

## Installation

```bash
go build -o tnxs ./cmd/tnxs
sudo install -m755 tnxs /usr/local/bin/tnxs   # optional
```

Or simply run it from the repository root after building.

`make build` (or `CGO_ENABLED=0 go build -o tnxs ./cmd/tnxs`) produces a
**fully static** binary with no C dependencies, so it runs on any Linux
host regardless of its C library — no dynamic interpreter or linker is
required at runtime.

---

## CLI usage

```text
tnxs                       launches the interactive TUI
tnxs tui                   launches the interactive TUI explicitly
tnxs status                Show Nix/Home Manager/devenv status
tnxs clean                 Run a configurable cleanup
tnxs gc                    Garbage collect unreachable store paths
tnxs gc-old                Remove old generations and garbage collect
tnxs dead                  Show unreachable store paths
tnxs live                  Show live store paths
tnxs roots                 Show garbage collection roots
tnxs optimise              Optimise/deduplicate the Nix store
tnxs devenv-gc             Clean devenv GC roots
tnxs devenv-update         Update devenv inputs and lock files
tnxs devenv-info           Show devenv environment info (read-only)
tnxs hm-generations        Show Home Manager generations
tnxs hm-expire             Expire old Home Manager generations
```

Global options (available on every command):

```text
--dry-run    Show actions without performing destructive operations
--yes        Skip confirmation prompts
--verbose    Enable verbose output
--quiet      Suppress informational output
--no-color   Disable colour
--version    Show version
-h, --help   Show help
```

### Examples

```bash
tnxs status
tnxs dead                        # preview what Nix considers unreachable
tnxs roots                       # what is keeping paths alive
tnxs gc --dry-run                # show what would be executed
tnxs gc                          # normal Nix garbage collection
tnxs gc-old --dry-run            # explains that old generations would go
tnxs hm-expire --older-than 30d --dry-run
tnxs hm-expire --older-than 30d
tnxs clean --dry-run             # complete cleanup plan, nothing executed
tnxs clean --yes                 # conservative cleanup, no prompts
tnxs clean --home-manager --gc --optimise --yes
```

### `tnxs clean`

`clean` runs several operations in a safe order. Default selection
(conservative):

1. `devenv gc`
2. `home-manager expire-generations "-30 days"`
3. `nix-store --gc`

Options:

```text
--dry-run
--yes
--hm-older-than 30d
--devenv          include devenv GC
--home-manager    include Home Manager generation expiry
--gc              include Nix GC
--optimise        include store optimisation
--aggressive      include aggressive GC (nix-collect-garbage -d)
```

If any of `--devenv/--home-manager/--gc/--optimise/--aggressive` is given,
only the selected steps run. Aggressive GC is never included by default.

### Exit codes

```text
0   success
1   operation failed
2   invalid command/arguments
3   required dependency missing
4   user cancelled
```

---

## TUI usage

Running `tnxs` (or `tnxs tui`) opens an interactive interface with the same
operations behind the scenes: status dashboard, dead-path preview, GC,
aggressive GC, optimisation, roots/live listing, Home Manager generations
and expiry, devenv GC — plus a **Clean up** workflow where you can select
multiple operations and review the plan before running it.

The `Status` item stands alone at the top of the menu, outside the Nix
Store group. The header shows a context label that follows the screen and
menu section you are on, and `⚡ tnxstor` badges the app version. Dry-run
outcomes are presented uniformly for every task (a yellow `[DRY RUN]`
badge with the plan); when dry-run is enabled, destructive tasks skip
their confirmation screen since nothing will execute.

Keyboard controls:

```text
↑ / k       Move up
↓ / j       Move down
Enter       Select / expand output
Esc         Back
q           Quit
?           Help
r           Refresh
d           Toggle dry-run (shown as [DRY RUN] in the header)
y           Confirm
n           Cancel
Space       Toggle selection (cleanup workflow)
```

While an operation runs you can press `Esc` to cancel. Long-running
operations show an indeterminate spinner — tnxstor never fabricates
progress percentages.

---

## Dry-run mode

`--dry-run` is a first-class feature of every destructive operation. It
prints exactly what would be executed and makes no changes:

```text
DRY RUN

Garbage collection

Would execute:

  nix-store --gc

No changes have been made.
```

`--dry-run` gates **every** operation, not just destructive ones. Read-only
diagnostics such as `tnxs dead`, `tnxs roots`, `tnxs live`,
`tnxs hm-generations` and `tnxs devenv-info` do not run their commands
either — they show exactly what they would run. The only commands that
still run are read-only *discovery* steps required to compute impact: for
example `tnxs hm-expire --older-than 30d --dry-run` executes
`home-manager generations` (read-only) so it can show you which
generations would be affected, while refusing to execute
`home-manager expire-generations "-30 days"`. tnxstor never fakes results:
it will not claim a number of freed bytes it has not measured.

---

## Safety model

- Nothing is ever deleted directly from `/nix/store`.
- Store reachability and deletion decisions belong to Nix's garbage
  collector; tnxstor orchestrates `nix-store`, `nix-collect-garbage`,
  `home-manager` and `devenv`.
- Commands are always executed with separate arguments (never through
  `sh -c`), so user-supplied values such as `--older-than` can never be
  interpolated into a shell string.
- Destructive operations require explicit confirmation in interactive mode
  (or `--yes`); non-interactive runs without `--yes` are refused.
- The current Home Manager generation can never be selected for expiry.
- `gc-old` (aggressive GC) is always clearly separated from normal GC and
  warns that you may lose rollback points.
- No automatic `sudo`: commands run as the invoking user and Nix handles
  its own permissions. If elevated rights are needed, Nix's error is
  reported clearly.

---

## Home Manager handling

Generations are listed with `home-manager generations` and displayed in a
table. Expiry uses the official mechanism:

```bash
home-manager expire-generations "-30 days"
```

tnxstor converts periods like `30d`, `12w`, `6m`, `1y` into the GNU date
expression Home Manager expects, shows you exactly which generations
qualify before doing anything, and never expires the current generation.

---

## devenv handling

devenv maintains timestamped GC roots itself. tnxstor runs the official
`devenv gc`, which cleans obsolete roots before performing Nix garbage
collection. It never deletes anything from devenv's GC directory directly,
and `devenv-gc` is always a distinct operation from plain Nix GC.

Two more devenv operations are provided for the **current project
directory**:

- `tnxs devenv-update` runs `devenv update`, refreshing the project's
  inputs and lock files. It honours `--dry-run`.
- `tnxs devenv-info` runs `devenv info`, a read-only overview of the
  environment.

---

## Nix GC behaviour

- `tnxs gc` runs `nix-store --gc`, which removes store paths no longer
  reachable from a GC root.
- `tnxs dead` previews with `nix-store --gc --print-dead` (read-only).
- `tnxs roots` shows live GC roots with `nix-store --gc --print-roots`
  and best-effort classification (Home Manager, devenv, User profiles,
  Manual roots, Other).
- `tnxs live` lists reachable paths with `nix-store --gc --print-live`.
- `tnxs gc-old` runs `nix-collect-garbage -d` after an explicit warning.

tnxstor does not reproduce the garbage collector's logic in Go; Nix decides
what is dead. Output parsing is best-effort and falls back to showing the
raw command output.

---

## Optimisation

`tnxs optimise` runs `nix-store --optimise`, which deduplicates identical
files in the store by hard-linking identical content. This is separate from
garbage collection. Where before/after measurements can be taken (via
read-only `du` of the store), tnxstor shows Before/After/Saved; otherwise
it shows the command output. This operation can take a while on large
stores and can be cancelled in the TUI.

---

## Development

```bash
go build ./...          # compile everything
go build -o tnxs ./cmd/tnxs
go test ./...           # unit + integration tests
go vet ./...            # static checks
```

The test suite never touches a real Nix store:

- Unit tests use fake runners (`internal/runner.FakeRunner`) and cover
  command construction, argument validation, dry-run behaviour, Home
  Manager generation parsing, Nix GC output parsing, byte-size parsing and
  operation ordering.
- Integration tests (`tests/`) build the real binary and run it against
  fake executables in `tests/fixtures/bin/` that emit representative
  outputs and log their invocations, so destructive calls can be asserted
  to have (or have not) happened.

Cross-compilation targets `linux/amd64` initially; `linux/arm64` works via
`GOARCH=arm64 go build`. Unsupported platforms are reported gracefully.

---

## License

MIT — see [LICENSE](LICENSE).

<!-- agent-sandbox:begin (managed by sandbox-note.sh, edits are overwritten) -->
## Agent sandboxes

**If `devenv` is not on PATH, you are in an agent sandbox** (a tmakr pod or
a local `agents-sandbox` container). These instructions then take precedence
over anything elsewhere in this file: devenv, direnv, `devenv shell` /
`devenv up`, push and pull-request steps.

### Git: commit, never push

- Commit your work on a branch. **Never push.** In a tmakr pod the GitHub
  token is read-only, so `git push` fails with 403 by design. Don't retry it,
  change remotes, switch to SSH, or look for other credentials.
- Tom fetches your commits from the sandbox, reviews them and pushes from his
  own machine. Uncommitted changes and stashes can't be fetched: commit before
  you say a task is done.
- Where this file says to open a PR, or to say or justify something in the
  PR, put it in your commit message body instead: Tom opens the PR from your
  commits.
- Start each piece of work on a new branch off `main`. Keep `main` matching
  `origin/main`; don't commit to it.
- Don't open pull requests or use `gh`. Once you've said a branch is done,
  don't rewrite its history (no rebase, amend or force): Tom may have fetched
  it already.

### Tools: no devenv

`devenv.nix` is for local development only.

- Don't install or bootstrap Nix, devenv or direnv. Use the tools already on
  PATH: Python 3.14 with `uv` (`uv sync`, `uv run …`; Python downloads are
  disabled), Node 24 with pnpm/bun, Go, Rust, C/C++ toolchains, ruff, eslint,
  Tailwind v4, git.
- Postgres: connection settings come from the environment. In a tmakr pod,
  if `PGHOST` is unset, run `set -a; . /etc/agent-db/database.env; set +a`.
  Don't start your own database server.
- Services that `devenv up` would start aren't running. Start what you need
  in the foreground (for example `uv run manage.py runserver`), or say what
  is missing.
- If something in `devenv.nix` has no equivalent here, say so and stop rather
  than working around it.
<!-- agent-sandbox:end -->
