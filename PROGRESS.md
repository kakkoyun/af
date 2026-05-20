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
- Disabled cobra's default completion command so the planned `af completions <shell>` surface remains controlled by TODO item I3.2.
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

---

## 2026-05-20 — Session 7: property-test scaffold

### Goal

Complete I0.5 by adding property-test scaffolding for lifecycle and
naming invariants without making formal verification a release gate.

### Done

- Wrote the property tests first and confirmed `go test ./internal/...`
  failed because `internal/lifecycle` and workstream naming helpers did
  not exist yet.
- Added `internal/lifecycle` with pure state/event transition helpers for
  `active`, `suspended`, `completed`, and `abandoned` workstreams.
- Added lifecycle property tests for terminal-state stickiness,
  terminal states never returning to active, and suspend/resume
  round-trips.
- Added `internal/workstream` naming helpers for `Sanitize` and
  `ApplyPrefix`.
- Added naming property tests for sanitize idempotency and prefix
  idempotency.
- Updated `TODO.md`, `CHANGELOG.md`, and ADR-052.

### Verification

- `make fmt-check` passes.
- `make lint` passes with `0 issues`.
- `make test` passes.
- `make test-property` passes.
- `make check` passes.
- Final verification log: `/tmp/af-i0-5-verify.log`.

### Next

Continue with `TODO.md` item I0.6: run baseline verification now that
Stage 0 scaffold checks are in place, then update `PROGRESS.md` with the
first green baseline.

---

## 2026-05-20 — Session 8: Stage 0 green baseline

### Goal

Complete I0.6 by proving the scaffold, static checks, testscript smoke
harness, property tests, and snapshot build tooling all pass together.

### Done

- Ran the full Stage 0 baseline after completing I0.1 through I0.5.
- Confirmed `make check` passes on the scaffold.
- Confirmed `make test-property` passes separately for the deeper
  property-test target.
- Confirmed `make release-snapshot` produces local snapshot artifacts and
  `make clean` removes generated `bin/` / `dist/` outputs.
- Marked Implementation Stage 0 complete in `TODO.md`.

### Verification

- `make fmt-check` passes.
- `make lint` passes with `0 issues`.
- `make test` passes.
- `make test-property` passes.
- `make check` passes.
- `make build` passes.
- `make release-snapshot` passes.
- `make clean` passes.
- Baseline log: `/tmp/af-i0-6-baseline.log`.

### Next

Begin Implementation Stage 1 with `TODO.md` item I1.1: implement layered
TOML config loading, schema defaults, global-only sections, `~`
expansion, proxy command config shapes, and config tests.

---

## 2026-05-20 — Session 9: layered config loader

### Goal

Complete I1.1 by implementing the ADR-036 configuration loader before
any command depends on configuration.

### Done

- Wrote config tests first for missing-file defaults, user/repo layering,
  repo-only global section handling, unsupported schema versions, and
  invalid proxy command shapes.
- Added `github.com/BurntSushi/toml` as the TOML runtime dependency.
- Added `internal/config` schema types and compiled defaults for the v1
  config surface.
- Implemented `Load` / `LoadWithOptions` with defaults → user → repo
  merge order, missing-file tolerance, schema-version checks, and context
  cancellation checks.
- Implemented global-only handling for `[obsidian.vaults]` and
  `[secret]` so repo config cannot override machine-scoped values.
- Implemented `~` expansion for worktree, PR template, Obsidian template,
  and Obsidian vault paths.
- Implemented proxy command config shapes for argv-mode arrays and
  shell-mode strings, with final merged-shape validation.
- Marked `TODO.md` I1.1 complete and advanced ADR-036 implementation
  state.

### Verification

- `go test ./internal/config` passes.
- `make fmt-check` passes.
- `make lint` passes with `0 issues`.
- `make test` passes.
- `make check` passes.
- `go list ./... | xargs -n 1 go doc` passes.
- Final verification log: `/tmp/af-i1-1-verify.log`.

### Next

Continue with `TODO.md` item I1.2: implement the shared duration grammar
for `d`/`w` plus stdlib duration units with table and property tests.

---

## 2026-05-20 — Session 10: shared duration grammar

### Goal

Complete I1.2 by implementing the shared duration parser used by future
`af clean --max-age` and `af retro --since` flags.

### Done

- Wrote table tests first for valid day/week shorthand, mixed forms,
  stdlib-compatible units, and invalid inputs.
