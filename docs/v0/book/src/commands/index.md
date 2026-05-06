# Command Index

Every `af` subcommand at a glance. Pages are auto-generated from `--help` via
`just book-gen` (see [`scripts/book-gen.sh`](https://github.com/kakkoyun/af/blob/main/scripts/book-gen.sh)),
so they always match the binary.

## Lifecycle

| Command | What it does |
|---|---|
| [`create`](create.md) | Scaffold a new workstream: branch + worktree + mux + agent. |
| [`resume`](resume.md) | Re-attach to an existing session; respawn dead agent panes. |
| [`done`](done.md) | Tear down a session, archive state, optionally delete the branch. |
| [`list`](list.md) | Show every session grouped by repo, with status. |

## Agents inside a session

| Command | What it does |
|---|---|
| [`agent`](agent.md) | Add, stop, or list extra agent slots in a session. |
| [`session-branch`](session-branch.md) | Launch a bare agent on an existing branch (no worktree). |

## Review & ship

| Command | What it does |
|---|---|
| [`diff`](diff.md) | Visual diff of the session vs. its base branch. |
| [`pr`](pr.md) | Open a GitHub pull request from session metadata. |
| [`editor`](editor.md) | Open the worktree in your terminal or GUI editor. |
| [`note`](note.md) | Open the session's Obsidian workstream note. |

## Housekeeping

| Command | What it does |
|---|---|
| [`gc`](gc.md) | Detect merged sessions and clean up worktrees + branches. |
| [`doctor`](doctor.md) | Check the environment for required tools; `--fix` to auto-install. |
| [`config`](config.md) | Show or initialise `~/.config/af/config.toml`. |
| [`completions`](completions.md) | Emit shell completion scripts. |

## Analytics

| Command | What it does |
|---|---|
| [`stats`](stats.md) | Aggregate workstream metrics from the event ledger. |
| [`export`](export.md) | Dump ledger data as JSON or CSV. |

## Meta

| Command | What it does |
|---|---|
| [`version`](version.md) | Print version information. |

## Machine-readable manifest

`commands/index.json` is a stable, machine-readable index of every command
page — useful for documentation tooling, IDE integrations, or the
`af doctor` self-check:

```json
{
  "commands": [
    { "name": "create", "page": "create.md" },
    ...
  ]
}
```

## Related

- [Quickstart](../quickstart.md) — the common happy path in five commands.
- [Three-Layer Architecture](../concepts/providers.md) — how agent, remote,
  and sandbox layers compose.
- [Approval Modes](../concepts/approval-modes.md) — `--auto` vs. `--yolo`.
