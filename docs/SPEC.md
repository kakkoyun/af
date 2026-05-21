# af — v1 Specification

> Specification for the v1 (Go) iteration of `af`. This document is
> kept consistent with the v1 ADRs (`docs/adr/031` through the latest
> entry in [`INDEX.md`](adr/INDEX.md)). Design changes go through new
> ADRs, then propagate here in the same commit. The Rust era's spec
> is preserved at `docs/v0/SPEC.md` for historical context only.
>
> **Authority.** Each ADR is authoritative for its decision. This
> document is the reader-friendly aggregate; whenever the SPEC and an
> ADR disagree, the ADR wins. Every section below names the ADR(s) it
> consolidates.

---

## 1. Overview

`af` creates **isolated development workstreams** for AI coding agents.
A workstream is the triple of:

- **Worktree** — a git checkout at a stable path on the user's machine
  (or a remote SSH host).
- **Multiplexer session** — a tmux session per workstream, with one
  pane per running agent.
- **Agent(s)** — one or more AI coding agents (`pi` by default;
  `claude` or `codex` on demand).

The workstream is identified by a **name** (sanitized for tmux), and
tracked via a TOML state file plus an append-only JSONL ledger stored
under `~/.local/share/af/v1/sessions/<name>/`.

A per-repo discovery symlink at `<repo>/.af/state.toml` lets the
binary find "the workstream tied to the current worktree" without
consulting tmux env vars.

`af` is **single-user, single-machine canonical**. State for a given
workstream lives on exactly one host — the one that ran `af create`.
Cross-machine workflows are supported via:

- `af create --remote HOST` (ADR-041) — the host *is* the remote;
  state still lives on the host that ran the command.
- `af control up` (ADR-063) — Tailscale Serve + superterm let other
  machines attach to the host's tmux read-only via a browser.
- `ssh host af ...` — the escape hatch for command-line access from
  another box.

There is no cross-machine state sync; see ADR-069 §2.

---

## 2. Workstream lifecycle

```
af create   ────►  active   ────►  af suspend  ────►  suspended  ────►  af resume  ────►  active
                                                                                              │
                                                                                              ▼
                                                                                          af done
                                                                                              │
                                                                                              ▼
                                                                                          completed
                                                                                          (or abandoned)
```

| State       | Meaning                                       | Tmux processes               | VM / Remote                  |
| ----------- | --------------------------------------------- | ---------------------------- | ---------------------------- |
| `active`    | Workstream running                            | Up                           | Up (if any)                  |
| `suspended` | User invoked `af suspend` to free resources   | Down                         | Down (VM destroyed)          |
| `completed` | `af done` ran cleanly; PR may be open/merged  | Down                         | Down                         |
| `abandoned` | `af done --force` on unmerged work            | Down                         | Down                         |

Defined in ADR-046. The `status` field in `state.toml` (per ADR-037)
takes one of these four values.

Side effects on transitions:

- **`af suspend` / `af done` for slicer-backed workstreams** run
  `af session-data sync --agent all` before destroying the VM, so
  agent conversation history is preserved (ADR-066, ADR-067).
- **`af create` / `af resume`** write the workstream's session ID into
  `state.toml.[[agents]].session_ids` and emit `agent_launched`
  / `agent_resumed` ledger events (ADR-039).
- **`af done`** force-refreshes `[pr].state` (ADR-071) before deciding
  `completed` vs `abandoned`.
- **`af suspend` / `af resume`** flip the Obsidian frontmatter
  `af_status` to `suspended` / `active` (ADR-047).

Suspended workstreams are reconstructible: `af resume <name>`
recreates the tmux session, recreates VMs/remote connections, and
relaunches each agent using its native resume mechanism
(`pi --continue`, `claude --continue`, `codex resume --last`).
Anything the agent did not persist to its own session log is lost.

---

## 3. Command surface

ADR-035 is the **authoritative CLI contract**; this section stays
consistent with it. When a per-command ADR adds or changes a flag,
ADR-035 and this section are updated in the same commit.

Every command that accepts `[session]` resolves it via the
ADR-070 resolution order: positional arg → `--session NAME` →
`AF_SESSION` env → cwd `.af/state.toml` symlink → `fzf` picker on
stderr (TTY only) → `EX_NOINPUT` error.

### 3.1 Creation, teardown, listing

| Command                                                                                                                                     | Purpose                                                                                                                                                                                      |
| ------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `af create [name] [--from BRANCH] [--current] [--from-pr N] [--bare] [--remote HOST] [--sandbox slicer] [--agent NAME] [--yolo] [--auto]`   | Create a workstream: branch, worktree, tmux session, primary agent (pi by default). Fork-source flags per ADR-038; sandbox is slicer-only per ADR-060; name uniqueness enforced per ADR-069. |
| `af done [session] [--force]`                                                                                                               | Tear down a workstream: force-refresh PR state, kill tmux, run `af session-data sync` for slicer VMs, remove worktree, delete branch (if `--force` or merged), archive state and ledger.     |
| `af list`                                                                                                                                   | List active + suspended workstreams grouped by repo. Read-only, no PR refresh.                                                                                                               |
| `af resume [session] [--bare] [--respawn]`                                                                                                  | Re-attach to active workstream, or rehydrate a suspended one. `--bare` skips multiplexer; `--respawn` recreates dead sandbox VMs.                                                            |
| `af suspend [session]`                                                                                                                      | Run `af session-data sync` for slicer VMs, persist state, tear down tmux + remote/sandbox. Workstream becomes `suspended`.                                                                   |
| `af session-branch`                                                                                                                         | Launch the default agent with a session ID derived from the current branch (no worktree). For ad-hoc work in the existing checkout.                                                          |

