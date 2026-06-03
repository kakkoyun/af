---
adr: 071
title: "PR State Lifecycle — TTL-Cached Refresh"
status: proposed
implementation: complete
date: 2026-05-21
last_modified: 2026-05-22
supersedes: []
superseded_by: null
related: ["036", "037", "048", "054", "055", "056", "057", "059", "068"]
tags: ["go", "pr", "github", "cache", "ledger", "lifecycle"]
---

# ADR-071: PR State Lifecycle — TTL-Cached Refresh

## Context

`state.toml.[pr]` (ADR-037) holds `number`, `url`, and `state`:

```toml
[pr]
number = 0
url    = ""
state  = ""   # "" | open | draft | closed | merged
```

Several commands depend on the `state` field:

- `af status` (ADR-054) renders the PR column.
- `af info` (ADR-055) shows the PR block.
- `af clean` (ADR-056) reaps merged workstreams (three-strategy
  merge detection; PR state is one input).
- `af sync` (ADR-059) reparents when the parent's PR is merged.
- `af done` (ADR-046) uses PR state plus merge detection to
  decide `completed` vs `abandoned`.

But no ADR says **who writes `state`, when**. ADR-054 runs `gh pr
view --json state` and renders the result but is unclear about
write-back. ADR-056's merge detection is independent of the
`[pr].state` cache. The contract has been ambiguous since ADR-037.

The cost shape is also unclear. `gh pr view` is a `~200–400ms`
network call. Running it on every `af list` would make `af list`
feel slow. Running it never makes the dashboard stale.

This ADR pins down the cache policy and the write contract.

## Decision

`state.toml.[pr].state` is a **TTL-bounded cache** of the upstream
PR state. It is not the source of truth — that's GitHub via `gh`.

### Schema additions

```toml
[pr]
number              = 42
url                 = "https://github.com/..."
state               = "open"        # cached
last_refreshed_at   = 2026-05-21T15:00:00Z   # added by this ADR
last_refresh_error  = ""                     # populated on failed refresh; cleared on success
```

```toml
[pr.cache]
ttl = "10m"   # optional repo/user config override; default 10m
```

`last_refreshed_at` is recorded in UTC. Zero/empty means "never
refreshed" — a fresh `gh pr view` is mandatory before any consumer
reads `state`.

### Config addition

```toml
[pr]
refresh_ttl = "10m"   # Go time.ParseDuration syntax
```

Lives in user or repo config (ADR-036 layered merge). Default
`10m`. Set to `0s` to force always-refresh (useful for testing).

### Refresh behaviour by command

| Command            | Refresh policy                                                                                            |
| ------------------ | --------------------------------------------------------------------------------------------------------- |
| `af list`          | **Never**. `af list` does not display PR state and stays instant.                                         |
| `af status`        | Refresh if `now - last_refreshed_at > refresh_ttl`. Inside TTL → cached. `--refresh` forces refresh.       |
| `af info`          | Refresh if outside TTL. `--refresh` forces refresh.                                                       |
| `af clean`         | **Always** force-refresh (the merge decision is correctness-critical). Per `[status].max_parallel` cap.    |
| `af sync`          | **Always** force-refresh for the parent workstream (reparenting decision is correctness-critical).         |
| `af done`          | **Always** force-refresh once before deciding `completed` vs `abandoned`.                                  |
| `af pr` (open new) | Writes `number`, `url`, `state = "open"` (or `"draft"` for `--draft`), `last_refreshed_at = now`.          |
| `af pr --refresh`  | Force-refresh without opening anything; no-op when `number == 0` (errors with `EX_DATAERR`).              |
| `af retro`         | **Never** refreshes. Operates on archived `state.toml`s; their PR state is whatever was last captured.    |

Force-refresh paths ignore the TTL but still respect
`[status].max_parallel` (ADR-054) when fanning out across multiple
workstreams.

### Refresh implementation

```text
gh pr view <number> --repo <repo_slug> --json state,isDraft,mergedAt,closedAt
```

with a per-fetch 5-second `context.WithTimeout`. Response mapping:

| GitHub response                       | af cached `state` |
| ------------------------------------- | ----------------- |
| `state: OPEN`, `isDraft: true`        | `"draft"`         |
| `state: OPEN`, `isDraft: false`       | `"open"`          |
| `state: CLOSED`, `mergedAt: null`     | `"closed"`        |
| `state: CLOSED`, `mergedAt: <stamp>`  | `"merged"`        |