- Added property tests proving `Nd` / `Nw` expand to exact 24-hour days
  and 168-hour weeks, and that stdlib units match `time.ParseDuration`.
- Added `internal/duration` with `Parse`, integer token scanning, `d` /
  `w` conversion, overflow checks, and contextual errors.
- Marked `TODO.md` I1.2 complete and advanced ADR-056 / ADR-058
  implementation state for the shared grammar slice.

### Verification

- `go test ./internal/duration` passes.
- `make fmt-check` passes.
- `make lint` passes with `0 issues`.
- `make test` passes.
- `make test-property` passes.
- `make check` passes.
- `go list ./... | xargs -n 1 go doc` passes.
- Final verification log: `/tmp/af-i1-2-verify.log`.

### Next

Continue with `TODO.md` item I1.3: implement naming, branch-prefix
rules, session-name sanitization, sub-branch naming, and UUID/session-ID
derivation.

---

## 2026-05-20 — Session 11: workstream naming helpers

### Goal

Complete I1.3 by implementing pure naming helpers for workstream
branches, tmux-safe session names, subagent branches, and agent session
IDs before state/worktree code depends on them.

### Done

- Wrote naming tests first for double-dash tmux sanitization, branch
  prefix rules, auto-generated session names, sub-branch naming, and
  deterministic UUID session IDs.
- Added `github.com/google/uuid` as the approved UUID runtime
  dependency.
- Updated `Sanitize` to match ADR-038's `--` replacement rule for tmux
  session names.
- Added branch prefix helpers that respect `prefix_on_fork_only` and
  `upstream` remote presence.
- Added auto session-name, sub-branch, and UUID5 session-ID derivation
  helpers.
- Marked `TODO.md` I1.3 complete and advanced ADR-038 / ADR-039
  implementation state.

### Verification

- `go test ./internal/workstream` passes.
- `make fmt-check` passes.
- `make lint` passes with `0 issues`.
- `make test` passes.
- `make test-property` passes.
- `make check` passes.
- `go list ./... | xargs -n 1 go doc` passes.
- Final verification log: `/tmp/af-i1-3-verify.log`.

### Next

Continue with `TODO.md` item I1.4: implement `state.toml` and
`ledger.jsonl` read/write, atomic writes, locking, schema checks,
derived metadata, and current-workstream discovery.

---

## 2026-05-20 — Session 12: state and ledger persistence

### Goal

Complete I1.4 by implementing the first durable session-state layer:
`state.toml`, `ledger.jsonl`, locks, repo slug parsing, and discovery.

### Done

- Wrote state tests first for atomic state round-trip, newer schema
  rejection, ledger append / `last_touched_at`, GitHub repo slug
  parsing, `.af/state.toml` discovery, and lock file lifecycle.
- Added `internal/session` schema types for the ADR-037 v1 state shape.
- Implemented `ReadState` with schema-version checks and
  `ErrSchemaTooNew`.
- Implemented `WriteState` with `state.toml.tmp`, fsync, rename, and
  directory fsync.
- Implemented append-only JSONL ledger writes and `LastTouchedAt` from
  the latest event timestamp.
- Implemented flock-backed `LockFile` / `Unlock` helpers using
  `golang.org/x/sys/unix`.
- Implemented GitHub remote `repo_slug` parsing and current-workstream
  discovery by explicit session, upward `.af/state.toml` symlink, or
  tmux-session fallback.
- Updated ADR-037 / SPEC examples to use omitted optional timestamps
  instead of invalid TOML `null` values.
- Marked `TODO.md` I1.4 complete and advanced ADR-037 implementation
  state.

### Verification

- `go test ./internal/session` passes.
- `make fmt-check` passes.
- `make lint` passes with `0 issues`.
- `make test` passes.
- `make check` passes.
- `go list ./... | xargs -n 1 go doc` passes.
- Final verification log: `/tmp/af-i1-4-verify.log`.

### Next

Continue with `TODO.md` item I1.5: implement local worktree path
planning, `.af/state.toml` symlink handling, sub-worktree path planning,
and git cleanup planning.

---

## 2026-05-20 — Session 13: worktree planning helpers

### Goal

Complete I1.5 by implementing pure worktree layout, discovery symlink,
and cleanup-planning helpers before command code executes real git
operations.

### Done

- Wrote tests first for stable primary worktree paths, sibling
  sub-worktree paths / branches, idempotent `.af/state.toml` symlink
  creation, conflicting symlink detection, and safe cleanup plans.
- Added `internal/git` worktree planning types and helpers for primary
  and subagent worktrees.
