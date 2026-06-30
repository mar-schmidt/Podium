package adapter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mar-schmidt/Podium/internal/config"
	podiumexec "github.com/mar-schmidt/Podium/internal/exec"
	podiumlog "github.com/mar-schmidt/Podium/internal/logging"
	"github.com/mar-schmidt/Podium/internal/store"
)

const defaultPermissionTimeout = 2 * time.Minute
const claudeStderrTailLimit = 16 * 1024

// ClaudeOptions configures the Claude Code adapter.
type ClaudeOptions struct {
	Discovery         podiumexec.Discovery
	DaemonAddr        string
	PermissionTimeout time.Duration
	MCPCommand        string
	Logger            *slog.Logger
}

// Claude drives Claude Code as a per-turn process.
type Claude struct {
	bin               string
	daemonAddr        string
	permissionTimeout time.Duration
	mcpCommand        string
	log               *slog.Logger
}

// NewClaude discovers the Claude Code CLI and returns an adapter.
func NewClaude(opts ClaudeOptions) (*Claude, error) {
	found, err := opts.Discovery.Find("claude")
	if err != nil {
		return nil, err
	}
	timeout := opts.PermissionTimeout
	if timeout == 0 {
		timeout = defaultPermissionTimeout
	}
	mcpCommand := opts.MCPCommand
	if mcpCommand == "" {
		if exe, err := os.Executable(); err == nil {
			mcpCommand = exe
		}
	}
	return &Claude{
		bin:               found.Path,
		daemonAddr:        opts.DaemonAddr,
		permissionTimeout: timeout,
		mcpCommand:        mcpCommand,
		log:               loggerOrDefault(opts.Logger),
	}, nil
}

// Start returns the existing Claude handle shape. Claude only yields a real
// session ID after the first turn, so a new session starts with an empty handle.
func (c *Claude) Start(ctx context.Context, req StartRequest) (Handle, error) {
	if err := ctx.Err(); err != nil {
		return Handle{}, err
	}
	return Handle{Provider: config.ProviderClaude}, nil
}

// Resume returns the persisted Claude session ID unchanged.
func (c *Claude) Resume(ctx context.Context, req ResumeRequest) (Handle, error) {
	if err := ctx.Err(); err != nil {
		return Handle{}, err
	}
	return req.Handle, nil
}

