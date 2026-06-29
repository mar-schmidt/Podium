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
//
// MCPConfig is treated as sensitive (it may embed server commands, local URLs,
// tokens, or credentials) and is never serialized to clients or logs — the
// `json:"-"` tag redacts it at every JSON boundary, REST and WebSocket alike
// (R8.29). It is read/written only through the store's column mapping.
type Agent struct {
	Name           string
	Provider       config.Provider
	Profile        string
	Model          string
	Effort         string
	PermissionMode config.PermissionMode
	Fallback       []string
	MCPConfig      string `json:"-"`
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
	TaskID         string
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

// TaskStatus is a roadmap task's kanban column.
type TaskStatus string

const (
	// TaskBacklog is unstarted work.
	TaskBacklog TaskStatus = "backlog"
	// TaskInProgress is work an agent has been started on.
	TaskInProgress TaskStatus = "in_progress"
	// TaskReview is work awaiting review.
	TaskReview TaskStatus = "review"
	// TaskDone is completed work.
	TaskDone TaskStatus = "done"
)

// Task is a roadmap item: a unit of work on a shared project, assignable to an
// agent and startable on demand (origin=roadmap) or at a scheduled pickup time.
// Tasks are independent in v1 — no inter-task dependencies (§2).
type Task struct {
	ID            string
	ProjectID     string
	Title         string
	Body          string
	AssignedAgent string
	Status        TaskStatus
	PickupAt      string // optional RFC3339 scheduled pickup time
	CreatedAt     string
	UpdatedAt     string
}
