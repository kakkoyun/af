# Progress Log

Narrative log of implementation progress. Updated after each work session.
See [TODO.md](TODO.md) for the task checklist and [docs/PLAN.md](docs/PLAN.md) for the plan.

---

## 2026-03-26 — Session 1: Project Setup & Planning

### Done

- **Project scaffold** — Rust CLI with edition 2024, MSRV 1.85, clippy pedantic + restriction
  - nursery lints, `unsafe_code = forbid`, `missing_docs = warn`. CI workflow (fmt, clippy,
  test on linux+mac, MSRV, cargo-deny, doc). Release workflow for 6 cross-compile targets.
  justfile with `check`, `fmt`, `lint`, `test`, `deny`, `install-hooks`.

- **Specification** — 635-line `docs/SPEC.md` reverse-engineered from the complete `cf`
  implementation (~3,500 lines of shell across 11 files). Covers all 8 commands, 6 session
  modes, flag parsing, session naming, metadata, providers, bootstrap, GC, editor integration,
  completions. Catalogued 85 existing tests (57 flag parsing + 28 GC).

- **Architecture Decision Records** — 10 ADRs covering:
  - ADR-001: Agent Provider (Claude, pi, Codex, Gemini, Amp)
  - ADR-002: Multiplexer Abstraction (tmux now, zellij later)
  - ADR-003: Layered Configuration System (TOML)
  - ADR-004: Remote Provider (workspaces, exe.dev)
  - ADR-005: Sandbox Provider (slicer, composable with remote)
  - ADR-006: Session Metadata (TOML files as source of truth)
  - ADR-007: Obsidian Integration (per-workstream notes)
  - ADR-008: Phased Delivery (6 phases)
  - ADR-009: Provisioning System (dotfiles-as-config)
  - ADR-010: Platform-Aware Dependencies (macOS, Arch, Debian)

- **Implementation Plan** — `docs/PLAN.md` with architecture diagram, crate structure
  (16 modules), per-phase deliverables, dependency list, testing strategy.

- **Working agreement** — `AGENTS.md` with TDD workflow, code quality standards, subagent
  coordination rules, definition of done.

### Current State

- Phase 0 in progress — scaffold done, no implementation code yet.
- `af version` works. All CI checks pass. 4 integration tests passing.

### Next

- Begin Phase 0 implementation: core types, UUID v5, session name sanitization.

---

## 2026-03-26 — Session 2: Phase 0 Implementation

### Done

- **Phase 0 nearly complete** — 18 of 20 tasks done. 122 tests passing.
  Spawned 4 subagents in parallel for independent modules, integrated their work.

- **Modules implemented:**
  - `config/mod.rs` (13 tests) — Layered TOML: defaults → user → project. Load, merge, roundtrip.
  - `session/types.rs` (6 tests) — SessionState schema: multi-agent slots, PR tracking, version pins.
  - `session/store.rs` (10 tests) — File-per-session TOML store: save/load/delete/list/archive.
  - `session/ledger.rs` (10 tests) — Append-only JSONL event log with builder pattern.
  - `session/naming.rs` (13 tests) — Sanitization (/.: → --), prefix logic.
  - `util/uuid.rs` (6 tests) — UUID v5, verified against Python output.
  - `git/branch.rs` (6 tests) — Main branch detection (main/master/trunk).
  - `git/remote.rs` (10 tests) — Org + repo name parsing for SSH and HTTPS URLs.
  - `platform/mod.rs` (14 tests) — Platform enum, os-release parsing, package manager.
  - `platform/deps.rs` (6 tests) — Dependency tier system (Must/Should/Nice).
  - `mux/mod.rs` + `mux/tmux.rs` (5 tests) — Multiplexer trait, full tmux implementation.
  - `agent/mod.rs` + `agent/claude.rs` (9 tests) — Agent trait, Claude provider.

- **Subagent coordination:** 4 parallel agents (uuid-naming, git-helpers, platform, traits).
  One agent overwrote a file I'd written (ledger.rs) — lesson learned: commit before spawning
  agents that could touch overlapping paths. Fixed by restoring my version post-integration.

- **Clippy fixes during integration:** `str_to_string`, `derivable_impls`, `doc_markdown`,
  `must_use` — all resolved. Full `just check` (fmt + clippy + test + deny + doc-check) green.

### Remaining Phase 0

- Git helpers: worktree create/remove (shells out to git)
- Git helpers: fetch + resolve base branch (upstream preferred)

### Current State

- 122 tests passing. All CI checks green.
- Phase 0 nearly complete, ready to start Phase 1 (local MVP commands).

### Next

- Complete remaining 2 Phase 0 git helper tasks.
- Begin Phase 1: `af create`, `af done`, `af list`, `af resume`.
