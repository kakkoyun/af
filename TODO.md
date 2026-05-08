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

## Stage D — ADRs 031–053 (23 commits)

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

## Stage E — ADR index

- [ ] C37: `docs(adr): docs/adr/INDEX.md (v0 archive link + v1 ADRs 031–053)`

---

## After the doc pass — implementation work

This section is **deliberately not pre-phased** per user directive. Each
ADR carries its own implementation lifecycle; lanes are picked up in the
order their ADR dependency graph dictates. As ADRs flip to
`implementation: in-progress`, items appear below as a flat checklist.

The expected lane sequence (subject to change once ADRs settle):

1. Toolchain bootstrap (ADRs 034, 035, 050, 051, 053).
2. Foundation packages: config, session, git, naming, uuid, mux/tmux (ADRs 036, 037, 038, 040).
3. Agent providers (ADR 043).
4. Core commands `create`/`done`/`list`/`resume`/`session-branch` (ADRs 039, 044).
5. Lifecycle commands `setup`/`suspend`/`resume`/`doctor`/`note`/`clean`/`config`/`completions` (ADRs 045, 046, 047, 056).
5b. Workstream introspection: `af status`, `af info`, `af retro` (ADRs 054, 055, 058).
5c. Stack support: `af stack`, `af unstack`, `af sync` (ADR 059).
6. Remote and sandbox (ADRs 041, 042, 049).
7. Proxy commands `editor`/`diff`/`pr` (ADR 048).
8. Formal-verification experimentation (ADR 052).
9. Rust source removal (single commit).

Items will be added as flat checkboxes once their ADRs are accepted.

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
