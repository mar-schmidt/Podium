package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mar-schmidt/Podium/internal/adapter"
	"github.com/mar-schmidt/Podium/internal/config"
	"github.com/mar-schmidt/Podium/internal/store"
)

// CreateSessionRequest creates a durable session bound to an agent. Empty
// settings inherit the agent defaults; origin is immutable after creation.
type CreateSessionRequest struct {
	AgentName      string
	Origin         store.SessionOrigin
	Profile        string
	Model          string
	Effort         string
	PermissionMode config.PermissionMode
	ScheduleID     string
	RunID          string
}

// CreateSession creates a durable session and starts a fake/provider backing
// handle that can be resumed later.
func (c *Core) CreateSession(ctx context.Context, req CreateSessionRequest) (store.Session, error) {
	agent, err := c.store.GetAgent(ctx, req.AgentName)
	if err != nil {
		return store.Session{}, err
	}
	sess := store.Session{
		AgentName:      agent.Name,
		Provider:       agent.Provider,
		Profile:        firstNonEmpty(req.Profile, agent.Profile),
		Model:          firstNonEmpty(req.Model, agent.Model),
		Effort:         firstNonEmpty(req.Effort, agent.Effort),
		PermissionMode: agent.PermissionMode,
		Origin:         req.Origin,
		ScheduleID:     req.ScheduleID,
		RunID:          req.RunID,
	}
	if req.PermissionMode != "" {
		sess.PermissionMode = req.PermissionMode
	}
	if sess.Origin == "" {
		return store.Session{}, fmt.Errorf("session origin is required")
	}

	created, err := c.store.CreateSession(ctx, sess)
	if err != nil {
		return store.Session{}, err
	}

	payload, err := c.ComposeInstructions(ctx, agent)
	if err != nil {
		return store.Session{}, err
	}
	handle, err := c.adapter.Start(ctx, adapter.StartRequest{
		SessionID:      created.ID,
		AgentName:      agent.Name,
		Provider:       created.Provider,
		Profile:        created.Profile,
		ProfileDir:     c.profileDir(created.Provider, created.Profile),
		Model:          created.Model,
		Effort:         created.Effort,
		PermissionMode: created.PermissionMode,
		WorkspaceDir:   c.AgentPaths(agent.Name).Workspace,
		Instructions:   payload.Bytes,
	})
	if err != nil {
		return store.Session{}, err
	}
	return c.store.UpdateSessionProviderHandle(ctx, created.ID, handle.ID)
}

// ListSessions returns all durable sessions.
func (c *Core) ListSessions(ctx context.Context) ([]store.Session, error) {
	return c.store.ListSessions(ctx)
}

// GetSession fetches a durable session.
func (c *Core) GetSession(ctx context.Context, id string) (store.Session, error) {
	return c.store.GetSession(ctx, id)
}

// TurnOptions configures one live adapter turn.
type TurnOptions struct {
	PermissionTurnID string
	PermissionRelay  adapter.PermissionRelay
}

// TurnEvent is streamed by core while an adapter turn is running.
type TurnEvent struct {
	Kind              adapter.EventKind
	Content           string
	PermissionRequest *adapter.PermissionRequest
	Message           *store.Message
}

// AppendTurn persists the user turn, drives the adapter, persists the assistant
// reply, and returns the new history entries.
func (c *Core) AppendTurn(ctx context.Context, sessionID, userMessage string) ([]store.Message, error) {
	events, err := c.StreamTurn(ctx, sessionID, userMessage, TurnOptions{})
	if err != nil {
		return nil, err
	}
	var messages []store.Message
	for event := range events {
		if event.Message != nil {
			messages = append(messages, *event.Message)
		}
	}
	return messages, nil
}

