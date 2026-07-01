// Package projects manages Podium's shared, system-level project ledger
// (§5.3 / D22): a single `projects.yaml` plus one subdirectory per project under
// ~/.podium/projects/. Projects are agent-independent — any agent can read and
// work on any project. Agents also maintain this file directly, so the ledger is
// the source of truth and v1 accepts last-write-wins (R5.12).
package projects

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"gopkg.in/yaml.v3"
)

var safeProjectID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// Project mirrors one entry in projects.yaml (R5.10). `Path` is relative to the
// projects directory.
type Project struct {
	ID          string   `yaml:"id" json:"id"`
	Name        string   `yaml:"name" json:"name"`
	Description string   `yaml:"description" json:"description"`
	Color       string   `yaml:"color,omitempty" json:"color"`
	Path        string   `yaml:"path" json:"path"`
	Status      string   `yaml:"status" json:"status"`
	Stack       []string `yaml:"stack" json:"stack"`
	Repo        string   `yaml:"repo" json:"repo"`
	Roadmap     []string `yaml:"roadmap" json:"roadmap"`
	Notes       string   `yaml:"notes" json:"notes"`
}

// ProjectPatch carries the mutable fields a user can edit from the UI. Nil
// pointers are left unchanged so partial updates are safe under the
// last-write-wins ledger.
type ProjectPatch struct {
	Name        *string
	Description *string
	Color       *string
	Status      *string
	Stack       *[]string
	Notes       *string
}

// ledgerFile is the on-disk shape of projects.yaml.
type ledgerFile struct {
	Projects []Project `yaml:"projects"`
}

// Ledger reads and writes the shared projects.yaml. It serializes Podium's own
// writes with a mutex; cross-process writes by agents remain last-write-wins.
type Ledger struct {
	dir  string // the projects directory (holds projects.yaml + project dirs)
	path string // projects.yaml
	mu   sync.Mutex
}

// New returns a Ledger rooted at the projects directory.
func New(projectsDir string) *Ledger {
	return &Ledger{
		dir:  projectsDir,
		path: filepath.Join(projectsDir, "projects.yaml"),
	}
}

// List returns all projects from the ledger. A missing ledger yields an empty
// list rather than an error.
func (l *Ledger) List() ([]Project, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	file, err := l.read()
	if err != nil {
		return nil, err
	}
	return file.Projects, nil
}

// Get returns a single project by id.
func (l *Ledger) Get(id string) (Project, error) {
	projects, err := l.List()
	if err != nil {
		return Project{}, err
	}
	for _, p := range projects {
		if p.ID == id {
			return p, nil
		}
	}
	return Project{}, fmt.Errorf("project %q not found", id)
}

// Create adds a new project: it validates the id, creates the project directory
// under the projects root, and appends an entry to the ledger. It errors if the
// id already exists.
func (l *Ledger) Create(p Project) (Project, error) {
	if !safeProjectID.MatchString(p.ID) {
		return Project{}, fmt.Errorf("invalid project id %q: use letters, numbers, dot, dash, or underscore", p.ID)
	}
	if p.Name == "" {
		p.Name = p.ID
	}
	if p.Path == "" {
		p.Path = p.ID
	}
	if p.Status == "" {
		p.Status = "active"
	}
	if p.Stack == nil {
		p.Stack = []string{}
	}
	if p.Roadmap == nil {
		p.Roadmap = []string{}
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	file, err := l.read()
	if err != nil {
		return Project{}, err
	}
	for _, existing := range file.Projects {
		if existing.ID == p.ID {
			return Project{}, fmt.Errorf("project %q already exists", p.ID)
		}
	}

	if err := os.MkdirAll(filepath.Join(l.dir, p.Path), 0o755); err != nil {
		return Project{}, fmt.Errorf("create project dir: %w", err)
	}
	file.Projects = append(file.Projects, p)
	if err := l.write(file); err != nil {
		return Project{}, err
	}
	return p, nil
}

// Update applies a partial patch to an existing project and rewrites the
// ledger. It errors if the id does not exist.
func (l *Ledger) Update(id string, patch ProjectPatch) (Project, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	file, err := l.read()
	if err != nil {
		return Project{}, err
	}
	idx := -1
	for i := range file.Projects {
		if file.Projects[i].ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return Project{}, fmt.Errorf("project %q not found", id)
	}
	p := &file.Projects[idx]
	if patch.Name != nil {
		p.Name = *patch.Name
	}
	if patch.Description != nil {
		p.Description = *patch.Description
	}
	if patch.Color != nil {
		p.Color = *patch.Color
	}
	if patch.Status != nil {
		p.Status = *patch.Status
	}
	if patch.Stack != nil {
		p.Stack = *patch.Stack
	}
	if patch.Notes != nil {
		p.Notes = *patch.Notes
	}
	if err := l.write(file); err != nil {
		return Project{}, err
	}
	return *p, nil
}

// SyncRoadmaps replaces each project's derived roadmap with the ordered task
// ids currently known for that project. Unknown project IDs in byProject are
// ignored; projects without tasks get an empty roadmap.
func (l *Ledger) SyncRoadmaps(byProject map[string][]string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	file, err := l.read()
	if err != nil {
		return err
	}
	changed := false
	for i := range file.Projects {
		next := byProject[file.Projects[i].ID]
		if next == nil {
			next = []string{}
		}
		if !sameStrings(file.Projects[i].Roadmap, next) {
			file.Projects[i].Roadmap = append([]string(nil), next...)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return l.write(file)
}

func (l *Ledger) read() (ledgerFile, error) {
	raw, err := os.ReadFile(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return ledgerFile{Projects: []Project{}}, nil
		}
		return ledgerFile{}, fmt.Errorf("read projects ledger: %w", err)
	}
	var file ledgerFile
	if err := yaml.Unmarshal(raw, &file); err != nil {
		return ledgerFile{}, fmt.Errorf("parse projects.yaml: %w", err)
	}
	if file.Projects == nil {
		file.Projects = []Project{}
	}
	return file, nil
}

func (l *Ledger) write(file ledgerFile) error {
	if err := os.MkdirAll(l.dir, 0o755); err != nil {
		return fmt.Errorf("create projects dir: %w", err)
	}
	raw, err := yaml.Marshal(file)
	if err != nil {
		return fmt.Errorf("marshal projects.yaml: %w", err)
	}
	if err := os.WriteFile(l.path, raw, 0o644); err != nil {
		return fmt.Errorf("write projects.yaml: %w", err)
	}
	return nil
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
