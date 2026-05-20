---
adr: 046
title: "af suspend / af resume Lifecycle"
status: proposed
implementation: in-progress
date: 2026-05-06
last_modified: 2026-05-21
supersedes: []
superseded_by: null
related: ["031", "037", "039", "041", "042", "043", "056"]
tags: ["go", "lifecycle", "suspend", "resume"]
---

# ADR-046: `af suspend` / `af resume` Lifecycle

## Context

The owner wants to step away from a workstream without ending it: tear
down the resource cost (VMs, remote SSH connections, the local tmux
server's processes for that session) and pick it back up later as if
nothing happened.

v0 had `af resume` only — re-attaching to a still-running tmux
session, or recreating a dead session from disk. v1 adds a paired
`af suspend` so the user can deliberately tear down expensive parts
(remote VMs, sandbox VMs) without losing the workstream's identity.

Resume must be capable of cold-rehydrate: state.toml + ledger.jsonl
on disk are the only persistent input.

## Decision

### Lifecycle states

```
af create   ──► active   ──► af suspend  ──► suspended  ──► af resume  ──► active
                  │                                                            │
                  │                                                            │
                  └─────────────► af done  ◄─────────────────────────────────-─┘
                                       │
                                       ▼
                                completed (clean) | abandoned (--force on unmerged)
```

`status` field in `state.toml` (per ADR-037) takes one of: `active`,
`suspended`, `completed`, `abandoned`.

### `af suspend [session]`

Tears down resource consumers, preserves identity:

| Step | Action                                                                                                                                                            |
| ---- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1    | Write `agent_stopped` ledger event for each running slot. Mark slots `suspended` in `state.toml`.                                                                 |
| 2    | Write `session_suspended` ledger event with `active_slots` snapshot.                                                                                              |
| 3    | Set `state.toml` `[session].status = "suspended"`, `suspended_at = <now>`.                                                                                        |
| 4    | If sandbox: `Sandbox.Teardown(handle)` (slicer VM deleted, sbx container deleted). State.toml records the **provider** but not a stale handle.                    |
| 5    | If remote: SSH in, `tmux kill-session -t <name>` on the remote. (The remote machine itself stays up — `af` doesn't manage remote-machine lifecycle, per ADR-041.) |
| 6    | Local tmux: `tmux kill-session -t <name>`.                                                                                                                        |
| 7    | Print: `Workstream <name> suspended. Resume with: af resume <name>`                                                                                               |

**What's preserved**: `state.toml`, `ledger.jsonl`, the worktree
filesystem, all sub-worktrees, the branch, the agent's own session log
files (which `af` never touches anyway).

**What's destroyed**: tmux session (local and remote), VM(s), SSH
connection. Anything in the agent's volatile state that wasn't
persisted to its own session log file.

### `af resume [session]`

Two paths depending on the workstream's current state:

#### Warm resume (status=active)

The tmux session is still alive somewhere. `af resume`:

1. If local tmux session exists: `tmux attach -t <name>` (or `switch-client` if already inside tmux). Done.
2. If remote tmux session exists: `ssh <host> tmux attach -t <name>`. Done.
3. If neither: fall through to cold rehydrate.

Recovery field in the ledger event: `recovery = "warm"`.

#### Cold rehydrate (status=suspended OR active-but-tmux-gone)

Recreate everything from `state.toml`:

1. Recreate sandbox/remote machinery (per `[execution]`):
   - **Sandbox local**: `Sandbox.Launch(LaunchOpts)` to create a fresh VM/container. Update `[execution].sandbox_id`.
   - **Sandbox remote**: SSH in, then `Sandbox.Launch` on the remote. Update `[execution].sandbox_id`.
   - **Remote (no sandbox)**: SSH in, ensure the remote worktree exists (`git fetch` if it does, `git clone` if not), and create a fresh remote tmux session.
   - **Local**: just create a fresh local tmux session.

2. For each agent slot in `state.toml`:
   - Recreate the tmux pane (split if not primary).
   - Mint a new session ID (per ADR-039); append to `slot.session_ids[]`.
   - Run `Agent.ResumeCmd(opts)` — `pi --continue`, `claude --continue`, `codex resume --last`.
   - Write `agent_resumed` ledger event.
   - Mark slot status `running`, set `last_resumed_at`.

3. Write `session_resumed` ledger event with `recovery = "cold"`.

4. Set `state.toml` `[session].status = "active"`, clear `suspended_at`.

5. Attach: `tmux attach -t <name>` (or `ssh <host> tmux attach -t <name>` for remote).

#### Resume options

| Flag        | Behaviour                                                                                                               |
| ----------- | ----------------------------------------------------------------------------------------------------------------------- |
| `--bare`    | Skip multiplexer; run the primary agent directly in the current shell. Useful for SSH sessions on a constrained remote. |
| `--respawn` | If a sandbox VM is dead per `Sandbox.IsHealthy`, recreate it. Without `--respawn`, an unhealthy sandbox is an error.    |

### Resumption guarantees

- **What's restored automatically**: tmux layout (panes per slot), workstream branch checked out, agent processes running with their own resume mechanism.
- **What's NOT restored**: agent in-memory context that the agent itself didn't persist, terminal scrollback within the panes, any VM-state that didn't survive teardown.

The agent's own `--continue` / `resume --last` decides what context it
brings back. `af` does not touch agent log files (ADR-043).

### Edge cases

| Case                                                    | Behaviour                                                                                                                                                      |
| ------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `af suspend` on a workstream with crashed slots         | Crashed slots are left as `crashed`; `af resume` will not auto-relaunch them. The user does `af agent add --slot <name> --agent <provider>` to spin a new one. |
| `af resume` when a slot's worktree was deleted manually | Error with hint: `worktree at <path> missing; recreate with 'git worktree add' or run 'af done --force'`.                                                      |
| `af resume --respawn` when sandbox provider is missing  | Error with `af doctor` hint.                                                                                                                                   |
| Concurrent `af resume <same>` from two shells           | `flock` (per ADR-037) makes one wait. The second runs against the now-active state and warm-attaches.                                                          |

### Concurrency with `af list`

`af list` is read-only and takes a shared `flock`. Resume holds an
exclusive lock during cold rehydrate, then releases before attaching.

## Consequences

- Long-running workstreams can be parked between sessions without losing identity.
- Remote and sandbox workstreams can be torn down completely when not needed, freeing resources.
- The agent's own resume semantics carry the load of "what context to bring back" — `af` doesn't need to track agent state.
- Cold rehydrate is the only legitimately complex code path; warm resume is trivial (tmux attach).

## Alternatives Considered

- **Symmetric `pause` / `unpause` semantics that preserve VMs.** Rejected; the explicit goal is to free resources during suspend.
- **Auto-suspend after N minutes idle.** Rejected; surprising; out of scope for v1.
- **Single `af resume` that handles both paths transparently** (no `af suspend`). Rejected; without `af suspend` the user has no way to deliberately tear down VMs without losing the workstream.
- **Save full agent process state via CRIU or similar.** Rejected; out of scope; agent-native resume is sufficient.

## References

- v0 ADR-011 — partially superseded for v1 (suspended status added).
- v0 ADR-017 (Remote Resume) — superseded by ADR-041's "tmux on remote" model.
- ADR-031 — v1 master.
- ADR-037 — session metadata (suspended status).
- ADR-039 — multi-agent slot model (resume per slot).
- ADR-041 — SSH remote (suspend kills remote tmux).
- ADR-042 — sandbox (suspend tears down VM).
- ADR-043 — agent providers (resume command per agent).
