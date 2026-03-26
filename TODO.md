# TODO

Tracked implementation tasks. Check off as completed. See [docs/PLAN.md](docs/PLAN.md) for
detailed phasing and [docs/adr/](docs/adr/) for architecture decisions.

---

## Phase 0 — Foundation

- [x] Project scaffold (Cargo.toml, CI, lints, release workflow)
- [x] Specification document (docs/SPEC.md)
- [x] Architecture Decision Records (docs/adr/)
- [x] Implementation plan (docs/PLAN.md)
- [x] Core types: `SessionId`, `SessionName`, `BranchName`, `WorktreePath`
- [x] UUID v5 generation (replace Python dependency)
- [x] Session name sanitization (`/.:` → `--`)
- [x] Configuration system: load + merge (user → project → env → CLI)
- [x] Session state store: TOML read/write/list/delete (`state.toml`)
- [x] Session event ledger: JSONL append/read (`ledger.jsonl`)
- [x] Ledger event types: created, agent_launched, resumed, completed, error
- [x] Version pinning: record af version + agent config hash at session creation
- [x] Git helpers: worktree create/remove, branch create/delete
- [x] Git helpers: main branch detection (main/master/trunk)
- [x] Git helpers: org detection from remote URL (SSH + HTTPS)
- [x] Git helpers: fetch + resolve base branch (upstream preferred)
- [x] Branch prefix logic (fork detection via `upstream` remote)
- [x] Platform detection (macOS, Arch, Debian)
- [x] Package manager abstraction (brew, pacman, apt)
- [x] Dependency table with tier system (Must/Should/Nice)
- [x] Multiplexer trait definition
- [x] tmux implementation (create/kill/attach/env/send-keys)
- [x] Agent provider trait definition
- [x] Claude Code agent provider (launch/resume/pr commands)

## Phase 1 — Local MVP

- [x] `af create [task-name]` — local worktree mode
- [x] `af create --from <branch>` — fork from specific branch
- [x] `af create --current` — fork from current branch
- [ ] `af create --from-pr <number>` — PR worktree (needs `gh`) *(deferred: requires gh integration)*
- [x] `af create --bare` — bare mode (no VM, host worktree)
- [x] `af create` — workspace mode (non-git directory)
- [x] `af create --agent <name>` — agent selection
- [x] Session limit guard (`max_sessions` config)
- [x] `af done [session]` — teardown with confirmation
- [x] `af done --force` — skip confirmation, force-delete unmerged
- [x] `af list` — grouped by repo, current repo first
- [x] `af resume [session]` — re-attach to session
- [x] `af resume` (no args) — fzf picker (if available)
- [ ] `af resume --bare` — resume in bare mode *(flag accepted, logic pending)*
- [x] `af session-branch` — branch-tied session ID
- [x] Ledger: emit events from create/done/resume commands
- [x] Agent session ID tracking in state.toml
- [x] `af doctor` — pre-flight dependency check
- [ ] `af doctor --fix` — auto-install missing dependencies *(placeholder, Phase 2)*
- [x] Integration tests: CLI help, flag conflicts, empty list

## Phase 2 — Multi-Agent + Config

- [x] Pi agent provider
- [x] Codex agent provider
- [x] Gemini agent provider
- [x] Amp agent provider
- [x] Multi-agent slot model in state.toml (primary + named slots) *(schema done in Phase 0)*
- [ ] `af agent add --slot <name> --agent <provider>` — add agent to workstream pane
- [ ] `af agent stop <slot>` — stop an agent in a slot
- [ ] `af agent list` — show agents in current workstream
- [ ] Multi-agent resume: restore all agent panes on `af resume`
- [ ] Multi-agent teardown: stop all agents on `af done`
- [x] `af config show` — dump effective configuration
- [x] `af config init` — create default config file
- [x] Shell completions: bash, zsh, fish
- [x] Agent availability check + fallback chain

## Phase 3 — Remote Providers

- [ ] Remote provider trait definition
- [ ] DD Workspaces provider (detect, create, teardown)
- [ ] exe.dev provider (detect, create, setup, teardown)
- [ ] `af create --remote [host]` — remote session
- [ ] `af create --yolo` — unattended mode
- [ ] SSH bootstrap pipeline (embedded default scripts)
- [ ] Dotfiles provisioning config (repo + install_cmd)
- [ ] Remote provisioning pipeline: bootstrap → dotfiles → auth
- [ ] Remote session resume (SSH drop detection + reconnect)
- [ ] Orphan detection in `af list`
- [ ] `af done` for remote sessions

## Phase 4 — Sandbox + Obsidian

- [ ] Sandbox provider trait definition
- [ ] Slicer sandbox provider (local)
- [ ] Slicer sandbox provider (remote: `--sandbox --remote`)
- [ ] VirtioFS path mapping
- [ ] VM health check + respawn in `af resume --respawn`
- [ ] `af auth setup/reroll/status/clear`
- [ ] Auth token injection (keychain/keyring/file)
- [ ] Obsidian note creation on `af create`
- [ ] `af note [session]` — open Obsidian note
- [ ] Frontmatter update on `af done` (status → completed)

## Phase 5 — GC + Editor + Polish

- [ ] `af gc` — list merged/closed worktrees
- [ ] `af gc --dry-run` — preview without action
- [ ] `af gc --all` — clean all without prompts
- [ ] Merge detection: GitHub PR state (via `gh`)
- [ ] Merge detection: git ancestry (`merge-base --is-ancestor`)
- [ ] Merge detection: squash-merge fingerprint (diff cksum)
- [ ] `af editor --terminal` — split pane with `$EDITOR`
- [ ] `af editor --visual` — VS Code/Zed GUI
- [ ] `af editor` for remote sessions (SSH + URL schemes)
- [ ] Session archival: move to archive/ on `af done`, retain for 90 days
- [ ] PR tracking: detect/record PR number+URL from branch
- [ ] Ledger events: pr_opened, pr_merged, pr_closed
- [ ] Agent session log discovery (claude, pi file path conventions)
- [ ] `af gc` prunes expired archives (older than retention_days)
- [ ] Migration: read `cf-sessions/*.env` → convert to TOML
- [ ] Man page generation
- [ ] Comprehensive `--help` text for all commands
- [ ] Error messages with actionable suggestions
- [ ] CHANGELOG.md (Keep a Changelog format)
- [ ] User guide (mdBook or similar, deployed to GitHub Pages)
- [ ] README.md final polish — remove all 🔜, all examples work

---

## Backlog (unscheduled)

- [ ] Zellij multiplexer implementation
- [ ] Docker-based sandbox provider
- [ ] `af log` — append to session log (Obsidian note)
- [ ] `af pr` — create PR from session branch
- [ ] `af sync` — sync remote sandbox with local worktree
- [ ] Dataview dashboard template for Obsidian
- [ ] `af doctor --verbose` — detailed version/path info for debugging
- [ ] `af stats` — workstream analytics from ledger data (duration, agent usage, etc.)
- [ ] `af export` — export ledger data for external analysis
- [ ] Workspace template support (pre-configured sessions per project)
