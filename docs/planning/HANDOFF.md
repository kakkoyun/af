# HANDOFF — Pattern-Hardening Sprint

**Scope:** cold-start entrypoint for any session (human or agent) picking up the
sprint toward `af 0.1.0`. Transient; delete when `v0.1.0` tags.

**Last updated:** 2026-04-21, Phase IV in progress (Phase II.5 + all seven
Phase III lanes landed on `phase-iv-integration`; four gates green as of
commit `ce0b79e`).

---

## 1. Read these, in this order (≈20 min)

1. `CLAUDE.md` — constitution (TDD, verification, immutability, commit format).
2. `AGENTS.md` — working agreement (process, subagent coordination).
3. `docs/CONVENTIONS.md` — file-ownership manifest (who owns what).
4. `TODO.md` — current checkbox state. **Phases 0–II are complete; Phase II.5
   and Phase III are open.**
5. `PROGRESS.md` — narrative of the last 9 sessions. Tail is freshest.
6. `docs/planning/gap-analysis.md` — the *why* behind every Phase II.5 / III
   decision. Long; skim §0 (directives), §8 (synthesized review), §9 (order of
   operations). Depth on demand.
7. `docs/planning/adr-drafts.md` — the *source text* for the ADRs Phase II.5
   will ratify. Each section becomes `docs/adr/NNN-<slug>.md`.

**Optional / reference only:** `docs/SPEC.md` + `docs/PLAN.md` are immutable.
`docs/adr/README.md` is the ADR index. The original sprint plan at
`~/.claude/plans/alrighty-analyz-this-project-compiled-snowglobe.md` is
historical; this doc supersedes it for current state.

---

## 2. Where we are right now

| Phase | Status |
|---|---|
| 0 — Foundation | ✅ complete |
| 1 — Local MVP | ✅ complete |
| 2 — Multi-Agent + Config | ✅ complete |
| I — Pattern-Hardening / Lane D foundation | ✅ complete (8 commits, 2026-04) |
| II — Design ADRs (015–021) | ✅ complete (5 ADRs + release-discipline) |
| II.5 — Revision round (ADRs 022–029 − 026) | ✅ complete (7 ADRs + amends + external-tools.md) |
| III — Implementation (7 lanes) | ✅ complete (L-FIX, L-REMOTE, L-SBX-DAEMON, L-AUTH, L-EDITOR, L-MUX-CMUX, L-AGENT-SANDBOX, L-BOOK) |
| **IV — Integration (lead-only)** | 🟡 in progress (modules wired, ADR-030 landed; Cargo deps + A-d guard + README/CHANGELOG remaining) |
| V — Release gate (user-triggered) | ⏳ blocked on IV |

**Codebase state (verified 2026-04-21, branch `phase-iv-integration`):**
`cargo fmt --check`, `cargo clippy -- -D warnings`, `cargo test`, and
`RUSTDOCFLAGS="-D warnings" cargo doc` all green at `ce0b79e`. 626 tests pass
across 9 suites (+234 since Session 9). 28 ADRs accepted (001–030 minus
dropped 026). 32 commits ahead of `main`.

---

## 3. What to do next (exact next-action)

Phase IV integration is in flight on branch `phase-iv-integration`. The
module wiring, ADR-030 (skill bundle installer), and rustdoc gate are all
landed. **Four items remain before the release gate (Phase V):**

**Step 1 — Wire `keyring` optional deps in `Cargo.toml` (Lane L-AUTH finisher).**

The auth module (`src/auth/*.rs`) is implemented but the feature arrays are
empty. Add `keyring`, `secrecy`, `zeroize` as `optional = true` deps and
populate `keyring = ["dep:keyring", "dep:secrecy", "dep:zeroize"]`. Verify
both `--features keyring` and `--no-default-features` builds compile. The
fallback path needs to be feature-gated to degrade cleanly when the feature
is off.

**Step 2 — Addendum A-d (Overnight-yolo guard).** In `src/cmd/create.rs`,
when `--yolo` is set AND no sandbox layer is active (`--sandbox=false` AND
`--agent-sandbox != os`), emit a prominent warning (directive D7). Red-first
integration test. One commit: `feat(cmd/create): warn on unsandboxed --yolo
(A-d)`.

**Step 3 — `README.md` + `CHANGELOG.md` polish.** Add the new surface
(`af auth`, `--agent-sandbox`, cmux selection via config, remote editor URL
schemes) to README. Stamp CHANGELOG `[0.1.0] - YYYY-MM-DD` and add the
comparison link. Link README to the book.

**Step 4 — Housekeeping (3 deletes + 1 update).** Update
`docs/CONVENTIONS.md` worktree table with the L-* lanes. Delete
`docs/planning/adr-drafts.md` and `docs/planning/gap-analysis.md` once the
user confirms the planning transients are no longer referenced.

**Step 5 — Phase V release gate (user-triggered).** `just release-dry-run`
→ verify 6-matrix build → user approves tag → `git tag -a v0.1.0 && git
push origin v0.1.0`.

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

When `v0.1.0` tags, this file, `docs/planning/adr-drafts.md`, and
`docs/planning/gap-analysis.md` all delete together. The durable artifacts
(ADRs, CONVENTIONS.md, README, book, CHANGELOG) carry forward the knowledge
without these transients.

If the sprint pauses and resumes later, this file is the re-entry point —
update §2 and §3, then read `PROGRESS.md` to pick up where the prior session
left off.
