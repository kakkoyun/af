---
adr: 039
title: "Multi-Agent Multi-Session Model"
status: proposed
implementation: pending
date: 2026-05-06
last_modified: 2026-05-06
supersedes: []
superseded_by: null
related: ["031", "037", "038", "043"]
tags: ["go", "agent", "session", "model"]
---

# ADR-039: Multi-Agent Multi-Session Model

## Context

The owner wants to run **multiple agents** (claude, pi, codex) on the
same workstream — for example, pi as the primary driver, claude in a
review pane, codex testing in a third pane. Each agent has its own
session lifecycle: it can be launched, stopped, resumed, and resumed
again. Across the lifetime of a workstream, an agent in slot `primary`
may go through many sessions (created, stopped, resumed, resumed,
stopped, etc.).

v0 had a slot model but conflated "current session ID" with "agent
identity." v1 separates them: a slot has **a list of session IDs**, in
chronological order of when they were associated with that slot.

Pi is the **default driver** per user directive.

## Decision

### Domain model

```
Workstream
├── 1 git worktree (the primary)
├── 0..N sub-worktrees (one per non-primary slot, ADR-038)
├── 1 tmux session
│   └── 1..N panes (one per agent slot)
└── 1..N agent slots
    └── each slot:
        ├── 1 agent provider (pi | claude | codex)
        ├── 1 tmux pane
        ├── 0..N session IDs (chronological, all sessions ever in this slot)
        ├── 1 status (running | stopped | crashed | suspended)
        └── 0..1 sub-worktree (if slot != "primary")
```

### Slot semantics

- **Slot name** is user-chosen at `af agent add --slot <name>` time, or
  auto-assigned (e.g. `pi`, `pi-2`, `claude`) if `--slot` is omitted.
- **`primary`** is reserved for the workstream's first agent (created
  by `af create`).
- **Slot names are unique within a workstream** but may repeat across
  workstreams.
- A slot persists across `af agent stop` followed by `af agent add
  --slot <same-name>` — the slot's `session_ids` list grows. This is
  how an agent "resumes" in the same slot.

### Session ID derivation

Each launch of an agent in a slot derives a new session ID:

```
session_id = uuid5(NAMESPACE_DNS, "{repo}/{branch}/{slot}/{launch-timestamp-ns}")
```

The timestamp ensures distinct sessions within the same slot have
distinct UUIDs, even though the slot identity (`(repo, branch, slot)`)
is stable.

Some agents (claude) accept a session ID. For those, `af` passes the
generated UUID. Others (pi, codex) don't expose a session ID flag at
launch time but support `--continue` / `resume --last`; for those, the
session ID is recorded for `af`'s tracking only.

### Default agent

Per user directive, the default for `af create` (when `--agent` is not
passed) is **pi**. Configurable via `[general].default_agent`.

### Pane assignment

- Slot `primary` lives in pane index 0 (the original window's pane).
- Each subsequent slot gets a new pane via `tmux split-window -v`. The
  `pane` field in `state.toml` records the tmux pane id (`%N`).
- `af agent stop` kills only that slot's pane; the workstream's tmux
  session survives unless the last pane is stopped — in which case the
  user must `af suspend` or `af done`.

### Resumption semantics

`af resume <session>` cold-rehydrates a workstream. For each slot:

1. Recreate the tmux pane (split if not primary).
2. Generate a **new** session ID and append to `session_ids[]`.
3. Launch the agent with its provider-specific resume command:
   - `pi --continue`
   - `claude --continue` (with `--session-id <new-uuid>` if needed)
   - `codex resume --last`
4. Record `agent_resumed` event in the ledger with the new session ID.

The agent itself decides what state to restore. `af` does not
manipulate agent log files (per ADR-043, `af` never deletes or
modifies them).

### `af agent` subcommands

| Command | Behaviour |
|---|---|
| `af agent add --slot <name> --agent <provider> [--session NAME]` | Create slot, sub-worktree if applicable (ADR-038), pane, launch. Append `agent_added` and `agent_launched` ledger events. |
| `af agent stop <slot> [--remove-worktree]` | Kill pane, mark slot status `stopped`. Append `agent_stopped` ledger event. With `--remove-worktree`, also `git worktree remove` the sub-worktree. |
| `af agent list [--session NAME]` | Tabular: slot, agent, status, pane, session_ids count, last session timestamp. |

### Crash detection

When `af list` runs, it checks each slot's pane for liveness:

- If `tmux list-panes` doesn't show the pane, mark the slot `crashed`
  and append `agent_crashed` to the ledger.

Pane-process exit code is not reliably available via tmux without a
hook; v1 doesn't try. Slots that exited cleanly via `af agent stop`
are `stopped`; slots whose pane vanished without `af agent stop` are
`crashed`.

## Consequences

- One workstream can host multiple AI agents working in parallel.
- A slot's history (every session ID it has ever held) is preserved in
  `state.toml` for analysis.
- The owner can `af note` to find the workstream's Obsidian note and
  see all agents involved across all sessions, not just the most
  recent.
- Pi defaulting reflects actual usage; claude and codex stay one flag
  away.

## Alternatives Considered

- **One session per slot, no resumption history.** Rejected; the owner
  wants to see a slot's full timeline.
- **Slot identity == session ID.** Rejected; conflates two concepts.
  Resuming an agent should not require renaming a slot.
- **No slot model; every agent is a separate workstream.** Rejected;
  defeats the multi-agent-on-one-codebase goal.

## References

- v0 ADR-001 (Agent Provider, multi-agent model) — superseded by this ADR.
- v0 ADR-011 (Workstream Lifecycle) — ledger events carried over.
- ADR-031 — v1 master.
- ADR-037 — session metadata (slot fields).
- ADR-038 — worktree layout (sub-worktrees).
- ADR-043 — agent providers.
