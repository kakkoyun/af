# af — Specification

> **af** (agentic-flow / automatic-flow / as-fuck) — a Rust CLI that manages isolated
> development sessions for AI coding agents.
>
> Reverse-engineered from the `cf` (Claude Focus) implementation in
> [kakkoyun/dotfiles](https://github.com/kakkoyun/dotfiles) (`macos/.zsh/aliases/claude.zsh`
> and supporting scripts/plugins). This document captures the complete behaviour of `cf` as
> a specification for the Rust rewrite.
>
> See also: [PLAN.md](PLAN.md) for phased delivery, [adr/](adr/) for architecture decisions.

---

## 1. Overview

`af` creates **isolated development sessions** using:

- **Git worktrees** — each session gets a dedicated branch + working directory
- **Terminal multiplexer** — each session is a named multiplexer session (tmux now, zellij later — see [ADR-002](adr/002-multiplexer-abstraction.md))
- **AI coding agent** — any supported agent is launched with a deterministic session ID (Claude Code, pi, Codex, Gemini, Amp — see [ADR-001](adr/001-agent-provider.md))

The tool supports multiple execution environments:

| Mode | Description | Flag |
|---|---|---|
| **Local** | Git worktree on host filesystem, Claude runs locally | *(default)* |
| **Workspace** | Non-git directory, Claude runs locally | *(auto: when `pwd` is not a git repo)* |
| **Remote** | Claude runs on a remote VM (DD Workspaces, exe.dev) | `--remote [host]` |
| **Sandbox** | Claude runs inside a Firecracker microVM (slicer) | `--sandbox` |
| **Sandbox-Remote** | Slicer VM on a remote host | `--sandbox --remote <host>` |
| **Bare** | Claude runs locally on the host worktree (review/PR mode) | `--bare` |

---

## 2. Command Surface

### 2.1 `cf` — Create a Session

```
cf [options] [task-name]
```

#### Flags

| Flag | Description | Constraints |
|---|---|---|
| `--from <branch>` | Fork from a specific branch instead of the default (main/master/trunk) | |
| `--from-pr <number>` | Create worktree from a GitHub PR (resolves head branch + base) | Incompatible with `--remote` |
| `--current` | Fork from the current branch | Requires a named branch (not detached HEAD) |
| `--remote [host]` | Create a remote workspace session | Optional host arg for sandbox-remote |
| `--sandbox` | Create a Firecracker VM sandbox via slicer | Mutually exclusive with `--bare` |
| `--bare` | Run Claude locally on a host worktree (review mode) | Mutually exclusive with `--sandbox` |
| `--yolo` | Skip all Claude permission prompts (`--dangerously-skip-permissions`) | Requires `--remote` or `--sandbox` |

#### Session Naming

1. **Explicit:** `cf my-task` → branch name = `my-task`
2. **Auto-generated:** `cf` → `<repo>-<YYYYMMDD-HHMMSS>`
3. **From branch:** `cf --from feature-x` (no task name) → reuses `feature-x` as name if the local branch exists (`branch_pinned=true`)
4. **From PR:** `cf --from-pr 42` → uses the PR's `headRefName` as the task name
5. **Remote auto-name:** `<repo>-r<hex-timestamp>` (hex avoids exe.dev name restrictions)

#### Branch Prefix Logic

When the repo has an `upstream` remote (i.e., is a fork), branches are prefixed with `kakkoyun/`:

- `cf my-task` → `kakkoyun/my-task`

Prefix is **skipped** when:

- `branch_pinned=true` (name defaulted to an existing local branch via `--from`)
- Name already starts with `kakkoyun/`
- No `upstream` remote exists

#### Session Name Sanitization

tmux session names cannot contain `/`, `.`, `:`. These are replaced with `--`:

- `kakkoyun/issue-42` → `kakkoyun--issue-42`
- `v1.2.3:hotfix` → `v1--2--3--hotfix`

#### Session Limit Guard

Maximum concurrent cf sessions: `CF_MAX_SESSIONS` (default: 10). Only counts sessions with the `@CF_SESSION` tmux option set.

#### Base Branch Resolution

