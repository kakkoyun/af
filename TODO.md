# TODO — v1

Tracked work for the v1 (Go rewrite) iteration. See [`PROGRESS.md`](PROGRESS.md)
for the narrative log and [`docs/adr/`](docs/adr/) for accepted decisions.

> **v0 history.** The Rust era's TODO list (final state at end of Session 11,
> with Phase 0–2 complete and most Phase 3/4 items deferred to a "0.2.0"
> that will never ship) is archived at [`docs/v0/TODO.md`](docs/v0/TODO.md).

---

## Stage A — Archive v0 docs ✅

Closed at commit `1659d60`.

- [x] C1: `docs(v0): archive top-level changelog/progress/todo` (`3cc5f5b`)
- [x] C2: `docs(v0): archive spec/plan/conventions` (`d6bf397`)
- [x] C3: `docs(v0): archive ADRs, reference, planning` (`570e477`)
- [x] C4: `docs(v0): archive mdbook scaffold` (`d9e6410`)
- [x] C5: `docs(v0): docs/v0/README.md index` (`1659d60`)

## Stage B — New top-level scaffolding (in progress)

- [x] C6: `docs(v1): top-level CHANGELOG.md` (`b36a1ce`)
- [x] C7: `docs(v1): top-level PROGRESS.md (Session 0)` (`0498299`)
- [ ] C8: `docs(v1): top-level TODO.md` ← this commit
- [ ] C9: `docs(v1): top-level README.md`
- [ ] C10: `docs(v1): CLAUDE.md and AGENTS.md`

## Stage C — v1 spec, plan, conventions

- [ ] C11: `docs(v1): docs/SPEC.md`
- [ ] C12: `docs(v1): docs/PLAN.md` (lightweight; drops impl-phase block)
- [ ] C13: `docs(v1): docs/CONVENTIONS.md`

## Stage D — ADRs 031–059 (29 commits)

ADRs land in this order; each is a single atomic commit. All initially
proposed (`status: proposed`); user reviews and accepts in follow-up
commits per ADR-032's lifecycle rules.

### Meta

- [ ] C14: ADR-031 v1 Migration to Go + Scope Reduction (master)
- [ ] C15: ADR-032 ADR Conventions for v1 (frontmatter, lifecycle)
- [ ] C16: ADR-033 Documentation Archival Policy (v0 → v1)

### Foundation (toolchain + structure)

- [ ] C17: ADR-034 Go Module Layout & Idiom
- [ ] C18: ADR-035 CLI Framework — cobra + pflag
- [ ] C19: ADR-036 Configuration — TOML, layered, global vault config
- [ ] C20: ADR-037 Session Metadata Schema
- [ ] C21: ADR-038 Workstream + Worktree Layout

### Domain model

- [ ] C22: ADR-039 Multi-Agent Multi-Session Model
- [ ] C23: ADR-040 tmux-only Multiplexer
- [ ] C24: ADR-041 SSH Remote Model
- [ ] C25: ADR-042 Sandbox Providers (slicer + sbx)
- [ ] C26: ADR-043 Agent Providers (claude, pi, codex; pi default)

### Commands

- [ ] C27: ADR-044 Doctor + Install Hints (local & --remote)
- [ ] C28: ADR-045 `af setup` — Environment Companion to Doctor
- [ ] C29: ADR-046 `af suspend` / `af resume` Lifecycle
- [ ] C30: ADR-047 Obsidian Integration — Notes + Bases
- [ ] C31: ADR-048 Minimal Proxy Commands (editor, diff, pr)

### Cross-cutting

- [ ] C32: ADR-049 Secret Management
- [ ] C33: ADR-050 Code Quality — golangci-lint pedantic
- [ ] C34: ADR-051 Testing Strategy
- [ ] C35: ADR-052 Formal Verification Experimentation
- [ ] C36: ADR-053 Build & Distribution — goreleaser + Make

### Command addenda

