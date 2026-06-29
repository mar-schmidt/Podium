// Package core owns Podium's provider-independent domain behavior: durable
// agents, sessions, history append, and instruction composition.
package core

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mar-schmidt/Podium/internal/adapter"
	"github.com/mar-schmidt/Podium/internal/config"
	"github.com/mar-schmidt/Podium/internal/store"
)

var safeAgentName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// Options configures a Core service.
type Options struct {
	Paths    config.Paths
	Store    *store.Store
	Adapter  adapter.Adapter
	Global   config.Global
	Profiles []config.Profile
}

// Core coordinates typed persistence, filesystem scaffolding, instruction
// composition, and adapter calls.
type Core struct {
	paths    config.Paths
	store    *store.Store
	adapter  adapter.Adapter
	global   config.Global
	profiles map[string]config.Profile
	composer InstructionComposer
}

// New creates a Core service.
func New(opts Options) (*Core, error) {
	if opts.Store == nil {
		return nil, errors.New("core store is required")
	}
	ad := opts.Adapter
	if ad == nil {
		ad = adapter.NewFake()
	}
	global := opts.Global
	if global.Provider == "" {
		global.Provider = config.ProviderClaude
	}
	if global.Effort == "" {
		global.Effort = "medium"
	}
	if global.PermissionMode == "" {
		global.PermissionMode = config.PermissionApprove
	}
	c := &Core{
		paths:    opts.Paths,
		store:    opts.Store,
		adapter:  ad,
		global:   global,
		profiles: map[string]config.Profile{},
		composer: NewFileComposer(opts.Paths),
	}
	for _, profile := range opts.Profiles {
		c.profiles[profile.Name] = profile
	}
	return c, nil
}

// Store exposes the typed persistence API used by this core.
func (c *Core) Store() *store.Store { return c.store }

// AgentPaths returns the well-known filesystem paths for an agent.
func (c *Core) AgentPaths(name string) AgentPaths {
	dir := filepath.Join(c.paths.AgentsDir, name)
	return AgentPaths{
		Root:      dir,
		Soul:      filepath.Join(dir, "SOUL.md"),
		Agents:    filepath.Join(dir, "AGENTS.md"),
		Workspace: filepath.Join(dir, "workspace"),
	}
}

// AgentPaths is the on-disk layout for one agent.
type AgentPaths struct {
	Root      string
	Soul      string
	Agents    string
	Workspace string
}

func validateAgentName(name string) error {
	if !safeAgentName.MatchString(name) {
		return fmt.Errorf("invalid agent name %q: use letters, numbers, dot, dash, or underscore", name)
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("invalid agent name %q: parent path segments are not allowed", name)
	}
	return nil
}

func (c *Core) profileDir(provider config.Provider, name string) string {
	if name == "" {
		return ""
	}
	profile, ok := c.profiles[name]
	if !ok || profile.Provider != provider {
		return ""
	}
	switch provider {
	case config.ProviderClaude:
		return profile.ConfigDir
	case config.ProviderCodex:
		return profile.HomeDir
	default:
		return ""
	}
}
