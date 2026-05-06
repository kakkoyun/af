# ADR-030: `af` Skill Bundle — URL-Driven Claude Code Skill Installer

**Status:** Accepted
**Date:** 2026-04-22
**Relates to:** ADR-007 (Obsidian), ADR-011 (Ledger), ADR-020 (mdBook), ADR-022 (cmux multiplexer).

## Context

`af` exposes 23+ subcommands and several invariants (session naming, branch
convention, Obsidian note layout, ledger event schema). Today an agent
rediscovers the surface every session from `--help` output and README
snippets, and the Obsidian note (ADR-007) is silent about which Claude Code
conversations touched the workstream.

Prior art — the **cmux skill** distributed at
`https://cmux-artemzhutov.netlify.app/skills/cmux.md` — demonstrates a simple
recipe: one markdown page with three fenced blocks (a `SKILL.md` body, a
bash helper `spawn-workspace.sh`, a Python `SessionStart`/`SessionEnd` hook
`cmux-session-map.py`) plus canonical install paths (`.claude/skills/<tool>/`
and `~/.claude/hooks/`). The user fetches the URL, extracts the blocks, and
`chmod +x`'s the scripts. Claude Code's skill and hook machinery does the
rest.

That pattern composes cleanly with `af`'s existing surfaces:

- The Obsidian note has a `## Log` section waiting for per-session entries.
- The session ledger (ADR-011) already records `agent_*` events and welcomes
  a new `claude_session_started` / `claude_session_ended` pair.
- The mdBook (ADR-020) is already the canonical versioned distribution
  channel — publishing a file at `book/src/skills/af.md` serves it at
  `https://kakkoyun.github.io/af/guide/skills/af.md` with no new pipeline.

Without this bundle, Claude Code on an `af` workstream is blind to the
workstream's own records.

## Decision

### 1. Ship a versioned skill bundle at a canonical URL

Publish `book/src/skills/af.md` as the single source of truth for the
bundle. The file contains, in this order, three fenced code blocks annotated
by install target:

1. **`SKILL.md`** — agent-facing instructions with a description opening on
   the trigger keywords: *"resume workstream", "open workstream note", "diff
   workstream", "af session", "create pr from workstream".* Documents the
   common idioms: `af session-branch`, `af note`, `af diff`, `af pr`, `af
   agent add`, `af resume`, and the `.af/` session-dir layout.
2. **`af-workstream.sh`** — bash helper wrapping the common
   spawn-a-pair-agent-on-the-current-workstream idiom on top of
   `af agent add`, equivalent in shape to cmux's `spawn-workspace.sh`.
3. **`af-session-bind.py`** — Python hook for `SessionStart`, `SessionEnd`,
   and `PreCompact` events. On start it binds the Claude session id to the
   current `af` session (resolved from `$AF_SESSION_NAME` or by walking up
   from `$PWD` to find `.af/session.toml`); on end it records the session
   duration and token counts; on pre-compact it flushes a summary line.

The bundle version tracks the `af` release — each tag republishes the file.

### 2. Install layout mirrors cmux's (Claude Code convention)

| Target | Path | Permissions |
|---|---|---|
| Skill body | `.claude/skills/af/SKILL.md` | 0644 |
| Helper script | `.claude/skills/af/scripts/af-workstream.sh` | 0755 |
| Session-binding hook | `~/.claude/hooks/af-session-bind.py` | 0755 |

