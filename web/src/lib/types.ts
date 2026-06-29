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
export type SessionOrigin = "web" | "cli" | "schedule" | "roadmap";
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
  ProviderHandle: string;
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
