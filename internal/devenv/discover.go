package devenv

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Environment describes a devenv project directory.
type Environment struct {
	// Name is the path relative to the discovery root. For an environment
	// found by walking up from the current directory it is the base name.
	Name string `json:"name"`
	// Dir is the absolute, cleaned project directory.
	Dir string `json:"dir"`
	// Current is true for the environment containing the current working
	// directory.
	Current bool `json:"current"`
}

// Label returns a short, human-readable name for menus.
func (e Environment) Label() string {
	if e.Name != "" && e.Name != "." {
		return e.Name
	}
	return filepath.Base(e.Dir)
}

// Discovery errors. Callers can test with errors.Is.
var (
	// ErrNoEnvironment means no usable devenv project was found.
	ErrNoEnvironment = errors.New("no devenv environment found")
	// ErrProjectsDirMissing means the discovery root does not exist.
	ErrProjectsDirMissing = errors.New("devenv projects directory not found")
	// ErrAmbiguousEnvironment means a name matched more than one project.
	ErrAmbiguousEnvironment = errors.New("ambiguous devenv environment name")
)

// DefaultMaxDepth bounds discovery below the projects root. Deeper trees are
// skipped rather than scanned without limit.
const DefaultMaxDepth = 4

// markers identify a devenv project. Any of these files is sufficient;
// `devenv.lock` alone is not, because stale lock files can be left behind.
var markers = []string{"devenv.nix", "devenv.yaml", "devenv.yml"}

// skippedDirs are never descended into during discovery. Hidden directories
// are skipped separately.
var skippedDirs = map[string]bool{
	"node_modules": true,
	"vendor":       true,
	"target":       true,
	"dist":         true,
	"build":        true,
	"result":       true,
	".git":         true,
	".hg":          true,
	".svn":         true,
	".direnv":      true,
	".devenv":      true,
	".cache":       true,
	".venv":        true,
	"venv":         true,
	"__pycache__":  true,
}

// DefaultProjectsDir returns the directory scanned for devenv projects:
// $TNXS_PROJECTS_DIR when set, otherwise ~/Projects (falling back to
// ~/projects when only the lower-case spelling exists).
func DefaultProjectsDir() string {
	if v := strings.TrimSpace(os.Getenv("TNXS_PROJECTS_DIR")); v != "" {
		return expandHome(v)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	upper := filepath.Join(home, "Projects")
	lower := filepath.Join(home, "projects")
	if isDir(upper) {
		return upper
	}
	if isDir(lower) {
		return lower
	}
	return upper
}

// IsProject reports whether dir contains a devenv project marker.
func IsProject(dir string) bool {
	for _, m := range markers {
		st, err := os.Stat(filepath.Join(dir, m))
		if err == nil && !st.IsDir() {
			return true
		}
	}
	return false
}

// FindProject walks up from dir looking for a devenv project. It returns the
// environment containing dir, if any.
func FindProject(dir string) (Environment, bool) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Environment{}, false
	}
	abs = filepath.Clean(abs)
	for {
		if IsProject(abs) {
			return Environment{Name: filepath.Base(abs), Dir: abs, Current: true}, true
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return Environment{}, false
		}
		abs = parent
	}
}

// Discover scans root for devenv projects up to maxDepth levels below root.
// Hidden directories and generated trees (node_modules, .devenv, …) are
// skipped. Results are sorted by relative name.
func Discover(root string, maxDepth int) ([]Environment, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("%w: empty path", ErrProjectsDirMissing)
	}
	abs, err := filepath.Abs(expandHome(root))
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrProjectsDirMissing, abs)
		}
		return nil, err
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("%w: %s is not a directory", ErrProjectsDirMissing, abs)
	}
	if maxDepth <= 0 {
		maxDepth = DefaultMaxDepth
	}

	var envs []Environment
	err = filepath.WalkDir(abs, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			// An unreadable subtree should not abort discovery of the
			// rest of the projects directory.
			if path == abs {
				return walkErr
			}
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if path != abs {
			if depthFrom(abs, path) > maxDepth {
				return filepath.SkipDir
			}
			if skippedDirs[d.Name()] || strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
		}
		if IsProject(path) {
			name, relErr := filepath.Rel(abs, path)
			if relErr != nil {
				name = filepath.Base(path)
			}
			envs = append(envs, Environment{Name: name, Dir: path})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(envs, func(i, j int) bool {
		if envs[i].Name == envs[j].Name {
			return envs[i].Dir < envs[j].Dir
		}
		return envs[i].Name < envs[j].Name
	})
	return envs, nil
}

// Select resolves which devenv environment to operate on:
//
//  1. dir, when given, must contain a project marker;
//  2. name, when given, must match exactly one discovered environment
//     (relative name, directory base name, or absolute path);
//  3. otherwise the project containing the current directory is used;
//  4. otherwise the sole discovered environment is used, and multiple
//     candidates produce a helpful error.
func Select(root, dir, name string) (Environment, error) {
	dir = strings.TrimSpace(dir)
	name = strings.TrimSpace(name)
	if dir != "" && name != "" {
		return Environment{}, fmt.Errorf("%w: use either a directory or an environment name, not both", ErrNoEnvironment)
	}
	if dir != "" {
		abs, err := filepath.Abs(expandHome(dir))
		if err != nil {
			return Environment{}, err
		}
		abs = filepath.Clean(abs)
		if !IsProject(abs) {
			return Environment{}, fmt.Errorf("%w: %s does not contain devenv.nix or devenv.yaml", ErrNoEnvironment, abs)
		}
		return Environment{Name: filepath.Base(abs), Dir: abs}, nil
	}
	if name != "" {
		envs, err := Discover(root, DefaultMaxDepth)
		if err != nil {
			return Environment{}, err
		}
		matches := matchName(envs, name)
		switch len(matches) {
		case 0:
			return Environment{}, fmt.Errorf("%w: %q (see `tnxs devenv-envs`)", ErrNoEnvironment, name)
		case 1:
			return matches[0], nil
		default:
			var dirs []string
			for _, m := range matches {
				dirs = append(dirs, m.Dir)
			}
			return Environment{}, fmt.Errorf("%w: %q matches %s; use --dir",
				ErrAmbiguousEnvironment, name, strings.Join(dirs, ", "))
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		if env, ok := FindProject(cwd); ok {
			return env, nil
		}
	}
	envs, err := Discover(root, DefaultMaxDepth)
	if err != nil {
		return Environment{}, fmt.Errorf("%w: %v; use --dir or --env (see `tnxs devenv-envs`)", ErrNoEnvironment, err)
	}
	switch len(envs) {
	case 0:
		return Environment{}, fmt.Errorf("%w under %s; use --dir or run from a project", ErrNoEnvironment, root)
	case 1:
		return envs[0], nil
	default:
		return Environment{}, fmt.Errorf("%w: multiple environments under %s; choose one with --env or --dir (see `tnxs devenv-envs`)",
			ErrNoEnvironment, root)
	}
}

func matchName(envs []Environment, name string) []Environment {
	var matches []Environment
	absName, _ := filepath.Abs(expandHome(name))
	for _, e := range envs {
		if e.Name == name || e.Dir == name || (absName != "" && e.Dir == absName) || filepath.Base(e.Dir) == name {
			matches = append(matches, e)
		}
	}
	return matches
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			if p == "~" {
				return home
			}
			return filepath.Join(home, strings.TrimPrefix(p, "~/"))
		}
	}
	return p
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func depthFrom(root, path string) int {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return 0
	}
	return strings.Count(rel, string(os.PathSeparator)) + 1
}
