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
