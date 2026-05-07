# af — v1 Conventions

> Go conventions, commit format, and file-ownership manifest for v1.
> Append-only; do not overwrite existing sections. The Rust-era
> conventions are archived at [`v0/CONVENTIONS.md`](v0/CONVENTIONS.md).

---

## Go conventions

### Module layout

- Module path: `github.com/kakkoyun/af` (set in `go.mod` once scaffold lands).
- Entry point: `cmd/af/main.go` — thin: arg parsing, slog init, dispatch via cobra.
- Library code: `internal/<package>/`. No `pkg/` directory until external consumers exist (none planned for v1).
- Examples: `examples/` for non-Go artefacts (`obsidian/active-workstreams.base`, etc.).
- Tests: same package as code (`<file>_test.go`); black-box tests in `<package>_test.go` (`package foo_test`); integration tests under `cmd/af/` using `testscript`.

### Naming

- Packages: short, lowercase, no underscores. `agent`, `config`, `mux`, `git`, `session`, `workstream`, `secret`.
- Exported identifiers: doc comment starts with the identifier name.
- Errors: package-level sentinels (`var ErrNotFound = errors.New("session not found")`); typed errors implement `Is`/`As` for nuance.
- Interfaces: small (3–5 methods); declared in the package that **uses** them, not the package that implements them. Exception: `internal/agent.Agent`, `internal/mux.Multiplexer`, `internal/sandbox.Sandbox` — these are central interfaces, declared in their own package by design.

### Errors

- Wrap with `fmt.Errorf("operation: %w", err)`.
- Sentinel errors are package-level `var`. Group at the top of the file.
- Use `errors.Is`/`errors.As` for inspection. Never `err.Error() == "..."`.
- Do not call `panic()` outside `cmd/af/main.go` and unreachable arms (e.g., switch defaults that should never trigger; document why).

### Logging

- `log/slog` from stdlib. No third-party logger.
- Default handler: `slog.NewTextHandler(os.Stderr, ...)` configured in `main`.
- Structured fields: `slog.Info("msg", "key", value, ...)`.
- Sensitive fields are redacted via a custom `slog.Handler` per ADR-049.
- No `fmt.Print*` or `os.Stdout.Write` outside `cmd/af/main.go` (where command output goes to stdout) — diagnostic logs go to slog.

### Context

- Every function that calls an external command, opens a file, or makes a network call takes `context.Context` as its first parameter.
- Cancellation is propagated. Long-running loops check `ctx.Done()`.
- `context.Background()` only at the top of `main`. Use `signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)` to wire Ctrl-C.

### Concurrency

- Goroutines must have a clear lifetime. Use `errgroup.Group` (stdlib `golang.org/x/sync/errgroup` allowed as the one exception to no-third-party-logger if needed) or hand-rolled `sync.WaitGroup`.
- Channels are owned by their sender; the sender closes.
- Mutex use is documented (`// safeMap protects entries against ...`).
- `go vet -race` runs in every CI build.

### Tests

- Unit tests for pure logic: 80%+ coverage on `internal/...` packages without IO.
- IO-heavy packages (`mux`, `git`, `sandbox`, `remote`) use interface seams; unit tests run against fakes.
- Integration tests (`cmd/af/...`) use `rogpeppe/go-internal/testscript` against a built binary.
- Property tests via stdlib `testing/quick` for naming, sanitization, and lifecycle invariants.
- No real `tmux`/`git`/`ssh` calls in unit tests.

---

## Commit format

```
<type>(<scope>): <description>
```

| Type       | Use for                                                 |
| ---------- | ------------------------------------------------------- |
| `feat`     | New user-facing functionality                           |
| `fix`      | Bug fix                                                 |
| `docs`     | Documentation only                                      |
| `refactor` | Code change that neither fixes a bug nor adds a feature |
| `test`     | Adding or fixing tests                                  |
| `chore`    | Tooling, build, dotfiles                                |
| `perf`     | Performance improvement                                 |
| `ci`       | CI/build workflow change                                |
| `build`    | Build system change (Makefile, goreleaser)              |
| `style`    | Formatting, whitespace                                  |

| Scope        | Use for                                                                                            |
| ------------ | -------------------------------------------------------------------------------------------------- |
| `agent`      | `internal/agent/...`                                                                               |
| `config`     | `internal/config/...`                                                                              |
| `git`        | `internal/git/...`                                                                                 |
| `mux`        | `internal/mux/...`                                                                                 |
| `obsidian`   | `internal/obsidian/...`                                                                            |
| `remote`     | `internal/remote/...`                                                                              |
| `sandbox`    | `internal/sandbox/...`                                                                             |
| `secret`     | `internal/secret/...`                                                                              |
| `session`    | `internal/session/...`                                                                             |
| `workstream` | `internal/workstream/...`                                                                          |
| `cmd`        | `cmd/af/...`                                                                                       |
| `adr`        | `docs/adr/...`                                                                                     |
| `v0`         | Anything under `docs/v0/` (rare; archive should be frozen)                                         |
| `v1`         | Top-level v1 doc files: README, CHANGELOG, PROGRESS, TODO, CLAUDE, AGENTS, SPEC, PLAN, CONVENTIONS |

