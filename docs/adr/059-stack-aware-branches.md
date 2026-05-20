---
adr: 059
title: "Stack-Aware Branch Model (af stack, af unstack, af sync)"
status: proposed
implementation: in-progress
date: 2026-05-08
last_modified: 2026-05-21
supersedes: []
superseded_by: null
related: ["031", "035", "037", "038", "046", "048", "054", "056"]
tags: ["go", "stack", "branch", "rebase", "lifecycle"]
---

# ADR-059: Stack-Aware Branch Model

## Context

af's current model (ADR-038) treats workstreams as siblings: each has a `base_branch`
(typically `upstream/main`), and there is no notion of a workstream depending on another
workstream's branch. In practice the owner often stacks features — `feat-b` is built on
`feat-a` while `feat-a`'s PR is in review. Without stack awareness, `feat-b`'s
`base_branch` is `feat-a`, and when `feat-a` merges to `main`, `feat-b` must be manually
rebased onto `main` and have its PR target updated.

Datadog's `gv stack`/`gv unstack`/`gv sync` solves this with a parent-branch model and
auto-reparenting on merge. `gv` leans on Graphite (`gt`); af will do it natively with
`git rebase` since adding a Graphite dep is out of scope (ADR-031).

## Decision

### Concept

A workstream may have a **parent workstream**. If set, the workstream's branch is
considered to stack on top of the parent's branch. On `af sync`, if the parent's PR has
merged, the child auto-reparents onto the parent's `base_branch` (i.e., the
grandparent or trunk) and rebases.

### Schema delta to ADR-037

A new `[stack]` section in `state.toml`:

```toml
[stack]
parent_session = ""        # workstream name of the parent; "" = no parent (default)
parent_branch  = ""        # resolved branch name of the parent at link time
linked_at      = null      # timestamp the parent was set
```

`base_branch` (existing in ADR-037 `[worktree]`) remains the trunk anchor. When
`parent_session` is non-empty, `base_branch` records the **trunk** the stack ultimately
targets (used when the entire stack is unstacked or the PR is opened against trunk).

Implementation note: ADR-037 will be amended in the same commit batch to include this
section. ADR-037 is still `proposed`, so amending it is in-scope per the freeze policy.

### Commands

#### `af stack [session] --parent PARENT`

Sets `parent_session = PARENT` and `parent_branch = <PARENT's branch>`. Validates:

- PARENT exists and is `active` or `suspended`.
- PARENT is in the same repo (same `git_root`).
- No cycles (walk parent chain, refuse if `session` appears in it).

Writes a `stack_linked` ledger event with `parent_session`, `parent_branch`.

`af stack` without `--parent` prints the current parent (and its parent, transitively).

#### `af unstack [session]`

Clears `[stack]`, falling back to `base_branch` for all subsequent ops. Writes
`stack_unlinked` ledger event.

#### `af sync [session]`

The auto-reparenting workhorse. Steps:

1. `git fetch <upstream-or-origin>`.
2. If `[stack].parent_session` is set:
   a. Read the parent's `state.toml`. Run merge detection on the parent
   using the same three-strategy chain as ADR-056 §"Merge detection
   (reusable)" (PR state → ancestry → squash fingerprint). If detected
   merged, **auto-reparent**: set this workstream's `parent_session` to
   the grandparent (or `""` if no grandparent), set `parent_branch` to
   the grandparent's branch (or this workstream's `base_branch`), write
   a `stack_reparented` ledger event.
   b. Repeat (a) recursively in case the grandparent also merged.
3. Determine target: parent's branch if still stacked, else `base_branch`.
4. Run `git rebase <target>` in the worktree. On conflict: leave the rebase in progress, emit a clear hint (`resolve, then 'git rebase --continue', then 'af sync' to re-attempt`), and exit non-zero.
5. If rebase succeeds, write a `synced` ledger event.

### Interaction with existing ADRs

- **ADR-046** (suspend/resume): suspended workstreams are still reparented on `af sync`
  if their parent merges. Resume picks up the new parent automatically.
- **ADR-048** (`af pr`): when opening a PR for a stacked workstream, `--base` defaults
  to `parent_branch`, not `base_branch`.
- **ADR-054** (`af status`): when a workstream is stacked, the `STATE` column gains a
  `→<parent>` suffix (e.g. `active→feat-a`).
- **ADR-037**: schema gains `[stack]` (see delta above).

### Out of scope for v1

- **Stack visualization** (a `tree`-style dump): defer; `af status` showing
  `→<parent>` and `af info` listing the full chain is enough.
- **Cross-repo stacks**: rejected; same-repo only.
- **Multi-parent stacks** (DAG): rejected; one-parent-per-workstream only.
- **Auto-rebase on conflict**: rejected; conflicts always require human resolution.
- **Graphite (`gt`) integration**: rejected per ADR-031 (no new deps).

## Consequences

- Stacked workflows survive trunk merges without manual rebase choreography.
- The parent-branch model is opt-in; non-stacked workstreams behave exactly as before.
- ADR-037 grows by one section; the schema bump remains `1` because `[stack]` is a new
  optional section (zero-value safe).
- Conflict resolution stays human; af never resolves merges.

## Alternatives Considered

- **Adopt Graphite (`gt`) as a runtime dep.** Rejected per ADR-031; the dep set is locked
  to 5 packages.
- **Multi-parent DAG stacks.** Rejected; complexity with no concrete need from owner.
- **Auto-merge on `af done` for the bottom of a stack.** Rejected; out of scope.
- **Track stack via git refs (`refs/stacks/...`) instead of state.toml.** Rejected; state
  belongs in `state.toml` for consistency with the rest of the schema.
- **Implement only `af sync` (no `stack`/`unstack`).** Rejected; without explicit
  linking, sync has no policy input.

## References

- ADR-031 — v1 master, dep cap.
- ADR-037 — state schema (extended by this ADR).
- ADR-038 — branch naming, worktree paths.
- ADR-046 — lifecycle (suspend/resume coexists with stacks).
- ADR-048 — `af pr --base` default changes for stacked workstreams.
- ADR-054 — `af status` displays stack relationship.
- ADR-056 §"Merge detection (reusable)" — three-strategy chain reused by `af sync` step 2.a.
- v0 ADRs 011, 014 (composition model) — informational; v0 had no stack concept.
