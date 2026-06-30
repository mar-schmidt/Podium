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
