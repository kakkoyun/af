# Conventions

Structural rules for working on this codebase. Read before writing any code or
spawning a subagent. The definitive source for the rules stated here is
`AGENTS.md` (working agreement); this file is the quick-reference form.

---

## File-Ownership Manifest

The following files are **shared** — owned exclusively by the lead agent during
integration (Phase IV of any sprint). No subagent session writes to them during
lane work. If a lane determines it needs one of these files, it stops and surfaces
the need to the lead.

| File | Why shared |
|---|---|
| `Cargo.toml` | Features, deps, lint config affect the whole crate |
| `src/cli.rs` | All subcommands and flags are defined here |
| `src/lib.rs` | Module graph; adding a module requires an entry here |
| `src/provider/mod.rs` | Provider traits and factory dispatch |
| `src/cmd/mod.rs` | Command dispatch table |
| `README.md` | User-facing contract |
| `CHANGELOG.md` | Release notes (see ADR-021) |
| `TODO.md` | Task checklist |
| `PROGRESS.md` | Session narrative log |
| `docs/adr/README.md` | ADR index |

**Rationale:** codified in ADR-015. Motivated by the Session 2 ledger.rs overwrite
incident where a subagent replaced a lead-authored file.

---

## Module-to-Directory Ownership

| Directory | Concern | Active lanes |
|---|---|---|
| `src/agent/` | Agent provider implementations | Lead-owned; extend with new provider files |
| `src/provider/` | Remote + sandbox provider implementations | Lane A1 (workspaces), Lane B1 (slicer remote) |
| `src/cmd/` | Subcommand implementations | Lane A2 (list), Lane B2 (auth), Lane B3 (resume), Lane B5 (editor) |
| `src/auth/` | Keyring wrapper (new module) | Lane B2 |
| `src/session/` | Session types, store, ledger | Stable; modify only for lifecycle changes |
| `src/git/` | Git helpers | Stable; add helpers for new commands as needed |
| `src/mux/` | Multiplexer trait + tmux | Stable until Zellij lane opens |
| `src/config/` | Config load + merge | Stable |
| `src/platform/` | OS + package manager | Stable |
| `src/provision/` | SSH bootstrap pipeline | Lane B1 adds slicer install step |
| `src/obsidian/` | Obsidian note integration | Stable |
| `src/util/` | UUID, notifications, shared utils | Stable |
| `book/` | mdBook user guide | Lane C1 owns entirely |
| `scripts/` | Shell helpers (book-gen, etc.) | Lane C1 |
| `tests/` | Integration tests | Each lane adds its own test file |
| `docs/adr/` | Architecture decisions | Each lane owns the ADR(s) it writes |

---

## Commit Format

```
<type>(<scope>): <what changed>

<optional body: WHY, not what>
```

Types: `feat`, `fix`, `test`, `refactor`, `docs`, `chore`, `ci`, `perf`, `build`.
Scope is required when the change targets a specific module or component.

**Rule:** if the message needs "and" more than once, split the commit.

---

## TDD Workflow (9 steps, from AGENTS.md)

1. Pick a task from `TODO.md`.
2. Write the test(s) defining expected behaviour.
3. Run tests — **confirm RED**. Never skip this step.
4. Write minimum implementation to pass.
5. Run tests — confirm GREEN.
6. Refactor (keep tests green).
7. `cargo fmt --check && cargo clippy --all-targets --all-features -- -D warnings && cargo test --all-features`
8. Commit.
9. Update `PROGRESS.md` and check off `TODO.md`.

---

## Definition of Done (every task)

- [ ] Tests exist and pass
- [ ] Clippy clean (`-D warnings`)
- [ ] Formatting clean (`cargo fmt --check`)
- [ ] Doc comments on all public items
- [ ] `cargo doc --no-deps` builds without warnings
- [ ] `README.md` updated if user-facing behaviour changed
- [ ] `book/src/commands/<cmd>.md` updated if command changed (new this sprint)
- [ ] `PROGRESS.md` entry written
- [ ] `TODO.md` checkbox checked
- [ ] Commit with proper format

---

## ADR-First Rule (P6)

No implementation lane starts until its governing ADR is accepted. ADRs encode the
"why" — without them, the code has no explanation and future sessions cannot
reconstruct the intent. See `docs/adr/` for the format.

---

## Subagent Dispatch Protocol

See ADR-015 for the full protocol. Quick reference:

Every subagent prompt must state:
- Branch name (`lane-<id>-<short>`)
- Worktree path (`../af-lane-<id>`)
- Owns (explicit absolute paths — relative to the worktree root)
- Does-not-touch (the shared-file table above)
- Referenced ADRs
- TDD + commit format
- Handback: push branch, open draft PR, **stop — do not merge**

---

## Worktree Protocol for Parallel Lanes

Each implementation lane runs in its own git worktree. This isolates file edits,
cargo target directories, and language server state between concurrent agents.

### Setup (lead, before dispatching a lane)

```bash
# Create worktree + branch in one command
git worktree add ../af-lane-a1 -b lane-a1-workspaces
git worktree add ../af-lane-b2 -b lane-b2-auth
# etc.
```

The worktree root is one level above the project root: `../af-lane-<id>`.
All absolute paths in the subagent prompt use this root.

### Subagent working directory

The subagent's working directory is the worktree root. All `cargo` commands,
file reads, and edits run there. The subagent must **not** `cd` to the main
worktree or touch any path outside its worktree directory.

### Review + integration (lead)

```bash
# See what the lane changed, relative to main
git diff main..lane-a1-workspaces

# After review, merge
git merge --no-ff lane-a1-workspaces -m "feat(provider): integrate Lane A1 workspaces"

# Cleanup
git worktree remove ../af-lane-a1
git branch -d lane-a1-workspaces
```

### Naming convention

| Lane | Branch | Worktree path |
|---|---|---|
| A1 | `lane-a1-workspaces` | `../af-lane-a1` |
| A2 | `lane-a2-orphan` | `../af-lane-a2` |
| B1 | `lane-b1-slicer-remote` | `../af-lane-b1` |
| B2 | `lane-b2-auth` | `../af-lane-b2` |
| B3 | `lane-b3-resume` | `../af-lane-b3` |
| B4 | `lane-b4-exedev-liveness` | `../af-lane-b4` |
| B5 | `lane-b5-editor-remote` | `../af-lane-b5` |
| C1 | `lane-c1-book` | `../af-lane-c1` |

### Why worktrees (not just branches)

- Each lane gets its own `target/` — no cargo lock contention between concurrent builds.
- Language servers (rust-analyzer) don't fight over the same workspace root.
- File edits are isolated: a subagent cannot accidentally touch a file it was told not to.
- `git diff main..<branch>` gives a clean review scope without switching branches.