- Added idempotent state discovery symlink creation with conflict errors
  for existing links or files pointing elsewhere.
- Added dry cleanup planning that removes all worktrees but only marks
  merged (or forced) sub-branches for deletion.
- Marked `TODO.md` I1.5 complete.

### Verification

- `go test ./internal/git` passes.
- `make fmt-check` passes.
- `make lint` passes with `0 issues`.
- `make test` passes.
- `make check` passes.
- `go list ./... | xargs -n 1 go doc` passes.
- Final verification log: `/tmp/af-i1-5-verify.log`.

### Next

Continue with `TODO.md` item I1.6: implement secret redaction handler
and the keyring interface with fakes; keep envelope transport disabled
until remote / sandbox stages.

---

## 2026-05-20 — Session 14: secret redaction and keyring fake

### Goal

Complete I1.6 by adding the local secret-management seams needed by
future auth and launch code, without enabling envelope transport yet.

### Done

- Wrote tests first for built-in / configured `slog` key redaction,
  nested group redaction, and fake keyring set/get/delete/list behavior.
- Added `NewRedactingHandler`, wrapping any `slog.Handler` and redacting
  built-in keys (`api_key`, `token`, `password`, `bearer`, `secret`,
  `auth`) plus config-provided extensions.
- Added a `Keyring` interface and `MemoryKeyring` fake for deterministic
  command and provider tests.
- Kept ephemeral envelope transport intentionally unimplemented until
  the remote / sandbox stages.
- Marked `TODO.md` I1.6 complete and advanced ADR-049 implementation
  state.

### Verification

- `go test ./internal/secret` passes.
- `make fmt-check` passes.
- `make lint` passes with `0 issues`.
- `make test` passes.
- `make check` passes.
- `go list ./... | xargs -n 1 go doc` passes.
- Final verification log: `/tmp/af-i1-6-verify.log`.

### Next

Continue with `TODO.md` item I1.7: implement Obsidian frontmatter
parse/emit helpers and note path resolution, fake-backed and without
command integration.

---

## 2026-05-20 — Session 15: Obsidian note primitives

### Goal

Complete I1.7 by implementing the pure/fake Obsidian note helpers that
future `af note` and lifecycle commands will use.

### Done

- Wrote tests first for versioned frontmatter parsing, frontmatter
  emission, opaque markdown body preservation, configured vault/folder
  path resolution, and fake note-store read/write behavior.
- Added `internal/obsidian` note types for `af_schema: 1`, agent slots,
  PR metadata, tags, and lifecycle timestamps.
- Added `ParseNote` and `EmitNote` using `gopkg.in/yaml.v3` while
  preserving the markdown body as an opaque string.
- Added `ResolveNotePath` for `[obsidian.vaults]`, `notes_vault`, and
  `notes_folder` routing.
- Added `Store` and `MemoryStore` as fake-backed persistence seams.
- Kept command integration, Obsidian URI opening, and lifecycle note
  updates for later stages.
- Marked `TODO.md` I1.7 complete and advanced ADR-047 implementation
  state.

### Verification

- `go test ./internal/obsidian` passes.
- `make fmt-check` passes.
- `make lint` passes with `0 issues`.
- `make test` passes.
- `make check` passes.
- `go list ./... | xargs -n 1 go doc` passes.
- Final verification log: `/tmp/af-i1-7-verify.log`.

### Next

Continue with Stage 2, starting `TODO.md` item I2.1: implement the
agent interface, provider registry, fake provider, and provider
availability checks.

---

## 2026-05-20 — Session 16: agent provider seams

### Goal

Complete I2.1 by adding the provider abstraction and fake-backed seams
before any command depends on real agent CLIs.

### Done

- Wrote tests first for known-agent fallback order, registry lookup,
  first-available selection, provider command rendering, `BodyCmd`, and
  PATH-based executable availability.
- Added `Agent`, `LaunchOpts`, `ResumeOpts`, `BodyOpts`, and
  `ApprovalMode` in `internal/agent`.
- Added pi, claude, and codex provider implementations with launch,
  resume, non-interactive body-generation, availability, and log-path
  methods.
- Added `Registry`, `DefaultRegistry`, known-agent fallback order, and
  explicit errors for unknown / unavailable agents.
- Added `Fake` provider for future command tests that must not require
  real pi/claude/codex binaries.
- Kept command integration and provider invocation for later stages.
- Marked `TODO.md` I2.1 complete and advanced ADR-043 implementation
  state.

### Verification

