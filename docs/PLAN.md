# af — v1 Plan

> Lightweight plan for the v1 (Go) iteration. Sequencing follows the
> ADR dependency graph rather than a parallel "phases" artifact —
> per user directive, all implementation work is captured inside the
> ADRs themselves via their `implementation` frontmatter lifecycle.
>
> This document is **editable during the planning phase** so it can
> stay consistent with the v1 ADRs as they iterate. It freezes once
> all v1 ADRs are `accepted` (per ADR-032's status lifecycle); after
> that, design or sequencing changes go through new ADRs in
> `docs/adr/`. The Rust-era plan is
> archived at [`docs/v0/PLAN.md`](v0/PLAN.md).

---

## Mental model

`af` v1 is built in three concentric rings of work:

```
┌────────────────────────────────────────────────┐
│  Ring 3 — Polish & cross-cutting               │
│  ADR-052 formal verification                    │
│  ADR-050 lint, ADR-051 testing                 │
│  ADR-053 build & distribution                  │
│  ┌──────────────────────────────────────────┐  │
│  │  Ring 2 — Commands & integrations        │  │
│  │  ADR-044 doctor, ADR-045 setup           │  │
│  │  ADR-046 suspend/resume                  │  │
│  │  ADR-047 Obsidian, ADR-048 proxies       │  │
│  │  ADR-049 secrets                         │  │
│  │  ┌────────────────────────────────────┐  │  │
│  │  │  Ring 1 — Foundation               │  │  │
│  │  │  ADR-034 module layout              │  │  │
│  │  │  ADR-035 CLI framework (cobra)      │  │  │
│  │  │  ADR-036 config (TOML)              │  │  │
│  │  │  ADR-037 session schema             │  │  │
│  │  │  ADR-038 worktree layout            │  │  │
│  │  │  ADR-039 multi-agent model          │  │  │
│  │  │  ADR-040 tmux multiplexer           │  │  │
│  │  │  ADR-041 SSH remote                 │  │  │
│  │  │  ADR-042 sandbox providers          │  │  │
│  │  │  ADR-043 agent providers            │  │  │
│  │  └────────────────────────────────────┘  │  │
│  └──────────────────────────────────────────┘  │
└────────────────────────────────────────────────┘

Meta layer (out of band, lands first):
  ADR-031 master / ADR-032 ADR conventions / ADR-033 archival policy
```

Ring 1 is the **foundation**: data shapes, interfaces, and the smallest
useful binary (`af version`, `af list`). Ring 2 is **commands** that
exercise the foundation. Ring 3 is **the harness** — lint, test,
verification, build — applied continuously as Rings 1 and 2 grow.

The operational, topologically sorted implementation checklist lives in
[`TODO.md`](../TODO.md). This plan stays deliberately lightweight: it
explains the dependency shape, while TODO carries the concrete
checkboxes. Each ADR still carries its own `implementation` frontmatter
lifecycle (`pending → in-progress → complete`); progress flows through
those status flips and the TODO checklist.

---

## Sequencing

The ADR dependency graph dictates the order:

| Stage                | What                                                                            | Gating ADRs                  |
| -------------------- | ------------------------------------------------------------------------------- | ---------------------------- |
| **Meta**             | Master, conventions, archival policy                                            | 031, 032, 033                |
| **Bootstrap**        | Module layout, CLI framework, lint/test harness, build                          | 034, 035, 050, 051, 053      |
| **Foundation**       | Config, session schema, worktree layout, mux, agents                            | 036, 037, 038, 039, 040, 043 |
| **Core commands**    | `create`, `done`, `list`, `session-branch`, `agent {add,stop,list}`             | depends on Foundation        |
| **Lifecycle**        | `setup`, `suspend`/`resume`, `clean`, `doctor`, `note`, `config`, `completions` | 044, 045, 046, 047, 056      |
| **Inspection**       | `list`, `status`, `info`                                                        | 054, 055                     |
| **Stacking**         | `stack`, `unstack`, `sync`                                                      | 059                          |
| **Retro**            | `retro` (post-archive note mining)                                              | 058                          |
| **Remote & sandbox** | `--remote`, `--sandbox`, secret management                                      | 041, 042, 049                |
| **Proxies**          | `editor`, `diff`, `pr --ai`                                                     | 048, 057                     |
| **Polish**           | Formal verification experiments                                                 | 052                          |
| **v0 retirement**    | Rust source/tooling removed early at rewrite start by user override.            | ADR-031                      |

This sequence is descriptive, not prescriptive. The concrete execution
order is maintained in `TODO.md`, with static checks and the test
harness scaffolded before feature work. Two stages can be in flight at
once if their ADRs don't conflict; the `implementation` frontmatter is
the source of truth for what's currently in progress.

---

## What's intentionally absent

- **No release plan.** v1 is single-user; install via `go install` or `make install`.
- **No deprecation policy** for v1 → v2. We'll write one if and when v2 happens.
- **No SLA on ADR review turnaround.** ADRs are reviewed asynchronously by the owner.
- **No formal verification on the critical path.** ADR-052 is experimental; v1 ships without it.
- **No CI matrix beyond the four cross-compile targets.** No Windows, no `freebsd`, no `riscv64`.

---

## References

- [`docs/SPEC.md`](SPEC.md) — v1 specification (editable during planning, frozen after).
- [`docs/CONVENTIONS.md`](CONVENTIONS.md) — Go conventions, file ownership.
- [`docs/adr/`](adr/) — v1 ADRs 031–059.
- [`docs/v0/PLAN.md`](v0/PLAN.md) — v0 (Rust era) plan, archived.
