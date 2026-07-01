// Package config resolves Podium's storage root, loads and validates the system
// configuration (config.yaml), and performs first-run scaffolding of the
// ~/.podium/ directory tree. All Podium state lives under a single root so the
// layout is predictable and backup-friendly on every OS (R9.1 / R10.2 / D14).
package config

import (
	"os"
	"path/filepath"
	"strings"
)

// EnvHome is the environment variable that overrides the storage root. Keeping
// the root overridable (rather than hard-coding ~/.podium/) is what lets Podium
// later run as a Home Assistant add-on with a mapped volume without a rewrite
// (Principle 7 / D18).
const EnvHome = "PODIUM_HOME"

// Paths holds every well-known location under the storage root. Resolve these
// once at startup and pass Paths around rather than recomputing strings.
type Paths struct {
	Home         string // the storage root itself (e.g. ~/.podium)
	ConfigYAML   string // config.yaml
	BaseAgents   string // AGENTS.md (Podium-owned base instructions)
	DB           string // podium.db (SQLite)
	AgentsDir    string // agents/
	ProjectsDir  string // projects/
	ProjectsYAML string // projects/projects.yaml
	SchedulesDir string // schedules/
	ProfilesDir  string // profiles/
	LogsDir      string // logs/
	ArchiveDir   string // archive/ (archived tasks and other exports)
}

// ResolveHome returns the absolute storage root. Precedence:
//  1. $PODIUM_HOME if set (with ~ expansion),
//  2. ~/.podium otherwise.
//
// The result is always absolute: a daemon may chdir (or be launched from an
// arbitrary cwd), so a relative PODIUM_HOME like "podium-data" must be anchored
// at resolution time rather than re-interpreted later (R10.2).
func ResolveHome() (string, error) {
	if v := strings.TrimSpace(os.Getenv(EnvHome)); v != "" {
		expanded, err := expandTilde(v)
		if err != nil {
			return "", err
		}
		return filepath.Abs(expanded)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".podium"), nil
}

// NewPaths derives all well-known paths from a resolved storage root.
func NewPaths(home string) Paths {
	return Paths{
		Home:         home,
		ConfigYAML:   filepath.Join(home, "config.yaml"),
		BaseAgents:   filepath.Join(home, "AGENTS.md"),
		DB:           filepath.Join(home, "podium.db"),
		AgentsDir:    filepath.Join(home, "agents"),
		ProjectsDir:  filepath.Join(home, "projects"),
		ProjectsYAML: filepath.Join(home, "projects", "projects.yaml"),
		SchedulesDir: filepath.Join(home, "schedules"),
		ProfilesDir:  filepath.Join(home, "profiles"),
		LogsDir:      filepath.Join(home, "logs"),
		ArchiveDir:   filepath.Join(home, "archive"),
	}
}

// expandTilde expands a leading ~ or ~/ to the user's home directory. Bare
// environment values like "~" or "~/podium" are common, so handle them rather
// than passing a literal tilde down to the filesystem.
func expandTilde(p string) (string, error) {
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, "~\\") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if p == "~" {
			return home, nil
		}
		return filepath.Join(home, p[2:]), nil
	}
	return filepath.Clean(p), nil
}
