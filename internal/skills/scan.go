package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Scan reads the three default skill roots and returns the deduplicated
// catalogue, sorted by skill name. Missing roots are treated as empty.
func Scan() ([]Skill, error) {
	roots, err := DefaultRoots()
	if err != nil {
		return nil, err
	}
	return ScanRoots(roots)
}

// entry is one raw skill folder found in a single root.
type entry struct {
	source   Source
	name     string // directory name (the dedup key)
	linkPath string // <root>/<name>
	realPath string // resolved real directory (symlinks followed)
	skillMd  string // <root>/<name>/SKILL.md (user-facing path)
	body     string // SKILL.md contents
	desc     string // frontmatter description
}

// ScanRoots discovers and deduplicates skills across the given roots.
func ScanRoots(roots Roots) ([]Skill, error) {
	grouped := map[string][]entry{}
	var names []string
	for _, src := range order {
		ents, err := readSkillDir(src, roots.dir(src))
		if err != nil {
			return nil, err
		}
		for _, e := range ents {
			if _, ok := grouped[e.name]; !ok {
				names = append(names, e.name)
			}
			grouped[e.name] = append(grouped[e.name], e)
		}
	}
	sort.Strings(names)

	out := make([]Skill, 0, len(names))
	for _, name := range names {
		out = append(out, buildSkill(name, grouped[name]))
	}
	return out, nil
}

// readSkillDir lists the immediate child skill folders of dir. Each child that
// is a directory (following symlinks) and contains a SKILL.md counts as one
// skill. Hidden entries (e.g. ~/.codex/skills/.system) are skipped, and a
// missing dir yields no entries rather than an error.
func readSkillDir(src Source, dir string) ([]entry, error) {
	if dir == "" {
		return nil, nil
	}
	items, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read skills dir %s: %w", dir, err)
	}
	var out []entry
	for _, it := range items {
		name := it.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		linkPath := filepath.Join(dir, name)
		info, err := os.Stat(linkPath) // follows symlinks
		if err != nil || !info.IsDir() {
			continue
		}
		skillMd := filepath.Join(linkPath, "SKILL.md")
		raw, err := os.ReadFile(skillMd)
		if err != nil {
			continue // not a skill folder
		}
		real, err := filepath.EvalSymlinks(linkPath)
		if err != nil {
			real = linkPath
		}
		_, desc := parseFrontmatter(string(raw))
		out = append(out, entry{
			source:   src,
			name:     name,
			linkPath: linkPath,
			realPath: real,
			skillMd:  skillMd,
			body:     string(raw),
			desc:     desc,
		})
	}
	return out, nil
}

// buildSkill collapses every entry sharing a name into one catalogue row,
// records its sources/locations, and detects content conflicts (S11).
func buildSkill(name string, entries []entry) Skill {
	sk := Skill{Name: name}

	// Sources + locations, one per source, in stable order.
	for _, src := range order {
		if e, ok := firstOfSource(entries, src); ok {
			sk.Sources = append(sk.Sources, src)
			sk.Locations = append(sk.Locations, Location{Source: src, Path: e.skillMd})
		}
	}

	// Description: prefer the canonical (agents) copy, else the first non-empty.
	if e, ok := firstOfSource(entries, SourceAgents); ok && e.desc != "" {
		sk.Description = e.desc
	}
	if sk.Description == "" {
		for _, e := range entries {
			if e.desc != "" {
				sk.Description = e.desc
				break
			}
		}
	}

	// Conflict = more than one distinct body across distinct real targets. A
	// union symlink that resolves to a provider's real folder is the same skill,
	// not a conflict, because both share one realPath.
	bodyByReal := map[string]string{}
	for _, e := range entries {
		bodyByReal[e.realPath] = e.body
	}
	uniqueBodies := map[string]struct{}{}
	for _, b := range bodyByReal {
		uniqueBodies[normalizeBody(b)] = struct{}{}
	}
	sk.Conflict = len(uniqueBodies) > 1

	if sk.Conflict {
		// Surface each source's own body honestly (no merging).
		for _, src := range order {
			if e, ok := firstOfSource(entries, src); ok {
				sk.Contents = append(sk.Contents, Content{Source: src, Body: e.body})
			}
		}
	} else {
		body := ""
		if e, ok := firstOfSource(entries, SourceAgents); ok {
			body = e.body
		} else if len(entries) > 0 {
			body = entries[0].body
		}
		sk.Contents = []Content{{Body: body}}
	}
	return sk
}

func firstOfSource(entries []entry, src Source) (entry, bool) {
	for _, e := range entries {
		if e.source == src {
			return e, true
		}
	}
	return entry{}, false
}

// parseFrontmatter extracts name/description from a leading `--- ... ---` YAML
// block. It returns empty strings when there is no parseable frontmatter.
func parseFrontmatter(body string) (name, desc string) {
	s := strings.TrimLeft(strings.TrimPrefix(body, "\ufeff"), " \t\r\n")
	if !strings.HasPrefix(s, "---") {
		return "", ""
	}
	rest := s[len("---"):]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return "", ""
	}
	var fm struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal([]byte(rest[:idx]), &fm); err != nil {
		return "", ""
	}
	return strings.TrimSpace(fm.Name), strings.TrimSpace(fm.Description)
}

// normalizeBody trims surrounding whitespace so trivial differences don't read
// as content conflicts.
func normalizeBody(b string) string { return strings.TrimSpace(b) }
