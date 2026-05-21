---
adr: 072
title: "state.toml Schema Amendments Roll-up"
status: proposed
implementation: pending
date: 2026-05-21
last_modified: 2026-05-21
supersedes: []
superseded_by: null
related: ["037", "059", "061", "062", "065", "067", "071"]
tags: ["go", "state", "schema", "rollup"]
---

# ADR-072: state.toml Schema Amendments Roll-up

## Context

ADR-037 defined `state.toml` for v1 (`schema_version = 1`). Six
subsequent ADRs amended the schema by adding new top-level blocks or
new keys, but none of them re-emitted the canonical schema dump.
Anyone reading ADR-037 alone sees a stale schema:

| ADR  | Amendment                                                      |
| ---- | -------------------------------------------------------------- |
| 059  | `[stack]` block (`parent_session`, `parent_branch`, `linked_at`). Folded into SPEC already. |
| 061  | What of repo `[control]` settings is captured at `af create` time. |
| 062  | Effective Slicer resource profile (`profile_name`, `group`, `vcpu`, …). |
| 065  | Worktree-lease state (`holder_vm`, `last_push_at`, `last_pull_at`). |
| 067  | Per-agent session-sync cursors (`agent`, `source_root`, `last_synced_at`, `last_hash`, `last_offset`). |
| 071  | `[pr].last_refreshed_at`, `[pr].last_refresh_error` (PR cache). |

This ADR consolidates the schema as of today, names the new fields
unambiguously, and amends ADR-037 by reference. It does **not**
supersede ADR-037 — ADR-037 still owns the foundational decision and
the `[session]`/`[worktree]`/`[execution]`/`[[agents]]`/`[pr]`/`[stack]`/`[versions]`
blocks. ADR-072 just adds the deltas.

`schema_version` stays at `1`. Additions are non-breaking; ADR-037
§"Schema migrations" already permits this.

## Decision

### Canonical state.toml as of ADR-072

The full schema, with new blocks marked `# ADR-NNN`:

```toml
schema_version = 1

[session]
name         = "kakkoyun--issue-42"
id           = "<uuid v5>"
created_at   = 2026-05-06T12:00:00Z
status       = "active"          # active | suspended | completed | abandoned
# suspended_at omitted until status = "suspended"

[worktree]
path         = "/Users/kemal/Workspace/.worktrees/af/kakkoyun--issue-42"
branch       = "kakkoyun/issue-42"
base_branch  = "upstream/main"
git_root     = "/Users/kemal/Workspace/Projects/Personal/af"
repo_slug    = "kakkoyun/af"

[execution]
mode             = "local"       # local | bare | remote | sandbox
multiplexer      = "tmux"
tmux_session     = "kakkoyun--issue-42"
ssh_host         = ""
remote_path      = ""
sandbox_provider = ""            # "" | "slicer"  (ADR-060; "sbx" rejected on load)
sandbox_id       = ""

[[agents]]
slot            = "primary"
provider        = "pi"
session_ids     = ["<uuid v5>"]
pane            = "%0"
status          = "running"
sub_worktree    = ""
sub_branch      = ""
created_at      = 2026-05-06T12:00:00Z

[pr]
number              = 0
url                 = ""
state               = ""
last_refreshed_at   = ""         # ADR-071 — ISO 8601; "" = never refreshed
last_refresh_error  = ""         # ADR-071 — truncated 120-char error; "" on success

[stack]                          # ADR-059
parent_session = ""
parent_branch  = ""

[control]                        # ADR-061 / ADR-063
provider        = ""             # captured from repo [control].remote_control; "" if disabled
port            = 0              # set by `af control up`; 0 when down
bound_at        = ""             # ISO; "" until first up

[sandbox.slicer.resources]       # ADR-062 — present only when execution.sandbox_provider == "slicer"
profile_name    = ""             # "" = default-group launch; else managed-group profile
group           = ""             # resolved group name ("af-<slug>-<profile>" or explicit)
vcpu            = 0              # 0 = group default
ram_gb          = 0
storage_size    = ""             # e.g. "25G"; "" = group default
gpu_count       = 0
image           = ""
hypervisor      = ""

[sandbox.slicer.lease]           # ADR-065 — present only when execution.sandbox_provider == "slicer"
holder_vm       = ""             # "" if not currently leased to a VM
last_push_at    = ""             # ISO; populated by `slicer wt push`
last_pull_at    = ""             # ISO; populated by `slicer wt pull`

[[session_sync]]                 # ADR-067 — one entry per agent/harness ever synced
agent           = "claude"       # claude | codex | pi | harness
source_root     = "/home/agent/.claude/sessions"   # path inside the VM
last_synced_at  = ""
last_hash       = ""             # sha256 of last imported tail, hex
last_offset     = 0              # byte offset for resumable JSONL appends

[versions]
af             = "1.0.0"
agent_versions = { pi = "...", claude = "..." }
```

### Optionality and omission rules