1. `--current` → `git branch --show-current`
2. `--from <branch>` → literal value
3. Default → `_cf_fetch_and_resolve_base()`:
   - Prefers `upstream` remote, falls back to `origin`
   - Fetches remote, resolves `<remote>/<main_branch>`
   - Main branch detection: tries `main`, `master`, `trunk` in order

#### Worktree Creation

- Location: `~/Workspace/.worktrees/<repo>/<branch_name>/`
- If local branch exists: `git worktree add <path> <branch>`
- If new: `git worktree add -b <branch> <path> <base_branch>`

#### Session ID

Deterministic UUID v5: `uuid5(NAMESPACE_DNS, "<repo>/<branch>")` — ensures resuming a session uses the same Claude session.

#### Claude Launch

| Condition | Command |
|---|---|
| Normal | `claude --session-id <uuid5>` |
| `--from-pr` | `claude --from-pr <number>` |
| `--yolo` | `claude --dangerously-skip-permissions --session-id <uuid5>` |
| `--yolo --from-pr` | `claude --dangerously-skip-permissions --from-pr <number>` |

#### tmux Attach Behaviour

- Inside tmux: `tmux switch-client -t <session>`
- Outside tmux: `tmux attach-session -t <session>`

---

### 2.2 `cfd` — Tear Down a Session

```
cfd [--force] [--vm] [session_name]
```

Without arguments, tears down the **current tmux session**.

| Flag | Description |
|---|---|
| `--force` | Force-delete even if branch is unmerged; skip confirmation prompt |
| `--vm` | Tear down the VM only, keep worktree and tmux session (sandbox only) |

#### Teardown Behaviour by Session Type

| Type | Actions |
|---|---|
| **Local** | Kill tmux → remove worktree → delete branch → fetch remote → show remaining worktrees |
| **Workspace** | Kill tmux → preserve directory |
| **Remote (workspaces)** | Kill tmux → `workspaces delete <name>` |
| **Remote (exe.dev)** | Kill tmux → `ssh exe.dev rm <name>` |
| **Sandbox** | Kill tmux → delete slicer VM → remove worktree → delete branch |
| **Sandbox (`--vm`)** | Delete VM only → set `CF_VM_MODE=bare` → print resume instructions |

#### Confirmation Prompt

All teardowns show a confirmation prompt listing what will be destroyed. Skipped with `--force`.

#### Metadata Fallback

If `CF_*` tmux env vars are missing, `cfd` attempts reconstruction from convention:

- Infers worktree path from `~/Workspace/.worktrees/<repo>/<session_name>/`
- Scans slicer VMs for tag `cf-session=<session_name>` to find orphaned VMs

---

### 2.3 `cfl` — List Active Sessions

```
cfl
```

No flags. Output is grouped by repository, sorted (current repo first, then alphabetical).

#### Output Format

```
── repo-name (current) ────────────────────────
  Local:
    session-name              branch=branch-name              /path/to/worktree
  Remote:
    session-name              branch=branch-name              [remote:hostname]
── other-repo ─────────────────────────────────
  Local:
    ...
── workspaces (no cf session) ─────────────────
    orphaned-workspace-name   ...
── exe.dev (no cf session) ────────────────────
    orphaned-vm-name          [exe.dev]
── slicer (no cf session) ─────────────────────
    orphaned-vm-name          [slicer]
```

#### Orphan Detection

For each remote provider (workspaces, exe.dev, slicer), sessions that exist remotely but have no matching tmux session are listed as orphans.

#### Metadata Recovery

Attempts `_cf_metadata_restore` from disk cache (`~/.local/share/cf-sessions/*.env`) before reading tmux env vars.

---

### 2.4 `cfr` — Resume a Session

```
cfr [--bare] [--respawn] [session_name]
```

| Flag | Description |
|---|---|
| `--bare` | Resume in bare mode (skip VM, run Claude locally) |
| `--respawn` | Respawn a dead slicer VM instead of falling back to bare |

Without a session name, shows an **fzf picker** of active sessions (+ orphaned worktrees).

#### Recovery Behaviour

