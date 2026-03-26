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
