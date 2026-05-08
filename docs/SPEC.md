# af — v1 Specification

> Specification for the v1 (Go) iteration of `af`. This document is
> **editable during the planning phase** so it can be kept consistent
> with the v1 ADRs as they iterate. It freezes once all v1 ADRs are
> `accepted` (per ADR-032's status lifecycle); after that, design
> changes go through new ADRs in `docs/adr/`. The Rust era's spec is
> preserved at `docs/v0/SPEC.md` for historical context only.

---

## 1. Overview

`af` creates **isolated development workstreams** for AI coding agents.
A workstream is the triple of:

- **Worktree** — a git checkout at a stable path on the user's machine
  (or a remote SSH host).
- **Multiplexer session** — a tmux session per workstream, with one pane
  per running agent.
- **Agent(s)** — one or more AI coding agents (pi by default; claude or
  codex on demand).

The workstream is identified by a **name** (sanitized for tmux), and
tracked via a TOML state file plus an append-only JSONL ledger stored
under `~/.local/share/af/v1/sessions/<name>/`.

A per-repo discovery symlink at `<repo>/.af/state.toml` lets the binary
find "the workstream tied to the current worktree" without consulting
tmux env vars.

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

| State       | Meaning                                      | Tmux server processes                     | VM / Remote                                    |
| ----------- | -------------------------------------------- | ----------------------------------------- | ---------------------------------------------- |
| `active`    | Workstream running                           | Up                                        | Up (if any)                                    |
| `suspended` | User invoked `af suspend` to free resources  | Down (the workstream's session is killed) | Down (VM destroyed, remote SSH session killed) |
| `completed` | `af done` ran cleanly; PR may be open/merged | Down (cleaned up)                         | Down                                           |
| `abandoned` | `af done --force` on unmerged work           | Down                                      | Down                                           |

Suspended workstreams are reconstructible: `af resume <name>` recreates
the tmux session, recreates VMs/remote connections, and relaunches each
agent using its native resume mechanism (`pi --continue`,
`claude --continue`, `codex resume --last`). Anything the agent did not
persist to its own session log is lost.

---

## 3. Command surface

### 3.1 Creation, teardown, listing

| Command                                    | Purpose                                                                                                                                                                 |
| ------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `af create [name]`                         | Create a workstream: branch, worktree, tmux session, primary agent (pi by default).                                                                                     |
| `af done [session]`                        | Tear down a workstream: kill tmux, remove worktree, delete branch (if `--force` or branch is merged), tear down remote/sandbox if applicable, archive state and ledger. |
| `af list`                                  | List active workstreams grouped by repo. Includes status column (`active`, `suspended`).                                                                                |
| `af resume [session] [--bare] [--respawn]` | Re-attach to an active workstream, or rehydrate a suspended one. `--bare` skips multiplexer; `--respawn` recreates dead sandbox VMs.                                    |
| `af suspend [session]`                     | Persist state, tear down tmux + remote/sandbox to free resources. Workstream becomes `suspended`.                                                                       |
| `af session-branch`                        | Launch the default agent with a session ID derived from the current branch (no worktree). For ad-hoc work in the existing checkout.                                     |

### 3.2 Multi-agent management

All three subcommands accept `--session NAME` to target a workstream other than the current one.

| Command                                                            | Purpose                                                                                                                         |
| ------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------- |
| `af agent add [--slot <name>] --agent <provider> [--session NAME]` | Add a new agent in a new tmux pane. `--slot` is optional; auto-assigned from the agent name. Sub-worktree if `slot != primary`. |
| `af agent stop <slot> [--remove-worktree] [--session NAME]`        | Stop the agent in the named slot. `--remove-worktree` also removes the sub-worktree.                                            |
| `af agent list [--session NAME]`                                   | Tabular output of slot, agent, status, pane.                                                                                    |

### 3.3 Inspection (ADR-054, ADR-055)

| Command                                       | Purpose                                                                                                      |
| --------------------------------------------- | ------------------------------------------------------------------------------------------------------------ |
| `af list`                                     | One-line per workstream, grouped by repo, current repo first. Read-only.                                     |
| `af status [--json] [--all] [--filter STATE]` | Multi-line dashboard with per-slot status. `--all` includes archived; `--filter` narrows by lifecycle state. |
| `af info [session] [--json] [--ledger N]`     | Detail view for one workstream. `--ledger N` shows the last N events.                                        |

### 3.4 Reaping (ADR-056)

| Command                                                                                 | Purpose                                                                                                                                                                                                                                                                            |
| --------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `af clean [--dry-run] [--include-abandoned] [--max-age DURATION] [--force [<name>...]]` | Reap workstreams verified as merged by three-strategy detection (PR state → ancestry → squash fingerprint). `--dry-run` previews; `--include-abandoned` adds `abandoned` ones; `--force <name>...` skips merge detection for named workstreams only. Replaces v0/early-v1 `af gc`. |

### 3.5 Stacking (ADR-059)

| Command                                | Purpose                                                                                                                                          |
| -------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------ |
| `af stack [session] [--parent PARENT]` | Link a workstream to a parent so subsequent operations base off the parent's branch. Without `--parent`, prints the current parent transitively. |
| `af unstack [session]`                 | Clear the workstream's parent link; subsequent ops use `base_branch` again.                                                                      |
| `af sync [session]`                    | Rebase/fast-forward the workstream onto its parent's current head. If parent merged, fall back to `base_branch`.                                 |

### 3.6 Environment & utilities

| Command                                   | Purpose                                                                                       |
| ----------------------------------------- | --------------------------------------------------------------------------------------------- |
| `af setup`                                | One-shot user-scope environment setup: gitignore entry, completions, config init, vault hint. |
| `af doctor [--remote <host>] [--verbose]` | Probe required tools; print install commands. **Never** auto-installs.                        |
| `af config show \| init`                  | Print effective config or write defaults.                                                     |
| `af completions <shell>`                  | Emit shell completion script (bash, zsh, fish, powershell).                                   |

### 3.7 Notes & retro (ADR-047, ADR-058)

| Command                                                                          | Purpose                                                                                                   |
| -------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------- |
| `af note [session] [--append TEXT]`                                              | Open the workstream's Obsidian note. With `--append TEXT`, append a timestamped log entry under `## Log`. |
| `af retro [--since DURATION] [--tag TAG]... [--search QUERY] [--ai] [--limit N]` | Mine archived workstream notes for patterns. `--ai` summarises via the configured agent.                  |

### 3.8 Proxy commands (config-driven, thin wrappers)

| Command                                                                 | Default behaviour                                                                                                         | Config knob                            |
| ----------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------- | -------------------------------------- |
| `af editor [--terminal\|--visual]`                                      | `$EDITOR` in a tmux split, or `code .` / `zed .` for visual.                                                              | `[editor].terminal`, `[editor].visual` |
| `af diff [session] [--base <ref>]`                                      | `git diff <base_branch>...HEAD` in the workstream's worktree, paged.                                                      | `[diff].cmd`                           |
| `af pr [session] [--title <t>] [--draft] [--web] [--ai] [--ai-model M]` | `gh pr create --base <base_branch> --head <branch>`. With `--ai`, the body is authored by the configured agent (ADR-057). | `[pr].cmd`                             |

### 3.9 Secrets (ADR-049)

| Command               | Purpose                                               |
| --------------------- | ----------------------------------------------------- |
| `af auth set <key>`   | Prompt for value (echo-off) and store in keyring.     |
| `af auth get <key>`   | Print value to stdout (TTY only; redacted otherwise). |
| `af auth status`      | List known keys with availability + source.           |
| `af auth clear <key>` | Remove from keyring.                                  |
| `af auth list`        | List names of all `af`-stored keys (no values).       |

### 3.10 Meta

| Command      | Purpose                                                |
| ------------ | ------------------------------------------------------ |
| `af version` | Print version, commit, build date.                     |
| `af --help`  | Top-level help. Subcommand help via `af <cmd> --help`. |

ADR-035 is the **authoritative CLI contract**; this section is kept consistent with it. When a per-command ADR adds or changes a flag, both ADR-035 and this section are updated in the same commit.

---

## 4. Workstream identifiers

### 4.1 Names

- User-supplied via `af create <name>`, or auto-generated as `<repo>-<YYYYMMDD-HHMMSS>`.
- Sanitized for tmux: `/`, `.`, `:` → `--`. Example: `kakkoyun/issue-42` → `kakkoyun--issue-42`.
- Branch prefix: when the repo has an `upstream` remote, `<name>` becomes `<config.branch.prefix>/<name>` before sanitization (config-driven; see ADR-038).

### 4.2 Session IDs

- The **slot identity** `(repo_name, branch_name, slot_name)` is stable across machines and reboots.
- Each agent **launch** within a slot mints a new UUID v5: `uuid5(NAMESPACE_DNS, "{repo}/{branch}/{slot}/{launch-timestamp-ns}")`. Resumes within a slot append to `state.toml`'s `session_ids[]`.
- Some agents accept the session ID via flag (claude `--session-id <uuid>`); others (pi, codex) use their native resume mechanism (`pi --continue`, `codex resume --last`) and the session ID is recorded for `af`'s tracking only. See ADR-039.

### 4.3 Worktree path

- Stable: `~/Workspace/.worktrees/<repo>/<branch>/`. Configurable via `[general].worktree_root`.
- Sub-worktrees for subagents: `~/Workspace/.worktrees/<repo>/<branch>--<slot>/` on branch `<branch>--<slot>` forked from `<branch>`. (See ADR-038.)

---

## 5. State files

### 5.1 Layout

```
~/.local/share/af/v1/
├── sessions/
│   └── <session>/
│       ├── state.toml           # Live workstream state
│       └── ledger.jsonl         # Append-only event log
├── archive/
│   └── <session>/               # Moved here by `af done`; retained per [lifecycle].retention_days
└── secrets/                     # Optional tmpfs envelope staging (see ADR-049)

<repo>/.af/
└── state.toml -> symlink to ~/.local/share/af/v1/sessions/<session>/state.toml
```

`<repo>/.af/` is added to the user's global `.gitignore` (`~/.config/git/ignore`)
by `af setup`.

### 5.2 `state.toml` schema (v1, schema_version = 1)

Full schema is defined in ADR-037. Top-level shape:

```toml
schema_version = 1

[session]
name         = "kakkoyun--issue-42"
id           = "<uuid v5>"
created_at   = 2026-05-06T12:00:00Z
status       = "active"       # active | suspended | completed | abandoned
suspended_at = null           # set when status = "suspended"

[worktree]
path        = "/Users/kemal/Workspace/.worktrees/af/kakkoyun--issue-42"
branch      = "kakkoyun/issue-42"
base_branch = "upstream/main"
git_root    = "/Users/kemal/Workspace/Projects/Personal/af"

[execution]
mode             = "local"    # local | bare | remote | sandbox
multiplexer      = "tmux"
tmux_session     = "kakkoyun--issue-42"
ssh_host         = ""         # populated for remote mode
remote_path      = ""
sandbox_provider = ""         # "" | "slicer" | "sbx"
sandbox_id       = ""

[[agents]]
slot            = "primary"
provider        = "pi"
session_ids     = ["<uuid v5>"]   # all session IDs ever associated with this slot
pane            = "%0"
status          = "running"   # running | stopped | crashed | suspended
sub_worktree    = ""          # absolute path to sibling sub-worktree, if any
sub_branch      = ""          # branch name of the sub-worktree
created_at      = 2026-05-06T12:00:00Z
last_resumed_at = null        # null until first resume

[pr]
number = 0
url    = ""
state  = ""

[versions]
af             = "1.0.0"
agent_versions = { pi = "...", claude = "..." }
```

### 5.3 `ledger.jsonl` events

One JSON object per line. Event types defined in ADR-037. Examples:
`session_created`, `agent_launched`, `agent_added`, `agent_stopped`,
`agent_crashed`, `session_suspended`, `session_resumed`,
`session_completed`, `session_abandoned`, `pr_opened`, `pr_merged`,
`pr_closed`, `error`.

Every agent-scoped event carries `slot`, `agent`, and (where relevant)
`session_id` keys.

---

## 6. Configuration

### 6.1 Files

| Path                       | Purpose                                                         |
| -------------------------- | --------------------------------------------------------------- |
| Compiled defaults          | Built into the binary.                                          |
| `~/.config/af/config.toml` | User-level (vaults, default agent, prefix, lifecycle, secrets). |
| `<repo>/.af/config.toml`   | Per-repo overrides (project-specific defaults).                 |

Merge order: defaults → user → repo. Last writer wins per field.

### 6.2 Schema

Full schema in ADR-036. Sections:

- `[general]` — `default_agent`, `multiplexer`, `max_sessions`, `worktree_root`.
- `[branch]` — `prefix`, `prefix_on_fork_only`.
- `[editor]` — `terminal`, `visual`.
- `[diff]` — `shell` (bool, default false), `cmd` (argv array or string per `shell`). Argv default: `["git", "diff", "{base}...HEAD"]`.
- `[pr]` — `shell` (bool, default false), `cmd` (argv or string), `flag_template` (table mapping `--title`/`--draft`/`--web` to argv fragments), `template`, `ai_model` (default model for `--ai`; `""` = agent default).
- `[status]` — `max_parallel` (default 8; cap on concurrent `gh pr view` fetches).
- `[remote]` — `default_host`, `ssh_options`.
- `[sandbox]` — `default_provider`, `slicer.*`, `sbx.*`.
- `[obsidian]` — `notes_vault` (key from `[obsidian.vaults]`), `notes_folder`, `notes_template`.
- `[obsidian.vaults]` — **global only**; map of vault-name → absolute path on this machine.
- `[doctor]` — `extra_tools`.
- `[secret]` — `keyring_service`.
- `[lifecycle]` — `retention_days`, `auto_archive`.

`[obsidian.vaults]` lives **only** in the user-level config because
vault paths are a per-machine concern unrelated to any project.

---

## 7. Agent providers

Three providers in v1, all behind a single `internal/agent.Agent`
interface. Defined in ADR-043.

| Agent    | Binary   | Default? | Resume flag                               | Yolo flag                        |
| -------- | -------- | -------- | ----------------------------------------- | -------------------------------- |
| `pi`     | `pi`     | ✅       | `--continue`                              | (TBD per agent's CLI)            |
| `claude` | `claude` |          | `--continue` (with `--session-id <uuid>`) | `--dangerously-skip-permissions` |
| `codex`  | `codex`  |          | `resume --last`                           | `--full-auto`                    |

Each provider exposes:

- `LaunchCmd(LaunchOpts) []string`
- `ResumeCmd(ResumeOpts) []string`
- `IsAvailable() bool`
- `SessionLogPaths(sessionID, projectPath) []string` — for analysis only; `af` never deletes agent log files.

---

## 8. Multiplexer

tmux only. Defined in ADR-040. Single `internal/mux.Multiplexer`
interface; one `Tmux` impl. Operations: create/kill session, attach,
send-keys, set/get session env, set option, list sessions, split pane,
list/kill panes.

---

## 9. Remote

SSH only. Defined in ADR-041. The "remote" is whatever string the user
passes to `--remote`: an alias from `~/.ssh/config`, or `user@host`, or
an IP. `af` does not validate or special-case it. Connection is
established via `ssh <host> <command>`; tmux is launched on the remote
to keep the session alive across drops.

There is **no** plugin layer. exe.dev, DD Workspaces, or any other VM
provider is provisioned externally by the user; `af` only consumes the
SSH host name.

---

## 10. Sandbox

Two providers behind a single `internal/sandbox.Sandbox` interface.
Defined in ADR-042.

| Provider | Binary   | Backend             | Local | Remote                        |
| -------- | -------- | ------------------- | ----- | ----------------------------- |
| `slicer` | `slicer` | Firecracker microVM | ✅    | ✅ (composes with `--remote`) |
| `sbx`    | `sbx`    | Docker AI Sandboxes | ✅    | ✅                            |

Composition: `af create --remote <host> --sandbox slicer` runs the
slicer daemon on the remote, builds a VM there, and launches the agent
inside.

---

## 11. Secrets

Defined in ADR-049.

- **Storage**: `zalando/go-keyring` (macOS Keychain, Linux Secret Service).
- **Service name**: `af` (no `af/` prefix on accounts).
- **Transport to sandbox / remote**: ephemeral envelope file. Never SSH `SetEnv`/`SendEnv`.
- **Storage location** (selected at runtime):
  - Linux with `/run/user/$UID/` writable: `/run/user/$UID/af-<session>/.env` (tmpfs).
  - macOS, or Linux without `/run/user/$UID/`: `~/.local/share/af/v1/secrets/af-<session>/.env` (persistent disk).
- **Transport mechanics** (no TBD; the launch wrapper is one shell snippet):
  1. `af` writes the envelope at the path above with `chmod 600`.
  2. Mount/copy/`scp` into the sandbox or remote as required.
  3. The launch wrapper runs `. <path>/.env && rm -f <path>/.env && exec <agent-cmd>`. The delete is non-optional — the wrapper will not exec the agent without first removing the envelope.
  4. Stray envelopes from crashes are reaped by `af setup`'s periodic cleanup hook (60-min stale-file sweep).
- **Redaction**: `slog` handlers redact known secret-bearing keys plus any user-listed `[secret].redact_keys`.
- **Threat model boundary**: see ADR-049 §"Threat model". Crash-window leakage on persistent-disk fallback is acknowledged and capped by the 60-minute sweep; users who need a hard guarantee run exclusively on Linux tmpfs.

---

## 12. Obsidian integration

Defined in ADR-047.

- `af create` writes `<vault>/<folder>/<session>.md` with frontmatter:
  ```yaml
  ---
  af_schema: 1
  af_session: kakkoyun--issue-42
  af_repo: af
  af_branch: kakkoyun/issue-42
  af_status: active
  af_agents: [pi]
  af_started_at: 2026-05-06T12:00:00Z
  af_completed_at: null
  ---
  ```
- `af done` updates `af_status` to `completed` (or `abandoned`) and
  sets `af_completed_at`.
- `af suspend` sets `af_status: suspended`. `af resume` reverts to
  `active`.
- `af note [session]` opens the file via Obsidian URI scheme or
  `$EDITOR`.
- An example Obsidian Bases definition ships at
  `examples/obsidian/active-workstreams.base` for users who want a
  live aggregator across active workstreams.

---

## 13. Doctor + Setup

| Command                     | Scope                                                                                                                                                                                                                          | Auto-install?                                                  |
| --------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------- |
| `af doctor`                 | Probe local tools (`tmux`, `git`, `pi`, `claude`, `codex`, `gh`, `slicer`, `sbx`, `fzf`); print install commands.                                                                                                              | **No.**                                                        |
| `af doctor --remote <host>` | Same probe over SSH; print install commands for the remote's package manager.                                                                                                                                                  | **No.**                                                        |
| `af setup`                  | Idempotent user-scope setup: add `.af/` to `~/.config/git/ignore`; install shell completions for the detected shell; create `~/.local/share/af/v1/` tree; run `af config init` if no config exists; print Obsidian vault hint. | Local user files only. **No** `sudo`, **no** package installs. |

Per-platform install hints in `af doctor` output:

- macOS: `brew install <pkg>` for tools available via Homebrew.
- Arch: `pacman -S <pkg>`.
- Debian/Ubuntu: `apt install <pkg>`.
- Tools without distro packages (e.g. `pi`, `slicer`): print upstream install instructions.

---

## 14. Build & distribution

Defined in ADR-053.

- Build tool: `Make` (`Makefile` at repo root).
- Cross-compile targets: `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`.
- Release tool: `goreleaser` for local cross-compile only. No GitHub Releases for v1.
- Distribution: `go install github.com/kakkoyun/af@latest` or `make install`.
- No Homebrew tap.

---

## 15. Out of scope for v1

- DD Workspaces remote provider, exe.dev special-casing.
- Zellij / Ghostty / cmux multiplexers.
- Skill bundle installer (v0 ADR-030).
- Auto-install in doctor.
- `af log`, workspace templates, Dataview dashboards.
- gemini, amp, copilot agents.
- mdBook user guide.
- Migration from v0 state files (`af migrate`).
- Releases, changelogs cross-signed against tags, Homebrew taps.

These are listed in `TODO.md` Backlog. They may return as ADRs in a
later iteration; they do not block v1.

---

## 16. References

- [`docs/adr/`](adr/) — v1 ADRs 031–059.
- [`docs/v0/SPEC.md`](v0/SPEC.md) — v0 (Rust era) spec, immutable.
- [`docs/v0/adr/`](v0/adr/) — 30 v0 ADRs, frozen.
