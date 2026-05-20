# Progress Log — v1

Narrative log of v1 (Go rewrite) implementation progress. Updated after each
work session. See [`TODO.md`](TODO.md) for the task checklist and
[`docs/PLAN.md`](docs/PLAN.md) (once written) for the implementation plan.

> **v0 history.** The Rust implementation's progress log (eleven sessions
> from 2026-03-26 through 2026-04-28) is archived at
> [`docs/v0/PROGRESS.md`](docs/v0/PROGRESS.md). v1 starts a fresh
> session count.

---

## 2026-05-06 — Session 0: v0 archival + v1 doc pass underway

### Goal

Pivot from the Rust implementation (v0) to a Go rewrite (v1) with a
deliberately reduced scope. Land all documentation before any Go code.

### Done

- **v0 doc archive — Stage A complete (5 atomic commits).** Moved
  `PROGRESS.md`, `TODO.md`, `CHANGELOG.md`, `docs/SPEC.md`,
  `docs/PLAN.md`, `docs/CONVENTIONS.md`, `docs/adr/` (30 ADRs),
  `docs/reference/`, `docs/planning/`, and `book/` to `docs/v0/`.
  Wrote `docs/v0/README.md` indexing the archive and explaining the
  v1 boundary. Commits: `3cc5f5b`, `d6bf397`, `570e477`, `d9e6410`,
  `1659d60`.

- **Top-level CHANGELOG.md** (commit `b36a1ce`) seeded with an
  `[Unreleased]` block for v1 and a forward-looking `Removed` section
  listing every v0 feature being cut.

### In progress

- **v1 doc pass — Stage B (5 commits).** New top-level scaffolding:
  `CHANGELOG.md` ✅, `PROGRESS.md` (this file), `TODO.md`,
  `README.md`, `CLAUDE.md`/`AGENTS.md`.

### Coming up (in order)

- **Stage C (3 commits).** v1 `docs/SPEC.md`, `docs/PLAN.md`
  (lightweight, references ADR groups), `docs/CONVENTIONS.md`.
- **Stage D (23 commits).** ADRs 031–053. ADR-031 is the master
  scope-reduction decision; ADR-032 establishes the v1 frontmatter
  conventions all subsequent ADRs follow; ADRs 033–053 cover Go
  module layout, CLI framework, configuration, session metadata,
  workstream layout, multi-agent model, tmux, SSH remote, sandbox,
  agents, doctor, `af setup`, `af suspend`/`resume`, Obsidian, proxy
  commands, secrets, lint, testing, formal verification, and build.
- **Stage E (1 commit).** `docs/adr/INDEX.md` linking v1 ADRs and
  pointing at the v0 archive for legacy ADRs.

### After the doc pass lands

The Rust source tree (`src/`, `Cargo.toml`, etc.) stays in-tree as a
write-reference while v1 is implemented. It is removed in a separate
commit once Go has functional parity, then accessible only via git
history.

Implementation lanes are not pre-phased; each ADR carries its own
`implementation: pending → in-progress → complete` frontmatter and
is picked up in the order their ADR dependency graph dictates.

### Decisions ratified this session (pre-ADR-acceptance)

These come from prompt-mode discussion. Each is captured in an ADR
proposal during Stage D:

- Go, single-user, no releases, atomic commits, pedantic lint.
- Stdlib-first dep policy. Approved candidate deps: `cobra`,
  `BurntSushi/toml`, `google/uuid`, `gopkg.in/yaml.v3`,
  `zalando/go-keyring`, `rogpeppe/go-internal/testscript`.
- Multiplexer: tmux only. Remote: SSH only (alias / user@host / IP).
  Sandbox: slicer + sbx. Agents: pi (default), claude, codex.
- Two new commands beyond v0 surface: `af setup`, `af suspend`.
- Worktree layout: stable `~/Workspace/.worktrees/<repo>/<branch>/`;
  sub-worktrees on sibling branches `<branch>--<slot>` for subagents.
- Per-repo discovery state: `<repo>/.af/state.toml` symlinked to
  global; `af setup` adds `.af/` to global `~/.config/git/ignore`.
- Obsidian: notes per workstream with versioned frontmatter; one
  example `.base` aggregator file shipped under `examples/obsidian/`.
- Editor / diff / pr: thin wrappers over config-defined commands.
- Build: Make replaces `just`; goreleaser cross-compiles
  `linux/{amd64,arm64}`, `darwin/{amd64,arm64}`.

### Next session

Continue Stage B (TODO.md, README.md, CLAUDE+AGENTS.md), then Stage C
(SPEC, PLAN, CONVENTIONS), then Stage D (ADRs 031-059).

---

## 2026-05-08 — Session 1: implementation DAG captured

### Goal

Turn the settled v1 ADR set into an implementation checklist that can be
worked from `TODO.md`, with static checks and test scaffolding before
feature work.

