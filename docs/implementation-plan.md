# Podium v1 — Phased Implementation Plan

> **This is the shared, version-controlled implementation plan** — the canonical
> reference for anyone (human or agent) working on Podium v1. It is a living
> document: as each phase is reviewed and approved, tick it off in the Progress
> log and record any deviations/decisions inline (mirroring the spec's §12
> decision log style). The authoritative product spec is
> [requirements.md](requirements.md) (v1.6); §/R/D references below point into it.

## Progress log
- **Phase 0 — Foundations & scaffold: ✅ COMPLETE (2026-06-29).** Repo at
  `/Users/marcus/Development/Podium` (git initialized, module
  `github.com/mar-schmidt/Podium`, Go 1.26). Built: `internal/config` (PODIUM_HOME
  resolution, commented default `config.yaml` shipped + written on first run,
  load/validate, idempotent scaffolding of `~/.podium/`), `internal/exec`
  (binary discovery w/ npm shim + `*_BIN` overrides, process-group kill via
  unix/windows build tags), `internal/store` (modernc SQLite + forward-only
  migration framework), `internal/server` (health endpoint + embedded-SPA handler
  w/ client-side-routing fallback), `internal/client` + `cmd/podium` (thin client,
  `status` command, daemon-attached per R11.1), `cmd/podiumd` (boot wiring +
  graceful shutdown), `web/` (Svelte 5 + Vite 6 + TS + Tailwind 4 SPA skeleton,
  `go:embed`'d into the binary). Docs: README, `docs/cli.md`,
  `docs/configuration.md`, `docs/integrations/README.md`, requirements copied to
  `docs/requirements.md`. Makefile (web→go build, version stamp, `cross` target).
  Verified: `go vet` + `go test ./...` green; 6 OS/arch cross-compiles; daemon
  scaffolds a fresh home, serves health + embedded SPA (real hashed assets) +
  deep-link fallback; `podium status` reports live/not-running.
  - *Notes:* installed Go 1.26.4 via Homebrew (was missing); `codex` CLI not
    present on the original dev machine (fine until Phase 5).
- **Phase 1 — Core domain: ✅ COMPLETE (2026-06-29).** Added SQLite migration v2
  for agents, sessions, immutable session origin, schedule/run linkage, rolling
  summaries, provider handles, and ordered user/assistant message history. Added
  typed `internal/store` APIs; `internal/adapter` seam with deterministic fake
  adapter; `internal/core` agent CRUD/scaffolding, session lifecycle, append-turn,
  and instruction composition. Agent creation scaffolds `SOUL.md` and
  `workspace/` while leaving per-agent `AGENTS.md` user-created. Composition
  writes provider-ready payloads in fixed order: Claude `workspace/CLAUDE.md`
  with `@` imports, Codex `workspace/AGENTS.md` concatenated bundle. Docs added:
  `docs/agents.md`, `docs/sessions.md`; CLI reference notes that Phase 1 exposes
  no new CLI commands. Verified: `go test ./...` green.
  - *Notes:* real Claude/Codex adapters and CLI/web commands remain deferred to
    later phases per scope.
- **Phase 2 — Claude adapter + approve relay: ✅ COMPLETE (2026-06-29).** Added
  real Claude Code per-turn adapter (`claude -p` stream-json, workspace cwd,
  model/effort flags, `--resume`, profile env, generated `CLAUDE.md` discovery),
  daemon-owned core wiring, NDJSON chat endpoint, CLI `agents`/`chat` commands,
  and an approve-mode MCP permission relay with configurable auto-deny timeout
  (`global.permission_timeout`). Added hidden `podiumd permission-mcp` helper,
  daemon permission broker, and docs in `docs/integrations/claude.md`. Verified
  manually with a throwaway `PODIUM_HOME`: yolo Claude turn streamed once, same
  session resumed after daemon restart, and approve-mode `Write` request surfaced
  in the CLI and succeeded after approval.
  - *Notes:* web/WebSocket permission UX remains Phase 3/4; Codex remains Phase
    5. The Claude permission MCP response shape was adjusted during live testing
    to match Claude Code's validator: a single MCP text block containing the JSON
    decision.
- **Phase 3 — WebSocket server contract + browser slice: ✅ COMPLETE
  (2026-06-29).** Added typed WebSocket endpoint `/api/ws` with Go contract
  structs and mirrored TypeScript types, REST get/list support for agents and
  sessions, and a minimal Svelte browser chat surface that can create agents,
  start web-origin sessions, stream turns, list/select sessions, and answer
  permission requests inline. The Phase 2 NDJSON endpoint remains for the CLI.
  Docs added in `docs/websocket.md`. Verified with fake-adapter WebSocket tests
  plus frontend build.
  - *Notes:* this is a functional contract slice, not the Phase 4 polished chat
    MVP; visual refinement, slash chips, auto-naming, and richer session filters
    remain deferred.

## Context

Podium is a **thin orchestration daemon for local LLM agents** (Claude Code, OpenAI Codex). It shells
out to the native `claude` and `codex` CLIs and leans on *their* MCP/tools/memory, while owning its own
durable truth: named agents, durable chat sessions, a canonical message history that can be replayed onto
a fresh backing CLI session on any profile/provider switch, an embedded scheduler, and a shared project
ledger. It ships as a single Go binary (`podiumd` daemon + `podium` thin CLI client) with an embedded
Svelte SPA web UI.

This plan turns [requirements.md](requirements.md) (v1.6, the authoritative spec — §/R/D references below
point into it) plus the Claude-design frontend export into an implementation we can build and review
**one phase at a time**. Each phase is independently reviewable and leaves the system in a working,
demoable state where possible.

**Two scoping decisions (confirmed with the user):**
1. **Full kanban in v1.** The design's "Roadmap/Docket" board is a first-class feature: a `task` entity
   (project-scoped, agent-assigned, status columns, optional scheduled pickup, run-on-demand → spawns a
   session with `origin = roadmap`). This **extends** the requirements doc, which only modelled freeform
   `backlog:`/`roadmap:` YAML arrays and put DAG/dependencies out of scope. Tasks remain **independent**
   (no inter-task dependency graph) to stay within the v1 out-of-scope line (§2).
2. **Vertical-slice-first sequencing.** Get Claude working end-to-end through the web chat UI early
   (Phases 0–4), then layer Codex, profiles/replay/fallback, scheduler, and the remaining UI pages.

---

## Guiding constraints (from the spec — apply to every phase)

- **Thin, not invasive** (Principle 1, 6): use native CLI MCP/tools/memory by default; `approve` is the
  safety boundary, `yolo`/strict are opt-in. Podium uses OpenClaw's *mechanisms* with the opposite
  *defaults* (§8.4b).
- **Podium owns canonical truth** (R4.1–R4.6): the CLI session/thread is a disposable backing resource;
  history lives in SQLite and is replayed on any switch.
- **One core, two faces** (Principle 3, R11.1): CLI and web are thin clients of one `podiumd`. The CLI
  always attaches to a running daemon — no in-process sessions.
- **Two process models, one adapter interface** (D7, §3): Claude = per-turn `claude -p` (resume via
  session ID); Codex = long-lived `codex app-server` (resume via `threadId`). The adapter abstracts
  "send a turn, stream events, return a resumable handle."
- **Deployment-target neutral** (Principle 7, D18): storage root overridable via `PODIUM_HOME`
  (default `~/.podium/`); web bind configurable; never hard-code process lifecycle. HA add-on is a later
  *packaging* concern, not a rewrite.
- **Cross-platform by construction** (Principle 4, §10): Windows/Linux/macOS first-class; all OS
  differences isolated to `internal/exec` + path handling; pure-Go deps only (no cgo).
- **Config as data** (Principle 5): declarative `config.yaml`; schedules and projects are self-describing
  files, not config blocks.

## Tech stack (decided — R3.1–R3.5, D17, D21)

- **Backend:** Go. `cobra` (CLI), `robfig/cron` (scheduler), `modernc.org/sqlite` (pure-Go, no cgo),
  `kardianos/service` (deferred OS-service feature), stdlib `net/http` + a WebSocket lib (e.g.
  `nhooyr.io/websocket`/`coder/websocket` or `gorilla/websocket` — pick one in Phase 3).
- **Frontend:** plain **Svelte + Vite + TypeScript + Tailwind** SPA (no SvelteKit/SSR). `vite build` →
  static assets embedded via `go:embed`, served by `podiumd`. Browser-native WebSocket for streaming.
- **Persistence:** embedded SQLite at `$PODIUM_HOME/podium.db` (history, rolling summaries, provider
  handles, session metadata, tasks). Agent workspaces/projects/schedules stay as plain files on disk.

## Repo layout (target — §3)

```
podium/
  go.mod
  cmd/podium/            # thin CLI client (cobra)
  cmd/podiumd/           # daemon: web server + scheduler + core
  internal/core/         # sessions, agents, history, composition — source of truth
  internal/adapter/      # adapter iface + claude + codex
  internal/exec/         # subprocess + cross-platform binary discovery + process groups
  internal/schedule/     # embedded cron/every scheduler
  internal/config/       # YAML load + validate + PODIUM_HOME resolution + scaffolding
  internal/store/        # SQLite persistence + migrations
  internal/server/       # http + websocket contract + embedded assets
  web/                   # Svelte+Vite+TS+Tailwind SPA (built → embedded)
  docs/integrations/     # the per-provider integration contract (Appendix A → source of truth)
  docs/cli.md            # CLI command reference (kept in sync with cobra help)
  docs/                  # per-feature user docs + config reference (grown each phase)
```

Runtime data layout under `$PODIUM_HOME` (R9.1): `config.yaml`, `AGENTS.md` (Podium-owned base),
`podium.db`, `agents/<name>/{SOUL.md, AGENTS.md?, workspace/}`, `projects/{projects.yaml, <proj>/}`,
`schedules/*.md`, `profiles/`.

---

## Cross-cutting (build once, use everywhere)

- **Fake adapter for tests/dev.** A `fakeAdapter` implementing the adapter interface (deterministic
  streamed events, scriptable tool-calls/permission-requests/rate-limit signals) so the whole core, web,
  replay, scheduler, and UI can be exercised in CI **without** invoking real CLIs or burning tokens. Real
  `claude`/`codex` runs are reserved for manual/integration verification.
- **Typed message contract.** One source-of-truth schema for the WebSocket protocol and REST DTOs
  (sessions w/ origin+schedule provenance, profiles, permission-request payloads, stream events, tasks),
  with TS types in `web/` mirroring the Go structs.
- **Security posture** (§8.6): redact MCP configs/credentials from user-facing logs (R8.29); never expose
  raw system prompts to clients (R8.30); profile auth state stays isolated, Podium never handles
  credentials (R8.32, R8.37); `yolo` is whole-machine by design — the guard is the explicit opt-in
  (R8.31).
- **Structured run logging** (R11.5) from Phase 2 onward.
- **Documentation discipline (every phase).** Each feature, command, and config field is documented as it
  is built — not deferred to the end — so user-facing documentation pages are a compile/aggregation step
  later, not a rediscovery exercise. Concretely, per phase we maintain:
  - **CLI reference:** every `podium` command/subcommand/flag gets a clear cobra `Short`/`Long`/`Example`
    (so `podium help` is authoritative) **and** a matching entry in `docs/cli.md`.
  - **Feature docs:** a short `docs/` page (or section) per user-facing feature — agents, sessions/chat,
    slash commands, profiles, scheduler, projects, roadmap/tasks, permission modes — describing behaviour
    and options.
  - **Config reference:** kept in sync with the commented default `config.yaml` and the integration
    contracts in `docs/integrations/`.
  - **Code-level:** exported Go packages/functions and the WS/REST contract types carry doc comments.
  A phase isn't "done" until its new commands/features/config are documented (added to each phase's exit
  criteria).

