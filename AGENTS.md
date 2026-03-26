# Working Agreement

Ground rules for all agents (human and AI) working on this project.
**Read this before writing any code.**

---

## Core Principles

1. **TDD — Test first, always.** Write the test, watch it fail, implement, watch it pass.
   No exceptions. No "I'll add tests later." If you can't write a test for it, you don't
   understand the requirement well enough.

2. **No corners cut.** Every function gets a doc comment. Every error path gets a test.
   Every public API gets an example in the doc comment. Clippy pedantic is on for a reason.

3. **Small, reviewable commits.** One logical change per commit. If a commit message needs
   "and" more than once, split it.

4. **Progress is tracked, not assumed.** Update `PROGRESS.md` after completing any task.
   Update `TODO.md` checkboxes. Future sessions start by reading these files.

---

## TDD Workflow

```
1. Pick a task from TODO.md (Phase 0 first, then Phase 1, etc.)
2. Write the test(s) that define the expected behaviour
3. Run `just test` — confirm the test FAILS (red)
4. Write the minimum implementation to pass the test
5. Run `just test` — confirm it PASSES (green)
6. Refactor if needed (keep tests green)
7. Run `just check` — fmt + clippy + test + deny must all pass
8. Commit with a descriptive message
9. Update PROGRESS.md and check off TODO.md
```

**Never skip step 3.** A test that passes before implementation is not testing anything.

---

## Code Quality Standards

### Rust

- **Every public item** has a `///` doc comment explaining what, why, and edge cases.
- **Every module** has a `//!` module-level doc comment.
- **Error types** use `thiserror` with human-readable messages.
- **No `unwrap()`** in library code. Use `?` or `expect("reason")` with a clear justification.
- **No `todo!()`** in committed code. Use `unimplemented!("reason: tracking issue #X")` if
  truly needed, with a tracking reference.
- **Clippy pedantic** warnings are errors in CI. Fix them, don't suppress them (unless
  explicitly justified with a comment citing the lint name and reason).

### Tests

- **Unit tests** live in the same file as the code, in a `#[cfg(test)] mod tests {}` block.
- **Integration tests** live in `tests/` and test the binary or public API boundaries.
- **Test names** describe the scenario: `test_sanitize_replaces_slash_with_double_dash`,
  not `test_sanitize` or `test1`.
- **Edge cases are mandatory:** empty input, Unicode, path separators, error paths.
- **No test depends on external state** (no real tmux, no real git remote, no network).
  Mock via traits.

### Commits

- Format: `<scope>: <what changed>`
- Scopes: `feat`, `fix`, `test`, `refactor`, `docs`, `chore`
- Examples:
  - `feat(session): add session name sanitization`
  - `test(git): add main branch detection tests for all variants`
  - `refactor(config): extract TOML merging into dedicated function`

---

## File Responsibilities

| File | Purpose | Updated when |
|---|---|---|
| `TODO.md` | Checkbox task list by phase | Task started or completed |
| `PROGRESS.md` | Narrative log of what was done, decisions made, blockers | After each work session |
| `docs/PLAN.md` | Original plan (immutable reference) | Never (create ADR for changes) |
| `docs/SPEC.md` | Specification (immutable reference) | Only via ADR for spec changes |
| `docs/adr/*.md` | Architecture decisions | When a design decision is made |
| `CLAUDE.md` | Agent context and build instructions | When build/test commands change |
| `AGENTS.md` | This file — working agreement | When process changes |

---

## Documentation Standards

Documentation is not an afterthought — it ships with the code.

### README.md is the contract

`README.md` shows the **target user experience**. Every command example in the README must
work (or be clearly marked as `🔜 Planned`). If the implementation doesn't match the README,
the implementation is wrong, not the README.

**Update README.md when:**

- A new command is implemented (add usage example)
- A command's flags change
- A new agent or provider is added
- Installation instructions change

### Rustdoc

- `cargo doc` must build without warnings (`RUSTDOCFLAGS="-D warnings"`).
- Every public type, function, trait, and module has a `///` doc comment.
- Doc comments include **examples** where the API is non-obvious.
- `//!` module-level docs explain the module's role in the architecture.

### GitHub Pages (docs site)

The project deploys documentation to GitHub Pages via a CI workflow:

- **Rustdoc API reference** — auto-generated from source
- **User guide** — `docs/guide/` (mdBook or similar, set up when content warrants it)
- **ADRs** — linked from the guide

The docs site is the canonical URL for non-developers. README links to it.

### Doc update rule

Every PR that changes user-facing behaviour must update:

1. `README.md` — if the command surface changes
2. Rustdoc — if the public API changes
3. `docs/SPEC.md` — only via ADR (immutable reference)
4. Relevant ADR — if a design decision changes

### Changelog

We'll maintain `CHANGELOG.md` (Keep a Changelog format) starting from the first release.
Each entry corresponds to a version tag. Unreleased changes accumulate under `## [Unreleased]`.

---

## Subagent Coordination

**Default: do the work yourself.** Only spawn subagents when there are genuinely independent,
non-overlapping modules that benefit from parallelism. Two tasks? Do them sequentially.

When spawning subagents:

1. **Commit all pending work first.** Subagents can overwrite uncommitted files.
   This is not theoretical — it happened in Phase 0 (ledger.rs was overwritten).
2. **Each subagent works on a branch**, not directly on `main`. The lead reviews
   and merges. This prevents file conflicts and enforces review.
3. **Each subagent gets a clear, scoped task** — one module, one trait, one set of tests.
4. **No subagent modifies shared files** (`Cargo.toml`, `cli.rs`, `lib.rs`, `mod.rs` parent
   declarations) without coordination. Only the lead agent modifies shared files.
5. **Subagents write their module + tests**, then report back.
6. **Integration is the lead agent's job** — wiring modules together, fixing lint
   violations, resolving conflicts.
7. **The lead runs `just check` after integration** — subagent code is not trusted to
   be lint-clean until verified in the full crate context.

---

## Definition of Done

A task is "done" when:

- [ ] Tests exist and pass (`just test`)
- [ ] Clippy is clean (`just lint`)
- [ ] Formatting is clean (`just fmt-check`)
- [ ] Doc comments are complete for all public items
- [ ] `cargo doc` builds without warnings
- [ ] README.md is updated if user-facing behaviour changed
- [ ] PROGRESS.md is updated
- [ ] TODO.md checkbox is checked
- [ ] Code is committed with a proper commit message