### Done

- Replaced the placeholder post-doc-pass section in `TODO.md` with a
  topologically sorted implementation plan for ADRs 034–059.
- Front-loaded Go scaffold, lint, format, build, and test harness setup
  before user-facing feature commands.
- Grouped later work into dependency-ordered stages: foundations,
  external-system fakes, utility commands, local workstream MVP,
  lifecycle/cleanup/stacking, remote/sandbox/secrets, proxies/AI/retro,
  and final hardening/v0 retirement.
- Updated `docs/PLAN.md` so it points to `TODO.md` as the operational
  checklist instead of saying no implementation phase artifact exists.

### Next

Start with `TODO.md` item I0.1: scaffold the Go module and package tree
from ADR-034 while keeping the v0 Rust tree read-only.

---

## 2026-05-09 — Session 2: Go module scaffold

### Goal

Start implementation from `TODO.md` by completing I0.1: create the Go
module scaffold while preserving the Rust v0 tree as read-only reference.

### Done

- Created `go.mod` for `github.com/kakkoyun/af` with the local Go 1.26
  toolchain pin.
- Added `cmd/af/` with a scaffold-only, context-aware entrypoint. The
  binary deliberately reports that the cobra command tree lands in I0.2.
- Followed the red/green cycle: `go test ./...` first failed on missing
  `run` / sentinel errors, then passed after implementation.
- Added package doc scaffolds for the planned `internal/...` packages
  from ADR-034 and placeholder `examples/` directories.
- Updated `TODO.md`, `CHANGELOG.md`, `README.md`, and ADR-034
  implementation frontmatter for the scaffold.

### Verification

- `gofmt -l cmd/af internal` produced no output after formatting.
- `gofumpt -l cmd/af internal` and `goimports -l -local
github.com/kakkoyun/af cmd/af internal` produced no output.
- `go build ./cmd/af` passes.
- `go test -race -count=1 ./...` passes.
- `go vet ./...` passes.
- `go list ./... | xargs -n 1 go doc` passes.
- `golangci-lint run` reports `0 issues`.

### Notes

- The pre-existing formatting-only diff in
  `docs/adr/037-session-metadata-schema.md` was left untouched.
- No v0 Rust files were modified.

### Next

Continue with `TODO.md` item I0.2: add the minimal cobra root command,
persistent root flags, `af version`, and `internal/version` build-info
wiring.

---

## 2026-05-09 — Session 3: cobra root + version

### Goal

Complete I0.2 by replacing the scaffold-only entrypoint with the minimal
cobra root command, persistent root flags, `af version`, and build-info
wiring from ADR-035 and ADR-053.

### Done

- Added the approved cobra dependency and generated `go.sum`.
- Added `internal/version` with link-time-overridable `Version`,
  `Commit`, and `Date` metadata plus `version.String()`.
- Replaced the scaffold-only `run` path with a cobra root command using
  `ExecuteContext`, root persistent flags (`--verbose` / `-v`,
  `--config`, `--session`), root help output, and `af version`.
- Followed TDD: new tests first failed on the missing cobra dependency,
  missing `internal/version`, and missing root constructors; after
  implementation, the package tests passed.
- Updated `TODO.md`, `CHANGELOG.md`, `README.md`, ADR-035, and ADR-053
  implementation frontmatter.

### Verification

- `gofmt -l cmd/af internal` produced no output.
- `gofumpt -l cmd/af internal` and `goimports -l -local
github.com/kakkoyun/af cmd/af internal` produced no output after
  formatting.
- `go build -o /tmp/af-scaffold ./cmd/af` passes.
- `go test -race -count=1 ./...` passes.
- `go vet ./...` passes.
- `go list ./... | xargs -n 1 go doc` passes.
- `golangci-lint run` reports `0 issues`.

### Notes

- The pre-existing formatting-only diff in
  `docs/adr/037-session-metadata-schema.md` remains untouched.
- No v0 Rust files were modified.

### Next

Continue with `TODO.md` item I0.3: add `.golangci.yml`, `Makefile`,
format/lint/test/check targets, and local snapshot build targets.

---

## 2026-05-09 — Session 4: remove Rust v0 source/tooling

### Goal

Honor the user's explicit override to remove the Rust v0 source/tooling
now that the Go rewrite has started, instead of keeping it in-tree until
parity.

### Done

- Removed tracked Rust-era source and tests (`src/`, `tests/`).
- Removed Rust-era tooling/config (`Cargo.toml`, `Cargo.lock`,
  `.cargo/`, `clippy.toml`, `deny.toml`, `rust-toolchain.toml`,
  `rustfmt.toml`, `justfile`) and the local `target/` build output.
