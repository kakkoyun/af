---
adr: 033
title: "Documentation Archival Policy (v0 → v1)"
status: accepted
implementation: complete
date: 2026-05-06
last_modified: 2026-07-03
supersedes: []
superseded_by: null
related: ["031", "032"]
tags: ["meta", "archival"]
---

# ADR-033: Documentation Archival Policy (v0 → v1)

## Context

ADR-031 retires the entire v0 design surface in one cut. The 30 v0
ADRs, the v0 SPEC, the v0 PLAN, the v0 PROGRESS log, the v0 CHANGELOG,
the v0 CONVENTIONS, the v0 mdBook scaffold, and the v0 reference
material all need a stable home. They must remain readable (the v0
implementation is reference material until removed) and they must
**not** be modified (their value is as a frozen historical record).

A future v2 (if it ever happens) needs the same treatment.

## Decision

### Layout

```
docs/
├── adr/                   # v1 ADRs (031+, append-only)
├── SPEC.md                # v1 specification (immutable)
├── PLAN.md                # v1 plan (immutable)
├── CONVENTIONS.md         # v1 conventions
└── v0/                    # frozen v0 archive
    ├── README.md          # archive index
    ├── adr/               # 30 v0 ADRs (001–030, with 026 retired)
    ├── SPEC.md, PLAN.md, CONVENTIONS.md, PROGRESS.md, TODO.md, CHANGELOG.md
    ├── reference/         # external-tools.md
    ├── planning/          # HANDOFF.md
    └── book/              # never-deployed mdBook
```

### Rules

1. **`docs/v0/**` is read-only.** No agent (lead or subagent) modifies any
   file under that prefix. Typo fixes that meaningfully affect
   comprehension may be made by the owner only, and must annotate the
   change in `docs/v0/README.md`.

2. **v0 ADRs keep their numbering.** v1 numbering starts at `031` and is
   append-only forever. Numbers are never reused across iterations.

3. **v0 → v1 cross-references** flow only one direction: v1 ADRs may
   cite v0 ADRs by archived path (`docs/v0/adr/014-three-layer-composition.md`).
   v0 ADRs are never updated to reference v1.

4. **Future iterations follow the same pattern.** If a v2 ever happens,
   move v1 to `docs/v1/` and start `docs/v2/` plus `docs/adr/` fresh
   with the next available number. The doc-archival commit pattern
   (this iteration's Stage A) is the template.

5. **Source code archival is separate.** This ADR is about
   documentation only. v0 source (`src/`, `Cargo.*`, etc.) is governed
   by ADR-031 §"Cutover": kept in-tree as reference, then removed in
   one commit when v1 reaches parity.

### What constitutes a v0 → v1 supersession

A v1 ADR that retires a v0 concept must:

1. Cite the v0 ADR(s) by number in `## Context` or `## Decision`.
2. Optionally list them in `supersedes` frontmatter — though semantic
   correctness is preserved either way, since v0 ADRs live in a
   separate directory and don't compete for v1 numbering.

This ADR uses convention (1) only: v0 ADRs aren't listed in
`supersedes`, since cross-iteration supersession is a different concept
than within-iteration supersession.

## Consequences

- The doc tree has a clear boundary: anything under `docs/v0/` is
  history; anything else is current.
- Searching ADRs is sometimes a two-pass operation (`grep -r foo docs/adr docs/v0/adr`).
  Acceptable trade-off — the volume isn't huge.
- v0 ADRs that contained useful patterns (e.g. ADR-018 external-tool
  testing) can be lifted into v1 ADRs verbatim or by reference; the
  v0 originals stay frozen.
- Future iterations get a clear pattern: archive then start fresh.

## Alternatives Considered

- **Keep v0 ADRs at `docs/adr/` and start v1 at 031 alongside.**
  Rejected: mixing iteration-eras in one directory makes the boundary
  blurry and complicates Obsidian Bases queries.
- **Delete v0 docs entirely; trust git history.** Rejected: v0 SPEC and
  some v0 ADRs (-006 session metadata, -011 lifecycle) are still useful
  as design references during v1 implementation.
- **Move v0 to a separate branch (`v0-archive`).** Rejected: archived
  docs are part of the project's intellectual history and should travel
  with the working tree.

## References

- ADR-031 — v1 master, scope cut, Rust source removal plan.
- ADR-032 — ADR conventions, supersession mechanics within a single iteration.
- [`docs/v0/README.md`](../v0/README.md) — the archive's own index.
