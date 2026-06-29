# Podium CLI reference

Podium ships two binaries:

- **`podiumd`** — the long-running daemon. Owns all session, agent, and schedule
  state; serves the web UI and API; runs the embedded scheduler.
- **`podium`** — a thin client that always talks to a running `podiumd`. It does
  not run sessions in-process (R11.1 / D2).

This page is kept in sync with the binaries' built-in help (`--help` on any
command is authoritative).

## Storage root

All Podium state lives under a single root:

- Default: `~/.podium/`
- Override: set the `PODIUM_HOME` environment variable.

On first start, `podiumd` scaffolds the tree and writes a commented default
`config.yaml`, the Podium-owned base `AGENTS.md`, and an empty project ledger.
Existing files are never overwritten.

## `podiumd`

Run the daemon (foreground):

```
podiumd
```

| Flag | Description |
| --- | --- |
| `--help` | Show help. |
| `--version` | Print version and commit. |

Bind address and port come from `config.yaml` (`server.bind`, `server.port`;
default `127.0.0.1:8787`).

## `podium`

Global flags:

| Flag | Description |
| --- | --- |
| `--addr host:port` | Daemon address. Precedence: `--addr` → `PODIUM_ADDR` → `config.yaml` → `127.0.0.1:8787`. |
| `--version` | Print version and commit. |

### `podium status`

Report whether the daemon is running, plus its version and uptime.

```
podium status
podium --addr 127.0.0.1:8787 status
```

Exits non-zero with a "start it with: podiumd" hint when the daemon is
unreachable.

### `podium agents list`

List durable agents known to the daemon.

```
podium agents list
```

### `podium agents create`

Create an agent through `podiumd`. This stores the agent, creates
`$PODIUM_HOME/agents/<name>/SOUL.md`, and creates its `workspace/`.

```
podium agents create jared
podium agents create builder --provider claude --model sonnet --effort medium --permission approve
```

| Flag | Description |
| --- | --- |
| `--provider claude|codex` | Provider for the agent. Empty inherits `global.provider`. |
| `--model name` | Default model. Empty means provider default. |
| `--effort level` | Default effort (`low`, `medium`, `high`, `xhigh`, `max`). |
| `--permission approve|yolo` | Agent permission default. |

### `podium chat`

Send one chat turn through the daemon. Use `--agent` to create a new CLI-origin
session or `--session` to continue an existing session.

```
podium chat --agent jared "Summarise this workspace"
podium chat --session <session-id> "Continue"
```

In `approve` mode, permission requests are shown inline. Answer `y`/`yes` to
allow the requested tool input unchanged; any other answer denies it. Unanswered
requests auto-deny after `global.permission_timeout`.

Slash commands can be sent as the message body:

| Command | Effect |
| --- | --- |
| `/model <name>` | Set the session model for subsequent turns. |
| `/effort low|medium|high|xhigh|max` | Set reasoning effort. |
| `/profile <name|default>` | Switch auth profile. `default` clears the profile; the next turn replays history into a fresh backing session/thread. |
| `/permission approve|yolo` | Set permission mode. |
| `/name <text>` | Rename the session. |
| `/describe <text>` | Set the session description. |
| `/help` | Print command help. |

---

*More commands and flags (schedules, projects, richer chat controls)
are added as later phases land; each gets an entry here when it ships.*