---

## Phases

### Phase 0 — Foundations & scaffold
**Goal:** A buildable repo where `podiumd` boots and `podium` attaches to it; storage root + config exist.
- `git init` in the working dir; Go module; repo skeleton (all `internal/*`, `cmd/*` stubs); Vite/Svelte
  app skeleton; Makefile/build script (`vite build` → `go build` with `go:embed`).
- `internal/config`: resolve `PODIUM_HOME` (default `~/.podium/`, R9.1/R10.2/D14); load+validate
  `config.yaml` (`global`, `profiles`, `agents`, `server` — §9 R9.2/D16); **first-run scaffolding** of the
  directory tree + Podium-owned base `AGENTS.md` + empty `projects.yaml`.
- **Ship a default `config.yaml`.** Keep a fully-commented template `config.yaml` in the repo
  (`internal/config/config.default.yaml`, mirroring the §9 illustrative shape: `global`, `profiles`,
  `agents`, `server`); on first run scaffolding **writes it to `$PODIUM_HOME/config.yaml` if absent** so a
  fresh install always has a real, self-documenting config to edit. Every field carries an inline comment
  explaining it (these comments seed the future config-reference docs page).
- `internal/exec`: cross-platform binary discovery for `claude`/`codex` (incl. Windows `.cmd`/`.exe`
  shims, npm locations — R8.1/R10.3); context cancellation + process-group kill scaffolding (R10.4).
