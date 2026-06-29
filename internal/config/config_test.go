package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultConfigIsValid(t *testing.T) {
	// The shipped default config.yaml must always load and validate — it's what a
	// fresh install runs on.
	dir := t.TempDir()
	p := NewPaths(dir)
	if _, err := Scaffold(p); err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	cfg, err := Load(p.ConfigYAML)
	if err != nil {
		t.Fatalf("load default config: %v", err)
	}
	if cfg.Server.Port != 8787 {
		t.Errorf("default port = %d, want 8787", cfg.Server.Port)
	}
	if cfg.Global.PermissionMode != PermissionApprove {
		t.Errorf("default permission mode = %q, want approve", cfg.Global.PermissionMode)
	}
}

func TestValidateRejectsUnknownProfileReference(t *testing.T) {
	c := &Config{
		Global: Global{Provider: ProviderClaude, PermissionMode: PermissionApprove},
		Agents: []Agent{{Name: "a", Profile: "ghost"}},
		Server: Server{Bind: "127.0.0.1", Port: 8787},
	}
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for unknown profile reference, got nil")
	}
}

func TestValidateRejectsProfileProviderMismatch(t *testing.T) {
	c := &Config{
		Global:   Global{Provider: ProviderClaude, PermissionMode: PermissionApprove, PermissionTimeout: "2m"},
		Profiles: []Profile{{Name: "codex-main", Provider: ProviderCodex, HomeDir: "/tmp/codex"}},
		Agents:   []Agent{{Name: "a", Provider: ProviderClaude, Profile: "codex-main"}},
		Server:   Server{Bind: "127.0.0.1", Port: 8787},
	}
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for provider/profile mismatch, got nil")
	}
}

func TestValidateChecksFallbackEntries(t *testing.T) {
	c := &Config{
		Global:   Global{Provider: ProviderClaude, PermissionMode: PermissionApprove, PermissionTimeout: "2m", Fallback: []string{"default", "ghost"}},
		Profiles: []Profile{{Name: "work", Provider: ProviderClaude, ConfigDir: "/tmp/claude"}},
		Agents:   []Agent{{Name: "a"}},
		Server:   Server{Bind: "127.0.0.1", Port: 8787},
	}
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for unknown fallback profile, got nil")
	}
}

func TestValidateRejectsDuplicateAgentNames(t *testing.T) {
	c := &Config{
		Global: Global{Provider: ProviderClaude, PermissionMode: PermissionApprove},
		Agents: []Agent{{Name: "dup"}, {Name: "dup"}},
		Server: Server{Bind: "127.0.0.1", Port: 8787},
	}
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for duplicate agent names, got nil")
	}
}

func TestScaffoldIsIdempotentAndPreservesEdits(t *testing.T) {
	dir := t.TempDir()
	p := NewPaths(dir)

	res, err := Scaffold(p)
	if err != nil {
		t.Fatalf("first scaffold: %v", err)
	}
	if !res.CreatedConfig || !res.CreatedBaseAgents || !res.CreatedProjects {
		t.Fatalf("first scaffold should create seed files, got %+v", res)
	}

	// Simulate a user edit, then re-scaffold; the edit must survive.
	edited := []byte("global:\n  provider: codex\n  permission_mode: approve\nserver:\n  bind: 127.0.0.1\n  port: 9001\n")
	if err := os.WriteFile(p.ConfigYAML, edited, 0o644); err != nil {
		t.Fatal(err)
	}
	res2, err := Scaffold(p)
	if err != nil {
		t.Fatalf("second scaffold: %v", err)
	}
	if res2.CreatedConfig {
		t.Error("second scaffold should not recreate existing config.yaml")
	}
	cfg, err := Load(p.ConfigYAML)
	if err != nil {
		t.Fatalf("load edited config: %v", err)
	}
	if cfg.Server.Port != 9001 || cfg.Global.Provider != ProviderCodex {
		t.Errorf("user edits not preserved: got port=%d provider=%s", cfg.Server.Port, cfg.Global.Provider)
	}
}

func TestResolveHomeUsesEnvOverride(t *testing.T) {
	want := filepath.Join(t.TempDir(), "custom")
	t.Setenv(EnvHome, want)
	got, err := ResolveHome()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("ResolveHome() = %q, want %q", got, want)
	}
}

// A relative PODIUM_HOME must be anchored to an absolute path at resolution
// time so a daemon launched from any cwd resolves the same root (R10.2).
func TestResolveHomeMakesRelativeOverrideAbsolute(t *testing.T) {
	t.Setenv(EnvHome, "podium-data")
	got, err := ResolveHome()
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("ResolveHome() = %q, want an absolute path", got)
	}
}
