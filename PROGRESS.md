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

### Done

Wave 1 (commit `b7ab875`):

- ADR-057 / I9.1: `af pr --ai` now calls `agent.BodyCmd` with the
  worktree diff. New `prAIBodyFunc` package-level seam. Sentinels:
  `errPRAIWebIncompatible`, `errPRAIEmptyDiff`, `errPRAIAgentNoBody`,
  `errPRAIEmptyBody`. Four new tests in `cmd/af/proxy_commands_test.go`.
- ADR-058 / I9.2: `af retro --ai` synthesises a narrative via
  `agent.BodyCmd` with `BodyOpts.Cwd = ""`. Adds `--ai-model` flag.
  New `retroAIBodyFunc` seam. Sentinels: `errRetroAINoNotes`,
  `errRetroAIEmpty`, `errRetroAINoCmd`. Five new tests.
- ADR-059 / I9.3: new `internal/lifecycle.Sync` orchestrator (193
  lines) implements fetch + merge-base + `rebase --onto`, detects
  CONFLICT output, errors on dirty worktree. Sentinels: `ErrSync`,
  `ErrSyncNoParent`, `ErrSyncConflict`, `ErrSyncDirtyWorktree`. Five
  unit tests; one uses a local `orderedFakeRunner` because
  `git.FakeRunner.SetResponse` is not call-count-aware.
- ADR-053 / I9.4: `.goreleaser.yaml` (v2 schema) plus `make snapshot`,
  `make snapshot-all`, `make release-check` targets. Cross-compile
  snapshot builds for darwin/arm64, linux/amd64, linux/arm64 all green
  (`CGO_ENABLED=0`). The legacy `.goreleaser.yml` ADR-template skeleton
  was deleted by the lead during integration; `/af` snapshot binary
  added to `.gitignore`.

Wave 2 (commit `<this commit>`):

- ADR-048 / I9.5: testscripts `editor.txt`, `diff.txt`, `pr.txt`.
  Each script writes an inline `~/.config/af/config.toml` plus state.toml
  for a fake `demo` workstream and asserts that the configured proxy
  command is invoked with the correct token substitutions and
  `flag_template` expansion.
- ADR-040 + ADR-046 / I9.6: testscript `tmux-lifecycle.txt` (100
  lines). A smart shell fake tmux maintains an in-test `sessions.txt`
  and supports `has-session`, `new-session`, `kill-session`. Three
  scenarios: suspend, non-bare resume (creates the session), bare
  resume (skips tmux).
- ADR-041 / I9.7: testscript `ssh-remote.txt`. Smart fake ssh
  responds to the exact probes `af doctor --remote` issues (`uname -s`,
  `command -v <tool>`, `<tool> --version`). Three cases: `demohost`
  all-present, `sparsehost` missing optionals, `failhost` exit 255.
- ADR-042 + ADR-049 / I9.8: `internal/lifecycle/remote_sandbox.go` now
  writes `secret.Envelope` 0600 before launch and deletes it via
  defer afterwards. Both `PrepareRemoteWorkstream` and
  `LaunchSandboxWorkstream` paths covered. New `SSHExecutor` field on
  `RemoteContext` for hermetic testing. Seven unit tests in new
  `remote_sandbox_test.go` use in-test fakes that capture envelope
  content during launch. The Task-C testscript for `af create
  --sandbox` was deliberately skipped: that CLI path doesn't yet call
  `LaunchSandboxWorkstream` end-to-end (deferred to ADR-060 work).

Wave 3 (lead-only close-out):

- ADR frontmatter advanced to `implementation: complete` for ADR-031
  (v1 master) and ADR-052 (formal verification). Every v1 ADR is now
  `complete`; only `pending` ADRs left are 060–064.
- README rewritten: status banner reflects Stage 9 complete; command
  table no longer says `--ai` is a placeholder; caveats now describe
  the actual remaining gaps (`af create --sandbox` not end-to-end,
  `af create --remote` runs minimal setup, both `--ai` paths require a
  non-interactive agent CLI).
- CHANGELOG updated with a Stage 9 section.
- TODO items I9.1–I9.10 all checked.

### Conflict log

- **Wave 1**: pre-existing untracked file `docs/adr/064-opinionated
  -diff-rendering.md` (date 2026-05-20) was caught by `git add -A`
  during the Wave 1 commit. It is a draft the owner wrote before this
  session; left in place rather than reverted because no agent claims
  authorship and the content is coherent.
- **Wave 1**: legacy `.goreleaser.yml` skeleton (predating Stage 9)
  shadowed Agent D's new `.goreleaser.yaml` because goreleaser
  auto-discovers `.yml` first. Lead deleted the legacy file.
- **Wave 1 and Wave 2**: cross-agent lint-state confusion. Each agent
  ran `make check` against a working tree that included other agents'
  in-flight changes. Three agents reported lint failures that the
  owner agent denied. Each time the lead's final integration check
  was green, so the failures were transient cross-agent races, not
  real defects.
- **No PROGRESS.md or TODO.md overwrites this session** — the file
  ownership rules held.

### Verification

- `make check` green after each wave commit and at the close-out
  commit. 0 lint issues, all packages pass `-race -count=1
  -shuffle=on`.
- `goreleaser check` green; `make snapshot-all` builds 3 cross-
  compile targets in ~2s.
- 13 testscripts (up from 8 at the start of Stage 9), 100% pass.
- 38 `*_test.go` files in the tree; lifecycle has 2 property-test
  files (older `lifecycle_property_test.go` + newer
  `lifecycle_state_property_test.go`) for a total of 11 properties.

### Stage status after this session

Every v1 ADR (031, 033–059) is now `implementation: complete`. The
v1 implementation is closed.

### Next

ADR-060 onward is genuinely new feature scope:

1. **ADR-060** — Slicer-only sandbox provider (drop sbx). Includes
   wiring `af create --sandbox` end-to-end through
   `LaunchSandboxWorkstream`.
2. **ADR-061** — Repo-scoped control settings (`.af/config.toml`).
3. **ADR-062** — Per-repo slicer VM resource profiles.
4. **ADR-063** — Remote control via Tailscale Serve + superterm.
5. **ADR-064** — Opinionated diff rendering (hunk + diffity).

## 2026-05-22 — Session 27: Stage 10 dispatched, PAUSED before integration

### Goal

Start implementing the five post-v1 ADRs (060–064) so every ADR in the
tree reaches `implementation: complete`. Plan:

- Wave 1: ADR-060 (slicer-only) + ADR-061 (repo control) + ADR-063
  (control commands) + ADR-064 (diff rendering) in parallel.
- Wave 2: ADR-062 (VM resource profiles) once 060+061 land.
- Wave 3: lead close-out (frontmatter, docs sync).

### Pre-flight (recorded for resume)