- `internal/store`: SQLite open + migration framework (initial empty schema).
- `internal/server`: minimal http server bound to `server.bind:port` (default `127.0.0.1:8787`).
- `cmd/podiumd` boots all of the above; `cmd/podium` connects and reports daemon status / "not running"
  guidance (R11.1/D2).
- Check Appendix A into `docs/integrations/` as the contract source of truth.
- **Covers:** R3.1–R3.4, R9.1–R9.2, R10.1–R10.4, R11.1, Principle 7.
- **Exit:** `podiumd` starts, scaffolds `~/.podium/`, serves a health endpoint; `podium status` reports
  live/not-running; cross-compiles to win/linux/mac.

### Phase 1 — Core domain: agents, sessions, persistence, instruction composition
**Goal:** The source-of-truth model exists and persists, independent of any provider.
- SQLite schema + `internal/store`: sessions (bound agent, settings, **channel origin** R4.10, schedule/
  run linkage R4.12), ordered message history (R4.1), rolling-summary column (R8.26), provider handle
  column (Claude session ID / Codex `threadId`), agents (D6/R11.2).
- `internal/core` agent model + CRUD (R5.1): on create, scaffold `agents/<name>/` with `SOUL.md`
  skeleton (always, R5.13), optional per-agent `AGENTS.md` left to user, `workspace/` (R5.5–R5.7).
