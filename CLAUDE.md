## Context

Rust CLI application — `af` (agentic-flow, automatic-flow, or as-fuck).
Workflow tooling for agentic/automatic programming.
Owner: @kakkoyun. Single-crate Rust project, edition 2024, MSRV 1.85.

## Constitution

These rules are non-negotiable. They survive context compaction and session boundaries.

### 1. TDD — No Exceptions

Write the test first. Watch it fail. Implement. Watch it pass. Refactor.
Never commit code without tests. Never skip the red step.
See `AGENTS.md` for the full TDD workflow.

### 2. Unverified Work Is Unfinished Work

Run `cargo fmt --check && cargo clippy --all-targets -- -D warnings && cargo test` before
every commit. If it doesn't pass, it's not done. Use `tracing::debug!` to log assumptions
and verify them at runtime.

### 3. Documentation Is the Spec

User-facing documentation (`README.md`, `CHANGELOG.md`, `--help` text) is the contract.
If the code doesn't match the docs, the code is wrong. Update docs with every feature change.

### 4. Immutable References

`docs/PLAN.md` and `docs/SPEC.md` are **immutable**. Never modify them.
To change a design decision, create a new ADR in `docs/adr/` for review.

### 5. Progress Tracking Is Mandatory

- `PROGRESS.md` — append-only narrative log. Write after every work session.
- `TODO.md` — checkbox task list by phase. Check off as completed. Add blockers.
- Use Claude Code Tasks for in-session tracking (resilient to context compaction).
- Commit after every small achievement, not in large batches.

### 6. No Corners Cut

Every public item gets a doc comment. Every error path gets a test. Clippy pedantic is on.
`unsafe` is forbidden. No `todo!()` in committed code. No `unwrap()` in library code.

### 7. Tight Commit Discipline

Format: `<type>(<scope>): <description>`. Stage specific files, never `git add .`.
One logical change per commit. Never commit interim files (plans, scratch notes).

### 8. Blockers Are Documented, Not Hidden

If blocked, record the blocker in `PROGRESS.md` and mark the TODO item with a note.
Move on to the next task. Never silently skip work.

## Session Start Protocol

Every new session, before writing any code:

1. Read `PROGRESS.md` — understand what was done and what's next
2. Read `TODO.md` — find the current phase and next unchecked task
3. Run `cargo fmt --check && cargo clippy --all-targets -- -D warnings && cargo test`
4. Verify baseline is green before making changes
5. Follow the TDD workflow in `AGENTS.md`

Note: `just` may not be available in all environments. Use raw `cargo` commands as fallback.

## Documentation Hierarchy

| File | Purpose | Mutability |
|---|---|---|
| `CLAUDE.md` | **Constitution** — rules, context, build commands | Update when process changes |
| `AGENTS.md` | **Working agreement** — TDD, quality standards, process | Update when process changes |
| `README.md` | **User-facing contract** — must match reality | Update with every feature |
| `CHANGELOG.md` | **Release notes** — Keep a Changelog format | Update with every feature |
| `PROGRESS.md` | **Narrative log** — what was done, blockers, decisions | Append after each session |
| `TODO.md` | **Task list** — checkbox items by phase + backlog | Check off / add as needed |
| `docs/SPEC.md` | Full specification | **IMMUTABLE** |
| `docs/PLAN.md` | Implementation plan with architecture diagram | **IMMUTABLE** |
| `docs/adr/` | Architecture Decision Records (11 ADRs) | Add new, never modify existing |

## Build & Test

```bash
cargo fmt --check       # Check formatting
cargo fmt               # Auto-format
cargo clippy --all-targets -- -D warnings  # Lint (pedantic, warnings=errors)
cargo test              # Run all tests
cargo build             # Debug build
cargo build --release   # Release build
cargo run -- <args>     # Run the CLI
cargo doc               # Build rustdoc (RUSTDOCFLAGS="-D warnings")
```

If `just` is available:
```bash
just check              # fmt + lint + test + deny + doc — MUST pass before commit
just deny               # License/advisory/ban checks
```

## Architecture

See `docs/PLAN.md` for the full module map. Three orthogonal layers:

- **Agent layer** (`agent/`) — 7 providers: claude, pi, codex, gemini, amp, copilot
- **Remote layer** (`provider/exedev`, `provider/workspaces`) — where the machine lives
- **Sandbox layer** (`provider/slicer`, `provider/docker`) — isolation around the agent

These compose: `--remote --sandbox --agent codex` = remote machine + sandbox + codex inside.

```
src/
├── main.rs          # Thin: parse args, init tracing, dispatch
├── cli.rs           # Clap derive definitions (all subcommands)
├── lib.rs           # Library crate — all core logic
├── config/          # TOML config system (ADR-003)
├── platform/        # OS detection, deps, package managers (ADR-009, ADR-010)
├── session/         # Session types, metadata store, ledger (ADR-006, ADR-011)
├── git/             # Worktree, branch, remote, PR helpers
├── mux/             # Multiplexer trait + tmux (ADR-002)
├── agent/           # Agent providers: claude, pi, codex, gemini, amp, copilot (ADR-001)
├── provider/        # Remote (exedev, workspaces) + sandbox (slicer, docker) (ADR-004, ADR-005)
├── provision/       # SSH bootstrap + dotfiles pipeline (ADR-009)
├── obsidian/        # Workstream notes integration (ADR-007)
├── cmd/             # Subcommand implementations
└── util/            # UUID v5, notifications, shared utilities
```

## Working Commands (current state)

```
af create [name]        # worktree + mux + agent (--remote, --sandbox, --yolo, --from-pr)
af done [session]       # teardown with confirmation, archive, remote cleanup
af list                 # grouped by repo
af resume [session]     # fzf picker, multi-agent recovery, --respawn for dead VMs
af agent add/stop/list  # multi-agent pane management
af gc [--dry-run] [--all]  # merge detection + cleanup
af editor [--terminal] [--visual]  # open codebase in editor
af diff [session]       # visual diff via diffity/delta
af pr [session]         # create GitHub PR from session metadata
af note [session]       # open Obsidian workstream note
af stats                # workstream analytics from ledger data
af export [--format]    # export ledger data as JSON/CSV
af doctor [--fix] [--verbose]  # dependency check + auto-install
af config show/init     # config management
af completions bash/zsh/fish  # shell completions
af session-branch       # branch-tied agent launch
af version              # version info
```

## Conventions

- **Edition 2024**, MSRV 1.85. Phased delivery per `docs/adr/008-phased-delivery.md`.
- **Clippy pedantic** with `-D warnings`. Fix every warning.
- `unsafe` is **forbidden** (`#[forbid(unsafe_code)]`).
- `print_stdout` / `print_stderr` / `dbg_macro` are warnings — use `tracing` in library code.
- Use `anyhow::Result` in the binary, `thiserror` for typed errors in the library.
- All public items require doc comments (`missing_docs = "warn"`).
- Commit format: `<type>(<scope>): <description>` (see `AGENTS.md` for scopes).

## Release

- Tag with `vX.Y.Z` to trigger the release workflow.
- Builds for: x86_64/aarch64 Linux (glibc + musl), x86_64/aarch64 macOS.
- Checksums (SHA256) are published alongside binaries.
