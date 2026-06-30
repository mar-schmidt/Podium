package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// RemoveAgent removes an agent entry from config.yaml when it exists. It edits
// the YAML syntax tree so user comments and unrelated settings survive.
func RemoveAgent(path, name string) error {
	return editFile(path, func(root *yaml.Node) (bool, error) {
		return removeAgent(root, name), nil
	})
}

// SetGlobal upserts the daemon-wide defaults under the top-level `global:`
// mapping in config.yaml. It edits the YAML syntax tree so user comments and
// unrelated settings (including global.permission_timeout) survive. Only the
// fields Podium's Settings page owns are touched: provider, model, effort,
// permission_mode, and fallback. Empty model/effort keys are removed so the
// file falls back to applyDefaults; an empty fallback drops the key entirely.
func SetGlobal(path string, g Global) error {
	return editFile(path, func(root *yaml.Node) (bool, error) {
		return setGlobal(root, g), nil
	})
}

func setGlobal(root *yaml.Node, g Global) bool {
	doc := root
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		doc = root.Content[0]
	}
	if doc.Kind != yaml.MappingNode {
		return false
	}
	global := mappingChild(doc, "global")
	if global == nil {
		global = &yaml.Node{Kind: yaml.MappingNode}
		doc.Content = append(doc.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "global"},
			global,
		)
	}

	changed := false
	changed = setScalarChild(global, "provider", string(g.Provider)) || changed
	changed = setScalarChild(global, "model", g.Model) || changed
	changed = setScalarChild(global, "effort", g.Effort) || changed
	changed = setScalarChild(global, "permission_mode", string(g.PermissionMode)) || changed
	changed = setSequenceChild(global, "fallback", g.Fallback) || changed
	return changed
}

// mappingChild returns the value node for key in a mapping node, or nil.
func mappingChild(mapping *yaml.Node, key string) *yaml.Node {
	if mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// setScalarChild upserts key->value in a mapping. An empty value removes the
// key (so config falls back to applyDefaults). Reports whether it mutated.
func setScalarChild(mapping *yaml.Node, key, value string) bool {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value != key {
			continue
		}
		if value == "" {
			mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
			return true
		}
		if mapping.Content[i+1].Value == value && mapping.Content[i+1].Kind == yaml.ScalarNode {
			return false
		}
		mapping.Content[i+1].Kind = yaml.ScalarNode
		mapping.Content[i+1].Tag = ""
		mapping.Content[i+1].Style = 0
		mapping.Content[i+1].Value = value
		return true
	}
	if value == "" {
		return false
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Value: value},
	)
	return true
}

// setSequenceChild upserts key->[]values in a mapping. An empty slice removes
// the key. Reports whether it mutated.
func setSequenceChild(mapping *yaml.Node, key string, values []string) bool {
	seq := &yaml.Node{Kind: yaml.SequenceNode}
	for _, v := range values {
		seq.Content = append(seq.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: v})
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value != key {
			continue
		}
		if len(values) == 0 {
			mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
			return true
		}
		if sequenceEqual(mapping.Content[i+1], values) {
			return false
		}
		mapping.Content[i+1] = seq
		return true
	}
	if len(values) == 0 {
		return false
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: key},
		seq,
	)
	return true
}

func sequenceEqual(node *yaml.Node, values []string) bool {
	if node.Kind != yaml.SequenceNode || len(node.Content) != len(values) {
		return false
	}
	for i, v := range values {
		if node.Content[i].Value != v {
			return false
		}
	}
	return true
}

func editFile(path string, edit func(root *yaml.Node) (bool, error)) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config for edit: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat config for edit: %w", err)
	}
	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return fmt.Errorf("parse config for edit %s: %w", path, err)
	}
	changed, err := edit(&root)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}

	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		_ = enc.Close()
		return fmt.Errorf("encode config edit: %w", err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("close config encoder: %w", err)
	}
	return writeFileAtomic(path, out.Bytes(), info.Mode().Perm())
}

func removeAgent(root *yaml.Node, name string) bool {
	doc := root
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		doc = root.Content[0]
	}
	if doc.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i+1 < len(doc.Content); i += 2 {
		key := doc.Content[i]
		if key.Value != "agents" {
			continue
		}
		agents := doc.Content[i+1]
		if agents.Kind != yaml.SequenceNode {
			return false
		}
		changed := false
		kept := agents.Content[:0]
		for _, entry := range agents.Content {
			if agentNodeName(entry) == name {
				changed = true
				continue
			}
			kept = append(kept, entry)
		}
		if changed {
			agents.Content = kept
		}
		return changed
	}
	return false
}

func agentNodeName(node *yaml.Node) string {
	if node.Kind != yaml.MappingNode {
		return ""
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == "name" {
			return node.Content[i+1].Value
		}
	}
	return ""
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp config: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp config: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}
