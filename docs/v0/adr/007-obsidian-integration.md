# ADR-007: Workstream Documentation (Obsidian)

**Status:** Accepted
**Date:** 2026-03-26

## Context

The user maintains Obsidian vaults for work tracking and wants each `af` workstream to
automatically create/link a document in the vault. This provides:

- A per-session scratchpad for notes, decisions, and context
- A queryable knowledge base of past workstreams (via Obsidian Dataview)
- A bridge between ephemeral git worktree sessions and persistent knowledge

## Decision

### Obsidian integration is opt-in and non-blocking

When enabled, `af create` will:

1. Create a markdown note in the configured Obsidian vault
2. Populate it with session metadata (frontmatter) and a template body
3. Store the note path in session metadata (so `af` can open it later)

### Note structure

```markdown
---
tags: [af, workstream]
af_session: kakkoyun--issue-42
af_repo: myrepo
af_branch: kakkoyun/issue-42
af_agent: claude
af_created: 2026-03-26T14:30:00
af_status: active
---

# issue-42

## Context
<!-- Why this workstream exists -->

## Plan
<!-- What needs to happen -->

## Log
<!-- Append-only session log -->

## Outcome
<!-- Summary when done -->
```

### CLI integration

```
af note [session]            # Open the Obsidian note for a session
af create --note "context"   # Create session + note with initial context
```

### Configuration

```toml
[obsidian]
enabled = true
vault_path = "~/Obsidian/Work"
folder = "workstreams"           # subfolder inside vault
template = "af-workstream"       # Obsidian template name (optional)
```

### Implementation

- Notes are plain markdown files — `af` writes them directly to the vault directory.
- No Obsidian API dependency — just filesystem writes. Obsidian picks up changes via file watcher.
- `af note` opens the file via `open obsidian://open?vault=Work&file=workstreams/issue-42`
  (macOS) or `xdg-open` (Linux).
- Frontmatter is YAML (Obsidian standard). `af` reads/writes it via a simple YAML parser.
- On `af done`, the `af_status` frontmatter field is updated to `completed`.

### Phase

This is a **Phase 4** feature. The core workflow (worktree + multiplexer + agent) must work
first. Obsidian integration is additive and never blocks the main workflow.

## Consequences

- Zero coupling to Obsidian — notes are plain markdown files. Works without Obsidian installed.
- Frontmatter enables Obsidian Dataview queries: `TABLE af_repo, af_agent WHERE af_status = "active"`.
- The `vault_path` is filesystem-based, so Linux and macOS work identically.
- Template support is optional — a default template is embedded in `af`.
- If Obsidian is not configured, all note-related operations are no-ops.