### 3.2 Multi-agent management

All three subcommands accept `--session NAME` to target a workstream
other than the inferred one.

| Command                                                            | Purpose                                                                                                                         |
| ------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------- |
| `af agent add [--slot <name>] --agent <provider> [--session NAME]` | Add a new agent in a new tmux pane. `--slot` is optional; auto-assigned from the agent name. Sub-worktree if `slot != primary`. |
| `af agent stop <slot> [--remove-worktree] [--session NAME]`        | Stop the agent in the named slot. `--remove-worktree` also removes the sub-worktree.                                            |
| `af agent list [--session NAME]`                                   | Tabular output of slot, agent, status, pane.                                                                                    |

### 3.3 Inspection

| Command                                                          | Purpose                                                                                                              |
| ---------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------- |
| `af list`                                                        | One-line per workstream, grouped by repo, current repo first. Read-only. PR state from cache only.                   |
| `af status [--json] [--all] [--filter STATE] [--refresh]`        | Multi-line dashboard with per-slot status (ADR-054). TTL-refreshed PR cache (ADR-071). `--refresh` forces refresh.   |
| `af info [session] [--json] [--ledger N] [--refresh]`            | Detail view for one workstream (ADR-055). `--ledger N` shows the last N events. `--refresh` forces PR refresh.       |

### 3.4 Reaping

| Command                                                                                 | Purpose                                                                                                                                                                                                                                                       |
| --------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `af clean [--dry-run] [--include-abandoned] [--max-age DURATION] [--force [<name>...]]` | Reap workstreams verified as merged by three-strategy detection (PR state → ancestry → squash fingerprint) per ADR-056. Always force-refreshes PR state (ADR-071). `--force <name>...` skips merge detection for named workstreams only. Replaces v0's `af gc`. |

### 3.5 Stacking

| Command                                | Purpose                                                                                                                                          |
| -------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------ |
| `af stack [session] [--parent PARENT]` | Link a workstream to a parent so subsequent operations base off the parent's branch. Without `--parent`, prints the current parent transitively. |
| `af unstack [session]`                 | Clear the workstream's parent link; subsequent ops use `base_branch` again.                                                                      |
| `af sync [session]`                    | Force-refresh parent PR state (ADR-071); rebase onto parent's current head; auto-reparent if parent merged. Conflicts halt with a hint.          |

### 3.6 Environment & utilities

| Command                                                                      | Purpose                                                                                                                                                                                                            |
| ---------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `af setup [--force] [--shell SHELL] [--skip-completions] [--skip-gitignore]` | One-shot user-scope environment setup: gitignore entry, completions, config init, vault hint. `--force` overwrites existing config; `--shell` overrides shell auto-detect; `--skip-*` flags skip individual steps. |
| `af doctor [--remote HOST] [--verbose]`                                      | Probe required tools (`tmux`, `git`, `pi`, `claude`, `codex`, `gh`, `slicer`, `fzf`, `tailscale`, `superterm`); print install commands. **Never** auto-installs.                                                   |
| `af config show \| init`                                                     | Print effective merged config, or write default config + Obsidian vault hint to `~/.config/af/config.toml`.                                                                                                        |
| `af completions <shell>`                                                     | Emit shell completion script (`bash zsh fish powershell`). Completion sources per ADR-068 §5.                                                                                                                      |

### 3.7 Notes & retro

| Command                                                                          | Purpose                                                                                                   |
| -------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------- |
| `af note [session] [--append TEXT]`                                              | Open the workstream's Obsidian note (or via `$EDITOR` fallback). With `--append TEXT`, append a timestamped log entry under `## Log`. |
| `af retro [--since DURATION] [--tag TAG]... [--search QUERY] [--ai] [--limit N]` | Mine archived workstream notes for patterns (ADR-058). `--ai` synthesises a narrative via `agent.BodyCmd` (ADR-057). Operates on archive only; no PR refresh. |

### 3.8 Editor (proxy)

| Command                                              | Default behaviour                                                                                                                  | Config knob                            |
| ---------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------- |
| `af editor [--terminal\|-t\|--visual\|-v] [session]` | `$EDITOR` in a tmux split, or `code .` / `zed .` for visual. Optional `[session]` targets a workstream other than the current one. | `[editor].terminal`, `[editor].visual` |

### 3.9 Diff rendering

Defined in ADR-064. `af diff` is no longer a pure proxy.

| Command                                          | Behaviour                                                                                                                                                                                          |
| ------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `af diff [session] [--base REF] [--web]`         | Terminal mode (default): if `hunk` is on `PATH`, pipe `git diff --no-color {base}...HEAD` to `hunk patch -`; else fall back to `git diff`; non-TTY emits `git diff --stat`. `--web` opens diffity. |

