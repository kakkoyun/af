# HANDOFF — Pattern-Hardening Sprint

**Scope:** cold-start entrypoint for any session (human or agent) picking up the
sprint toward `af 0.1.0`. Transient; delete when `v0.1.0` tags.

**Last updated:** 2026-04-28, Session 11. Phase III fully merged to `main`;
Phase IV has three open code items + two doc items before the release gate.

---

## 1. Read these, in this order (≈15 min)

1. `CLAUDE.md` — constitution (TDD, verification, immutability, commit format).
2. `AGENTS.md` — working agreement (process, subagent coordination).
3. `docs/CONVENTIONS.md` — file-ownership manifest (who owns what).
4. `TODO.md` — current checkbox state. Phases 0–III complete; Phase IV has
   three unchecked items.
5. `PROGRESS.md` — narrative log. Tail is freshest (Session 10 is last entry).

**Optional / reference only:** `docs/SPEC.md` + `docs/PLAN.md` are immutable.
`docs/adr/README.md` indexes all 29 accepted ADRs (001–030 minus dropped 026).
`docs/reference/external-tools.md` is the verified CLI-surface reference for
external tools (slicer, sbx, workspaces, ssh, gh).

> `docs/planning/adr-drafts.md` and `docs/planning/gap-analysis.md` were
> deleted in Session 11 — all content landed in canonical ADRs and
> `docs/reference/external-tools.md`.

---

## 2. Where we are right now

| Phase | Status |
|---|---|
| 0 — Foundation | ✅ complete |
| 1 — Local MVP | ✅ complete |
| 2 — Multi-Agent + Config | ✅ complete |
| I — Pattern-Hardening / Lane D foundation | ✅ complete |
| II — Design ADRs (015–021) | ✅ complete |
| II.5 — Revision round (ADRs 022–029 − 026) | ✅ complete |
| III — Implementation (7 lanes) | ✅ complete — all merged to `main` (Session 11) |
| **IV — Integration (lead-only)** | 🟡 in progress — 3 code items + 2 doc items open |
| IV.5 — ADR-030 follow-through (L-SKILL) | 🔲 not started — greenfield, no blockers |
| V — Release gate (user-triggered) | ⏳ blocked on IV |

**Codebase state (verified 2026-04-28, branch `main` @ `03db316`):**
`cargo fmt --check`, `cargo clippy --all-targets -- -D warnings`, `cargo test`,
and `RUSTDOCFLAGS="-D warnings" cargo doc` all green. 626 tests / 9 suites.
29 ADRs accepted (001–030 minus dropped 026). `main` is 75 commits ahead of
`origin/main` (not yet pushed — user-gated).

### What is wired and working

| Feature | Status |
|---|---|
| `--remote`, `--sandbox`, `--yolo`, `--agent`, `--agent-sandbox` on `af create` | ✅ binary works |
| `af list` STATUS column (orphan detection via provider liveness) | ✅ |
| `af editor` remote URL schemes (vscode-remote, cursor, zed ssh) | ✅ |
| cmux multiplexer — select via `config.toml: multiplexer = "cmux"` | ✅ |
| `--sandbox --remote` (slicer remote-daemon URL mode, ADR-024) | ✅ |
| DD Workspaces provider | ✅ |
| mdBook user guide (`book/`) | ✅ |

### What is NOT wired yet (Phase IV open code)

| Item | Gap | File(s) |
|---|---|---|
| `af auth` CLI subcommand | `src/cmd/auth.rs` + `src/auth/` exist and compile, but **no `Auth` variant in `src/cli.rs`** — binary returns "unrecognized subcommand 'auth'" | `src/cli.rs`, `src/cmd/mod.rs` dispatch |
| `keyring` optional deps | `Cargo.toml` has `keyring = []` feature stub but **no `dep:keyring`, `dep:secrecy`, `dep:zeroize`** as `optional = true` — feature is a no-op | `Cargo.toml` |
| Overnight-yolo guard (A-d) | `--yolo` emits no warning when run without sandbox — directive D7 | `src/cmd/create.rs` |

### What docs are stale (Phase IV open docs)

| File | Problem |
|---|---|
| `README.md` | `--remote`/`--sandbox`/`--yolo` still listed as "Planned (not yet implemented)"; `af auth` missing from command table; no mention of `--agent-sandbox` or cmux config |
| `CHANGELOG.md` | "Deferred to 0.2.0" section lists DD Workspaces, `af auth`, `af editor` remote, `--sandbox --remote`, orphan detection, mdBook, cmux — **all shipped** — section is incorrect |
| `PROGRESS.md` | No Session 11 entry (merge to main + lane cleanup) |

---

## 3. What to do next (exact next-actions)

Work directly on `main`. All lanes are merged. No worktrees needed for these tasks.

**Step 1 — Wire `af auth` into `src/cli.rs`.**