// SendTurn launches one `claude -p` process and streams parsed events.
func (c *Claude) SendTurn(ctx context.Context, req TurnRequest) (<-chan Event, error) {
	if req.Settings.WorkspaceDir == "" {
		return nil, errors.New("claude workspace dir is required")
	}
	args, cleanup, err := c.args(req)
	if err != nil {
		c.providerLog(req).Warn("provider turn setup failed", "stage", "args", "error", err)
		return nil, err
	}
	cmd := podiumexec.Command(ctx, c.bin, args...)
	cmd.Dir = req.Settings.WorkspaceDir
	cmd.Env = c.env(req.Settings.ProfileDir)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cleanup()
		c.providerLog(req).Warn("provider process pipe failed", "stage", "stdin", "error", err)
		return nil, fmt.Errorf("claude stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cleanup()
		c.providerLog(req).Warn("provider process pipe failed", "stage", "stdout", "error", err)
		return nil, fmt.Errorf("claude stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cleanup()
		c.providerLog(req).Warn("provider process pipe failed", "stage", "stderr", "error", err)
		return nil, fmt.Errorf("claude stderr: %w", err)
	}
	c.providerLog(req).Debug("provider process starting", "stage", "start", "command", c.bin, "resuming", req.Handle.ID != "")
	if err := cmd.Start(); err != nil {
		cleanup()
		c.providerLog(req).Warn("provider process start failed", "stage", "start", "error", err)
		return nil, fmt.Errorf("start claude: %w", err)
	}

	if err := writeClaudeInput(stdin, req.Message, req.History, req.Handle.ID != ""); err != nil {
		_ = podiumexec.Kill(cmd)
		cleanup()
		c.providerLog(req).Warn("provider stdin write failed", "stage", "write_input", "error", err)
		return nil, err
	}

	out := make(chan Event, 32)
	go func() {
		defer cleanup()
		defer close(out)
		parsec := make(chan error, 1)
		stderrc := make(chan stderrResult, 1)
		go func() { parsec <- parseClaudeStream(ctx, stdout, out) }()
		go func() { stderrc <- collectStderr(stderr, claudeStderrTailLimit) }()
		waitErr := cmd.Wait()
		parseErr := <-parsec
		stderrResult := <-stderrc
		if ctx.Err() != nil {
			return
		}
		if parseErr != nil {
			c.providerLog(req).Warn("provider stream parse failed", "stage", "parse_stdout", "error", podiumlog.Redact(parseErr.Error()))
			sendAdapterEvent(ctx, out, Event{Kind: EventAssistantMessage, Content: fmt.Sprintf("claude stream error: %v", parseErr)})
			return
		}
		if stderrResult.err != nil {
			c.providerLog(req).Warn("provider stderr read failed", "stage", "read_stderr", "error", stderrResult.err, "stderr_tail", podiumlog.RedactTail(stderrResult.text, claudeStderrTailLimit))
			sendAdapterEvent(ctx, out, Event{Kind: EventAssistantMessage, Content: fmt.Sprintf("claude stderr error: %v", stderrResult.err)})
			return
		}
		if waitErr != nil {
			if claudeRateLimitedText(stderrResult.text) {
				c.providerLog(req).Warn("provider rate limited", "stage", "wait", "rate_limited", true, "stderr_tail", podiumlog.RedactTail(stderrResult.text, claudeStderrTailLimit))
				sendAdapterEvent(ctx, out, Event{Kind: EventRateLimited, Content: stderrResult.text})
				return
			}
			message := fmt.Sprintf("claude exited with error: %v", waitErr)
			if stderrResult.text != "" {
				message += ": " + stderrResult.text
			}
			c.providerLog(req).Warn("provider process exited with error", "stage", "wait", "exit_error", waitErr, "stderr_tail", podiumlog.RedactTail(stderrResult.text, claudeStderrTailLimit))
			sendAdapterEvent(ctx, out, Event{Kind: EventAssistantMessage, Content: message})
			return
		}
		sendAdapterEvent(ctx, out, Event{Kind: EventTurnDone})
	}()
	return out, nil
}

func (c *Claude) providerLog(req TurnRequest) *slog.Logger {
	return c.log.With(
		"provider", string(config.ProviderClaude),
		"profile", req.Settings.Profile,
		"session", req.SessionID,
		"agent", req.Settings.AgentName,
	)
}

// Teardown has no persistent Claude process to stop.
func (c *Claude) Teardown(ctx context.Context, handle Handle) error {
	return ctx.Err()
}

func (c *Claude) args(req TurnRequest) ([]string, func(), error) {
	args := []string{
		"-p",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--include-partial-messages",
		"--verbose",
		"--replay-user-messages",
	}
	// Expose the skills union: the agent's workspace contains a .claude/skills
	// link to ~/.agents/skills, and --add-dir brings that scope into Claude's
	// discovery without touching CLAUDE_CONFIG_DIR (S6/S7).
	if req.Settings.WorkspaceDir != "" {
		args = append(args, "--add-dir", req.Settings.WorkspaceDir)
	}
	if req.Settings.Model != "" {
		args = append(args, "--model", req.Settings.Model)
	}
	if req.Settings.Effort != "" {
		args = append(args, "--effort", req.Settings.Effort)
	}
	if req.Handle.ID != "" {
		args = append(args, "--resume", req.Handle.ID)
	}
	cleanup := func() {}
	switch req.Settings.PermissionMode {
	case config.PermissionYolo:
		args = append(args, "--permission-mode", "bypassPermissions")
	default:
		// Unattended (scheduled) preapproved run: there is no human to answer a
		// prompt, so use Claude's native allow-list and rely on `claude -p`
		// auto-denying anything not pre-approved — no permission MCP relay (§7.7).
		if req.Settings.Unattended {
			if allowed := nonEmptyTools(req.Settings.AllowedTools); len(allowed) > 0 {
				args = append(args, "--allowedTools", strings.Join(allowed, ","))
			}
			break
		}
		if c.daemonAddr == "" || c.mcpCommand == "" {
			return nil, cleanup, errors.New("claude approve mode needs daemon address and MCP command")
		}
		configPath, err := c.writeMCPConfig(req)
		if err != nil {
			return nil, cleanup, err
		}
		cleanup = func() { _ = os.Remove(configPath) }
		args = append(args,
			"--mcp-config", configPath,
			"--permission-prompt-tool", "mcp__podium_permission__prompt",
		)
	}
	return args, cleanup, nil
}

func (c *Claude) writeMCPConfig(req TurnRequest) (string, error) {
	if err := os.MkdirAll(filepath.Join(req.Settings.WorkspaceDir, ".podium"), 0o755); err != nil {
		return "", fmt.Errorf("create claude mcp dir: %w", err)
	}
	turnID := req.Settings.PermissionTurnID
	if turnID == "" {
		turnID = req.SessionID
	}
	payload := map[string]any{
		"mcpServers": map[string]any{
			"podium_permission": map[string]any{
				"command": c.mcpCommand,
				"args": []string{
					"permission-mcp",
					"--addr", c.daemonAddr,
					"--turn", turnID,
					"--timeout", c.permissionTimeout.String(),
				},
			},
		},
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(req.Settings.WorkspaceDir, ".podium", fmt.Sprintf("claude-mcp-%s.json", sanitizeFilename(turnID)))
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return "", fmt.Errorf("write claude mcp config: %w", err)
	}
	return path, nil
}

func (c *Claude) env(profileDir string) []string {
	env := os.Environ()
	if profileDir == "" {
		return unsetEnv(env, "CLAUDE_CONFIG_DIR")
	}
	return append(unsetEnv(env, "CLAUDE_CONFIG_DIR"), "CLAUDE_CONFIG_DIR="+profileDir)
}

func writeClaudeInput(stdin io.WriteCloser, message string, history []store.Message, resumed bool) error {
	defer stdin.Close()
	enc := json.NewEncoder(stdin)
	if !resumed {
		for _, msg := range history {
			if msg.Content == "" {
				continue
			}
			if err := enc.Encode(claudeInputMessage(string(msg.Role), msg.Content)); err != nil {
				return fmt.Errorf("write history to claude: %w", err)
			}
		}
	}
	if err := enc.Encode(claudeInputMessage("user", message)); err != nil {
		return fmt.Errorf("write user turn to claude: %w", err)
	}
	return nil
}

func claudeInputMessage(role, text string) map[string]any {
	return map[string]any{
		"type": role,
		"message": map[string]any{
			"role": role,
			"content": []map[string]string{
				{"type": "text", "text": text},
			},
		},
	}
}

func parseClaudeStream(ctx context.Context, r io.Reader, out chan<- Event) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		events, err := parseClaudeLine(line)
		if err != nil {
			return err
		}
		for _, event := range events {
			if !sendAdapterEvent(ctx, out, event) {
				return ctx.Err()
			}
		}
	}
	return scanner.Err()
}