- `go test ./internal/agent` passes.
- `make fmt-check` passes.
- `make lint` passes with `0 issues`.
- `make test` passes.
- `make check` passes.
- `go list ./... | xargs -n 1 go doc` passes.
- Final verification log: `/tmp/af-i2-1-verify.log`.

### Next

Continue with `TODO.md` item I2.2: implement the multiplexer interface,
fake mux, and tmux command construction without requiring real tmux in
tests.

---

## 2026-05-20 — Session 17: tmux multiplexer seam

### Goal

Complete I2.2 by adding the tmux-only multiplexer seam, command builder,
and fake implementation before command integration.

### Done

- Wrote tests first for tmux create-session command construction,
  vertical split pane-id parsing, and fake multiplexer session/pane/env
  behavior.
- Added `Multiplexer`, `Session`, `Pane`, and errors in `internal/mux`.
- Added `Tmux` with an injectable `Runner`, `ExecRunner`, and command
  construction for create/kill/attach/send-keys/env/options/list/split
  pane operations.
- Added `RecordingRunner` so tests assert argv without touching a real
  tmux server.
- Added `FakeMultiplexer` for future command tests.
- Kept CLI command integration for later stages.
- Marked `TODO.md` I2.2 complete and advanced ADR-040 implementation
  state.

### Verification

- `go test ./internal/mux` passes.
- `make fmt-check` passes.
- `make lint` passes with `0 issues`.
- `make test` passes.
- `make check` passes.
- `go list ./... | xargs -n 1 go doc` passes.
- Final verification log: `/tmp/af-i2-2-verify.log`.

### Next

Continue with `TODO.md` item I2.3: implement SSH remote command
construction, remote path mapping, and fake remote executor.

---

## 2026-05-20 — Session 18: SSH remote seam

### Goal

Complete I2.3 by adding the SSH command-construction seam, remote clone
path mapping, and fake executor before command integration.

### Done

- Wrote tests first for SSH argv construction with configured options,
  remote clone path mapping, remote probe command construction, and fake
  executor command capture / queued output.
- Added `Command`, `Executor`, `ExecExecutor`, and `FakeExecutor` in
  `internal/remote`.
- Added `SSH` with opaque host handling and options prepended exactly as
  ADR-041 specifies.
- Added `ClonePath` for `~/af-clones/<repo>/<branch>` and
  `ProbeCommand` for remote tool availability checks.
- Kept clone/launch/attach command integration for later stages.
- Marked `TODO.md` I2.3 complete and advanced ADR-041 implementation
  state.

### Verification

- `go test ./internal/remote` passes.
- `make fmt-check` passes.
- `make lint` passes with `0 issues`.
- `make test` passes.
- `make check` passes.
- `go list ./... | xargs -n 1 go doc` passes.
- Final verification log: `/tmp/af-i2-3-verify.log`.

### Next

Continue with `TODO.md` item I2.4: implement sandbox provider
interfaces, fake sandbox, and slicer/sbx command construction.

---

## 2026-05-20 — Session 19: sandbox provider seams

### Goal

Complete I2.4 by adding the sandbox provider abstraction, slicer/sbx
command builders, and fake sandbox before command integration.

### Done

- Wrote tests first for known-provider order, slicer launch command
  construction, sbx launch command construction, and fake sandbox
  launch/health/list/teardown behavior.
- Added `Sandbox`, `LaunchOpts`, `Handle`, `Command`, `Runner`, and
  `ExecRunner` in `internal/sandbox`.
- Added slicer and sbx provider implementations with launch, attach,
  health, teardown, list, availability, and attach-command construction.
- Added `RecordingRunner` so tests assert argv without touching real
  slicer/sbx binaries.
- Added `Fake` sandbox provider for future command tests.
- Kept CLI command integration for later stages.
- Marked `TODO.md` I2.4 complete and advanced ADR-042 implementation
  state.

### Verification

- `go test ./internal/sandbox` passes.
- `make fmt-check` passes.
- `make lint` passes with `0 issues`.
- `make test` passes.
- `make check` passes.
- `go list ./... | xargs -n 1 go doc` passes.
- Final verification log: `/tmp/af-i2-4-verify.log`.

### Next

Continue with `TODO.md` item I2.5: wire command-facing code to fakes in
tests so no unit or testscript path requires real tmux, ssh, slicer,
sbx, or agent CLIs.

---

## 2026-05-20 — Session 20: testscript fake PATH wiring

### Goal

