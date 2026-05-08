---
adr: 056
title: "af clean — Reap Completed Workstreams"
status: proposed
implementation: pending
date: 2026-05-08
last_modified: 2026-05-08
supersedes: []
superseded_by: null
related: ["031", "035", "037", "038", "046", "047", "048", "059"]
tags: ["go", "command", "lifecycle", "cleanup"]
---

# ADR-056: `af clean` — Reap Completed Workstreams

## Context

`af done` (per ADR-046) ends one workstream. After a busy week the owner accumulates
multiple workstreams whose PRs are merged or closed but whose worktrees, sub-worktrees,
and `state.toml` files still consume disk and clutter `af status`.

Datadog's `gv clean` solves this: iterate worktrees, query PR state, archive everything
where the PR merged or closed. af needs the same.

### Supersession of `af gc` mention in ADR-038

ADR-038 §"Cleanup" mentions `af gc` informally as the command that "cleans workstreams
whose branch is merged or closed (per merge-detection rules carried over from v0 ADR-011
§3.5; v1 keeps the same three-strategy approach: PR state → ancestry → squash
fingerprint)." There is no dedicated ADR for `af gc`.

This ADR **formalises that command under the name `af clean`** for two reasons:

1. **Vocabulary.** `gc` connotes background garbage collection (cf. `runtime.GC`). The
   command is a deliberate, user-invoked sweep — `clean` reads more naturally and
   matches grove's `gv clean`.
2. **Ownership.** A one-liner in another ADR's prose is too thin a specification for a
   command that performs destructive operations. ADR-056 is the dedicated ADR.

ADR-038's Cleanup section is edited in the same commit batch to point at this ADR. The
**three-strategy merge detection** from v0 ADR-011 §3.5 is preserved verbatim — there is
no detection regression.

## Decision

### Command

```
af clean [--dry-run] [--include-abandoned] [--max-age DURATION] [--force [<name>...]]
```

| Flag                  | Behaviour                                                                                                                                                                                                                       |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| (default)             | Reap workstreams detected as merged or closed by the three-strategy chain.                                                                                                                                                      |
| `--dry-run`           | List what would be reaped; perform no destructive ops                                                                                                                                                                           |
| `--include-abandoned` | Also reap workstreams whose status is already `abandoned`                                                                                                                                                                       |
| `--max-age DURATION`  | Only reap workstreams older than DURATION. See §"Duration grammar" below: `7d`, `2w`, `120h`, `5h30m` are all valid.                                                                                                            |
| `--force <name>...`   | Skip merge detection; reap each named workstream directly. Must be paired with at least one workstream name — bulk-reap still requires merge detection. Useful for cleaning up failed experiments where no merge ever happened. |

### Reap algorithm — three-strategy merge detection

For each `~/.local/share/af/v1/sessions/*/state.toml`:

1. Read state (shared flock).
2. **Determine merge status** by trying three strategies in order. The first strategy
   that returns a definitive answer wins; later strategies are skipped.

   | Order | Strategy               | Inputs                                                                                 | Verdict semantics                                                                                                                                                           |
   | ----- | ---------------------- | -------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
   | 1     | **PR state**           | `[pr].number` if non-zero; live `gh pr view <num> --json state` (5s timeout, 1h cache) | `merged` or `closed` → reapable. `open`/`draft` → not reapable. Network failure → fall through to (2).                                                                      |
   | 2     | **Ancestry**           | `[worktree].branch`, `[worktree].base_branch`                                          | `git merge-base --is-ancestor <branch> <base_branch>` exit 0 → reapable. Catches merge-commit and fast-forward merges. Misses squash merges.                                |
   | 3     | **Squash fingerprint** | branch diff vs base; recent commits on `base_branch`                                   | Compute `git diff <base>...<branch>` patch-id (`git patch-id`). Walk last 200 commits on `base_branch`; if any commit's patch-id matches → reapable. Catches squash merges. |

   If all three strategies say "not merged" or "unknown", the workstream is **not**
   reaped this run. Strategy verdicts are recorded as `merge_detection: <strategy>`
   on the reap event in the ledger.

#### Merge detection (reusable)

The three-strategy chain above is a **reusable contract** consumed by both
`af clean` (to decide what to reap) and `af sync` (ADR-059, to detect if
a parent workstream has merged):

- **Inputs:** a `state.toml` read (for `[pr].number`, `[worktree].branch`,
  `[worktree].base_branch`), a live `gh` call (strategy 1), and local `git`
  invocations (strategies 2 and 3).
- **Output:** `merged | open | unknown`.
- **Strategy chain:** strategy 1 (PR state) → strategy 2 (ancestry) →
  strategy 3 (squash fingerprint). First definitive answer wins.
- Network and git errors in strategy 1 fall through to strategy 2;
  a still-uncertain result after strategy 2 falls through to strategy 3.
  If all three are inconclusive, output is `unknown`.

3. **`--max-age` gate.** If `--max-age DURATION` is set and the workstream's
   `last_touched_at` is more recent than `now - DURATION` → skip. DURATION is
   parsed per the grammar in §"Duration grammar" — not `time.ParseDuration`,
   which doesn't accept day or week suffixes.

### Duration grammar

Used by `--max-age` here and `af retro --since` (ADR-058). Defined in
`internal/duration/duration.go` as a small wrapper around stdlib
`time.ParseDuration`:

