---
adr: 048
title: "Minimal Proxy Commands (editor, diff, pr)"
status: proposed
implementation: pending
date: 2026-05-06
last_modified: 2026-05-08
supersedes: []
superseded_by: null
related: ["031", "036", "037", "047", "057", "059"]
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

| Mode                   | Action                                                                                                                                                    |
| ---------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `--terminal` (default) | Run `[editor].terminal` in a tmux split inside the workstream's session. If `[editor].terminal == "$EDITOR"`, expand `$EDITOR` from the env at exec time. |
| `--visual`             | Run `[editor].visual <worktree-path>`. If `[editor].visual` is empty, auto-detect by trying `code` then `zed` in PATH.                                    |

For remote workstreams (per ADR-041), `--visual` falls back to printing
a `vscode-remote://` or `zed://ssh/...` URL the user can click. This is
the entirety of the v0 ADR-019 remote-editor URL scheme machinery,
distilled to two lines of conditional URL construction.

### `af diff [session] [--base REF]`

Runs `[diff].cmd` with token interpolation (per ADR-036).

Default config (argv form, per §"Command parsing: explicit argv vs.
shell" below):

```toml
[diff]
shell = false
cmd   = ["git", "diff", "{base}...HEAD"]
```

The `{base}`, `{head}`, `{worktree}` tokens are substituted from
workstream state. The command runs with the workstream's worktree as
its working directory.

`--base` overrides the base branch. The default resolves as:

1. If `state.toml.[stack].parent_branch` is non-empty → that.
2. Else `state.toml.[worktree].base_branch`.

This honors stacked workstreams transparently; non-stacked workstreams behave
unchanged.

User customisations:

```toml
# argv-style (default; recommended)
[diff]
cmd = ["delta", "--paging", "always", "{base}...HEAD"]

# shell-style (when you need pipes, redirects, &&, etc.)
[diff]
shell = true
cmd = "git diff --color=always {base}...HEAD | diff-so-fancy | less"
```

### `af pr [session] [--title T] [--draft] [--web]`

Runs `[pr].cmd` with token interpolation. Pushes the workstream's
branch first if not pushed.

Default config (argv form):

```toml
[pr]
shell = false
cmd   = ["gh", "pr", "create", "--base", "{base}", "--head", "{head}"]
```

Flags map to additional arguments through a configurable
`flag_template` table on the `[pr]` section. v1 ships defaults that
match `gh pr create`:

```toml
[pr]
cmd = ["gh", "pr", "create", "--base", "{base}", "--head", "{head}"]
flag_template = {
  title = ["--title", "{title}"],
  draft = ["--draft"],
  web   = ["--web"],
  body  = ["--body", "{body}"],
}
```

When the user passes `--title T`, `af` appends `flag_template.title`
with `{title}` substituted. When `--draft` is passed, `af` appends
`flag_template.draft` verbatim. Boolean flags whose template is empty
or missing are silently ignored.

**`af pr --ai` body delivery.** When `--ai` is passed (per ADR-057),
`af` captures the agent's stdout into `{body}` and **automatically
appends `flag_template.body`** to the resulting argv. The user does
not pass `--body` separately. If `flag_template.body` is unset,
`af pr --ai` errors out before the agent runs, with a hint to add
it to `[pr].flag_template`.

`--ai` is **incompatible** with `--web`: the web flow defers body
authoring to `gh`'s browser dialog, so invoking an agent first would
waste an API call. `af pr --ai --web` is rejected at flag-validation
time before the agent runs (see ADR-057's failure table).

For configurations that don't use `gh` (e.g. `glab` for GitLab), the
user overrides `flag_template` in their config; v1 ships only the `gh`
defaults. The flag-template surface is documented under ADR-036's
`[pr]` section.

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

### Command parsing: explicit argv vs. shell

Each proxy section (`[editor]`, `[diff]`, `[pr]`) accepts `cmd` in one
of two shapes, with an explicit `shell` flag that picks the execution
mode. **There is no auto-detection from string contents** — the user
declares intent.

#### Argv form (default)

```toml
[diff]
cmd = ["delta", "--paging", "always", "{base}...HEAD"]
# shell = false  (the default)
```

- `cmd` is a TOML array of strings.
- Each element is independently passed through token substitution
  (`{base}`, `{head}`, `{worktree}`, `{title}`, `{body}`).
- `af` invokes `exec.CommandContext(ctx, cmd[0], cmd[1:]...)` directly.
- Tokens with spaces (e.g. a multi-word `{title}`) survive intact
  because each argv element is independent.
- **No shell is involved.** `|`, `&&`, `;`, `>`, `*`, `~`, etc. inside
  argv elements are passed literally to the spawned binary; they do
  not have shell semantics.

#### Shell form (opt-in)

```toml
[diff]
shell = true
cmd = "git diff --color=always {base}...HEAD | diff-so-fancy | less"
```

- `cmd` is a single string.
- Token substitution is applied to the string, then `af` invokes
  `exec.CommandContext(ctx, "sh", "-c", expandedCmd)`.
- Pipes, redirects, glob expansion, environment variable expansion,
  and other shell features all work normally.
- Token values containing single quotes or shell metacharacters are
  shell-quoted before substitution to prevent injection of structure.
  (Tokens are workstream-derived: branch names, paths, PR titles. Shell
  quoting them is sufficient for v1's threat model.)

#### Why explicit and not auto-detected

The original draft of this ADR proposed auto-detecting shell mode by
scanning for metacharacters. That is unreliable: `git diff X...Y |
delta` does not begin with `|` so the heuristic misses it; conversely
a branch name containing `&&` or `|` triggers a false positive. The
user knows which mode they want; an explicit flag eliminates the
guesswork. This also gives `af` a clean place to apply per-mode
security policy (no shell expansion in argv mode means no path
injection through `{worktree}`).

#### Default-form recommendation

Defaults ship in argv form because it's safer and faster (no shell
fork). Users who want pipes opt in via `shell = true`.

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
- ADR-059 — stacked workstreams change `--base` default to `parent_branch`.
