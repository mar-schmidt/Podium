package core

import (
	"context"
	"strings"
	"time"

	"github.com/mar-schmidt/Podium/internal/adapter"
	"github.com/mar-schmidt/Podium/internal/config"
	"github.com/mar-schmidt/Podium/internal/store"
)

// AllowListRelay resolves permission requests for unattended runs without a
// human: it allows tools whose name is on the allow-list and denies everything
// else. An empty allow-list denies all side-effecting actions — the stricter
// default for scheduled routines (R7.8). It is the in-process decider Codex (and
// the fake adapter) consult; Claude enforces the same allow-list natively via
// `--allowedTools` (§7.7).
type AllowListRelay struct {
	allowed map[string]bool
}

// NewAllowListRelay builds a relay from a list of permitted tool names.
func NewAllowListRelay(tools []string) *AllowListRelay {
	allowed := make(map[string]bool, len(tools))
	for _, t := range tools {
		if trimmed := strings.TrimSpace(t); trimmed != "" {
			allowed[trimmed] = true
		}
	}
	return &AllowListRelay{allowed: allowed}
}

// RequestPermission decides immediately from the allow-list; it never blocks on
// a human and ignores the timeout.
func (r *AllowListRelay) RequestPermission(_ context.Context, req adapter.PermissionRequest, _ time.Duration) (adapter.PermissionDecision, error) {
	if r.allowed[req.ToolName] {
		return adapter.PermissionDecision{Behavior: "allow", UpdatedInput: req.Input}, nil
	}
	return adapter.PermissionDecision{Behavior: "deny", Message: "not in preapproved allow-list"}, nil
}

// ScheduledRunRequest describes one unattended run of a schedule against an
// agent. The body becomes the user turn layered on the agent's standing identity
// (R7.3a).
type ScheduledRunRequest struct {
	ScheduleName string
	RunID        string
	AgentName    string
	Model        string
	Effort       string
	// Yolo runs with whole-machine auto-approval. When false the run is
	// preapproved: approve mode plus the AllowedTools allow-list.
	Yolo         bool
	AllowedTools []string
	Task         string
}

// RunScheduled creates a durable schedule-origin session (R7.9), runs the task
// as one normal Podium turn with the unattended permission policy, and returns
// the created session. A non-nil error means the turn itself failed.
func (c *Core) RunScheduled(ctx context.Context, req ScheduledRunRequest) (store.Session, error) {
	permission := config.PermissionApprove
	var relay adapter.PermissionRelay
	allowed := req.AllowedTools
	if req.Yolo {
		permission = config.PermissionYolo
		allowed = nil
	} else {
		relay = NewAllowListRelay(req.AllowedTools)
	}

	sess, err := c.CreateSession(ctx, CreateSessionRequest{
		AgentName:      req.AgentName,
		Origin:         store.OriginSchedule,
		Model:          req.Model,
		Effort:         req.Effort,
		PermissionMode: permission,
		ScheduleID:     req.ScheduleName,
		RunID:          req.RunID,
	})
	if err != nil {
		return store.Session{}, err
	}

	events, err := c.StreamTurn(ctx, sess.ID, req.Task, TurnOptions{
		PermissionTurnID: req.RunID,
		PermissionRelay:  relay,
		Unattended:       true,
		AllowedTools:     allowed,
	})
	if err != nil {
		return sess, err
	}

	var turnErr string
	for event := range events {
		if event.Kind == "error" {
			turnErr = event.Content
		}
	}
	if turnErr != "" {
		return sess, &ScheduledRunError{Message: turnErr}
	}
	return sess, nil
}

// ScheduledRunError reports that an unattended turn produced an error event.
type ScheduledRunError struct {
	Message string
}

func (e *ScheduledRunError) Error() string { return e.Message }