- [ ] C37: ADR-054 `af status` — Workstream Dashboard
- [ ] C38: ADR-055 `af info` — Workstream Detail View
- [ ] C39: ADR-056 `af clean` — Reap Completed Workstreams
- [ ] C40: ADR-057 `af pr --ai` — Agent-Authored PR Body
- [ ] C41: ADR-058 `af retro` — Mine Archived Workstream Notes
- [ ] C42: ADR-059 Stack-Aware Branch Model

## Stage E — ADR index

- [ ] C43: `docs(adr): docs/adr/INDEX.md (v0 archive link + v1 ADRs 031–059)`

---

## After the doc pass — topologically sorted implementation plan

This is the operational implementation checklist for ADRs 034–059. It is
topologically sorted by dependency, with static checks and test harnesses
landing before feature behaviour. `docs/PLAN.md` remains the high-level
map; this section is the task source of truth.

Tracking rules for every item below:

- Set the relevant ADR `implementation` frontmatter to `in-progress`
  when starting, and `complete` when that ADR's implementation is done.
- Follow TDD: write the failing test first, implement, then refactor with
  tests green.
- Run `make check` after each item once the Makefile exists. Before that,
  run the equivalent targeted `go test` / format / lint command.
- Append to `PROGRESS.md` at the end of each work session, including
  blockers and the next unchecked item.

### Implementation Stage 0 — Go scaffold, static checks, and tests first

No product feature work until this stage is green.

- [x] I0.1: ADR-034 — create the Go module scaffold (`go.mod`,
      `cmd/af/`, `internal/...`, `examples/`).
- [x] I0.2: ADR-035 + ADR-053 — add a minimal cobra root command,
      persistent root flags, `af version`, and `internal/version` build-info
      wiring.
- [x] I0.2a: User override — remove the Rust v0 source/tooling tree
      (`src/`, `tests/`, Cargo files, `justfile`, and Rust tool configs) at
      rewrite start; rely on `docs/v0/` and git history for reference.
- [x] I0.3: ADR-050 + ADR-053 — add `.golangci.yml`, `Makefile`,
      `gofumpt`, `goimports`, `make fmt-check`, `make lint`, `make test`,
      `make check`, and local snapshot build targets.
- [x] I0.4: ADR-051 — add the test scaffold: `testscript` harness,
      `cmd/af/testdata/script/`, fake external-command hooks, package
      `testutil` helpers, and baseline smoke scripts for `af version` /
      `af --help`.
- [x] I0.5: ADR-052 — add the property-test scaffold for lifecycle and
      naming invariants without enabling formal verification as a release
      gate.
- [x] I0.6: Baseline verification — `make check` passes on the scaffold;
      update `PROGRESS.md` with the first green baseline.

### Implementation Stage 1 — Pure foundations and durable state

These packages are mostly pure or fake-backed. They unblock all commands.

- [x] I1.1: ADR-036 — implement layered TOML config loading, schema
      defaults, global-only sections, `~` expansion, proxy command config
      shapes, and config tests.
- [x] I1.2: ADR-056 + ADR-058 — implement the shared duration grammar
      (`d`/`w` plus stdlib duration units) with table and property tests.
- [x] I1.3: ADR-038 + ADR-039 — implement naming, branch-prefix rules,
      session-name sanitization, sub-branch naming, and UUID/session-ID
      derivation.
- [x] I1.4: ADR-037 — implement `state.toml` and `ledger.jsonl`
      read/write, atomic writes, flock locking, schema version checks,
      derived `last_touched_at`, `repo_slug`, and current-workstream
      discovery.
- [ ] I1.5: ADR-038 — implement local worktree path planning,
      `.af/state.toml` symlink handling, sub-worktree path planning, and git
      cleanup planning.
- [ ] I1.6: ADR-049 — implement secret redaction handler and the keyring
      interface with fakes; keep envelope transport disabled until remote /
      sandbox stages.