To keep the file small for the common case (local workstreams, no
PR yet, no sandbox), the **new blocks are omitted entirely when
unused**:

| Block                          | Present when                                                         |
| ------------------------------ | -------------------------------------------------------------------- |
| `[stack]`                      | `parent_session != ""`. Else omitted.                                |
| `[control]`                    | The repo's `[control].remote_control` is non-empty at create time. Else omitted. |
| `[sandbox.slicer.resources]`   | `execution.sandbox_provider == "slicer"` **and** any resource field is non-zero. Else omitted. |
| `[sandbox.slicer.lease]`       | `execution.sandbox_provider == "slicer"` **and** a `slicer wt push` has occurred. Else omitted. |
| `[[session_sync]]`             | At least one successful `af session-data sync` has imported data for that agent. |
| `[pr].last_refreshed_at`       | First refresh attempt has occurred (success or failure). Else key omitted from `[pr]`. |
| `[pr].last_refresh_error`      | Last refresh failed. Cleared (key omitted) on next successful refresh. |

Writers `omitempty` all of the above via Go struct tags.

### Validation rules at load time

- `schema_version == 1` required; higher → `ErrSchemaTooNew` (per
  ADR-037).
- `execution.sandbox_provider` ∈ {`""`, `"slicer"`}. Loading a file
  with `"sbx"` returns the ADR-060 migration error.
- `pr.state` ∈ {`""`, `"open"`, `"draft"`, `"closed"`, `"merged"`}.
- `session.status` ∈ {`"active"`, `"suspended"`, `"completed"`,
  `"abandoned"`}.
- Each `[[agents]].status` ∈ {`"running"`, `"stopped"`, `"crashed"`,
  `"suspended"`}.
- `[[session_sync]].agent` ∈ {`"claude"`, `"codex"`, `"pi"`,
  `"harness"`}. Other values are rejected with `EX_DATAERR`.
- Numeric fields (`vcpu`, `ram_gb`, `gpu_count`, `port`,
  `last_offset`) must be non-negative.
- ISO 8601 timestamps in `last_refreshed_at`, `last_push_at`,
  `last_pull_at`, `last_synced_at`, `bound_at` must parse via Go's
  `time.RFC3339`; empty string treated as "not set" (no error).

Invalid values fail load with `EX_DATAERR` (65) per ADR-068 §2.

### Forward link from ADR-037

ADR-037 gets a one-line amendment in its `## Decision > state.toml
schema` section:

> **Schema delta:** ADRs 059, 061, 062, 065, 067, 071, and 072 add
> blocks/fields to this schema. ADR-072 holds the canonical
> consolidated dump.

This is the only update ADR-037 receives; per ADR-032's
"Updates after `accepted`" rule, material changes go through new
ADRs (which this is).

## Consequences

- One place to look for the canonical state shape.
- `schema_version` stays at `1` — no migrations needed across v1.
- New fields are clearly attributed to their owning ADR via
  comments, so a reader can trace the *why* per field.
- Future schema additions follow the same pattern: a new ADR adds
  the field with a comment naming the ADR number; ADR-072 is
  amended in place (per ADR-032's typo/clarification rule) to
  include the field in the canonical dump.
- The `omitempty` discipline keeps simple local workstreams' state
  files short and reviewable.

## Alternatives Considered

- **Edit ADR-037 in place to embed every new field.** Rejected per
  ADR-032 §"Updates after `accepted`": material changes get a new
  ADR. ADR-037 is the foundational decision; later schema deltas
  are amendments, not substitutions.
- **Supersede ADR-037 entirely with this ADR.** Rejected: the
  foundational schema decision in ADR-037 (TOML + ledger.jsonl,
  per-session directory, archive layout, atomic writes,
  schema_version field) still stands. No reason to retire it.
- **Bump `schema_version` to 2 to mark the additions.** Rejected:
  additions are non-breaking; bumping the schema would force every
  binary to run a migration for no observable benefit.
- **Move the new blocks out of `state.toml` into separate files
  (`control.toml`, `sandbox.toml`, etc.).** Rejected: spreads
  related state across many files, complicates atomic writes, and
  loses the single-source-of-truth property `state.toml` was
  introduced for.

## References

- ADR-032 — ADR conventions; how this ADR sits relative to ADR-037.
- ADR-037 — foundational state.toml schema; this ADR's parent.
- ADR-046 — lifecycle states inform `session.status`.
- ADR-059 — `[stack]` block (already in ADR-037-era SPEC).
- ADR-060 — slicer-only sandbox; `"sbx"` invalid in
  `execution.sandbox_provider`.
- ADR-061 — repo-scoped `[control]` settings; `[control]` state
  capture.
- ADR-062 — `[sandbox.slicer.resources]`.
- ADR-065 — `[sandbox.slicer.lease]`.
- ADR-067 — `[[session_sync]]`.
- ADR-068 §4 — locking around state.toml writes.
- ADR-071 — `[pr].last_refreshed_at`, `[pr].last_refresh_error`.