On non-zero `gh` exit, network timeout, or rate-limit response:

- `last_refresh_error` is set to the error message (truncated to
  120 chars).
- `last_refreshed_at` is **not** advanced.
- `state` is left at the previous cached value.
- Tabular renderers (status, info) show `?` for the PR column on
  that workstream and emit a `slog.Warn` once per command
  invocation (not once per failed fetch — avoids log spam).

`af clean` and `af sync` treat refresh failure as a hard error
(`EX_TEMPFAIL`, 75) for the affected workstream and skip it from
the reap / sync batch with a clear stderr message. They never
silently fall through to merge detection on stale data.

### Ledger events

The append-only ledger records **only state flips**, not every
refresh attempt. Event types added by this ADR:

```json
{"ts": "...", "event": "pr_state_changed", "from": "open", "to": "merged", "number": 42, "url": "..."}
```

Plus the existing ledger events (`pr_opened`, `pr_merged`,
`pr_closed`) — these stay defined in ADR-037, but are now derived
from the flip detector:

- The first flip from `""` to anything emits `pr_opened` (covers
  both `af pr` and "PR opened externally and discovered on
  refresh").
- A flip `to == "merged"` emits `pr_merged`.
- A flip `to == "closed"` emits `pr_closed`.
- Every flip also emits the verbose `pr_state_changed` for retro.

Failed refresh attempts emit nothing. The error trail lives in
`last_refresh_error` and (optionally) `slog`.

### Concurrency

PR-state writes acquire the per-session lock from ADR-068 §4
because they modify `state.toml` and append to `ledger.jsonl`.
Concurrent `af status` invocations may queue briefly during
refresh; the second invocation typically finds the cache fresh
and skips the call.

## Consequences

- `af list` stays sub-100ms; no network.
- `af status` and `af info` are bounded by the TTL; the dashboard
  is at most `refresh_ttl` stale.
- `af clean` / `af sync` / `af done` are correctness-critical and
  always pay the refresh cost. Their failures are loud.
- The ledger gains one new event class (`pr_state_changed`) with a
  clean before/after pair, useful for `af retro --ai` narrative
  generation.
- `gh` rate limits are respected — TTL ensures we don't hammer.
- The cache stays self-healing: a single fresh refresh advances
  the timestamp and clears `last_refresh_error`.

## Alternatives Considered

- **Pure lazy cache, never expires (refresh only on user request).**
  Rejected: dashboard staleness becomes invisible until the user
  notices.
- **Eager refresh on every invocation.** Rejected: makes `af list`
  network-bound and slow; pointless for the 90% case where state
  didn't change.
- **Background daemon with periodic refresh.** Rejected: violates
  ADR-031's "no daemon" posture and ADR-069 §1's network promise
  (daemon would refresh outside user-invoked commands).
- **Webhook subscription to GitHub.** Out of scope for a single-user
  local CLI; requires a public endpoint or polling proxy.
- **Make `state` the source of truth and never call `gh`.**
  Rejected: drift becomes invisible; merge detection in `af clean`
  would silently miss externally-merged PRs.

## References

- ADR-036 — config layered merge; `[pr].refresh_ttl` lives here.
- ADR-037 — state.toml schema (`[pr]` block). This ADR adds
  `last_refreshed_at` and `last_refresh_error` fields; non-breaking
  per ADR-072.
- ADR-048 — `af pr` is the writer for the initial `state = "open"`.
- ADR-054 — `af status --refresh`; `[status].max_parallel` cap also
  governs force-refresh fan-out.
- ADR-055 — `af info --refresh`.
- ADR-056 — three-strategy merge detection in `af clean`; this ADR
  ensures the cache is fresh before that detector runs.
- ADR-057 — `af pr --ai` writes the PR via the same path as
  vanilla `af pr`.
- ADR-059 — `af sync` parent refresh.
- ADR-068 §2 — exit codes (`EX_DATAERR` for `--refresh` without PR,
  `EX_TEMPFAIL` for refresh failures in correctness paths).
- ADR-068 §4 — per-session lock acquired for writes.
- GitHub CLI: <https://cli.github.com/manual/gh_pr_view>
