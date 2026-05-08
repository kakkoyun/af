---
adr: 037
title: "Session Metadata Schema (state.toml + ledger.jsonl)"
status: proposed
implementation: pending
date: 2026-05-06
last_modified: 2026-05-08
supersedes: []
superseded_by: null
related: ["031", "036", "038", "039", "046", "059"]
tags: ["go", "session", "state", "ledger"]
---

# ADR-037: Session Metadata Schema (state.toml + ledger.jsonl)

## Context

Each workstream needs persistent metadata to survive shell exits, tmux
crashes, machine reboots, and `af suspend` cycles. v0 split this into a
mutable TOML state file (current snapshot) and an append-only JSONL
ledger (event history). v1 keeps this two-layer model but tightens the
schema and adds a `schema_version` for future migrations.

The schema must support:

- A workstream with **multiple agents** (ADR-039), each in its own slot.
- An agent that has been **resumed many times**: each slot tracks all of
  its session IDs, in chronological order.
- **Sub-worktrees** for subagents (ADR-038).
- **Suspended** lifecycle state (ADR-046) without losing slot membership.
- **Remote** and **sandbox** metadata when applicable.

## Decision

### Storage layout

```
~/.local/share/af/v1/
├── sessions/
│   └── <session-name>/
│       ├── state.toml       # mutable, current snapshot
│       └── ledger.jsonl     # append-only event log
└── archive/
    └── <session-name>/      # moved here by `af done`
        ├── state.toml
        └── ledger.jsonl
```

Per-repo discovery symlink (per ADR-038): `<repo>/.af/state.toml` →
absolute path of the canonical `state.toml`.

### `state.toml` schema (`schema_version = 1`)

```toml
schema_version = 1

[session]
name        = "kakkoyun--issue-42"        # tmux-sanitized
id          = "<uuid v5>"                 # uuid5(repo, branch, "session")
created_at  = 2026-05-06T12:00:00Z
status      = "active"                    # active | suspended | completed | abandoned
suspended_at = null                       # set when status = "suspended"

[worktree]
path        = "/Users/kemal/Workspace/.worktrees/af/kakkoyun--issue-42"
branch      = "kakkoyun/issue-42"
base_branch = "upstream/main"
git_root    = "/Users/kemal/Workspace/Projects/Personal/af"

[execution]
mode             = "local"          # local | bare | remote | sandbox
multiplexer      = "tmux"
tmux_session     = "kakkoyun--issue-42"
ssh_host         = ""               # populated for remote mode (ADR-041)
remote_path      = ""               # workstream path on the remote host
sandbox_provider = ""               # "" | "slicer" | "sbx" (ADR-042)
sandbox_id       = ""               # provider-specific id (slicer VM hostname, sbx ID)

[[agents]]
slot          = "primary"
provider      = "pi"                # ADR-043
session_ids   = ["<uuid v5>", ...]  # all session IDs ever associated with this slot
pane          = "%0"                # tmux pane id
status        = "running"           # running | stopped | crashed | suspended
sub_worktree  = ""                  # absolute path to sibling sub-worktree, if any
sub_branch    = ""                  # branch name of the sub-worktree
created_at    = 2026-05-06T12:00:00Z
last_resumed_at = null              # null until first resume

[pr]
number = 0                          # 0 = no PR yet
url    = ""
state  = ""                         # "" | "open" | "merged" | "closed"

[stack]
parent_session = ""                 # workstream name of the parent; "" = no parent (ADR-059)
parent_branch  = ""                 # resolved branch name of the parent at link time
linked_at      = null               # timestamp the parent was set

[versions]
af = "1.0.0"
agent_versions = { pi = "...", claude = "..." }
```

### `ledger.jsonl` event schema

One JSON object per line, sorted by timestamp ascending. Never edited,
only appended. Each line is a complete record (no continuation).

Common fields on every event:

```json
{"ts": "2026-05-06T12:00:00.123Z", "event": "<event>", ...}
```

Event types:

| Event | Required fields | Description |
|---|---|---|
| `session_created` | `mode`, `branch`, `base_branch`, `agents` (initial slot list), `af_version` | `af create` |
| `session_suspended` | `active_slots` | `af suspend` |
| `session_resumed` | `recovery` (`"warm"` if tmux still alive, `"cold"` if rehydrating from disk) | `af resume` |
| `session_completed` | `duration_seconds` | `af done` clean |
| `session_abandoned` | `reason`, `duration_seconds` | `af done --force` on unmerged |
| `agent_launched` | `slot`, `agent`, `session_id`, `pane`, `cmd` | New agent process started |
| `agent_added` | `slot`, `agent`, `pane`, `cmd` | `af agent add` |
| `agent_resumed` | `slot`, `agent`, `cmd` | Agent re-attached |
| `agent_stopped` | `slot`, `agent`, `reason` | `af agent stop` or session teardown |
| `agent_crashed` | `slot`, `agent`, `exit_code` | Detected non-zero exit |
| `pr_opened` | `number`, `url` | `af pr` or detection |
| `pr_merged` | `number` | Detection on `af done` |
| `pr_closed` | `number` | Detection on `af done` |
| `error` | `op`, `message` | Recoverable error logged for diagnostics |

Slot-scoped events always carry `slot` and `agent` to disambiguate
multi-agent workstreams.

### Atomic writes

`state.toml` writes are **atomic**: write to `state.toml.tmp`, `fsync`,
then `rename` over `state.toml`. Prevents corruption on crash.

`ledger.jsonl` writes append a single line via `O_APPEND | O_CREATE`
with `O_SYNC` to ensure ordering on disk. Each line ends with `\n`.

### Concurrency

A single workstream has one writer at a time (the `af` invocation
operating on it). Cross-process locks via `flock(2)` on
`<session>/state.toml.lock`. Acquired on entry to mutating operations
(`af create`, `af agent add`, `af agent stop`, `af suspend`, `af done`,
`af gc`, `af resume` on cold rehydrate); read-only operations
(`af list`, `af note`, `af note --append` against a single workstream)
take a shared lock.

**`af list` is strictly read-only.** It displays whatever is in
`state.toml` for each workstream. Drift between `state.toml` and the
actual tmux/sandbox state (e.g. a slot whose pane has been killed
out-of-band) is reconciled lazily by the next mutating command that
touches the affected slot — see ADR-039 §"Crash detection (lazy)". A
user who wants explicit reconciliation without side-effecting any
workstream runs `af gc`.

`flock` is stdlib via `golang.org/x/sys/unix` (a quasi-stdlib package).
Approved as a transitive-of-stdlib dep, no separate ADR.

### Schema migrations

`Load(state.toml)` reads `schema_version` first. If it's `1`, parse as
v1. If it's higher than the binary supports, return
`ErrSchemaTooNew`. If lower than the binary supports, run the
appropriate migration (none needed for v1.0).

Future schema bumps add a new value and a migration step. Old binaries
refuse new schemas; new binaries upgrade old schemas in place.

### File-discovery rules

`af` resolves "the current workstream" as follows, in order:

1. If `--session NAME` is passed: load `~/.local/share/af/v1/sessions/NAME/state.toml`.
2. Else if inside a tmux session named after a workstream: load that.
3. Else if the cwd has `<repo>/.af/state.toml` symlink: follow it.
4. Else: error "no current workstream; specify --session NAME or run inside a workstream's tmux session."

ADR-038 specifies the symlink mechanics.

## Consequences

- The two-layer model (live state + event log) gives both a quick "what
  is the workstream right now?" and a "what happened?" trail.
- `flock` ensures multi-process safety for `af list` running while `af
  create` writes.
- Atomic `state.toml` writes prevent half-written files surviving
  crashes.
- Schema versioning gives a migration path without breaking old data.
- `~/.local/share/af/v1/` namespacing means a future v2 lives at
  `~/.local/share/af/v2/` without colliding.

## Alternatives Considered

- **Single JSON file per workstream.** Rejected; loses the append-only
  ledger and complicates future migrations.
- **SQLite.** Rejected; adds a runtime dep and a data-management burden
  out of scope for the workstream count we expect (tens, not thousands).
- **Per-event YAML files.** Rejected; verbose and harder to grep.
- **`go-flock`** as an explicit dep. Rejected; `golang.org/x/sys/unix`
  is the canonical home for `flock`.

## References

- v0 ADR-006 (Session Metadata) and ADR-011 (Workstream Lifecycle &
  Session Ledger) — superseded by this ADR for v1.
- ADR-031 — v1 master.
- ADR-036 — config schema (`~/.local/share/af/v1/` is implicit).
- ADR-038 — workstream + worktree layout (per-repo symlink).
- ADR-039 — multi-agent multi-session model (slot semantics).
- ADR-046 — suspend/resume lifecycle (uses `session_suspended` event).
