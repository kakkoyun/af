## Context

Rust CLI application — `af` (agentic-flow, automatic-flow, or as-fuck).
Workflow tooling for agentic/automatic programming.

## ⚠️  Read Before Writing Code

**Read `AGENTS.md` first.** It contains the working agreement: TDD workflow, code quality
standards, commit conventions, and the definition of done. Violating it will be caught in review.

## Session Start Protocol

Every new session, before writing any code:

1. Read `PROGRESS.md` — understand what was done and what's next
2. Read `TODO.md` — find the current phase and next unchecked task
3. Run `just check` — verify the project is green before making changes
4. Follow the TDD workflow in `AGENTS.md`

## Documentation

| File | Purpose |
|---|---|
| `AGENTS.md` | **Working agreement** — TDD, quality standards, process |
| `PROGRESS.md` | Narrative log of what was done, blockers, decisions |
| `TODO.md` | Checkbox task list by phase |
| `docs/SPEC.md` | Full specification (immutable reference) |
| `docs/PLAN.md` | Implementation plan with architecture diagram (immutable) |
| `docs/adr/` | Architecture Decision Records (10 ADRs) |

## Build & Test

```bash
just check          # Run all checks (fmt, lint, test, deny) — MUST pass before commit
just fmt            # Format code
just lint           # Run clippy (pedantic)
just test           # Run tests
just deny           # License/advisory/ban checks
just build          # Debug build
just build-release  # Release build
just run <args>     # Run the CLI
```

## Architecture

See `docs/PLAN.md` for the full module map. Key layout:

```
src/
├── main.rs          # Thin: parse args, init tracing, dispatch
├── cli.rs           # Clap derive definitions
├── lib.rs           # Library crate — all core logic
├── config/          # TOML config system (ADR-003)
├── platform/        # OS detection, deps, package managers (ADR-009, ADR-010)
├── provision/       # Bootstrap + dotfiles pipeline (ADR-009)
├── session/         # Session types, metadata store (ADR-006)
├── git/             # Worktree, branch, remote helpers
├── mux/             # Multiplexer trait + tmux (ADR-002)
├── agent/           # Agent provider trait + impls (ADR-001)
├── provider/        # Remote + sandbox providers (ADR-004, ADR-005)
├── obsidian/        # Workstream notes (ADR-007)
├── cmd/             # Subcommand implementations
└── util/            # UUID v5, shared utilities
```

## Conventions

- **TDD** — test first, implement second. See `AGENTS.md` for the full workflow.
- **Edition 2024**, MSRV 1.85. Phased delivery per `docs/adr/008-phased-delivery.md`.
- **Clippy pedantic** is on with `-D warnings` in CI. Fix every warning.
- `unsafe` is **forbidden** (`#[forbid(unsafe_code)]` in Cargo.toml).
- `print_stdout` / `print_stderr` / `dbg_macro` are warnings — use `tracing` in library code.
- Use `anyhow::Result` in the binary, `thiserror` for typed errors in the library.
- All public items require doc comments (`missing_docs = "warn"`).
- Run `just check` before every commit. Green CI is non-negotiable.
- Commit format: `<scope>: <what changed>` (see `AGENTS.md` for scopes).

## Release

- Tag with `vX.Y.Z` to trigger the release workflow.
- Builds for: x86_64/aarch64 Linux (glibc + musl), x86_64/aarch64 macOS.
- Checksums (SHA256) are published alongside binaries.