- Updated `README.md`, `AGENTS.md`, `CLAUDE.md`, `docs/CONVENTIONS.md`,
  `docs/PLAN.md`, ADR-031, `CHANGELOG.md`, and `TODO.md` so the project
  no longer tells agents to preserve a Rust working tree.

### Verification

- Checked that stale "keep Rust read-only until parity" wording is gone
  from active project docs.
- Confirmed `.cargo/`, `Cargo.toml`, `Cargo.lock`, `clippy.toml`,
  `deny.toml`, `rust-toolchain.toml`, `rustfmt.toml`, `justfile`, `src/`,
  `tests/`, and `target/` no longer exist in the working tree.
- `gofmt -l cmd/af internal` produced no output.
- `gofumpt -l cmd/af internal` and `goimports -l -local
github.com/kakkoyun/af cmd/af internal` produced no output.
- `go build -o /tmp/af-scaffold ./cmd/af` passes.
- `go test -race -count=1 ./...` passes.
- `go vet ./...` passes.
- `go list ./... | xargs -n 1 go doc` passes.
- `golangci-lint run` reports `0 issues`.

### Notes

- The previous read-only stance came from the project constitution,
  AGENTS/CLAUDE guidance, ADR-031, and TODO I8.4. The user explicitly
  superseded that plan to keep the rewrite focused and avoid Rust files
  slowing searches/lint/context.
- `docs/v0/**` remains the frozen v0 documentation archive; deleted Rust
  source remains available through git history.
- The pre-existing formatting-only diff in
  `docs/adr/037-session-metadata-schema.md` remains untouched.

### Next

Continue with `TODO.md` item I0.3: add `.golangci.yml`, `Makefile`,
format/lint/test/check targets, and local snapshot build targets.

---

## 2026-05-20 — Session 5: build tooling baseline

### Goal

Complete I0.3 by adding the pinned build, format, lint, test, check, and
snapshot tooling from ADR-050 and ADR-053.

### Done

- Captured the red baseline first: `make check` failed because no
  `check` rule existed.
- Added `Makefile` targets: `fmt`, `fmt-check`, `lint`, `test`,
  `test-property`, `check`, `build`, `install`, `release-snapshot`,
  `snapshot`, and `clean`.
- Added `.golangci.yml` using golangci-lint v2's `default: all`
  pedantic baseline with explicit, documented disables.
- Added `.goreleaser.yml` for local snapshot cross-compiles across
  `linux/{amd64,arm64}` and `darwin/{amd64,arm64}` with version ldflags.
- Added `.gitignore` entries for Go build artifacts (`bin/`, `dist/`,
  coverage output).
- Fixed lint findings surfaced by the new pedantic config in the
  existing scaffold tests and command wiring.
- Updated `TODO.md`, `CHANGELOG.md`, `README.md`, ADR-050, and ADR-053.

### Verification

- `make fmt-check` passes.
- `make lint` passes with `0 issues`.
- `make test` passes (`go test -race -count=1 -shuffle=on ./...`).
- `make check` passes.
- `make build` passes.
- `make release-snapshot` produced local cross-compile artifacts under
  `dist/`.
- `make clean` removes `bin/` and `dist/`.
- Final verification log: `/tmp/af-i0-3-verify-final.log`.

### Next

Continue with `TODO.md` item I0.4: add the `testscript` harness,
`cmd/af/testdata/script/`, fake external-command hooks, package
`testutil` helpers, and baseline smoke scripts for `af version` /
`af --help`.

---

## 2026-05-20 — Session 6: testscript smoke scaffold

### Goal

Complete I0.4 by adding the CLI `testscript` scaffold, reusable test
helpers, fake-command hooks, and baseline smoke scripts for the commands
implemented so far.

### Done

- Wrote the red test first: `cmd/af/testscript_test.go` and baseline
  scripts failed before `internal/testutil` and the `testscript`
  dependency existed.
- Added `github.com/rogpeppe/go-internal/testscript` as a dev test
  dependency.
- Added `internal/testutil` helpers for building the `af` binary in a
  temp directory, creating fake executable commands, creating test
  directories, and prepending to `PATH`.
- Wired the testscript setup to build `af`, prepend a per-scenario
  `bin/`, and expose `AF_TEST_FAKEBIN` for future fake external-command
  scenarios.
- Added smoke scripts for `exec af version` and `exec af --help`.
- Disabled cobra's default completion command so the planned `af
  completions <shell>` surface remains controlled by TODO item I3.2.
- Updated `TODO.md`, `CHANGELOG.md`, and ADR-051.

### Verification

- `go test ./cmd/af` passes.
- `make fmt-check` passes.
- `make lint` passes with `0 issues`.
- `make test` passes.
- `make check` passes.
- Final verification log: `/tmp/af-i0-4-verify.log`.

### Next

Continue with `TODO.md` item I0.5: add the property-test scaffold for
lifecycle and naming invariants without enabling formal verification as a
release gate.
