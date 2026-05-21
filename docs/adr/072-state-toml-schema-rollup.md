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
| 061  | What of repo `[control]` settings is captured at `af create` time (as implemented: `execution.remote_control`). |
| 062  | Effective Slicer resource profile (as implemented: flat `execution.sandbox_resource_*` fields + `execution.sandbox_managed_group`). |
| 065  | Worktree-lease state (as implemented: top-level `[slicer_wt]` block: `vm`, `path`, `pushed_at`, `pulled_at`, `lease_state`). |
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

This schema reflects the **as-implemented** shape after Stage 11.
ADRs 061 and 062 chose to flatten their state capture into the
existing `[execution]` block (one field per concept, prefixed with
`sandbox_resource_` or `remote_control`) instead of nested blocks.
ADR-065 added a top-level `[slicer_wt]` block. ADRs 067 (session
sync) and 071 (PR cache) are still `implementation: pending`; their
fields are marked **PROPOSED** below and will land on first
implementation.

```toml
schema_version = 1

[session]
name           = "kakkoyun--issue-42"
id             = "<uuid v5>"
created_at     = 2026-05-06T12:00:00Z
status         = "active"          # active | suspended | completed | abandoned
approval_mode  = ""                # optional; agent-provider approval mode override
max_agents     = 0                 # optional override; 0 = config default
# suspended_at omitted until status = "suspended"

[worktree]
path         = "/Users/kemal/Workspace/.worktrees/af/kakkoyun--issue-42"
branch       = "kakkoyun/issue-42"
base_branch  = "upstream/main"
git_root     = "/Users/kemal/Workspace/Projects/Personal/af"
repo_slug    = "kakkoyun/af"

[execution]
mode             = "local"         # local | bare | remote | sandbox
multiplexer      = "tmux"
tmux_session     = "kakkoyun--issue-42"
ssh_host         = ""              # populated for remote mode
remote_path      = ""
sandbox_provider = ""              # "" | "slicer"  (ADR-060; "sbx" rejected on load)
sandbox_id       = ""
# ADR-061 capture (omitempty):
remote_control               = ""  # "" | "superterm"; from repo [control].remote_control
# ADR-062 captured Slicer resource profile (all omitempty):
sandbox_resource_profile     = ""
sandbox_resource_vcpu        = 0
sandbox_resource_ram_gb      = 0
sandbox_resource_storage_size = ""
sandbox_resource_gpu_count   = 0
sandbox_resource_image       = ""
sandbox_resource_hypervisor  = ""
sandbox_managed_group        = ""  # resolved "af-<slug>-<profile>" or explicit group

[[agents]]
slot            = "primary"
provider        = "pi"
session_ids     = ["<uuid v5>"]
pane            = "%0"
status          = "running"        # running | stopped | crashed | suspended
sub_worktree    = ""
sub_branch      = ""
created_at      = 2026-05-06T12:00:00Z
# last_resumed_at omitted until first resume

[pr]
number              = 0
url                 = ""
state               = ""           # "" | open | draft | closed | merged   (ADR-071)
# PROPOSED (ADR-071, implementation pending):
last_refreshed_at   = ""           # ISO; "" = never refreshed
last_refresh_error  = ""           # truncated 120-char error; cleared on success

[stack]                            # ADR-059 — present even when empty
parent_session = ""
parent_branch  = ""
# linked_at omitted until parent is set

[slicer_wt]                        # ADR-065 — omitted entirely when vm == ""
vm           = ""                  # holder VM name; "" when not leased
path         = ""                  # worktree path leased to the VM
pushed_at    = 2026-05-21T15:00:00Z
pulled_at    = ""                  # ISO; set after `slicer wt pull`
lease_state  = ""                  # held_by_vm | pulled | discarded

[[session_sync]]                   # PROPOSED (ADR-067, implementation pending)
agent           = "claude"         # claude | codex | pi | harness
source_root     = "/home/agent/.claude/sessions"
last_synced_at  = ""
last_hash       = ""               # sha256 of last imported tail, hex
last_offset     = 0                # byte offset for resumable JSONL appends

[versions]
af             = "1.0.0"
agent_versions = { pi = "...", claude = "..." }
```

