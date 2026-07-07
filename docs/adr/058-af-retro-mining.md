---
adr: 058
title: "af retro — Mine Archived Workstream Notes"
status: accepted
implementation: complete
date: 2026-05-08
last_modified: 2026-07-03
supersedes: []
superseded_by: null
related: ["031", "035", "037", "047", "057"]
tags: ["go", "command", "obsidian", "retrospective"]
---

# ADR-058: `af retro` — Mine Archived Workstream Notes

## Context

Every workstream produces an Obsidian note (per ADR-047). On `af done`/`af clean` those
notes survive in the vault — the workstream's archive moves to `~/.local/share/af/v1/archive/`
but the markdown note keeps living at `<vault>/<folder>/<session>.md` with frontmatter
`af_status: completed | abandoned`.

Datadog's `gv retro` mines its archive folder for "reusable patterns and institutional
context." For a single-user tool the same idea is even simpler: filter completed notes
by tag/PR/age, optionally summarise via an agent, and surface the result.

The Bases aggregator (ADR-047) already gives a queryable table of _active_ and
_suspended_ workstreams in Obsidian. `af retro` is the **completed/abandoned** counterpart
and is **terminal-side**, not vault-side.

## Decision

### Command

```
af retro [--since DURATION] [--tag TAG]... [--search QUERY] [--ai] [--limit N]
```

| Flag               | Behaviour                                                                                                                                                                                                          |
| ------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `--since DURATION` | Only notes whose `af_completed_at` is within DURATION. Grammar defined in ADR-056 §"Duration grammar" — supports `Nd`/`Nw` shorthand on top of stdlib `time.ParseDuration`. Examples: `30d`, `4w`, `90d`, `5h30m`. |
| `--tag TAG`        | Filter by `af_tags` containing TAG; repeatable (AND semantics)                                                                                                                                                     |
| `--search QUERY`   | Plain-text grep over note bodies (case-insensitive)                                                                                                                                                                |
| `--ai`             | Pass selected notes to the primary agent for synthesis (see below)                                                                                                                                                 |
| `--limit N`        | Cap to N most recent matches (default 50)                                                                                                                                                                          |

### Discovery

Iterate `~/.local/share/af/v1/archive/*/state.toml`. For each archived workstream,
locate its note via the Obsidian path scheme (per ADR-047 §"Note path"). If the note is
missing (vault unreachable, user moved it), include the workstream metadata only and
emit a `slog.Warn`.

Cross-vault support: any vault listed in `[obsidian.vaults]` is searched. The
`notes_folder` value applies to each (configurable per-vault if owner adds that later).

### Default output

```
3 archived workstreams matching --tag refactor --since 30d:

kakkoyun--refactor-config       2026-04-12  PR #138 merged
  Goal: extract toml loading into internal/config
  Outcome: shipped; reduced cmd/af main from 200 to 80 lines

kakkoyun--refactor-mux          2026-04-22  PR #141 merged
  Goal: ...
  ...
```

The renderer extracts the `## Goal` and `## Outcome` sections from each note's body
(per ADR-047's body template). If sections are missing, falls back to the first
paragraph after frontmatter.

### `--ai` mode

When `--ai` is set, after the filtered list is built, af:

1. Concatenates the selected notes (capped by `--limit`).
2. Invokes the primary agent's `BodyCmd` (introduced in ADR-057 — same mechanism).
3. Feeds a retrospective-specific prompt:

   ```
   Review the workstream notes below and identify:
   - 3 reusable patterns
   - 3 recurring problems
   - 1 process improvement worth proposing
   ```

4. Streams the agent's stdout to the user's stdout.

If `BodyCmd` returns `false` for the configured agent, `af retro --ai` errors with the
same hint as `af pr --ai`.

The agent is invoked with `BodyOpts.Cwd = ""` because retro operates over
archived workstreams whose worktrees have been removed by `af done`/`af clean`.
Per ADR-057's contract, providers use `os.TempDir()` as the working directory
when `Cwd` is empty. This is safe for non-interactive print mode — the agent
reads stdin and writes stdout; no repo operations occur.

### `--json` (omitted from v1)

Deferred. The default human output composes well with `grep`/`fzf`; the `state.toml`
data is already accessible via `af info --json`.

## Consequences

- Completed work becomes searchable from the terminal without opening Obsidian.
- `--ai` reuses ADR-057's `BodyCmd` mechanism — no new agent interface methods.
- Notes remain the source of truth; af is read-only over them (never edits archived
  notes).
- Bases (active/suspended view) and `af retro` (completed view) are clearly partitioned.

## Alternatives Considered

- **Implement as a pure Bases query.** Rejected; the AI-summary path needs terminal-side
  glue, and shell composability matters for the non-AI case.
- **Build an LLM-only retro (skip structured filtering).** Rejected; structured filtering
  is fast, cacheable, and useful even without an agent on hand.
- **Crawl the archive `state.toml` only, ignore notes.** Rejected; the human signal
  lives in the note body, not the structured state.
- **Ingest into a SQLite FTS index.** Rejected for v1; corpus is small (tens to hundreds
  of notes), filesystem grep is sufficient. Revisit if it becomes slow.

## References

- ADR-031 — v1 master.
- ADR-037 — archive layout.
- ADR-047 — note body template, frontmatter schema.
- ADR-057 — `BodyCmd` mechanism reused by `--ai`; `BodyOpts.Cwd = ""` contract used here.
