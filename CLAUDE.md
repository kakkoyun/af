# CLAUDE.md — v1 Project Constitution

## Context

`af` (agentic-flow / automatic-flow / as-fuck) — a single-user Go CLI for
stitching together AI coding agents, tmux, sandboxes, and remote machines.
Owner: @kakkoyun. Single-module Go project, MSRV pinned in `go.mod` once
toolchain lands.

**v0 (Rust) is reference material at `src/`.** It is not built and not
modified. v1 lives at `cmd/af/` and `internal/...` once implementation
begins. See [`docs/v0/README.md`](docs/v0/README.md) for the archive.

## Constitution

These rules are non-negotiable. They survive context compaction and
session boundaries.

### 1. TDD — No Exceptions

Write the test first. Watch it fail. Implement. Watch it pass. Refactor.
Never commit code without tests. Never skip the red step. Pure-logic
packages must be 80%+ covered; IO-shimmed packages rely on `testscript`
golden tests (see ADR-051 once written).

### 2. Unverified Work Is Unfinished Work

Run `make check` (or its component commands) before every commit. If it
doesn't pass, it's not done. The bar:

```bash
gofumpt -l .                                    # zero output
goimports -l .                                  # zero output
golangci-lint run                               # zero warnings (all linters on)
go test -race -count=1 ./...                    # all green
```

Use `slog` to log assumptions and verify them at runtime.

### 3. Documentation Is the Spec

User-facing documentation (`README.md`, `CHANGELOG.md`, command `--help`
text, ADR bodies) is the contract. If the code doesn't match the docs,
the code is wrong. Update docs with every feature change.

### 4. Versioned References

`docs/SPEC.md` and `docs/PLAN.md` are **editable during the planning
phase** so they can be kept consistent with the v1 ADRs as those
iterate. They freeze once all v1 ADRs are `accepted` (per ADR-032's
lifecycle). **After freeze, they are immutable** — design changes go
through new ADRs in `docs/adr/` (append-only from 031). v0 ADRs
(`docs/v0/adr/`) are frozen historical record at all times.

### 5. Progress Tracking Is Mandatory

- `PROGRESS.md` — append-only narrative log. Write after every work session.
- `TODO.md` — checkbox task list mirroring the doc-pass commits and any post-doc-pass implementation work.
- Use Claude Code Tasks for in-session tracking (resilient to compaction).
- Commit after every small achievement, never in large batches.

### 6. No Corners Cut

Every exported identifier gets a doc comment. Every error path gets a
test. All `golangci-lint` linters are on. Mandatory: `unparam`,
`nolintlint`, `revive`, `staticcheck`, `errcheck`, `gocritic`, `gosec`.
No `panic()` in library code outside genuinely-unreachable arms.

### 7. Tight Commit Discipline

Format: `<type>(<scope>): <description>`. Stage specific files, never
`git add .` blindly. One logical change per commit. Never commit interim
files (plans, scratch notes). The `secret-guard` pre-commit hook will
flag suspicious diffs — review the warning, don't bypass.

### 8. Stdlib-First Dependencies

Reach for the Go standard library before any third-party package.
External deps require an ADR justification. The approved v1 set is in
ADR-031 (master). Adding a dep is a new ADR or an amendment, not a
silent `go get`.

### 9. Blockers Are Documented, Not Hidden

If blocked, record the blocker in `PROGRESS.md` and mark the TODO item
with a note. Move on to the next task. Never silently skip work.

### 10. v0 Tree Is Read-Only

`src/`, `Cargo.toml`, `Cargo.lock`, `clippy.toml`, `deny.toml`,
`rust-toolchain.toml`, `rustfmt.toml`, `.cargo/`, `justfile` are kept
in-tree as reference until v1 has parity. **Do not modify them.** Do
not build them. Refer to them for behavioural questions only.

## Session Start Protocol

Every new session, before writing any code:

1. Read `PROGRESS.md` — understand what was done last.
2. Read `TODO.md` — find the current stage and next unchecked task.
3. Run `make check` (once it exists) or its component commands.
4. Verify baseline is green before making changes.
5. Follow the TDD workflow in `AGENTS.md`.