| Scenario | Action |
|---|---|
| tmux session exists | Attach/switch to it |
| tmux session gone, worktree exists | Recreate tmux session, restore metadata from disk, `claude --continue` |
| Remote SSH dropped | Detect via `pane_current_command != ssh*`, reconnect with exponential backoff |
| Slicer VM healthy | Reconnect via `slicer vm shell` |
| Slicer VM dead + `--respawn` | Create new VM, provision, reconnect |
| Slicer VM dead (no `--respawn`) | Fall back to bare mode, warn user |

---

### 2.5 `cfgc` — Garbage Collect

```
cfgc [--dry-run] [--all]
```

| Flag | Description |
|---|---|
| `--dry-run` | List candidates without touching anything |
| `--all` | Clean all merged/closed without per-session prompts |

#### Merge Detection (Priority Order)

1. **GitHub PR state** via `gh pr view <branch> --json state`:
   - `MERGED` → merged
   - `CLOSED` → closed
   - `OPEN` → open
2. **Git ancestry:** `git merge-base --is-ancestor <branch> <main>` → merged
3. **Squash-merge heuristic:** Compare combined diff fingerprint (cksum) of the feature branch against each non-merge commit on main (up to 100 commits). If a match is found → merged.
4. **Fallback:** open

#### Cleanup Actions

For `merged` and `closed` branches:

- Kill associated tmux session (if any)
- `git worktree remove --force`
- `git branch -D`
- Remove empty parent directory
- Clean orphan metadata files from `~/.local/share/cf-sessions/`

---

### 2.6 `cf-open-editor` — Open Codebase in Editor

```
cf-open-editor [--terminal|-t | --visual|-v] [session_name]
```

| Flag | Description |
|---|---|
| `--terminal`, `-t` | (default) Open `$EDITOR` in a new vertical tmux split |
| `--visual`, `-v` | Open VS Code or Zed GUI editor |

#### Session Type Handling

| Type | `--terminal` | `--visual` |
|---|---|---|
| Local / Bare | `tmux split-window -h` + `$EDITOR .` | `code`/`zed $path` |
| Sandbox-local | Same (VirtioFS maps path) | Same |
| Sandbox-remote | **Error:** VSOCK barrier | **Error:** VSOCK barrier |
| Remote | SSH + `$EDITOR .` in remote path | VS Code/Zed URL scheme |

#### Visual Editor Detection

1. `$CF_VISUAL_EDITOR` env var
2. `code` in `$PATH` (VS Code)
3. `zed` in `$PATH`

#### Remote URL Schemes

```
VS Code:  vscode://vscode-remote/ssh-remote+<host><abs_path>?windowId=_blank
Zed:      zed://ssh/<host><abs_path>
```

#### Convenience Aliases

| Alias | Maps to |
|---|---|
| `cfe` | `cf-open-editor --visual` |
| `cfed` | `cf-open-editor --terminal` |

#### tmux Keybindings

| Binding | Action |
|---|---|
| `prefix + e` | Terminal editor split |
| `prefix + o` | Visual editor |

---

### 2.7 `csb` — Claude Session Branch

```
csb
```

Launch Claude with a session ID tied to the current git branch (not a worktree session). UUID v5 of the branch name.

---

### 2.8 `cfauth` — Claude Auth Management (Sandbox)

```
cfauth [setup|reroll|status|clear]
```

Manages long-lived Claude API tokens for sandbox VMs via macOS Keychain.

| Subcommand | Description |
|---|---|
| `setup` | Prompt for token, store in Keychain |
| `reroll` | Delete old token, prompt for new one |
| `status` | Show token source (env → keychain → file → none) |
| `clear` | Remove from Keychain |

#### Token Lookup Order (for VM injection)

1. `CF_CLAUDE_API_KEY` env var
2. macOS Keychain (service: `cf-claude-token`)
3. `~/.config/cf/claude-token` file (must be mode 600)

---

## 3. Session Metadata

### 3.1 tmux Environment Variables

Stored per-session via `tmux set-environment`:

| Variable | Description | Set by |
|---|---|---|
| `CF_WORKTREE_PATH` | Absolute path (or `remote:<host>`) | All modes |
| `CF_BRANCH_NAME` | Git branch name | Git modes |
| `CF_GIT_ROOT` | Repo root path | Git modes |
| `CF_SESSION_ID` | UUID v5 of `<repo>/<branch>` | All modes |
| `CF_BASE_BRANCH` | Branch this session forked from | Git modes |
| `CF_VM_MODE` | `sandbox` or `bare` | Sandbox/bare |
| `CF_VM_NAME` | Slicer VM hostname | Sandbox |
| `CF_REMOTE_PROVIDER` | `workspaces`, `exedev`, or `slicer` | Remote/sandbox |
| `CF_REMOTE_HOST` | SSH host (or `local`) | Remote/sandbox |
| `CF_REMOTE_NAME` | Remote workspace name | Remote |
| `CF_REMOTE_WORK_DIR` | Absolute path on remote host | Remote/sandbox |
| `CF_YOLO_MODE` | `true` when `--yolo` | Remote/sandbox |
| `CF_WORKSPACE_MODE` | `1` for non-git workspace mode | Workspace |

### 3.2 Disk Persistence

Location: `~/.local/share/cf-sessions/<session-name>.env`

All `CF_*` tmux env vars are saved to disk after session creation. This survives tmux server restarts and reboots. On resume (`cfr`), metadata is re-injected from disk.

### 3.3 tmux Session Marker

`@CF_SESSION` option (value `1`) is set on all cf-created tmux sessions. Used by session-limit guard and listing.

---

## 4. Remote Provider Plugin System

### 4.1 Architecture

Providers are sourced as separate zsh files from the same directory as `claude.zsh`:

```
claude-dd-workspaces.zsh   # Datadog Workspaces
claude-exedev.zsh           # exe.dev
claude-slicer.zsh           # Firecracker/slicer
```

### 4.2 Provider Interface

Each provider must implement:

```
_cf_remote_provider_<name>_detect()
    → echo provider name if available; return 1 otherwise

_cf_remote_create_<name> <name> <repo> [branch]
    → echo ssh_host on success

_cf_remote_setup_<name> <ssh_host> <repo> [branch] [git_root]  (optional)
    → Post-bootstrap setup (e.g., cloning repo for exe.dev)
```

### 4.3 Provider Selection

`_cf_remote_provider()` probes providers in order:

1. `_cf_remote_provider_workspaces_detect` — org must be in `CF_WORKSPACES_ORGS`
2. `_cf_remote_provider_exedev_detect` — any org NOT in `CF_WORKSPACES_ORGS`

### 4.4 Provider: Datadog Workspaces

- **Detection:** Repo org in `CF_WORKSPACES_ORGS` (default: `DataDog ddoghq open-telemetry`) + `workspaces` CLI installed
- **Creation:** `workspaces create <name> -R <repo> -d kakkoyun/dotfiles -s zsh -y [-b <branch>]`
- **SSH host:** `workspace-<name>`
- **Teardown:** `workspaces delete <name>`

### 4.5 Provider: exe.dev

- **Detection:** Repo org NOT in `CF_WORKSPACES_ORGS` + `ssh exe.dev whoami` succeeds
- **Creation:** `ssh exe.dev new --name=<name> --json` → parse `ssh_dest`
- **Post-bootstrap:** Clone repo via HTTPS, checkout branch, provision dotfiles, forward gh auth
- **Teardown:** `ssh exe.dev rm <name>`

### 4.6 Provider: Slicer (Firecracker)

- **Detection:** `slicer` CLI in PATH
- **Local creation:**
  - Ensure `slicer-mac` daemon running
  - Ensure `slicer-mac.yaml` has `share_home: ~/Workspace/.worktrees/`
  - Select host group (fewest running VMs first, most VCPUs tiebreak; `CF_SLICER_GROUP` override)
  - `slicer vm add <group> --tag cf-session=<name>`
  - Discover hostname via before/after diff of `slicer vm list`
  - Wait for agent ready (60s timeout)
- **Remote creation:** Same via SSH to remote host
- **VM path mapping:** Host `~/Workspace/.worktrees/<repo>/<branch>/` → VM `/home/ubuntu/host/<repo>/<branch>/` (VirtioFS)
- **Provisioning:** Bootstrap (Node/tmux/Claude) + dotfiles + gh auth + Claude API key injection
- **Teardown:** `slicer vm delete <hostname>`
- **Remote token management:** 1Password-backed via `slicer-token store/refresh/clear`