Complete I2.5 by ensuring command-facing tests shadow external CLIs with
fakes rather than requiring real tmux, ssh, slicer, sbx, or agent binaries.

### Done

- Wrote `fake-path.txt` first and confirmed it failed by invoking the
  real local `tmux` because the testscript fake-bin directory was only
  exported as `AF_TEST_FAKEBIN`, not prepended to `PATH`.
- Updated the testscript setup to write fake `tmux`, `ssh`, `slicer`,
  `sbx`, `pi`, `claude`, and `codex` executables per scenario.
- Prepended the fake-bin directory after the built `af` binary directory
  and before the host `PATH`.
- Added a regression script that executes each fake binary and asserts
  the fake output.
- Marked `TODO.md` I2.5 complete.

### Verification

- `go test ./cmd/af -run TestScripts/fake-path` passes.
- `make fmt-check` passes.
- `make lint` passes with `0 issues`.
- `make test` passes.
- `make check` passes.
- `go list ./... | xargs -n 1 go doc` passes.
- Final verification log: `/tmp/af-i2-5-verify.log`.

### Next

Continue with Stage 3, starting `TODO.md` item I3.1: implement
`af config init` and `af config show`.

## 2026-05-21 — Session 21: af config init and show

### Goal

Complete I3.1 by adding `af config init` (write annotated user-config
template) and `af config show` (print effective merged configuration as
TOML).

### Done

- Verified the previous implementer's Stage 2 claims: I2.1–I2.5 are
  substantially complete; `make check` was green; coverage for the four
  Stage 2 seams is 41–77%; testscript `fake-path.txt` actually exercises
  the shadowed external CLIs; ADRs 040–043 and 051 carry the correct
  `implementation: in-progress` frontmatter. Reverted a stray uncommitted
  PROGRESS.md whitespace regression on line 335 (lost continuation
  indent under a wrapped bullet).
- Wrote the failing tests first: `internal/config/render_test.go`
  (section coverage, argv/shell polymorphism, deterministic vault
  ordering, round-trip through `Load`), `internal/config/template_test.go`
  (decodes to `Defaults()`, annotation markers, parent-dir creation,
  overwrite refusal, empty path), and `cmd/af/config_test.go` (help,
  default path under `$HOME`, `--config` override, refuse overwrite,
  effective merge, defaults-only fallback).
- Implemented `internal/config/render.go` as a hand-rolled deterministic
  TOML renderer: every section is emitted in a stable order; map keys
  (`pr.flag_template`, `obsidian.vaults`) are sorted; `ProxyCommandConfig`
  is rendered as either `cmd = [...]` or `cmd = "..."` per ADR-036.
- Implemented `internal/config/template.go`: `UserConfigTemplate()`
  returns the annotated TOML body; `WriteUserConfig(path)` creates
  parent dirs at 0750, writes the file at 0600, and returns a wrapped
  `fs.ErrExist` when the target already exists. Exported
  `ResolveUserConfigPath` so command code reuses the same default path.
- Implemented `cmd/af/config.go` with constructor-per-subcommand idiom
  (`newConfigCmd`/`newConfigInitCmd`/`newConfigShowCmd`); wired into
  `newRootCmdWithOptions` so `--config` flows through.
- Added `testdata/script/config-init.txt` and `config-show.txt`
  testscript scenarios.
- Resolved eight lint findings on the first pass: contextcheck false
  positive on the cobra constructor chain silenced at `main.go:35` with
  a documented `//nolint:contextcheck` comment; err113 replaced a
  dynamic error string with a wrapped `fs.ErrExist`; gosec G304 on test
  reads suppressed with a tempdir-scope comment; three noinlineerr
  rewrites; one staticcheck De Morgan simplification.
- Updated `TODO.md` (I3.1 ✓), `CHANGELOG.md`, and ADR-036
  `last_modified` (frontmatter `implementation: in-progress` remains
  until all command-facing ADR-036 surface lands; `af setup` will pick
  up `WriteUserConfig` next).

### Verification

- `go test -race -count=1 -shuffle=on ./...` passes.
- `go test -v -run 'TestScripts/(config-init|config-show)' ./cmd/af`
  passes.
- `make fmt-check` passes.
- `make lint` passes with `0 issues`.
- `make test` passes.
- `make check` passes.
- `go list ./... | xargs -n 1 go doc` passes; `Render`,
  `UserConfigTemplate`, `WriteUserConfig`, and `ResolveUserConfigPath`
  carry first-word doc comments.

### Next