- **Instruction composition** (R5.15/§5.4/D19): compose base `AGENTS.md` + per-agent `AGENTS.md`(opt) +
  `SOUL.md` in fixed order. Two delivery strategies behind one composer interface: `@`-import file
  (Claude) and concatenated bundle (Codex) — wired to adapters in later phases (R5.16/R8.22).
- Session lifecycle in core (create/list/get/append-turn) with no live backend yet (drive via fake
  adapter). Origin set at creation, immutable (R4.10).
- **Covers:** R4.1, R4.10, R4.12, R5.1–R5.17, R11.2, D6, D19, D20.
- **Exit:** create agents (dirs scaffolded correctly); create sessions; append turns via fake adapter;
  history survives daemon restart; composition produces correct payloads for both delivery modes.

### Phase 2 — Claude adapter + approve permission relay (CLI vertical slice)
**Goal:** A real Claude turn runs end-to-end and streams, driven from `podium` CLI.
- `internal/adapter` interface (R8.1–R8.5): start→resumable handle, send-turn→stream events, resume,
  teardown.
- **Claude adapter** (§8.2): `claude -p --input-format stream-json --output-format stream-json
  --include-partial-messages --verbose` (R8.6); `--model`/`--effort` (R8.7); `cwd = workspace/` (R8.21);
  generated **Podium-managed `CLAUDE.md`** with `@`-imports (or `--append-system-prompt-file`, pick one
  consistently — R8.22); resume via persisted session ID `--resume` (R8.9); stream-json I/O + event
  correlation incl. `--replay-user-messages` (R8.10).
