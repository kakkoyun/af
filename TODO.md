# TODO — v1

Tracked work for the v1 (Go rewrite) iteration. See [`PROGRESS.md`](PROGRESS.md)
for the narrative log and [`docs/adr/`](docs/adr/) for accepted decisions.

> **v0 history.** The Rust era's TODO list (final state at end of Session 11,
> with Phase 0–2 complete and most Phase 3/4 items deferred to a "0.2.0"
> that will never ship) is archived at [`docs/v0/TODO.md`](docs/v0/TODO.md).

---

## Handover snapshot (read this first)

**Status at HEAD `2d7ad71`** (2026-05-22, after Stage 11 + the
gap-analysis pass):

- Every numbered ADR from **031 to 065** is `implementation: complete`.
- `pending` ADRs: **066** (VM agent-session export), **067**
  (automatic agent-session sync), **068** (operational UX contract),
  **069** (boundary & privacy), **070** (session selection &
  inference), **071** (PR state TTL cache), **072** (state.toml
  schema roll-up).
- ADRs 068–072 are the **gap-analysis batch** — cross-cutting
  contracts plus a state-schema consolidation. They were drafted to
  match what's currently shipped, so most of 068/072 is
  documentation of existing behaviour. ADRs 069/070/071 add new
  required behaviour. See §"Stage 12 / 13 reading list" below for
  what each one implies for code work.
- ADR-032 is `implementation: n/a` (it is the conventions ADR itself).
- `make check` is green: 0 lint, all 21 packages pass
  `-race -count=1 -shuffle=on`.
- `goreleaser check` is green; `make snapshot-all` cross-builds for
  darwin/arm64, linux/amd64, linux/arm64.
