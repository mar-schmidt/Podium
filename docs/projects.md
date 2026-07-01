# Projects and the Roadmap

## Projects (shared ledger)

Projects are **shared, system-level, agent-independent** resources (§5.3 / D22) —
modelling how a real team collaborates on the same codebase, book, or initiative.
They are not owned by any one agent: every agent can read and work on every
project.

- Each project is a subdirectory under `~/.podium/projects/` plus an entry in the
  single shared ledger `~/.podium/projects/projects.yaml` (R5.10).
- Agents are instructed (via the Podium-owned base `AGENTS.md`) to work **with and
  against** the ledger — create a project dir and record/update its entry whenever
  they create or maintain something durable (R5.11).
- Because agents maintain the file directly, the ledger is the source of truth and
  v1 accepts **last-write-wins** on concurrent writes (R5.12).

A ledger entry:

```yaml
projects:
  - id: mission-control
    name: Mission Control
    description: Next.js dashboard UI.
    path: mission-control        # relative to ~/.podium/projects/
    status: active               # active | paused | done
    stack: [Next.js, TypeScript]
    repo: null
    roadmap: []                  # derived roadmap task IDs
    notes: >
      Anything the next agent needs to know.
```

Create and browse projects from the **Projects** page in the web UI, or list them
with `podium projects list`.

## Roadmap (tasks)

The **Roadmap** is a kanban board of **tasks** — units of work on a project,
assignable to an agent and startable on demand. Tasks are a Podium-managed entity
(persisted in SQLite) and are **independent** in v1: there are no inter-task
dependencies (within the §2 out-of-scope line).

Each project's `roadmap` array in `projects.yaml` is a derived list of task IDs
for that project. Task details live in SQLite and are expanded by Podium when
drafting roadmap prompts.

A task has: a project, a title and optional body, an assigned agent, a status
column (`backlog` → `in_progress` → `review` → `done`), and an optional scheduled
pickup time.

### Starting work

- **Start on demand** (a backlog card with an assigned agent): creates a durable
  **roadmap-origin session** bound to the agent, linked back to the task, and
  seeds it with the task as the first turn. The chat shows a "part of `<project>`"
  provenance banner. The task moves to **In Progress**.
- **Open in chat** (an already-started card): reopens the task's existing session
  to continue manually.
- **Scheduled pickup**: a task with a pickup time is started automatically by the
  embedded scheduler when that time arrives, running unattended under the
  stricter **preapproved** permission policy (§7.7) — side effects are denied
  unless explicitly safe. Starting moves it to In Progress so it is not picked up
  twice. Pickup times are interpreted as UTC.

Drag cards between columns to change status, assign an agent from the card, and
add tasks with **+ New task**. Observe tasks with `podium tasks list`.

## HTTP API

- `GET /api/projects`, `POST /api/projects`
- `GET /api/tasks`, `POST /api/tasks`
- `PATCH /api/tasks/<id>` (assign / move / edit / set pickup)
- `POST /api/tasks/<id>/start` — start on demand → returns the session
- `GET  /api/tasks/<id>/session` — the task's latest session (404 if not started)

Roadmap sessions carry `origin = roadmap` and a `task_id`; the session detail
endpoint (`GET /api/sessions/<id>`) includes the task and project name for the
provenance banner.
