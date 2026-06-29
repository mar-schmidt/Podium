package adapter

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mar-schmidt/Podium/internal/config"
)

func TestClaudeArgsApproveWritesPermissionMCPConfig(t *testing.T) {
	workspace := t.TempDir()
	c := &Claude{
		daemonAddr:        "127.0.0.1:8787",
		permissionTimeout: time.Minute,
		mcpCommand:        "/tmp/podiumd",
	}

	args, cleanup, err := c.args(TurnRequest{
		SessionID: "session-1",
		Settings: TurnSettings{
			Model:            "sonnet",
			Effort:           "medium",
			PermissionMode:   config.PermissionApprove,
			WorkspaceDir:     workspace,
			PermissionTurnID: "turn-1",
		},
	})
	defer cleanup()
	if err != nil {
		t.Fatalf("args: %v", err)
	}
	got := strings.Join(args, " ")
	for _, want := range []string{
		"-p",
		"--input-format stream-json",
		"--output-format stream-json",
		"--model sonnet",
		"--effort medium",
		"--mcp-config",
		"--permission-prompt-tool mcp__podium_permission__prompt",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("args %q missing %q", got, want)
		}
	}
	configIndex := indexOf(args, "--mcp-config")
	if configIndex == -1 || configIndex+1 >= len(args) {
		t.Fatalf("missing mcp config path in args: %#v", args)
	}
	raw, err := os.ReadFile(args[configIndex+1])
	if err != nil {
		t.Fatalf("read mcp config: %v", err)
	}
	if !strings.Contains(string(raw), "permission-mcp") || !strings.Contains(string(raw), "turn-1") {
		t.Fatalf("unexpected mcp config:\n%s", raw)
	}
}

func TestClaudeArgsYoloBypassesPermissions(t *testing.T) {
	c := &Claude{}
	args, cleanup, err := c.args(TurnRequest{
		Handle: Handle{ID: "claude-session"},
		Settings: TurnSettings{
			PermissionMode: config.PermissionYolo,
			WorkspaceDir:   t.TempDir(),
		},
	})
	defer cleanup()
	if err != nil {
		t.Fatalf("args: %v", err)
	}
	got := strings.Join(args, " ")
	if !strings.Contains(got, "--permission-mode bypassPermissions") {
		t.Fatalf("expected yolo permissions in args: %q", got)
	}
	if !strings.Contains(got, "--resume claude-session") {
		t.Fatalf("expected resume handle in args: %q", got)
	}
}

func TestParseClaudeStream(t *testing.T) {
	input := strings.NewReader(`{"type":"system","session_id":"abc"}
{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"O"}},"session_id":"abc"}
{"type":"assistant_delta","delta":{"text":"hel"}}
{"type":"assistant","message":{"content":[{"type":"text","text":"hello"}]}}
`)
	out := make(chan Event, 8)
	if err := parseClaudeStream(context.Background(), input, out); err != nil {
		t.Fatalf("parse: %v", err)
	}
	close(out)
	var events []Event
	for event := range out {
		events = append(events, event)
	}
	if len(events) != 5 {
		b, _ := json.Marshal(events)
		t.Fatalf("expected 5 events, got %d: %s", len(events), b)
	}
	if events[0].Kind != EventHandleUpdated || events[0].Handle.ID != "abc" {
		t.Fatalf("bad handle event: %+v", events[0])
	}
	if events[1].Kind != EventHandleUpdated || events[1].Handle.ID != "abc" {
		t.Fatalf("bad nested handle event: %+v", events[1])
	}
	if events[2].Kind != EventAssistantDelta || events[2].Content != "O" {
		t.Fatalf("bad nested delta event: %+v", events[2])
	}
	if events[3].Kind != EventAssistantDelta || events[3].Content != "hel" {
		t.Fatalf("bad delta event: %+v", events[3])
	}
	if events[4].Kind != EventAssistantMessage || events[4].Content != "hello" {
		t.Fatalf("bad assistant event: %+v", events[4])
	}
}

func indexOf(values []string, want string) int {
	for i, value := range values {
		if value == want {
			return i
		}
	}
	return -1
}
