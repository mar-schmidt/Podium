// Package client is the thin transport the `podium` CLI uses to talk to a
// running podiumd over HTTP. The CLI never runs sessions in-process — it is
// always a client of the daemon (R11.1 / D2).
package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/mar-schmidt/Podium/internal/adapter"
	"github.com/mar-schmidt/Podium/internal/config"
	"github.com/mar-schmidt/Podium/internal/server"
	"github.com/mar-schmidt/Podium/internal/store"
)

// ErrDaemonUnreachable indicates podiumd is not accepting connections at the
// configured address (most commonly: it isn't running).
var ErrDaemonUnreachable = errors.New("podiumd is not reachable")

// Client talks to podiumd at a base URL like http://127.0.0.1:8787.
type Client struct {
	baseURL string
	http    *http.Client
}

// New returns a client for the given host:port.
func New(addr string) *Client {
	return &Client{
		baseURL: "http://" + addr,
		http:    &http.Client{Timeout: 5 * time.Second},
	}
}

// Health fetches /healthz. It maps connection refusals to ErrDaemonUnreachable so
// the CLI can print a helpful "start the daemon" message instead of a raw error.
func (c *Client) Health(ctx context.Context) (server.Health, error) {
	var h server.Health
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/healthz", nil)
	if err != nil {
		return h, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		var netErr *net.OpError
		if errors.As(err, &netErr) {
			return h, fmt.Errorf("%w at %s", ErrDaemonUnreachable, c.baseURL)
		}
		return h, fmt.Errorf("%w: %v", ErrDaemonUnreachable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return h, fmt.Errorf("unexpected status %d from %s", resp.StatusCode, c.baseURL)
	}
	if err := json.NewDecoder(resp.Body).Decode(&h); err != nil {
		return h, fmt.Errorf("decode health: %w", err)
	}
	return h, nil
}

// AgentCreateRequest is the CLI transport shape for POST /api/agents.
type AgentCreateRequest struct {
	Name           string                `json:"name"`
	Provider       config.Provider       `json:"provider,omitempty"`
	Profile        string                `json:"profile,omitempty"`
	Model          string                `json:"model,omitempty"`
	Effort         string                `json:"effort,omitempty"`
	PermissionMode config.PermissionMode `json:"permission_mode,omitempty"`
}

// CreateAgent creates an agent through the daemon.
func (c *Client) CreateAgent(ctx context.Context, req AgentCreateRequest) (store.Agent, error) {
	var agent store.Agent
	if err := c.postJSON(ctx, "/api/agents", req, &agent); err != nil {
		return agent, err
	}
	return agent, nil
}

// ListAgents lists agents from the daemon.
func (c *Client) ListAgents(ctx context.Context) ([]store.Agent, error) {
	var agents []store.Agent
	if err := c.getJSON(ctx, "/api/agents", &agents); err != nil {
		return nil, err
	}
	return agents, nil
}

// ChatRequest sends one message, either to an existing session or to a new
// session created from AgentName.
type ChatRequest struct {
	SessionID string `json:"session_id,omitempty"`
	AgentName string `json:"agent_name,omitempty"`
	Message   string `json:"message"`
}

// StreamEvent is one newline-delimited event from /api/chat.
type StreamEvent struct {
	Type    string                     `json:"type"`
	Session *store.Session             `json:"session,omitempty"`
	Message *store.Message             `json:"message,omitempty"`
	Delta   string                     `json:"delta,omitempty"`
	Notice  string                     `json:"notice,omitempty"`
	Request *adapter.PermissionRequest `json:"request,omitempty"`
	Error   string                     `json:"error,omitempty"`
}

// Chat streams one turn from the daemon.
func (c *Client) Chat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, <-chan error) {
	events := make(chan StreamEvent)
	errs := make(chan error, 1)
	go func() {
		defer close(events)
		defer close(errs)
		raw, _ := json.Marshal(req)
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(raw))
		if err != nil {
			errs <- err
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpClient := &http.Client{}
		resp, err := httpClient.Do(httpReq)
		if err != nil {
			errs <- err
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			errs <- fmt.Errorf("chat status %d: %s", resp.StatusCode, bytes.TrimSpace(body))
			return
		}
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
		for scanner.Scan() {
			var event StreamEvent
			if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
				errs <- err
				return
			}
			select {
			case <-ctx.Done():
				errs <- ctx.Err()
				return
			case events <- event:
			}
		}
		if err := scanner.Err(); err != nil {
			errs <- err
		}
	}()
	return events, errs
}

// DecidePermission sends an allow/deny decision for a pending permission
// request.
func (c *Client) DecidePermission(ctx context.Context, id string, decision adapter.PermissionDecision) error {
	return c.postJSON(ctx, "/api/permission-decisions/"+id, decision, nil)
}

func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GET %s status %d: %s", path, resp.StatusCode, bytes.TrimSpace(body))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) postJSON(ctx context.Context, path string, in any, out any) error {
	raw, _ := json.Marshal(in)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("POST %s status %d: %s", path, resp.StatusCode, bytes.TrimSpace(body))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
