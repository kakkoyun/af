---
adr: 048
title: "Minimal Proxy Commands (editor, diff, pr)"
status: proposed
implementation: pending
date: 2026-05-06
last_modified: 2026-05-06
supersedes: []
superseded_by: null
related: ["031", "036", "037", "047"]
tags: ["go", "proxy", "editor", "diff", "pr"]
---

# ADR-048: Minimal Proxy Commands (`editor`, `diff`, `pr`)

## Context

v0 grew rich implementations for `af diff` (diffity-based visual diff
viewer with delta/git-diff fallback), `af pr` (full GitHub PR creation
flow), `af stats`, and `af export`. The owner has decided these should
be **thin wrappers around tools chosen via config**, not in-binary
re-implementations.

The principle: `af` knows which workstream you're in (the worktree, the
base branch, the PR number, the commit range). The user's external
tools (`git`, `gh`, `delta`, `code`, `zed`) know how to do the actual
work. `af`'s job is to call those tools with the right context.

`af stats` and `af export` are dropped for v1 (per ADR-031).

## Decision

### `af editor [--terminal|-t|--visual|-v] [session]`

Opens the workstream's worktree in an editor.

| Mode | Action |
|---|---|
| `--terminal` (default) | Run `[editor].terminal` in a tmux split inside the workstream's session. If `[editor].terminal == "$EDITOR"`, expand `$EDITOR` from the env at exec time. |
| `--visual` | Run `[editor].visual <worktree-path>`. If `[editor].visual` is empty, auto-detect by trying `code` then `zed` in PATH. |

For remote workstreams (per ADR-041), `--visual` falls back to printing
a `vscode-remote://` or `zed://ssh/...` URL the user can click. This is
the entirety of the v0 ADR-019 remote-editor URL scheme machinery,
distilled to two lines of conditional URL construction.

### `af diff [session] [--base REF]`

Runs `[diff].cmd` with token interpolation (per ADR-036).

Default config:
```toml
[diff]
cmd = "git diff {base}...HEAD"
```

The `{base}`, `{head}`, `{worktree}` tokens are substituted from
workstream state. The command runs with the workstream's worktree as
its working directory.

`--base` overrides the base branch. The default uses
`state.toml.[worktree].base_branch`.

User customisations:
```toml
[diff]
cmd = "delta --paging always {base}...HEAD"
# or
cmd = "git diff --color=always {base}...HEAD | diff-so-fancy | less"
```

### `af pr [session] [--title T] [--draft] [--web]`

Runs `[pr].cmd` with token interpolation. Pushes the workstream's
branch first if not pushed.

Default config:
```toml
[pr]
cmd = "gh pr create --base {base} --head {head}"
```

Flags map to additional arguments **only if** the configured command
supports them. v1 implements:

- `--title T` → appended as `--title T` to the configured command (the default `gh pr create` accepts it).
- `--draft` → appended as `--draft`.
- `--web` → appended as `--web`.

For configurations that don't use `gh`, the user can override the flag
mapping in `[pr]` with `flag_template = "..."`. v1 ships sensible
defaults for `gh` only.

After the command exits successfully, `af`:

1. Detects the resulting PR number (parses `gh pr view --json number,url`).
2. Updates `state.toml` `[pr]` and the Obsidian frontmatter (per ADR-047).
3. Writes a `pr_opened` ledger event.

If the configured command is not `gh`, step 1 is skipped; the user
runs `gh pr view` themselves to update PR tracking.

### Why thin wrappers

- The owner already has favourite diff and PR tools (delta, git-diff,
  gh, sometimes hub). Hard-coding `af` to re-implement them locks the
  user into one workflow.
- Token interpolation is small enough (`{base}`, `{head}`, `{worktree}`,
  `{title}`, `{body}`) to maintain by hand.
- Bugs in the underlying tools are not `af`'s problem.

### Command parsing

`[diff].cmd` and `[pr].cmd` are parsed with a small `shlex`-style
splitter (hand-rolled or via the stdlib `strings.Fields` for simple
cases — TBD at impl time). Token replacement happens **before**
splitting, so tokens with spaces (e.g. a multi-word PR title) are
preserved correctly.

If the command after substitution begins with a recognized shell
metacharacter (`|`, `&&`, `;`), `af` runs it via `sh -c "<cmd>"`
instead of direct `exec`. This lets users build pipelines without `af`
parsing them.

## Consequences

- The diff/pr/editor surface stays small and predictable.
- Users can swap tools without touching `af`'s code.
- Less code to maintain than v0's bespoke diff/PR implementations.

## Alternatives Considered

- **Keep v0's rich `af diff` (diffity, fallback chain, etc.).** Rejected per scope cut.
- **Drop `af diff` entirely.** Rejected; the convenience of "diff against the workstream's base from the worktree dir" is real.
- **Make `af pr` a thicker wrapper that knows about `gh` natively.** Rejected; we'd duplicate `gh`'s flag surface and break when `gh` adds flags.

## References

- v0 ADR-019 (Remote Editor URL Schemes) — partially carried forward.
- ADR-031 — v1 master, drops `af stats` / `af export`.
- ADR-036 — `[editor]` `[diff]` `[pr]` config schema and token interpolation rules.
- ADR-037 — workstream state for token sources.
- ADR-047 — Obsidian note updated on `af pr`.
