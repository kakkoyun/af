---
adr: 055
title: "af info — Workstream Detail View"
status: proposed
implementation: pending
date: 2026-05-08
last_modified: 2026-05-08
supersedes: []
superseded_by: null
related: ["031", "037", "038", "039", "054"]
tags: ["go", "command", "info", "introspection"]
---

# ADR-055: `af info` — Workstream Detail View

## Context

`af status` (ADR-054) gives a one-line-per-workstream overview. The owner often wants
the full picture for one workstream: every slot, every session ID, the ledger tail,
sandbox/remote attachment, base branch — without `cat`-ing four files.

Datadog's `gv info <name>` solves the equivalent. For af this is essentially a
pretty-print of `state.toml` plus the last N ledger events.

## Decision

### Command

```
af info [session] [--json] [--ledger N]
```

| Flag         | Behaviour                                                                      |
| ------------ | ------------------------------------------------------------------------------ |
| `[session]`  | Workstream name; resolved per ADR-037 file-discovery rules if omitted          |
| `--json`     | Emit the merged `state.toml` + ledger tail as JSON                             |
| `--ledger N` | Show the last N ledger events (default 10; `--ledger 0` suppresses)            |

### Default output

```
Session     kakkoyun--issue-42  (active, started 3h ago)
Repo        af  @ kakkoyun/issue-42  (base: upstream/main)
Worktree    ~/Workspace/.worktrees/af/kakkoyun--issue-42
Execution   local + sandbox=slicer (vm-abc123)
Agents
  primary   pi      running    pane=%0   sessions=2
  review    claude  running    pane=%1   sessions=1   sub-worktree=...--review
PR          #142  open  https://github.com/kakkoyun/af/pull/142  CI: passing
Note        ~/Vaults/personal/00 - af/kakkoyun--issue-42.md
Ledger (last 10)
  12:00:00  session_created   mode=sandbox  agents=[primary]
  12:00:01  agent_launched    slot=primary  agent=pi
  12:18:42  agent_added       slot=review   agent=claude
  ...
```

### Data sources

Single read of `state.toml` (shared `flock`) + tail of `ledger.jsonl` (read-only).
`af info` performs no network I/O — PR state shown is whatever `state.toml.[pr]` was
last set to (by `af pr` per ADR-048). For live PR state, the user runs `af status` or
`gh pr view`.

### `--json` schema

```json
{
  "session": { "...all of state.toml.[session]..." },
  "worktree": { "..." },
  "execution": { "..." },
  "agents": [ "..." ],
  "pr": { "..." },
  "note_path": "...",
  "ledger_tail": [ {"ts": "...", "event": "...", "...": "..."} ]
}
```

Field order is stable. Adding fields is a minor (non-breaking) change; removing a field
requires a schema bump.

## Consequences

- One command shows the full state of one workstream — no more `cat` chains.
- No new deps; no network I/O.
- Pairs naturally with `af status` (overview) for a fleet/single split.

## Alternatives Considered

- **Fold into `af status <name>`.** Rejected; `af status` is plural-by-design and its
  flag matrix (`--all`, `--filter`) doesn't fit a single-workstream view cleanly.
- **Live PR fetch in `af info`.** Rejected; `af status` already does that, and `af info`
  benefits from being instant + offline-capable.
- **Render the ledger as a tree of nested events.** Rejected; flat tail is faster to
  read and easier to grep.

## References

- ADR-031 — v1 master.
- ADR-037 — state and ledger schemas.
- ADR-038 — discovery rules.
- ADR-039 — slot model rendered in `Agents`.
- ADR-054 — `af status` (companion command).
