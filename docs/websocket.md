# WebSocket Contract

Phase 3 adds a browser-native WebSocket endpoint at:

```text
GET /api/ws
```

The browser sends JSON client messages and receives JSON server messages. The
TypeScript mirror lives in `web/src/lib/types.ts`; the Go source of truth lives
in `internal/server/ws_contract.go`.

## Client Messages

```json
{"type":"list"}
```

Refresh agents and sessions.

```json
{"type":"create_session","request_id":"...","agent_name":"jared"}
```

Create a web-origin session.

```json
{"type":"send_turn","request_id":"...","agent_name":"jared","message":"Hello"}
{"type":"send_turn","request_id":"...","session_id":"...","message":"Continue"}
```

Send a turn to a new or existing session.

Slash commands use the same `send_turn` envelope and return `notice`, `session`,
and `done` messages rather than provider deltas.

Web turns are daemon-owned. Closing or reconnecting the socket does not cancel
an active turn; the browser can reattach to the session:

```json
{"type":"attach_session","request_id":"...","session_id":"..."}
```

Explicit user cancellation is session-scoped:

```json
{"type":"stop_turn","request_id":"...","session_id":"..."}
```

Session settings can be changed without writing a slash command into chat
history:

```json
{"type":"update_session_settings","request_id":"...","session_id":"...","permission_mode":"yolo"}
```

```json
{
  "type": "permission_decision",
  "request_id": "<permission request id>",
  "decision": {"behavior":"allow","updatedInput":{}}
}
```

Answer an inline permission request. Denies use:

```json
{"behavior":"deny","message":"Denied from web"}
```

## Server Messages

| Type | Payload |
| --- | --- |
| `hello` | Connection acknowledgement. |
| `state` | `agents`, `sessions`, `active_turns`. |
| `session` | Active/created session. |
| `history` | Ordered stored messages. |
| `message` | One stored user or assistant message. |
| `delta` | Incremental assistant text. |
| `assistant` | Final assistant text fallback. |
| `permission_request` | Tool approval request. |
| `user_input_request` | Provider/user clarification request. |
| `turn_state` | Current active-turn snapshot for a session. |
| `notice` | Non-history UI notice, usually from slash commands. |
| `done` | Turn complete. |
| `error` | Error string. |

Session payloads include display metadata (`Name`, `Description`, `AutoNamed`)
and current settings (`Model`, `Effort`, `PermissionMode`). Permission requests
include `expires_at` when a timeout is active so clients can show an auto-deny
countdown.

## REST Support

The web UI also uses REST for initial CRUD and history fetches:

| Endpoint | Purpose |
| --- | --- |
| `GET /api/agents` | List agents. |
| `POST /api/agents` | Create an agent. |
| `GET /api/agents/{name}` | Get one agent. |
| `GET /api/sessions` | List sessions. |
| `POST /api/sessions` | Create a session. |
| `GET /api/sessions/{id}` | Get one session and ordered history. |

The older Phase 2 NDJSON `POST /api/chat` endpoint remains for the CLI.
