// Package system reports host and environment information. tnxstor is not a
// NixOS tool; the host OS is displayed for informational purposes only.
package system

import (
	"context"
	"os"
	"strings"

	"tnxstor/internal/runner"
)

// Info describes the host environment relevant to tnxstor.
type Info struct {
	OSName   string // e.g. "Fedora Linux"
	OSPretty string // e.g. "Fedora Linux 42 (Atomic)"
	NixOS    bool
}

// DetectOS reads /etc/os-release (informational only).
func DetectOS() Info {
	info := Info{}
	data, err := os.ReadFile("/etc/os-release")
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			switch {
			case strings.HasPrefix(line, "PRETTY_NAME="):
				info.OSPretty = strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), `"`)
			case strings.HasPrefix(line, "NAME="):
				info.OSName = strings.Trim(strings.TrimPrefix(line, "NAME="), `"`)
			}
		}
	}
	if info.OSPretty == "" {
		info.OSPretty = info.OSName
	}
	info.NixOS = strings.Contains(strings.ToLower(info.OSName), "nixos")
	return info
}

// Deps records which external tools are available.
type Deps struct {
	Nix               bool
	NixStore          bool
	NixCollectGarbage bool
	HomeManager       bool
	Devenv            bool
	Du                bool
}

// DetectDeps resolves each required binary in PATH.
func DetectDeps(r runner.Runner) Deps {
	_, deps := r.LookPath("nix")
	_, ns := r.LookPath("nix-store")
	_, ncd := r.LookPath("nix-collect-garbage")
	_, hm := r.LookPath("home-manager")
	_, dv := r.LookPath("devenv")
	_, du := r.LookPath("du")
	return Deps{Nix: deps, NixStore: ns, NixCollectGarbage: ncd, HomeManager: hm, Devenv: dv, Du: du}
}

// NixRemote returns $NIX_REMOTE (may be empty).
func NixRemote() string { return os.Getenv("NIX_REMOTE") }

// Env gathers cheap environment facts used by status output.
func Env(r runner.Runner, ctx context.Context) map[string]string {
	out := map[string]string{
		"NIX_REMOTE": NixRemote(),
	}
	if res := r.Run(ctx, "nix", "--version"); res.OK() {
		out["nix_version"] = strings.TrimSpace(res.Stdout)
	}
	return out
}