`[diff].cmd` from ADR-036/ADR-048 is now an explicit escape hatch
(`[diff].mode = "custom"` activates it) rather than the default. The
default `[diff].mode = "opinionated"` selects the dispatch table
above. Token interpolation (`{base}`, `{head}`, `{worktree}`) still
applies in custom mode.

### 3.10 PR creation (proxy + agent body)

| Command                                                                  | Behaviour                                                                                                                                       | Config knob              |
| ------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------ |
| `af pr [session] [--title T] [--draft] [--web] [--ai] [--ai-model M] [--refresh]` | `gh pr create --base <base_branch> --head <branch>` by default. With `--ai`, the body is authored by the configured agent (ADR-057). `--refresh` force-refreshes PR cache (ADR-071) without opening anything. | `[pr].cmd`, `[pr].ai_model` |

`--ai` and `--web` are mutually exclusive.

### 3.11 Secrets

Defined in ADR-049.

| Command               | Purpose                                               |
| --------------------- | ----------------------------------------------------- |
| `af auth set <key>`   | Prompt for value (echo-off) and store in keyring.     |
| `af auth get <key>`   | Print value to stdout (TTY only; redacted otherwise). |
| `af auth status`      | List known keys with availability + source.           |
| `af auth clear <key>` | Remove from keyring.                                  |
| `af auth list`        | List names of all `af`-stored keys (no values).       |

### 3.12 Remote control

Defined in ADR-063. **Host-level**, not per-workstream — superterm
exposes the whole tmux server.

| Command                                                              | Purpose                                                                                                          |
| -------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| `af control up [session] [--remote HOST] [--provider superterm] [--port PORT] [--json]` | Start/ensure remote control is available. Boots `superterm` + Tailscale Serve binding.        |
| `af control down [--remote HOST]`                                    | Tear down the remote-control binding.                                                                            |
| `af control status [--remote HOST] [--json]`                         | Report up/down + Tailscale URL + bound tmux server.                                                              |

Repo config (`<repo>/.af/config.toml`) sets `[control].remote_control = "superterm"` to opt the repo in (ADR-061); state.toml captures `execution.remote_control` (ADR-072).

### 3.13 VM agent session-data sync

Defined in ADR-066 and ADR-067. Applies only to slicer-backed
workstreams.

| Command                                                                                  | Purpose                                                                                                                                  |
| ---------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------- |
| `af session-data sync [session] [--agent all\|claude\|codex\|pi\|harness] [--continue-host] [--dry-run]` | Inventory + copy session data from the VM home directory into the host's matching agent/harness directories. Idempotent. |
| `af session-data list [session] [--vm VM]`                                               | Inventory session files in the VM without importing.                                                                                     |
| `af pull [session]`                                                                      | Run `slicer wt pull` for the workstream; import VM branches, fast-forward host branch, release the lease (ADR-065).                      |

`af suspend` and `af done` invoke `af session-data sync --agent all`
automatically before tearing down the VM. `--continue-host`
normalises imported sessions so the user can continue the
conversation on the host (when the agent format permits).

### 3.14 Review draft

Defined in ADR-073. Read-only, repo-aware PR review draft written to
`<worktree>/.af/reviews/`. Shares the `agent.BodyCmd` pattern
(ADR-043 / ADR-057) with `af pr --ai` and `af retro --ai`, but is
shaped for richer per-repo guidance via an immutable system prompt
plus four-layer per-repo append (user config → repo config →
repo-relative file → CLI flag).

| Command | Purpose |
| --- | --- |
| `af review [session] [--pr N] [--agent NAME] [--model M] [--base REF] [--out PATH] [--stdout] [--append-prompt TEXT] [--skill NAME]... [--print-system-prompt]` | Build a review-draft prompt (immutable af system prompt + per-repo appends + suggested skills + PR context + `gh pr diff` output), invoke the configured agent's `BodyCmd`, and atomically write the resulting report to `<worktree>/.af/reviews/<UTC-ts>-pr<n>.md` (`0o600` under `0o750`, `.tmp` + rename). |

- PR resolution: `--pr N` wins; otherwise `gh pr view --json
  number,title,headRefName,baseRefName` against the current branch.
  No PR → `errReviewNoPR`.
- Diff source: `gh pr diff <n>` (matches the PR UI diff). Empty diff
  → `errReviewEmptyDiff` (agent is not invoked).
- `--print-system-prompt` is a debugging aid: it prints the fully
  resolved prompt and exits without invoking the agent or hitting
  the PR.
- `--stdout` prints the report to stdout instead of writing a file.
- `--skill ""` suppresses the suggested-skills hints for one
  invocation; `--skill <name>` (repeatable) replaces the config list.
- Named failure modes: `errReviewNoPR`, `errReviewEmptyDiff`,
  `errReviewAgentUnavailable`, `errReviewEmptyBody`, plus `gh`
  missing and agent non-zero exit (no partial write).
- A `review.report.written` ledger event is appended when a session
  is active, with fields `{pr, path, agent, model}`. Otherwise no
  state mutation.

### 3.15 Meta

| Command      | Purpose                                                |
| ------------ | ------------------------------------------------------ |
| `af version` | Print version, commit, build date.                     |
| `af --help`  | Top-level help. Subcommand help via `af <cmd> --help`. |

