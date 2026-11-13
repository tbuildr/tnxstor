package devenv

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeMarker(t *testing.T, dir, marker string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, marker), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverFindsProjectsAndSkipsNoise(t *testing.T) {
	root := t.TempDir()
	writeMarker(t, filepath.Join(root, "alpha"), "devenv.nix")
	writeMarker(t, filepath.Join(root, "beta"), "devenv.yaml")
	writeMarker(t, filepath.Join(root, "nested", "gamma"), "devenv.nix")
	writeMarker(t, filepath.Join(root, "lock-only"), "devenv.lock")
	writeMarker(t, filepath.Join(root, ".hidden"), "devenv.nix")
	writeMarker(t, filepath.Join(root, "node_modules", "pkg"), "devenv.nix")
	writeMarker(t, filepath.Join(root, "alpha", ".devenv"), "devenv.nix")
	writeMarker(t, filepath.Join(root, "deep", "1", "2", "3", "4", "5"), "devenv.nix")

	envs, err := Discover(root, DefaultMaxDepth)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"alpha", "beta", "nested/gamma"}
	if len(envs) != len(want) {
		t.Fatalf("got %d environments %+v, want %d", len(envs), envs, len(want))
	}
	for i, name := range want {
		if envs[i].Name != name {
			t.Errorf("env %d = %q, want %q", i, envs[i].Name, name)
		}
		if !filepath.IsAbs(envs[i].Dir) {
			t.Errorf("env %d dir is not absolute: %q", i, envs[i].Dir)
		}
	}
}

func TestDiscoverRootItself(t *testing.T) {
	root := t.TempDir()
	writeMarker(t, root, "devenv.nix")
	envs, err := Discover(root, DefaultMaxDepth)
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) != 1 || envs[0].Dir != root {
		t.Fatalf("got %+v, want root project", envs)
	}
	if got := envs[0].Label(); got != filepath.Base(root) {
		t.Errorf("label = %q, want %q", got, filepath.Base(root))
	}
}

func TestDiscoverMissingRoot(t *testing.T) {
	_, err := Discover(filepath.Join(t.TempDir(), "does-not-exist"), DefaultMaxDepth)
	if !errors.Is(err, ErrProjectsDirMissing) {
		t.Fatalf("expected ErrProjectsDirMissing, got %v", err)
	}
}

func TestFindProjectAscends(t *testing.T) {
	proj := filepath.Join(t.TempDir(), "project")
	writeMarker(t, proj, "devenv.nix")
	sub := filepath.Join(proj, "a", "b", "c")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	env, ok := FindProject(sub)
	if !ok {
		t.Fatal("project not found from subdirectory")
	}
	if env.Dir != proj || !env.Current {
		t.Errorf("env = %+v, want %s marked current", env, proj)
	}
}

func TestSelectByDirAndName(t *testing.T) {
	root := t.TempDir()
	alpha := filepath.Join(root, "alpha")
	beta := filepath.Join(root, "team", "beta")
	writeMarker(t, alpha, "devenv.nix")
	writeMarker(t, beta, "devenv.yaml")

	tests := []struct {
		name    string
		dir     string
		env     string
		want    string
		wantErr error
	}{
		{name: "explicit dir", dir: alpha, want: alpha},
		{name: "relative name", env: "team/beta", want: beta},
		{name: "base name", env: "beta", want: beta},
		{name: "absolute path name", env: alpha, want: alpha},
		{name: "missing name", env: "nope", wantErr: ErrNoEnvironment},
		{name: "dir not a project", dir: root, wantErr: ErrNoEnvironment},
		{name: "both selectors", dir: alpha, env: "alpha", wantErr: ErrNoEnvironment},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env, err := Select(root, tc.dir, tc.env)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected %v, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if env.Dir != tc.want {
				t.Errorf("dir = %q, want %q", env.Dir, tc.want)
			}
		})
	}
}

func TestSelectAmbiguousBaseName(t *testing.T) {
	root := t.TempDir()
	writeMarker(t, filepath.Join(root, "x", "demo"), "devenv.nix")
	writeMarker(t, filepath.Join(root, "y", "demo"), "devenv.nix")
	_, err := Select(root, "", "demo")
	if !errors.Is(err, ErrAmbiguousEnvironment) {
		t.Fatalf("expected ErrAmbiguousEnvironment, got %v", err)
	}
	// The relative name disambiguates.
	env, err := Select(root, "", filepath.Join("y", "demo"))
	if err != nil {
		t.Fatal(err)
	}
	if env.Dir != filepath.Join(root, "y", "demo") {
		t.Errorf("dir = %q", env.Dir)
	}
}

func TestSelectUsesCurrentProject(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "current")
	writeMarker(t, proj, "devenv.nix")
	writeMarker(t, filepath.Join(root, "other"), "devenv.nix")
	sub := filepath.Join(proj, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)

	env, err := Select(root, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if env.Dir != proj {
		t.Errorf("dir = %q, want %q", env.Dir, proj)
	}
}

func TestSelectSoleDiscoveredEnvironment(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "only")
	writeMarker(t, proj, "devenv.nix")
	// Use a cwd outside both root and project.
	t.Chdir(t.TempDir())

	env, err := Select(root, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if env.Dir != proj {
		t.Errorf("dir = %q, want %q", env.Dir, proj)
	}
}

func TestSelectMultipleWithoutSelector(t *testing.T) {
	root := t.TempDir()
	writeMarker(t, filepath.Join(root, "one"), "devenv.nix")
	writeMarker(t, filepath.Join(root, "two"), "devenv.nix")
	t.Chdir(t.TempDir())

	_, err := Select(root, "", "")
	if !errors.Is(err, ErrNoEnvironment) {
		t.Fatalf("expected ErrNoEnvironment, got %v", err)
	}
}

func TestDefaultProjectsDirOverrideAndHomeExpansion(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TNXS_PROJECTS_DIR", dir)
	if got := DefaultProjectsDir(); got != dir {
		t.Errorf("got %q, want %q", got, dir)
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("TNXS_PROJECTS_DIR", "~/code")
	if got, want := DefaultProjectsDir(), filepath.Join(home, "code"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDefaultProjectsDirFallsBackToExistingSpelling(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("TNXS_PROJECTS_DIR", "")
	lower := filepath.Join(home, "projects")
	if err := os.MkdirAll(lower, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := DefaultProjectsDir(); got != lower {
		t.Errorf("got %q, want %q", got, lower)
	}
}
