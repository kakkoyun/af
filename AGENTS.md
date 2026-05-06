# Working Agreement — v1

Ground rules for every agent (human or AI) working on this project.
**Read this before writing any code.** See [`CLAUDE.md`](CLAUDE.md) for
the constitution and [`docs/CONVENTIONS.md`](docs/CONVENTIONS.md) for
file-ownership and subagent dispatch protocol.

> **v0 boundary.** The Rust source tree (`src/`, `Cargo.toml`, `justfile`,
> etc.) is reference material only. Do not modify it. v1 implementation
> goes under `cmd/af/` and `internal/...`.

---

## Core Principles

1. **TDD — Test first, always.** Write the test, watch it fail, implement,
   watch it pass. No exceptions. If you can't write a test for it, you
   don't understand the requirement well enough.

2. **No corners cut.** Every exported identifier gets a doc comment. Every
   error path gets a test. Every public API gets an example in the doc
   comment when non-obvious. `golangci-lint` pedantic is on for a reason.

3. **Small, reviewable commits.** One logical change per commit. If a
   commit message needs "and" more than once, split it.

4. **Progress is tracked, not assumed.** Update `PROGRESS.md` after
   completing any task. Update `TODO.md` checkboxes. Future sessions
   start by reading these files.

5. **Stdlib first.** Reach for Go's standard library before any
   third-party package. New deps require an ADR justification.

---

## TDD Workflow

```
1. Pick a task from TODO.md (in stage order).
2. Write the test(s) that define the expected behaviour.
3. Run `go test ./...` — confirm the test FAILS (red).
4. Write the minimum implementation to pass the test.
5. Run `go test ./...` — confirm it PASSES (green).
6. Refactor if needed (keep tests green).
7. Run `make check` (or fmt-check + lint + race test) — must all pass.
8. Commit with a descriptive message.
9. Update PROGRESS.md and check off TODO.md.
```

**Never skip step 3.** A test that passes before implementation is not
testing anything.

---

## Code Quality Standards

### Go

- **Every exported identifier** has a `// Foo does ...` doc comment whose first word is the identifier name (per `revive`/`golint` convention).
- **Every package** has a `// Package <name> does ...` doc comment in one file (typically `doc.go` or the same-named `.go` file).
- **Errors** wrap with `fmt.Errorf("context: %w", err)`. Sentinel errors are package-level `var Err... = errors.New(...)`. Custom error types embed via `errors.Is`/`errors.As`.
- **No `panic()`** outside `cmd/af/main.go` and genuinely unreachable code paths. Use `errors.New` or typed errors.
- **No `fmt.Print*` / `fmt.Println` / `os.Stdout.Write` outside `cmd/af/main.go`** — use `slog` for diagnostics, return values for output.
- **`golangci-lint`** with all linters enabled. Mandatory: `errcheck`, `staticcheck`, `unparam`, `revive`, `gocritic`, `gosec`, `nolintlint`. Exceptions justified inline (`//nolint:name // reason`).
- **Format**: `gofumpt -w .` (stricter `gofmt`) + `goimports -w .` (import order). CI checks `-l` (zero output).

### Tests

- **Unit tests** live in the same package as the code, in `<file>_test.go`.
- **Black-box tests** for exported API live in `<package>_test.go` (declared `package foo_test`).
- **Integration tests** for the binary live under `cmd/af/` as `<command>_test.go` using `rogpeppe/go-internal/testscript`.
- **Test names** follow `TestThing_DoesX_WhenY`. e.g., `TestSanitize_ReplacesSlashWithDoubleDash`.
- **Edge cases are mandatory:** empty input, Unicode, path separators, error paths, context cancellation.
- **No test depends on external state** — no real tmux, no real git remote, no network. Mock via interface seams (per-package, narrow).

### Commits

