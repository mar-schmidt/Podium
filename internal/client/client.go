// Package client is the thin transport the `podium` CLI uses to talk to a
// running podiumd over HTTP. The CLI never runs sessions in-process — it is
// always a client of the daemon (R11.1 / D2).
package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/mar-schmidt/Podium/internal/server"
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
