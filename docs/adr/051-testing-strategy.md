---
adr: 051
title: "Testing Strategy"
status: proposed
implementation: pending
date: 2026-05-06
last_modified: 2026-05-06
supersedes: []
superseded_by: null
related: ["031", "034", "050", "052"]
tags: ["go", "testing"]
---

# ADR-051: Testing Strategy

## Context

`af` v1 is a CLI that orchestrates `git`, `tmux`, `ssh`, `slicer`,
`sbx`, agent CLIs, and a keyring. None of those should run during unit
tests; the tests need **interface seams** for the IO surface and
**testscript** for the integration surface.

v0 ADR-018 introduced a `CommandRunner` trait, then v0 ADR-029 dropped
it in favour of feature gates + assert_cmd. v1 takes a third path
that's cleaner in Go: each `internal/<pkg>/` defines a narrow
interface for its IO needs; production code uses real impls; tests
use fakes.

## Decision

### Test layers

| Layer | Tool | Scope |
|---|---|---|
| Unit | stdlib `testing` | Pure logic in `internal/<pkg>/` |
| Property | stdlib `testing/quick` | Invariants over generated inputs (e.g. naming, sanitization, lifecycle transitions) |
| Integration (CLI) | `rogpeppe/go-internal/testscript` | `cmd/af/...` end-to-end against a built binary |
| Manual | (none) | Real tmux/ssh/sandbox interactions, run by the owner before merging risky PRs |

### Interface seams (replaces v0 CommandRunner)

Each `internal/<pkg>/` that does IO defines one or more **narrow**
interfaces (3–5 methods) capturing exactly its IO needs. Production
impls call out to real systems; test fakes implement the same
interface in memory.

```go
// internal/git/git.go (sketch)
type Git interface {
    WorktreeAdd(ctx context.Context, root, branch, base, target string) error
    WorktreeRemove(ctx context.Context, root, target string) error
    BranchExists(ctx context.Context, root, name string) (bool, error)
    CurrentBranch(ctx context.Context, root string) (string, error)
}

// production
type Exec struct{}
func (Exec) WorktreeAdd(ctx context.Context, root, branch, base, target string) error {
    return exec.CommandContext(ctx, "git", "-C", root, "worktree", "add", ...).Run()
}

// test fake
type Fake struct {
    Calls []string
    // ...
}
```

Same pattern for `internal/mux`, `internal/agent`, `internal/sandbox`,
`internal/remote`, `internal/secret`. Each provides a `Fake` (or
`Stub`) sibling type for tests.

### `testing/quick` for properties

Properties to verify:

- `Sanitize(s)` is idempotent (`Sanitize(Sanitize(s)) == Sanitize(s)`).
- `ApplyPrefix(name, prefix)` doesn't double-apply.
- Workstream lifecycle transitions form a valid DAG (no `completed → active`).
- UUID v5 derivation: same inputs → same output.

Each property test runs N=100 iterations with random inputs from
`testing/quick.Generate` defaults.

### `testscript` for CLI golden tests

`cmd/af/testdata/script/<scenario>.txt` files describe end-to-end
flows:

```
# create.txt
af setup
af create --agent pi mytask
af list
stdout 'mytask\s+active'
af suspend mytask
af list
stdout 'mytask\s+suspended'
af resume mytask
af done mytask --force
```

The framework runs each scenario in an isolated tempdir with mocked
external commands (configurable per scenario via `script` directives).

### What's NOT tested

| Concern | Why not |
|---|---|
| Real tmux server | Requires CI tmux; fragile across versions |
| Real ssh to a real host | Requires network + persistent infra |
| Real slicer/sbx VMs | Requires Firecracker/Docker daemons |
| Real Anthropic/OpenAI APIs | Requires keys + costs money |

These are exercised manually before risky PRs. The owner runs `af
create` against a real machine, observes correct behaviour, then
ships.

### Coverage target

`internal/<pkg>/` packages with **no IO** (e.g. `internal/naming`,
`internal/uuid`, `internal/config` excluding loaders): **80%+**.

`internal/<pkg>/` packages with IO use interface fakes; coverage on the
production impl is necessarily lower because the impl just shells out.

`cmd/af/` is exercised by testscript; line coverage there is whatever
the scenarios drive.

No coverage gate in CI for v1 (single-user; pragmatism wins). The
owner monitors `go test -cover` output during development.

### `go test` invocation

```bash
go test -race -count=1 -shuffle=on ./...
```

- `-race` catches concurrent-access bugs.
- `-count=1` disables the test cache (avoids stale state confusing investigation).
- `-shuffle=on` randomises test order (catches inter-test coupling).

### `make test`

```make
test:
	go test -race -count=1 -shuffle=on ./...

test-property:
	go test -run TestProperty -count=10000 -timeout 120s ./...
```

The property-only target runs more iterations than the default for
deeper exploration.

## Consequences

- Unit tests are deterministic and fast (no real systems touched).
- Integration tests are realistic (built binary against scripted scenarios).
- Coverage is high where it counts (pure logic) without fighting the IO surface.
- Adding a new package means adding its interface seam; subagents working on a package own its tests.

## Alternatives Considered

- **`CommandRunner` trait (v0)**. Rejected per v0 ADR-029's reasoning: pollutes call sites with a runtime-injected dep. Per-package narrow interfaces are cleaner.
- **`assert_cmd` equivalent in Go**. The `os/exec` + golden-output pattern in `testscript` covers this; no separate library needed.
- **Real tmux in CI** via `xvfb`/headless. Rejected; fragile and slow.
- **Coverage gate at 80%**. Rejected for v1; would block legitimate IO-bound code.

## References

- [`rogpeppe/go-internal/testscript`](https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript)
- [`testing/quick`](https://pkg.go.dev/testing/quick)
- v0 ADR-018, v0 ADR-029 — superseded for v1.
- ADR-031 — v1 master, dep set.
- ADR-034 — Go module idiom (interfaces declared where used).
- ADR-050 — lint config (test files have looser exclusions).
- ADR-052 — formal verification (extends property testing).