Continue with `TODO.md` item I3.2: implement `af completions <shell>`
(ADR-035 + ADR-045) using cobra's built-in completion generators.

## 2026-05-21 — Session 22: af completions

### Goal

Complete I3.2 by adding `af completions <bash|zsh|fish|powershell>` via cobra's built-in generators.

### Done

- Wrote failing tests first: `cmd/af/completions_test.go` (bash, zsh, fish, powershell scripts; unknown-shell error; missing-arg error) and `testdata/script/completions.txt`.
- Implemented `cmd/af/completions.go` with `newCompletionsCmd` dispatching to `root.GenBashCompletion`, `GenZshCompletion`, `GenFishCompletion(_, true)`, and `GenPowerShellCompletionWithDesc`. Static sentinel `errUnsupportedShell` wraps the unknown-shell case for err113 conformance.
- Wired into root command tree.
- Updated TODO, CHANGELOG, ADR-035 last_modified.

### Verification

- `go test -race -count=1 -shuffle=on ./...` passes.
- `make check` passes (0 lint issues).

### Next

Continue with `TODO.md` item I3.3: implement local `af doctor` (ADR-044) using the existing interface seams.

## 2026-05-21 — Session 23: af doctor (local + remote)

### Goal

Complete I3.3 and I3.4 by adding `af doctor` (local) and `af doctor --remote <host>` per ADR-044, plus the supporting `internal/doctor` package.

### Done

- New `internal/doctor` package: Tier enum (Must/Should/Nice), Probe struct with OR-group support for the agent trio, Lookup interface (`LookPath(ctx,name)`, `Version(ctx,binary)`), Run aggregator, Render emitter with per-platform install hints.
- `internal/doctor/system.go`: SystemLookup over `os/exec`, ParseOSRelease, DetectPlatform with Darwin shortcut and Arch/Debian classification via /etc/os-release.
- `internal/doctor/remote.go`: RemoteCommander seam (satisfied by `internal/remote.SSH`), RemoteLookup using `command -v` and `<bin> --version || true`, RemoteOSRelease via `cat /etc/os-release`, DetectRemotePlatform.
- `cmd/af/doctor.go`: `af doctor [--remote host] [--verbose]` wired into root; loads layered config to pick up `[doctor].extra_tools` and `[remote].ssh_options`; renders the report; exits 1 on missing TierMust tools.
- testscript scenario `doctor.txt` exercises the local path through the fake-PATH shadowing (git/tmux/pi all fakes).
- testscript fakebin extended to include `git` so doctor scenarios are hermetic.
- Resolved a long lint pass (~14 issues across govet/cyclop/err113/errcheck/nilerr/funlen/perfsprint/nolintlint/revive/contextcheck) without regressions.
- Updated TODO (I3.3, I3.4 ✓), CHANGELOG, and ADR-044 (`implementation: in-progress`, `last_modified: 2026-05-21`).

### Verification

- `go test -race -count=1 -shuffle=on ./...` passes.
- `make check` passes (0 lint issues).
- `go doc ./internal/doctor` lists Probe, Lookup, Run, Render, SystemLookup, RemoteLookup, DetectPlatform, DetectRemotePlatform.

### Next

Continue with `TODO.md` item I3.5: implement `af setup` (ADR-045 + ADR-049) — state directory creation, config init, global gitignore update, completion install, secrets directory, Obsidian vault hint.

## 2026-05-21 — Session 25: Stages 4–8 closeout via parallel agents

### Goal

Drive every remaining TODO item to `[x]` and land Stage 8 (property tests, docs sync, ADR frontmatter audit) in parallel.

### Done

This session combines work from a single-threaded lead pass (Stages 4–7) and a final four-way parallel fan-out (Stage 8).

Lead-pass commits (single thread):

- I4.1: `af create [name]` orchestrator in `internal/lifecycle.Create` + cmd wiring. New `internal/git.Runner` seam with `ExecRunner` and `FakeRunner`.
- I4.2 / I4.3: `af list` and `af info [--json] [--ledger N]`. New `session.ReadLedgerTail`.
- I4.4 / I4.5 / I4.6: `af agent` (list/add/stop), `af done [--force]`, `af session-branch`.
- I5.1 / I5.2 / I5.3 / I5.4 / I5.5 / I5.6: `af suspend`, `af resume [--bare]`, `af note --append`, `af clean [--dry-run --include-abandoned --max-age D --force]`, `af status [--json --all --filter]`, `af stack/unstack/sync`.
- I6.1 / I6.2 / I6.3 / I6.4 / I6.5: `internal/secret.Envelope` ephemeral env-file writer; `lifecycle.PrepareRemoteWorkstream` + `LaunchSandboxWorkstream` using the existing remote/sandbox seams; `af create --remote/--sandbox` flags wired through.
- I7.1 / I7.2 / I7.3 / I7.4 / I7.5 / I7.6: new `internal/proxy` package with argv/shell token expansion; `af editor`, `af diff`, `af pr [--ai --ai-model]`, `af retro`.