// StreamTurn persists the user turn, streams adapter events, persists the final
// assistant reply, and emits the newly stored messages.
func (c *Core) StreamTurn(ctx context.Context, sessionID, userMessage string, opts TurnOptions) (<-chan TurnEvent, error) {
	if strings.TrimSpace(userMessage) == "" {
		return nil, fmt.Errorf("user message is required")
	}
	sess, err := c.store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	history, err := c.store.ListMessages(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	userMessages, err := c.store.AppendMessages(ctx, sessionID, []store.Message{{
		Role:    store.RoleUser,
		Content: userMessage,
	}})
	if err != nil {
		return nil, err
	}
	events, err := c.adapter.SendTurn(ctx, adapter.TurnRequest{
		SessionID: sessionID,
		Handle: adapter.Handle{
			Provider: sess.Provider,
			ID:       sess.ProviderHandle,
		},
		Message: userMessage,
		History: history,
		Settings: adapter.TurnSettings{
			AgentName:        sess.AgentName,
			Profile:          sess.Profile,
			ProfileDir:       c.profileDir(sess.Provider, sess.Profile),
			Model:            sess.Model,
			Effort:           sess.Effort,
			PermissionMode:   sess.PermissionMode,
			WorkspaceDir:     c.AgentPaths(sess.AgentName).Workspace,
			PermissionTurnID: firstNonEmpty(opts.PermissionTurnID, fmt.Sprintf("%s-%d", sessionID, time.Now().UnixNano())),
		},
		Relay: opts.PermissionRelay,
	})
	if err != nil {
		return nil, err
	}

	streamOut := make(chan TurnEvent, 16)
	go func() {
		defer close(streamOut)
		for _, msg := range userMessages {
			msg := msg
			if !sendTurnEvent(ctx, streamOut, TurnEvent{Kind: "message_stored", Message: &msg}) {
				return
			}
		}
		var assistant strings.Builder
		for event := range events {
			switch event.Kind {
			case adapter.EventAssistantDelta:
				assistant.WriteString(event.Content)
				if !sendTurnEvent(ctx, streamOut, TurnEvent{Kind: event.Kind, Content: event.Content}) {
					return
				}
			case adapter.EventAssistantMessage:
				assistant.Reset()
				assistant.WriteString(event.Content)
				if !sendTurnEvent(ctx, streamOut, TurnEvent{Kind: event.Kind, Content: event.Content}) {
					return
				}
			case adapter.EventPermissionRequest:
				if !sendTurnEvent(ctx, streamOut, TurnEvent{Kind: event.Kind, PermissionRequest: event.PermissionRequest}) {
					return
				}
			case adapter.EventHandleUpdated:
				if event.Handle != nil {
					if _, err := c.store.UpdateSessionProviderHandle(ctx, sessionID, event.Handle.ID); err != nil {
						_ = sendTurnEvent(ctx, streamOut, TurnEvent{Kind: "error", Content: err.Error()})
						return
					}
				}
			case adapter.EventTurnDone:
			}
		}
		if assistant.Len() == 0 {
			return
		}
		assistantMessages, err := c.store.AppendMessages(ctx, sessionID, []store.Message{{
			Role:    store.RoleAssistant,
			Content: assistant.String(),
		}})
		if err != nil {
			_ = sendTurnEvent(ctx, streamOut, TurnEvent{Kind: "error", Content: err.Error()})
			return
		}
		for _, msg := range assistantMessages {
			msg := msg
			if !sendTurnEvent(ctx, streamOut, TurnEvent{Kind: "message_stored", Message: &msg}) {
				return
			}
		}
		go c.autoNameSessionBackground(sessionID)
		_ = sendTurnEvent(ctx, streamOut, TurnEvent{Kind: adapter.EventTurnDone})
	}()
	return streamOut, nil
}

// History returns a session's canonical history.
func (c *Core) History(ctx context.Context, sessionID string) ([]store.Message, error) {
	return c.store.ListMessages(ctx, sessionID)
}

// ComposeInstructions composes instructions using the session provider's
// delivery mode without sending them to a real provider.
func (c *Core) ComposeInstructions(ctx context.Context, agent store.Agent) (InstructionPayload, error) {
	switch agent.Provider {
	case "claude":
		return c.composer.Compose(ctx, agent, DeliveryClaudeImport)
	case "codex":
		return c.composer.Compose(ctx, agent, DeliveryCodexBundle)
	default:
		return InstructionPayload{}, fmt.Errorf("unknown provider %q", agent.Provider)
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func sendTurnEvent(ctx context.Context, ch chan<- TurnEvent, event TurnEvent) bool {
	select {
	case <-ctx.Done():
		return false
	case ch <- event:
		return true
	}
}
