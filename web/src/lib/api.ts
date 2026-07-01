// Typed REST helpers for the Podium daemon. The chat stream uses the WebSocket
// (see Chat.svelte); everything else is plain JSON over these helpers.

import type {
  Agent,
  AgentDetail,
  GitHubDevicePoll,
  GitHubDeviceStart,
  GitHubRepo,
  GitHubStatus,
  GlobalConfig,
  GlobalConfigPatch,
  Health,
  LogSnapshot,
  LogStreamEvent,
  ProfileInfo,
  Project,
  ScheduleStatus,
  Session,
  SessionDetail,
  Skill,
  Task,
  TaskStatus,
  UpdateApplyResult,
  UpdateStatus,
} from "./types";

async function asJSON<T>(res: Response): Promise<T> {
  if (!res.ok) {
    const body = await res.text();
    throw new Error(body || `${res.status} ${res.statusText}`);
  }
  return (await res.json()) as T;
}

export async function getHealth(): Promise<Health> {
  return asJSON(await fetch("/healthz"));
}

export async function checkUpdate(): Promise<UpdateStatus> {
  return asJSON(await fetch("/api/update"));
}

export async function applyUpdate(force = false): Promise<UpdateApplyResult> {
  return asJSON(
    await fetch("/api/update/apply", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ force }),
    }),
  );
}

export async function listAgents(): Promise<Agent[]> {
  return (await asJSON<Agent[] | null>(await fetch("/api/agents"))) ?? [];
}

export async function listSkills(): Promise<Skill[]> {
  return (await asJSON<Skill[] | null>(await fetch("/api/skills"))) ?? [];
}

export async function listProfiles(): Promise<ProfileInfo[]> {
  return (await asJSON<ProfileInfo[] | null>(await fetch("/api/profiles"))) ?? [];
}

export async function getConfig(): Promise<GlobalConfig> {
  return asJSON(await fetch("/api/config"));
}

export async function updateConfig(patch: GlobalConfigPatch): Promise<GlobalConfig> {
  return asJSON(
    await fetch("/api/config", {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(patch),
    }),
  );
}

export async function getLogs(lines = 200): Promise<LogSnapshot> {
  return asJSON(await fetch(`/api/logs?lines=${encodeURIComponent(String(lines))}`));
}

export async function followLogs(lines: number, signal: AbortSignal, onEvent: (event: LogStreamEvent) => void): Promise<void> {
  const res = await fetch(`/api/logs/follow?lines=${encodeURIComponent(String(lines))}`, { signal });
  if (!res.ok) {
    const body = await res.text();
    throw new Error(body || `${res.status} ${res.statusText}`);
  }
  if (!res.body) return;
  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    const parts = buffer.split("\n");
    buffer = parts.pop() ?? "";
    for (const part of parts) {
      const line = part.trim();
      if (!line) continue;
      onEvent(JSON.parse(line) as LogStreamEvent);
    }
  }
  if (buffer.trim()) {
    onEvent(JSON.parse(buffer) as LogStreamEvent);
  }
}

export interface HireRequest {
  name: string;
  provider: string;
  model: string;
  effort: string;
  permission_mode: string;
}

export async function hireAgent(req: HireRequest): Promise<Agent> {
  return asJSON(
    await fetch("/api/agents", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(req),
    }),
  );
}

export async function getAgent(name: string): Promise<AgentDetail> {
  return asJSON(await fetch(`/api/agents/${encodeURIComponent(name)}`));
}

export interface AgentUpdate {
  provider?: string;
  profile?: string;
  model?: string;
  effort?: string;
  permission_mode?: string;
  fallback?: string[];
  soul?: string;
}

export async function updateAgent(name: string, patch: AgentUpdate): Promise<AgentDetail> {
  return asJSON(
    await fetch(`/api/agents/${encodeURIComponent(name)}`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(patch),
    }),
  );
}

export interface AgentDeleteResult {
  archive_path?: string;
  archived_sessions: number;
}

export async function deleteAgent(name: string, confirmation: string): Promise<AgentDeleteResult> {
  return asJSON(
    await fetch(`/api/agents/${encodeURIComponent(name)}`, {
      method: "DELETE",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ confirmation }),
    }),
  );
}

export async function listSessions(): Promise<Session[]> {
  return (await asJSON<Session[] | null>(await fetch("/api/sessions"))) ?? [];
}

export async function getSession(id: string): Promise<SessionDetail> {
  return asJSON(await fetch(`/api/sessions/${id}`));
}

export async function listSchedules(): Promise<ScheduleStatus[]> {
  return (await asJSON<ScheduleStatus[] | null>(await fetch("/api/schedules"))) ?? [];
}

export async function runSchedule(name: string): Promise<unknown> {
  return asJSON(await fetch(`/api/schedules/${name}/run`, { method: "POST" }));
}

