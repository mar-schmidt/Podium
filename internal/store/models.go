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
	Name           string
	Description    string
	AutoNamed      bool
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

// RunTrigger records what caused a scheduled run.
type RunTrigger string

const (
	// TriggerCron marks a run fired by the embedded cron scheduler.
	TriggerCron RunTrigger = "cron"
	// TriggerManual marks a run started by an explicit "Run now".
	TriggerManual RunTrigger = "manual"
)

// RunStatus is the lifecycle state of a scheduled run.
type RunStatus string

const (
	// RunRunning marks an in-flight scheduled run.
	RunRunning RunStatus = "running"
	// RunSuccess marks a scheduled run that completed without error.
	RunSuccess RunStatus = "success"
	// RunError marks a scheduled run that failed.
	RunError RunStatus = "error"
)

// ScheduleRun records one execution of a schedule. It links the schedule to the
// durable session it produced (R7.9 / R4.12) so the run can be revisited and
// continued manually.
type ScheduleRun struct {
	ID           string
	ScheduleName string
	SessionID    string
	Trigger      RunTrigger
	Status       RunStatus
	Error        string
	StartedAt    string
	FinishedAt   string
}
