# ADR-008: Phased Delivery Strategy

**Status:** Accepted
**Date:** 2026-03-26

## Context

The full `cf` feature set is ~3,500 lines of shell across 11 files. Rewriting everything at
once is high-risk. The user is learning Rust and wants to iterate incrementally with full test
coverage at each step.

## Decision

Deliver in **6 phases**, each producing a usable binary. Every phase ends with a working `af`
that's strictly more capable than the previous one.

### Phase 0 — Foundation (scaffolding ✅ + core types)

**Goal:** Abstractions, config, session metadata — no user-facing commands yet.

- [x] Project scaffold (done)
- [ ] Core types: `SessionId`, `BranchName`, `SessionName`, `WorktreePath`
- [ ] Configuration system (ADR-003): load/merge user + project config
- [ ] Session metadata store (ADR-006): TOML read/write/list
- [ ] Platform detection: macOS, Arch, Debian (ADR-010)
- [ ] Package manager abstraction: brew, pacman, apt (ADR-010)
- [ ] Dependency table with tier system: Must/Should/Nice (ADR-009)
- [ ] Multiplexer trait + tmux implementation (ADR-002): create/kill/attach/env
- [ ] Agent provider trait + Claude implementation (ADR-001): launch/resume commands
- [ ] Git helpers: worktree create/remove, branch ops, main branch detection, org detection
- [ ] UUID v5 generation (deterministic session IDs)
- [ ] Session name sanitization

**Tests:** Unit tests for every helper. No integration tests yet.

### Phase 1 — Local MVP

**Goal:** `af create`, `af done`, `af list`, `af resume` for local git worktree sessions.
Replace the core `cf` / `cfd` / `cfl` / `cfr` workflow.

- [ ] `af create [task-name]` — worktree + tmux + agent launch
- [ ] `af done [session]` — teardown with confirmation
- [ ] `af list` — show active sessions
- [ ] `af resume [session]` — reattach
- [ ] `--from`, `--current`, `--from-pr` flags
- [ ] Branch prefix logic (fork detection)
- [ ] Session limit guard
- [ ] Workspace mode (non-git directories)
- [ ] `--bare` mode
- [ ] `af session-branch` (csb equivalent)
- [ ] `af doctor` — pre-flight dependency check (ADR-009, ADR-010)
- [ ] `af doctor --fix` — auto-install missing dependencies

**Tests:** Integration tests using temp git repos + tmux.
**Milestone:** Can use `af` for daily local workflow.

### Phase 2 — Multi-Agent + Config

**Goal:** Support multiple agents and the full configuration system.

- [ ] `--agent <name>` flag on `af create`
- [ ] Pi agent provider
- [ ] Codex agent provider
- [ ] Gemini agent provider
- [ ] Amp agent provider
- [ ] `af config` subcommand (show effective config, init)
- [ ] Shell completions (bash, zsh, fish via `clap_complete`)

**Tests:** Agent launch command generation tests (no real agent needed).
**Milestone:** Can switch agents per session.

### Phase 3 — Remote Providers

**Goal:** `af create --remote` with workspaces and exe.dev providers.

- [ ] Remote provider trait + routing logic
- [ ] DD Workspaces provider
- [ ] exe.dev provider
- [ ] SSH bootstrap pipeline (embedded defaults)
- [ ] Dotfiles provisioning config: repo + install_cmd (ADR-009)
- [ ] Remote provisioning pipeline: bootstrap → dotfiles → auth
- [ ] Remote session resume (reconnect on SSH drop)
- [ ] Orphan detection for remote environments
- [ ] `--yolo` flag (unattended mode)

**Tests:** Provider detection + command generation (mocked SSH).
**Milestone:** Full remote workflow.

### Phase 4 — Sandbox + Obsidian

**Goal:** `af create --sandbox` and Obsidian note integration.

- [ ] Sandbox provider trait + slicer implementation
- [ ] Local sandbox (VirtioFS path mapping)
- [ ] Remote sandbox (`--sandbox --remote`)
- [ ] VM health check + respawn
- [ ] Auth management (`af auth`)
- [ ] Obsidian integration (ADR-007)
- [ ] `af note` subcommand

**Tests:** Sandbox command generation, note creation/update.
**Milestone:** Feature parity with `cf`.

### Phase 5 — GC + Editor + Polish

**Goal:** Garbage collection, editor integration, and production hardening.

- [ ] `af gc` — merge detection (PR state, ancestry, squash-merge fingerprint)
- [ ] `af editor` — open codebase in terminal/visual editor
- [ ] Migration tool: read `cf-sessions/*.env` → convert to `af` TOML
- [ ] Man page generation
- [ ] `af completions` subcommand
- [ ] Comprehensive error messages with suggestions
- [ ] Telemetry-free, privacy-respecting design (document it)

**Tests:** GC heuristics with crafted git histories.
**Milestone:** Production-ready, daily-driver quality.

## Consequences

- Each phase is independently valuable — the user can start using `af` from Phase 1.
- Phases are ordered by frequency of use (local > multi-agent > remote > sandbox > gc).
- The trait-based architecture (Phases 0-1) pays off in Phases 2-4 where providers multiply.
- Obsidian integration is deferred to Phase 4 — it's additive and should not block core workflow.
- Shell completions come in Phase 2 because `clap_complete` needs the full CLI shape.
