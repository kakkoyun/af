---
adr: 054
title: "af status — Workstream Dashboard"
status: proposed
implementation: pending
date: 2026-05-08
last_modified: 2026-05-08
supersedes: []
superseded_by: null
related: ["031", "037", "038", "046", "047", "048"]
tags: ["go", "command", "status", "dashboard"]
---

# ADR-054: `af status` — Workstream Dashboard

## Context

The owner currently has no single command answering "what am I working on?" `af list`
(implied by ADR-037's discovery rules but not its own ADR yet) prints a flat list of
workstream names. To see a workstream's PR state, lifecycle, or branch, the owner has to
`cat state.toml`, run `gh pr list`, and cross-reference manually.

Datadog's `gv status` solves this with a unified dashboard: worktrees, stacks, PRs, CI,
active sessions. The pattern transfers cleanly to af: every input it needs already lives
in `state.toml` (per ADR-037) plus a single concurrent `gh pr view` rollup.

## Decision

### Command

```
af status [--json] [--all] [--filter STATE]
```

| Flag              | Behaviour                                                                              |
| ----------------- | -------------------------------------------------------------------------------------- |
| (default)         | Tabular output, columns below, sorted by `last_touched_at` descending                 |
| `--json`          | Emit JSON array; one object per workstream, schema documented below                   |
| `--all`           | Include `completed` and `abandoned` workstreams from `archive/`                        |
| `--filter STATE`  | Filter by lifecycle state: `active`, `suspended`, `completed`, `abandoned`             |

### Default columns

```
SESSION                       BRANCH                      STATE      AGENTS    PR     CI       LAST
kakkoyun--issue-42            kakkoyun/issue-42           active     pi+claude #142   passing  2m ago
kakkoyun--refactor-config     kakkoyun/refactor-config    suspended  pi        #138   pending  3h ago
```

- `STATE` from `state.toml.[session].status`. **Stacked workstreams**
  (per ADR-059, with `[stack].parent_session` non-empty) get a
  `→<parent_session>` suffix appended to STATE: e.g.
  `active→feat-a` or `suspended→feat-a`. The arrow makes the parent
  link visible at a glance without widening the table.
- `AGENTS` is `+`-joined slot providers from `state.toml.[[agents]]`.
- `PR` from `state.toml.[pr].number`; resolved live via `gh pr view --json state,statusCheckRollup`.
- `CI` from `gh` JSON `statusCheckRollup`; one of `passing | failing | pending | none`.
- `LAST` from `last_touched_at` (latest `ts` in `ledger.jsonl`).

### Concurrent `gh` fetch

For each workstream with a non-zero PR number, spawn a goroutine that runs
`gh pr view <num> --json state,statusCheckRollup --repo <repo>`. Use `sync.WaitGroup`
plus a buffered channel; cap parallelism at 8 (configurable via `[status].max_parallel`,
default 8). Each fetch carries a 5-second `context.WithTimeout`. On timeout or non-zero
exit, emit `state="?"`, `ci="?"` rather than failing the whole render.

### Output stability

`--json` output is **machine-stable** (sorted by session name, fixed key order via Go
`encoding/json` plus a struct definition). The tabular default is human-only.

### Discovery

Iterate `~/.local/share/af/v1/sessions/*/state.toml` plus, with `--all`,
`~/.local/share/af/v1/archive/*/state.toml`. Take a shared `flock` per ADR-037. Skip
files that fail to parse with a `slog.Warn`.

### Empty result

```
No workstreams. Create one with: af create <name>
```

## Consequences

- A single command answers the owner's most common day-start question.
- Concurrent `gh` calls keep render time sub-second for typical (≤20) workstream counts.
- The `--json` output enables shell pipelines (`af status --json | jq '.[] | select(.ci=="failing")'`)
  without an `af stats` command (dropped per ADR-031).
- No new dep: `gh` is already required (per ADR-044 doctor probes).
- The Bases aggregator (ADR-047) and `af status` answer the same question from
  different angles — Bases for vault-side glanceability, `af status` for terminal-side.

## Alternatives Considered

- **Read `state.toml` only, no `gh`.** Rejected; PR state goes stale without live fetch.
- **Cache `gh` results to disk.** Rejected for v1; the cap-at-8 concurrent fetch is fast enough.
- **Render via TUI (bubbletea).** Rejected; adds a transitive dep tree for marginal gain.
- **Fold into `af list`.** Rejected; `af list` is name-only and useful in scripts.
  `af status` is the human-readable view.

## References

- ADR-031 — v1 master.
- ADR-037 — `state.toml` schema (data source).
- ADR-038 — workstream layout (discovery).
- ADR-046 — lifecycle states feeding the `STATE` column.
- ADR-047 — Obsidian Bases is the vault-side counterpart.
- ADR-048 — `af pr` updates the `[pr]` block this command renders.