- The string is split on a regexp matching `(\d+)(d|w)` at any position.
- Each `Nd` token is converted to `(N * 24)h` and `Nw` to `(N * 168)h`
  before the string is handed to `time.ParseDuration`.
- Mixed forms work: `1w3d` → `240h`; `5h30m` is passed through unchanged;
  `2w12h` → `348h`.
- Months and years are deliberately **not** supported: their length
  varies; if the user wants "older than ~30 days" they write `30d`.
- Calendar-day rounding: `Nd` is exactly `N * 24h` from the moment of
  invocation, not aligned to local midnight. The user wanting calendar
  days runs the command at a fixed time of day.

Valid grammar in EBNF:

```
duration = ( pair )+
pair     = number unit
number   = [0-9]+
unit     = "d" | "w" | "h" | "m" | "s" | "ms" | "us" | "ns"
```

Invalid input (`30 days`, `1 month`, negative numbers, empty string)
fails parsing with a clear error message; the command exits non-zero
before any reaping decision.

4. **Reap action.** Equivalent to `af done` per ADR-046:
   - Write `session_completed` ledger event with `reaped_by: "af clean"`,
     `merge_detection: <strategy>`.
   - Move session dir to `~/.local/share/af/v1/archive/<name>/`.
   - Remove worktree + all sub-worktrees per ADR-038 §"Cleanup".
   - Update Obsidian frontmatter `af_status: completed`, `af_completed_at: <ts>`
     (per ADR-047).

### `--include-abandoned` semantics

When set, also reap workstreams whose `[session].status == "abandoned"`. By default
abandoned workstreams stay put — `af done --force` already moved them out of the
`active`/`suspended` set, and the user may want to keep their notes around as a record.

### Why three strategies, not just PR state

PR-state alone misses two real cases:

1. **Squash-merge repos** (GitHub default for many teams). The merged commit on `main`
   has a fresh SHA; ancestry returns false. Without strategy 3, every squash-merged
   workstream stays in `active` forever.
2. **Workstreams without a tracked PR** (e.g., the owner pushed and merged via web UI
   without ever running `af pr`). `[pr].number == 0`, so strategy 1 skips. Strategy 2
   or 3 still finds them.

This three-strategy approach is the v0 ADR-011 §3.5 carry-over that ADR-038 already
promises.

### Output

```
Reaped (3):
  kakkoyun--issue-42        merged via pr-state          PR #142    (3 days ago)
  kakkoyun--fix-typo        merged via squash-fingerprint           (1 day ago)
  kakkoyun--refactor-mux    merged via ancestry          PR #138    (5 hours ago)

Skipped (1):
  kakkoyun--feat-stack      PR #150 open                            (use af done to end manually)
```

The detection-strategy column makes the basis for each reap auditable. `--dry-run`
replaces `Reaped` with `Would reap` and performs no destructive ops.

### Concurrency

Reaping is sequential per workstream (each holds its own exclusive flock). PR state
fetches across workstreams use the same goroutine pool as `af status` (cap 8). Failed
fetches downgrade to "PR state unknown — skipping".

### Safety

- Reaping requires a positive merge verdict from at least one of the three
  detection strategies. Workstreams that no strategy can verify as merged
  are skipped (the user can still run `af done <name>` manually).
- `--force <name>` is the only path that bypasses merge detection. It
  refuses to bulk-reap (must be paired with one or more workstream names).
- `--dry-run` is the recommended first invocation for any user new to the
  command.

## Consequences

- Disk and `af status` stay tidy without manual `af done` per workstream.
- Workstream archival continues to flow through ADR-046's existing `af done` mechanics —
  no new lifecycle paths or ledger events (only an additional `merge_detection` field
  on the existing `session_completed` event).
- `gh pr view` rate limits are respected via the shared 8-goroutine cap.
- Three-strategy detection handles squash-merge repos and PR-less branches without
  regression vs. v0.
- The informal `af gc` mention in ADR-038 is replaced by this ADR. ADR-038 is edited
  to point here in the same commit batch.

## Alternatives Considered

- **Auto-reap on every `af done`.** Rejected; surprising side-effects, single-purpose
  commands compose better.
- **Cron-driven background sweep.** Rejected; out of scope for v1 (single-user, no daemon).
- **Keep the name `af gc` per ADR-038's mention.** Rejected; `gc` connotes background
  garbage collection (`runtime.GC`), which is the wrong mental model for a deliberate
  user-invoked sweep. ADR-038 is edited to point here.
- **PR-state-only detection.** Rejected; misses squash-merged workstreams (GitHub
  default for many teams) and PR-less branches. Three-strategy detection per v0
  ADR-011 §3.5 is preserved.
- **Fourth strategy: search base-branch commits by message for the branch name.**
  Rejected; brittle (relies on PR-title-in-merge-message convention), low marginal
  recall over patch-id matching.

## References

- v0 ADR-011 §3.5 — three-strategy merge detection (preserved).
- ADR-031 — v1 master.
- ADR-037 — state schema, lifecycle status field.
- ADR-038 — worktree + sub-worktree removal mechanics; this ADR replaces the `af gc` mention there.
- ADR-046 — `af done` (the underlying op `af clean` performs in batch).
- ADR-047 — Obsidian frontmatter updates on completion.
- ADR-048 — `af pr` populates the `[pr].state` field this command reads.
- ADR-059 — `af sync` reuses the merge-detection contract defined in §"Merge detection (reusable)".
- `git patch-id(1)` — strategy 3 mechanic.
