---
adr: 034
title: "Go Module Layout & Idiom"
status: proposed
implementation: complete
date: 2026-05-06
last_modified: 2026-05-09
supersedes: []
superseded_by: null
related: ["031", "035", "050", "051"]
tags: ["go", "layout", "idiom"]
---

# ADR-034: Go Module Layout & Idiom

## Context

v1 starts a new Go module from scratch. Decisions about layout, error
wrapping style, logging, context propagation, and `init()` hygiene need
to be locked early because every subsequent ADR (and every package
implementation) builds on them.

## Decision

### Module path

```
github.com/kakkoyun/af
```

Set in `go.mod`. Toolchain pinned to the latest stable Go (currently
`go 1.24` or whatever is current at scaffold time; pin via `go.mod`
`toolchain` directive to ensure reproducibility).

### Directory layout

```
af/
├── cmd/
│   └── af/
│       └── main.go              # thin entry point
├── internal/
│   ├── agent/                   # Agent interface + claude/pi/codex impls (ADR-043)
│   ├── config/                  # TOML loader, layered (ADR-036)
│   ├── git/                     # worktree, branch, remote, PR helpers
│   ├── mux/                     # Multiplexer interface + tmux impl (ADR-040)
│   ├── obsidian/                # notes + Bases (ADR-047)
│   ├── remote/                  # SSH host model (ADR-041)
│   ├── sandbox/                 # slicer + sbx (ADR-042)
│   ├── secret/                  # keyring + ephemeral envelope (ADR-049)
│   ├── session/                 # state.toml + ledger.jsonl (ADR-037)
│   └── workstream/              # worktree layout, sub-worktrees (ADR-038)
├── examples/
│   └── obsidian/                # example .base file, etc. (ADR-047)
├── docs/                        # SPEC, PLAN, CONVENTIONS, adr/, v0/
├── Makefile                     # ADR-053
├── .golangci.yml                # ADR-050
├── .goreleaser.yml              # ADR-053
├── go.mod, go.sum
└── README.md, CHANGELOG.md, ...
```

**No `pkg/` directory.** External Go consumers of `af` are not a goal
in v1; everything goes under `internal/` for the explicit
"unimportable" guarantee. If a v1.x ever needs to expose a library API,
that's a future ADR.

### Error idiom

- **Wrap with context**: `fmt.Errorf("op X: %w", err)`. Always include the operation name; never just `return err`.
- **Sentinel errors**: package-level `var Err... = errors.New("...")`. Group at the top of the file.
- **Typed errors**: when callers need structured detail, define a struct with `Error()` and (optionally) `Is()` / `As()` methods.
- **No `panic` outside `cmd/af/main.go`** — tests get `t.Fatal`, library code returns errors. The single legitimate panic site is `main`'s top-level recover-and-log to surface unhandled bugs.

### Logging

- **Stdlib `log/slog` only.** No third-party logger.
- **Configured in `main`**: `slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: ...})`, level controlled by `--verbose` flag and/or `AF_LOG_LEVEL` env.
- **Diagnostic logs go to stderr.** Stdout is reserved for command output (machine-readable when `--json`, human-readable otherwise).
- **No `fmt.Print*` outside `cmd/af/main.go`.** Library code returns errors and structured values; the main package decides how to format them.

### Context propagation

```go
// Every IO-bound function takes ctx as its first parameter.
func (s *Store) Save(ctx context.Context, state *SessionState) error
func (m *Tmux) CreateSession(ctx context.Context, name string, cwd string) error
func (a *Pi) Launch(ctx context.Context, opts LaunchOpts) (*exec.Cmd, error)
```

- `context.Background()` exists at exactly one site: the top of
  `main()`, immediately wrapped by
  `signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)` for
  graceful Ctrl-C handling.
- All `os/exec` calls use `exec.CommandContext`, never `exec.Command`.
- Tests pass `t.Context()` (Go 1.24+) or `context.Background()`.

### `init()` discipline

- **No `init()` functions** in `internal/...` packages. Wiring lives in `main` or in explicit `New...` constructors.
- The single exception is `cmd/af/main.go` if registering cobra subcommands needs init-time work (preferable: register in `main` directly).

### Concurrency

- Goroutines have a clear lifetime, owned by a `sync.WaitGroup` or `errgroup.Group`.
- `golang.org/x/sync/errgroup` is allowed as a transitive-of-stdlib dep if needed (it's a quasi-stdlib package). Adding it requires noting in CHANGELOG.
- Channels are owned by their sender; the sender closes.
- All concurrent code runs under `go test -race`.

### Cobra integration

Per ADR-035: each subcommand is a function returning `*cobra.Command`,
called from `main` to register on the root. No subcommand registers
itself via `init()`.

```go
// cmd/af/main.go (sketch)
func main() {
    ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer cancel()

    root := newRootCmd(ctx)
    root.AddCommand(
        newCreateCmd(),
        newDoneCmd(),
        newListCmd(),
        // ...
    )
    if err := root.ExecuteContext(ctx); err != nil {
        slog.Error("af", "err", err)
        os.Exit(1)
    }
}
```

## Consequences

- New contributors can find any package by following the `internal/<concept>/` mapping.
- Errors carry call-site context for free; `errors.Is`/`As` works everywhere.
- `slog` provides structured logging with no dep cost.
- `context` plumbing is uniform — every external call is cancellable.
- Tests are deterministic without real tmux/git/ssh because every external call goes through an interface seam (per ADR-051).

## Alternatives Considered

- **`pkg/` for "would-be public" code, `internal/` for guarded.** Rejected: no external consumers; one rule is clearer.
- **`zerolog` / `zap` for logging.** Rejected: stdlib `slog` is sufficient and dep-free.
- **Functional-style error returns with custom Result type.** Rejected: idiomatic Go is `(T, error)`; deviating costs more than it saves.
- **Global logger.** Rejected: `slog` already provides one; we use the package functions (`slog.Info`, etc.) and avoid threading a logger through every signature.

## References

- [Effective Go](https://go.dev/doc/effective_go)
- [Standard project layout](https://github.com/golang-standards/project-layout) — we deliberately don't follow it; `cmd/` and `internal/` are sufficient.
- ADR-031 — v1 master, dep set.
- ADR-035 — CLI framework.
- ADR-050 — lint config.
- ADR-051 — testing strategy (interface seams).
