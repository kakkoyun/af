# Architecture Decision Records — v1

v1 ADRs are append-only from `031`. Each ADR follows the frontmatter
convention defined in [ADR-032](032-adr-conventions.md).

> **v0 archive.** The 30 ADRs (`001`–`030`, with 026 retired) from the
> Rust era are frozen at [`docs/v0/adr/README.md`](../v0/adr/README.md).

---

## v1 ADR catalogue

| ADR                                               | Title                                                                          | Status   | Implementation | Tags                           |
| ------------------------------------------------- | ------------------------------------------------------------------------------ | -------- | -------------- | ------------------------------ |
| [031](031-v1-go-rewrite-and-scope-reduction.md)   | v1: Migration to Go + Scope Reduction (master)                                 | proposed | pending        | meta, scope, v1                |
| [032](032-adr-conventions.md)                     | ADR Conventions for v1                                                         | proposed | n/a            | meta, conventions              |
| [033](033-documentation-archival-policy.md)       | Documentation Archival Policy (v0 → v1)                                        | proposed | complete       | meta, archival                 |
| [034](034-go-module-layout.md)                    | Go Module Layout & Idiom                                                       | proposed | pending        | go, layout, idiom              |
| [035](035-cli-framework-cobra.md)                 | CLI Framework — cobra + pflag                                                  | proposed | pending        | go, cli, cobra                 |
| [036](036-configuration-toml-layered.md)          | Configuration — TOML, layered, with global Obsidian vault paths                | proposed | pending        | go, config, toml               |
| [037](037-session-metadata-schema.md)             | Session Metadata Schema (state.toml + ledger.jsonl)                            | proposed | pending        | go, session, state, ledger     |
| [038](038-workstream-and-worktree-layout.md)      | Workstream + Worktree Layout (stable paths, sub-worktrees, per-repo discovery) | proposed | pending        | go, worktree, workstream, fs   |
| [039](039-multi-agent-multi-session.md)           | Multi-Agent Multi-Session Model                                                | proposed | pending        | go, agent, session, model      |
| [040](040-tmux-only-multiplexer.md)               | tmux-only Multiplexer                                                          | proposed | pending        | go, mux, tmux                  |
| [041](041-ssh-remote-model.md)                    | SSH Remote Model (no provider plugins)                                         | proposed | pending        | go, remote, ssh                |
| [042](042-sandbox-providers-slicer-sbx.md)        | Sandbox Providers (slicer + sbx)                                               | proposed | pending        | go, sandbox, slicer, sbx       |
| [043](043-agent-providers.md)                     | Agent Providers (claude, pi, codex; pi default)                                | proposed | pending        | go, agent, pi, claude, codex   |
| [044](044-doctor-and-install-hints.md)            | `af doctor` + Install Hints (local & --remote)                                 | proposed | pending        | go, doctor, install            |
| [045](045-af-setup.md)                            | `af setup` — Environment Companion to Doctor                                   | proposed | pending        | go, setup, command             |
| [046](046-af-suspend-resume-lifecycle.md)         | `af suspend` / `af resume` Lifecycle                                           | proposed | pending        | go, lifecycle, suspend, resume |
| [047](047-obsidian-integration.md)                | Obsidian Integration — Notes + Bases                                           | proposed | pending        | go, obsidian, notes            |
| [048](048-minimal-proxy-commands.md)              | Minimal Proxy Commands (editor, diff, pr)                                      | proposed | pending        | go, proxy, editor, diff, pr    |
| [049](049-secret-management.md)                   | Secret Management (keyring + tmpfs envelope)                                   | proposed | pending        | go, secrets, keyring, security |
| [050](050-code-quality-golangci-lint-pedantic.md) | Code Quality — golangci-lint Pedantic                                          | proposed | pending        | go, lint, quality              |
| [051](051-testing-strategy.md)                    | Testing Strategy                                                               | proposed | pending        | go, testing                    |
| [052](052-formal-verification.md)                 | Formal Verification Experimentation                                            | proposed | pending        | go, verification, experimental |
| [053](053-build-and-release-goreleaser-make.md)   | Build & Release — goreleaser + Make                                            | proposed | pending        | go, build, release, goreleaser |

23 ADRs total.

---

## Conceptual groupings

The catalogue's logical structure (the order ADRs land in is
dependency order; the order below is conceptual):

### Meta layer