Add `Auth(cmd::auth::AuthArgs)` variant to the `Commands` enum in `src/cli.rs`.
Add the dispatch arm in `main.rs`. Add `keyring`, `secrecy`, `zeroize` as
`optional = true` deps in `Cargo.toml` and populate:
```toml
keyring = ["dep:keyring", "dep:secrecy", "dep:zeroize"]
```
Verify `--features keyring` and `--no-default-features` both compile. Add
`#[cfg(feature = "keyring")]` gates in `src/auth/keyring.rs` if not already
present. Red-first test: `af auth setup --provider host` round-trips.

**Step 2 — Overnight-yolo guard (A-d).**

In `src/cmd/create.rs`: when `opts.yolo == true` AND `opts.sandbox == false`
AND `opts.agent_sandbox != AgentSandbox::Os`, emit `eprintln!` warning
(directive D7). Red-first integration test in `tests/cli_test.rs`.
Commit: `feat(cmd/create): warn on unsandboxed --yolo (A-d)`.

**Step 3 — Fix `README.md`.**

- Remove "Planned (not yet implemented)" block under `af create` options —
  `--remote`, `--sandbox`, `--yolo` all work.
- Add `--agent-sandbox <none|os>` to the `af create` options table.
- Add `af auth` row to the Commands table.
- Add note that multiplexer is selectable via `config.toml: multiplexer = "cmux"`.
- Keep "Planned" block only for `--remote host` workspaces and sandbox variants
  that are genuinely deferred.

**Step 4 — Fix `CHANGELOG.md`.**

Move the following out of "Deferred to 0.2.0" and into a new
`#### Pattern-Hardening Sprint (2026-04)` section under `[0.1.0]`:
- DD Workspaces provider
- `af auth setup/reroll/status/clear` (wired behind `keyring` feature)
- `af editor` for remote sessions (SSH + URL schemes)
- `--sandbox --remote` remote daemon mode
- Orphan detection in `af list`
- mdBook user guide
- cmux multiplexer
- `--agent-sandbox=os` per-agent OS sandbox (ADR-028)
- `af skill install` (ADR-030) — add when L-SKILL lands

Date stamp stays `Unreleased` until `v0.1.0` tag is cut.

**Step 5 — PROGRESS.md Session 11 entry.**

Append entry for this session: merge `phase-iv-integration` → `main`
(fast-forward, 37 commits), 7 lane worktrees + branches removed, full status
audit, HANDOFF.md refreshed. 626 tests still green.

**Step 6 — Phase IV.5: Lane L-SKILL.**

Greenfield work, no blockers. Per ADR-030:
- New subcommand `af skill install [--url URL] [--skill-dir DIR] [--hook-dir DIR]`
- `book/src/skills/af.md` bundle page with three fenced blocks (SKILL.md +
  `af-workstream.sh` + `af-session-bind.py`)
- `hooks/af-session-bind.py` in-tree

**Step 7 — Phase V release gate (user-triggered).**

`just release-dry-run` → verify 6-matrix build → user approves →
`git tag -a v0.1.0` + `git push origin main` + `git push origin v0.1.0`.

---

## 4. Rules of engagement (condensed from CLAUDE.md + AGENTS.md)

- **TDD — no exceptions.** Test → red → impl → green → refactor → `just
  check` → commit.
- **`just check` = fmt + clippy (pedantic, `-D warnings`) + test + deny +
  `RUSTDOCFLAGS="-D warnings" cargo doc`.** All pass, every commit.
- **Commit format:** `<type>(<scope>): <description>`. One logical change per
  commit. No Co-Authored-By trailers.
- **Never touch shared files from a subagent lane:** `Cargo.toml`, `src/cli.rs`,
  `src/lib.rs`, any parent `mod.rs`, `README.md`, `CHANGELOG.md`, `TODO.md`,
  `PROGRESS.md`, `docs/adr/README.md`. Lead integrates in Phase IV.
- **ADRs are immutable once accepted.** New decisions → new ADR with explicit
  supersession header.
- **No `unsafe` (forbidden), no `todo!()`, no `.unwrap()` in library code.**
- **Every user-facing change updates `README.md`.** This sprint additionally
  requires updating `book/src/commands/<cmd>.md` (once Lane L-BOOK scaffolds
  the tree).
- **PROGRESS.md gets one entry per session**, appended — never edited in
  place.

---

## 5. How you signal you're done with your piece

1. All boxes ticked in the relevant `TODO.md` section.
2. `just check` green on the working branch.
3. PROGRESS.md entry appended with: what was done / ADRs referenced / files
   touched / tests delta / next pointer / blockers (or "none").
4. If you touched shared files, stop and flag the lead instead.
5. Leave this HANDOFF.md updated: bump "Last updated" + §2 status + §3
   next-action.

---

## 6. When this file is deleted

When `v0.1.0` tags, delete this file. The planning transients
(`adr-drafts.md`, `gap-analysis.md`) were already deleted in Session 11.
The durable artifacts (ADRs, CONVENTIONS.md, README, book, CHANGELOG) carry
forward the knowledge without these transients.

If the sprint pauses and resumes later, this file is the re-entry point —
update §2 and §3, then read `PROGRESS.md` to pick up where the prior session
left off.