---

## 4. Workstream identifiers

### 4.1 Names

- User-supplied via `af create <name>`, or auto-generated as
  `<repo>-<YYYYMMDD-HHMMSS>`.
- Sanitized for tmux: `/`, `.`, `:` → `--`. Example:
  `kakkoyun/issue-42` → `kakkoyun--issue-42`.
- Optional branch prefix: when the repo has an `upstream` remote and
  `[branch].prefix_on_fork_only = true` (default), `<name>` becomes
  `<config.branch.prefix>/<name>` before sanitization (ADR-038).
- **Globally unique per host** across `sessions/` and `archive/`.
  `af create` fails closed on collision (ADR-069 §3).

### 4.2 Session IDs

- The **slot identity** `(repo_name, branch_name, slot_name)` is
  stable across machines and reboots.
- Each agent **launch** within a slot mints a new UUID v5:
  `uuid5(NAMESPACE_DNS, "{repo}/{branch}/{slot}/{launch-timestamp-ns}")`.
  Resumes within a slot append to `state.toml`'s `session_ids[]`.
- Some agents accept the session ID via flag (claude `--session-id
  <uuid>`); others (pi, codex) use their native resume mechanism and
  the session ID is recorded for `af`'s tracking only. ADR-039.

### 4.3 Worktree path

- Stable: `~/Workspace/.worktrees/<repo>/<branch>/`. Configurable via
  `[general].worktree_root`.
- Sub-worktrees for subagents:
  `~/Workspace/.worktrees/<repo>/<branch>--<slot>/` on branch
  `<branch>--<slot>` forked from `<branch>` (ADR-038).

### 4.4 Resolution order for `[session]`

Per ADR-070, in order until one resolves:

1. Explicit positional arg `[session]`.
2. `--session NAME` flag (wins with warning if both 1 and 2 given).
3. `AF_SESSION` env var.
4. cwd inference: walk up to find `.af/state.toml` symlink.
5. Interactive `fzf` picker on stderr, **only if** stdin and stderr
   are TTYs, `fzf` is on `PATH`, and at least one workstream exists.
6. Hard error → `EX_NOINPUT` (66).

Read-only commands (`af list`, `af status`) skip resolution and list
everything.

---

## 5. State files

### 5.1 Layout

```
~/.local/share/af/v1/
├── sessions/
│   └── <session>/
│       ├── state.toml           # Live workstream state
│       ├── ledger.jsonl         # Append-only event log
│       └── .af.lock             # ADR-068 §4 per-session flock
├── archive/
│   └── <session>/               # Moved here by `af done` when [lifecycle].auto_archive=true (ADR-046)
└── secrets/                     # Persistent-disk fallback for the ephemeral envelope (ADR-049)

<repo>/.af/
├── state.toml -> symlink to ~/.local/share/af/v1/sessions/<session>/state.toml
└── reviews/                  # `af review` writes drafts here (ADR-073); 0o750 dir, 0o600 files
```

`<repo>/.af/` is added to the user's global `.gitignore`
(`~/.config/git/ignore`) by `af setup`.

### 5.2 `state.toml` schema (v1, schema_version = 1)

Foundational schema is defined in ADR-037; the consolidated
as-implemented schema is defined in ADR-072. ADRs 061, 062, 065 are
fully implemented (Stage 10 + Stage 11). ADRs 067 (session sync) and
071 (PR cache) are accepted but implementation pending; their fields
are marked **PROPOSED** below.

```toml
schema_version = 1

[session]
name          = "kakkoyun--issue-42"
id            = "<uuid v5>"
created_at    = 2026-05-06T12:00:00Z
status        = "active"            # active | suspended | completed | abandoned
approval_mode = ""                  # optional; agent-provider approval override
max_agents    = 0                   # optional; 0 = config default
# suspended_at omitted until status = "suspended"

[worktree]
path         = "/Users/kemal/Workspace/.worktrees/af/kakkoyun--issue-42"
branch       = "kakkoyun/issue-42"
base_branch  = "upstream/main"
git_root     = "/Users/kemal/Workspace/Projects/Personal/af"
repo_slug    = "kakkoyun/af"        # owner/name from upstream/origin; "" => `af status` CI = n/a

[execution]
mode             = "local"          # local | bare | remote | sandbox
multiplexer      = "tmux"
tmux_session     = "kakkoyun--issue-42"
ssh_host         = ""               # populated for remote mode
remote_path      = ""
sandbox_provider = ""               # "" | "slicer" (ADR-060; "sbx" rejected on load)
sandbox_id       = ""
remote_control               = ""   # ADR-061 capture; "" | "superterm" (omitempty)
sandbox_resource_profile     = ""   # ADR-062 captured profile (all omitempty below)
sandbox_resource_vcpu        = 0
sandbox_resource_ram_gb      = 0
sandbox_resource_storage_size = ""
sandbox_resource_gpu_count   = 0
sandbox_resource_image       = ""
sandbox_resource_hypervisor  = ""
sandbox_managed_group        = ""   # "af-<slug>-<profile>" or explicit group

[[agents]]
slot            = "primary"
provider        = "pi"
session_ids     = ["<uuid v5>"]
pane            = "%0"
status          = "running"         # running | stopped | crashed | suspended
sub_worktree    = ""                # absolute path to sibling sub-worktree, if any
sub_branch      = ""
created_at      = 2026-05-06T12:00:00Z
# last_resumed_at omitted until first resume

[pr]
number              = 0
url                 = ""
state               = ""            # "" | open | draft | closed | merged
# PROPOSED (ADR-071, implementation pending):
last_refreshed_at   = ""            # ISO; "" = never refreshed
last_refresh_error  = ""            # truncated 120-char error

[stack]                             # ADR-059 — always present; empty when unstacked
parent_session = ""
parent_branch  = ""
# linked_at omitted until the parent is set

[slicer_wt]                         # ADR-065 — omitted entirely when vm == ""
vm            = ""                  # holder VM; "" when not leased
path          = ""                  # worktree path leased to the VM
pushed_at     = 2026-05-21T15:00:00Z
pulled_at     = ""                  # ISO; set after `slicer wt pull`
lease_state   = ""                  # held_by_vm | pulled | discarded

[[session_sync]]                    # PROPOSED (ADR-067, implementation pending)
agent           = "claude"          # claude | codex | pi | harness
source_root     = "/home/agent/.claude/sessions"
last_synced_at  = ""
last_hash       = ""                # sha256 of last imported tail
last_offset     = 0                 # byte offset for resumable JSONL appends

[versions]
af             = "1.0.0"
agent_versions = { pi = "...", claude = "..." }
```

