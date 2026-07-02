package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadUserFileReadsLegacyAuthEnvAndWritesEnvVars(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.yaml")
	if err := os.WriteFile(path, []byte(`mcp_servers:
  - name: github
    transport: http
    url: https://example.test/mcp
    auth_env: GITHUB_TOKEN
  - name: filesystem
    transport: stdio
    command: npx
    args: ["-y", "@modelcontextprotocol/server-filesystem"]
    env_vars: [PROJECT_ROOT]
`), 0o600); err != nil {
		t.Fatal(err)
	}
	servers, err := LoadUserFile(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(servers) != 2 {
		t.Fatalf("servers len = %d", len(servers))
	}
	if got := strings.Join(servers[0].EnvVars, ","); got != "GITHUB_TOKEN" {
		t.Fatalf("legacy auth_env not mapped to env_vars: %q", got)
	}
	if err := SaveUserFile(path, servers); err != nil {
		t.Fatalf("save: %v", err)
	}
	text := mustRead(t, path)
	if strings.Contains(text, "auth_env") {
		t.Fatalf("save should write canonical env_vars only:\n%s", text)
	}
	if !strings.Contains(text, "env_vars") {
		t.Fatalf("save missing env_vars:\n%s", text)
	}
}

func TestImportNativeConfigsAndEnvStatus(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "present")
	dir := t.TempDir()
	claudePath := filepath.Join(dir, "claude.json")
	if err := os.WriteFile(claudePath, []byte(`{"mcpServers":{"github":{"transport":"http","url":"https://example.test/mcp","env":{"GITHUB_TOKEN":"secret"}}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	claude, err := ImportClaude(claudePath)
	if err != nil {
		t.Fatalf("import claude: %v", err)
	}
	if len(claude) != 1 || claude[0].Name != "github" || claude[0].Transport != TransportHTTP {
		t.Fatalf("bad claude import: %+v", claude)
	}

	codexPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(codexPath, []byte(`
[mcp_servers.postgres]
command = "npx"
args = ["-y", "@modelcontextprotocol/server-postgres"]

[plugins."computer-use@openai-bundled".mcp_servers.computer-use]
command = "computer-use"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	codex, err := ImportCodex(codexPath)
	if err != nil {
		t.Fatalf("import codex: %v", err)
	}
	if len(codex) != 2 {
		t.Fatalf("codex import len = %d: %+v", len(codex), codex)
	}
	merged := dedupe(append(claude, codex...))
	var github Server
	for _, s := range merged {
		if s.Name == "github" {
			github = s
		}
	}
	if len(github.EnvStatus) != 1 || !github.EnvStatus[0].Set {
		t.Fatalf("env status not populated: %+v", github.EnvStatus)
	}
}

func TestCodexProfileDisablesUnassignedAndBridgesHTTP(t *testing.T) {
	old := execLookPath
	execLookPath = func(file string) (string, error) { return "/usr/local/bin/" + file, nil }
	defer func() { execLookPath = old }()

	assigned := []Server{{
		Name:      "github",
		Transport: TransportHTTP,
		URL:       "https://example.test/mcp",
	}}
	all := append(assigned, Server{
		Name:           "computer-use",
		Transport:      TransportStdio,
		Command:        "computer-use",
		CodexTablePath: `plugins."computer-use@openai-bundled".mcp_servers.computer-use`,
	})
	profile, unavailable := CodexProfile(assigned, all)
	if len(unavailable) != 0 {
		t.Fatalf("unexpected unavailable: %+v", unavailable)
	}
	for _, want := range []string{
		"[mcp_servers.github]",
		`command = "mcp-proxy"`,
		`args = ["https://example.test/mcp"]`,
		`default_tools_approval_mode = "approve"`,
		`[plugins."computer-use@openai-bundled".mcp_servers.computer-use]`,
		"enabled = false",
	} {
		if !strings.Contains(profile, want) {
			t.Fatalf("profile missing %q:\n%s", want, profile)
		}
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
