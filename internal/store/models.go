package store

import "github.com/mar-schmidt/Podium/internal/config"

// SessionOrigin records where a session was created. It is provenance and is
// immutable after creation.
type SessionOrigin string

const (
	// OriginWeb marks a session created from the web UI.
	OriginWeb SessionOrigin = "web"
	// OriginCLI marks a session created from the CLI.
	OriginCLI SessionOrigin = "cli"
	// OriginSchedule marks a session created by a scheduled run.
	OriginSchedule SessionOrigin = "schedule"
	// OriginRoadmap marks a session created from a roadmap task.
	OriginRoadmap SessionOrigin = "roadmap"
)

// MessageRole identifies the speaker for a canonical history entry.
type MessageRole string

const (
	// RoleUser is a user-authored history entry.
	RoleUser MessageRole = "user"
	// RoleAssistant is an assistant-authored history entry.
	RoleAssistant MessageRole = "assistant"
)

// Agent is Podium's durable definition of a named colleague.
type Agent struct {
	Name           string
	Provider       config.Provider
	Profile        string
	Model          string
	Effort         string
	PermissionMode config.PermissionMode
	Fallback       []string
	MCPConfig      string
	CreatedAt      string
	UpdatedAt      string
}

// Session is Podium's durable conversation unit and current provider settings.
type Session struct {
	ID             string
	AgentName      string
	Provider       config.Provider
	Profile        string
	Model          string
	Effort         string
	PermissionMode config.PermissionMode
	Origin         SessionOrigin
	ScheduleID     string
	RunID          string
	RollingSummary string
	ProviderHandle string
	CreatedAt      string
	UpdatedAt      string
}

// Message is one ordered entry in a session's canonical history.
type Message struct {
	ID        int64
	SessionID string
	Seq       int
	Role      MessageRole
	Content   string
	CreatedAt string
}