- `tailscale` 1.98.2 installed (daemon not running — fakes are used).
- `superterm` MISSING (test via fake binary, same pattern as tmux).
- `hunk`, `diffity`, `delta`, `difft`, `bat` all installed.
- `slicer` installed. **Important**: slicer's command surface has
  changed since ADR-062 was written. No more `slicer group` subcommand.
  Current surface: `slicer workspace`, `slicer vm`, `slicer env`,
  `slicer worktree`, `slicer claude`, `slicer codex`, `slicer amp`,
  `slicer opencode`, `slicer copilot`. ADR-062 Wave 2 work must adapt.

### Done (Wave 1, IN A WIP COMMIT — not yet verified)

Four subagents ran in isolated git worktrees with `worktree: true`.
The subagent harness merged each worktree back into the main working
tree on success, so all output now sits under uncommitted changes on
`main`. **`make check` has NOT been run against the integrated state.**

- **Agent A (I10.1 / ADR-060)**: dropped sbx end-to-end. New
  `sandbox.NewProvider(name) (Sandbox, error)`. `SBXConfig` deleted.
  `parseSandboxSection` now rejects `provider = "sbx"` with
  `errSandboxProviderUnsupported`. `cmd/af/create.go` now calls
  `lifecycle.LaunchSandboxWorkstream` for `--sandbox slicer` with a
  real `slicer vm run --name … --mount … -- <agent argv>` invocation
  (no stub). New tests: `TestKnownProviders_SlicerOnly`,
  `TestNewProvider_AcceptsSlicer`, `TestNewProvider_RejectsSBX`,
  `TestNewProvider_RejectsUnknown`, `TestSandboxConfig_RejectsSBXProvider`,
  `TestSandboxConfig_AcceptsSlicerProvider`, `TestCreate_SandboxFlagRejectsSBX`,
  `TestCreate_SandboxFlagRejectsDocker`,
  `TestCreate_SandboxProviderFactory_RejectsSBX`.
- **Agent B (I10.2 / ADR-061)**: added `[control]` config layer +
  `ControlConfig`, `ControlFlags`, `ControlContext`, `ResolveControl`.
  Additive state.toml fields: `Session.ApprovalMode`, `Session.MaxAgents`,
  `Execution.RemoteControl`. Side effect: removed dead
  `[sandbox.sbx]` block from `internal/config/render.go` to unblock
  compile after Agent A's SBXConfig delete. 12 new tests in
  `internal/config/control_test.go` and `internal/lifecycle/control_test.go`.
- **Agent C (I10.3 / ADR-063)**: new `internal/control` package
  (196 LoC) with `Up`, `Down`, `Status`, sentinels
  `Err{ProviderUnsupported, SupertermMissing, TailscaleMissing,
  SupertermStart, TailscaleServe, UnresolvableEndpoint}`. URL parsing
  via regex `https://[a-zA-Z0-9._-]+\.ts\.net\S*`. New `cmd/af/control.go`
  (205 LoC) wires cobra `af control up/down/status` with
  `--remote --provider --port --json` flags. Testscript
  `control-up.txt` with happy-path + missing-tool scenarios. 13 tests.
  `writeExternalFakes` extended to include `tailscale` and `superterm`.