During the doc pass (no Go code yet), step 3 is reduced to "no v0
files modified" — `git status -- src/ Cargo.toml Cargo.lock` should
be empty.

## Documentation Hierarchy

| File                  | Purpose                                          | Mutability                  |
| --------------------- | ------------------------------------------------ | --------------------------- |
| `CLAUDE.md`           | Constitution — rules, context, build commands    | Update when process changes |
| `AGENTS.md`           | Working agreement — TDD, quality, subagent rules | Update when process changes |
| `README.md`           | User-facing contract — must match reality        | Update with every feature   |
| `CHANGELOG.md`        | Release notes — Keep a Changelog format          | Update with every feature   |
| `PROGRESS.md`         | Narrative log per session                        | Append after each session   |
| `TODO.md`             | Task list per stage + backlog                    | Check off / add as needed   |
| `docs/SPEC.md`        | v1 specification                                 | Editable during planning; **IMMUTABLE** after freeze |
| `docs/PLAN.md`        | v1 plan (lightweight; references ADRs)           | Editable during planning; **IMMUTABLE** after freeze |
| `docs/CONVENTIONS.md` | Go conventions, file ownership                   | Append, never overwrite     |
| `docs/adr/`           | v1 ADRs 031–053 (append-only)                    | New ADRs only               |
| `docs/v0/`            | Frozen Rust-era archive                          | **READ-ONLY**               |

## Build & Test (target state, once Go scaffold lands)

```bash
make build              # go build ./cmd/af
make test               # go test -race -count=1 ./...
make lint               # golangci-lint run
make fmt                # gofumpt -w . && goimports -w .
make fmt-check          # gofumpt -l . && goimports -l . (zero output)
make check              # fmt-check + lint + test — MUST pass before commit
make install            # go install ./cmd/af
```

Until the `Makefile` exists, run the equivalent `go ...` commands
directly. Do **not** run `cargo` or `just` — those target v0.

## Architecture (target state, per ADRs to be written)

```
cmd/af/
└── main.go             # Entry point: parse args, init slog, dispatch via cobra

internal/
├── agent/              # Agent interface + claude/pi/codex impls (ADR-043)
├── config/             # TOML loader, layered (ADR-036)
├── git/                # Worktree, branch, remote, PR helpers
├── mux/                # Multiplexer interface + tmux impl (ADR-040)
├── obsidian/           # Notes + Bases (ADR-047)
├── remote/             # SSH host model (ADR-041)
├── sandbox/            # slicer + sbx (ADR-042)
├── secret/             # keyring + tmpfs envelope transport (ADR-049)
├── session/            # state.toml + ledger.jsonl (ADR-037)
└── workstream/         # Worktree layout, sub-worktrees (ADR-038)

docs/
├── adr/                # v1 ADRs (031+, append-only)
├── SPEC.md, PLAN.md, CONVENTIONS.md
└── v0/                 # frozen Rust-era archive
```

Module path: `github.com/kakkoyun/af` (set in `go.mod` once scaffold lands).

## Conventions

- **Edition**: latest stable Go (the version pinned in `go.mod`).
- **Lint**: `golangci-lint` with all linters on. See ADR-050.
- **Format**: `gofumpt` (stricter `gofmt`); `goimports` for import order.
- **Errors**: wrap with `fmt.Errorf("...: %w", err)`; sentinel errors per package as `var ErrThing = errors.New("...")`.
- **Logging**: `log/slog` from stdlib; structured fields, never `fmt.Println` outside `cmd/af/main.go`.
- **Context**: every function that calls an external command (`exec.Command`, `os.Open` for big reads, `net.Dial`) takes a `context.Context`.
- **No `init()`**: wiring lives in `main()`. Tests may use `TestMain` sparingly.
- **Commit format**: `<type>(<scope>): <description>` — types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`, `perf`, `ci`, `build`, `style`. Scopes follow package names: `agent`, `config`, `mux`, etc., or `v0`/`v1` for doc-pass commits.

## Release

**v1 has no release.** Single-user. Install via `go install` or
`make install`. `goreleaser` config lives at `.goreleaser.yml` for
local cross-compile only — see ADR-053.