- **`approve` mode** (§8.4/§6.5/D12): run a small **MCP permission server** via
  `--permission-prompt-tool`; receive `{tool_name,input,tool_use_id}`, relay to user, return
  `allow/deny (+updatedInput)` (R8.17); **mandatory configurable timeout → auto-deny** (R8.18).
- **`yolo` mode:** `--permission-mode bypassPermissions` (whole-machine — R8.20/R8.31).
- Wire into core: streamed events persisted as canonical history; CLI renders stream + inline
  approve/deny prompt. Author `docs/integrations/claude.md`.
- **Covers:** R8.1–R8.10, R8.17–R8.21, R5.18–R5.21, R6.5, D12.
- **Exit:** from `podium`, send a message to a Claude agent, watch tokens stream, approve/deny a real
  tool call (and observe auto-deny on timeout), resume the session after restart.

### Phase 3 — Web server, typed WebSocket contract, embedded SPA host
**Goal:** The browser can drive the same core over a live WebSocket; one binary serves the SPA.
- `internal/server`: WebSocket endpoint + **typed message contract** (client→server: send turn, slash
  command, permission decision; server→client: stream deltas, tool calls/results, lifecycle, permission
  request, settings/state changes — R6.4/R11.4). REST for list/get (sessions, agents).
- Serve embedded `web/` build via `go:embed`; client-side routing; dev proxy to Vite during development.
- Permission relay (Phase 2) plumbed over WS so approve/deny works from the browser (R6.5).
- **Covers:** R3.5, R6.4, R11.4, D21 (server side).
- **Exit:** open the UI in a browser, start a Claude session, stream a turn, approve/deny inline — all
  over WebSocket against the live daemon.

### Phase 4 — Frontend Chat MVP (the hero) + auto-naming + core slash commands
**Goal:** The designed chat experience, matching the Claude-design look (warm/soft, terracotta accent).
- Svelte SPA: left nav + `podiumd live` status; **Sessions** list with origin/agent filters and origin
  badges (web/cli/schedule/roadmap); **chat pane** with token streaming, inline approve/deny card incl.
  auto-deny countdown, session header w/ origin badge; **slash-command chips** (`/model`, `/effort`,
  `/permission`, `/name`, `/describe`, `/help`) (R6.1–R6.5); "Hire agent" modal (name, backend, permission
  mode). Tailwind theme tokens from the design.
- `/model`/`/effort`/`/permission` as session-scoped defaults (R6.2/R6.3); `/name`,`/describe` manual
  overrides.
- **Auto-naming** (R4.7–R4.9/D3): after first user message+response, generate name+description using the
  session's own model at **low/minimal effort**, **non-blocking** (async populate).
- **Covers:** R4.7–R4.9, R6.1–R6.5, D3, D21 (client). (`/profile` lands in Phase 6.)
- **Exit:** full chat round-trip in the browser matching the design; sessions auto-name after first
  exchange; slash chips change settings live.

### Phase 5 — Codex adapter (second process model, same interface)
**Goal:** Codex works behind the identical adapter interface; the UI is provider-agnostic.
- **Codex adapter** (§8.3): long-lived `codex app-server --listen stdio://` + JSON-RPC client over
  stdio; restart/reconnect on failure (R8.11); `thread/start`/`thread/resume`/`turn/start` (R8.12);
  `model`/`effort` in protocol (R8.13); `cwd` on thread/turn start (R8.21); resume via `threadId` (R8.15);
  correlate by `threadId`+`turnId` (R8.16).
- **Permissions** (§8.4/§8.19): `approve` = `approvalPolicy:on-request` + `sandbox:workspace-write` with
  approval requests relayed; `yolo` = `approvalPolicy:never` + `sandbox:danger-full-access`. Sandbox and
  approval kept independent (R8.19).
