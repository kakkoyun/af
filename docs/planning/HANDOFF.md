# HANDOFF — Pattern-Hardening Sprint

**Scope:** cold-start entrypoint for any session (human or agent) picking up the
sprint toward `af 0.1.0`. Transient; delete when `v0.1.0` tags.

**Last updated:** 2026-04-21, end of Session 9 (post critic/security/architect
synthesis, post directives D1–D7).

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
| **II.5 — Revision round (ADRs 022–029 − 026)** | 🟡 not started |
| **III — Implementation (7 lanes)** | 🟡 not started |
| IV — Integration (lead-only) | ⏳ blocked on III |
| V — Release gate (user-triggered) | ⏳ blocked on IV |

**Codebase state (verified 2026-04-21):** `cargo test` → 392 passed / 4 suites
/ 7.95s. ~11.4K LOC src. 14 ADRs accepted (001–015, 021 per §7 order). Build
clean since Lane D lifted the lld-linker override.

---

## 3. What to do next (exact next-action)

**Step 1 — Lane L-FIX (no ADR dependency, ~1 hour, red-first).**

Three standalone `fix(docker):` commits in `src/provider/docker.rs`:

- **G4** — line 56, `.args(["create", "claude", ".", "--name", name])` drops
  the caller-supplied workdir. Pass `workdir` through.
- **G5** — line 29, `KNOWN_SBX_AGENTS = ["claude", "codex"]` is missing the
  four newer agents. Extend to match `agent/` providers.
- **G6** — `SandboxProvider::create()` calls `sbx create` then `sbx run` later
  double-creates. Drop `sbx create`; let `sbx run` create on first use
  (ADR-023 ratifies this).

Each commit: red test first, minimum impl, `just check`, commit. No file
beyond `src/provider/docker.rs` + its test module.

**Step 2 — Phase II.5 ADR authoring** (lead-only, single branch
`phase-2.5-adr-revision`, ~2 hours).

Author ADRs 022, 023, 024, 025, 027, 028, 029 (A-b addendum) from the text in
`docs/planning/adr-drafts.md` — lift into Nygard format, commit each as
`docs(adr): ADR-NNN <title>`. Then:

- Write `docs/reference/external-tools.md` (CLI surface reference — G10).
- Amend ADR-017 probe to `StrictHostKeyChecking=accept-new` (security N2).
- Amend ADR-016 account naming to `<provider>` (drop `af/` prefix).
- Update `docs/adr/README.md` with 022–029.
- Delete `docs/planning/adr-drafts.md` when all ADRs land.

**Step 3 — Scope-call checkpoint.** Take §8.6 open items to the user:
Windows stance, headless `af auth`, multi-user keyring, `insta` vs
`include_str!` snapshot, awk vs `git-cliff`, `xtask` vs shell,
`cargo audit` CVE verify. Most defer to 0.2.0 with one-sentence ADRs.

**Step 4 — Phase III, 7 lanes in parallel** (per `TODO.md` Phase III
table — L-REMOTE, L-SBX-DAEMON, L-AUTH, L-EDITOR, L-MUX-CMUX,
L-AGENT-SANDBOX, L-BOOK). Each lane has explicit owns / does-not-touch
constraints; see §8.4 of gap-analysis.md and the subagent prompt template in
the original sprint plan §16.

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
