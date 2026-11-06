package nix

import "testing"

func TestParseGCFreed(t *testing.T) {
	var gib float64 = 1024 * 1024 * 1024
	want := uint64(4.72 * gib)
	cases := []struct {
		name string
		out  string
		want uint64
		ok   bool
	}{
		{"standard", "1376 store paths deleted, 4.72 GiB freed\n", want, true},
		{"nothing", "1376 store paths deleted, 0.00 MiB freed\n", 0, true},
		{"empty", "", 0, false},
		{"garbage", "hello world\n", 0, false},
	}
	for _, c := range cases {
		got, ok := ParseGCFreed(c.out)
		if ok != c.ok {
			t.Errorf("%s: ok = %v, want %v", c.name, ok, c.ok)
			continue
		}
		if ok && c.want > 0 {
			if got < c.want-1024*1024 || got > c.want+1024*1024 {
				t.Errorf("%s: got %d, want ~%d", c.name, got, c.want)
			}
		}
	}
}

func TestParseDeadPaths(t *testing.T) {
	out := `/nix/store/aaaa-env
/nix/store/bbbb-env
/nix/store/cccc-env
`
	paths, _, _ := ParseDeadPaths(out)
	if len(paths) != 3 {
		t.Fatalf("got %d paths, want 3: %v", len(paths), paths)
	}
	if paths[0] != "/nix/store/aaaa-env" {
		t.Errorf("first path = %q", paths[0])
	}
}

func TestParseRoots(t *testing.T) {
	out := `/nix/var/nix/profiles/per-user/root/profile -> /nix/store/aaaa-env
/nix/var/nix/profiles/default -> /nix/store/bbbb-env
/home/alice/.local/state/nix/profiles/home-manager -> /nix/store/cccc-env
/home/alice/devenv/.devenv -> /nix/store/dddd-env
`
	roots := ParseRoots(out)
	if len(roots) != 4 {
		t.Fatalf("got %d roots, want 4", len(roots))
	}
	if roots[0].Link != "/home/alice/.local/state/nix/profiles/home-manager" {
		t.Errorf("sorting broken: %q", roots[0].Link)
	}
	if ClassifyRoot(roots[0].Link) != "Home Manager" {
		t.Errorf("HM root classified as %q", ClassifyRoot(roots[0].Link))
	}
	if ClassifyRoot("/home/alice/devenv/.devenv") != "devenv" {
		t.Error("devenv classification failed")
	}
	if ClassifyRoot("/nix/var/nix/profiles/default") != "User profiles" {
		t.Error("profile classification failed")
	}
	if ClassifyRoot("/somewhere/random") != "Other" {
		t.Error("unknown roots must classify as Other")
	}
}

func TestParseDeadPathsEmpty(t *testing.T) {
	paths, _, _ := ParseDeadPaths("nothing to delete\n0 paths\n")
	if len(paths) != 0 {
		t.Errorf("expected 0 paths, got %d", len(paths))
	}
}
