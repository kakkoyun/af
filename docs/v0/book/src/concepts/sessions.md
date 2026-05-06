# Sessions

A **session** is the unit of work in `af`. One session = one worktree + one
multiplexer session + one (or more) agent processes, all tied to a branch.

## What a session bundles

| Component | Where it lives |
|---|---|
| Git worktree | `~/Workspace/.worktrees/<repo>/<session-name>` |
| Branch | `<prefix>/<session-name>` (prefix depends on fork status) |
| Multiplexer session | `tmux`/`cmux` session named after the session |
| Agent process(es) | Inside the multiplexer, one per slot |
| Session state | `~/.local/state/af/sessions/<session-id>/state.toml` |
| Event ledger | `~/.local/state/af/sessions/<session-id>/ledger.jsonl` |
| Obsidian note (optional) | `<vault>/Workstreams/<session-name>.md` |

The session **ID** is a UUIDv5 derived from repo URL + session name — stable
across machines, so the same session can be referenced consistently.

## Lifecycle

```
af create   →   af resume   ↺   af done
   │              │              │
   │              │              └─ teardown: stop agents, remove worktree,
   │              │                 archive state + ledger, optional branch delete
   │              └─ re-attach: restore all agents in their slots
   └─ scaffold: create branch + worktree + mux session, launch agent(s)
```

### create

```bash
af create fix-auth-bug
```

Writes `state.toml` immediately, emits a `created` event to the ledger, spawns
the multiplexer session, then launches the configured agent inside it. See
[`create`](../commands/create.md) for flags (`--agent`, `--from`, `--sandbox`,
`--remote`, etc.).

### resume

```bash
af resume fix-auth-bug       # specific session
af resume                    # fzf picker over all sessions
```

Re-attaches to the mux session. If any agent pane has exited, `af resume`
respawns it from the saved `state.toml` + agent-slot info. See
[`resume`](../commands/resume.md).

### done

```bash
af done                      # with confirmation
af done --force              # skip prompts; force-delete unmerged branch
```

Tears down in reverse order: stops every agent, kills the mux session,
removes the worktree, emits `completed` or `closed` to the ledger, archives
the session under `~/.local/state/af/archive/` (retained 90 days by
default), and optionally deletes the branch. See [`done`](../commands/done.md).

## State on disk

`state.toml` is the durable anchor. Everything else — Obsidian note, ledger,
branch, worktree — is derivable from it.

```toml
# Example state.toml (abridged)
id = "3f8c4d2a-..."
name = "fix-auth-bug"
branch = "kakkoyun/fix-auth-bug"
worktree = "/Users/kemal/Workspace/.worktrees/af/fix-auth-bug"
af_version = "0.1.0"
agent_config_hash = "sha256:..."

[execution]
mode = "Local"

[[agents]]
slot = "primary"
name = "claude"
session_id = "abc123..."
```

The **ledger** (`ledger.jsonl`) is append-only. Each session event
(`created`, `agent_launched`, `resumed`, `pr_opened`, `completed`, …) is one
JSON line. Use [`af export`](../commands/export.md) and
[`af stats`](../commands/stats.md) to analyse.

## Multi-agent sessions

A single session can host multiple agents in separate panes:

```bash
af create backend-refactor                  # primary agent (claude)
af agent add --slot review --agent codex    # review pane running codex
af agent list                               # show all slots
af agent stop review                        # stop one slot
```

On [`resume`](../commands/resume.md), every slot is restored. On
[`done`](../commands/done.md), every slot is stopped.

## Remote sessions

With `--remote`, the worktree + mux + agent all live on a remote VM. Your
local state.toml still tracks the session but `execution.mode = "Remote"`
with provider + SSH target metadata. See
[Three-Layer Architecture](providers.md).

## Related

- [`create`](../commands/create.md), [`resume`](../commands/resume.md),
  [`done`](../commands/done.md) — the lifecycle commands.
- [`list`](../commands/list.md) — every session grouped by repo.
- [`stats`](../commands/stats.md), [`export`](../commands/export.md) —
  ledger analytics.