---

## 5. Bootstrap & Provisioning

### 5.1 Remote Bootstrap (`cf-bootstrap-remote`)

Runs via SSH on remote VMs. Installs:

1. Node.js (via nodesource)
2. tmux
3. Claude Code (`npm install -g @anthropic-ai/claude-code`)
4. SSH agent forwarding fix (stable symlink at `~/.ssh/ssh_auth_sock`)
5. tmux mouse + bell config
6. Claude notification hook (`printf '\a'` on notification)
7. Ghostty terminfo deployment

### 5.2 Slicer Bootstrap (`cf-bootstrap-slicer`)

Same as remote bootstrap but without SSH agent forwarding shim.

### 5.3 Dotfiles Provisioning (`cf-provision-dotfiles`)

Runs after bootstrap. Tool tiers:

- **Must-have:** tmux, nvim, claude (bail if missing)
- **Should-have:** stow, gh (warn if missing)
- **Nice-to-have:** delta, diffnav, starship (silent fallback)

Actions:

1. Add GitHub SSH known host
2. Install neovim, stow, gh CLI
3. Clone dotfiles repo → `make install/shared`
4. Configure pager fallbacks if delta not available

---

## 6. Completions

Zsh tab-completion for all commands:

- `cf` — flags + task name + branch completion (for `--from`)
- `cfd` — flags + session name picker (with branch descriptions)
- `cfr` — flags + session name picker
- `cfgc` — flags only
- `cfauth` — subcommands: setup, reroll, status, clear

---

## 7. Environment Variables (User Config)

| Variable | Default | Description |
|---|---|---|
| `CF_MAX_SESSIONS` | `10` | Maximum concurrent cf sessions |
| `CF_VISUAL_EDITOR` | *(auto-detect)* | Override visual editor for `cf-open-editor` |
| `CF_WORKSPACES_ORGS` | `DataDog ddoghq open-telemetry` | Orgs that route to DD Workspaces |
| `CF_SLICER_GROUP` | *(auto: fewest VMs)* | Override slicer host group selection |
| `CF_CLAUDE_API_KEY` | *(none)* | Override Claude API token for sandbox injection |
| `EDITOR` | `nvim` | Terminal editor for `cf-open-editor --terminal` |

---

## 8. File Layout (Current Bash Implementation)

```
macos/.zsh/aliases/
  claude.zsh                   # Core: cf, cfd, cfl, cfr, cfgc, csb, cfauth + helpers
  claude-dd-workspaces.zsh     # Provider plugin: Datadog Workspaces
  claude-exedev.zsh            # Provider plugin: exe.dev
  claude-slicer.zsh            # Provider plugin: Firecracker/slicer

macos/.local/bin/
  cf-open-editor               # Editor integration (bash script)
  cf-restore-metadata          # tmux-resurrect post-restore hook
  claude-review                # Open changed files in nvim

script/
  cf-bootstrap-remote          # Bootstrap remote VMs
  cf-bootstrap-slicer          # Bootstrap slicer VMs
  cf-provision-dotfiles        # Full dev env provisioning
  test_cf_flags                # Flag parsing tests (57 tests)
  test_cf_gc                   # GC helper tests (28 tests)
```

---

## 9. Test Coverage (Existing)

### 9.1 `test_cf_flags` — 57 tests

Tests mirror the flag-parsing logic from `claude-focus()`:

| Area | Tests |
|---|---|
| `--current` / `--from` resolution | 6 |
| Workspace mode detection | 4 |
| `--from-pr` parsing | 3 |
| Session name sanitization | 3 |
| Branch prefix (double-prefix guard) | 2 |
| `--from` name defaulting | 4 |
| `--from` + prefix interaction | 3 |
| `--remote` flag parsing | 5 |
| Remote session name generation | 2 |
| Remote org detection (SSH + HTTPS) | 5 |
| `--yolo` flag parsing | 4 |
| Remote provider selection (workspaces) | 7 |
| exe.dev provider detection | 7 |
| `--sandbox` / `--bare` flag parsing | 12 |

### 9.2 `test_cf_gc` — 28 tests

Tests mirror GC helpers:

