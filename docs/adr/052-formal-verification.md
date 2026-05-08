---
adr: 052
title: "Formal Verification Experimentation"
status: proposed
implementation: pending
date: 2026-05-06
last_modified: 2026-05-08
supersedes: []
superseded_by: null
related: ["031", "037", "046", "051"]
tags: ["go", "verification", "experimental"]
---

# ADR-052: Formal Verification Experimentation

## Context

The owner asked to experiment with property-based testing and formal
verification, with formal-verification scope captured as its own ADR.
Property-based testing is already in scope per ADR-051 (`testing/quick`).
This ADR addresses **formal verification proper** — proving properties
about state machines via models that can be checked against
implementations.

The first candidate is the **workstream lifecycle state machine** from
ADR-046. The owner wants confidence that the implementation can never
reach an invalid state (e.g. `completed → active`) regardless of
input ordering or partial failures.

This is **explicitly experimental and non-blocking for v1**. The aim is
to learn whether formal-verification techniques pay off in this
codebase before committing.

## Decision

### Scope

Formal-verification work in v1 is bounded to:

1. **The workstream lifecycle state machine** (ADR-046): `active`,
   `suspended`, `completed`, `abandoned`, with transitions driven by
   `af create`, `af suspend`, `af resume`, `af done`, `af done --force`.

2. **The agent slot status state machine** (ADR-039): `running`,
   `stopped`, `crashed`, `suspended`.

Other parts of the system (config merge, naming sanitization, UUID
derivation) are covered by property tests (ADR-051) — that's
already-shipped formal-light verification.

### Approach

Two layers, applied independently:

#### Layer 1 — Property tests of the state machine in Go

A small Go-side package `internal/lifecycle/` defines the state
machine as data:

```go
// internal/lifecycle/lifecycle.go (sketch)

type State int
const (
    Active State = iota
    Suspended
    Completed
    Abandoned
)

type Event int
const (
    Suspend Event = iota
    Resume
    Done
    DoneForce
)

// Transitions is a complete map of allowed (state, event) -> state.
var Transitions = map[Key]State{
    {Active, Suspend}:    Suspended,
    {Active, Done}:       Completed,
    {Active, DoneForce}:  Abandoned,
    {Suspended, Resume}:  Active,
    {Suspended, Done}:    Completed,
    {Suspended, DoneForce}: Abandoned,
}
```

Property tests via `testing/quick`:

- **Reachability**: every state is reachable from `Active` via some
  event sequence.
- **No-loop on terminal**: from `Completed` and `Abandoned`, no event
  is valid.
- **Idempotency of suspend on suspended workstream**: applying
  `Suspend` to `Suspended` is a no-op error, never a state change.
- **Composition**: `Resume(Suspend(s)) == s` when `s` is `Active`.

Production code at `internal/workstream/lifecycle.go` consults
`Transitions` rather than hand-coding switches, so the property tests
on the table directly govern the runtime.

#### Layer 2 — Optional TLA+ model

A `docs/specs/lifecycle.tla` TLA+ model encodes the same state machine
with TLA+'s temporal-logic operators. Properties:

**v1 verifies safety properties only.** The earlier draft of this
ADR also specified a liveness property ("every workstream eventually
reaches `Completed` or `Abandoned`"), but that's wrong: a `Suspended`
workstream is allowed to stay suspended indefinitely — the user's whole
point of `af suspend` is to park work without ending it. v1 makes no
liveness claim about workstream termination.

Safety properties (these _do_ hold):

- **Valid transitions only**: `[](Next \in AllowedTransitions(state))`.
  No path through the system reaches a state via an unallowed event.
- **Terminal stickiness**: `[](state = Completed => [](state = Completed))`
  and the equivalent for `Abandoned`. Once terminal, always terminal.
- **No mutation after terminal**: ledger events that would change `state.toml` (status, agents, pr) are forbidden once `state = Completed` or `state = Abandoned`. Modeled as: every action precondition checks `state \notin {Completed, Abandoned}`.
- **Suspend idempotency**: `Suspend` from `Suspended` is a no-op (an
  error path that doesn't transition).
- **Resume reachability**: from `Suspended`, `Resume` is always
  enabled and transitions to `Active`.

The TLC model checker runs over a small bounded universe (e.g. ≤ 4
slot transitions) and verifies these safety properties.

The TLA+ model is **not run in CI**. It's a research artefact,
checked manually if a state-machine bug surfaces.

### What's NOT in scope

- Verification of concurrency invariants (`flock` correctness, etc.).
  Out of scope; we trust the OS.
- Verification of git operations. The git binary is the authority.
- Verification of network protocols. SSH is the authority.

### Success criteria

After v1.0 ships, evaluate:

- Did property tests on `Transitions` catch any real bugs?
- Did the TLA+ model catch anything property tests missed?
- Was the maintenance cost (writing the model, running TLC manually)
  worth the confidence gain?

If yes to (1) only: keep property testing, drop TLA+.
If yes to (1) and (2): expand TLA+ to cover the agent-slot machine.
If no to both: drop both, document the experiment outcome in a follow-up ADR.

### Tooling

- **Property tests**: stdlib `testing/quick`. Already covered by ADR-051.
- **TLA+**: TLA+ Toolbox or `tlapm` from the [TLA+ project](https://lamport.azurewebsites.net/tla/tla.html). External tool, not vendored.

## Consequences

- Property tests on the lifecycle table are essentially free (small Go code).
- TLA+ is a learning experiment with bounded cost (one `.tla` file, one CI-skip).
- If the experiment fails, dropping it is a single commit.

## Alternatives Considered

- **Skip formal verification entirely.** Rejected; the owner explicitly asked to experiment.
- **Use Coq / Lean / Isabelle for proper proofs.** Rejected; out-of-skill investment for marginal benefit on a state machine this small.
- **Model-checking via `gomc` or similar.** Rejected; less mature than TLA+, no clear win.
- **Run TLA+ in CI**. Rejected for v1; the model checker can be slow and the value is in deliberate research, not gating commits.

## References

- [TLA+ Home Page](https://lamport.azurewebsites.net/tla/tla.html)
- [Hillel Wayne — Practical TLA+](https://www.hillelwayne.com/post/practical-tla/)
- ADR-031 — v1 master.
- ADR-037 — workstream metadata (status field).
- ADR-046 — suspend/resume lifecycle.
- ADR-051 — testing strategy (property tests).
