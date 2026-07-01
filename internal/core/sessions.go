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
	TaskID         string
	ProjectID      string
}

// CreateSession creates a durable session and starts a fake/provider backing
// handle that can be resumed later.
func (c *Core) CreateSession(ctx context.Context, req CreateSessionRequest) (store.Session, error) {
	agent, err := c.store.GetAgent(ctx, req.AgentName)
	if err != nil {
		return store.Session{}, err
	}
	projectID := strings.TrimSpace(req.ProjectID)
	if projectID != "" {
		if _, err := c.ledger.Get(projectID); err != nil {
			return store.Session{}, err
		}
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
		TaskID:         req.TaskID,
		ProjectID:      projectID,
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

	payload, err := c.ComposeInstructionsForProvider(ctx, agent, created.Provider)
	if err != nil {
		return store.Session{}, err
	}
	projectCtx, err := c.sessionProjectExecutionContext(ctx, created)
	if err != nil {
		return store.Session{}, err
	}
	handle, err := c.adapter.Start(ctx, adapter.StartRequest{
		SessionID:          created.ID,
		AgentName:          agent.Name,
		Provider:           created.Provider,
		Profile:            created.Profile,
		ProfileDir:         c.profileDir(created.Provider, created.Profile),
		Model:              created.Model,
		Effort:             created.Effort,
		PermissionMode:     created.PermissionMode,
		WorkspaceDir:       c.AgentPaths(agent.Name).Workspace,
		ExtraWorkspaceDirs: c.sessionExtraWorkspaceDirs(projectCtx),
		Instructions:       payload.Bytes,
	})
	if err != nil {
		return store.Session{}, err
	}
	return c.store.UpdateSessionProviderHandle(ctx, created.ID, handle.ID)
}

// ListSessions returns all durable sessions, with a compatibility ProjectID
// fallback for roadmap sessions created before sessions.project_id existed.
func (c *Core) ListSessions(ctx context.Context) ([]store.Session, error) {
	sessions, err := c.store.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	c.enrichLegacyProjectIDs(ctx, sessions)
	return sessions, nil
}

func (c *Core) enrichLegacyProjectIDs(ctx context.Context, sessions []store.Session) {
	tasks, err := c.store.ListTasks(ctx)
	if err != nil {
		return // sessions are still usable without project enrichment
	}
	projectByTask := make(map[string]string, len(tasks))
	for _, t := range tasks {
		projectByTask[t.ID] = t.ProjectID
	}
	for i := range sessions {
		if sessions[i].ProjectID == "" && sessions[i].TaskID != "" {
			sessions[i].ProjectID = projectByTask[sessions[i].TaskID]
		}
	}
}

// GetSession fetches a durable session.
func (c *Core) GetSession(ctx context.Context, id string) (store.Session, error) {
	sess, err := c.store.GetSession(ctx, id)
	if err != nil {
		return store.Session{}, err
	}
	if sess.ProjectID == "" && sess.TaskID != "" {
		if task, err := c.store.GetTask(ctx, sess.TaskID); err == nil {
			sess.ProjectID = task.ProjectID
		}
	}
	return sess, nil
}

// DeleteSession removes a durable session and its message history. Like agent
// deletion, it does not explicitly stop a running provider adapter — the handle
// simply becomes unreferenced — so an in-flight turn should be avoided.
func (c *Core) DeleteSession(ctx context.Context, id string) error {
	if _, err := c.store.GetSession(ctx, id); err != nil {
		return err
	}
	return c.store.DeleteSession(ctx, id)
}

// TurnOptions configures one live adapter turn.
type TurnOptions struct {
	PermissionTurnID string
	PermissionRelay  adapter.PermissionRelay
	UserInputRelay   adapter.UserInputRelay
	// Unattended marks a turn with no human approver (a scheduled run). It and
	// AllowedTools select the provider's preapproved policy (§7.7).
	Unattended   bool
	AllowedTools []string
}

// TurnEvent is streamed by core while an adapter turn is running.
type TurnEvent struct {
	Kind              adapter.EventKind
	Content           string
	PermissionRequest *adapter.PermissionRequest
	UserInputRequest  *adapter.UserInputRequest
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
	streamOut := make(chan TurnEvent, 16)
	go func() {
		defer close(streamOut)
		for _, msg := range userMessages {
			msg := msg
			if !sendTurnEvent(ctx, streamOut, TurnEvent{Kind: "message_stored", Message: &msg}) {
				return
			}
		}

		current := sess
		tried := map[string]bool{}
		runLog := c.log.With(
			"event", "run",
			"session", sess.ID,
			"agent", sess.AgentName,
			"origin", string(sess.Origin),
			"unattended", opts.Unattended,
		)
		runLog.Info("turn started",
			"provider", string(current.Provider),
			"profile", current.Profile,
			"permission", string(current.PermissionMode),
		)
		for {
			tried[targetKey(current.Provider, current.Profile)] = true
			if err := c.ensureSessionInstructions(ctx, current); err != nil {
				runLog.Warn("turn failed", "stage", "compose", "error", err)
				_ = sendTurnEvent(ctx, streamOut, TurnEvent{Kind: "error", Content: err.Error()})
				return
			}
			projectCtx, err := c.sessionProjectExecutionContext(ctx, current)
			if err != nil {
				runLog.Warn("turn failed", "stage", "project_context", "error", err)
				_ = sendTurnEvent(ctx, streamOut, TurnEvent{Kind: "error", Content: err.Error()})
				return
			}
			providerMessage := userMessage
			if strings.TrimSpace(projectCtx.Prompt) != "" {
				providerMessage = projectCtx.Prompt + "\n\nUser message:\n" + userMessage
			}
			events, err := c.adapter.SendTurn(ctx, c.turnRequest(current, history, providerMessage, opts, c.sessionExtraWorkspaceDirs(projectCtx)))
			if err != nil {
				runLog.Warn("turn failed", "stage", "dispatch", "provider", string(current.Provider), "error", err)
				_ = sendTurnEvent(ctx, streamOut, TurnEvent{Kind: "error", Content: err.Error()})
				return
			}
			assistant, rateLimited, ok := c.consumeAdapterEvents(ctx, streamOut, sessionID, events)
			if !ok {
				runLog.Info("turn aborted", "provider", string(current.Provider))
				return
			}
			if rateLimited {
				next, err := c.nextFallbackSession(ctx, current, tried)
				if err != nil {
					runLog.Warn("turn failed", "stage", "fallback", "from", targetLabel(current.Provider, current.Profile), "error", err)
					_ = sendTurnEvent(ctx, streamOut, TurnEvent{Kind: "error", Content: err.Error()})
					return
				}
				runLog.Info("turn fallback",
					"from", targetLabel(current.Provider, current.Profile),
					"to", targetLabel(next.Provider, next.Profile),
					"fallback_from", targetLabel(current.Provider, current.Profile),
					"fallback_to", targetLabel(next.Provider, next.Profile),
					"rate_limited", true,
				)
				current = next
				continue
			}
			if assistant.Len() == 0 {
				runLog.Info("turn finished", "provider", string(current.Provider), "reply_bytes", 0)
				return
			}
			assistantMessages, err := c.store.AppendMessages(ctx, sessionID, []store.Message{{
				Role:    store.RoleAssistant,
				Content: assistant.String(),
			}})
			if err != nil {
				runLog.Warn("turn failed", "stage", "persist", "error", err)
				_ = sendTurnEvent(ctx, streamOut, TurnEvent{Kind: "error", Content: err.Error()})
				return
			}
			for _, msg := range assistantMessages {
				msg := msg
				if !sendTurnEvent(ctx, streamOut, TurnEvent{Kind: "message_stored", Message: &msg}) {
					return
				}
			}
			if !c.noBg {
				go c.autoNameSessionBackground(sessionID)
				go c.refreshRollingSummaryBackground(sessionID)
			}
			runLog.Info("turn finished", "provider", string(current.Provider), "reply_bytes", assistant.Len())
			_ = sendTurnEvent(ctx, streamOut, TurnEvent{Kind: adapter.EventTurnDone})
			return
		}
	}()
	return streamOut, nil
}

// sessionExtraWorkspaceDirs returns the directories exposed to a session's
// provider process beyond its own workspace. The shared project ledger under
// ~/.podium/projects/ is always included so agents can honor the base operating
// rule to consult projects.yaml; a roadmap session additionally gets its bound
// project's downloaded source snapshot repo directory (projectCtx.Root).
func (c *Core) sessionExtraWorkspaceDirs(projectCtx projectExecutionContext) []string {
	return nonEmptyStrings(c.paths.ProjectsDir, projectCtx.Root)
}

func (c *Core) turnRequest(sess store.Session, history []store.Message, userMessage string, opts TurnOptions, extraWorkspaceDirs []string) adapter.TurnRequest {
	return adapter.TurnRequest{
		SessionID: sess.ID,
		Handle: adapter.Handle{
			Provider: sess.Provider,
			ID:       sess.ProviderHandle,
		},
		Message: userMessage,
		History: replayHistory(sess, history),
		Settings: adapter.TurnSettings{
			AgentName:          sess.AgentName,
			Profile:            sess.Profile,
			ProfileDir:         c.profileDir(sess.Provider, sess.Profile),
			Model:              sess.Model,
			Effort:             sess.Effort,
			PermissionMode:     sess.PermissionMode,
			WorkspaceDir:       c.AgentPaths(sess.AgentName).Workspace,
			ExtraWorkspaceDirs: extraWorkspaceDirs,
			PermissionTurnID:   firstNonEmpty(opts.PermissionTurnID, fmt.Sprintf("%s-%d", sess.ID, time.Now().UnixNano())),
			Unattended:         opts.Unattended,
			AllowedTools:       opts.AllowedTools,
		},
		Relay: opts.PermissionRelay,
		Input: opts.UserInputRelay,
	}
}

func (c *Core) consumeAdapterEvents(ctx context.Context, streamOut chan<- TurnEvent, sessionID string, events <-chan adapter.Event) (strings.Builder, bool, bool) {
	var assistant strings.Builder
	for event := range events {
		switch event.Kind {
		case adapter.EventAssistantDelta:
			assistant.WriteString(event.Content)
			if !sendTurnEvent(ctx, streamOut, TurnEvent{Kind: event.Kind, Content: event.Content}) {
				return assistant, false, false
			}
		case adapter.EventAssistantMessage:
			assistant.Reset()
			assistant.WriteString(event.Content)
			if !sendTurnEvent(ctx, streamOut, TurnEvent{Kind: event.Kind, Content: event.Content}) {
				return assistant, false, false
			}
		case adapter.EventPermissionRequest:
			if !sendTurnEvent(ctx, streamOut, TurnEvent{Kind: event.Kind, PermissionRequest: event.PermissionRequest}) {
				return assistant, false, false
			}
		case adapter.EventUserInputRequest:
			if !sendTurnEvent(ctx, streamOut, TurnEvent{Kind: event.Kind, UserInputRequest: event.UserInputRequest}) {
				return assistant, false, false
			}
		case adapter.EventHandleUpdated:
			if event.Handle != nil {
				if _, err := c.store.UpdateSessionProviderHandle(ctx, sessionID, event.Handle.ID); err != nil {
					_ = sendTurnEvent(ctx, streamOut, TurnEvent{Kind: "error", Content: err.Error()})
					return assistant, false, false
				}
			}
		case adapter.EventRateStatus:
			if event.RateStatus != nil && event.RateStatus.UsedPercent >= 80 {
				go c.refreshRollingSummaryBackground(sessionID)
			}
		case adapter.EventRateLimited:
			return assistant, true, true
		case adapter.EventTurnDone:
		}
	}
	return assistant, false, true
}

// History returns a session's canonical history.
func (c *Core) History(ctx context.Context, sessionID string) ([]store.Message, error) {
	return c.store.ListMessages(ctx, sessionID)
}

// ComposeInstructions composes instructions using the session provider's
// delivery mode without sending them to a real provider.
func (c *Core) ComposeInstructions(ctx context.Context, agent store.Agent) (InstructionPayload, error) {
	return c.ComposeInstructionsForProvider(ctx, agent, agent.Provider)
}

// ComposeInstructionsForProvider composes the same agent identity for a
// specific provider target. It is used when a session switches provider while
// staying bound to the same Podium agent.
func (c *Core) ComposeInstructionsForProvider(ctx context.Context, agent store.Agent, provider config.Provider) (InstructionPayload, error) {
	switch provider {
	case "claude":
		return c.composer.Compose(ctx, agent, DeliveryClaudeImport)
	case "codex":
		return c.composer.Compose(ctx, agent, DeliveryCodexBundle)
	default:
		return InstructionPayload{}, fmt.Errorf("unknown provider %q", provider)
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

func nonEmptyStrings(values ...string) []string {
	var out []string
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			out = append(out, v)
		}
	}
	return out
}

func sendTurnEvent(ctx context.Context, ch chan<- TurnEvent, event TurnEvent) bool {
	select {
	case <-ctx.Done():
		return false
	case ch <- event:
		return true
	}
}