**Derived values** (not stored):

- `last_touched_at` — latest `ts` in `ledger.jsonl`. O(1) via tailing
  the last line. Used by `af status` (sort key) and `af clean
  --max-age`. ADR-037 §"Derived values".

### 5.3 `ledger.jsonl` events

One JSON object per line. ADR-037 defines the foundational event
types; ADR-071 adds `pr_state_changed`; ADR-073 adds
`review.report.written`. Notable events:

```
session_created, agent_launched, agent_added, agent_stopped,
agent_crashed, agent_resumed,
session_suspended, session_resumed,
session_completed, session_abandoned,
pr_opened, pr_merged, pr_closed, pr_state_changed,
stack_linked, stack_unlinked, stack_reparented, synced,
control_up, control_down,
session_data_synced,
review.report.written,
error
```

Every agent-scoped event carries `slot`, `agent`, and (where
relevant) `session_id` keys. `review.report.written` carries
`{pr, path, agent, model}` (ADR-073 §9); it uses dot-notation
because the event semantically lives in the `review` sub-namespace.
New namespaced event classes follow the same `<area>.<verb>` pattern.

### 5.4 Atomicity & concurrency

Per ADR-068 §4:

- Each session directory carries `.af.lock` at the root.
- Mutating ops (`create`, `suspend`, `resume`, `done`, `note --append`,
  `agent {add,stop}`, `sync`, `pr`, `stack`, `session-data sync`,
  PR-state writes) acquire it (exclusive flock, 30-second timeout).
  Timeout → `EX_TEMPFAIL` (75).
- Read-only ops (`list`, `status`, `info`) do not lock. ADR-037's
  atomic `state.toml` writes guarantee they see a consistent file.
- No cross-session lock. No daemon. Each `af` invocation is a fresh
  process.

---

## 6. Configuration

### 6.1 Files

| Path                       | Purpose                                                         |
| -------------------------- | --------------------------------------------------------------- |
| Compiled defaults          | Built into the binary.                                          |
| `~/.config/af/config.toml` | User-level (vaults, default agent, prefix, lifecycle, secrets). |
| `<repo>/.af/config.toml`   | Per-repo overrides; **repo-only** sections live here.           |

Merge order: defaults → user → repo. Last writer wins per field.
Repo-only sections that **must not** appear in the user config:
`[control]` (ADR-061) and `[sandbox.slicer.resources]` (ADR-062),
because both are inherently per-project decisions.

### 6.2 Schema

Foundational schema in ADR-036. Sections, with ADR ownership noted:

- `[general]` — `default_agent`, `multiplexer`, `max_sessions`,
  `worktree_root`. (ADR-036)
- `[branch]` — `prefix`, `prefix_on_fork_only`. (ADR-038)
- `[editor]` — `terminal`, `visual`. (ADR-048)
- `[diff]` — `mode` (`"opinionated"` default per ADR-064, or
  `"custom"`), `shell`, `cmd`, token-interpolated. (ADR-036 + ADR-064)
- `[pr]` — `shell`, `cmd`, `flag_template`, `template`, `ai_model`,
  `refresh_ttl` (default `10m`, ADR-071).
- `[review]` — `agent`, `model`, `system_prompt_append`,
  `system_prompt_append_file`, `suggested_skills`
  (default `["/review", "/go-review", "/simplify"]`). Layered
  user → repo per ADR-036 precedence. (ADR-073)
- `[status]` — `max_parallel` (default 8; cap on concurrent `gh pr
  view` fetches). (ADR-054)