ADR-031 sets the v1 boundary; ADR-032 codifies the format every other
ADR follows; ADR-033 makes v0 docs read-only.

- 031 master / 032 conventions / 033 archival policy

### Foundation

Module layout, CLI framework, configuration shape, session metadata,
and worktree filesystem layout. Together these define the static shape
of v1 before any command logic lands.

- 034 Go layout / 035 cobra / 036 config / 037 sessions / 038 worktrees

### Domain model

How `af` thinks about its workstreams and the things attached to them.

- 039 multi-agent / 040 tmux / 041 SSH remote / 042 sandbox / 043 agents

### Commands & integrations

Each user-facing command and the integrations that back it.

- 044 doctor / 045 setup / 046 suspend-resume / 047 Obsidian / 048 proxies

### Cross-cutting

Concerns that touch every ADR above.

- 049 secrets / 050 lint / 051 testing / 052 formal verification / 053 build & release

---

## Supersession map

v1 ADRs that retire v0 concepts:

| v0 ADR | Title                                         | Superseded for v1 by                                      |
| ------ | --------------------------------------------- | --------------------------------------------------------- |
| 001    | Agent Provider                                | 039 (multi-agent), 043 (agents)                           |
| 002    | Multiplexer Abstraction                       | 040 (tmux-only)                                           |
| 003    | Layered Configuration System                  | 036 (config)                                              |
| 004    | Remote Provider                               | 041 (SSH remote)                                          |
| 005    | Sandbox Provider                              | 042 (sandbox)                                             |
| 006    | Session Metadata                              | 037 (session schema)                                      |
| 007    | Obsidian Integration                          | 047 (notes + Bases)                                       |
| 008    | Phased Delivery                               | 031 (master), no formal phasing in v1                     |
| 009    | Provisioning System                           | 044 (doctor) — provisioning dropped                       |
| 010    | Platform-Aware Dependencies                   | 044 (doctor)                                              |
| 011    | Workstream Lifecycle & Session Ledger         | 037 (schema), 046 (suspend/resume)                        |
| 012    | Tri-State Approval Mode                       | 043 (agents — ApprovalMode enum)                          |
| 013    | Local Wiki Abstraction                        | 047 (Obsidian-only in v1)                                 |
| 014    | Three-Layer Composition Model                 | 042 (sandbox), 041 (remote) — composition simplified      |
| 015    | Subagent Coordination Patterns                | `docs/CONVENTIONS.md` (no v1 ADR; carried as conventions) |
| 016    | Secret Storage                                | 049 (secrets)                                             |
| 017    | Remote Session Resume                         | 041 (SSH remote), 046 (resume)                            |
| 018    | External Tool Dependency Testing              | 051 (testing)                                             |
| 019    | Remote Editor URL Schemes                     | 048 (proxy commands)                                      |
| 020    | mdBook User Guide Structure                   | (dropped — single-user, no guide)                         |
| 021    | Release Discipline                            | 053 (build & release)                                     |
| 022    | cmux Multiplexer Provider                     | 040 (tmux-only)                                           |
| 023    | Sandbox Agent-Layer Conflict Resolution       | 042 (sandbox)                                             |
| 024    | Remote Sandbox via Daemon URL                 | 042 (sandbox)                                             |
| 025    | Secret Boundaries                             | 049 (secrets)                                             |
| 027    | Remote = SSH Target                           | 041 (SSH remote)                                          |
| 028    | Agent-Level OS Sandbox                        | (dropped — out of v1 scope)                               |
| 029    | External Tool Testing — CommandRunner Dropped | 051 (testing)                                             |
| 030    | af Skill Bundle                               | (dropped — out of v1 scope)                               |

ADR-026 was retired without being finalised in v0.

---

## How to add a new ADR

1. Pick the next available number (next available is 054 as of this writing).
2. Create `docs/adr/NNN-kebab-case-title.md` with the frontmatter from ADR-032.
3. Body: Context → Decision → Consequences → Alternatives → References.
4. Add a row to the catalogue table above.
5. If superseding an existing v1 ADR, update its frontmatter
   `superseded_by` and link in the supersession map below the
   catalogue.
6. Commit as `docs(adr): ADR-NNN <title>`.

## How to update an existing ADR

Per ADR-032 §"Updates after `accepted`": typo/clarification only after
accept. Anything material → write a new ADR that supersedes the old.
