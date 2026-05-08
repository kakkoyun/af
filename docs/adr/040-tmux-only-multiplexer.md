---
adr: 040
title: "tmux-only Multiplexer"
status: proposed
implementation: pending
date: 2026-05-06
last_modified: 2026-05-06
supersedes: []
superseded_by: null
related: ["031", "039", "046"]
tags: ["go", "mux", "tmux"]
---

# ADR-040: tmux-only Multiplexer

## Context

v0 grew two multiplexer implementations (tmux, cmux) with a third
(zellij) reserved as a stub, plus discussion of Ghostty terminal as a
fourth. The owner directed v1 to support **tmux only**. The
`Multiplexer` interface is preserved as a single-impl abstraction; new
backends are out of scope until a concrete need appears.

## Decision

### Single interface, single impl

```go
// internal/mux/mux.go

type Session struct {
    Name     string
    Attached bool
}

type Pane struct {
    ID  string  // tmux pane id, e.g. "%5"
    CWD string
}

type Multiplexer interface {
    IsAvailable(ctx context.Context) bool
    InsideSession(ctx context.Context) (string, bool, error)

    CreateSession(ctx context.Context, name, cwd string) error
    KillSession(ctx context.Context, name string) error
    SessionExists(ctx context.Context, name string) (bool, error)
    Attach(ctx context.Context, name string) error
    SendKeys(ctx context.Context, session, pane, keys string) error

    SetEnv(ctx context.Context, session, key, value string) error
    GetEnv(ctx context.Context, session, key string) (string, error)
    SetOption(ctx context.Context, session, key, value string) error
    ListSessions(ctx context.Context) ([]Session, error)

    SplitVertical(ctx context.Context, session, cwd string) (paneID string, err error)
    KillPane(ctx context.Context, session, pane string) error
    ListPanes(ctx context.Context, session string) ([]Pane, error)
}
```

The single implementation lives at `internal/mux/tmux.go` as
`type Tmux struct{}`. It shells out to the `tmux` binary via
`exec.CommandContext`.

### Why no cmux/zellij/ghostty in v1

- **cmux** (v0 ADR-022): single-user adoption is uncertain; tmux is sufficient.
- **zellij**: never implemented in v0; no in-house experience.
- **ghostty**: a terminal, not a multiplexer; misclassified in v0.
- Each additional impl roughly doubles the multiplexer test surface for marginal value.

### tmux assumptions

- tmux 3.x or newer (any modern install).
- `@AF_SESSION` option marks tmux sessions as managed by `af`. Used by `ListSessions` and the v0-equivalent session-limit guard.
- Session names use `--` instead of `/.:` (per ADR-038 sanitization).

### What's not in the interface

- **`KillServer`**: too aggressive; `af suspend` handles per-workstream teardown (ADR-046).
- **Window management** (`new-window`, `select-window`): v1 uses panes only.
- **Hooks** (e.g. `pane-died`): would let us auto-detect agent crashes, but requires tmux config changes outside `af`'s control. Defer.

### Testing

Per ADR-051: `Multiplexer` interface enables a `FakeMultiplexer` for
unit tests, and the `testscript` integration tests run against the
built binary with that fake injected via env var. **No test in CI
ever touches a real tmux server**; that's reserved for the manual
smoke-test tier (also defined in ADR-051) where the owner exercises
the full flow on a workstation before merging risky PRs.

## Consequences

- One mux implementation to maintain.
- Tests don't need a tmux server.
- Adding zellij/cmux later is a new ADR + a new struct; the interface
  was deliberately kept generic enough.

## Alternatives Considered

- **Keep cmux as a second impl.** Rejected per scope cut (ADR-031).
- **Drop the interface and call tmux directly from `cmd/af/...`.**
  Rejected; the interface is the testing seam (ADR-051) and keeps
  command code provider-agnostic.

## References

- v0 ADR-002, v0 ADR-022 — superseded by this ADR for v1.
- ADR-031 — v1 master.
- ADR-039 — multi-agent slot model uses panes.
- ADR-046 — `af suspend` kills the workstream's tmux session.
- ADR-051 — testing strategy (interface seams).