- `[remote]` — `default_host`, `ssh_options`. (ADR-041)
- `[sandbox]` — `default_provider` (must resolve to `"slicer"` per
  ADR-060), `[sandbox.slicer]` (`group`), `[sandbox.slicer.resources]`
  (vcpu/ram_gb/storage_size/gpu_count/image/hypervisor, ADR-062;
  repo-only).
- `[obsidian]` — `notes_vault` (key from `[obsidian.vaults]`),
  `notes_folder`, `notes_template`. (ADR-047)
- `[obsidian.vaults]` — **user-only**; map of vault-name → absolute
  path on this machine.
- `[control]` — **repo-only** (ADR-061); `remote_control =
  "superterm" | "off"`, plus future per-repo launch defaults.
- `[doctor]` — `extra_tools`. (ADR-044)
- `[secret]` — `keyring_service`, `redact_keys`. (ADR-049)
- `[lifecycle]` — `auto_archive`, `retention_days` (reserved).
  (ADR-046)

---

## 7. Agent providers

Three providers in v1, all behind a single `internal/agent.Agent`
interface. Defined in ADR-043.

| Agent    | Binary   | Default? | Resume flag                               | Yolo flag                        |
| -------- | -------- | -------- | ----------------------------------------- | -------------------------------- |
| `pi`     | `pi`     | ✅       | `--continue`                              | (per agent's CLI)                |
| `claude` | `claude` |          | `--continue` (with `--session-id <uuid>`) | `--dangerously-skip-permissions` |
| `codex`  | `codex`  |          | `resume --last`                           | `--full-auto`                    |

Each provider exposes (full signatures in ADR-043):

- `Name() string`, `Binary() string`, `IsAvailable(ctx) bool`
- `LaunchCmd(LaunchOpts) []string` — argv for new session.
- `ResumeCmd(ResumeOpts) []string` — argv for resumed session.
- `PRCmd(prNumber, LaunchOpts) ([]string, bool)` — argv for
  PR-review session, when supported.
- `BodyCmd(BodyOpts) ([]string, bool)` — argv for non-interactive
  print mode used by `af pr --ai` / `af retro --ai` (ADR-057). The
  prompt template is **hard-coded** in v1.
- `SessionLogPaths(sessionID, projectPath) []string` — for analysis
  only; `af` never deletes agent log files.

---

## 8. Multiplexer

tmux only. Defined in ADR-040. Single `internal/mux.Multiplexer`
interface; one `Tmux` impl. Operations: create/kill session, attach,
send-keys, set/get session env, set option, list sessions, split pane,
list/kill panes.

The tmux session name equals the workstream name; `af create` calls
`tmux setenv -t <session> AF_SESSION <session>` so any `af`
invocation from inside a pane resolves the right workstream (ADR-070
step 3).

---

## 9. Remote

SSH only. Defined in ADR-041. The "remote" is whatever string the
user passes to `--remote`: an alias from `~/.ssh/config`, or
`user@host`, or an IP. `af` does not validate or special-case it.
Connection is established via `ssh <host> <command>`; tmux is
launched on the remote to keep the session alive across drops.

There is **no** plugin layer. exe.dev, DD Workspaces, or any other VM
provider is provisioned externally by the user; `af` only consumes
the SSH host name.

The remote-control surface (`af control up/down/status`, ADR-063)
attaches another machine to the host's tmux via Tailscale Serve +
superterm. Canonical state still lives on the host (ADR-069 §2).

---

## 10. Sandbox

Slicer only. Defined in ADR-042, narrowed to slicer-only by ADR-060.

| Provider | Binary   | Backend             | Local | Remote                        |
| -------- | -------- | ------------------- | ----- | ----------------------------- |
| `slicer` | `slicer` | Firecracker microVM | ✅    | ✅ (composes with `--remote`) |

`af create --sandbox slicer` is the only valid form. A state file
recording `sandbox_provider = "sbx"` from an experimental build fails
closed with a migration hint (ADR-060).

Composition: `af create --remote <host> --sandbox slicer` runs the
slicer daemon on the remote, builds a VM there, and launches the
agent inside.

### 10.1 Resource profiles

Defined in ADR-062. Repo config `[sandbox.slicer.resources]` declares
the desired profile (vcpu, ram, storage, gpu, image, hypervisor).
`af create --sandbox slicer` resolves a managed group named
`af-<repo-slug>-<profile>`, probes Slicer for its existence, creates
it with `count: 0` if missing, and records the effective profile as
flat fields under `state.toml.[execution]` (prefix
`sandbox_resource_*` + `sandbox_managed_group`; see ADR-072
§Block-naming rationale). Group-shape mismatch is a hard error.

### 10.2 Worktree transport (`slicer wt`)

Defined in ADR-065. The host worktree is **leased to the VM** while
the VM holds it. `slicer wt push` initialises the VM with a
sanitised `.git` (no host hooks, no credentials, identity-only
sync); `slicer wt pull` brings VM commits back as
`refs/slicer/<vm>/*` refs that fast-forward the host branch. The
user must not edit the host worktree until pull completes.

`state.toml.[slicer_wt]` records the holder VM, the leased worktree
path, push/pull timestamps, and a `lease_state` of
`held_by_vm | pulled | discarded` (ADR-072).

`af` wraps the pull side with an `af pull [session]` command (added
by Stage 11 implementation): it dispatches `slicer wt pull` for the
workstream, imports VM branches under `refs/slicer/<vm>/*`,
fast-forwards the host branch, and updates the lease to `pulled`.
The workstream must have `[slicer_wt].lease_state == "held_by_vm"`.
After `af pull`, the host branch carries the VM commits and can be
pushed normally.

### 10.3 Agent session-data sync

Defined in ADR-066 (transport) and ADR-067 (lifecycle integration).
`af session-data sync` (auto-run on `af suspend` / `af done`)
harvests Claude / Codex / pi / harness session files from the VM
home directory and merges them into the host's matching directories
with prefix-append guards and per-agent cursors recorded in
`state.toml.[[session_sync]]`.

---

## 11. Secrets

Defined in ADR-049.

- **Storage**: `zalando/go-keyring` (macOS Keychain, Linux Secret
  Service).
- **Service name**: `af` (no `af/` prefix on accounts).
- **Transport to sandbox / remote**: ephemeral envelope file. Never
  SSH `SetEnv`/`SendEnv`.
- **Storage location** (selected at runtime):
  - Linux with `/run/user/$UID/` writable:
    `/run/user/$UID/af-<session>/.env` (tmpfs).
  - macOS, or Linux without `/run/user/$UID/`:
    `~/.local/share/af/v1/secrets/af-<session>/.env` (persistent
    disk).
- **Transport mechanics**:
  1. `af` writes the envelope at the path above with `chmod 600`.
  2. Mount/copy/`scp` into the sandbox or remote as required.
  3. The launch wrapper runs
     `. <path>/.env && rm -f <path>/.env && exec <agent-cmd>`. The
     delete is non-optional.
  4. Stray envelopes from crashes are reaped by an inline 60-minute
     stale-file sweep run at the head of every `af` invocation that
     touches the secrets directory.
- **Redaction**: `slog` handlers redact known secret-bearing keys
  plus any user-listed `[secret].redact_keys`.

---

## 12. Obsidian integration

Defined in ADR-047. `af` writes a per-workstream markdown note with
`af_schema: 1` frontmatter:

```yaml
---
af_schema: 1
af_session: kakkoyun--issue-42
af_repo: af
af_branch: kakkoyun/issue-42
af_base_branch: upstream/main
af_status: active
af_agents:
  - slot: primary
    provider: pi
    status: running
af_started_at: 2026-05-06T12:00:00Z
af_completed_at: null
af_pr_number: 0
af_pr_url: ""
af_pr_state: ""
tags: [af]
af_tags: []
---
```

`tags: [af]` drives the example Obsidian Base
(`examples/obsidian/active-workstreams.base`). `af_tags` is the
user-editable extension list.

Lifecycle hooks:

- `af create` writes the note.
- `af suspend` / `af resume` flip `af_status`.
- `af done` sets `af_status` to `completed` or `abandoned` and
  populates `af_completed_at`.
- `af note [session]` opens the file via Obsidian URI scheme or
  `$EDITOR`.

Since canonical state is single-machine (ADR-069 §2), the
`af_session` value is unique *per host* only. A multi-machine vault
should treat `(af_repo, af_branch)` as the composite key.

---

## 13. Doctor + Setup

| Command                     | Scope                                                                                                                                                                                                                          | Auto-install?                                                  |
| --------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------- |
| `af doctor`                 | Probe local tools (`tmux`, `git`, `pi`, `claude`, `codex`, `gh`, `slicer`, `fzf`, `tailscale`, `superterm`); print install commands.                                                                                            | **No.**                                                        |
| `af doctor --remote <host>` | Same probe over SSH; print install commands for the remote's package manager.                                                                                                                                                  | **No.**                                                        |
| `af setup`                  | Idempotent user-scope setup: add `.af/` to `~/.config/git/ignore`; install shell completions for the detected shell; create `~/.local/share/af/v1/` tree; run `af config init` if no config exists; print Obsidian vault hint. | Local user files only. **No** `sudo`, **no** package installs. |

Per-platform install hints in `af doctor` output:

- macOS: `brew install <pkg>` for tools available via Homebrew.
- Arch: `pacman -S <pkg>`.
- Debian/Ubuntu: `apt install <pkg>`.
- Tools without distro packages (e.g. `pi`, `slicer`, `superterm`):
  print upstream install instructions.

`fzf` is recommended (powers the ADR-070 picker fallback); doctor
calls this out explicitly.

---

## 14. Build & distribution

Defined in ADR-053.

- Build tool: `Make` (`Makefile` at repo root).
- Cross-compile targets: `linux/amd64`, `linux/arm64`, `darwin/amd64`,
  `darwin/arm64`.
- Release tool: `goreleaser` in `--snapshot` mode only. No GitHub
  Releases for v1.
- Distribution: `go install github.com/kakkoyun/af@latest` or `make
  install`.
- No Homebrew tap.

---

## 15. Operational contracts

This section consolidates the cross-cutting contracts defined in
ADRs 068–071. Each command in §3 honours them.

### 15.1 Network / telemetry promise (ADR-069 §1)

`af`'s own code makes **zero** outbound network calls. The only
network traffic during an `af` invocation comes from sub-processes
the user explicitly invoked (`git`, `gh`, `ssh`, `slicer`,
`tailscale`, agent CLIs).

Specifically, `af` does **not**: check for newer versions, emit
crash reports, ship analytics, or phone home for any reason. New
outbound calls require an amending ADR.

### 15.2 stdout / stderr / TTY / color (ADR-068 §3)

- **stdout** carries the command's data (tabular text, JSON, or
  empty).