- [ ] I1.7: ADR-047 — implement Obsidian frontmatter parse/emit helpers
      and note path resolution, fake-backed and without command integration.

### Implementation Stage 2 — External system interfaces and fakes

This stage creates every seam before commands depend on real tools.

- [ ] I2.1: ADR-043 — implement `internal/agent.Agent`, `BodyCmd`,
      provider registry, fake provider, and availability checks for `pi`,
      `claude`, and `codex`.
- [ ] I2.2: ADR-040 — implement `internal/mux.Multiplexer`, fake mux,
      and tmux command construction with tests that do not require real tmux.
- [ ] I2.3: ADR-041 — implement SSH remote command construction,
      remote path mapping, and fake remote executor.
- [ ] I2.4: ADR-042 — implement sandbox provider interfaces, fake
      sandbox, and slicer/sbx command construction.
- [ ] I2.5: ADR-051 — wire all command-facing code to fakes in tests;
      no unit or testscript path may require real tmux, ssh, slicer, sbx, or
      agent CLIs.

### Implementation Stage 3 — Utility commands before workstreams

These commands validate the scaffold without creating workstreams.

- [ ] I3.1: ADR-036 — implement `af config init` and `af config show`.
- [ ] I3.2: ADR-035 + ADR-045 — implement `af completions <shell>`.
- [ ] I3.3: ADR-044 — implement local `af doctor` using the interface
      probes and install-hint rendering.
- [ ] I3.4: ADR-041 + ADR-044 — implement `af doctor --remote <host>`
      with fake-backed SSH probes.
- [ ] I3.5: ADR-045 + ADR-049 — implement `af setup`: state directory
      creation, config init, global gitignore update, completion install,
      secrets directory creation, and Obsidian vault hint.
- [ ] I3.6: ADR-049 — implement `af auth set|get|status|clear|list`
      against the keyring interface, including TTY/redaction behaviour.

### Implementation Stage 4 — Local workstream MVP

First feature slice: local-only, no remote, no sandbox, one primary
agent. This proves config, state, git, mux, and agent seams together.

- [ ] I4.1: ADR-038 + ADR-039 — implement local `af create [name]`
      with branch/worktree creation, state/ledger creation, note creation,
      tmux session creation, and primary-agent launch.
- [ ] I4.2: ADR-037 + ADR-035 — implement `af list` as a read-only view
      over active/suspended local workstreams.
- [ ] I4.3: ADR-055 — implement `af info [session] [--json] [--ledger N]`
      using state + ledger tail only.
- [ ] I4.4: ADR-039 — implement `af agent list`, then `af agent add`,
      then `af agent stop`, including sub-worktree creation/removal.
- [ ] I4.5: ADR-038 + ADR-046 — implement local `af done [session]`
      and `af done --force` with worktree/sub-worktree cleanup, archive move,
      ledger events, and Obsidian status updates.
- [ ] I4.6: ADR-035 — implement `af session-branch` for ad-hoc work in
      the current checkout.

### Implementation Stage 5 — Lifecycle, notes, cleanup, and stacking

These build on the local MVP and should remain fake-backed in tests.

- [ ] I5.1: ADR-046 — implement local `af suspend` and warm/cold
      `af resume`, including per-slot resume and crash reconciliation.
- [ ] I5.2: ADR-047 — implement `af note [session]` and
      `af note --append TEXT`, including fallback editor behaviour.
- [ ] I5.3: ADR-056 — implement reusable merge detection
      (`pr-state`, ancestry, squash fingerprint) as an internal service.
- [ ] I5.4: ADR-056 — implement `af clean` with dry-run,
      include-abandoned, max-age, force-by-name, archive, and Obsidian
      updates.
- [ ] I5.5: ADR-059 — implement `af stack`, `af unstack`, and `af sync`
      using the reusable merge-detection contract.
- [ ] I5.6: ADR-054 — implement `af status [--json] [--all]
[--filter STATE]`, including stack suffixes, `repo_slug` handling,
      bounded `gh` fan-out, and stable JSON.

