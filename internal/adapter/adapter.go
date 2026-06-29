// Package adapter defines the provider seam between Podium core and local LLM
// CLIs. Phase 1 ships only a deterministic fake implementation; real Claude and
// Codex process handling lands in later phases.
package adapter

import (
	"context"

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
}

// EventKind classifies streamed adapter output.
type EventKind string

const (
	// EventAssistantDelta is an incremental assistant text chunk.
	EventAssistantDelta EventKind = "assistant_delta"
	// EventAssistantMessage is the final assistant message for the turn.
	EventAssistantMessage EventKind = "assistant_message"
	// EventHandleUpdated carries a replacement resumable provider handle.
	EventHandleUpdated EventKind = "handle_updated"
	// EventTurnDone marks the end of a turn stream.
	EventTurnDone EventKind = "turn_done"
)

// Event is one streamed provider event.
type Event struct {
	Kind    EventKind
	Content string
	Handle  *Handle
}

// Adapter abstracts over provider process models: per-turn Claude processes and
// a long-lived Codex app-server both fit this start/resume/send/teardown shape.
type Adapter interface {
	Start(context.Context, StartRequest) (Handle, error)
	Resume(context.Context, ResumeRequest) (Handle, error)
	SendTurn(context.Context, TurnRequest) (<-chan Event, error)
	Teardown(context.Context, Handle) error
}