- **Composition delivery for Codex:** concatenated bundle into the `AGENTS.md` Codex reads from `cwd`
  (R8.22); **double-load guard** — ensure the canonical agent-root `AGENTS.md` isn't also picked up by
  Codex's root→cwd walk (R8.22a). Author `docs/integrations/codex.md`.
- **Covers:** R8.11–R8.16, R8.19, R8.22, R8.22a, D7.
- **Exit:** a Codex agent streams a turn, handles approve/deny, survives app-server restart, resumes via
  `threadId`; identity/instructions delivered once (no duplication).

### Phase 6 — Profiles, mid-chat switching, history replay, rolling summary, fallback chain
**Goal:** The durability model — switch profile/provider mid-chat without losing the conversation.
- **Profiles** (§8.7/D8): optional named auth contexts → `CLAUDE_CONFIG_DIR`/`CODEX_HOME` (R8.8/R8.14/
  R8.34); unset when no profile (global login, R8.35); "default" is first-class switch source/target
  (R8.36); Podium owns only dir+name, never credentials (R8.37). `/profile` slash command (R6.2).
- **History replay** (R4.3–R4.6/§8.5): on profile/provider switch or lost backing session, create a fresh
  backing session/thread and replay canonical history (R8.24 Claude / R8.25 Codex). **Provider-agnostic**
  (R4.6) — validated cross-provider now that both adapters exist.
- **Rolling summary** (R8.26–R8.28/D4): maintain a pre-computed rolling summary refreshed every N turns
  while capacity is ample; replay sends `summary + recent verbatim`. **Rate-status trigger**: Codex
  `token_count` (`rate_limits.*`, threshold e.g. 80%) drives proactive refresh now; Claude relies on the
  rolling summary + `api_retry`(429) until utilization is cleanly exposed.
