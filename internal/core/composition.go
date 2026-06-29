package core

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mar-schmidt/Podium/internal/config"
	"github.com/mar-schmidt/Podium/internal/store"
)

// DeliveryMode selects how composed instructions are delivered to a provider.
type DeliveryMode string

const (
	// DeliveryClaudeImport writes a generated CLAUDE.md made of @ imports.
	DeliveryClaudeImport DeliveryMode = "claude_import"
	// DeliveryCodexBundle writes a generated AGENTS.md with concatenated content.
	DeliveryCodexBundle DeliveryMode = "codex_bundle"
)

// InstructionSource is one physical source included in a composed payload.
type InstructionSource struct {
	Label string
	Path  string
}

// InstructionPayload is the provider-ready instruction artifact.
type InstructionPayload struct {
	Mode    DeliveryMode
	Path    string
	Bytes   []byte
	Sources []InstructionSource
}

// InstructionComposer composes Podium's base instructions, optional per-agent
// instructions, and SOUL.md in the fixed order required by the spec.
type InstructionComposer interface {
	Compose(context.Context, store.Agent, DeliveryMode) (InstructionPayload, error)
}

// FileComposer composes instruction payloads from the Podium home directory.
type FileComposer struct {
	paths config.Paths
}

// NewFileComposer returns a filesystem-backed instruction composer.
func NewFileComposer(paths config.Paths) *FileComposer {
	return &FileComposer{paths: paths}
}

// Compose produces and writes the provider-ready instruction payload.
func (c *FileComposer) Compose(ctx context.Context, agent store.Agent, mode DeliveryMode) (InstructionPayload, error) {
	if err := ctx.Err(); err != nil {
		return InstructionPayload{}, err
	}
	agentPaths := agentPaths(c.paths, agent.Name)
	sources, err := c.sources(agentPaths)
	if err != nil {
		return InstructionPayload{}, err
	}
	switch mode {
	case DeliveryClaudeImport:
		return c.composeClaude(agent, agentPaths, sources)
	case DeliveryCodexBundle:
		return c.composeCodex(agent, agentPaths, sources)
	default:
		return InstructionPayload{}, fmt.Errorf("unknown instruction delivery mode %q", mode)
	}
}

func (c *FileComposer) sources(paths AgentPaths) ([]InstructionSource, error) {
	sources := []InstructionSource{{Label: "base AGENTS.md", Path: c.paths.BaseAgents}}
	if _, err := os.Stat(paths.Agents); err == nil {
		sources = append(sources, InstructionSource{Label: "agent AGENTS.md", Path: paths.Agents})
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("stat %s: %w", paths.Agents, err)
	}
	sources = append(sources, InstructionSource{Label: "SOUL.md", Path: paths.Soul})
	for _, src := range sources {
		if _, err := os.Stat(src.Path); err != nil {
			return nil, fmt.Errorf("instruction source %s: %w", src.Path, err)
		}
	}
	return sources, nil
}

func (c *FileComposer) composeClaude(agent store.Agent, paths AgentPaths, sources []InstructionSource) (InstructionPayload, error) {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "# Podium generated Claude context for %s\n\n", agent.Name)
	for _, src := range sources {
		fmt.Fprintf(&buf, "@%s\n", filepath.Clean(src.Path))
	}
	payloadPath := filepath.Join(paths.Workspace, "CLAUDE.md")
	return writePayload(DeliveryClaudeImport, payloadPath, buf.Bytes(), sources)
}

func (c *FileComposer) composeCodex(agent store.Agent, paths AgentPaths, sources []InstructionSource) (InstructionPayload, error) {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "# Podium generated Codex instructions for %s\n\n", agent.Name)
	for i, src := range sources {
		raw, err := os.ReadFile(src.Path)
		if err != nil {
			return InstructionPayload{}, fmt.Errorf("read instruction source %s: %w", src.Path, err)
		}
		if i > 0 {
			buf.WriteString("\n\n")
		}
		fmt.Fprintf(&buf, "<!-- Source: %s -->\n\n", src.Path)
		buf.Write(bytes.TrimSpace(raw))
		buf.WriteByte('\n')
	}
	payloadPath := filepath.Join(paths.Workspace, "AGENTS.md")
	return writePayload(DeliveryCodexBundle, payloadPath, buf.Bytes(), sources)
}

func writePayload(mode DeliveryMode, path string, data []byte, sources []InstructionSource) (InstructionPayload, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return InstructionPayload{}, fmt.Errorf("create parent of %s: %w", path, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return InstructionPayload{}, fmt.Errorf("write instruction payload %s: %w", path, err)
	}
	return InstructionPayload{
		Mode:    mode,
		Path:    path,
		Bytes:   data,
		Sources: append([]InstructionSource(nil), sources...),
	}, nil
}

func agentPaths(paths config.Paths, name string) AgentPaths {
	dir := filepath.Join(paths.AgentsDir, name)
	return AgentPaths{
		Root:      dir,
		Soul:      filepath.Join(dir, "SOUL.md"),
		Agents:    filepath.Join(dir, "AGENTS.md"),
		Workspace: filepath.Join(dir, "workspace"),
	}
}
