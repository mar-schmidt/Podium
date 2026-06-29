// Shared TypeScript types mirroring the server data model and WebSocket
// protocol.

export interface Health {
  status: string;
  version: string;
  commit: string;
  started: string;
  uptime_ms: number;
}

export type Provider = "claude" | "codex";
export type PermissionMode = "approve" | "yolo";
export type SessionOrigin = "web" | "cli" | "onboarding" | "schedule" | "roadmap";
export type MessageRole = "user" | "assistant";

export interface Agent {
  Name: string;
  Provider: Provider;
  Profile: string;
  Model: string;
  Effort: string;
  PermissionMode: PermissionMode;
}

export interface Session {
  ID: string;
  AgentName: string;
  Name: string;
  Description: string;
  AutoNamed: boolean;
  Provider: Provider;
  Profile: string;
  Model: string;
  Effort: string;
  PermissionMode: PermissionMode;
  Origin: SessionOrigin;
  ScheduleID: string;
  RunID: string;
  TaskID: string;
  ProjectID: string;
  ProviderHandle: string;
}

export type TaskStatus = "backlog" | "in_progress" | "review" | "done";

export interface Project {
  id: string;
  name: string;
  description: string;
  color: string;
  path: string;
  status: string;
  stack: string[];
  repo: string;
  backlog: string[];
  roadmap: string[];
  notes: string;
}

// AgentDetail is the GET /api/agents/<name> response: durable defaults plus the
// editable SOUL.md body.
export interface AgentDetail extends Agent {
  Soul: string;
}

// Task mirrors store.Task (Go-exported field names, no json tags).
export interface Task {
  ID: string;
  ProjectID: string;
  Title: string;
  Body: string;
  AssignedAgent: string;
  Status: TaskStatus;
  PickupAt: string;
  CreatedAt: string;
  UpdatedAt: string;
}

// SessionDetail is the GET /api/sessions/<id> response, including roadmap
// provenance when the session was started from a task.
export interface SessionDetail {
  session: Session;
  history: Message[];
  task?: Task;
  project_id?: string;
  project_name?: string;
}

export type RunPermission = "preapproved" | "yolo";
export type RunTrigger = "cron" | "manual";
export type RunStatus = "running" | "success" | "error";

// ScheduleRun mirrors store.ScheduleRun (Go-exported field names, no json tags).
export interface ScheduleRun {
  ID: string;
  ScheduleName: string;
  SessionID: string;
  Trigger: RunTrigger;
  Status: RunStatus;
  Error: string;
  StartedAt: string;
  FinishedAt: string;
}

// ScheduleStatus mirrors schedule.Status (json-tagged, snake_case).
export interface ScheduleStatus {
  name: string;
  path: string;
  agent: string;
  model: string;
  effort: string;
  cron: string;
  every: string;
  run_permission: RunPermission;
  allowed_tools: string[];
  enabled: boolean;
  next_run?: string;
  parse_error?: string;
  runs: ScheduleRun[];
}

export interface Message {
  ID: number;
  SessionID: string;
  Seq: number;
  Role: MessageRole;
  Content: string;
}

export interface PermissionRequest {
  id: string;
  turn_id: string;
  tool_name: string;
  tool_use_id: string;
  input: Record<string, unknown>;
  expires_at?: string;
}

export interface PermissionDecision {
  behavior: "allow" | "deny";
  updatedInput?: Record<string, unknown>;
  message?: string;
}

export type ClientMessage =
  | { type: "list"; request_id?: string }
  | { type: "create_session"; request_id: string; agent_name: string }
  | {
      type: "send_turn";
      request_id: string;
      agent_name?: string;
      session_id?: string;
      message: string;
    }
  | { type: "permission_decision"; request_id: string; decision: PermissionDecision };

export interface ServerMessage {
  type:
    | "hello"
    | "state"
    | "session"
    | "history"
    | "message"
    | "delta"
    | "assistant"
    | "permission_request"
    | "notice"
    | "done"
    | "error";
  request_id?: string;
  agents?: Agent[];
  sessions?: Session[];
  session?: Session;
  history?: Message[];
  message?: Message;
  delta?: string;
  notice?: string;
  request?: PermissionRequest;
  error?: string;
}
