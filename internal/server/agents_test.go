package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/mar-schmidt/Podium/internal/adapter"
	"github.com/mar-schmidt/Podium/internal/config"
	"github.com/mar-schmidt/Podium/internal/core"
	"github.com/mar-schmidt/Podium/internal/store"
)

func TestDeleteAgentRejectsConfirmationMismatch(t *testing.T) {
	ctx := context.Background()
	_, srv, cleanup := newAgentAPITestServer(t)
	defer cleanup()
	if _, err := srv.core.CreateAgent(ctx, core.CreateAgentRequest{Name: "atlas", Provider: config.ProviderClaude}); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/agents/atlas", bytes.NewBufferString(`{"confirmation":"wrong"}`))
	rr := httptest.NewRecorder()
	srv.handleAgent(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
	if _, err := srv.core.GetAgent(ctx, "atlas"); err != nil {
		t.Fatalf("agent should remain after mismatch: %v", err)
	}
}

func TestDeleteAgentRemovesDatabaseRowAndConfigEntry(t *testing.T) {
	ctx := context.Background()
	paths, srv, cleanup := newAgentAPITestServer(t)
	defer cleanup()
	if _, err := srv.core.CreateAgent(ctx, core.CreateAgentRequest{Name: "atlas", Provider: config.ProviderClaude}); err != nil {
		t.Fatalf("create atlas: %v", err)
	}
	if _, err := srv.core.CreateAgent(ctx, core.CreateAgentRequest{Name: "builder", Provider: config.ProviderCodex}); err != nil {
		t.Fatalf("create builder: %v", err)
	}
	writeConfig(t, paths.ConfigYAML, `global:
  provider: claude
  permission_mode: approve
agents:
  - name: atlas
    provider: claude
  - name: builder
    provider: codex
server:
  bind: 127.0.0.1
  port: 8787
`)

	req := httptest.NewRequest(http.MethodDelete, "/api/agents/atlas", bytes.NewBufferString(`{"confirmation":"atlas"}`))
	rr := httptest.NewRecorder()
	srv.handleAgent(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if _, err := srv.core.GetAgent(ctx, "atlas"); err == nil {
		t.Fatal("expected atlas to be deleted from store")
	}
	if _, err := srv.core.GetAgent(ctx, "builder"); err != nil {
		t.Fatalf("builder should remain in store: %v", err)
	}
	cfg, err := config.Load(paths.ConfigYAML)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(cfg.Agents) != 1 || cfg.Agents[0].Name != "builder" {
		t.Fatalf("config agents = %+v", cfg.Agents)
	}
}

func TestDeleteAgentSucceedsWhenConfigEntryIsAbsent(t *testing.T) {
	ctx := context.Background()
	paths, srv, cleanup := newAgentAPITestServer(t)
	defer cleanup()
	if _, err := srv.core.CreateAgent(ctx, core.CreateAgentRequest{Name: "atlas", Provider: config.ProviderClaude}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	before, err := os.ReadFile(paths.ConfigYAML)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/agents/atlas", bytes.NewBufferString(`{"confirmation":"atlas"}`))
	rr := httptest.NewRecorder()
	srv.handleAgent(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	after, err := os.ReadFile(paths.ConfigYAML)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("config without matching entry should not be rewritten")
	}
}

func TestDeleteAgentFailsWhenSessionsReferenceAgent(t *testing.T) {
	ctx := context.Background()
	paths, srv, cleanup := newAgentAPITestServer(t)
	defer cleanup()
	agent, err := srv.core.CreateAgent(ctx, core.CreateAgentRequest{Name: "atlas", Provider: config.ProviderClaude})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, err := srv.core.CreateSession(ctx, core.CreateSessionRequest{AgentName: agent.Name, Origin: store.OriginCLI}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	writeConfig(t, paths.ConfigYAML, `global:
  provider: claude
  permission_mode: approve
agents:
  - name: atlas
    provider: claude
server:
  bind: 127.0.0.1
  port: 8787
`)

	req := httptest.NewRequest(http.MethodDelete, "/api/agents/atlas", bytes.NewBufferString(`{"confirmation":"atlas"}`))
	rr := httptest.NewRecorder()
	srv.handleAgent(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "existing sessions still reference this agent") {
		t.Fatalf("expected clear session-reference error, got %q", rr.Body.String())
	}
	if _, err := srv.core.GetAgent(ctx, "atlas"); err != nil {
		t.Fatalf("agent should remain after blocked delete: %v", err)
	}
	cfg, err := config.Load(paths.ConfigYAML)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(cfg.Agents) != 1 || cfg.Agents[0].Name != "atlas" {
		t.Fatalf("config should remain unchanged, got %+v", cfg.Agents)
	}
}

func newAgentAPITestServer(t *testing.T) (config.Paths, *Server, func()) {
	t.Helper()
	home := t.TempDir()
	paths := config.NewPaths(home)
	if _, err := config.Scaffold(paths); err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	if err := os.WriteFile(paths.BaseAgents, []byte("base layer\n"), 0o644); err != nil {
		t.Fatalf("write base agents: %v", err)
	}
	db, err := store.Open(paths.DB)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	coreSvc, err := core.New(core.Options{Paths: paths, Store: db, Adapter: adapter.NewFake()})
	if err != nil {
		t.Fatalf("new core: %v", err)
	}
	srv := New(Options{Bind: "127.0.0.1", Port: 0, Core: coreSvc, Paths: paths})
	return paths, srv, func() {
		if err := db.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	}
}

func writeConfig(t *testing.T, path, raw string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}
