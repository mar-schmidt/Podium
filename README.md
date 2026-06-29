# Podium

A thin orchestration layer for local LLM agents (Claude Code, OpenAI Codex).
Podium shells out to the native `claude` and `codex` CLIs and leans on *their*
MCP, tools, and memory, while owning its own durable truth: named agents, durable
chat sessions, a canonical history that replays onto a fresh backing CLI session
on any profile/provider switch, an embedded scheduler, and a shared project
ledger. It ships as a single Go binary with an embedded Svelte web UI.

> **Status:** under active development. This is **Phase 0 — Foundations**: the
> daemon boots, scaffolds `~/.podium/`, opens its database, and serves a health
> endpoint plus the embedded SPA shell. See
> [the implementation plan](#implementation-plan) for what's coming.

## Quick start (dev)

Prerequisites: Go 1.26+, Node 20+ (for building the web UI).

```sh
# 1. Build the web UI (embedded into the daemon).
cd web && npm install && npm run build && cd ..

# 2. Build the binaries.
make build          # or: go build ./cmd/podiumd && go build ./cmd/podium

# 3. Run the daemon (foreground). It scaffolds ~/.podium on first run.
./podiumd

# 4. In another shell, check it's live.
./podium status
```

Open http://127.0.0.1:8787 for the web UI.

To develop the frontend with hot reload, run `npm run dev` in `web/` (it proxies
API/WebSocket traffic to a running `podiumd`).

## Layout

```
cmd/podium/     thin CLI client
cmd/podiumd/    daemon: web server + scheduler + core
internal/       core, adapter, exec, schedule, config, store, server, client
web/            Svelte + Vite + TS + Tailwind SPA (built → embedded)
docs/           requirements, CLI reference, configuration, integration contracts
```

All runtime state lives under `$PODIUM_HOME` (default `~/.podium/`).

## Documentation

- [Implementation plan](docs/implementation-plan.md) — **the living, phased plan
  and progress log. Start here if you're picking up the work.**
- [Requirements](docs/requirements.md) — the authoritative spec (v1.6).
- [CLI reference](docs/cli.md)
- [Configuration](docs/configuration.md)
- [Integration contracts](docs/integrations/README.md)

## Implementation plan

Podium is built in reviewable phases, tracked in
[docs/implementation-plan.md](docs/implementation-plan.md) — the shared,
version-controlled source of truth for what's done and what's next. Phase 0
establishes the foundations; subsequent phases add the core domain, the Claude
and Codex adapters, the chat web UI, profiles/replay/fallback, the scheduler, and
the projects/roadmap UI. The plan's Progress log is updated as each phase lands.
