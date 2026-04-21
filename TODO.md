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
- [x] `af create --from-pr <number>` — PR worktree (needs `gh`)
- [x] `af create --bare` — bare mode (no VM, host worktree)
- [x] `af create` — workspace mode (non-git directory)
- [x] `af create --agent <name>` — agent selection
- [x] Session limit guard (`max_sessions` config)
- [x] `af done [session]` — teardown with confirmation
- [x] `af done --force` — skip confirmation, force-delete unmerged
- [x] `af list` — grouped by repo, current repo first
- [x] `af resume [session]` — re-attach to session
- [x] `af resume` (no args) — fzf picker (if available)
- [x] `af resume --bare` — resume in bare mode (runs agent directly in terminal)
- [x] `af session-branch` — branch-tied session ID
- [x] Ledger: emit events from create/done/resume commands
- [x] Agent session ID tracking in state.toml
- [x] `af doctor` — pre-flight dependency check
- [x] `af doctor --fix` — auto-install missing dependencies via platform package manager
- [x] Integration tests: CLI help, flag conflicts, empty list

## Phase 2 — Multi-Agent + Config

- [x] Pi agent provider
- [x] Codex agent provider
- [x] Gemini agent provider
- [x] Amp agent provider
- [x] Multi-agent slot model in state.toml (primary + named slots) *(schema done in Phase 0)*
- [x] `af agent add --slot <name> --agent <provider>` — add agent to workstream pane
- [x] `af agent stop <slot>` — stop an agent in a slot
- [x] `af agent list` — show agents in current workstream
- [x] Multi-agent resume: restore all agent panes on `af resume`
- [x] Multi-agent teardown: stop all agents on `af done`
- [x] `af config show` — dump effective configuration
- [x] `af config init` — create default config file
- [x] Shell completions: bash, zsh, fish
- [x] Agent availability check + fallback chain

## Phase 3 — Remote Providers

- [x] Remote provider trait definition + stubs (workspaces, exedev, slicer)
- [ ] DD Workspaces provider (detect, create, teardown) *(deferred: requires workspaces CLI)*
- [x] exe.dev provider (detect, create, setup, teardown via SSH)
- [x] `af create --remote [host]` — remote session via exe.dev/SSH
- [x] `af create --yolo` — unattended mode (passes through to agent LaunchOpts)
- [x] `af create --sandbox` — sandbox mode via slicer
- [x] SSH bootstrap pipeline (embedded default scripts via include_str!)
- [x] Dotfiles provisioning config (repo + install_cmd)
- [x] Remote provisioning pipeline: bootstrap → dotfiles → auth
- [ ] Remote session resume (SSH drop detection + reconnect) *(deferred)*
- [ ] Orphan detection in `af list` *(deferred)*
- [x] `af done` for remote sessions (exe.dev teardown)

## Phase 4 — Sandbox + Obsidian

- [x] Sandbox provider trait definition
- [x] Slicer sandbox provider (local) — vm lifecycle + agent sandbox commands
- [ ] Slicer sandbox provider (remote: `--sandbox --remote`) *(deferred)*
- [x] VirtioFS path mapping (slicer map_path)
- [x] VM health check + respawn in `af resume --respawn`
- [ ] `af auth setup/reroll/status/clear` *(deferred: requires keyring integration)*
- [ ] Auth token injection (keychain/keyring/file) *(deferred)*
- [x] Obsidian note creation on `af create`
- [x] `af note [session]` — open Obsidian note
- [x] Frontmatter update on `af done` (status → completed)

## Phase 5 — GC + Editor + Polish

- [x] `af gc` — list merged/closed worktrees
- [x] `af gc --dry-run` — preview without action
- [x] `af gc --all` — clean all without prompts
- [x] Merge detection: GitHub PR state (via `gh`)
- [x] Merge detection: git ancestry (`merge-base --is-ancestor`)
- [x] Merge detection: squash-merge fingerprint (diff cksum)
- [x] `af editor --terminal` — split pane with `$EDITOR`
- [x] `af editor --visual` — VS Code/Zed GUI
- [ ] `af editor` for remote sessions (SSH + URL schemes) *(deferred: requires remote providers)*
- [x] Session archival: move to archive/ on `af done`, retain for 90 days
- [x] PR tracking: detect/record PR number+URL from branch
- [x] Ledger events: pr_opened, pr_merged, pr_closed (emitted on af done)
- [x] Agent session log discovery (claude, pi file path conventions)
- [x] `af gc` prunes expired archives (older than retention_days)
- [x] Migration: read `cf-sessions/*.env` → convert to TOML
- [x] Man page generation (`af mangen` hidden subcommand)
- [x] Comprehensive `--help` text for all commands
- [x] Error messages with actionable suggestions
- [x] CHANGELOG.md (Keep a Changelog format)
- [ ] User guide (mdBook or similar, deployed to GitHub Pages)
- [x] README.md final polish — remove stale 🔜, mark planned features, all examples verified

