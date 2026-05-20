---
adr: 038
title: "Workstream + Worktree Layout (stable paths, sub-worktrees, per-repo discovery)"
status: proposed
implementation: in-progress
date: 2026-05-06
last_modified: 2026-05-20
supersedes: []
superseded_by: null
related: ["031", "037", "039", "045", "056"]
tags: ["go", "worktree", "workstream", "fs"]
---

# ADR-038: Workstream + Worktree Layout

## Context

A workstream needs a deterministic, predictable filesystem location so
that:

- The same workstream resumes at the same path across reboots.
- The owner can `cd` into a workstream's worktree and `af list` /
  `af note` work without needing the workstream name.
- Subagents can have **isolated git state** (their own worktree on
  their own branch) when they need to experiment without polluting the
  primary agent's branch.
- A workstream can be discovered from inside its worktree via a
  per-repo symlink that's globally git-ignored.

v0 had stable worktree paths but no sub-worktree mechanism (all agents
shared the same worktree+branch) and no per-repo discovery file.

## Decision

### Worktree path layout

```
<config.general.worktree_root>/<repo>/<branch>/         # primary
<config.general.worktree_root>/<repo>/<branch>--<slot>/ # sub-worktree per subagent
```

Default `worktree_root`: `~/Workspace/.worktrees`.

For example, repo `af`, branch `kakkoyun/issue-42`, primary slot
`primary` plus a `tests` slot:

```
~/Workspace/.worktrees/af/kakkoyun/issue-42/                       # primary
~/Workspace/.worktrees/af/kakkoyun/issue-42--tests/                # subagent
```

The branch in the sub-worktree is `kakkoyun/issue-42--tests`, forked
from `kakkoyun/issue-42` at the moment `af agent add --slot tests` is
called.

### Why sibling sub-worktrees on sibling branches

Three layouts considered:

| Layout                                              | Path                                                | Branch              | Pro                                                                                  | Con                                                                                |
| --------------------------------------------------- | --------------------------------------------------- | ------------------- | ------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------- |
| **A — sibling worktree, sibling branch** _(chosen)_ | `~/W/.worktrees/<repo>/<branch>--<slot>/`           | `<branch>--<slot>`  | Full git isolation per subagent. Subagents can rebase/merge back to primary cleanly. | More disk; one worktree per slot                                                   |
| B — nested under primary, shared branch             | `~/W/.worktrees/<repo>/<branch>/.subagents/<slot>/` | `<branch>` (shared) | Less disk                                                                            | git refuses two worktrees on the same branch unless detached; concurrency footguns |
| C — nested + detached HEAD                          | same as B                                           | (none)              | No git refusal                                                                       | Lose branch tracking, no merge-back                                                |

**Layout A wins** because subagents that produce real diffs need to be
able to merge back into the primary branch, which only works with
their own branch.

The merge-back mechanic is intentionally unimplemented in v1: subagents
push branches; the owner reviews and merges manually. Future ADRs may
formalise this.

### Sub-worktree lifecycle

- `af agent add --slot <slot> --agent <provider>` creates the sub-worktree if `<slot>` is not `primary`. The branch `<branch>--<slot>` is created from `<branch>`'s current HEAD.
- `af agent stop <slot>` does **not** remove the sub-worktree by default; the user can keep poking at the work. Pass `af agent stop <slot> --remove-worktree` to clean up.
- `af done` removes all sub-worktrees of the workstream and deletes their branches **only if** they are merged into `<branch>` or the user passes `--force`.

### Per-repo discovery symlink

```
<repo-root>/.af/state.toml -> ~/.local/share/af/v1/sessions/<session>/state.toml
<repo-root>/.af/                                                                        (directory)
```

- Created by `af create`.
- The `.af/` directory may also hold a per-repo `config.toml` (per ADR-036).
- The directory is **gitignored globally** by `af setup` adding `.af/` to `~/.config/git/ignore` (see ADR-045).
- Inside a worktree, `af list`, `af note`, `af resume`, `af suspend` resolve "the current workstream" via this symlink before falling back to tmux-based discovery (per ADR-037 §"File-discovery rules").

### Sub-worktree symlink

Each sub-worktree gets its own `.af/state.toml` symlink pointing at the
**same** canonical state file as the primary. (The state file knows
about all slots.) `cd` into a sub-worktree gives `af` enough context to
resolve back to the workstream.

### Naming

#### Session name (== tmux session name)

- User-supplied via `af create <name>`, or auto-generated as `<repo>-<YYYYMMDD-HHMMSS>`.
- Sanitized: `/`, `.`, `:` → `--`. tmux dislikes those characters in session names.
- Example: `kakkoyun/issue-42` → `kakkoyun--issue-42`.

#### Branch name

- Same as session name **before** sanitization for tmux.
- Optional config-driven prefix: `<config.branch.prefix>/<name>` when:
  - `config.branch.prefix` is non-empty.
  - The repo has an `upstream` remote (and `config.branch.prefix_on_fork_only` is true, the default).
  - The user-supplied name doesn't already start with the prefix.

#### Sub-branch name

- `<primary-branch>--<slot>`. Same `--` separator as the session-name sanitization rule, but it's a literal in this context (not a sanitization).

### Worktree creation rules

- Default: fork from `upstream/<main>` if upstream remote exists, else `origin/<main>`. Main detection tries `main`, `master`, `trunk` in that order.
- `--from <branch>`: fork from the named local or remote branch.
- `--current`: fork from the current branch in the cwd's repo.
- `--from-pr <N>`: resolve PR head/base via `gh pr view --json` and fork from the head.

### Cleanup

- `af done` performs `git worktree remove --force` on the workstream's primary worktree, recursively for sub-worktrees.
- `af clean` (ADR-056) batch-reaps workstreams whose branch is merged or closed. It uses the three-strategy merge detection from v0 ADR-011 §3.5 (PR state → ancestry → squash fingerprint) and is the dedicated ADR for that command.

## Consequences

- Worktree paths are predictable; muscle memory works.
- Subagents have full git isolation when they need it.
- The per-repo symlink eliminates the "I forgot the workstream name" problem.
- Disk usage scales with sub-worktree count, but worktrees are cheap (shared object database).

## Alternatives Considered

- **Layout B (nested, shared branch).** Rejected; git refuses dual worktrees on the same branch.
- **Layout C (nested, detached HEAD).** Rejected; loses branch tracking and merge-back.
- **No sub-worktrees at all.** Rejected; subagents would clobber the primary branch.
- **Per-repo `.af/` versioned in git.** Rejected; the symlink and per-repo config are user-environment, not project artefacts.

## References

- v0 ADR-006 (session metadata, worktree paths) — partially superseded for v1.
- ADR-031 — v1 master.
- ADR-037 — session metadata schema (sub_worktree, sub_branch fields).
- ADR-039 — multi-agent multi-session (slot ↔ sub-worktree mapping).
- ADR-045 — `af setup` writes the global gitignore entry for `.af/`.