| Area | Tests |
|---|---|
| `_cf_main_branch` detection | 5 |
| `_cf_is_squash_merged` (diff fingerprint) | 3 |
| `_cf_merged_status` (PR state + git fallback) | 7 |
| `cfgc` option parsing | 5 |
| Regression guard (no bare `status` variable) | 1 |

---

## 10. Design Decisions for the Rust Rewrite

Detailed decisions are captured in [Architecture Decision Records](adr/):

| ADR | Decision |
|---|---|
| [001](adr/001-agent-provider.md) | Agent Provider trait — support Claude, pi, Codex, Gemini, Amp |
| [002](adr/002-multiplexer-abstraction.md) | Multiplexer trait — tmux now, zellij later |
| [003](adr/003-configuration-system.md) | Layered TOML config — user + project + env + CLI |
| [004](adr/004-remote-provider.md) | Remote Provider trait — workspaces, exe.dev, extensible |
| [005](adr/005-sandbox-provider.md) | Sandbox Provider trait — slicer, composable with remote |
| [006](adr/006-session-metadata.md) | Session metadata — TOML files as source of truth, mux env as cache |
| [007](adr/007-obsidian-integration.md) | Obsidian integration — per-workstream notes with frontmatter |
| [008](adr/008-phased-delivery.md) | Phased delivery — 6 phases, each producing a usable binary |
| [009](adr/009-provisioning.md) | Provisioning — dotfiles-as-config, bootstrap pipeline |
| [010](adr/010-platform-deps.md) | Platform-aware dependency management — macOS, Arch, Debian |
| [011](adr/011-workstream-lifecycle.md) | Workstream lifecycle — event ledger, agent session tracking, PR association, retention |

### 10.1 What to Keep

- **Core workflow:** worktree + multiplexer + agent session isolation
- **Deterministic session IDs** (UUID v5)
- **Branch prefix logic** for fork repos
- **Squash-merge detection** heuristic
- **Metadata persistence** to disk
- **Provider system** (compiled-in providers with trait boundaries)

### 10.2 What to Improve

- **Agent-agnostic:** Support 5+ AI agents, not just Claude
- **Multiplexer-agnostic:** tmux now, zellij experimentable later
- **Cross-platform:** Linux (Arch, Debian) + macOS as first-class citizens
- **Configuration:** Layered TOML config instead of scattered shell env vars
- **Error handling:** Typed errors with actionable user messages
- **Auth management:** Platform-native keyring (macOS Keychain, Linux secret-service)
- **Shell completions:** bash, zsh, fish via `clap_complete`
- **Testing:** Real unit tests, not mirrored shell test doubles
- **Performance:** Native UUID v5 (no Python subprocess)
- **Single binary:** All functionality in one `af` binary with subcommands
- **Obsidian integration:** Per-workstream documents for knowledge management

### 10.3 CLI Surface

```
af create [options] [task-name]     # cf
af done [options] [session-name]    # cfd
af list                             # cfl
af resume [options] [session-name]  # cfr
af gc [options]                     # cfgc
af editor [options] [session-name]  # cf-open-editor
af auth [setup|reroll|status|clear] # cfauth
af session-branch                   # csb
af config [show|init]               # new: config management
af doctor [--fix] [--yes]           # new: dependency check + install
af note [session]                   # new: Obsidian integration
```

### 10.4 External Dependencies

| Dependency | Required | Purpose |
|---|---|---|
| `git` | Yes | Worktree management, branch ops |
| `tmux` or `zellij` | Yes (one) | Session management |
| Any AI agent | Yes (one) | Claude, pi, Codex, Gemini, or Amp |
| `gh` | Optional | PR state queries, auth forwarding |
| `fzf` | Optional | Session picker in `resume` |
| `slicer` | Optional | Firecracker sandbox support |
| `workspaces` | Optional | DD Workspaces support |

---

## 11. Appendix: Org Detection

```
SSH:    git@github.com:ORG/repo.git  →  ORG
HTTPS:  https://github.com/ORG/repo  →  ORG
```

Strip `.git` suffix first, then:

- SSH: split on `:`, take path, split on `/`, take first component
- HTTPS: strip last path component, take last component of remainder
