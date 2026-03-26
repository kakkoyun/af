# ADR-011: Workstream Lifecycle & Session Ledger

**Status:** Accepted
**Date:** 2026-03-26
**Amends:** ADR-006 (extends session metadata with lifecycle events)

## Context

ADR-006 defines session metadata as a static TOML snapshot. That captures *what* a session
is, but not *what happened* during it. The user needs:

1. **Workstream history** — a log of events: created, agent launched, agent resumed, PR
   opened, worktree cleaned, session ended. Useful for analysing work patterns.

2. **Agent session tracking** — each `af` workstream may spawn multiple agent sessions
   (e.g., `claude --session-id X`, then later `claude --continue`, or switching from
   `claude` to `pi` mid-stream). Each agent also creates its own session files:
   - Claude: `~/.claude/projects/<path>/<uuid>.jsonl`
   - pi: `~/.pi/agent/sessions/<path>/<timestamp>_<uuid>.jsonl`
   These are the agent's property — `af` should **never delete them** but should record
   references so they can be found later for analysis.

3. **PR association** — when a PR is opened from a workstream branch, `af` should record
   the PR number/URL. This links code review back to the workstream.

4. **Crash recovery** — if tmux dies, a remote machine crashes, or the user reboots,
   the ledger has enough state to reconstruct what was running and attempt revival.

5. **Retention with rolling cleanup** — `af` session files should be kept for a
   configurable period (default: 90 days) after the workstream ends. Agent session
   files are never touched by `af`.

6. **Version pinning** — record the `af` version and agent config versions (skills list,
   settings hash) at session creation. This helps diagnose "it worked last week" issues.

## Decision

### Two-layer session storage

```
~/.local/share/af/
├── sessions/
│   └── kakkoyun--issue-42/
│       ├── state.toml        # Live state (mutable, current snapshot)
│       └── ledger.jsonl      # Append-only event log
└── archive/
    └── kakkoyun--issue-42/   # Moved here after af done (retained for 90 days)
        ├── state.toml
        └── ledger.jsonl
```

### `state.toml` — Live session state (extends ADR-006)

```toml
[session]
name = "kakkoyun--issue-42"
id = "550e8400-e29b-41d4-a716-446655440000"
created_at = 2026-03-26T14:30:00Z
status = "active"              # active | paused | completed | abandoned

[worktree]
path = "/home/user/Workspace/.worktrees/myrepo/kakkoyun/issue-42"
branch = "kakkoyun/issue-42"
base_branch = "upstream/main"
git_root = "/home/user/Work/myrepo"

[execution]
mode = "local"
multiplexer = "tmux"
multiplexer_session = "kakkoyun--issue-42"

# Multiple agents can run concurrently on the same workstream.
# Each entry is keyed by a slot name (user-chosen or auto-assigned).
# A workstream always has a "primary" agent launched at creation time;
# additional agents can be added via `af agent add`.

[[agents]]
slot = "primary"
provider = "claude"
session_ids = ["550e8400-e29b-41d4-a716-446655440000"]
pane = "0"                     # multiplexer pane ID (for send-keys / reconnect)
status = "running"             # running | stopped | crashed

[[agents]]
slot = "review"
provider = "pi"
session_ids = ["7a1b2c3d-..."]
pane = "1"
status = "running"

# [[agents]]
# slot = "test-runner"
# provider = "codex"
# session_ids = []
# pane = "2"
# status = "stopped"

[pr]
number = 0                     # 0 = no PR yet
url = ""
state = ""                     # "" | "open" | "merged" | "closed"

[versions]
af = "0.1.0"
agent_config_hash = "a1b2c3d4" # SHA256 prefix of agent settings at creation time
# skills = ["skill-a", "skill-b"]  # agent skills active at session creation

[remote]
# Only present for remote/sandbox modes
# provider = "exedev"
# host = "myvm.exe.xyz"
# work_dir = "/home/ubuntu/src/myrepo"
# sandbox_provider = "slicer"
# sandbox_name = "slicer-3"
# yolo = false
```

### `ledger.jsonl` — Append-only event log

One JSON object per line, chronologically ordered. Never edited, only appended.

