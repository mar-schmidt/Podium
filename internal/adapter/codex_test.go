package adapter

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mar-schmidt/Podium/internal/config"
	"github.com/mar-schmidt/Podium/internal/store"
)

func TestCodexParamsUseNativePermissionModes(t *testing.T) {
	approveStart := codexThreadStartParams(StartRequest{
		Model:          "gpt-5.5",
		PermissionMode: config.PermissionApprove,
		WorkspaceDir:   "/tmp/workspace",
	})
	if approveStart["approvalPolicy"] != "on-request" || approveStart["sandbox"] != "read-only" {
		t.Fatalf("bad approve thread params: %#v", approveStart)
	}
	approveTurn := codexTurnStartParams("thread-1", "hi", TurnSettings{
		Effort:         "high",
		PermissionMode: config.PermissionApprove,
		WorkspaceDir:   "/tmp/workspace",
	})
	policy, ok := approveTurn["sandboxPolicy"].(map[string]any)
	if !ok || policy["type"] != "readOnly" || policy["networkAccess"] != false {
		t.Fatalf("bad approve turn sandbox policy: %#v", approveTurn["sandboxPolicy"])
	}
	if approveTurn["effort"] != "high" {
		t.Fatalf("turn effort missing: %#v", approveTurn)
	}

	yoloStart := codexThreadStartParams(StartRequest{
		PermissionMode: config.PermissionYolo,
		WorkspaceDir:   "/tmp/workspace",
	})
	if yoloStart["approvalPolicy"] != "never" || yoloStart["sandbox"] != "danger-full-access" {
		t.Fatalf("bad yolo thread params: %#v", yoloStart)
	}
	yoloTurn := codexTurnStartParams("thread-1", "hi", TurnSettings{
		PermissionMode: config.PermissionYolo,
		WorkspaceDir:   "/tmp/workspace",
	})
	policy, ok = yoloTurn["sandboxPolicy"].(map[string]any)
	if !ok || policy["type"] != "dangerFullAccess" {
		t.Fatalf("bad yolo turn sandbox policy: %#v", yoloTurn["sandboxPolicy"])
	}

	allow := codexApprovalResponse("item/commandExecution/requestApproval", nil, PermissionDecision{Behavior: "allow"}).(map[string]any)
	if allow["decision"] != "accept" {
		t.Fatalf("allow decision did not map to accept: %#v", allow)
	}
	deny := codexApprovalResponse("item/commandExecution/requestApproval", nil, PermissionDecision{Behavior: "deny"}).(map[string]any)
	if deny["decision"] != "decline" {
		t.Fatalf("deny decision did not map to decline: %#v", deny)
	}
}

func TestCodexReplayMessageIncludesHistoryAndLiveTurn(t *testing.T) {
	got := codexReplayMessage([]store.Message{
		{Role: store.RoleUser, Content: "remember alpha"},
		{Role: store.RoleAssistant, Content: "alpha remembered"},
	}, "continue")
	for _, want := range []string{"<podium_history>", "user: remember alpha", "assistant: alpha remembered", "Live user turn:\ncontinue"} {
		if !strings.Contains(got, want) {
			t.Fatalf("replay message missing %q:\n%s", want, got)
		}
	}
}

func TestCodexRateStatusAndLimitParsing(t *testing.T) {
	status, ok := codexRateStatus(json.RawMessage(`{"rate_limits":{"primary":{"used_percent":82.5},"secondary":{"used_percent":20}}}`))
	if !ok || status.UsedPercent != 82.5 {
		t.Fatalf("bad rate status: %+v ok=%v", status, ok)
	}
	if !codexRateLimited(json.RawMessage(`{"error":{"message":"usage_limit_exceeded"}}`)) {
		t.Fatal("expected usage limit to be detected")
	}
}