- **stderr** carries all diagnostics, progress, prompts, fzf picker
  output, and errors.
- Color is enabled when the target stream is a TTY, `NO_COLOR` is
  unset, and `TERM != dumb`. `FORCE_COLOR` overrides the first two
  signals; `NO_COLOR` always wins.
- No global `--quiet` flag in v1.

### 15.3 Exit codes (ADR-068 §2)

| Code  | Symbol            | Meaning                                              |
| ----- | ----------------- | ---------------------------------------------------- |
| `0`   | `EX_OK`           | Success.                                             |
| `1`   | `EX_GENERAL`      | Generic unclassified error.                          |
| `2`   | `EX_USAGE_COBRA`  | Cobra-surfaced usage error.                          |
| `64`  | `EX_USAGE`        | Argument or flag validation failure.                 |
| `65`  | `EX_DATAERR`      | Bad `state.toml` / `config.toml` / `ledger.jsonl`.   |
| `66`  | `EX_NOINPUT`      | Session, branch, or file not found.                  |
| `69`  | `EX_UNAVAILABLE`  | Required external tool missing.                      |
| `70`  | `EX_SOFTWARE`     | Internal invariant violated (a bug).                 |
| `75`  | `EX_TEMPFAIL`     | Retryable failure (network, lock timeout).           |
| `77`  | `EX_NOPERM`       | Permission denied.                                   |
| `130` | `EX_INTERRUPTED`  | `SIGINT` received during the command.                |