func parseClaudeLine(line []byte) ([]Event, error) {
	var raw map[string]any
	if err := json.Unmarshal(line, &raw); err != nil {
		return nil, fmt.Errorf("parse claude json %q: %w", string(line), err)
	}
	var events []Event
	if id := firstString(raw, "session_id", "sessionId"); id != "" {
		events = append(events, Event{
			Kind:   EventHandleUpdated,
			Handle: &Handle{Provider: config.ProviderClaude, ID: id},
		})
	}
	eventType := firstString(raw, "type", "event")
	switch eventType {
	case "stream_event":
		if nested, ok := raw["event"].(map[string]any); ok {
			if text := extractText(nested); text != "" {
				events = append(events, Event{Kind: EventAssistantDelta, Content: text})
			}
		}
	case "assistant_delta", "text_delta", "content_block_delta":
		if text := extractText(raw); text != "" {
			events = append(events, Event{Kind: EventAssistantDelta, Content: text})
		}
	case "assistant", "message":
		if text := extractText(raw); text != "" {
			events = append(events, Event{Kind: EventAssistantMessage, Content: text})
		}
	case "result":
		if text := firstString(raw, "result", "content"); text != "" {
			events = append(events, Event{Kind: EventAssistantMessage, Content: text})
		}
	case "api_retry":
		if claudeRateLimited(raw) {
			events = append(events, Event{Kind: EventRateLimited, Content: "claude rate limited"})
		}
	case "error":
		message := claudeErrorMessage(raw)
		if claudeRateLimitedText(message) {
			events = append(events, Event{Kind: EventRateLimited, Content: message})
		} else if message != "" {
			events = append(events, Event{Kind: EventAssistantMessage, Content: "claude error: " + message})
		}
	}
	return events, nil
}

