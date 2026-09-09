package operations

import (
	"context"
	"strings"

	"tnxstor/internal/homemanager"
	"tnxstor/internal/nix"
	"tnxstor/internal/system"
)

// StatusInfo is the read-only overview behind `tnxs status` and the TUI
// status dashboard.
type StatusInfo struct {
	OS         system.Info
	Deps       system.Deps
	NixVersion string
	StorePath  string
	NixRemote  string
	Daemon     bool
	DaemonNote string
	StoreSize  uint64
	StoreKnown bool
	HMCurrent  int
	HMCount    int
	HMHasData  bool
	DevenvRoot bool
	DeadCount  int
	DeadKnown  bool
	DeadBytes  uint64
}

// Status gathers system information. Read-only; measurements are taken once
// per invocation (callers should cache across UI refreshes).
func (e *Executor) Status(ctx context.Context) (StatusInfo, error) {
	var st StatusInfo
	st.OS = system.DetectOS()
	st.Deps = system.DetectDeps(e.R)
	st.NixRemote = system.NixRemote()

	if res := e.R.Run(ctx, "nix", "--version"); res.OK() {
		st.NixVersion = strings.TrimSpace(res.Stdout)
	}

	if st.Deps.NixStore {
		res := e.R.Run(ctx, "nix-store", "--gc", "--print-dead")
		if res.OK() && res.Stdout != "" {
			if i := strings.Index(res.Stdout, "/store/"); i > 0 {
				st.StorePath = res.Stdout[:i+len("/store")]
			}
		}
	}
	if st.StorePath == "" {
		st.StorePath = "/nix/store"
	}

	// Daemon detection: empty NIX_REMOTE means local store access; a
	// "daemon" value or unix socket indicates daemon-mediated access.
	switch st.NixRemote {
	case "", "local":
		st.Daemon = false
		st.DaemonNote = "local store (NIX_REMOTE empty)"
	case "daemon":
		st.Daemon = true
		st.DaemonNote = "NIX_REMOTE=daemon"
	default:
		st.Daemon = strings.Contains(st.NixRemote, "unix://") || strings.Contains(st.NixRemote, "daemon")
		st.DaemonNote = "NIX_REMOTE=" + st.NixRemote
	}
	// A running nix-daemon service also implies multi-user mode.
	if !st.Daemon {
		if res := e.R.Run(ctx, "systemctl", "is-active", "nix-daemon"); res.OK() {
			st.Daemon = true
			st.DaemonNote = "nix-daemon service active"
		}
	}

	if st.Deps.NixStore && st.Deps.Du {
		if n, ok := nix.StoreSize(ctx, e.R, st.StorePath); ok {
			st.StoreSize, st.StoreKnown = n, true
		}
	}

	if st.Deps.HomeManager {
		if res := e.R.Run(ctx, "home-manager", "generations"); res.OK() {
			gens := homemanager.ParseGenerations(res.Stdout)
			if len(gens) > 0 {
				st.HMHasData = true
				st.HMCount = len(gens)
				for _, g := range gens {
					if g.Current && g.Number > st.HMCurrent {
						st.HMCurrent = g.Number
					}
				}
				if st.HMCurrent == 0 {
					st.HMCurrent = gens[0].Number
				}
			}
		}
	}

	if st.Deps.Devenv {
		if res := e.R.Run(ctx, "nix-store", "--gc", "--print-roots"); res.OK() {
			for _, root := range nix.ParseRoots(res.Stdout) {
				if nix.ClassifyRoot(root.Link) == "devenv" {
					st.DevenvRoot = true
					break
				}
			}
		}
	}

	return st, nil
}