### Block-naming rationale

The choice between flat fields-in-`[execution]` (ADR-061/062 capture)
and a dedicated top-level block (ADR-065 `[slicer_wt]`, the
PROPOSED ADR-067 `[[session_sync]]`) is intentional:

- **Flat in `[execution]`** when the state is purely a *snapshot* of
  per-workstream provisioning that never grows in cardinality
  (one VM has one resource profile, one workstream has one remote
  control provider).
- **Top-level block** when the state has its own lifecycle and may
  acquire fields independently of execution metadata
  (`[slicer_wt]` tracks a lease that grows `pulled_at` later;
  `[[session_sync]]` is an array — one entry per agent harvested).

Future additions follow this convention.

### Optionality and omission rules

| Block / field                              | Present when                                                         |
| ------------------------------------------ | -------------------------------------------------------------------- |
| `[stack]`                                  | Always present, but `parent_session == ""` means unstacked.          |
| `execution.remote_control`                 | The repo's `[control].remote_control` was non-empty at create time. Else omitted via `omitempty`. |
| `execution.sandbox_resource_*`             | `execution.sandbox_provider == "slicer"` **and** the corresponding resource field is non-default. Else omitted. |
| `execution.sandbox_managed_group`          | `execution.sandbox_provider == "slicer"` and a managed group was resolved. Else omitted. |
| `[slicer_wt]`                              | `execution.sandbox_provider == "slicer"` **and** a `slicer wt push` has occurred (i.e. `vm != ""`). Else omitted. |
| `[[session_sync]]` (PROPOSED)              | At least one successful `af session-data sync` has imported data for that agent (per ADR-067 once it lands). |
| `[pr].last_refreshed_at` (PROPOSED)        | First refresh attempt has occurred (per ADR-071 once it lands).      |
| `[pr].last_refresh_error` (PROPOSED)       | Last refresh failed. Cleared on next successful refresh.             |

Writers use Go `omitempty` tags for all of the above.

### Validation rules at load time

- `schema_version == 1` required; higher → `ErrSchemaTooNew` (per
  ADR-037).
- `execution.sandbox_provider` ∈ {`""`, `"slicer"`}. Loading a file
  with `"sbx"` returns the ADR-060 migration error.
- `execution.remote_control` ∈ {`""`, `"superterm"`}.
- `slicer_wt.lease_state` ∈ {`""`, `"held_by_vm"`, `"pulled"`,
  `"discarded"`}.
- `pr.state` ∈ {`""`, `"open"`, `"draft"`, `"closed"`, `"merged"`}.
- `session.status` ∈ {`"active"`, `"suspended"`, `"completed"`,
  `"abandoned"`}.
- Each `[[agents]].status` ∈ {`"running"`, `"stopped"`, `"crashed"`,
  `"suspended"`}.
- `[[session_sync]].agent` (PROPOSED) ∈ {`"claude"`, `"codex"`,
  `"pi"`, `"harness"`}.
- Numeric fields (`sandbox_resource_vcpu`, `sandbox_resource_ram_gb`,
  `sandbox_resource_gpu_count`, `max_agents`, `last_offset`,
  `pr.number`) must be non-negative.
- ISO 8601 timestamps (`pushed_at`, `pulled_at`, `last_refreshed_at`,
  `last_synced_at`) must parse via Go's `time.RFC3339`; empty string
  treated as "not set".

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
- ADR-062 — flat `execution.sandbox_resource_*` fields + `execution.sandbox_managed_group`.
- ADR-065 — top-level `[slicer_wt]` block (`vm`, `path`, `pushed_at`, `pulled_at`, `lease_state`).
- ADR-067 — PROPOSED `[[session_sync]]` (implementation pending).
- ADR-068 §4 — locking around state.toml writes.
- ADR-071 — `[pr].last_refreshed_at`, `[pr].last_refresh_error`.