- **Fallback chain** (§8.8/D13): ordered per-agent (and global default) list of profile entries (each
  1:1 with a provider); on rate-limit detection (R8.33) step to next target with replay; may cross
  providers; defined end-of-chain behaviour (surface, don't loop) (R8.38–R8.41).
- **Covers:** R4.3–R4.6, R6.2, R8.24–R8.41, D4, D8, D13.
- **Exit:** mid-chat `/profile` switch replays seamlessly; simulated rate-limit (fake adapter) steps the
  fallback chain Claude→Codex with intact history; rolling summary keeps replay bounded on long sessions.

### Phase 7 — Embedded scheduler, run history, provenance
**Goal:** Recurring agent routines run unattended and produce revisitable sessions.
- `internal/schedule` with `robfig/cron`: scan `~/.podium/schedules/*.md`, parse **YAML frontmatter**
  (`agent`,`model`,`effort`,`cron`/`every`,`run_permission`,`enabled`) + markdown body as the task
  (R7.2/R7.2a/D23); files are source of truth, no config block (R7.2).
- Each run = a **normal Podium session** against the named agent in its `workspace/` with full composed
  identity (R7.3/R7.3a), origin `schedule`, persisting schedule+run linkage so it can be revisited and
  continued manually and filtered by schedule (R7.9/R4.12).
- **Unattended permission policy** (§7.7): `yolo` (whole-machine, deliberate) or `preapproved`
  (allow-list; Claude `allowedTools` / Codex granular; non-listed → auto-deny). Default = stricter
  `preapproved` empty allow-list (R7.7/R7.8).
- Manual trigger ("Run now"), next-run times, run-history sparkline — readable/writable from CLI + web
  (R7.5).
- **Covers:** R7.1–R7.9, D15/D23, R4.12.
- **Exit:** drop a schedule file → it registers and fires; runs appear with history; open a run's session
  and continue it manually; disabled files don't fire; preapproved denies un-listed side effects.

### Phase 8 — Projects ledger, Roadmap/Docket kanban (tasks), remaining UI pages
**Goal:** Shared collaboration surface + the full designed UI beyond chat.
- **Project ledger** (§5.3/D22): shared `~/.podium/projects/projects.yaml` (R5.10 shape) + one dir per
  project; any agent reads/works any project (R5.9/R5.8 shared access, D11); base `AGENTS.md` instructs
  agents to work *with/against* the ledger (R5.11/R8.23); accept last-write-wins (R5.12).
- **Tasks/kanban (v1 extension):** `task` entity (project, assigned agent, status Backlog/In&nbsp;
  Progress/Review/Done, optional scheduled pickup time, freeform body). Persisted in SQLite. Actions:
  create/assign/move, **Start on demand** → spawn session (`origin = roadmap`, provenance banner "part of
  <project>" in chat), scheduled pickup → enqueue via scheduler. Tasks are **independent** (no DAG — stays
  within §2). Kanban board UI per design (drag between columns, "+ New task", "Open in chat").
- **Remaining pages** per design: **Agents** (list/manage, hire), **Schedules** (cards from Phase 7,
  run-now, run history), **Workspace/Projects** (project structure + per-agent workspace browse).
- **Covers:** R5.8–R5.12, R7.5 (UI), D11, D22, plus the confirmed kanban extension.
- **Exit:** create a project + task on the board, assign to an agent, Start → a roadmap-origin session
  opens with provenance; all nav pages functional and matching the design.

### Phase 9 — Cross-platform hardening, packaging, security & logging polish
**Goal:** Ship-quality single binary on all three OSes.
- Verify binary discovery / process-group kill / path handling on Windows + Linux + macOS (R10.1–R10.4);
  hung-agent cancellation paths.
- Single static binary builds with embedded SPA per OS; `PODIUM_HOME` override + configurable bind verified
  (Principle 7 — keeps HA add-on a later packaging step, not a rewrite).
- Security/log review pass: MCP/credential redaction (R8.29), no raw prompt leakage (R8.30), profile
  isolation (R8.32), `yolo` opt-in messaging (R8.31); structured run logging complete (R11.5).
- Finalize `docs/integrations/*.md` as the implemented contract; README/quickstart.
- **Covers:** R8.29–R8.33, R10.1–R10.4, R11.5, Principle 7.
- **Exit:** clean cross-OS builds run end-to-end; security review pass complete; docs current.

---

## Out of scope for v1 (per §2 — do not build, but don't preclude)
Inter-routine/inter-task DAG dependencies; OS-service boot persistence (`kardianos/service` stubbed only);
providers beyond Claude/Codex; multi-user/remote; HA add-on packaging; channel integrations beyond
web/cli/schedule/roadmap (the `origin` attribute is captured now — R4.10/R4.11 — so the data model needn't
change later); per-agent `tools: strict` replacement (additive only in v1 — R5.4/D1); ledger write
serialization (last-write-wins accepted — R5.12); concurrency cap (none in v1 — R11.3/D5).

## Notes / risks to watch
- **Kanban extends the spec.** Tasks are a new entity the requirements doc didn't define; keep them
  independent and project-scoped to avoid crossing the DAG out-of-scope line. Revisit the data model with
  the user at the start of Phase 8.
- **Claude rate-status is asymmetric** (R8.27): proactive rate-trigger ships for Codex; Claude leans on
  the rolling summary until utilization headers are cleanly exposed — don't block Phase 6 on it.
- **Codex double-loading** (R8.22a) must be verified against Codex's *actual* file-discovery behaviour
  during Phase 5, not assumed.
- **`approve` blocking + timeout** (R8.18): the Claude permission call blocks the agent — the auto-deny
  timeout is mandatory, not optional, from Phase 2.

## How each phase is verified
- **Automated:** unit/integration tests against the **fake adapter** (deterministic streams, scripted
  tool-calls, permission requests, and rate-limit signals) — exercises core, replay, fallback, scheduler,
  and the WS contract without real CLIs or token spend. SQLite round-trip/restart tests for durability.
- **Manual/integration:** real `claude`/`codex` runs for each adapter phase (stream a turn, approve/deny,
  resume after restart, mid-chat switch/replay, a scheduled run) — documented as a short runbook per phase.
- **Frontend:** drive the live daemon in a browser and compare against the design screenshots; type-check
  the shared WS/REST contract types.
- **Cross-platform:** Phase 9 runs the end-to-end runbook on Windows, Linux, and macOS.
