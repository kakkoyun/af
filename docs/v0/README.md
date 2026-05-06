# af — v0 (Rust era) archive

This directory preserves the documentation produced during the Rust
implementation of `af`. **It is read-only.** v1 (the Go rewrite) lives
under `docs/` at the repository root.

If you came here looking for the active project, go up one level.

---

## Why this archive exists

`af` was originally a Bash tool (`cf` / "Claude Focus") that lived in
`kakkoyun/dotfiles`. It was rewritten in Rust through 2026-03 → 2026-04
across eleven sessions. The Rust implementation reached ~16k LOC across
60 source files with 626 tests and 30 ADRs before the project pivoted
to Go for the v1 rewrite.

The **v1 boundary** was drawn for two reasons:

1. **Scope reduction.** The Rust tree had drifted from "cf in Rust"
   into a 30-ADR architecture covering remote providers, sandbox
   composition, multi-multiplexer support, skill-bundle installers,
   provisioning pipelines, and more. v1 keeps a deliberately smaller
   surface (see `docs/adr/031` once written).
2. **Language fit.** The owner writes more Go than Rust day-to-day;
   Go's standard library covers most of what `af` needs, and the
   reduced scope makes Rust's strengths less compelling here.

The Rust implementation was **not released to anyone other than the
owner**, so there is no compatibility burden on v1.

---

## What's archived here

| Path | Contents |
|---|---|
| `PROGRESS.md` | Eleven-session narrative log (2026-03-26 → 2026-04-28) |
| `TODO.md` | Final v0 task checklist (pattern-hardening sprint complete; some Phase 3/4 items deferred to "0.2.0") |
| `CHANGELOG.md` | Keep-a-Changelog format; "0.1.0 - Unreleased" with a Deferred-to-0.2.0 section that v1 supersedes |
| `SPEC.md` | Full specification reverse-engineered from the original Bash `cf`. Immutable. |
| `PLAN.md` | Phased delivery plan for the Rust rewrite. Immutable. |
| `CONVENTIONS.md` | File-ownership manifest and subagent dispatch protocol. Updated through Phase II.5. |
| `adr/` | 30 ADRs (001–030, with 026 retired). v0 is **append-only and frozen** as of v1 cutover. |
| `reference/external-tools.md` | Verified CLI surface for `slicer`, `sbx`, `workspaces`, `ssh`, `gh`. Useful reference for v1 sandbox/remote work. |
| `planning/HANDOFF.md` | Final handoff state at end of Session 11. |
| `book/` | mdBook user-guide scaffold (never deployed). |

---

## What's NOT in this archive (yet)

The **Rust source tree** (`src/`, `tests/`, `Cargo.toml`, `Cargo.lock`,
`clippy.toml`, `deny.toml`, `rust-toolchain.toml`, `rustfmt.toml`,
`.cargo/`, `justfile`) is **still at the repository root**. It is kept
in-tree as a write-reference while v1 is implemented in Go. It will be
removed in a separate commit once Go has functional parity. Until then:

- Do **not** modify the Rust source.
- Do **not** build against it as if it were active code.
- Refer to it for behavioural questions ("how did v0 handle X?") only.

After removal, the Rust source remains accessible via git history
(`git log --all --oneline -- src/` and `git show <sha>:src/...`).

---

## How to read v0 ADRs

The 30 ADRs follow the [Michael Nygard format](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions/).
The full index lives at `adr/README.md`. Status values used in v0:

- **Accepted** — ratified
- **Superseded by ADR-NNN** — replaced by a later v0 ADR
- **(retired)** — never finalized (only ADR-026 has this state)

v1 ADRs use a richer status lifecycle defined in v1's ADR-032.

---

## Cross-references

- v1 ADRs supersede groups of v0 ADRs. The mapping table lives in
  v1's ADR-031 (master migration ADR).
- v1 reuses some v0 schemas verbatim (session metadata, naming rules,
  UUID-v5 derivation). Those are explicitly cited from v1 ADRs back
  into the relevant v0 ADR for traceability.
- The owner's working agreement (`AGENTS.md`, `CLAUDE.md`) is
  re-authored for v1 at the repository root; the v0 versions are
  preserved in git history but were not archived here separately
  because they were continuously edited rather than versioned.