func claudeRateLimited(raw map[string]any) bool {
	for _, key := range []string{"status", "status_code", "statusCode"} {
		switch v := raw[key].(type) {
		case float64:
			if int(v) == 429 {
				return true
			}
		case string:
			if v == "429" {
				return true
			}
		}
	}
	return claudeRateLimitedText(claudeErrorMessage(raw))
}

func claudeRateLimitedText(message string) bool {
	message = strings.ToLower(message)
	return strings.Contains(message, "rate limit") ||
		strings.Contains(message, "rate_limit") ||
		strings.Contains(message, "usage limit") ||
		strings.Contains(message, "usage_limit") ||
		strings.Contains(message, "too many requests") ||
		strings.Contains(message, "429")
}

func claudeErrorMessage(raw map[string]any) string {
	if message := firstString(raw, "message", "error", "reason"); message != "" {
		return message
	}
	if errObj, ok := raw["error"].(map[string]any); ok {
		if message := firstString(errObj, "message", "error", "reason", "type", "code"); message != "" {
			return message
		}
	}
	if nested, ok := raw["event"].(map[string]any); ok {
		return claudeErrorMessage(nested)
	}
	return ""
}

func extractText(raw map[string]any) string {
	if text := firstString(raw, "text", "content"); text != "" {
		return text
	}
	if delta, ok := raw["delta"].(map[string]any); ok {
		if text := firstString(delta, "text"); text != "" {
			return text
		}
	}
	if block, ok := raw["content_block"].(map[string]any); ok {
		if text := firstString(block, "text"); text != "" {
			return text
		}
	}
	if message, ok := raw["message"].(map[string]any); ok {
		if text := contentText(message["content"]); text != "" {
			return text
		}
	}
	return contentText(raw["content"])
}

func contentText(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, item := range v {
			switch block := item.(type) {
			case string:
				parts = append(parts, block)
			case map[string]any:
				if text := firstString(block, "text", "content"); text != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "")
	default:
		return ""
	}
}

func firstString(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := raw[key].(string); ok {
			return v
		}
	}
	return ""
}

type stderrResult struct {
	text string
	err  error
}

func collectStderr(r io.Reader, limit int) stderrResult {
	tail := &limitedTail{limit: limit}
	_, err := io.Copy(tail, r)
	return stderrResult{text: strings.TrimSpace(tail.String()), err: err}
}

type limitedTail struct {
	limit int
	data  []byte
}

func (b *limitedTail) Write(p []byte) (int, error) {
	written := len(p)
	if b.limit <= 0 {
		return written, nil
	}
	if len(p) >= b.limit {
		b.data = append(b.data[:0], p[len(p)-b.limit:]...)
		return written, nil
	}
	b.data = append(b.data, p...)
	if overflow := len(b.data) - b.limit; overflow > 0 {
		copy(b.data, b.data[overflow:])
		b.data = b.data[:b.limit]
	}
	return written, nil
}

func (b *limitedTail) String() string {
	return string(b.data)
}

func unsetEnv(env []string, key string) []string {
	prefix := key + "="
	out := env[:0]
	for _, value := range env {
		if !strings.HasPrefix(value, prefix) {
			out = append(out, value)
		}
	}
	return out
}

func nonEmptyTools(tools []string) []string {
	out := make([]string, 0, len(tools))
	for _, t := range tools {
		if trimmed := strings.TrimSpace(t); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func sanitizeFilename(name string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_")
	return replacer.Replace(name)
}

func sendAdapterEvent(ctx context.Context, out chan<- Event, event Event) bool {
	select {
	case <-ctx.Done():
		return false
	case out <- event:
		return true
	}
}