The `.claude/skills/af/` directory is per-project (picked up by Claude Code's
skill discovery) while the hook is per-user (registered in
`~/.claude/settings.json`'s hook block the installer appends to).

### 3. New subcommand: `af skill install`

```
af skill install [--url URL] [--skill-dir DIR] [--hook-dir DIR]
                 [--from-file PATH] [--force] [--dry-run]
```

- Default `--url` resolves to the current binary's release channel
  (`env!("AF_SKILL_URL")` baked at build time, falling back to
  `https://kakkoyun.github.io/af/guide/skills/af.md`).
- Default `--skill-dir` is `./.claude/skills/af`; default `--hook-dir` is
  `$XDG_CONFIG_HOME/claude/hooks` (or `~/.claude/hooks`).
- `--from-file` bypasses the network — the installer parses a local file,
  enabling airgapped installs and tests.
- `--dry-run` prints the planned writes without touching the filesystem.
- Pre-existing files are moved aside to `<name>.bak-<rfc3339>` unless
  `--force`.
- Idempotent: re-running with identical bundle content is a no-op.

The installer is a thin markdown parser over pulldown-cmark (already a
transitive dep via `mdbook`; if not, add it explicitly — it is 0 additional
Rust footprint at runtime): walk the code blocks, match each by its fenced
info-string (e.g., <code>```bash path=scripts/af-workstream.sh</code>), write,
`chmod`, done.

### 4. Hook integrates with ledger + Obsidian without new dependencies

`af-session-bind.py` is a standalone Python 3 script (Claude Code's hook
runner shells out) that:

- Reads the Claude Code hook payload from stdin (`session_id`,
  `hook_event_name`, `cwd`, timing).
- Resolves the `af` session by `$AF_SESSION_NAME`, else by scanning `$PWD`
  and ancestors for `.af/session.toml`.
- **Ledger write:** appends a newline-delimited JSON record directly to
  `<session_dir>/ledger.jsonl` — the same format ADR-011 already accepts.
  No socket, no IPC — file append is atomic for small writes on the
  ADR-011-supported filesystems.
- **Obsidian write:** if `obsidian.enabled`, appends a line to the note's
  `## Log` section. Respects the same `frontmatter` rules ADR-007 fixed
  (uses the `replace_frontmatter_field` idiom already in
  `src/obsidian/mod.rs`).

No Rust-side Obsidian daemon. No new data path between the hook and `af`.

### 5. Non-Claude-Code agents degrade gracefully

`af skill install` is a no-op on agents other than Claude Code — the
SKILL.md and hook do nothing for codex/gemini/amp/copilot/pi. The
`af-workstream.sh` helper, however, works under any shell and stays useful
for everyone. This follows the degrade-to-none precedent of ADR-028.

## Alternatives considered

- **Embed the bundle directly in the `af` binary.** Rejected: updating the
  SKILL description would require a new `af` release. The URL-driven model
  lets users pin a bundle independently, follows cmux's precedent, and
  leverages the mdBook we already publish.
- **Ship only the Python hook, skip the SKILL.md.** Rejected: the SKILL
  description is what makes Claude Code *discover* the idiom. Without it,
  the agent still calls `af --help` on every session. The hook alone fixes
  the ledger half of the problem but not the agent-teaching half.
- **Expose a Unix socket (`/tmp/af.sock`) like cmux's `/tmp/cmux.sock`.**
  Deferred. A socket is warranted once we want bidirectional
  notifications (progress updates, status pills). For 0.1.0, filesystem
  writes to the session directory are enough and require no supervision.
  0.2.0 can add it if demand materializes.
- **Per-repo skill that lives in the consumer repo's `.claude/skills/`.**
  Rejected: that design drifts out of sync with `af`'s CLI surface as soon
  as a new subcommand lands. A canonical URL tracked with `af` itself keeps
  the bundle honest.
- **Make the installer generic: `af skill install <URL>` as a general
  fetcher.** Rejected for scope — it invites us to become a package
  manager. The installer is specifically for `af`'s own bundle; a generic
  fetcher can come later if users ask.

## Consequences

- **New Lane L-SKILL:** ~200 LOC for `af skill install` + the in-tree
  `hooks/af-session-bind.py` + the `book/src/skills/af.md` page. Plus
  integration tests via `--from-file` (ADR-029's idiom).
- **Ledger gains two new event types** — `claude_session_started` and
  `claude_session_ended` — recorded without any Rust code path, purely by
  the Python hook appending to `ledger.jsonl`. `af stats` and `af export`
  pick them up for free thanks to ADR-011's schema being a superset-by-
  design.
- **Obsidian Log section becomes live** — each Claude Code conversation
  that touches a workstream leaves a timestamped entry. This is what ADR-007
  was originally reaching for and was never wired up.
- **Security boundary:** the hook makes no network calls and only writes
  under `<session_dir>/ledger.jsonl` and `<vault>/Workstreams/<name>.md`.
  Path traversal is prevented by resolving both locations through the `af`
  binary's own config, never from the hook payload. The bash helper only
  invokes documented `af` subcommands — audit-friendly.
- **`af doctor`** gains a check: "af skill bundle installed? (optional)" —
  info-level, never blocks.
- **Discoverability:** `af --help` mentions `af skill install` so users
  find it without reading release notes. The mdBook grows a *Skill bundle*
  chapter linked from the introduction (ADR-020 §Structure already has the
  `concepts/` and `commands/` directories — this is a new `skills/`
  sibling).
- **Future-proofing:** if cmux (ADR-022) lands as a multiplexer and the
  user runs `af` inside a cmux workspace, the hook can additionally read
  `$CMUX_WORKSPACE_ID` / `$CMUX_SURFACE_ID` and write them into the
  ledger, giving us a three-way join (Claude session ↔ af session ↔ cmux
  surface) at zero code cost.
