package adapter

import (
	"context"
	"encoding/json"
	"errors"
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
	workspace := t.TempDir()
	args, cleanup, err := c.args(TurnRequest{
		Handle: Handle{ID: "claude-session"},
		Settings: TurnSettings{
			PermissionMode: config.PermissionYolo,
			WorkspaceDir:   workspace,
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
	// Skills exposure: the workspace (holding .claude/skills) is added so Claude
	// discovers the union (S6).
	if !strings.Contains(got, "--add-dir "+workspace) {
		t.Fatalf("expected --add-dir %s in args: %q", workspace, got)
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

func TestParseClaudeRateLimitErrorEvent(t *testing.T) {
	input := strings.NewReader(`{"type":"error","error":{"message":"Claude usage limit reached. Try again later."}}
`)
	out := make(chan Event, 1)
	if err := parseClaudeStream(context.Background(), input, out); err != nil {
		t.Fatalf("parse: %v", err)
	}
	close(out)
	event := <-out
	if event.Kind != EventRateLimited {
		t.Fatalf("expected rate limit event, got %+v", event)
	}
}

func TestClaudeRateLimitedText(t *testing.T) {
	for _, message := range []string{
		"rate limit exceeded",
		"usage_limit_exceeded",
		"too many requests",
		"HTTP 429 from upstream",
	} {
		if !claudeRateLimitedText(message) {
			t.Fatalf("expected %q to be classified as a rate limit", message)
		}
	}
	if claudeRateLimitedText("authentication failed") {
		t.Fatal("auth failure should not be classified as a rate limit")
	}
}

func TestClaudeWaitErrorKeepsProviderMessage(t *testing.T) {
	event, send := claudeWaitEvent(errors.New("exit status 1"), "", claudeStreamTrack{lastMessage: "claude error: not logged in"})
	if send {
		t.Fatalf("expected provider message to be preserved without generic replacement, got send=%v event=%+v", send, event)
	}
}

func TestClaudeWaitErrorUsesStderrWhenNoProviderMessage(t *testing.T) {
	event, send := claudeWaitEvent(errors.New("exit status 1"), "not logged in", claudeStreamTrack{})
	if !send {
		t.Fatal("expected generic event when no provider message was emitted")
	}
	if event.Kind != EventAssistantMessage || !strings.Contains(event.Content, "not logged in") {
		t.Fatalf("unexpected event: %+v", event)
	}
}

func TestCollectStderrKeepsTail(t *testing.T) {
	got := collectStderr(strings.NewReader("0123456789abcdef"), 6)
	if got.err != nil {
		t.Fatalf("collect stderr: %v", got.err)
	}
	if got.text != "abcdef" {
		t.Fatalf("expected stderr tail, got %q", got.text)
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
