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

- [x] ADR-018: External Tool Dependency Testing — `CommandRunner` trait + feature gates (Lane A1)
- [x] ADR-016: Secret Storage — `keyring` crate, macOS Keychain + Arch Secret Service dbus (Lane B2)
- [x] ADR-017: Remote Session Resume — SSH probe + reconnect flow (Lane B3)
- [x] ADR-019: Remote Editor URL Schemes — vscode-remote://, cursor://, `zed ssh://` (Lane B5)
- [x] ADR-020: mdBook User Guide — book structure + `index.json` machine index (Lane C1)
- [x] `docs/CONVENTIONS.md`: updated with worktree protocol + naming table
- [x] `docs/adr/README.md`: all 21 ADRs indexed (001–021)

### Phase II.5 — ADR Revision Round (post-research gap analysis) — REVISED

Triggered by CLI-surface research (2026-04-21), three-reviewer synthesis
(critic + security + architect), and user directives D1–D7. See
`docs/planning/gap-analysis.md §8–§9` and `docs/planning/adr-drafts.md`.

**Pre-Phase-II.5 (independent of any ADR):**

- [ ] **Lane L-FIX**: `fix(docker):` three commits — workdir passthrough (G4),
      full `KNOWN_SBX_AGENTS` list (G5), drop double-create (G6). Red-first,
      regression tests. Land before or during Phase II.5.

**ADR authoring (lead-only, single branch `phase-2.5-adr-revision`):**

- [ ] Ratify ADR-022: cmux Multiplexer Provider (first-class per D3; critic's
      defer-to-0.2.0 recommendation rejected by directive — see §8.5)
- [ ] Ratify ADR-023: Sandbox Agent-Layer Conflict Resolution (ratifies slicer
      split Option A; folds sbx double-create fix — Lane L-FIX carries code)
- [ ] Ratify ADR-024: Remote Sandbox via Daemon URL (slicer `--url`/`--token`;
      supersedes ADR-014 §"Composition model" L37–41 for slicer)
- [ ] Ratify ADR-025: Secret Boundaries (narrows ADR-016 per D1 + security
      N1/N3/H-a/H-b/H-c; forbids SSH `SetEnv`/`SendEnv`; host+exedev only;
      `secrecy::SecretString` + dedicated Linux collection)
- [ ] Ratify ADR-027: Remote = SSH Target (narrows ADR-004 + ADR-017; drops
      `setup()` from `RemoteProvider` trait; adds `ssh_target()` +
      `Liveness` enum; `accept-new` on probe)
- [ ] Ratify ADR-028: Agent-Level OS Sandbox (adds `--agent-sandbox=<none|os>`
      per D6; orthogonal to VM-sandbox layer)
- [ ] **Addendum ADR-029 (A-b)**: ADR-018 supersession — drop `CommandRunner`
      trait per critic [C 2.1]; adopt feature-gates + `assert_cmd` only
- [ ] **DROPPED**: ADR-026 (provider-specific liveness) — folds into ADR-027
- [ ] Write `docs/reference/external-tools.md` (verified CLI surface reference — G10)
- [ ] Amend ADR-017 probe to `StrictHostKeyChecking=accept-new` (security C2/N2)
- [ ] Amend ADR-016 account naming to `<provider>` (drop `af/` prefix per [C 2.2])
- [ ] Update `docs/adr/README.md` with ADRs 022–029
- [ ] Update `docs/CONVENTIONS.md` worktree table with L-* lanes
- [ ] Delete `docs/planning/adr-drafts.md` and `docs/planning/gap-analysis.md`
      once all ADRs land

**Scope-call checkpoint (user):** §8.6 open items (Windows stance, headless
`af auth`, multi-user keyring, `insta` vs `include_str!` snapshot, awk vs
`git-cliff` extraction, `xtask` vs shell, `cargo audit` CVE verify). Most can
defer to 0.2.0 with a one-sentence ADR; user calls.

### Phase III — Implementation (parallel, file-disjoint) — REVISED (12 → 7 lanes)

Architect [A] consolidation. Each lane is single-sentence-scope and
file-disjoint; lead integrates.

- [ ] **Lane L-REMOTE**: `RemoteProvider` trait narrowing per ADR-027 + DD
      Workspaces provider (`workspaces create/list/ssh-config/delete/restart`)
      + `ssh_target()` + `is_alive()` + orphan column in `af list`. Folds
      former A1, A2, B3, B4. (G9, G11, H-e)
- [ ] **Lane L-SBX-DAEMON**: Slicer `--sandbox --remote` via `--url`/`--token`
      per ADR-024. One `SandboxConfig.remote_daemon` field + test. Folds
      former B1. (G1)
- [ ] **Lane L-AUTH**: `af auth setup/reroll/status/clear` + keyring per
      ADR-016 as narrowed by ADR-025. Host + exedev only. `secrecy::SecretString`
      + `/run/user/$UID/af-<session>/.env` (not SSH `SetEnv`). Folds former
      B2; **DROPS** former B2.5 (no af-level cross-provider sync per D1).
- [ ] **Lane L-EDITOR**: `af editor` for remote sessions — URL schemes
      (ADR-019) + `workspaces connect` branch for DD sessions. Folds former
      B5. (G8)
- [ ] **Lane L-MUX-CMUX**: cmux as second `Multiplexer` impl per ADR-022.
      Promoted mandatory per directive D3 (see gap-analysis §8.5 conflict).
      Trait unchanged. (G7)
- [ ] **Lane L-AGENT-SANDBOX**: Per-agent `--agent-sandbox=os` mapping per
      ADR-028. `src/agent/codex.rs` → `-s workspace-write`; `src/agent/claude.rs`
      → documented no-op; others → degrade to `none` with info log. (G15, D6)
- [ ] **Lane L-BOOK**: mdBook user guide + `scripts/book-gen.sh` +
      `commands/index.json` + CI per ADR-020. Adds
      `book/src/concepts/approval-modes.md` page (A-c) lifting ADR-012's
      per-agent table. (G14, D5)
- [ ] **NEUTRALIZED**: Lane S1 — ADR-023 ratifies shipped slicer split in ~2
      paragraphs; no code change.

### Phase IV — Integration (lead-only)

- [ ] Wire all new modules into `src/lib.rs`, `src/cli.rs`, `src/cmd/mod.rs`,
      `src/provider/mod.rs`, `src/mux/mod.rs`
- [ ] Update `Cargo.toml` with optional deps for keyring (`secrecy`, `zeroize`),
      workspaces, cmux (if gated), slicer-remote
- [ ] **Addendum A-d**: Overnight-yolo guard in `src/cli.rs` + `src/cmd/create.rs`
      — warn when `--yolo` runs without VM sandbox AND without `--agent-sandbox=os`
      (G16, D7)
- [ ] Update `README.md` with new commands (`af auth`, `--agent-sandbox`,
      cmux selection via config)
- [ ] Final `CHANGELOG.md` date stamp + version link
- [ ] Update `docs/adr/README.md` with ADRs 022–029 (total catalog 001–029
      minus dropped 026)
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
