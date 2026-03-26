# TODO

Tracked implementation tasks. Check off as completed. See [docs/PLAN.md](docs/PLAN.md) for
detailed phasing and [docs/adr/](docs/adr/) for architecture decisions.

---

## Phase 0 — Foundation

- [x] Project scaffold (Cargo.toml, CI, lints, release workflow)
- [x] Specification document (docs/SPEC.md)
- [x] Architecture Decision Records (docs/adr/)
- [x] Implementation plan (docs/PLAN.md)
- [ ] Core types: `SessionId`, `SessionName`, `BranchName`, `WorktreePath`
- [ ] UUID v5 generation (replace Python dependency)
- [ ] Session name sanitization (`/.:` → `--`)
- [ ] Configuration system: load + merge (user → project → env → CLI)
- [ ] Session metadata store: TOML read/write/list/delete
- [ ] Git helpers: worktree create/remove, branch create/delete
- [ ] Git helpers: main branch detection (main/master/trunk)
- [ ] Git helpers: org detection from remote URL (SSH + HTTPS)
- [ ] Git helpers: fetch + resolve base branch (upstream preferred)
- [ ] Branch prefix logic (fork detection via `upstream` remote)
- [ ] Platform detection (macOS, Arch, Debian)
- [ ] Package manager abstraction (brew, pacman, apt)
- [ ] Dependency table with tier system (Must/Should/Nice)
- [ ] Multiplexer trait definition
- [ ] tmux implementation (create/kill/attach/env/send-keys)
- [ ] Agent provider trait definition
- [ ] Claude Code agent provider (launch/resume/pr commands)

## Phase 1 — Local MVP

- [ ] `af create [task-name]` — local worktree mode
- [ ] `af create --from <branch>` — fork from specific branch
- [ ] `af create --current` — fork from current branch
- [ ] `af create --from-pr <number>` — PR worktree (needs `gh`)
- [ ] `af create --bare` — bare mode (no VM, host worktree)
- [ ] `af create` — workspace mode (non-git directory)
- [ ] `af create --agent <name>` — agent selection
- [ ] Session limit guard (`max_sessions` config)
- [ ] `af done [session]` — teardown with confirmation
- [ ] `af done --force` — skip confirmation, force-delete unmerged
- [ ] `af list` — grouped by repo, current repo first
- [ ] `af resume [session]` — re-attach to session
- [ ] `af resume` (no args) — fzf picker (if available)
- [ ] `af resume --bare` — resume in bare mode
- [ ] `af session-branch` — branch-tied session ID
- [ ] `af doctor` — pre-flight dependency check
- [ ] `af doctor --fix` — auto-install missing dependencies
- [ ] Integration tests: temp git repo + mock multiplexer

## Phase 2 — Multi-Agent + Config

- [ ] Pi agent provider
- [ ] Codex agent provider
- [ ] Gemini agent provider
- [ ] Amp agent provider
- [ ] `af config show` — dump effective configuration
- [ ] `af config init` — create default config file
- [ ] Shell completions: bash, zsh, fish
- [ ] Agent availability check + fallback chain

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
- [ ] Migration: read `cf-sessions/*.env` → convert to TOML
- [ ] Man page generation
- [ ] Comprehensive `--help` text for all commands
- [ ] Error messages with actionable suggestions

---

## Backlog (unscheduled)

- [ ] Zellij multiplexer implementation
- [ ] Docker-based sandbox provider
- [ ] `af log` — append to session log (Obsidian note)
- [ ] `af pr` — create PR from session branch
- [ ] `af sync` — sync remote sandbox with local worktree
- [ ] Dataview dashboard template for Obsidian
- [ ] `af doctor --verbose` — detailed version/path info for debugging
- [ ] Workspace template support (pre-configured sessions per project)