export interface NewScheduleRequest {
  name: string;
  agent: string;
  model?: string;
  effort?: string;
  cron?: string;
  every?: string;
  run_permission: string;
  allowed_tools?: string[];
  body: string;
}

export async function createSchedule(req: NewScheduleRequest): Promise<ScheduleStatus> {
  return asJSON(
    await fetch("/api/schedules", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(req),
    }),
  );
}

export async function listProjects(): Promise<Project[]> {
  return (await asJSON<Project[] | null>(await fetch("/api/projects"))) ?? [];
}

export interface NewProjectRequest {
  id: string;
  name: string;
  description: string;
  stack: string[];
  notes: string;
}

export async function createProject(req: NewProjectRequest): Promise<Project> {
  return asJSON(
    await fetch("/api/projects", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(req),
    }),
  );
}

export interface ProjectPatch {
  name?: string;
  description?: string;
  color?: string;
  status?: string;
  stack?: string[];
  notes?: string;
}

export async function updateProject(id: string, patch: ProjectPatch): Promise<Project> {
  return asJSON(
    await fetch(`/api/projects/${encodeURIComponent(id)}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(patch),
    }),
  );
}

export async function describeProject(id: string, agent: string): Promise<string> {
  const res = await asJSON<{ description: string }>(
    await fetch(`/api/projects/${encodeURIComponent(id)}/describe`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ agent }),
    }),
  );
  return res.description;
}

export async function githubStatus(): Promise<GitHubStatus> {
  return asJSON(await fetch("/api/github/status"));
}

export async function githubDeviceStart(): Promise<GitHubDeviceStart> {
  return asJSON(await fetch("/api/github/device/start", { method: "POST" }));
}

export async function githubDevicePoll(device_code: string): Promise<GitHubDevicePoll> {
  return asJSON(
    await fetch("/api/github/device/poll", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ device_code }),
    }),
  );
}

export async function githubRepos(): Promise<GitHubRepo[]> {
  return (await asJSON<GitHubRepo[] | null>(await fetch("/api/github/repos"))) ?? [];
}

export interface ConnectProjectRepoRequest {
  owner: string;
  name: string;
  full_name: string;
  html_url: string;
  default_branch: string;
  ref?: string;
  force?: boolean;
}

export async function connectProjectRepo(id: string, req: ConnectProjectRepoRequest): Promise<Project> {
  const res = await fetch(`/api/projects/${encodeURIComponent(id)}/repo`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
  if (res.status === 409) {
    throw new Error("CONFIRM_REPLACE");
  }
  return asJSON(res);
}

export async function syncProjectRepo(id: string, force = false): Promise<Project> {
  const res = await fetch(`/api/projects/${encodeURIComponent(id)}/repo/sync`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ force }),
  });
  if (res.status === 409) {
    throw new Error("CONFIRM_REPLACE");
  }
  return asJSON(res);
}

export async function disconnectProjectRepo(id: string): Promise<Project> {
  return asJSON(await fetch(`/api/projects/${encodeURIComponent(id)}/repo`, { method: "DELETE" }));
}

export interface TaskDescribeRequest {
  id?: string;
  agent?: string;
  project_id?: string;
  title?: string;
  body?: string;
  assigned_agent?: string;
}

export async function describeTask(req: TaskDescribeRequest): Promise<string> {
  const id = req.id?.trim();
  const body = {
    agent: req.agent,
    project_id: req.project_id,
    title: req.title,
    body: req.body,
    assigned_agent: req.assigned_agent,
  };
  const res = await asJSON<{ body: string }>(
    await fetch(id ? `/api/tasks/${encodeURIComponent(id)}/describe` : "/api/tasks/describe", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }),
  );
  return res.body;
}

export async function listTasks(): Promise<Task[]> {
  return (await asJSON<Task[] | null>(await fetch("/api/tasks"))) ?? [];
}

export interface NewTaskRequest {
  project_id: string;
  title: string;
  body: string;
  assigned_agent: string;
  status?: TaskStatus;
  pickup_at?: string;
}

export async function createTask(req: NewTaskRequest): Promise<Task> {
  return asJSON(
    await fetch("/api/tasks", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(req),
    }),
  );
}

export interface TaskPatch {
  project_id?: string;
  title?: string;
  body?: string;
  assigned_agent?: string;
  status?: TaskStatus;
  pickup_at?: string;
}

export async function updateTask(id: string, patch: TaskPatch): Promise<Task> {
  return asJSON(
    await fetch(`/api/tasks/${id}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(patch),
    }),
  );
}

export async function startTask(id: string): Promise<Session> {
  return asJSON(await fetch(`/api/tasks/${id}/start`, { method: "POST" }));
}

// taskSession returns the latest session for a started task, or null if none.
export async function taskSession(id: string): Promise<Session | null> {
  const res = await fetch(`/api/tasks/${id}/session`);
  if (res.status === 404) return null;
  return asJSON(res);
}
