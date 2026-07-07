---
adr: 032
title: "ADR Conventions for v1"
status: accepted
implementation: n/a
date: 2026-05-06
last_modified: 2026-07-03
supersedes: []
superseded_by: null
related: ["031", "033"]
tags: ["meta", "conventions"]
---

# ADR-032: ADR Conventions for v1

## Context

v0 ADRs used Michael Nygard's plain-prose format with a `**Status:**`
line at the top. As the catalogue grew to 30 entries it became hard to
filter, aggregate, or correlate ADRs without grep. v1 introduces an
Obsidian Bases-compatible frontmatter so ADRs can be queried and
displayed in vault-side dashboards.

The owner asked for a `status` frontmatter field with values like
"accepted / abandoned / implemented." Those three terms conflate two
independent lifecycles: the **decision** (accepted? abandoned?) and the
**code** (implemented?). This ADR splits them.

## Decision

Every v1 ADR begins with a YAML frontmatter block matching this schema:

```yaml
---
adr: NNN # zero-padded three-digit number
title: "..." # human-readable title
status: proposed # see lifecycle below
implementation: pending # see lifecycle below
date: YYYY-MM-DD # original authoring date (immutable)
last_modified: YYYY-MM-DD # bump on any body change
supersedes: ["NNN", ...] # ADR numbers this replaces (may be empty list)
superseded_by: null # set to "NNN" when a later ADR retires this
related: ["NNN", ...] # informational cross-refs (may be empty list)
tags: ["..."] # free-form, used by Obsidian Bases filters
---
```

### Status lifecycle (the decision)

```
proposed ──► accepted ──► superseded
   │             │
   │             └──► deprecated
   │             │
   │             └──► abandoned
   │
   └──► rejected
```

| Value        | Meaning                                               |
| ------------ | ----------------------------------------------------- |
| `proposed`   | Drafted; under review; not yet ratified by the owner  |
| `accepted`   | Ratified; the decision stands                         |
| `rejected`   | Considered and declined; kept as a record of "no"     |
| `superseded` | Replaced by a later ADR; `superseded_by` is set       |
| `deprecated` | No longer applies but not formally replaced           |
| `abandoned`  | Was accepted but never executed; the project moved on |

### Implementation lifecycle (the code)

```
pending ──► in-progress ──► complete

n/a (for meta ADRs that have no code)
```

| Value         | Meaning                                                       |
| ------------- | ------------------------------------------------------------- |
| `pending`     | Code matching this ADR has not started                        |
| `in-progress` | Code is being written                                         |
| `complete`    | Code lives up to the ADR; tests cover the contract            |
| `n/a`         | This ADR is meta (e.g. conventions, archival policy); no code |

The two fields are **independent**. An accepted ADR can have
`implementation: pending` for weeks; an `accepted, complete` ADR can
later become `superseded, complete` (the original code may still be
running while the supersession's implementation is `pending`).

### File naming

```
docs/adr/NNN-kebab-case-title.md
```

- `NNN` is zero-padded three-digit. Numbers are **append-only** from `031` onward.
- Title slug is kebab-case (`v1-go-rewrite-and-scope-reduction`, `tmux-only-multiplexer`).
- A number is **never reused**; an abandoned ADR keeps its number.

### Body structure

Every ADR body, after the frontmatter, follows this order:

```
# ADR-NNN: Title

## Context
…why we need a decision…

## Decision
…what we decided…

## Consequences
…what changes because of this decision…

## Alternatives Considered  (optional)
…what we rejected and why…

## References  (optional)
…cross-links to other ADRs, SPEC, external sources…
```

### Body rules

- **Maximum 3 pages of body.** If an ADR doesn't fit, split it into multiple ADRs that reference each other.
- **Code listings: type signatures only.** No full implementations. Implementation lives in code; the ADR captures the _why_.
- **Every supersession** lists the retired ADR number in `supersedes` frontmatter and explains the why in `## Context` or `## Decision`.
- **Diagrams** use ASCII; no embedded images.
- **No mutable wisdom in the body.** If the body needs updating after a key insight, write a new ADR rather than editing the old.

### Updates after `accepted`

An accepted ADR's body may be updated only for:

1. **Typo fixes** — bump `last_modified`, no status change.
2. **Clarifications** that don't change the decision — bump `last_modified`, document in a `## Amendments` section appended to the body.
3. **Anything material** — write a new ADR that supersedes this one.

## Consequences

- ADRs become Obsidian Bases-queryable. A `.base` file at
  `docs/adr/.adr-base.md` (proposed in ADR-047 territory) can render
  "all `accepted, in-progress` ADRs" or "all `proposed` ADRs awaiting
  review" as live tables.
- The frontmatter inflates each ADR by ~10 lines but the trade-off is
  clearly worthwhile.
- The `last_modified` field requires discipline; CI may later add a
  pre-commit check that bumps it automatically. Out of scope for v1.

## Alternatives Considered

- **Single `status` field with values `accepted | abandoned | implemented`** (the owner's initial suggestion). Rejected because it conflates the decision and the implementation. Documented in the Context section above.
- **Plain prose like v0.** Rejected because it doesn't support filtering/aggregation.
- **External tools (`adr-tools`, `adrdir`).** Rejected: another dependency for marginal value; the schema is small enough to maintain by hand.

## References

- [Michael Nygard's ADR format](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions/) — original inspiration.
- [MADR (Markdown Architectural Decision Records)](https://adr.github.io/madr/) — comparison point; we deliberately don't follow its template literally.
- ADR-031 — v1 master.
- ADR-033 — documentation archival policy.
- `docs/CONVENTIONS.md` — file ownership; ADR files are owned by their author with owner review.
