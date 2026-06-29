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

---

*More commands (agents, sessions, chat, profiles, schedules, projects) are added
as later phases land; each gets an entry here when it ships.*
