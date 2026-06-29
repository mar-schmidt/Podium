package store

import (
	"encoding/json"
	"strings"
	"testing"
)

// MCPConfig is sensitive (it may embed server commands, local URLs, tokens, or
// credentials) and must never reach a client or a log line. Both the REST and
// WebSocket surfaces serialize store.Agent through encoding/json, so a guard on
// the marshalled form locks the contract in one place (R8.29).
func TestAgentJSONRedactsMCPConfig(t *testing.T) {
	secret := "https://internal.example/mcp?token=SUPERSECRET"
	raw, err := json.Marshal(Agent{Name: "builder", MCPConfig: secret})
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	if strings.Contains(got, secret) {
		t.Fatalf("agent JSON leaked MCP config secret: %s", got)
	}
	if strings.Contains(got, "MCPConfig") || strings.Contains(strings.ToLower(got), "mcp") {
		t.Fatalf("agent JSON exposed an MCP key: %s", got)
	}
	// Sanity: non-sensitive fields are still present for clients.
	if !strings.Contains(got, "builder") {
		t.Fatalf("agent JSON unexpectedly dropped Name: %s", got)
	}
}