### Implementation Stage 6 — Remote, sandbox, and secret transport

Do this after the local lifecycle is solid; it composes the same state
and command paths with remote/sandbox execution.

- [ ] I6.1: ADR-049 — implement ephemeral envelope creation,
      source-and-delete wrappers, lazy stale-envelope sweep, and tests for
      redaction/no-secret-in-state invariants.
- [ ] I6.2: ADR-041 — implement `af create --remote`, remote clone/path
      setup, remote tmux launch, `af resume` attach, and remote teardown.
- [ ] I6.3: ADR-042 — implement `af create --sandbox`, sandbox launch,
      health check, teardown, and `--respawn`.
- [ ] I6.4: ADR-041 + ADR-042 + ADR-049 — compose
      `--remote --sandbox` with remote-side envelope transport and teardown.
- [ ] I6.5: ADR-046 — extend suspend/resume/done/clean tests across
      local, remote, sandbox, and remote+sandbox modes using fakes.

### Implementation Stage 7 — Proxy commands, PR AI, and retrospectives

These are deliberately late because they depend on config, state,
Obsidian notes, agent `BodyCmd`, and local/stack base resolution.

- [ ] I7.1: ADR-048 — implement `af editor [--terminal|--visual]
[session]`, including remote URL fallback.
- [ ] I7.2: ADR-048 + ADR-059 — implement `af diff [session]
[--base REF]`, argv-vs-shell parsing, token interpolation, and stacked
      base defaults.
- [ ] I7.3: ADR-048 — implement base `af pr [session] [--title]
[--draft] [--web]`, push-if-needed, PR metadata detection, state
      update, ledger event, and Obsidian PR fields.
- [ ] I7.4: ADR-057 — implement `af pr --ai` and `--ai-model` using
      primary-agent `BodyCmd`, body prompt construction, `flag_template.body`,
      empty-diff/empty-body errors, and `--web` incompatibility.
- [ ] I7.5: ADR-058 — implement `af retro` filters (`--since`, `--tag`,
      `--search`, `--limit`) over archived notes.
- [ ] I7.6: ADR-058 + ADR-057 — implement `af retro --ai` using
      `BodyCmd` with `BodyOpts.Cwd = ""`.

### Implementation Stage 8 — Hardening, verification, and v0 retirement

This stage should not add broad new feature surface.

- [ ] I8.1: ADR-052 — add lifecycle state-machine property tests and
      document any invariants worth carrying into optional TLA+.
- [ ] I8.2: ADR-050 + ADR-051 + ADR-053 — run full quality pass:
      coverage review, `make check`, cross-compile snapshot, and manual
      smoke plan for real tmux/ssh/sandbox paths.
- [ ] I8.3: Update README, CHANGELOG, Godoc, ADR implementation
      frontmatter, TODO, and PROGRESS for all completed v1 behaviour.
- [x] I8.4: Remove the Rust v0 source tree (`src/`, `tests/`,
      `Cargo.toml`, `Cargo.lock`, `justfile`, Rust tool configs). Completed
      early during Stage 0 by explicit user override; no final v0 source
      cleanup remains.

---

## Backlog (post-v1, unscheduled)

These were considered for v1 and explicitly cut. Listed here so they're
not lost.

- DD Workspaces remote provider (out of scope for single-user v1).
- Zellij / Ghostty / cmux multiplexers (single-multiplexer policy).
- Skill bundle installer (v0 ADR-030 retired; revisit if Claude Code skill ecosystem matures).
- Auto-install in `af doctor` (v1 doctor is hint-only; revisit if the per-platform install surface stabilises).
- Workspace templates / pre-configured sessions per project.
- `af log` (append a structured log entry to the Obsidian note).
- Dataview dashboards in Obsidian (Bases approach in ADR-047 may obsolete this).
- Homebrew tap / GitHub Releases (re-evaluate if v1 escapes single-user scope).
