# Integration contracts

This directory holds the **per-provider integration contract** — exactly how
Podium drives each backing CLI. It is the source of truth the `internal/adapter`
layer implements against (requirements §8).

| Provider | Contract | Status |
| --- | --- | --- |
| Claude | [`claude.md`](claude.md) | implemented in Phase 2 |
| Codex | [`codex.md`](codex.md) | implemented in Phase 5 |

The grounding reference (process model, CLI parameters, app-server protocol, and
the OpenClaw policy values Podium deliberately inverts) is captured in
[`../requirements.md`](../requirements.md) Appendix A. Podium adopts the
*mechanisms* but inverts the *defaults* (`approve` over `bypassPermissions`,
inherit native MCP over strict host-only) per Principle 6 / §8.4b.

Two process models behind one interface (D7):

- **Claude — per-turn.** `claude -p` with stream-json stdin/stdout; resume via a
  persisted session ID + `--resume`.
- **Codex — long-lived app-server.** A single `codex app-server --listen
  stdio://` process; lifecycle via `thread/start` / `thread/resume` /
  `turn/start`; resume via a persisted `threadId`.