func TestCodexStreamsTurnAndRelaysApproval(t *testing.T) {
	t.Setenv("PODIUM_CODEX_FAKE_MODE", "approval")
	codex := newTestCodex(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "AGENTS.md"), []byte("workspace instructions\n"), 0o644); err != nil {
		t.Fatalf("write agents: %v", err)
	}

	handle, err := codex.Start(ctx, StartRequest{
		SessionID:      "session-1",
		Provider:       config.ProviderCodex,
		PermissionMode: config.PermissionApprove,
		WorkspaceDir:   workspace,
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	relay := &recordingRelay{behavior: "allow", requests: make(chan PermissionRequest, 1)}
	events, err := codex.SendTurn(ctx, TurnRequest{
		SessionID: "session-1",
		Handle:    handle,
		Message:   "run a command",
		Settings: TurnSettings{
			PermissionMode:   config.PermissionApprove,
			WorkspaceDir:     workspace,
			PermissionTurnID: "podium-turn-1",
		},
		Relay: relay,
	})
	if err != nil {
		t.Fatalf("send turn: %v", err)
	}

	text := collectCodexText(t, events)
	if text != "approved" {
		t.Fatalf("unexpected assistant text %q", text)
	}
	req := <-relay.requests
	if req.TurnID != "podium-turn-1" || req.ToolName != "codex.command" || req.ToolUseID != "item-1" {
		t.Fatalf("bad permission request: %+v", req)
	}
}

func TestCodexResumesThreadAfterAppServerRestart(t *testing.T) {
	t.Setenv("PODIUM_CODEX_FAKE_MODE", "normal")
	codex := newTestCodex(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "AGENTS.md"), []byte("workspace instructions\n"), 0o644); err != nil {
		t.Fatalf("write agents: %v", err)
	}

	handle, err := codex.Start(ctx, StartRequest{
		SessionID:      "session-1",
		Provider:       config.ProviderCodex,
		PermissionMode: config.PermissionYolo,
		WorkspaceDir:   workspace,
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	codex.client("").reset()

	events, err := codex.SendTurn(ctx, TurnRequest{
		SessionID: "session-1",
		Handle:    handle,
		Message:   "hello after restart",
		Settings: TurnSettings{
			PermissionMode: config.PermissionYolo,
			WorkspaceDir:   workspace,
		},
	})
	if err != nil {
		t.Fatalf("send turn after restart: %v", err)
	}
	if text := collectCodexText(t, events); text != "resumed" {
		t.Fatalf("unexpected assistant text after restart %q", text)
	}
}

func TestCodexHelperProcess(t *testing.T) {
	if os.Getenv("PODIUM_CODEX_HELPER") != "1" {
		return
	}
	runFakeCodexAppServer()
	os.Exit(0)
}

type recordingRelay struct {
	behavior string
	requests chan PermissionRequest
}

func (r *recordingRelay) RequestPermission(ctx context.Context, req PermissionRequest, timeout time.Duration) (PermissionDecision, error) {
	select {
	case r.requests <- req:
	case <-ctx.Done():
		return PermissionDecision{Behavior: "deny"}, ctx.Err()
	}
	return PermissionDecision{Behavior: r.behavior}, nil
}

func newTestCodex(t *testing.T) *Codex {
	t.Helper()
	wrapper := filepath.Join(t.TempDir(), "codex")
	script := "#!/bin/sh\nexec env PODIUM_CODEX_HELPER=1 " + strconv.Quote(os.Args[0]) + " -test.run=TestCodexHelperProcess -- \"$@\"\n"
	if err := os.WriteFile(wrapper, []byte(script), 0o755); err != nil {
		t.Fatalf("write codex wrapper: %v", err)
	}
	t.Setenv("CODEX_BIN", wrapper)
	codex, err := NewCodex(CodexOptions{PermissionTimeout: time.Second})
	if err != nil {
		t.Fatalf("new codex: %v", err)
	}
	return codex
}

func collectCodexText(t *testing.T, events <-chan Event) string {
	t.Helper()
	var text strings.Builder
	for event := range events {
		switch event.Kind {
		case EventAssistantDelta:
			text.WriteString(event.Content)
		case EventAssistantMessage:
			text.Reset()
			text.WriteString(event.Content)
		}
	}
	return text.String()
}

func runFakeCodexAppServer() {
	enc := json.NewEncoder(os.Stdout)
	scanner := bufio.NewScanner(os.Stdin)
	loaded := map[string]bool{}
	threadID := "thread-1"
	nextTurn := 0
	var pendingApproval struct {
		threadID string
		turnID   string
		active   bool
	}
	for scanner.Scan() {
		var msg codexRPCMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}
		if len(msg.ID) > 0 && msg.Method == "" {
			var resp struct {
				Result struct {
					Decision string `json:"decision"`
				} `json:"result"`
			}
			_ = json.Unmarshal(scanner.Bytes(), &resp)
			if pendingApproval.active {
				final := "denied"
				if resp.Result.Decision == "accept" {
					final = "approved"
				}
				writeFakeDelta(enc, pendingApproval.threadID, pendingApproval.turnID, final)
				writeFakeCompleted(enc, pendingApproval.threadID, pendingApproval.turnID, final)
				pendingApproval.active = false
			}
			continue
		}
		switch msg.Method {
		case "initialize":
			writeFakeResponse(enc, msg.ID, map[string]any{
				"userAgent":      "fake-codex",
				"codexHome":      "/tmp/fake-codex-home",
				"platformFamily": "unix",
				"platformOs":     "test",
			})
		case "initialized":
		case "thread/start":
			var params struct {
				CWD string `json:"cwd"`
			}
			_ = json.Unmarshal(msg.Params, &params)
			loaded[threadID] = true
			writeFakeResponse(enc, msg.ID, fakeThreadResponse(threadID, params.CWD))
		case "thread/resume":
			var params struct {
				ThreadID string `json:"threadId"`
				CWD      string `json:"cwd"`
			}
			_ = json.Unmarshal(msg.Params, &params)
			if params.ThreadID == "" {
				params.ThreadID = threadID
			}
			loaded[params.ThreadID] = true
			writeFakeResponse(enc, msg.ID, fakeThreadResponse(params.ThreadID, params.CWD))
		case "turn/start":
			var params struct {
				ThreadID string `json:"threadId"`
			}
			_ = json.Unmarshal(msg.Params, &params)
			if !loaded[params.ThreadID] {
				writeFakeError(enc, msg.ID, "thread not loaded")
				continue
			}
			nextTurn++
			turnID := fmt.Sprintf("turn-%d", nextTurn)
			writeFakeResponse(enc, msg.ID, map[string]any{"turn": map[string]any{"id": turnID}})
			if os.Getenv("PODIUM_CODEX_FAKE_MODE") == "approval" {
				pendingApproval = struct {
					threadID string
					turnID   string
					active   bool
				}{threadID: params.ThreadID, turnID: turnID, active: true}
				writeFakeRequest(enc, json.RawMessage("100"), "item/commandExecution/requestApproval", map[string]any{
					"threadId":    params.ThreadID,
					"turnId":      turnID,
					"itemId":      "item-1",
					"startedAtMs": time.Now().UnixMilli(),
					"command":     "echo ok",
					"cwd":         "/tmp",
				})
			} else {
				writeFakeDelta(enc, params.ThreadID, turnID, "res")
				writeFakeDelta(enc, params.ThreadID, turnID, "umed")
				writeFakeCompleted(enc, params.ThreadID, turnID, "resumed")
			}
		case "thread/unsubscribe":
			writeFakeResponse(enc, msg.ID, map[string]any{})
		}
	}
}

func fakeThreadResponse(threadID, cwd string) map[string]any {
	return map[string]any{
		"thread": map[string]any{
			"id":  threadID,
			"cwd": cwd,
		},
		"instructionSources": []string{filepath.Join(cwd, "AGENTS.md")},
	}
}

func writeFakeResponse(enc *json.Encoder, id json.RawMessage, result any) {
	_ = enc.Encode(map[string]any{"id": id, "result": result})
}

func writeFakeError(enc *json.Encoder, id json.RawMessage, message string) {
	_ = enc.Encode(map[string]any{
		"id": id,
		"error": map[string]any{
			"code":    -32000,
			"message": message,
		},
	})
}

func writeFakeRequest(enc *json.Encoder, id json.RawMessage, method string, params any) {
	_ = enc.Encode(map[string]any{"id": id, "method": method, "params": params})
}

func writeFakeDelta(enc *json.Encoder, threadID, turnID, delta string) {
	_ = enc.Encode(map[string]any{
		"method": "item/agentMessage/delta",
		"params": map[string]any{
			"threadId": threadID,
			"turnId":   turnID,
			"itemId":   "assistant-1",
			"delta":    delta,
		},
	})
}

func writeFakeCompleted(enc *json.Encoder, threadID, turnID, text string) {
	_ = enc.Encode(map[string]any{
		"method": "turn/completed",
		"params": map[string]any{
			"threadId": threadID,
			"turn": map[string]any{
				"id": turnID,
				"items": []map[string]any{{
					"type":  "agentMessage",
					"id":    "assistant-1",
					"text":  text,
					"phase": "final_answer",
				}},
			},
		},
	})
}