- Format: `<type>(<scope>): <description>`
- Types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`, `perf`, `ci`, `build`, `style`.
- Scopes: package names (`agent`, `config`, `mux`, `git`, ...) or `v0`/`v1` for doc-pass commits.
- Examples:
  - `feat(session): record schema_version=1 on save`
  - `test(workstream): cover sub-worktree path collision`
  - `refactor(config): extract layered merge into dedicated function`

---

## File Responsibilities

| File | Purpose | Updated when |
|---|---|---|
| `TODO.md` | Checkbox task list by stage + backlog | Task started or completed |
| `PROGRESS.md` | Narrative log per session | After each work session |
| `docs/SPEC.md` | v1 specification (immutable reference) | Never (create ADR) |
| `docs/PLAN.md` | Lightweight pointer to ADR groups (immutable) | Never (create ADR) |
| `docs/CONVENTIONS.md` | Go style + file-ownership manifest | Append, don't overwrite |
| `docs/adr/NNN-*.md` | Architecture decisions (append-only from 031) | New ADRs only |
| `CLAUDE.md` | Constitution | When non-negotiable rules change |
| `AGENTS.md` | This file — working agreement | When process changes |
| `docs/v0/**` | Frozen Rust-era archive | **NEVER** |

---

## Documentation Standards

Documentation is not an afterthought — it ships with the code.

### README.md is the contract

Every command example in the README must work, or be clearly marked as
`🔜 Planned`. If the implementation doesn't match the README, the
implementation is wrong.

**Update README.md when:**
- A new command is implemented (add usage example).
- A command's flags change.
- A new agent or provider is added.
- Installation instructions change.

### Godoc

- `go doc ./...` must produce useful output for every exported identifier.
- Every exported type, function, and method has a `// Foo ...` comment.
- Doc comments include **examples** where the API is non-obvious. Use
  `Example*` test functions for executable examples that `go doc` renders.
- Package docs (`// Package foo ...`) explain the package's role in the
  architecture and link to the relevant ADR.

### ADRs

- Append-only from ADR-031. Numbering never reused.
- Frontmatter format defined in ADR-032 (status + implementation lifecycle).
- Max 3 pages of body. Code listings: type signatures only.
- Every ADR that supersedes another lists it in `supersedes` frontmatter and backreferences it in `## References`.

### Doc update rule

Every commit that changes user-facing behaviour must update:

1. `README.md` if the command surface changes.
2. Godoc if the public API changes.
3. The relevant ADR's `implementation` frontmatter (`pending → in-progress → complete`).
4. `CHANGELOG.md` `[Unreleased]` block.

---

## Subagent Coordination

**Default: do the work yourself.** Only spawn subagents when there are
genuinely independent, non-overlapping packages that benefit from
parallelism. Two tasks? Do them sequentially.

When spawning subagents:

1. **Commit all pending work first.** Subagents can overwrite uncommitted files.
2. **Each subagent works on a branch**, not directly on `main`. The lead reviews and merges.
3. **Each subagent gets a clear, scoped task** — one package, one interface, one set of tests.
4. **No subagent modifies shared files** (`go.mod`, `cmd/af/main.go`, `internal/.../doc.go` parents) without coordination. Only the lead modifies shared files.
5. **Subagents write their package + tests**, then report back.
6. **Integration is the lead's job** — wiring packages together, fixing lint violations, resolving conflicts.
7. **The lead runs `make check` after integration** — subagent code is not trusted to be lint-clean until verified in the full module context.

The full file-ownership manifest lives in `docs/CONVENTIONS.md`.

---

## Definition of Done

A task is "done" when:

- [ ] Tests exist and pass (`go test -race -count=1 ./...`)
- [ ] `golangci-lint run` is clean (zero warnings)
- [ ] `gofumpt -l . && goimports -l .` is clean (zero output)
- [ ] Doc comments are complete for all exported identifiers
- [ ] `go doc ./...` builds without warnings
- [ ] README.md is updated if user-facing behaviour changed
- [ ] CHANGELOG.md `[Unreleased]` block updated
- [ ] PROGRESS.md is updated
- [ ] TODO.md checkbox is checked
- [ ] Relevant ADR's `implementation` frontmatter advanced
- [ ] Code is committed with a proper commit message