Body explains **why**, not what. Keep under 72 chars per line. Reference
the relevant ADR(s) when the commit implements a decision.

Examples:

- `feat(workstream): create sub-worktree on agent add`
- `fix(session): preserve schema_version when archiving`
- `test(naming): cover Unicode separators in sanitize`
- `docs(adr): ADR-038 workstream + worktree layout`

---

## File-ownership manifest

Used to coordinate parallel work and prevent overwrites when subagents
are spawned. Every file in the repo has exactly one **primary owner**.

### Top-level

| Path                                                               | Primary owner                                     |
| ------------------------------------------------------------------ | ------------------------------------------------- |
| `README.md`                                                        | Lead (any agent updating after a feature change). |
| `CHANGELOG.md`                                                     | Lead (every feature commit appends here).         |
| `PROGRESS.md`                                                      | Lead (one append per session).                    |
| `TODO.md`                                                          | Lead. Subagents do not edit.                      |
| `CLAUDE.md`, `AGENTS.md`, `docs/CONVENTIONS.md`                    | Lead only.                                        |
| `docs/SPEC.md`, `docs/PLAN.md`                                     | Editable during planning to stay consistent with ADRs; **immutable after freeze**. No owner post-freeze; new ADR required. |
| `docs/adr/NNN-*.md`                                                | The author of that ADR. Reviewed by the owner.    |
| `docs/v0/**`                                                       | **Frozen.** No owner.                             |
| `Makefile`, `.golangci.yml`, `.goreleaser.yml`, `go.mod`, `go.sum` | Lead only.                                        |
| `cmd/af/main.go`                                                   | Lead only.                                        |

### Per-package

Each `internal/<pkg>/` package has one primary owner per task. Subagents
working on different packages may run in parallel; subagents working on
the same package are serialized.

The package interfaces (`internal/agent/agent.go`, `internal/mux/mux.go`,
`internal/sandbox/sandbox.go`) are owned by the lead. Implementations
(`internal/agent/pi.go`, `internal/mux/tmux.go`, etc.) may be delegated.

### v0 reference tree

`src/`, `tests/`, `Cargo.toml`, `Cargo.lock`, `clippy.toml`, `deny.toml`,
`rust-toolchain.toml`, `rustfmt.toml`, `.cargo/`, `target/`, `justfile`
are **read-only**. No agent — lead or subagent — may modify them. They
will be removed in a single commit once Go has functional parity.

---

## Subagent dispatch protocol

Mostly inherited from v0 (proven during Phase II.5 of the Rust
implementation). Re-stated here in Go-flavoured form.

1. **Lead commits all pending work** before dispatching subagents.
2. **Each subagent gets a branch**, never `main`. Branch naming: `lane-<topic>` (e.g., `lane-config`, `lane-mux-tmux`).
3. **Each subagent's task is single-package-scoped**. The dispatch prompt names exactly one `internal/<pkg>/` directory the subagent may modify, and lists shared files (in §file-ownership-manifest) the subagent must not touch.
4. **Subagent writes package + tests + package-doc**, then reports back with the diff and `go test ./internal/<pkg>/...` output.
5. **Lead integrates** the subagent's branch via merge or fast-forward, then runs `make check` to verify lint-clean in full module context.
6. **No two subagents touch the same package** at the same time.
7. **No subagent modifies** `cmd/af/main.go`, `go.mod`, `Makefile`, `docs/CONVENTIONS.md`, `docs/SPEC.md`, `docs/PLAN.md`, or any `docs/v0/**` file.

The dispatch prompt template lives at `docs/CONVENTIONS.md#dispatch-prompt-template`
once a real subagent dispatch is needed. For now the rules above are
sufficient.

---

## References

- [`CLAUDE.md`](../CLAUDE.md) — constitution, build commands.
- [`AGENTS.md`](../AGENTS.md) — working agreement (TDD workflow, definition of done).
- [`docs/SPEC.md`](SPEC.md) — what we're building.
- [`docs/PLAN.md`](PLAN.md) — sequencing.
- [`docs/adr/`](adr/) — design decisions.
- [`docs/v0/CONVENTIONS.md`](v0/CONVENTIONS.md) — Rust-era conventions, archived.