### 15.4 JSON output schema (ADR-068 §1)

Every `--json` payload:

```json
{
  "schema": <integer>,
  "data": { ...command-specific... }
}
```

`schema` is per-command; bumps only on breaking changes. Additions
to `data` are non-breaking. Errors during `--json` invocations
write a JSON error doc to **stderr** (stdout stays empty) and exit
non-zero:

```json
{"schema": <int>, "error": {"code": "EX_NOINPUT", "message": "...", "hint": "..."}}
```

### 15.5 `AF_*` environment variables

| Variable                  | When set                                  | When read                                          |
| ------------------------- | ----------------------------------------- | -------------------------------------------------- |
| `AF_SESSION`              | `af create` writes via `tmux setenv` to the pane env. | Step 3 of ADR-070 resolution.            |
| `AF_LOG_LEVEL`            | User exports (e.g. `debug`, `info`, `warn`, `error`). | Read by `slog` setup in `main`.          |
| `AF_CONFIG`               | User exports to override config path.                 | Config loader (compiled defaults → user → repo). |
| `AF_TEST_FAKEBIN`         | Set by testscript scenarios.                          | Test harness prepends to `PATH`.        |
| `AF_TEST_MUX`             | `fake` to swap the multiplexer interface in tests.    | `internal/mux` factory.                 |
| `NO_COLOR` / `FORCE_COLOR`| User exports.                                         | Per §15.2 above.                        |

New `AF_*` vars require an amending ADR.

### 15.6 Concurrency (ADR-068 §4)

Per-session `flock` at `.af.lock` for mutating ops; no cross-session
lock; no daemon. Lock timeout returns `EX_TEMPFAIL` (75).

### 15.7 Tab completion (ADR-068 §5)

Cobra completion for bash/zsh/fish/powershell, sourced via
`af completions <shell>`. Completes workstream names, branch names,
agent providers, sandbox providers, SSH host aliases, slot names,
shell names, and lifecycle states. No network in completion code
paths.

---

## 16. Out of scope for v1

- DD Workspaces remote provider, exe.dev special-casing (ADR-031,
  ADR-041).
- Zellij / Ghostty / cmux multiplexers (ADR-040).
- Skill bundle installer (v0 ADR-030).
- Auto-install in doctor (ADR-044).
- `af log`, workspace templates, Dataview dashboards.
- `sbx` sandbox provider (retired by ADR-060).
- `gemini`, `amp`, `copilot` agents (ADR-043).
- mdBook user guide.
- Migration from v0 state files (`af migrate`).
- Releases, changelogs cross-signed against tags, Homebrew taps
  (ADR-053).
- Multi-machine state sync (ADR-069 §2 — single-machine canonical
  is the explicit decision).
- Telemetry, version-check pings, crash reports (ADR-069 §1).
- Outbound network calls from `af` itself (ADR-069 §1).
- Global `--quiet` flag (ADR-068 §3).
- TLA+ / formal-method coverage beyond ADR-052's two state machines.

These are listed in `TODO.md` Backlog. They may return as ADRs in a
later iteration; they do not block v1.

---

## 17. References

- [`docs/adr/INDEX.md`](adr/INDEX.md) — full v1 ADR catalogue (031–073).
- [`docs/v0/SPEC.md`](v0/SPEC.md) — v0 (Rust era) spec, immutable.
- [`docs/v0/adr/`](v0/adr/) — 30 v0 ADRs, frozen.
