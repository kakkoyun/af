# ADR-015: Subagent Coordination Patterns

**Status:** Accepted
**Date:** 2026-04-21

## Context

Session 2 of development established an important lesson: a subagent overwrote
`session/ledger.rs`, a file the lead had already implemented, because neither the
lead nor the subagent had explicit file-ownership rules. Recovery required restoring
the lead's version post-integration.

As `af` enters the pattern-hardening sprint (ADRs 015–021, Lanes A–C), multiple
subagent sessions will work in parallel on disjoint modules. Without a coordination
contract, the Session 2 overwrite pattern will repeat.

`AGENTS.md` already codifies the coordination principle in prose (§Subagent
Coordination). This ADR elevates it to an accepted design decision with a concrete
file-ownership manifest and a standard dispatch protocol.

## Decision

### File-ownership manifest

The following files are **shared** and may only be modified by the lead agent
(the human operator or the primary orchestrating session) during Phase IV
integration. No subagent writes to them during its lane:

- `Cargo.toml` — features, dependencies, lint config
- `src/cli.rs` — subcommand and argument definitions
- `src/lib.rs` — crate-level module graph
- `src/provider/mod.rs` — provider trait definitions and factory
- `src/cmd/mod.rs` — command dispatch table
- `README.md` — user-facing contract
- `CHANGELOG.md` — release notes
- `TODO.md` — task checklist
- `PROGRESS.md` — session narrative log
- `docs/adr/README.md` — ADR index

All other files are **owned** by at most one lane at a time. The lane spec (§9 of
the sprint plan) assigns ownership explicitly. A lane that determines it needs a
shared file **stops** and reports to the lead; it does not edit the file.

### Lane dispatch protocol

Every subagent dispatch prompt must include:

1. **Branch name** — `lane-<id>-<short-description>` (e.g., `lane-a1-workspaces`).
2. **Owns** — explicit list of absolute paths this lane may write.
3. **Does-not-touch** — the shared-file list above (copy verbatim).
4. **Referenced ADRs** — the accepted ADRs that govern this lane's design.
5. **TDD stance** — red → green → refactor → `cargo fmt && cargo clippy && cargo test` → commit.
6. **Commit format** — `<type>(<scope>): <what>` per `AGENTS.md`.
7. **Handback protocol** — push branch, open draft PR or output diff digest, stop.
   Do NOT merge to `main`.

### Lead integration protocol

After receiving a lane's work:

1. Review the diff. Every line must be understood before merging.
2. Run the full check in a clean checkout: `cargo fmt --check && cargo clippy --all-targets --all-features -- -D warnings && cargo test --all-features`.
3. Update shared files (wire new modules into `lib.rs`, add deps to `Cargo.toml`,
   add subcommands to `cli.rs`).
4. Run the full check again after wiring.
5. Merge to `main` (or sprint branch).
6. Append a PROGRESS.md Session entry summarising the integration.

### Definition-of-Done template (Pattern P7)

Every lane and integration commit closes with a PROGRESS.md entry following this shape:

```
## YYYY-MM-DD — Session N: <Lane id> — <title>

### Done
- <bullet per task completed>

### Design decisions
- <ADRs written or referenced>

### Files touched
- <owned files list>

### Tests
- N new, all green. Full check clean.

### Next
- <pointer to next lane or integration step>

### Blockers
- <external deps or open questions, or "none">
```

## Consequences

- Subagent sessions are safe to run in parallel on disjoint module trees.
- The shared-file manifest is the canonical reference; `docs/CONVENTIONS.md`
  duplicates it for discoverability.
- Any modification to the shared-file list requires a new ADR or an amendment to
  this one — it cannot change silently in AGENTS.md prose.
- Lead integration overhead is real: every lane requires a review + wiring pass.
  This is intentional; quality gates must not be bypassed for throughput.