---

## Pattern-Hardening Sprint (initial topics — must complete before 0.1.0 tag)

See `~/.claude/plans/alrighty-analyz-this-project-compiled-snowglobe.md` for the full
lane specs, parallelism map, and Definition-of-Done checklists.

### Phase I — Foundation (Lane D) ✅

- [x] `fix(cargo)`: drop lld linker override on Apple targets
- [x] `feat(cargo)`: introduce workspaces/slicer-remote/keyring feature flags
- [x] `docs(adr)`: ADR-015 subagent coordination patterns
- [x] `docs(conventions)`: file-ownership manifest (`docs/CONVENTIONS.md`)
- [x] `docs(adr)`: ADR-021 release discipline & CHANGELOG-driven notes
- [x] `ci(release)`: CHANGELOG-driven release notes (drop generate_release_notes)
- [x] `ci(check)`: add cargo-audit job
- [x] `feat(just)`: release-dry-run + book-gen recipes
- [x] `docs(changelog)`: reconcile 0.1.0 with all shipped Phase 3/4/5 work
- [x] `docs(adr)`: update ADR index with 012-015 and 021

### Phase II — Design ADRs (parallel, no code yet)

- [ ] ADR-018: External Tool Dependency Testing (Lane A1)
- [ ] ADR-016: Secret Storage for `af auth` (Lane B2)
- [ ] ADR-017: Remote Session Resume & Reconnect Strategy (Lane B3)
- [ ] ADR-019: Remote Editor URL Scheme Strategy (Lane B5)
- [ ] ADR-020: mdBook User Guide Structure + Machine Index (Lane C1)

### Phase III — Implementation (parallel, file-disjoint)

- [ ] **Lane A1**: DD Workspaces provider (real impl + CommandRunner trait + fake)
- [ ] **Lane A2**: Orphan detection in `af list`
- [ ] **Lane B1**: Slicer `--sandbox --remote` composition
- [ ] **Lane B2**: `af auth setup/reroll/status/clear` + keyring
- [ ] **Lane B3**: Remote session resume (SSH drop detection + reconnect)
- [ ] **Lane B4**: exe.dev SSH liveness check (folds into B3)
- [ ] **Lane B5**: `af editor` for remote sessions (URL schemes)
- [ ] **Lane C1**: mdBook user guide + `scripts/book-gen.sh` + `index.json` + CI

### Phase IV — Integration (lead-only)

- [ ] Wire all new modules into `src/lib.rs`, `src/cli.rs`, `src/cmd/mod.rs`
- [ ] Update `Cargo.toml` with optional deps for keyring/workspaces
- [ ] Update `README.md` with new commands
- [ ] Final `CHANGELOG.md` date stamp + version link
- [ ] Update `docs/adr/README.md` with ADRs 016-020
- [ ] Final `cargo test --all-features` green
- [ ] PROGRESS.md session entry

### Phase V — Release Gate (user-triggered)

- [ ] `just release-dry-run` — all 6 matrix targets green
- [ ] Delete draft release
- [ ] User approves tag
- [ ] `git tag -a v0.1.0` + `git push origin v0.1.0`

---

## Backlog (unscheduled)

- [x] Remote control: superterm notification integration (notify on create/done, agent-hook stop)
- [x] `af diff` subcommand — visual diff via diffity with delta/git-diff fallback
- [x] Configurable editor per context: config.editor.terminal/visual with priority chain
- [ ] Local multiplexer providers: Ghostty, cmux (beyond tmux/zellij)
- [ ] Obsidian + Claude Code working documents (shared context/notes during sessions)
- [ ] Zellij multiplexer implementation
- [x] Docker-based sandbox provider (via sbx CLI — Docker AI Sandboxes)
- [ ] `af log` — append to session log (Obsidian note)
- [x] `af pr` — create PR from session branch (via gh pr create)
- [ ] `af sync` — sync remote sandbox with local worktree
- [ ] Dataview dashboard template for Obsidian
- [x] `af doctor --verbose` — detailed version/path info for debugging
- [x] `af stats` — workstream analytics from ledger data (agent usage, event counts)
- [x] `af export` — export ledger data as JSON/CSV
- [ ] Workspace template support (pre-configured sessions per project)