- Working tree is clean. `main` is 29 commits ahead of `origin/main`
  (no push planned yet — v1.0.0 release timing is the owner's call).

**Next session pickup options** (in roughly increasing effort):

1. **Wire `SlicerWTAvailable` into `af doctor`'s default probe list.**
   The probe function exists in `internal/doctor/system.go` with a
   `// TODO(ADR-065)` marker. Small (~30 lines) follow-up; see
   I12.1 below.
2. **Add a `TestEditor_LeaseWarning`** that exercises the warning
   path without spawning a real editor (e.g. inject a fake editor
   command via the existing seam). Small (~20 lines); I12.2.
3. **Implement ADR-066 + ADR-067** (the agent-session-export +
   automatic-sync pair). Larger; I12.3–I12.4. Spec is well-defined;
   parallelizable into two worktree agents.
4. **Implement ADRs 068–072** (gap-analysis batch). New cross-cutting
   contracts that need to land before v1.0.0. See §"Stage 12 / 13
   reading list" below.
5. **Cut v1.0.0 release.** `goreleaser release --clean` after a
   dry-run validation; tag, push, GitHub release. Out of scope until
   the owner is ready; the project is in release-ready shape modulo
   the follow-ups above.

### Stage 12 / 13 reading list — the gap-analysis batch

The owner ran a SPEC-vs-ADR reconciliation pass in branch
`docs/gap-analysis-v1` (now merged into `main`). Five new ADRs and a
full SPEC rewrite landed. **None of the gap-analysis commits changed
any code** — they're documentation-only; existing code keeps
working. Concrete impact on future code work:

- **ADR-068 (Operational UX contract).** Mostly documents what's
  already shipped. Concrete commitments going forward:
    - Every command that ships `--json` carries
      `{"schema": <int>, "data": {...}}` (per-command schema number).
    - The exit-code table in §2 (sysexits) is canonical — new error
      classes must pick a code from the table.
    - Per-session flock at `~/.local/share/af/v1/sessions/<name>/.af.lock`
      for mutating ops (currently atomic per-file flock per ADR-037
      — needs lifting to a session-level lock for consistent semantics
      across mixed-file ops like `af note --append`, `af stack`).
    - Tab-completion sources per §5 are mandatory; some of them
      (sessions, slot names) may need adding.
- **ADR-069 (Boundary & privacy).** All three sub-decisions are
  mostly already true; documenting the contract.
    - **No outbound calls from `af`'s own code.** Worth a `gosec`-style
      rule that rejects any new `net/http` import outside
      sub-process invocations.
    - **Single-machine canonical state.** No code change needed.
    - **Strict-fail name collisions across active+suspended+archived.**
      Verify `af create` enforces this. Quick test to add.
- **ADR-070 (Session selection & inference).** New behaviour:
    - Resolution order: arg → `--session` → `AF_SESSION` → cwd
      symlink → fzf picker on stderr (TTY only) → `EX_NOINPUT`.
    - `AF_SESSION` env var is new; `af create` should
      `tmux setenv -t <session> AF_SESSION <session>` so panes inherit.
    - fzf picker is new; falls back to error when stdin/stderr aren't
      both TTYs or fzf is missing.
- **ADR-071 (PR state TTL cache).** New behaviour:
    - `state.toml.[pr]` gains two fields (PROPOSED in ADR-072):
      `last_refreshed_at`, `last_refresh_error`.
    - `[pr].refresh_ttl = "10m"` config knob (default 10m).
    - Refresh policy: `af list` never refreshes; `af status`/`af info`
      refresh on TTL expiry; `af clean`/`af sync`/`af done` always
      force-refresh.
    - New ledger event `pr_state_changed` on flips.
    - `af pr --refresh` and `af status --refresh` explicit
      force-refresh paths.
- **ADR-072 (state.toml schema roll-up).** Aligns the canonical
  state.toml dump in ADR-037 with what's actually implemented after
  Stages 9–11 (flat `execution.sandbox_resource_*` fields,
  `[slicer_wt]` block, `execution.remote_control`). Ships **no new
  fields** — only catalogues the existing ones. The two `PROPOSED`
  blocks (`[[session_sync]]`, `[pr].last_refreshed_at`) are forward
  pointers to ADR-067/ADR-071's implementation work.

Good approach for the next implementor: pick ADR-070 + ADR-071
first (they're new-behaviour ADRs and unblock other work like
`af list` performance and the dashboard freshness story). ADR-068's
formal flock lift can come with whatever ADR adds the next mutating
command. ADR-069 is mostly tests.

**Where to look first**:

- `PROGRESS.md` Session 29 has the full Stage 11 narrative + deferrals.
- Stage 12 plan below (the only remaining `[ ]` items in this file).
- For ADR scope: `docs/adr/066-*.md` and `docs/adr/067-*.md`.
- For the slicer-wt code path that just landed: `internal/sandbox/
slicerwt.go`, `internal/lifecycle/pull.go`, `cmd/af/pull.go`,
  and the lease checks scattered across `done.go`, `suspend_resume.go`,
  `proxy_commands.go`, `status.go`, `info.go`.

---

## Stage A — Archive v0 docs ✅

Closed at commit `1659d60`.

- [x] C1: `docs(v0): archive top-level changelog/progress/todo` (`3cc5f5b`)
- [x] C2: `docs(v0): archive spec/plan/conventions` (`d6bf397`)
- [x] C3: `docs(v0): archive ADRs, reference, planning` (`570e477`)
- [x] C4: `docs(v0): archive mdbook scaffold` (`d9e6410`)
- [x] C5: `docs(v0): docs/v0/README.md index` (`1659d60`)

## Stage B — New top-level scaffolding ✅

- [x] C6: `docs(v1): top-level CHANGELOG.md` (`b36a1ce`)
- [x] C7: `docs(v1): top-level PROGRESS.md (Session 0)` (`0498299`)
- [x] C8: `docs(v1): top-level TODO.md`
- [x] C9: `docs(v1): top-level README.md` (refreshed in Stage 9 + Stage 10)
- [x] C10: `docs(v1): CLAUDE.md and AGENTS.md`

## Stage C — v1 spec, plan, conventions ✅

- [x] C11: `docs(v1): docs/SPEC.md`
- [x] C12: `docs(v1): docs/PLAN.md` (lightweight; drops impl-phase block)
- [x] C13: `docs(v1): docs/CONVENTIONS.md`

## Stage D — ADRs 031–059 (29 commits) ✅

Every ADR from 031 to 059 has been written, accepted, and implemented
(see Stage E for the index and the implementation stages below for
code landing). The trackers below remain for historical traceability.

### Meta

- [x] C14: ADR-031 v1 Migration to Go + Scope Reduction (master)
- [x] C15: ADR-032 ADR Conventions for v1 (frontmatter, lifecycle)
- [x] C16: ADR-033 Documentation Archival Policy (v0 → v1)

### Foundation (toolchain + structure)

- [x] C17: ADR-034 Go Module Layout & Idiom
- [x] C18: ADR-035 CLI Framework — cobra + pflag
- [x] C19: ADR-036 Configuration — TOML, layered, global vault config
- [x] C20: ADR-037 Session Metadata Schema
- [x] C21: ADR-038 Workstream + Worktree Layout

### Domain model

- [x] C22: ADR-039 Multi-Agent Multi-Session Model
- [x] C23: ADR-040 tmux-only Multiplexer
- [x] C24: ADR-041 SSH Remote Model
- [x] C25: ADR-042 Sandbox Providers (slicer + sbx; sbx later dropped by ADR-060)
- [x] C26: ADR-043 Agent Providers (claude, pi, codex; pi default)

### Commands

- [x] C27: ADR-044 Doctor + Install Hints (local & --remote)
- [x] C28: ADR-045 `af setup` — Environment Companion to Doctor
- [x] C29: ADR-046 `af suspend` / `af resume` Lifecycle
- [x] C30: ADR-047 Obsidian Integration — Notes + Bases
- [x] C31: ADR-048 Minimal Proxy Commands (editor, diff, pr)

### Cross-cutting

- [x] C32: ADR-049 Secret Management
- [x] C33: ADR-050 Code Quality — golangci-lint pedantic
- [x] C34: ADR-051 Testing Strategy
- [x] C35: ADR-052 Formal Verification Experimentation
- [x] C36: ADR-053 Build & Distribution — goreleaser + Make

### Command addenda

- [x] C37: ADR-054 `af status` — Workstream Dashboard
- [x] C38: ADR-055 `af info` — Workstream Detail View
- [x] C39: ADR-056 `af clean` — Reap Completed Workstreams
- [x] C40: ADR-057 `af pr --ai` — Agent-Authored PR Body
- [x] C41: ADR-058 `af retro` — Mine Archived Workstream Notes
- [x] C42: ADR-059 Stack-Aware Branch Model

## Stage E — ADR index ✅

- [x] C43: `docs(adr): docs/adr/INDEX.md (v0 archive link + v1 ADRs 031–059)`

## Post-v1 ADRs (060–065) ✅

ADRs added after the original Stage A–E plan. Implementation tracked
under Implementation Stages 9–11 below.

- [x] ADR-060: Slicer-Only Sandbox Provider (drop sbx) — Stage 10 Wave 1.
- [x] ADR-061: Repo-Scoped Control Settings — Stage 10 Wave 1.
- [x] ADR-062: Repo-Scoped Slicer VM Resource Profiles — Stage 10 Wave 2.
- [x] ADR-063: Remote Control via Tailscale Serve and superterm — Stage 10 Wave 1.
- [x] ADR-064: Opinionated Diff Rendering (hunk + diffity) — Stage 10 Wave 1.
- [x] ADR-065: Slicer Worktree Transport (`slicer wt`) — Stage 11.

> **Note**: ADR-066 (VM agent-session export) and ADR-067 (automatic
> agent-session sync) are owner drafts (`status: proposed`,
> `implementation: pending`). They are deferred to a future stage and
> are not part of the up-to-ADR-065 implementation target.

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
- [x] I1.5: ADR-038 — implement local worktree path planning,
      `.af/state.toml` symlink handling, sub-worktree path planning, and git
      cleanup planning.
- [x] I1.6: ADR-049 — implement secret redaction handler and the keyring
      interface with fakes; keep envelope transport disabled until remote /
      sandbox stages.
- [x] I1.7: ADR-047 — implement Obsidian frontmatter parse/emit helpers
      and note path resolution, fake-backed and without command integration.

### Implementation Stage 2 — External system interfaces and fakes

This stage creates every seam before commands depend on real tools.

- [x] I2.1: ADR-043 — implement `internal/agent.Agent`, `BodyCmd`,
      provider registry, fake provider, and availability checks for `pi`,
      `claude`, and `codex`.
- [x] I2.2: ADR-040 — implement `internal/mux.Multiplexer`, fake mux,
      and tmux command construction with tests that do not require real tmux.
- [x] I2.3: ADR-041 — implement SSH remote command construction,
      remote path mapping, and fake remote executor.
- [x] I2.4: ADR-042 — implement sandbox provider interfaces, fake
      sandbox, and slicer/sbx command construction.
- [x] I2.5: ADR-051 — wire all command-facing code to fakes in tests;
      no unit or testscript path may require real tmux, ssh, slicer, sbx, or
      agent CLIs.

### Implementation Stage 3 — Utility commands before workstreams

These commands validate the scaffold without creating workstreams.

- [x] I3.1: ADR-036 — implement `af config init` and `af config show`.
- [x] I3.2: ADR-035 + ADR-045 — implement `af completions <shell>`.
- [x] I3.3: ADR-044 — implement local `af doctor` using the interface
      probes and install-hint rendering.
- [x] I3.4: ADR-041 + ADR-044 — implement `af doctor --remote <host>`
      with fake-backed SSH probes.
- [x] I3.5: ADR-045 + ADR-049 — implement `af setup`: state directory
      creation, config init, global gitignore update, completion install,
      secrets directory creation, and Obsidian vault hint.
- [x] I3.6: ADR-049 — implement `af auth set|get|status|clear|list`
      against the keyring interface, including TTY/redaction behaviour.

### Implementation Stage 4 — Local workstream MVP

First feature slice: local-only, no remote, no sandbox, one primary
agent. This proves config, state, git, mux, and agent seams together.

- [x] I4.1: ADR-038 + ADR-039 — implement local `af create [name]`
      with branch/worktree creation, state/ledger creation, note creation,
      tmux session creation, and primary-agent launch.
- [x] I4.2: ADR-037 + ADR-035 — implement `af list` as a read-only view
      over active/suspended local workstreams.
- [x] I4.3: ADR-055 — implement `af info [session] [--json] [--ledger N]`
      using state + ledger tail only.
- [x] I4.4: ADR-039 — implement `af agent list`, then `af agent add`,
      then `af agent stop`, including sub-worktree creation/removal.
- [x] I4.5: ADR-038 + ADR-046 — implement local `af done [session]`
      and `af done --force` with worktree/sub-worktree cleanup, archive move,
      ledger events, and Obsidian status updates.
- [x] I4.6: ADR-035 — implement `af session-branch` for ad-hoc work in
      the current checkout.

### Implementation Stage 5 — Lifecycle, notes, cleanup, and stacking

These build on the local MVP and should remain fake-backed in tests.

- [x] I5.1: ADR-046 — implement local `af suspend` and warm/cold
      `af resume`, including per-slot resume and crash reconciliation.
- [x] I5.2: ADR-047 — implement `af note [session]` and
      `af note --append TEXT`, including fallback editor behaviour.
- [x] I5.3: ADR-056 — implement reusable merge detection
      (`pr-state`, ancestry, squash fingerprint) as an internal service.
- [x] I5.4: ADR-056 — implement `af clean` with dry-run,
      include-abandoned, max-age, force-by-name, archive, and Obsidian
      updates.
- [x] I5.5: ADR-059 — implement `af stack`, `af unstack`, and `af sync`
      using the reusable merge-detection contract.
- [x] I5.6: ADR-054 — implement `af status [--json] [--all]
[--filter STATE]`, including stack suffixes, `repo_slug` handling,
      bounded `gh` fan-out, and stable JSON.

### Implementation Stage 6 — Remote, sandbox, and secret transport

Do this after the local lifecycle is solid; it composes the same state
and command paths with remote/sandbox execution.

- [x] I6.1: ADR-049 — implement ephemeral envelope creation,
      source-and-delete wrappers, lazy stale-envelope sweep, and tests for
      redaction/no-secret-in-state invariants.
- [x] I6.2: ADR-041 — implement `af create --remote`, remote clone/path
      setup, remote tmux launch, `af resume` attach, and remote teardown.
- [x] I6.3: ADR-042 — implement `af create --sandbox`, sandbox launch,
      health check, teardown, and `--respawn`.
- [x] I6.4: ADR-041 + ADR-042 + ADR-049 — compose
      `--remote --sandbox` with remote-side envelope transport and teardown.
- [x] I6.5: ADR-046 — extend suspend/resume/done/clean tests across
      local, remote, sandbox, and remote+sandbox modes using fakes.

### Implementation Stage 7 — Proxy commands, PR AI, and retrospectives

These are deliberately late because they depend on config, state,
Obsidian notes, agent `BodyCmd`, and local/stack base resolution.

- [x] I7.1: ADR-048 — implement `af editor [--terminal|--visual]
[session]`, including remote URL fallback.
- [x] I7.2: ADR-048 + ADR-059 — implement `af diff [session]
[--base REF]`, argv-vs-shell parsing, token interpolation, and stacked
      base defaults.
- [x] I7.3: ADR-048 — implement base `af pr [session] [--title]
[--draft] [--web]`, push-if-needed, PR metadata detection, state
      update, ledger event, and Obsidian PR fields.
- [x] I7.4: ADR-057 — implement `af pr --ai` and `--ai-model` using
      primary-agent `BodyCmd`, body prompt construction, `flag_template.body`,
      empty-diff/empty-body errors, and `--web` incompatibility.
- [x] I7.5: ADR-058 — implement `af retro` filters (`--since`, `--tag`,
      `--search`, `--limit`) over archived notes.
- [x] I7.6: ADR-058 + ADR-057 — implement `af retro --ai` using
      `BodyCmd` with `BodyOpts.Cwd = ""`.

### Implementation Stage 8 — Hardening, verification, and v0 retirement

This stage should not add broad new feature surface.

- [x] I8.1: ADR-052 — add lifecycle state-machine property tests and
      document any invariants worth carrying into optional TLA+.
- [x] I8.2: ADR-050 + ADR-051 + ADR-053 — run full quality pass:
      coverage review, `make check`, cross-compile snapshot, and manual
      smoke plan for real tmux/ssh/sandbox paths.
- [x] I8.3: Update README, CHANGELOG, Godoc, ADR implementation
      frontmatter, TODO, and PROGRESS for all completed v1 behaviour.
- [x] I8.4: Remove the Rust v0 source tree (`src/`, `tests/`,
      `Cargo.toml`, `Cargo.lock`, `justfile`, Rust tool configs). Completed
      early during Stage 0 by explicit user override; no final v0 source
      cleanup remains.

### Implementation Stage 9 — Close out the 11 in-progress ADRs

After the Session 25 audit revealed that several Stage 5/7 items shipped
as placeholders (sync, pr --ai, retro --ai) and that ADRs 040/041/042/
046/048/049/052/053 still need their real-tool surface verified, this
stage exists to land the deferred logic and advance every in-progress
ADR to `implementation: complete`.

Wave 1 — deferred placeholders + release tooling (parallel agents):

- [x] I9.1: ADR-057 — wire `af pr --ai` to `agent.BodyCmd(BodyOpts{Cwd,
  Model})`; build the prompt from the worktree diff; handle
      empty-diff and empty-body errors; reject `--ai` with `--web`.
- [x] I9.2: ADR-058 — wire `af retro --ai` to `agent.BodyCmd` with
      `BodyOpts.Cwd = ""`; synthesise narrative from the collected
      notes; revert ADR-058 frontmatter to `in-progress` first, then
      advance to `complete` once shipped.
- [x] I9.3: ADR-059 — implement `af sync` real rebase algorithm:
      `git fetch`, `git rebase --onto parent-base parent-head head`,
      detect/report conflicts.
- [x] I9.4: ADR-053 — add `.goreleaser.yaml` and `make snapshot`
      target; verify cross-compile snapshot builds (darwin/arm64,
      linux/amd64, linux/arm64) with the installed goreleaser 2.15.4.

Wave 2 — integration tests + envelope wiring:

- [x] I9.5: ADR-048 — testscript scenarios for `af editor`, `af diff`,
      `af pr` using fake-path shadow binaries; verify token
      interpolation and `flag_template` expansion end-to-end.
- [x] I9.6: ADR-040 + ADR-046 — tmux integration testscript that
      exercises a smart-fake tmux state machine; verify SessionExists + CreateSession + suspend/resume respawn (bare vs non-bare).
- [x] I9.7: ADR-041 — SSH integration test using a smart-fake ssh
      that responds to `af doctor --remote` probes; covers all-present
      / sparse-host / failing-host cases.
- [x] I9.8: ADR-042 + ADR-049 — wired `secret.Envelope` into
      `PrepareRemoteWorkstream` and `LaunchSandboxWorkstream`
      (write-before, deferred Delete after). 7 unit tests cover
      success, skip-when-empty, write-failure, and nil-provider paths.
      Testscript skipped: `af create --sandbox` does not yet call
      `LaunchSandboxWorkstream` end-to-end; unit tests are the
      load-bearing evidence (see Task C deliverable in PROGRESS).

Wave 3 — close-out:

- [x] I9.9: Advanced ADR frontmatter to `complete` for 031, 040, 041,
      042, 046, 048, 049, 052, 053, 057, 058, 059. Only `pending`
      ADRs left are 060–064 (post-v1 scope).
- [x] I9.10: Refreshed README (status banner, command table, caveats),
      CHANGELOG (Stage 9 section), PROGRESS (Session 26 close-out).
      Residual scope going to ADR-060 (slicer-only sandbox), 061 (repo-
      scoped control), 062 (per-repo VM profiles), 063 (Tailscale +
      superterm remote), 064 (opinionated diff rendering).

### Implementation Stage 10 — Post-v1 ADRs (PAUSED mid-Wave-1)

> **Status**: Paused 2026-05-22. Wave 1 (I10.1–I10.4) work was produced
> by four parallel subagents in isolated git worktrees and merged back
> into the main working tree. The changes are committed on `main` under
> a `wip(stage-10)` commit; integration verification (`make check`)
> against the merged state has NOT been run yet. Resume by running
> `make check`, resolving any cross-agent lint, then committing
> Wave 1 proper.

Wave 1 — 4 ADRs in parallel (output present, unverified):

- [x] I10.1: ADR-060 — drop sbx, wire `af create --sandbox slicer`
      end-to-end through `LaunchSandboxWorkstream`. Real `slicer vm
  run` exec (no stub). 9 new tests.
- [x] I10.2: ADR-061 — `[control]` section in `<repo>/.af/config.toml`
      with layered precedence (CLI > repo > user > defaults).
      `ControlConfig`, `ResolveControl`, `ControlContext`, additive
      state.toml fields. 12 new tests.
- [x] I10.3: ADR-063 — `af control up/down/status` composing
      superterm + tailscale serve. New `internal/control` package,
      cobra wiring, testscript, fakes. 13 new tests.
- [x] I10.4: ADR-064 — opinionated diff rendering. New
      `internal/diff` package; hunk-piped path for interactive TTY,
      `git diff --stat` for non-TTY, `diffity base..head` for `--web`.
      8 new tests.

Wave 1 INTEGRATION:

- [x] I10.5: `make check` was green on the first integration run
      (worktree isolation prevented all cross-agent drift fears).
- [x] I10.6: Wave 1 committed as `feat(v1): Stage 10 Wave 1 — close
  I10.1-I10.4`.

Wave 2 — 1 ADR (depends on Wave 1):

- [x] I10.7: ADR-062 — `[sandbox.slicer.resources]` schema (`name,
  vcpu, ram_gb, storage_size, gpu_count, image, hypervisor`),
      validation (negative-ints, size grammar, hypervisor vocab,
      group-vs-resources mutual exclusion), `sandbox.ResolveLaunchGroup`
      with `ExecGroupProber` parsing `slicer vm group` output, state
      capture of 8 additive `Execution.sandbox_resource_*` fields,
      `cmd/af/create.go` plumbing. 14 new tests. Per-VM resource
      argv flags (--cpu/--memory/etc.) deferred with an inline
      `// ADR-062 §Resolution step 6` comment; managed groups are
      identified by name and the launch passes `--group <name>`.

Wave 3 — close-out:

- [x] I10.8: All five ADR frontmatter blocks (060/061/062/063/064)
      advanced to `implementation: complete`. Every numbered ADR
      from 031 to 064 is now `complete`. Only `pending` ADRs
      remaining are owner drafts 065/066/067.
- [x] I10.9: README "Caveats" updated (dropped `af create --sandbox
  not yet end-to-end`; added the optimistic group-shape match
      caveat from ADR-062 plus a pointer to the owner drafts).
      CHANGELOG gained a Stage 10 section. PROGRESS Session 28
      records the close-out.

### Implementation Stage 11 — ADR-065 slicer worktree transport

After Stage 10 closed every ADR up to 064, the owner accepted ADR-065
(slicer `wt push/pull` as the slicer sandbox transport). This stage
implements it so the v1 ADR set is complete up to and including 065.

- [x] I11.1: ADR-065 — `af create --sandbox slicer` now invokes
      `slicer wt push --launch [--hostgroup G] [--depth N] --tag af
    --tag af-session=NAME <worktree-path>` via the new
      `internal/sandbox/slicerwt.go` module. VM name parsed from
      output via permissive regex with last-word fallback.
- [x] I11.2: ADR-065 — additive `[slicer_wt]` state schema landed in
      `internal/session/state.go` with `SlicerWTState`,
      `SlicerWTLeaseState` constants, and `State.IsLeasedToVM()`
      helper. Round-trip tests in `state_test.go`.
- [x] I11.3: ADR-065 — new `af pull [session]` command
      (`cmd/af/pull.go`) calls `lifecycle.Pull` which runs
      `slicer wt pull <vm> <worktree-path>` and updates the lease
      to `pulled` with timestamp. Refusal sentinels for missing /
      already-pulled / discarded leases.
- [x] I11.4: ADR-065 — lease enforcement landed:
      `af done --force` and `af suspend --force` mark the lease
      `discarded`; without `--force` they refuse with a
      `ErrDoneLeasedToVM` / `ErrSuspendLeasedToVM` message pointing
      to `af pull`. `af pr` refuses outright on `held_by_vm`.
      `af diff` and `af editor` print a stderr warning. `af status`
      shows `[vm=X lease=S]` in the text row and exposes
      `slicer_wt_vm` / `slicer_wt_lease` in JSON. `af info` adds a
      "Slicer worktree:" section and a full `slicer_wt` block in
      JSON.
- [x] I11.5: ADR-065 — `SlicerWTAvailable` probe added to
      `internal/doctor/system.go`. Currently exposed as a function;
      wiring into `af doctor`'s default probe list is left as a
      `// TODO(ADR-065)` follow-up to keep the change small.
- [x] I11.6: Wave 3 close-out — README status banner updated to
      Stages 0–11; caveats list dropped ADR-065. CHANGELOG gained a
      Stage 11 section. PROGRESS Session 29 records the close-out.
      Every numbered ADR from 031 to 065 is now
      `implementation: complete`.

### Implementation Stage 12 — Follow-ups + ADR-066/067 (next session)

Small follow-ups from Stage 11, plus the next two pending ADRs. These
are the only `[ ]` items in this file.

- [ ] I12.1: Wire `internal/doctor.SlicerWTAvailable` into the
      `af doctor` default probe list as a non-blocking warning per
      ADR-065. The probe function exists; this is just the
      `defaultProbes()` registration + a `TestSystem_SlicerWTReported`
      test asserting that `af doctor` mentions the wt API status when
      slicer is installed.
- [ ] I12.2: Add `TestEditor_LeaseWarning` for the lease-warning path
      in `runEditor`. Use the existing `editorCommand` seam (or add
      one if missing) so the test never spawns a real editor. Verify
      stderr contains the "host worktree may be stale" message.
- [ ] I12.3: ADR-066 — implement VM agent-session export. Per the
      ADR, this means a host-side allowlist copy of
      `~/.claude/projects/**`, `~/.codex/sessions/**`, pi's resolved
      `sessionDir`, and harness session roots from the VM back to the
      host as part of `af pull` (or a sibling command). Read
      `docs/adr/066-agent-session-export-from-slicer-vms.md` for the
      exact allowlist + denylist + safety rules.
- [ ] I12.4: ADR-067 — automatic agent-session sync state machine.
      Per the ADR, this captures sync state in `state.toml` and runs
      the export from I12.3 automatically at sane points (after
      successful pull, before `af done`, on resume). Read
      `docs/adr/067-automatic-agent-session-export.md` for the
      details and exact failure-mode handling. May be parallelizable
      with I12.3 in worktrees if the state fields are sequenced
      carefully (I12.3 lands the export module; I12.4 wires the
      automatic triggers around it).
- [ ] I12.5: Wave 3 close-out for Stage 12 — advance ADR-066 and
      ADR-067 frontmatter to `implementation: complete`, update
      README/CHANGELOG/PROGRESS, check off I12.1–I12.5.

### Implementation Stage 13 — Gap-analysis batch (ADRs 068–072)

The five ADRs added by the gap-analysis pass on branch
`docs/gap-analysis-v1`. See the Stage 12 / 13 reading list above for
the scope summary; see each ADR for the full contract.

- [ ] I13.1: ADR-070 — implement the session-resolution chain:
      `arg` → `--session` flag → `AF_SESSION` env → cwd symlink →
      fzf picker (stderr, TTY-only) → `EX_NOINPUT`. Add `AF_SESSION`
      propagation via `tmux setenv` in `af create` / `af resume`.
      `internal/session/resolve.go` is the natural home; testscript
      coverage in `session-resolve.txt`.
- [ ] I13.2: ADR-071 — TTL-bounded PR cache:
      - Add `last_refreshed_at` and `last_refresh_error` to
        `PRState` (omitempty).
      - Add `[pr].refresh_ttl` to config schema (default `10m`,
        Go duration syntax).
      - Hook the refresh path through `af status`/`af info`
        (TTL-respecting) and `af clean`/`af sync`/`af done`
        (always force-refresh).
      - Add `--refresh` flag to `af pr`, `af status`, `af info`.
      - Emit `pr_state_changed` ledger events on flips.
      - Map empty-PR `--refresh` to `EX_DATAERR` (65).
- [ ] I13.3: ADR-068 §4 — lift per-file flock (ADR-037) to per-session
      flock at `<session>/.af.lock`. Mutating ops acquire exclusive
      with 30s timeout → `EX_TEMPFAIL` on timeout. Read-only ops
      (`list`, `status`, `info`) don't lock. Audit existing mutating
      commands; testscript coverage in `concurrency.txt`.
- [ ] I13.4: ADR-068 §1 — JSON envelope `{schema, data}` for every
      `--json`-bearing command. Existing `af status --json`/`af
      info --json` schemas migrate; new commands plug into
      `internal/jsonio/`. Errors on `--json` writes go to stderr as a
      JSON error doc.
- [ ] I13.5: ADR-068 §2 — sysexits exit-code table. Audit every
      `return fmt.Errorf` / `os.Exit` for code mapping; centralise
      in `internal/exitcode/`. Wire into `main` so a returned
      `*ExitError` sets the right code.
- [ ] I13.6: ADR-068 §5 — tab-completion. Audit each command in
      `cmd/af/` for `cmd.RegisterFlagCompletionFunc` and arg
      completion. Session/slot/host/agent/sandbox completions per
      §5 table.
- [ ] I13.7: ADR-069 §3 — strict name-collision check in `af
      create` covering active + suspended + archived. Verify the
      friendly error message + `EX_DATAERR`. Likely already
      in place against `sessions/`; verify against `archive/`.
- [ ] I13.8: ADR-069 §1 — add a CI/lint rule that rejects
      `net/http` imports outside `internal/sandbox/`, `internal/
      remote/`, etc. `golangci-lint` `depguard` config.
- [ ] I13.9: Wave 3 close-out for Stage 13 — advance ADR-068
      through ADR-072 frontmatter to `implementation: complete`,
      update README/CHANGELOG/PROGRESS, check off I13.1–I13.9.

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
