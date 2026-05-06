# ADR-029: External Tool Testing — `CommandRunner` Trait Dropped (Addendum to ADR-018)

**Status:** Accepted
**Date:** 2026-04-21
**Supersedes:** ADR-018 §Decision (the `CommandRunner` trait).

## Context

ADR-018 introduced a `CommandRunner` trait so that every provider could be
tested without the real external binary (slicer, sbx, workspaces, etc.)
present. In practice the trait threads `Box<dyn CommandRunner>` through every
provider constructor (~24 call sites) at a real indirection cost.

Re-reading ADR-018's Context, the actual problem being solved is **CI
fragility from external tool availability** — CI agents that don't have
slicer/sbx/workspaces installed should still run the test suite. That problem
is solved by feature gates alone (`#[cfg(feature = "slicer")]` and friends),
combined with `assert_cmd` for integration coverage when the binary is
present.

The `CommandRunner` trait solves a different problem — **unit-test
determinism on shell-output branches** — at a cost that outweighs its
benefit at two providers.

## Decision

- **Adopt feature gates plus `assert_cmd` only.** Drop the `CommandRunner`
  trait. Providers call `std::process::Command` directly.
- **Integration tests** run under the appropriate feature gate when the
  external binary is available. They exercise the real tool surface.
- **Unit tests stub at the public provider surface**, not at the process
  boundary. A test that needs to verify what args were passed to `sbx` can
  construct the command and assert on its `args()`, without a trait dyn.
- **Escape hatch:** if a specific provider later needs branch coverage on
  shell failure paths (e.g., a `sbx` daemon crash mid-stream), that provider
  introduces a local `CommandRunner`-style trait scoped to its module — not a
  workspace-wide trait.

## Alternatives considered

- **Keep the trait, accept the 24-call-site threading.** Rejected: the cost
  is paid every Phase III lane touches the provider layer, with no unit-test
  coverage benefit that `assert_cmd` or command inspection cannot match.
- **Generics with `#[cfg(test)]` default.** Rejected: equivalent ergonomic
  cost to the trait at two providers; adopt only if branch-coverage demand
  appears per-provider.

## Consequences

- Saves roughly 200 LOC and one coordination axis from every Phase III lane
  that touches providers.
- Lane L-REMOTE and Lane L-SBX-DAEMON both simplify.
- Integration tests must be guarded by feature flags; a CI matrix that runs
  with and without each feature catches the two interesting cases (tool
  present, tool absent).
- ADR-018's Context (CI fragility) remains valid; this ADR narrows the
  Decision section to the minimum mechanism that solves it.