Final parallel fan-out (four worker subagents, file-ownership-scoped):

- **Agent A (I8.1):** `internal/lifecycle/lifecycle_state_property_test.go` with seven `testing/quick`-style property tests for the lifecycle state machine. Covers terminal-state absorption, suspend/resume round-trip, idempotency on already-in-state events, Done/DoneForce terminality, and `EventFromIndex` totality.
- **Agent B (I8.3 docs):** 219-line README rewrite covering quickstart, the full ADR-035 command tree, configuration pointers, caveats (real SSH/sandbox not battle-tested; `af pr --ai`, `af retro --ai`, `af sync` are placeholders).
- **Agent C (I8.3 ADRs):** Frontmatter audit across 23 ADRs. Set `implementation: complete` for 035, 036, 037, 038, 039, 043, 044, 045, 047, 051, 054, 055, 056, 058. Set `implementation: in-progress` for 046, 048, 057, 059 (placeholders / deferred). Left 040/041/042/049/052/053 at `in-progress` (scaffold + tests pending). Bumped every touched `last_modified` to 2026-05-21.
- **Agent D (I8.2 tests):** 14 happy-path tests across `cmd/af/{suspend_resume,note,clean,status,stack,proxy_commands,retro}_test.go`, all using the established `executeCommand` + `writeTestSessionState` helpers. `make check` green.

### Conflict handling

Agent B initially overwrote `PROGRESS.md` with a 10-line stub when it tried to update the file as part of its docs sync. The coordinator detected the regression in `git status`, reverted via `git checkout HEAD -- PROGRESS.md`, and (this session entry) is the authoritative update. No other agent touched outside their declared file boundaries.

### Verification

- `make check` passes (0 lint issues, all packages green with `-race -count=1 -shuffle=on`).
- `go test ./internal/lifecycle/... -count=1` passes including the new property tests.
- 16 cmd/af test files total; 14 new tests from Agent D.
- 23 ADRs touched by Agent C, frontmatter only.

### Stage status after this session

- Stages 0–3: ✅ complete.
- Stage 4 (local MVP): ✅ complete (6/6).
- Stage 5 (lifecycle commands): ✅ complete (6/6).
- Stage 6 (remote+sandbox+secret): ✅ scaffolded (5/5). Real SSH/slicer/sbx integration tests remain a follow-up under ADR-040/041/042 frontmatter still `in-progress`.
- Stage 7 (proxy + retro): ✅ complete (6/6). `--ai` paths are placeholders pending ADR-057 wiring.
- Stage 8 (hardening): ✅ complete (4/4) for the implementation cut. Cross-compile snapshot and real-tool smoke remain release-time work.

### Next

v1 is feature-complete for single-user use. Outstanding follow-ups (each a small PR):

1. Wire `af pr --ai` to call `agent.BodyCmd` instead of writing a placeholder body (ADR-057).
2. Implement the `af sync` rebase algorithm (ADR-059).
3. Add `--from-pr N` and `--respawn` flags to `af create` and `af resume` (ADR-035 surface alignment).
4. Real SSH / slicer / sbx integration smoke tests (ADR-040/041/042).

## 2026-05-21 — Session 24: Stage 3 closeout (setup + auth)

### Goal

Finish Stage 3 by adding `af setup` (I3.5, ADR-045) and `af auth` (I3.6, ADR-049).

### Done