```jsonl
{"ts":"2026-03-26T14:30:00Z","event":"session_created","af_version":"0.1.0","agents":["claude"],"mode":"local","branch":"kakkoyun/issue-42","base":"upstream/main"}
{"ts":"2026-03-26T14:30:01Z","event":"agent_launched","slot":"primary","agent":"claude","session_id":"550e8400-...","pane":"0","cmd":"claude --session-id 550e8400-..."}
{"ts":"2026-03-26T15:00:00Z","event":"agent_added","slot":"review","agent":"pi","pane":"1","cmd":"pi"}
{"ts":"2026-03-26T15:00:01Z","event":"agent_launched","slot":"review","agent":"pi","session_id":"7a1b2c3d-...","pane":"1","cmd":"pi"}
{"ts":"2026-03-26T15:45:00Z","event":"agent_resumed","slot":"primary","agent":"claude","cmd":"claude --continue"}
{"ts":"2026-03-26T16:30:00Z","event":"agent_stopped","slot":"review","agent":"pi","reason":"user_request"}
{"ts":"2026-03-26T17:00:00Z","event":"agent_added","slot":"tests","agent":"codex","pane":"2","cmd":"codex --full-auto"}
{"ts":"2026-03-26T17:00:01Z","event":"agent_launched","slot":"tests","agent":"codex","session_id":"c0d3x-...","pane":"2","cmd":"codex --full-auto"}
{"ts":"2026-03-26T17:30:00Z","event":"pr_opened","number":142,"url":"https://github.com/org/repo/pull/142"}
{"ts":"2026-03-26T18:00:00Z","event":"session_paused","reason":"user_detached","active_agents":["claude","codex"]}
{"ts":"2026-03-27T09:00:00Z","event":"session_resumed","recovery":"metadata_restore"}
{"ts":"2026-03-27T12:00:00Z","event":"pr_merged","number":142}
{"ts":"2026-03-27T12:05:00Z","event":"session_completed","duration_hours":21.58,"agents_used":["claude","pi","codex"]}
```

#### Event types

Every agent event carries a `slot` field to identify which agent instance is affected.

| Event | When | Key fields |
|---|---|---|
| `session_created` | `af create` | agents (initial list), mode, branch, base, af_version |
| `agent_launched` | Agent process started | **slot**, agent, session_id, pane, cmd |
| `agent_added` | Additional agent joined workstream | **slot**, agent, pane, cmd |
| `agent_resumed` | Agent reattached | **slot**, agent, cmd |
| `agent_stopped` | Agent intentionally stopped | **slot**, agent, reason |
| `agent_crashed` | Agent process died unexpectedly | **slot**, agent, exit_code |
| `mux_reconnected` | Multiplexer session recovered | recovery method |
| `remote_reconnected` | SSH/VM reconnection | host, attempt |
| `sandbox_respawned` | Dead VM replaced | old_vm, new_vm |
| `pr_opened` | PR created from branch | number, url |
| `pr_merged` | PR merged | number |
| `pr_closed` | PR closed without merge | number |
| `session_paused` | User detached | reason, active_agents |
| `session_resumed` | User reattached | recovery method |
| `session_completed` | `af done` | duration_hours, agents_used |
| `session_abandoned` | `af done --force` on unmerged | reason |
| `error` | Any recoverable error | message, context |

### Agent session file discovery

`af` records agent session IDs in `state.toml` but does **not** manage the agent's files.
For analysis, `af` can locate them via convention:

```rust
pub trait AgentProvider {
    // ... existing methods ...

    /// Return paths to session log files for a given session ID.
    /// Returns empty vec if the agent doesn't support session discovery.
    fn session_log_paths(&self, session_id: &str, project_path: &Path) -> Vec<PathBuf>;
}
```

| Agent | Session file pattern |
|---|---|
| Claude | `~/.claude/projects/<encoded_path>/<session_id>.jsonl` |
| pi | `~/.pi/agent/sessions/<encoded_path>/<timestamp>_<session_id>.jsonl` |
| Others | TBD (implement as we learn their patterns) |

### Retention & archival

```toml
# ~/.config/af/config.toml

[lifecycle]
# Days to retain archived session data after af done
retention_days = 90
# Auto-archive completed sessions (move from sessions/ to archive/)
auto_archive = true
```

Lifecycle:

1. **Active:** `sessions/<name>/` — workstream is in progress
2. **Archived:** `af done` moves to `archive/<name>/` with `status = "completed"`
3. **Expired:** `af gc` deletes archives older than `retention_days`

`af` **never deletes**:

- Agent session files (`~/.claude/projects/...`, `~/.pi/agent/sessions/...`)
- Git branches that have open PRs
- Worktree directories (those are cleaned by `af done`/`af gc`)

### PR tracking

`af` can detect PR creation in two ways:

1. **Explicit:** `af pr` command (future) creates a PR and records it
2. **Detection:** On `af list` / `af gc`, check `gh pr view <branch>` and update state

### Version pinning

At session creation, `af` records:

```toml
[versions]
af = "0.1.0"
agent_config_hash = "a1b2c3d4"
```

The `agent_config_hash` is a short hash of the agent's settings file at creation time.
This lets the user correlate "session X was created with config version Y" when debugging
regression in agent behaviour after skill updates.

## Consequences

- The **ledger** is the permanent record. Even after archival and eventual deletion of
  `state.toml`, the ledger captures what happened for pattern analysis.
- JSONL format is append-friendly, grep-friendly, and trivially parseable.
- Agent session files are the agent's domain — `af` references but never owns them.
- The `archive/` directory is a grace period, not permanent storage. Users who want
  permanent records should use the Obsidian integration (ADR-007).
- PR tracking closes the loop: workstream → branch → PR → merge status.
- Version pinning helps debug "it worked before" — diff the agent config hash.
- The event log enables future analytics: average session duration, agent switch frequency,
  crash recovery rate, time-to-PR, etc.
