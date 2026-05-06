# af

**af** (agentic-flow / automatic-flow / as-fuck) — a single-user CLI for
stitching together the AI coding agents, terminal multiplexer, sandbox,
and remote machines that I actually use.

> **Status — v1 doc pass.** This repository is mid-rewrite. Source is
> Rust (v0, in `src/`); v1 is being written in Go and currently exists
> only as documentation under `docs/`. The Rust tree is **reference
> material only** until v1 has functional parity. See
> [`docs/v0/README.md`](docs/v0/README.md) for the v0 archive.

## What it does

I work on a repo. I want pi (or claude, or codex) to focus on one task
without touching my main checkout. I want the worktree, the branch, and
the agent session tied together. When I'm done, I want everything cleaned
up. When I want to step away, I want to suspend the workstream — tear down
the VM, kill the tmux server processes, free resources — and pick it back
up later as if nothing happened.

`af` does that.

## v1 goals (not yet implemented)

- **Single binary**, written in Go, cross-compiled for `linux/{amd64,arm64}` and `darwin/{amd64,arm64}`.
- **Stdlib-first** dependency policy. Five runtime deps total: cobra, BurntSushi/toml, google/uuid, gopkg.in/yaml.v3, zalando/go-keyring.
- **Pedantic** lint (all `golangci-lint` linters on).
- **Atomic commits**.
- **No release** — single-user; install via `go install` or `make install`.

## v1 scope (planned)

| Capability | Detail |
|---|---|
| Multiplexer | tmux only |
| Agents | pi (default), claude, codex |
| Remote | SSH host (alias from `~/.ssh/config`, `user@host`, or IP); no provider plugin layer |
| Sandbox | slicer (Firecracker) and sbx (Docker AI Sandboxes) |
| Worktree layout | Stable `~/Workspace/.worktrees/<repo>/<branch>/`; sibling sub-worktrees for subagents |
| State | TOML state file + JSONL ledger per workstream, global at `~/.local/share/af/v1/sessions/`; per-repo discovery symlink at `<repo>/.af/state.toml` |
| Obsidian | One markdown note per workstream, versioned frontmatter, optional Obsidian Bases aggregator |
| Secrets | macOS Keychain / Linux Secret Service via `zalando/go-keyring`; tmpfs envelope file for transport (never SSH `SetEnv`) |

## Planned commands

```
af create [task-name]       # worktree + tmux + primary agent (pi by default)
af done [session]           # tear down a workstream
af list                     # list active workstreams
af resume [session]         # re-attach to a workstream
af suspend [session]        # save state, tear down VMs/remote/tmux to free resources
af session-branch           # launch agent tied to current branch (no worktree)

af agent add/stop/list      # add or stop additional agents in a workstream
af gc                       # clean merged/closed workstreams

af setup                    # one-shot user-scope setup (gitignore, completions, config init, vault hint)
af doctor [--remote <host>] # probe deps; print install commands; never auto-install
af note [session]           # open the Obsidian note for a workstream

af editor                   # open worktree in $EDITOR (config-driven)
af diff [session]           # git diff against the workstream's base branch (config-driven)
af pr [session]             # create a PR via gh (config-driven)

af config show/init         # print or initialise config
af completions <shell>      # generate shell completion script
af version                  # print version
```

## v0 → v1 boundary

- **v0** (Rust, `src/`, `Cargo.toml`, `justfile`, etc.) is in this tree as reference. **Do not modify.** It will be removed once v1 has parity.
- **v1** (Go) lives under `cmd/af/` and `internal/...` (paths to be created during implementation). Documentation is under `docs/`.
- All v0 design history is at [`docs/v0/`](docs/v0/) (30 ADRs, full SPEC, full PLAN, eleven-session PROGRESS log).

## Documentation

| Resource | Description |
|---|---|
| [`CHANGELOG.md`](CHANGELOG.md) | Keep-a-Changelog format; `[Unreleased]` for v1 |
| [`PROGRESS.md`](PROGRESS.md) | Narrative log per work session |
| [`TODO.md`](TODO.md) | Doc-pass and post-doc-pass checklist |
| [`docs/SPEC.md`](docs/SPEC.md) | v1 specification *(written in stage C of the doc pass)* |
| [`docs/PLAN.md`](docs/PLAN.md) | Lightweight pointer to ADR groupings *(stage C)* |
| [`docs/CONVENTIONS.md`](docs/CONVENTIONS.md) | Go style, commit format, file ownership *(stage C)* |
| [`docs/adr/`](docs/adr/) | v1 ADRs 031–053 *(stage D, append-only)* |
| [`docs/v0/`](docs/v0/) | Frozen v0 (Rust era) archive |
| [`AGENTS.md`](AGENTS.md) | Working agreement for AI agents touching this repo |
| [`CLAUDE.md`](CLAUDE.md) | Project constitution (rules that survive context compaction) |

## Installation (planned)

```bash
go install github.com/kakkoyun/af@latest
# or
git clone https://github.com/kakkoyun/af && cd af && make install
```

Neither command works yet — there is no Go code in this repository. The
documentation is the contract until implementation begins.

## License

[MIT](LICENSE)
