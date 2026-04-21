# ADR-023: Sandbox Agent-Layer Conflict Resolution

**Status:** Accepted
**Date:** 2026-04-21

## Context

`src/provider/slicer.rs` mixes two slicer abstractions: `slicer vm {add,delete}`
for lifecycle plus `slicer {claude,codex,amp,copilot}` for agent launch,
falling back to `slicer workspace` for unknown agents. `src/provider/docker.rs`
calls `sbx create` in `SandboxProvider::create()` and `sbx run` via
`agent_sandbox_cmd()` — technically a double-create, because `sbx run` creates
the sandbox on first use.

The question is whether these splits are design drift that needs unwinding, or
shipped behavior that should be ratified as the intended shape. This ADR
answers both, per-provider.

## Decision

- **Slicer:** keep the current split. Lifecycle is af-owned (so af can list and
  tear down without invoking agent-opinionated code); agent subcommands
  leverage slicer's built-in agent setup. The split is shipped and tested;
  changing it without user-visible benefit would be churn.
- **sbx:** drop `sbx create` from `SandboxProvider::create()` and let `sbx run`
  handle creation on first use. This matches sbx's own documentation, removes
  the double-create, and closes gap G6.

## Alternatives considered

- **Rework slicer to `slicer workspace .` + agent subcommand composition.**
  Cleaner in principle but touches `provider/slicer.rs` broadly with no
  user-visible benefit; rejected.
- **Keep `sbx create` and switch `agent_sandbox_cmd` to `sbx exec` / `sbx run
  <name>`.** More ceremony than the sbx docs prescribe; rejected.

## Consequences

- The slicer change is a no-op; this ADR simply ratifies shipped behavior.
- The sbx change is folded into Lane L-FIX alongside the `docker.rs` workdir
  and agent-map bugs; all three sbx fixes ship in one commit sequence before
  Phase II.5 opens.
- Provider authors writing new sandbox backends should follow the sbx pattern:
  let the launch command create on first use; keep provider `create()` cheap
  (name-only) or a no-op.
