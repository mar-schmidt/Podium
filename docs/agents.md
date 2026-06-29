# Agents

Agents are durable, named colleagues maintained by Podium. Each agent has stored
defaults for provider, profile, model, effort, permission mode, fallback chain,
and optional additive MCP config.

Creating an agent scaffolds:

```text
$PODIUM_HOME/agents/<name>/
  SOUL.md
  workspace/
```

`SOUL.md` is always created from a small identity skeleton and is user-owned.
`workspace/` is the cwd used by backing provider processes in later phases.
Podium does not create `agents/<name>/AGENTS.md`; that file is optional and left
for the user to add when an agent needs extra standing instructions.

## Instruction Layers

Podium composes agent instructions in this fixed order:

1. `$PODIUM_HOME/AGENTS.md`
2. `$PODIUM_HOME/agents/<name>/AGENTS.md` when present
3. `$PODIUM_HOME/agents/<name>/SOUL.md`

The delivery artifact depends on the provider:

| Provider | Workspace artifact | Contents |
| --- | --- | --- |
| Claude | `workspace/CLAUDE.md` | A generated file with `@` imports for each instruction source. |
| Codex | `workspace/AGENTS.md` | A generated bundle concatenating the instruction sources in order. |

Phase 1 only produces these payloads and tests them with a fake adapter. Real
Claude and Codex CLI wiring lands in later phases.
