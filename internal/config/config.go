package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// PermissionMode is Podium's two-value permission posture (§5.5 / D12).
type PermissionMode string

const (
	// PermissionApprove relays each side-effecting action to the user (default,
	// the only real safety boundary in v1).
	PermissionApprove PermissionMode = "approve"
	// PermissionYolo auto-approves everything with whole-machine access (opt-in).
	PermissionYolo PermissionMode = "yolo"
)

// Provider identifies a backing CLI.
type Provider string

const (
	ProviderClaude Provider = "claude"
	ProviderCodex  Provider = "codex"
)

// Config is the parsed config.yaml. It does not define schedules (self-describing
// files, §7) or projects (shared ledger, §5.3) — only agents, profiles, defaults,
// and the server bind (R9.2).
type Config struct {
	Global   Global    `yaml:"global"`
	Profiles []Profile `yaml:"profiles"`
	Agents   []Agent   `yaml:"agents"`
	Server   Server    `yaml:"server"`
}

// Global holds defaults applied across agents unless overridden per agent.
type Global struct {
	Provider       Provider       `yaml:"provider"`
	Model          string         `yaml:"model"`
	Effort         string         `yaml:"effort"`
	PermissionMode PermissionMode `yaml:"permission_mode"`
	Fallback       []string       `yaml:"fallback"`
}

// Profile is an optional named auth context, 1:1 with one underlying account
// (§8.7). Podium owns only the directory path and name — never credentials.
type Profile struct {
	Name     string   `yaml:"name"`
	Provider Provider `yaml:"provider"`
	// ConfigDir is exported as CLAUDE_CONFIG_DIR (Claude profiles).
	ConfigDir string `yaml:"config_dir,omitempty"`
	// HomeDir is exported as CODEX_HOME (Codex profiles).
	HomeDir string `yaml:"home_dir,omitempty"`
}

// Agent is a named colleague maintained by Podium (§5.1). Empty optional fields
// inherit from Global.
type Agent struct {
	Name           string         `yaml:"name"`
	Provider       Provider       `yaml:"provider,omitempty"`
	Profile        string         `yaml:"profile,omitempty"`
	Model          string         `yaml:"model,omitempty"`
	Effort         string         `yaml:"effort,omitempty"`
	PermissionMode PermissionMode `yaml:"permission_mode,omitempty"`
	Fallback       []string       `yaml:"fallback,omitempty"`
	MCPConfig      string         `yaml:"mcp_config,omitempty"`
}

// Server is the web UI / API bind address.
type Server struct {
	Bind string `yaml:"bind"`
	Port int    `yaml:"port"`
}

// Load reads and validates config.yaml at the given path. The file is expected
// to exist (Scaffold writes it on first run); a missing file is an error so the
// daemon fails loudly rather than running on invisible defaults.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config %s: %w", path, err)
	}
	return &cfg, nil
}

// applyDefaults fills zero-valued fields with sane defaults so a minimal
// config.yaml still produces a working configuration.
func (c *Config) applyDefaults() {
	if c.Global.Provider == "" {
		c.Global.Provider = ProviderClaude
	}
	if c.Global.Effort == "" {
		c.Global.Effort = "medium"
	}
	if c.Global.PermissionMode == "" {
		c.Global.PermissionMode = PermissionApprove
	}
	if c.Server.Bind == "" {
		c.Server.Bind = "127.0.0.1"
	}
	if c.Server.Port == 0 {
		c.Server.Port = 8787
	}
}

// Validate checks structural and referential integrity: known enums, unique
// names, profile/provider consistency, and that agent profile references resolve.
func (c *Config) Validate() error {
	if err := validateProvider(c.Global.Provider); err != nil {
		return fmt.Errorf("global.provider: %w", err)
	}
	if err := validatePermission(c.Global.PermissionMode); err != nil {
		return fmt.Errorf("global.permission_mode: %w", err)
	}

	profileNames := map[string]Provider{}
	for i, p := range c.Profiles {
		if p.Name == "" {
			return fmt.Errorf("profiles[%d]: name is required", i)
		}
		if _, dup := profileNames[p.Name]; dup {
			return fmt.Errorf("profiles[%d]: duplicate profile name %q", i, p.Name)
		}
		if err := validateProvider(p.Provider); err != nil {
			return fmt.Errorf("profiles[%d] (%s): %w", i, p.Name, err)
		}
		switch p.Provider {
		case ProviderClaude:
			if p.ConfigDir == "" {
				return fmt.Errorf("profiles[%d] (%s): claude profile needs config_dir", i, p.Name)
			}
		case ProviderCodex:
			if p.HomeDir == "" {
				return fmt.Errorf("profiles[%d] (%s): codex profile needs home_dir", i, p.Name)
			}
		}
		profileNames[p.Name] = p.Provider
	}

	agentNames := map[string]bool{}
	for i, a := range c.Agents {
		if a.Name == "" {
			return fmt.Errorf("agents[%d]: name is required", i)
		}
		if agentNames[a.Name] {
			return fmt.Errorf("agents[%d]: duplicate agent name %q", i, a.Name)
		}
		agentNames[a.Name] = true
		if a.Provider != "" {
			if err := validateProvider(a.Provider); err != nil {
				return fmt.Errorf("agents[%d] (%s): %w", i, a.Name, err)
			}
		}
		if a.PermissionMode != "" {
			if err := validatePermission(a.PermissionMode); err != nil {
				return fmt.Errorf("agents[%d] (%s): %w", i, a.Name, err)
			}
		}
		if a.Profile != "" {
			if _, ok := profileNames[a.Profile]; !ok {
				return fmt.Errorf("agents[%d] (%s): unknown profile %q", i, a.Name, a.Profile)
			}
		}
	}

	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port out of range: %d", c.Server.Port)
	}
	return nil
}

func validateProvider(p Provider) error {
	switch p {
	case ProviderClaude, ProviderCodex:
		return nil
	default:
		return fmt.Errorf("unknown provider %q (want claude|codex)", p)
	}
}

func validatePermission(m PermissionMode) error {
	switch m {
	case PermissionApprove, PermissionYolo:
		return nil
	default:
		return fmt.Errorf("unknown permission_mode %q (want approve|yolo)", m)
	}
}
