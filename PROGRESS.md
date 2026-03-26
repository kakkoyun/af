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

- **Phase 0 completed** — final 2 git tasks done sequentially (no subagents).
  - `git/worktree.rs` (7 tests) — create/remove worktrees, delete branches. Real temp git repos.
  - `git/resolve.rs` (12 tests) — preferred_remote, has_upstream, fetch, list_local_branches,
    detect_main_branch_local, resolve_base_branch, fetch_and_resolve_base. Uses cloned bare repos
    for remote-tracking ref tests.

- **AGENTS.md updated retrospectively** — subagent coordination rules tightened:
  commit before spawning, subagents work on branches not main, lead reviews.

### Current State

- **Phase 0: COMPLETE.** All 20 tasks checked off. 141 tests passing. All CI green.
- Ready for Phase 1: `af create`, `af done`, `af list`, `af resume`.

### Next

- Phase 1: implement the `af create` command (local worktree mode first).

---

## 2026-03-26 — Session 3: Phase 1 Implementation

### Done

- **Phase 1 substantially complete** — 17 of 20 tasks done. 150 tests passing.

- **CLI definition** (`cli.rs`) — 7 subcommands with clap derive:
  create, done, list, resume, session-branch, doctor, version.
  Flag conflicts enforced: `--from` vs `--current`, `--yes` requires `--fix`.

- **Command implementations:**
  - `cmd/create.rs` — full local worktree mode: detect git root → resolve base →
    name generation (explicit, auto, branch-pinned) → prefix logic → worktree
    creation → mux session → agent launch → state.toml + ledger.jsonl.
    Workspace mode for non-git directories. `--from`, `--current`, `--bare`, `--agent`.
  - `cmd/done.rs` — resolve session (arg or current mux), confirmation prompt,
    kill mux → remove worktree → delete branch → archive. Ledger events.
  - `cmd/list.rs` — load all sessions, group by repo, mark current repo.
  - `cmd/resume.rs` — resume by name or fzf picker. Recreate dead mux sessions
    from disk metadata, relaunch agent with `--continue`. Ledger events.
  - `cmd/session_branch.rs` — lightweight: launch agent with branch-tied UUID.
  - `cmd/doctor.rs` — build dependency list from config, tier-based reporting.

- **Integration tests** (13 new) — help output for all subcommands, flag
  conflict validation, empty list behavior, `--yes` requires `--fix`.

- **Clippy fixes** — resolved 11 issues: redundant clones, `map().unwrap_or_else()`,
  identical if blocks, boolean simplification, `process::exit`, missing docs.

### Deferred to Phase 2

- `--from-pr` — requires `gh` CLI integration for PR branch resolution.
- `--doctor --fix` — auto-install placeholder, full implementation in Phase 2.
- `resume --bare` — flag accepted, dedicated bare-mode logic pending.

### Current State

- **Phase 1: SUBSTANTIALLY COMPLETE.** 17/20 tasks done. 150 tests passing.
- 3 tasks deferred (reasonable — they depend on Phase 2 features or `gh` integration).
- The tool is now usable for daily local workflow with `af create`, `af done`, `af list`, `af resume`.

### Next

- Phase 2: Multi-agent support, remaining agent providers, config commands, completions.
