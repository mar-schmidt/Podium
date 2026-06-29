package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mar-schmidt/Podium/internal/config"
	"github.com/mar-schmidt/Podium/internal/store"
)

// CreateAgentRequest describes a new agent and its defaults. Empty provider,
// effort, and permission fields inherit from the global config defaults.
type CreateAgentRequest struct {
	Name           string
	Provider       config.Provider
	Profile        string
	Model          string
	Effort         string
	PermissionMode config.PermissionMode
	Fallback       []string
	MCPConfig      string
}

// CreateAgent scaffolds the agent directory and stores its durable definition.
func (c *Core) CreateAgent(ctx context.Context, req CreateAgentRequest) (store.Agent, error) {
	if err := validateAgentName(req.Name); err != nil {
		return store.Agent{}, err
	}
	agent := store.Agent{
		Name:           req.Name,
		Provider:       req.Provider,
		Profile:        req.Profile,
		Model:          req.Model,
		Effort:         req.Effort,
		PermissionMode: req.PermissionMode,
		Fallback:       append([]string(nil), req.Fallback...),
		MCPConfig:      req.MCPConfig,
	}
	c.applyAgentDefaults(&agent)
	if err := scaffoldAgent(c.AgentPaths(agent.Name), agent.Name); err != nil {
		return store.Agent{}, err
	}
	return c.store.CreateAgent(ctx, agent)
}

// GetAgent fetches one agent.
func (c *Core) GetAgent(ctx context.Context, name string) (store.Agent, error) {
	return c.store.GetAgent(ctx, name)
}

// ListAgents returns all agents ordered by name.
func (c *Core) ListAgents(ctx context.Context) ([]store.Agent, error) {
	return c.store.ListAgents(ctx)
}

// UpdateAgent updates an agent's durable defaults. It also ensures the agent's
// expected directory layout exists.
func (c *Core) UpdateAgent(ctx context.Context, agent store.Agent) (store.Agent, error) {
	if err := validateAgentName(agent.Name); err != nil {
		return store.Agent{}, err
	}
	c.applyAgentDefaults(&agent)
	if err := scaffoldAgent(c.AgentPaths(agent.Name), agent.Name); err != nil {
		return store.Agent{}, err
	}
	return c.store.UpdateAgent(ctx, agent)
}

// DeleteAgent removes the durable agent definition. Files on disk are left in
// place so user-authored identity/instructions are never deleted implicitly.
func (c *Core) DeleteAgent(ctx context.Context, name string) error {
	return c.store.DeleteAgent(ctx, name)
}

// ReadAgentSoul returns the contents of an agent's SOUL.md, or empty string if
// the file does not exist yet.
func (c *Core) ReadAgentSoul(name string) (string, error) {
	if err := validateAgentName(name); err != nil {
		return "", err
	}
	data, err := os.ReadFile(c.AgentPaths(name).Soul)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read SOUL.md for agent %q: %w", name, err)
	}
	return string(data), nil
}

// WriteAgentSoul overwrites an agent's SOUL.md with the given content, creating
// the agent directory if needed.
func (c *Core) WriteAgentSoul(name, content string) error {
	if err := validateAgentName(name); err != nil {
		return err
	}
	paths := c.AgentPaths(name)
	if err := os.MkdirAll(paths.Root, 0o755); err != nil {
		return fmt.Errorf("create agent dir: %w", err)
	}
	if err := os.WriteFile(paths.Soul, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write SOUL.md for agent %q: %w", name, err)
	}
	return nil
}

func (c *Core) applyAgentDefaults(agent *store.Agent) {
	if agent.Provider == "" {
		agent.Provider = c.global.Provider
	}
	if agent.Model == "" {
		agent.Model = c.global.Model
	}
	if agent.Effort == "" {
		agent.Effort = c.global.Effort
	}
	if agent.PermissionMode == "" {
		agent.PermissionMode = c.global.PermissionMode
	}
	if len(agent.Fallback) == 0 {
		agent.Fallback = append([]string(nil), c.global.Fallback...)
	}
}

func scaffoldAgent(paths AgentPaths, name string) error {
	for _, dir := range []string{paths.Root, paths.Workspace} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create dir %s: %w", dir, err)
		}
	}
	template := strings.ReplaceAll(string(config.AgentSoulTemplate()), "{{agent_name}}", name)
	if _, err := writeIfAbsent(paths.Soul, []byte(template), 0o644); err != nil {
		return err
	}
	return nil
}

func writeIfAbsent(path string, data []byte, perm os.FileMode) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("stat %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("create parent of %s: %w", path, err)
	}
	if err := os.WriteFile(path, data, perm); err != nil {
		return false, fmt.Errorf("write %s: %w", path, err)
	}
	return true, nil
}