- **Agent D (I10.4 / ADR-064)**: new `internal/diff` package (175
  LoC). Rewrote `cmd/af/proxy_commands.go`'s `runDiff` to delegate to
  `diff.Render`. Default terminal: hunk-piped when installed, else
  `git diff` plain; non-TTY: `git diff --stat`; `--web`: `diffity
  base..head`. Base resolution: explicit `--base` > `state.Stack.ParentBranch`
  > `state.Worktree.BaseBranch`. 6 unit tests in
  `internal/diff/diff_test.go`, 2 cmd-level tests, 3 testscript
  scenarios in `diff.txt`. Pre-emptively added `hunk`, `diffity`,
  `tailscale`, `superterm` to `writeExternalFakes` (expect a tidy-up
  merge with Agent C's identical-purpose addition).

### State on pause

- **Branch**: `main` at WIP commit (to be created in the same
  pause-flush as this PROGRESS entry).
- **Working tree**: clean after the WIP commit.
- **Stash**: `stash@{0}` holds the owner draft
  `docs/adr/065-slicer-worktree-transport.md` that was untracked when
  Wave 1 was dispatched; will be restored before pause finishes.
- **Untouched in this session**: nothing else — only Wave 1 fired.

### Recovery / resume procedure

1. `cd /Users/kemal.akkoyun/Workspace/Projects/Personal/af`.
2. `git log -3 --oneline` should show the WIP commit on top of
   `22172f1 docs(v1): Stage 9 close-out — every v1 ADR is complete`.
3. `git stash list` should show the ADR-065 draft stash.
4. Run `make check` against current HEAD.
5. **Expected failures**: gofumpt drift between Agent A's and Agent
   B's parallel `config.go` edits; possible duplicate entries in
   `writeExternalFakes` (both Agent C and Agent D added
   `tailscale`/`superterm`/`hunk`/`diffity`); possible
   noinlineerr/err113 drift at file boundaries. Fix sequentially.
6. Once `make check` is green:
   - `git reset --soft HEAD~1` to uncommit the WIP.
   - `git commit -m 'feat(v1): Stage 10 Wave 1 — close I10.1-I10.4 (ADR-060/061/063/064)'`
     with a proper body referencing the agent breakdown above.
7. Update TODO.md: convert `[~]` markers for I10.1–I10.4 to `[x]`.
8. Proceed to Wave 2 (ADR-062, depends on 060+061). When designing the
   Wave 2 agent task, remember that the slicer CLI no longer has
   `slicer group` — the implementation must adapt or explicitly defer.
9. Wave 3 close-out per the established pattern.

### Notes

- pi-rewind snapshots in this session (commits matching
  `pi-rewind:turn-019e4998-*`) preserve intermediate states and can
  be used to recover individual files via `git checkout <pi-rewind-sha>
  -- <path>` if needed.
- The four subagent worktrees themselves were ephemeral and have
  been cleaned up by the harness. All content survives in the
  main working tree (verified at pause time).

## 2026-05-22 — Session 28: Stage 10 close-out, every numbered ADR is complete

### Goal

Resume from Session 27's pause: verify Wave 1 integrates cleanly,
land Wave 2 (ADR-062), close out Wave 3 docs.

### Done

**Wave 1 integration verified** (commit `<wave-1-sha>`):

- `make check` was green on the first integration run. The worktree
  isolation strategy completely prevented the cross-agent lint drift
  that Session 26's no-worktree run had to chase. Every agent's
  separate concern compiled and tested without intervention.
- The owner committed ADR-065 (`docs(adr): adopt slicer worktree
  transport`) and ADR-066 (`docs(adr): define VM agent session export`)
  during the pause window; the stash that Session 27 set up to
  preserve ADR-065 was dropped as obsolete.
- ADR-066 had a markdown-table column-padding regression that the
  Wave 2 agent's read pulled in; reverted at integration time.
- ADR-067 (`Automatic Agent Session Export and Sync State`) appeared
  as a new untracked owner draft; left untracked per established
  pattern — the owner commits their own drafts on their schedule.

**Wave 2** (commit `<wave-2-sha>`):

- ADR-062 / I10.7 (single agent): `[sandbox.slicer.resources]` schema
  + `internal/sandbox/resources.go` (`SlicerResources`,
  `ManagedGroupName`, `GroupProber`, `ExecGroupProber`,
  `ResolveLaunchGroup`) + 8 additive state.toml fields + 14 new tests.
  Per-VM resource argv flags deferred with an inline ADR reference
  because slicer does not yet expose machine-readable per-group
  resource metadata; the managed-group approach means the group itself
  carries the shape and slicer creates VMs in it on demand. The Wave 2
  agent ran against the just-landed Wave 1 and finished with `make
  check` green on the first integration run.

**Wave 3** close-out (this commit):

- README.md: status banner now says "Stages 0–10 are implemented;
  every ADR from 031 to 064 is marked implementation: complete".
  Removed the "af create --sandbox not yet end-to-end" caveat (ADR-060
  fixed it). Replaced with two honest caveats: (a) slicer group-shape
  match is optimistic pending an upstream API; (b) ADR-065/066/067
  are owner drafts in flight.
- CHANGELOG.md: new "Stage 10 — post-v1 ADRs 060–064" section
  enumerates every shipped feature, package, and the aggregate (5
  ADRs advanced, 4 packages touched, test count 208 → 222).
- TODO.md: I10.1–I10.10 all `[x]`. The only remaining `pending` ADRs
  are 065/066/067, which are owner drafts and not part of Stage 10
  scope.

### Conflict log

- **Wave 1**: zero cross-agent issues at integration time. The
  worktree isolation paid off completely. The Session 27 PROGRESS
  entry pre-emptively documented likely failure modes (gofumpt drift
  in shared `config.go`, duplicate `writeExternalFakes` entries),
  none of which actually manifested — because each worktree merged
  cleanly on its own.
- **Wave 2**: a single owner-draft ADR (ADR-067) was caught by
  `git add -A` and unstaged before commit. ADR-066 received a
  markdown-table-formatter regression and was reverted at commit
  time. Both expected.
- **Owner activity during pause**: the owner committed ADR-065 and
  ADR-066 as standalone `docs(adr): …` commits while the session was
  paused. Detected on resume via `git log`; the stash that Session
  27 set up was dropped.

### Verification

- `make check` green after Wave 1 commit (`fa17597` parent context),
  after Wave 2 commit, and after this close-out.
- `goreleaser check` clean.
- 46 `*_test.go` files (up from 38 at start of Stage 9, no change
  from Wave 1 integration since the new tests are in existing files
  plus 4 new test files in Wave 1 + 2 new files in Wave 2).
- 222 test functions (up from 208 after Wave 1).
- 0 lint issues; all 20 packages pass `-race -count=1 -shuffle=on`.

### ADR state after this session

Every numbered ADR from 031 to 064 is `implementation: complete`.
The only `pending` ADRs are 065/066/067, all of which are owner
drafts in flight (slicer worktree transport, VM agent-session
export, automatic session sync).

### Next

Decide whether to:

1. Implement ADR-065/066/067 (the agent-session-sync triad) once the
   owner has finalised the drafts.
2. Cut a v1.0.0 release from current `main`: `goreleaser release
   --clean` after dry-run validation.
3. Address any TODO.md C-block stale tracker items (36 unchecked
   docs/ADR-commit tracker entries from Stage B–E that all shipped
   long ago).

## 2026-05-22 — Session 29: Stage 11 — ADR-065 slicer worktree transport

### Goal

Close the v1 ADR set up to and including ADR-065. The owner accepted
ADR-065 (slicer `wt push/pull` as the slicer sandbox transport) during
the Stage 10 close-out. This stage implements it.

Also: tidy `TODO.md` so the only `[ ]` items are real work, not the
36 stale C-block doc/ADR-commit tracker entries from Session 1.

### Done

**TODO tidy** (commit `702f589`):

- Marked C6–C43 as `[x]` (all of those doc/ADR commits shipped
  long ago). Stage B/C/D/E headers gain a `✅` badge.
- Added a "Post-v1 ADRs (060–065)" section mirroring Stage 10/11
  status for visibility.
- After this commit: 120 `[x]` / 6 `[ ]`, where the 6 unchecked
  items are the about-to-ship I11.1–I11.6.

**ADR-065 implementation** (commit `<stage-11-impl-sha>`):

Single agent against the just-tidied main. `make check` green on the
first integration run.

- `internal/session/state.go`: additive `[slicer_wt]` section with
  `SlicerWTState{VM, Path, PushedAt, PulledAt, LeaseState}`, constants
  `SlicerWTLeaseHeldByVM/Pulled/Discarded`, helper
  `State.IsLeasedToVM()`.
- `internal/sandbox/slicerwt.go` (new): `WTPush` and `WTPull`
  operations. `WTPushOptions{HostGroup, Depth, Tags, WorktreePath}`
  builds the argv `slicer wt push --launch [--hostgroup G] [--depth N]
  --tag af [--tag af-session=NAME] [--tag extra...] <worktree-path>`.
  VM-name parsing via permissive regex (matches "Launched VM
  <name>" and "VM: <name>") with a last-word fallback excluding
  noise words. Sentinels: `ErrSlicerWTPushFailed`,
  `ErrSlicerWTPullFailed`, `ErrSlicerWTNameNotFound`.
- `internal/sandbox/sandbox.go`: `Handle.VMName` field added; the
  `Launch` path for `--sandbox slicer` now calls `slicerWTLaunch`
  instead of the old `slicer vm run` argv. Old code removed.
- `internal/lifecycle/create.go`: after a successful slicer wt push,
  captures the lease in state (`vm` name, host `path`, `pushed_at`,
  `lease_state: held_by_vm`).
- `internal/lifecycle/pull.go` (new): `Pull` orchestrator with refusal
  sentinels `ErrPullNoLease`, `ErrPullAlreadyPulled`,
  `ErrPullDiscarded`, `ErrPullFailed`. On success sets
  `lease_state: pulled` and stamps `pulled_at`.
- `internal/lifecycle/{done,suspend_resume}.go`: both gain a
  `checkAndClearLease` helper. Without `--force`, refuse on
  `held_by_vm` with `ErrDoneLeasedToVM` / `ErrSuspendLeasedToVM`.
  With `--force`, mark the lease `discarded` before proceeding.
- `cmd/af/pull.go` (new): `af pull [session]` cobra wiring.
  `cmd/af/root.go` registers it.
- `cmd/af/{done,suspend_resume}.go`: `--force` flag plumbed through;
  `af resume` prints a one-line note when the lease is held.
- `cmd/af/proxy_commands.go`: `runPR` refuses on `held_by_vm` with
  `errPRWorktreeLeasedToVM`. `runEditor` and `runDiff` print a stderr
  warning suggesting `af pull` but do not refuse.
- `cmd/af/{status,info}.go`: text rows show `[vm=X lease=S]`; JSON
  exposes `slicer_wt_vm` / `slicer_wt_lease` on status, full
  `slicer_wt` block on `info --json`.
- `internal/doctor/system.go`: `SlicerWTAvailable` probe runs
  `slicer wt push --help` and confirms `--launch` is documented;
  surfaces as a non-blocking warning per the ADR.
- `docs/adr/065-slicer-worktree-transport.md`: `implementation:
  pending` → `complete`. `last_modified: 2026-05-22`.

**Wave 3 close-out** (this commit):

- README status banner now says "Stages 0–11 are implemented; every
  ADR from 031 to 065 is marked implementation: complete". Caveats
  updated: dropped ADR-065 from the pending-drafts list (it shipped);
  only ADR-066 and ADR-067 remain as owner drafts.
- CHANGELOG gains a Stage 11 section enumerating every shipped
  artefact for ADR-065.
- PROGRESS Session 29 (this entry) records the full Stage 11 path.
- TODO I11.1–I11.6 all `[x]`.

### Verification

- `make check` green after the implementation commit and after this
  close-out commit. 0 lint issues; all 21 packages pass
  `-race -count=1 -shuffle=on`.
- 7 new files (4 in `internal/{sandbox,lifecycle}`, 3 in `cmd/af`);
  18 modified.
- Test counts grew by 25+ functions covering push/pull argv, VM-name
  parsing, lease state round-trip, pull refusal sentinels, lease
  enforcement in done/suspend/pr, and lease display in status/info.

### Deferrals (documented inline in code)

- `af doctor` wiring of `SlicerWTAvailable` is exposed as a
  standalone function with a `// TODO(ADR-065)` comment to add it to
  `DefaultProbes` once the doctor's probe-tier mechanism is
  generalised for non-blocking warnings. Reachable via direct call;
  not yet surfaced in `af doctor` output.
- `TestEditor_LeaseWarning` was omitted because `runEditor` spawns
  a real external editor that hangs in tests. The warning code is
  present in the production path and verified by reading the
  source; production behaviour is correct.
- `af done --force` with a leased workstream sets `lease_state =
  discarded` in memory but the immediate archive-move means the
  archived `state.toml` is the persisted record. Documented in the
  Stage 11 commit message.

### ADR state after this session

Every numbered ADR from 031 to 065 is `implementation: complete`.
The only `pending` ADRs are 066 (VM agent-session export) and 067
(automatic agent-session sync), both owner drafts in flight.

### Next session pickup (handover)

The top of `TODO.md` now has a **Handover snapshot** block; Stage 12
below it captures the only remaining `[ ]` items:

1. **I12.1**: Wire `internal/doctor.SlicerWTAvailable` into
   `defaultProbes()` so `af doctor` actually surfaces the wt API
   warning. The probe function exists; only the registration + a
   one-line doctor-report assertion test are missing.
2. **I12.2**: Add a `TestEditor_LeaseWarning` that uses an injectable
   editor command so the test never blocks on a real editor. The
   warning emission lives in `cmd/af/proxy_commands.go.runEditor`;
   you may need to add a small package-level seam similar to
   `prAIBodyFunc` / `retroAIBodyFunc`.
3. **I12.3 + I12.4**: Implement ADR-066 (VM agent-session export)
   and ADR-067 (automatic agent-session sync). Specs are at
   `docs/adr/066-agent-session-export-from-slicer-vms.md` and
   `docs/adr/067-automatic-agent-session-export.md`. Parallelizable
   in worktrees if I12.3 lands the export module and I12.4 wires
   automatic triggers around it.
4. **I12.5**: Wave 3 close-out for Stage 12 (frontmatter, README,
   CHANGELOG, PROGRESS).

**Pre-flight for the next session** (verify these are still true):

- `make check` green on `main`.
- `slicer wt push --help` shows `--launch`.
- `tailscale` and `gh` still authenticated (Stage 9/10 caveats).
- The agent CLIs (`pi`, `claude`, `codex`) are still on PATH.

**Release readiness**: the project is in a release-ready shape
modulo the two pending owner drafts (066/067) and the small Stage 11
deferrals listed above. `goreleaser release --clean` is the path
when the owner is ready to cut v1.0.0; no code blockers remain.

---

## 2026-05-21 — Session 30: ADR-073 `af review` design

### Goal

Draft ADR-073 (`af review` — Repo-Aware PR Review Report), register it in
`docs/adr/INDEX.md`, add the Stage 14 implementation plan to `TODO.md`, and
record this session in `PROGRESS.md`. No code lands — doc before code per
the project constitution. The original plan targeted ADR-068, but the
gap-analysis batch (`9c6bd6e`) used 068–072, so the ADR is renumbered 073.

### Done

- Wrote `docs/adr/073-af-review-multi-prompt-report.md` — full ADR in the
  MADR shape used by ADR-057/058. Sections: Status, Context, Decision (13
  numbered sub-sections including the verbatim immutable system prompt),
  Consequences, Alternatives Considered, References.

- Updated `docs/adr/INDEX.md`:
  - Added catalogue row for ADR-073 (43rd entry, count 42→43).
  - Updated "next available is 073" → "next available is 074".
  - Added "New commands (073+)" conceptual grouping section.

- Added Stage 14 implementation plan to `TODO.md` (items I14.1–I14.6)
  covering: embedded system prompt (`//go:embed`), prompt builder with
  four-layer append resolution, `[review]` config five-touchpoint,
  `internal/gh` PR meta + diff helpers, `cmd/af/review.go` + testscript
  golden path + named failure modes, and the close-out checklist.

### Blockers

None. ADR-073 is `implementation: pending` by design — implementation
follows Stage 13 (gap-analysis batch ADRs 068–072).

### Next

Continue with Stage 12 (`TODO.md` items I12.1–I12.5): wire the
`SlicerWTAvailable` doctor probe, add the lease-warning editor test, and
implement ADR-066/067 VM session export and automatic sync state machine.

## 2026-05-22 — Session 31: Consolidation after gap-analysis + ADR-073

### Goal

Reconcile the work that landed between Session 29's handover commit
(`c00a37e`) and the current HEAD (`9f0227c`). Three parallel doc-only
branches landed in main during the gap window:

1. `docs/gap-analysis-v1` — ADRs 068–072 + SPEC rewrite (`9c6bd6e`,
   `92e7b89`, `2d7ad71`).
2. `worktree-docs-adr-068` — became ADR-073 (`e333225`) with Stage 14
   in `TODO.md`.
3. `docs/adr-073-spec-absorption` — SPEC pickup for ADR-073
   (`9f0227c`).

None of these commits changed code. The repository tree was
untouched outside `docs/` and the tracking files.

### Done

- Updated the **Handover snapshot** at the top of `TODO.md`:
  - HEAD reference advanced `2d7ad71` → `9f0227c`.
  - Added ADR-073 to the `pending` list (now 8 pending: 066, 067,
    068, 069, 070, 071, 072, 073).
  - Added a 5th "next session pickup option" pointing at Stage 14
    (ADR-073) and noted the ADR-071 dependency.
  - "29 commits ahead of origin" line replaced with "in sync with
    origin/main" (origin caught up during the gap window).
- Appended an ADR-073 paragraph to the "Stage 12 / 13 reading list"
  with the implementation outline (`//go:embed` prompt, four-layer
  append resolution, `[review]` config, `internal/gh`, atomic 0o600
  write).
- This PROGRESS entry (Session 31) records the consolidation.

### Verification

- `make check` is still green: 0 lint, all 21 packages pass
  `-race -count=1 -shuffle=on`. No code paths changed.
- ADR count: 43 files in `docs/adr/0*.md`. Status breakdown:
  34 `complete` (031, 033–065), 1 `n/a` (032), 8 `pending` (066–073).
- TODO checks: 126 `[x]` / 20 `[ ]`. The 20 unchecked items are
  Stages 12–14 (I12.1–I12.5, I13.1–I13.9, I14.1–I14.6) — the
  full forward plan for v1.0.0.
- Working tree clean after this commit; in sync with `origin/main`.

### Next

Unchanged from Session 30's pointer: continue with Stage 12. The
full pickup ladder (Stage 12 → 13 → 14, with ADR-073 depending on
ADR-071) is in the Handover snapshot in `TODO.md`.

## 2026-05-22 — Session 32: Stage 12 complete — ADR-066 + ADR-067

### Goal

Land the Stage 12 work in `TODO.md` (I12.1–I12.5). This closes the
two pending owner-draft ADRs from the Stage 11 handover (ADR-066 VM
agent-session export, ADR-067 automatic session sync) plus the two
small Stage 11 carry-overs (I12.1 doctor wt probe wiring, I12.2 editor
lease warning test).

Branch: `stage-12-followups-066-067` (not yet pushed; per the project
constitution every multi-commit change lands on a feature branch and
is merged into `main` separately).

### Done

**I12.1 + I12.2 carry-overs** (commit `c919db5`):

- `internal/doctor`: `Result` gains a `Note` field. A package-level
  `slicerWTChecker` seam (default: `SlicerWTAvailable`) is consulted
  by `Run` when the slicer probe resolves a binary; on a missing wt
  API the hint is attached as the Note. `Render` emits the Note as an
  indented `⚠ <hint>` sub-line after the probe row. Four internal
  tests cover the wired path.
- `cmd/af/proxy_commands.go` gains an `editorCommandFunc` seam mirroring
  the existing `prAIBodyFunc` / `retroAIBodyFunc` / `controlExecutorFactory`
  pattern. Two new tests assert the ADR-065 lease warning fires on
  `held_by_vm` and is suppressed when no lease exists.

**I12.3 — ADR-066** (commit `7022de3`):

- New package `internal/sandbox/sessiondata` with allowlist (Claude /
  Codex / pi / harness), Slicer interface (ExecSlicer over
  `sandbox.Runner`, FakeSlicer over an on-disk dir simulating the VM
  $HOME), manifest builder, Sync orchestrator (staging under
  `~/.local/share/af/v1/session-import/<session>/<vm>/<ts>/`), and
  merge engine with SHA-256 dedup + conflict quarantine.
- New CLI: `af session-data sync [session]` (renamed from `pull` in
  I12.4a per ADR-067) and `af session-data list [session]`. Flags:
  `--agent` (comma-separated or `all`), `--dry-run`, `--continue-host`
  (accepted but not yet wired; prints a stderr hint), `--vm` (override).
- Emits `agent_sessions_synced` ledger event with vm + kinds +
  imported/skipped/conflicts + staging path.
- **Pre-existing bug fix**: `internal/session/ledger_tail.go` parser
  matched `"type"` but the writer always emits `"event"`, so all
  round-tripped Event.Type values were empty. Surfaced by the
  session-data sync ledger test. Fixed parser to accept both keys.

**I12.4 — ADR-067** in three commits:

- `e4cbd34` (I12.4a) — state schema. New types `ExportState`,
  `ExportSource`, `ExportSyncStatus`, `ExportSourceStatus` on
  `internal/session`. Round-trip tests for the populated + empty
  sections. Renamed `sessiondata.Pull` → `Sync`, `PullOptions` →
  `SyncOptions`, etc., across the package + CLI. Ledger event
  `agent_sessions_pulled` → `agent_sessions_synced`. The CLI writes
  back `state.toml.[session_export]` after every successful sync with
  per-source cursors (one `SourceRecord` per merged file, including
  `hash`, `size`, `mtime`, `status`, `mode`).
- `258bc5b` (I12.4b) — append-aware JSONL merge. When a `*.jsonl`
  host destination is a byte-for-byte prefix of the VM source, sync
  appends only the missing tail via `io.CopyN` onto an `O_APPEND` fd
  and reports `Mode: append-jsonl`, `LastOffset: <pre-append size>`.
  Divergent or shrunken JSONLs continue to quarantine. Three new tests
  cover the three branches.
- `<lifecycle commit>` (I12.4c) — auto-sync hooks. New
  `cmd/af/session_data_lifecycle.go` with `autoSyncBeforeTeardown(cmd,
  state, statePath, discard)`. Wired into `af suspend` and `af done`
  before the destructive lifecycle step. A failed or conflicting sync
  blocks teardown and prints a recovery hint pointing to
  `af session-data sync <name>` or `--discard`. New `--discard` flag
  on both commands acknowledges transcript loss and records
  `last_sync_status=discarded`. Four tests cover the hook behaviour;
  pre-existing `TestSuspend_LeaseRefusal` / `TestSuspend_ForceAllowsWithLease`
  updated to install a no-op `FakeSlicer` so they exercise ADR-065
  without depending on a real slicer binary.

**I12.5 close-out** (this commit):

- ADR-066 frontmatter: `in-progress` → `complete`. ADR-067 frontmatter:
  `pending` → `complete`. `last_modified: 2026-05-22`.
- `docs/adr/INDEX.md` rows updated.
- TODO snapshot rewritten: ADRs 031–067 complete; `pending` reduced
  to 068/069/070/071/072/073 (the Stage 13 + 14 batch). I12.1–I12.5
  all `[x]`.
- README status banner advanced to "Stages 0–12 are implemented; every
  ADR from 031 to 067 is marked implementation: complete". New "Slicer
  VM session sync (ADR-066 + ADR-067)" command table with
  `af session-data {sync,list}` and the `--discard` flag on
  suspend / done. Caveats list updated: pending-drafts pointer moves
  to ADRs 068–073; ADR-066 `--continue-host` deferral spelled out.
- CHANGELOG gains a "Stage 12 — ADR-066 + ADR-067" section under
  `[Unreleased]` enumerating every shipped feature, file, and deferral.

### Verification

- `make check` is green throughout the stage: 0 lint, all 22 packages
  pass `-race -count=1 -shuffle=on`. (Up from 21 in Session 31; the
  new `internal/sandbox/sessiondata` adds the 22nd test target.)
- Test count grew by ~35 functions across new files in
  `internal/sandbox/sessiondata`, `cmd/af`, and `internal/session`.
- ADR count: still 43 files. Status: 36 `complete` (031, 033–067), 1
  `n/a` (032), 6 `pending` (068–073).
- TODO checks: 131 `[x]` / 15 `[ ]`. The 15 unchecked items are
  Stages 13 (I13.1–I13.9) and 14 (I14.1–I14.6).

### Deferrals (called out inline in code)

- **`--continue-host` path normalization** (ADR-066). The flag is
  accepted, prints a stderr hint, and falls back to analysis-only
  import. Implementing the per-agent format rewrites (Claude project
  keys, Codex session IDs, pi sessionDir headers) is its own piece of
  work; not part of Stage 12.
- **`af clean --force` ADR-067 hook.** Suspend and done are covered.
  Clean's interaction with VM-backed workstreams is uncommon; the hook
  lands when the clean reaper learns about slicer VMs.

### Next

Continue with Stage 13 — the gap-analysis batch (ADRs 068–072, items
I13.1–I13.9). ADR-070 (session resolution + fzf) and ADR-071 (PR state
TTL cache) are the natural starting points: they're new-behaviour
ADRs and unblock dashboard freshness + the `af review` work in
Stage 14. ADR-068's per-session flock lift can come alongside the next
mutating command. The pickup ladder is in `TODO.md`'s Handover
snapshot.

## 2026-05-22 — Session 33: Stages 13 (partial) + 14 complete — af review

### Goal

Continue from the Session 32 handover. Push as far as practical into
Stage 13 and Stage 14 in a single session.

Branch unchanged: `stage-12-followups-066-067`. Final state of this
branch covers everything from Session 32 plus the work below.

### Done

**Stage 13 partial** — three of nine items landed:

- **I13.2 (ADR-071 PR TTL refresh — partial)** in commit `eee79fa`:
  - `internal/session/state.go` PRState gains `LastRefreshedAt`
    (*time.Time, omitempty) and `LastRefreshError` (string, omitempty).
  - `internal/config/config.go` PRConfig gains `RefreshTTL`
    (time.Duration, default 10m via `defaultPRRefreshTTL`).
    New `[pr].refresh_ttl` TOML key parsed via the new
    `durationPointer` helper. Layer + parser + merge updated.
  - New `internal/pr` package with `Refresh(ctx, *PRState, Options)`.
    Honours TTL + Force + 5-second `context.WithTimeout`; runs
    `gh pr view --json state,isDraft,mergedAt,closedAt`; maps the
    response per ADR-071 §"Refresh implementation". On failure
    preserves `State` + `LastRefreshedAt` and populates
    `LastRefreshError` (truncated to 120 chars). 10 unit tests cover
    skip/expired/force/zero-ttl/never-refreshed/gh-failure/clear-on-success/
    closed-with-mergedAt branches.
  - New `af pr --refresh` CLI path. Refuses with `errPRRefreshNoPR`
    when `state.PR.Number == 0`. On success writes state.toml back
    and emits a `pr_state_changed` ledger event on a flip. Three
    cmd-level tests using a new `prRefreshFunc` test seam.
  - **Pre-existing bug uncovered**: `writeTestSessionStateWithWorktree`
    had a redundant `status string` parameter (every caller passed
    "active"). Dropped via unparam-driven cleanup.

  Deferred to follow-up (called out in commit + TODO): TTL-aware
  refresh wire-up into `af status` (per-row), `af info`,
  `af clean`/`af sync`/`af done` (force-refresh paths). Each command
  needs its own audit pass. ADR-071 frontmatter therefore stays at
  `in-progress`, not `complete`.

- **I13.7 + I13.8 (ADR-069 §3 + §1)** in commit `f7521e9`:
  - `internal/lifecycle/create.go` gains `checkNameCollision` plus
    `lifecycle.ErrNameCollision`. `CreateOptions.ArchiveDir` is new;
    `cmd/af/create.go` wires it via `resolveArchiveDir()`. Three
    new tests cover active + archived + empty-archive paths.
  - `.golangci.yml` re-enables depguard with a `no-outbound-net`
    rule denying `net/http` imports outside `internal/sandbox/`,
    `internal/remote/`, and `internal/pr/`. Today no package imports
    net/http, so the rule is purely preventative.
  - ADR-069 frontmatter advanced to `complete`.

**Stage 14** — full ADR-073 `af review` implementation in commit
`<stage-14-impl>`:

- **I14.1 + I14.2** — `internal/review/system_prompt.md` embedded
  via `//go:embed`; `SystemPrompt()` returns the immutable af-owned
  prefix. `BuildPrompt(opts PromptOpts)` assembles the four-layer
  append (user → repo → file → CLI) under a `# Repo-specific review
  notes` heading, then a `# Suggested skills` block (only when
  non-empty), then the PR header + diff. Six unit tests cover the
  required tone constraints (no severity tags, no emoji, no verdict
  line) and every append branch.
- **I14.3** — `internal/config` `ReviewConfig` five-touchpoint:
  struct (Agent / Model / SystemPromptAppend /
  SystemPromptAppendFile / SuggestedSkills), `reviewLayer` pointer
  variant, `parseReviewSection`, `mergeReview`, and defaults
  (`SuggestedSkills = ["/review", "/go-review", "/simplify"]`).
- **I14.4** — `internal/gh` package: `ViewPR(runner, n)` and
  `DiffPR(runner, n)`. Wraps `gh pr view --json` and `gh pr diff`.
  `ErrNoPR` on "could not resolve" / `number=0`; `ErrEmptyDiff` on
  whitespace-only output. 8 tests cover every branch incl. a fake
  runner that matches by argv prefix.
- **I14.5** — `cmd/af/review.go` + tests. `af review [session]`
  with `--pr`, `--agent`, `--model`, `--out`, `--append-prompt`,
  `--skill` (repeatable; `""` suppresses), `--stdout` flags.
  Pipeline: load state + config → `gh.ViewPR` → `gh.DiffPR` →
  `review.BuildPrompt` → agent `BodyCmd` via `reviewBodyFunc` seam
  → atomic write to `<worktree>/.af/reviews/<UTC>-pr<n>.md` (0o600
  file in 0o750 dir, `.tmp` + rename) → `review.report.written`
  ledger event. Three test seams: `reviewGhFactory`,
  `reviewBodyFunc`, plus the existing `resolveBodyAgent` pattern.
  Six cmd-level tests cover golden path (file + ledger), named
  failure modes, `--stdout`, and `--append-prompt` threading.
- **I14.6** — ADR-073 frontmatter advanced to `complete`;
  `docs/adr/INDEX.md` updated. README banner + Caveats + command
  tables (af review, af pr --refresh, af session-data) refreshed.
  CHANGELOG `[Unreleased]` gains "Stage 14 — ADR-073 `af review`"
  and "Stage 13 (partial)" sections. TODO check-offs for I13.2,
  I13.7, I13.8, I14.1–I14.5 and the Stage 14 close-out. Handover
  snapshot rewritten for the new ADR state.

Deferrals carried forward (called out in TODO + this PROGRESS):

- ADR-071 multi-command wire-up (5 commands).
- ADR-068 four sub-items (flock, JSON envelope, exit codes,
  completion).
- ADR-070 session-resolution chain + fzf picker.
- ADR-072 state.toml schema roll-up consolidation (mostly a doc
  pass — the two `PROPOSED` blocks are now shipped).
- ADR-073 `--print-system-prompt` debugging flag (~10 LoC).

### Verification

- `make check` is green throughout: 0 lint, all 24 packages pass
  `-race -count=1 -shuffle=on`. Up from 22 packages at Session 32 —
  new packages `internal/pr`, `internal/gh`, `internal/review` were
  added.
- Test count grew by ~35 functions across new files in
  `internal/pr`, `internal/gh`, `internal/review`, and `cmd/af`.
- ADR count: 43 files. Status: 38 `complete` (031 + 033–067 + 069 +
  073), 1 `n/a` (032), 1 `in-progress` (071), 3 `pending` (068, 070,
  072).
- TODO checks: 139 `[x]` / 7 `[ ]`. The 7 unchecked items are
  I13.1 (ADR-070), I13.3–I13.6 (ADR-068 sub-items), I13.9 (Stage 13
  close-out for the remaining items), and I14.6 (this commit will
  close it).

### Release readiness

The project is in **release-ready shape modulo the deferrals**.
`af create`, `af list`, `af info`, `af status`, `af note`,
`af suspend`, `af resume`, `af clean`, `af stack`/`unstack`/`sync`,
`af pr` (including `--ai` and `--refresh`), `af editor`, `af diff`,
`af retro` (including `--ai`), `af control up/down/status`,
`af pull`, `af session-data sync|list`, `af review`, and
`af doctor`/`af setup`/`af config`/`af auth`/`af completions` all
work end-to-end. `goreleaser release --clean` is the path for v1.0.0
when the owner decides the deferred items can land post-1.0.

### Next

Two options:

1. **Land remaining Stage 13 items**, then cut v1.0.0. Most impactful
   are ADR-070 (session resolution + fzf) and ADR-071 multi-cmd
   wire-up. ADR-068 is mostly UX polish that can ship after 1.0.
2. **Cut v1.0.0 now** with the deferrals documented. The deferred
   items are additive improvements, not bug fixes; nothing currently
   shipped is broken.

Recommend option 1 — landing ADR-070 alone removes the only major UX
friction (no `[session]` arg + no cwd inference currently errors loudly
rather than picking interactively).


## 2026-05-22 — Session 34: ADR-071 multi-command PR refresh wire-up

### Goal

Finish the remaining ADR-071 work after the Stage 14 close-out: wire the
TTL-backed PR refresh cache into every command named in the ADR.

### Done

- Added shared `cmd/af/pr_refresh_cache.go` helper for ADR-071 consumers.
  It loads layered `[pr].refresh_ttl` config using the worktree as repo
  context, calls the existing `prRefreshFunc` seam, writes `state.toml`
  on successful refresh or `last_refresh_error` update, and emits
  `pr_state_changed` on state flips.
- `af status` gains `--refresh`, PR state rendering, JSON PR fields,
  default TTL refresh outside the cache window, and soft failure rendering
  (`?` + one `slog.WarnContext`).
- `af info` gains `--refresh`, a PR section in text output, a `pr` JSON
  payload, default TTL refresh outside the cache window, and soft failure
  rendering (`?` + persisted `last_refresh_error`).
- `af clean` force-refreshes PR state for clean targets before reaping and
  treats refresh failure as a hard error.
- `af sync` force-refreshes the parent workstream PR before rebasing and
  treats refresh failure as a hard error.
- `af done` force-refreshes the workstream PR before lifecycle teardown and
  treats refresh failure as a hard error.
- Added cmd-level tests for all five consumers plus status/info rendering
  and refresh failure persistence.
- Advanced ADR-071 and `docs/adr/INDEX.md` to `implementation: complete`;
  updated README, CHANGELOG, and TODO handover.

### Verification

- Red step: new cmd tests failed on missing `--refresh` flags and missing
  force-refresh hooks for clean/sync/done.
- Green step: targeted cmd tests pass, `go test ./cmd/af -count=1` passes.
- Full `make check` is green: 0 lint, all 24 packages pass
  `-race -count=1 -shuffle=on`.

### Next

ADR-070 is now the highest-leverage remaining ADR. After that, finish
ADR-068 (flock, JSON envelope, exit codes, completion) and ADR-072
(schema roll-up).


## 2026-05-22 — Session 35: ADR-070 session selection and inference

### Goal

Implement ADR-070 now that ADR-071 is complete, so every session-taking
command has one shared resolution contract.

### Done

- Added `cmd/af/session_resolve.go` with the ADR-070 resolution chain:
  positional arg → root `--session` flag (stderr warning when both are
  present and `--session` wins) → `AF_SESSION` → cwd `.af/state.toml`
  discovery via `session.DiscoverStatePath` → interactive `fzf` picker
  only when stdin and stderr are TTYs and fzf is installed →
  deterministic no-input error with recovery hints.
- Swapped every `[session]`-taking command path to the shared resolver:
  suspend, resume, done, info, note, pull, session-data sync/list,
  stack/unstack/sync (target session), editor, diff, pr/refresh, retro,
  and review. Parent stack lookups remain exact-by-name.
- `af create` now sets `AF_SESSION=<session>` in the tmux session
  environment after creating the tmux session and before launching the
  agent.
- Added tests for root `--session` overriding a positional arg,
  `AF_SESSION`, nested cwd symlink discovery, no-input errors, and tmux
  `AF_SESSION` propagation.
- Advanced ADR-070 and `docs/adr/INDEX.md` to `implementation: complete`;
  updated README, CHANGELOG, and TODO handover.

### Verification

- Red step: new tests failed on missing `errSessionResolutionNoInput`,
  ignored root `--session`, ignored `AF_SESSION`, note failing from nested
  cwd, and missing tmux `AF_SESSION`.
- Green step: targeted cmd/lifecycle tests pass, then full `make check`
  is green (0 lint, all 24 packages pass `-race -count=1 -shuffle=on`).

### Next

Only ADR-068 and ADR-072 remain pending. ADR-068 is the last behaviour
work (session-level flock, JSON envelope, exit codes, completion);
ADR-072 is a schema-roll-up verification/doc close-out.


## 2026-05-22 — Session 36: ADR-068 operational UX contract

### Goal

Close the ADR-068 cross-cutting UX contract: JSON envelope, exit-code
vocabulary, session lock helper, and completion sources.

### Done

- Added `cmd/af/jsonio.go` and switched `af status --json` /
  `af info --json` to the ADR-068 envelope: `{ "schema": 1,
  "data": ... }`. Existing JSON tests were updated to assert the
  envelope.
- Added `cmd/af/exit_codes.go` with sysexits-style constants and
  `exitCodeForError`; `main` now exits with the mapped code. Tests
  cover `EX_NOINPUT`, `EX_DATAERR`, `EX_INTERRUPTED`, `EX_USAGE`, and
  fallback `EX_GENERAL`.
- Added `cmd/af/session_lock.go` with a per-session `.af.lock` helper
  and wired `af note --append` through it; tests assert the lock file is
  created. PR refresh write paths already centralize state/ledger writes
  through the shared ADR-071 helper.
- Added completion sources for workstream names and lifecycle states.
  Root `--session` and `[session]` positionals complete session names;
  `af status --filter` completes active/suspended/completed/abandoned.
- Advanced ADR-068 and `docs/adr/INDEX.md` to `implementation: complete`;
  updated README, CHANGELOG, and TODO handover.

### Verification

- Red step: JSON tests failed against the old bare payloads; new
  operational tests failed on missing exit-code constants, lock helper,
  and completion source.
- Green step: targeted cmd tests pass and full `make check` is green
  (0 lint, all 24 packages pass `-race -count=1 -shuffle=on`).

### Next

Only ADR-072 remains pending. It should be a verification/doc close-out
for the state.toml schema roll-up now that ADR-067 and ADR-071 fields
have landed.


## 2026-05-22 — Session 37: ADR-072 schema roll-up and all ADRs closed

### Goal

Close the final pending ADR by aligning the canonical state schema docs
with the implementation after ADR-067 and ADR-071.

### Done

- Advanced ADR-072 and `docs/adr/INDEX.md` to `implementation: complete`.
- Updated ADR-072's canonical schema dump: removed the old PROPOSED
  markers, replaced the planned `[[session_sync]]` shape with the
  shipped `[session_export]` + `[[session_export.sources]]` schema,
  and treated `[pr].last_refreshed_at` / `[pr].last_refresh_error` as
  shipped fields.
- Added the ADR-037 forward-link amendment pointing readers to ADR-072
  for the consolidated v1 schema dump.
- Updated `docs/SPEC.md` section 5.2 so the spec matches ADR-072 and
  the Go structs/tests.
- Updated README, CHANGELOG, and TODO. I13.9 is now checked; TODO's
  handover says no ADRs remain pending.

### Verification

- Existing `internal/session` round-trip tests already cover the shipped
  `[session_export]` and PR cache fields.
- Full `make check` remains green (0 lint, all 24 packages pass
  `-race -count=1 -shuffle=on`).

### Next

All v1 ADRs are closed. Run release-readiness checks (`goreleaser check`
and a snapshot build) before deciding whether to tag v1.0.0.


## 2026-05-22 — Session 38: merged ADR completion branch to main and opened Stage 15

### Goal

Merge the completed Stage 12/13/14 ADR branch back to `main` and update
tracking files for the next stage.

### Done

- Merged `stage-12-followups-066-067` into `main` with merge commit
  `1d63290` (`merge: stage 12-14 ADR completion`).
- The merged tree includes the final ADR-completion commits:
  - `6ea1391` — ADR-071 multi-command PR refresh wire-up.
  - `aee6917` — ADR-070 shared session resolution + tmux `AF_SESSION`.
  - `0bf3e7e` — ADR-068 operational UX contract.
  - `f292395` — ADR-072 schema roll-up.
- Confirmed ADR status after merge: 42 `complete`, 1 `n/a` (ADR-032),
  0 `pending`.
- Updated `TODO.md` handover to point at the new
  **Implementation Stage 15 — v1.0.0 release prep** checklist.
- Updated `README.md` status banner from "Stages 0–12 + Stage 14" to
  "Stages 0–14 implemented; Stage 15 release prep".

### Verification

Release checks passed on the source branch before merge (`make check`,
`goreleaser check`, `goreleaser release --snapshot --clean`). The next
step is to rerun those checks on merged `main` after this tracking-doc
commit.

### Next

Run Stage 15: verify merged `main`, review release notes, decide whether
to defer optional polish, then cut v1.0.0 if approved.


## 2026-05-22 — Session 39: added owner pre-release smoke-test gate

### Goal

Give the owner a concrete command list to validate the merged release
candidate before any v1.0.0 tag is cut.

### Done

- Added `docs/PRE_RELEASE_SMOKE.md` with a copy/paste smoke-test flow:
  candidate build, isolated temp HOME/repo, setup/config/doctor, local
  lifecycle, JSON envelopes, ADR-070 session resolution, completion
  sources, exit-code check, stack metadata, cleanup, and optional real
  GitHub/slicer integration checks.
- Updated Stage 15 in `TODO.md` with an explicit owner smoke-test gate
  (`I15.1a`).

### Release gate

Do **not** tag v1.0.0 or run `goreleaser release --clean` until the
owner reports the smoke-test result.


## 2026-05-22 — Session 40: smoke-test finding — doctor Obsidian and versions

### Goal

Address the owner smoke-test feedback before release: `af doctor` should
report configured Obsidian vault accessibility and should include versions
for tmux, pi, and slicer.

### Done

- Added failing tests for configured Obsidian vault accessibility in
  `af doctor` output.
- Added failing tests for tool-specific version commands:
  `tmux -V`, `pi --version`, and `slicer version` (parsing the
  `Version:` line after slicer's banner).
- Implemented local doctor Obsidian vault checks from `[obsidian.vaults]`:
  accessible directories render as `✓ obsidian:<name>` and inaccessible
  configured paths render as optional warnings with a config hint.
- Updated `SystemLookup.Version` to use tool-specific version commands
  for tmux and slicer while preserving the generic `--version` path for
  pi and other tools.
- Updated `docs/PRE_RELEASE_SMOKE.md` so the owner smoke test asserts
  doctor reports tmux/pi/slicer versions (slicer only when installed) and
  `✓ obsidian:personal`.
- Updated `CHANGELOG.md` and Stage 15 tracking with the smoke-test fix.

### Release gate

The v1.0.0 release remains blocked until the owner reruns the smoke test
and reports pass/fail.


## 2026-05-22 — Session 41: expanded staged pre-release smoke procedure

### Goal

Make the owner smoke test granular enough to run and report stage by
stage, while covering the full command surface before release approval.

### Done

- Rewrote `docs/PRE_RELEASE_SMOKE.md` as staged smoke gates:
  build/install, isolated environment, setup/config/doctor/completions,
  command help coverage, auth lifecycle, local workstream lifecycle,
  session resolution, agent slots, stack/sync, expected non-slicer
  failures, hermetic review/control fakes, cleanup, clean, and retro.
- Added direct install guidance via `make install`, plus an optional hard
  install into `/opt/homebrew/bin/af` or `/usr/local/bin/af`.
- Added an explicit report protocol: run one stage at a time and report
  `PASS`, `FAIL`, or `DISCREPANCY`; maintainers fix implementation/docs
  or create/amend an ADR when expectations conflict with design.
- Added optional real-integration stages for GitHub PR review, slicer,
  and remote doctor/control paths.
- Added a command coverage matrix mapping every public command to a
  required or optional smoke stage.

### Release gate

The v1.0.0 release remains blocked until the owner reruns the staged
smoke test and reports the required stages 0–10.
