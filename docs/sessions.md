# Sessions

A Podium session is the durable conversation unit. It stores the bound agent,
current settings, immutable origin, provider resume handle, rolling summary area,
and the full ordered message history in SQLite.

## Origin

Every session has one origin set at creation:

| Origin | Meaning |
| --- | --- |
| `web` | Created by the web UI. |
| `cli` | Created by the CLI. |
| `schedule` | Created by a scheduled run. |
| `roadmap` | Created from a roadmap task. |

Origin is provenance only. A session can later be continued from another
channel, but its origin does not change.

## Schedule Linkage

Sessions have nullable `schedule_id` and `run_id` fields so scheduled sessions
can link back to the schedule/run that produced them. The scheduler itself is
implemented in a later phase.

## History

Message history is stored as strictly ordered `user` and `assistant` messages.
The provider's own session or thread is treated as a resumable backing resource;
Podium's SQLite history is the canonical truth that survives daemon restarts.

Phase 1 drives turns through a deterministic fake adapter. Real provider turns,
streaming UI, replay, and rolling-summary refresh are layered on this model in
later phases.