- `internal/setup` performs the seven idempotent ADR-045 steps (state dir tree, default config via `config.WriteUserConfig` from I3.1, global gitignore with optional `core.excludesfile` honouring, shell detection, bash/zsh/fish completion install, Obsidian vault hint, summary). Injected `GitConfigurer` and shell-generator funcs make every step hermetic in tests.
- `cmd/af/setup.go` wires cobra flags `--force`, `--shell`, `--skip-completions`, `--skip-gitignore`. Real git access goes through `os/exec`; tests use a fake.
- `internal/secret.SystemKeyring` wraps `zalando/go-keyring` and maintains an in-keyring index account so `List` works on top of the OS keyring API that has no native enumeration.
- `cmd/af/auth.go` adds `set`, `get`, `status`, `clear`, `list`. `set` reads via `term.ReadPassword` on a TTY (falls back to stdin); `get` prints plain on a TTY but emits `[REDACTED:abcd...]` on a non-TTY writer. Status lists the curated trio (anthropic/openai/github) plus any extras under "Other keyring entries:".
- `newAuthContextOverride` exposes a test seam so command-level integration tests can substitute a `MemoryKeyring` plus deterministic secret reader.
- Added dependencies: `github.com/zalando/go-keyring`, `golang.org/x/term`.
- TODO I3.5 ✓, I3.6 ✓, CHANGELOG entries, ADR-045 + ADR-049 frontmatter updated.

### Verification

- `make check` passes (0 lint issues).
- `go test -race -count=1 -shuffle=on ./...` passes.

### Next

Move into Stage 4 with `TODO.md` item I4.1: implement local `af create [name]` — the first feature slice that proves config, state, git, mux, and agent seams compose.

## 2026-05-22 — Session 26: Stage 9 — close out in-progress ADRs

### Goal

Follow the Session 25 audit: replace the three remaining placeholders
(`af pr --ai`, `af retro --ai`, `af sync`) with real implementations, add
`.goreleaser.yaml` + snapshot tooling, write the missing integration
testscripts for tmux / ssh / sandbox / proxy, wire `secret.Envelope`
into the remote+sandbox create flow, then advance every still-`in-progress`
ADR (040, 041, 042, 046, 048, 049, 052, 053, 057, 058, 059) to
`implementation: complete`.

### Pre-flight (confirmed before launching agents)

All tools present on host: `go 1.26.3`, `gofumpt`, `goimports`,
`golangci-lint`, `make`, `git 2.54.0`, `jq`, `tmux 3.6a`, `ssh
OpenSSH_10.2p1`, `gh 2.92.0` (authed as `kakkoyun`, scopes
`repo,workflow`), `pi`, `claude`, `codex`, `slicer`, `sbx`, `docker`,
`lima`, `security` (macOS Keychain), `gpg`, `goreleaser 2.15.4`,
`zig` (cross-compile).

### Plan

Wave 1 — four parallel subagents, each owns disjoint file globs:

- **A**: ADR-057 `af pr --ai` real BodyCmd wiring — owns
  `cmd/af/proxy_commands.go`, `cmd/af/proxy_commands_test.go`,
  `docs/adr/057-*.md` frontmatter.
- **B**: ADR-058 `af retro --ai` real BodyCmd wiring + frontmatter
  flip-then-complete — owns `cmd/af/retro.go`, `cmd/af/retro_test.go`,
  `docs/adr/058-*.md`.
- **C**: ADR-059 `af sync` rebase algorithm — owns `cmd/af/stack.go`
  (sync only), new `internal/lifecycle/sync.go`, new
  `internal/lifecycle/sync_test.go`, `docs/adr/059-*.md`.
- **D**: ADR-053 goreleaser + `make snapshot` — owns new
  `.goreleaser.yaml`, `Makefile` (snapshot target only),
  `docs/adr/053-*.md`.

After Wave 1 integration, Wave 2 (also four parallel agents):

- **E**: ADR-048 testscript scenarios for `editor`/`diff`/`pr` —
  `cmd/af/testdata/script/editor.txt`, `diff.txt`, `pr.txt`.
- **F**: ADR-040 + ADR-046 tmux integration testscript —
  `cmd/af/testdata/script/tmux-lifecycle.txt`.
- **G**: ADR-041 SSH localhost integration test —
  `cmd/af/testdata/script/ssh-remote.txt` (skipped if no sshd).
- **H**: ADR-042 + ADR-049 envelope-into-create wiring —
  `internal/lifecycle/remote_sandbox.go`, new envelope integration tests.

Wave 3 — close-out (lead, single-threaded):

- Advance ADR frontmatter to `complete` for everything that now ships
  real behaviour; update README/CHANGELOG/PROGRESS; check off TODO
  items I9.1–I9.10.

### Lead-coordinator rules for this session (re Session 25 conflict)

1. No subagent touches `PROGRESS.md` or `TODO.md`. The lead writes both.
2. Each subagent declares its file ownership at the top of its prompt
   and is explicitly forbidden from touching any other path.
3. Lead runs `git status` after every wave and reverts any file written
   outside the declared scope before integrating.
