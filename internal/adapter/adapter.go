// Package adapter defines the provider seam between Podium core and local LLM
// CLIs. Phase 1 ships only a deterministic fake implementation; real Claude and
// Codex process handling lands in later phases.
package adapter

import (
	"context"
	"encoding/json"
	"time"

	"github.com/mar-schmidt/Podium/internal/config"
	"github.com/mar-schmidt/Podium/internal/store"
)

// Handle is a provider-owned resume token such as a Claude session ID or Codex
// threadId, annotated with the provider that owns it.
type Handle struct {
	Provider config.Provider
	ID       string
}

// StartRequest contains the provider-neutral data needed to create a backing
// CLI session or thread.
type StartRequest struct {
	SessionID      string
	AgentName      string
	Provider       config.Provider
	Profile        string
	ProfileDir     string
	Model          string
	Effort         string
	PermissionMode config.PermissionMode
	WorkspaceDir   string
	Instructions   []byte
}

// ResumeRequest asks an adapter to bind to an existing provider handle.
type ResumeRequest struct {
	SessionID string
	Handle    Handle
}

// TurnRequest sends one user turn to the active backing session.
type TurnRequest struct {
	SessionID string
	Handle    Handle
	Message   string
	History   []store.Message
	Settings  TurnSettings
	Relay     PermissionRelay
}

// TurnSettings are the current session settings needed by per-turn providers
// such as Claude.
type TurnSettings struct {
	AgentName        string
	Profile          string
	ProfileDir       string
	Model            string
	Effort           string
	PermissionMode   config.PermissionMode
	WorkspaceDir     string
	PermissionTurnID string
}

// RateStatus reports provider-exposed rate-limit utilization when available.
type RateStatus struct {
	UsedPercent float64
}

// EventKind classifies streamed adapter output.
type EventKind string

const (
	// EventAssistantDelta is an incremental assistant text chunk.
	EventAssistantDelta EventKind = "assistant_delta"
	// EventAssistantMessage is the final assistant message for the turn.
	EventAssistantMessage EventKind = "assistant_message"
	// EventPermissionRequest asks the client to approve or deny a tool action.
	EventPermissionRequest EventKind = "permission_request"
	// EventHandleUpdated carries a replacement resumable provider handle.
	EventHandleUpdated EventKind = "handle_updated"
	// EventRateStatus carries provider rate-limit utilization.
	EventRateStatus EventKind = "rate_status"
	// EventRateLimited reports that the active turn cannot continue on this
	// backing target because the provider rate-limited it.
	EventRateLimited EventKind = "rate_limited"
	// EventTurnDone marks the end of a turn stream.
	EventTurnDone EventKind = "turn_done"
)

// Event is one streamed provider event.
type Event struct {
	Kind              EventKind
	Content           string
	Handle            *Handle
	PermissionRequest *PermissionRequest
	RateStatus        *RateStatus
}

// Adapter abstracts over provider process models: per-turn Claude processes and
// a long-lived Codex app-server both fit this start/resume/send/teardown shape.
type Adapter interface {
	Start(context.Context, StartRequest) (Handle, error)
	Resume(context.Context, ResumeRequest) (Handle, error)
	SendTurn(context.Context, TurnRequest) (<-chan Event, error)
	Teardown(context.Context, Handle) error
}

// PermissionRequest is the provider-neutral approval payload surfaced to a user.
type PermissionRequest struct {
	ID        string          `json:"id"`
	TurnID    string          `json:"turn_id"`
	ToolName  string          `json:"tool_name"`
	ToolUseID string          `json:"tool_use_id"`
	Input     json.RawMessage `json:"input"`
	ExpiresAt time.Time       `json:"expires_at,omitempty"`
}

// PermissionDecision is returned to the provider permission mechanism.
type PermissionDecision struct {
	Behavior     string          `json:"behavior"`
	UpdatedInput json.RawMessage `json:"updatedInput,omitempty"`
	Message      string          `json:"message,omitempty"`
}

// PermissionRelay receives permission requests and waits for user decisions.
type PermissionRelay interface {
	RequestPermission(context.Context, PermissionRequest, time.Duration) (PermissionDecision, error)
}
