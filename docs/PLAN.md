# af — Implementation Plan

> Phased delivery plan for the Rust rewrite of `cf`. See [SPEC.md](SPEC.md) for the full
> specification and [adr/](adr/) for architecture decisions.

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────┐
│                        af CLI                            │
│  create · done · list · resume · gc · editor · auth      │
├─────────┬──────────┬───────────┬────────────┬───────────┤
│ Config  │ Session  │    Git    │  Obsidian  │  Completions
│ System  │ Metadata │  Helpers  │  (opt-in)  │           │
├─────────┴──────────┴───────────┴────────────┴───────────┤
│                    Provider Layer                         │
│  ┌──────────┐  ┌────────────┐  ┌──────────────┐         │
│  │  Agent   │  │ Multiplexer│  │   Remote     │         │
│  │ Provider │  │ (tmux/     │  │  Provider    │         │
│  │          │  │  zellij)   │  │              │         │
│  ├──────────┤  ├────────────┤  ├──────────────┤         │
│  │ claude   │  │ tmux       │  │ workspaces   │         │
│  │ pi       │  │ zellij(*)  │  │ exedev       │         │
│  │ codex    │  └────────────┘  └──────────────┘         │
│  │ gemini   │  ┌────────────┐                            │
│  │ amp      │  │  Sandbox   │                            │
│  └──────────┘  │  Provider  │                            │
│                │  slicer    │                            │
│                └────────────┘                            │
└─────────────────────────────────────────────────────────┘
                (*) = future
```

---

## Crate Structure (single crate, module-based)

```
src/
├── main.rs              # Entry point: tracing + clap dispatch
├── cli.rs               # Clap definitions (all subcommands)
├── lib.rs               # Re-exports, crate-level docs
├── config/
│   ├── mod.rs           # Config loading, merging, types
│   └── defaults.rs      # Compiled-in defaults
├── session/
│   ├── mod.rs           # Session types, SessionId, metadata
│   ├── store.rs         # TOML persistence (read/write/list/delete)
│   └── naming.rs        # Name sanitization, branch prefix logic
├── git/
│   ├── mod.rs           # Git operations (worktree, branch, remote)
│   ├── worktree.rs      # Create, remove, list worktrees
│   ├── branch.rs        # Branch ops, main detection, prefix
│   └── remote.rs        # Org detection, fetch, remote resolution
├── mux/
│   ├── mod.rs           # Multiplexer trait
│   └── tmux.rs          # tmux implementation
├── agent/
│   ├── mod.rs           # AgentProvider trait
│   ├── claude.rs        # Claude Code provider
│   ├── pi.rs            # pi provider
│   ├── codex.rs         # Codex provider
│   ├── gemini.rs        # Gemini CLI provider
│   └── amp.rs           # Amp provider
├── provider/
│   ├── mod.rs           # RemoteProvider trait
│   ├── workspaces.rs    # DD Workspaces
│   ├── exedev.rs        # exe.dev
│   └── slicer.rs        # SandboxProvider + slicer implementation
├── obsidian/
│   ├── mod.rs           # Note creation, frontmatter, open
│   └── template.rs      # Embedded default template
├── cmd/
│   ├── create.rs        # af create
│   ├── done.rs          # af done
│   ├── list.rs          # af list
│   ├── resume.rs        # af resume
│   ├── gc.rs            # af gc
│   ├── editor.rs        # af editor
│   ├── auth.rs          # af auth
│   ├── config_cmd.rs    # af config
│   └── note.rs          # af note
└── util/
    ├── mod.rs           # Shared utilities
    └── uuid.rs          # UUID v5 generation
```

---

## Phase 0 — Foundation

**Delivers:** Core types, traits, config system. No user-facing commands (except `af version`).

### Key dependencies

| Crate | Purpose |
|---|---|
| `clap` | CLI parsing (already added) |
| `anyhow` | Error handling in binary (already added) |
| `thiserror` | Typed errors in library (already added) |
| `tracing` | Structured logging (already added) |
| `serde` + `toml` | Config and session metadata serialization |
| `uuid` | UUID v5 generation |
| `dirs` | XDG directory resolution |
| `which` | Binary discovery for agents/tools |

### Deliverables

1. **Config module** — Load `~/.config/af/config.toml`, merge with defaults
2. **Session store** — TOML file per session in `~/.local/share/af/sessions/`
3. **Git helpers** — Shell out to `git` for worktree/branch/remote ops
4. **Multiplexer trait + tmux** — Shell out to `tmux` for session management
5. **Agent trait + Claude** — Command generation for Claude Code
6. **Naming utilities** — Sanitization, prefix logic, UUID v5

### How to validate

```bash
just test     # All unit tests pass
just lint     # Zero warnings
just check    # Full CI pipeline
```

---

## Phase 1 — Local MVP

**Delivers:** Daily-drivable replacement for `cf`/`cfd`/`cfl`/`cfr` in local mode.

### Commands

| Command | Flags | What it does |
|---|---|---|
| `af create [name]` | `--from`, `--current`, `--from-pr`, `--bare`, `--agent` | Create worktree + mux session + launch agent |
| `af done [session]` | `--force` | Teardown: kill mux, remove worktree, delete branch |
| `af list` | | Show active sessions grouped by repo |
| `af resume [session]` | `--bare` | Re-attach to existing session |
| `af session-branch` | | Launch agent with branch-tied session ID |

### Integration test strategy

- Create temp git repos with known branch structures
- Mock tmux via the `Multiplexer` trait (in-memory session store)
- Test agent command generation (don't launch real agents)
- Test worktree creation/cleanup on real filesystem

---

## Phase 2 — Multi-Agent + Config + Completions

**Delivers:** Agent switching, full config system, shell completions.

### New

- `--agent` flag selects provider
- `af config show` — dump effective config
- `af config init` — create default config file
- `af completions bash/zsh/fish` — generate completion scripts
- All 5 agent providers implemented

---

## Phase 3 — Remote Providers

**Delivers:** `af create --remote` with DD Workspaces and exe.dev.

### New

- `--remote [host]` flag
- `--yolo` flag
- SSH bootstrap pipeline
- Remote session resume with reconnect
- Orphan detection in `af list`

---

## Phase 4 — Sandbox + Obsidian

**Delivers:** `af create --sandbox`, auth management, Obsidian notes.

### New

- `--sandbox` flag (composable with `--remote`)
- `af auth setup/reroll/status/clear`
- `af note [session]`
- VirtioFS path mapping
- VM health check + respawn in `af resume`

---

## Phase 5 — GC + Editor + Polish

**Delivers:** Production-ready quality.

### New

- `af gc --dry-run/--all`
- `af editor --terminal/--visual`
- Squash-merge detection heuristic
- Migration from `cf-sessions/*.env`
- Man pages, comprehensive `--help`

---

## Cross-Cutting Concerns

### Error Handling

- Library code: `thiserror` enums per module
- Binary code: `anyhow` with `.context("descriptive message")`
- User-facing errors: actionable messages ("run `af resume <name>` to reconnect")

### Testing Strategy

| Layer | Tool | What |
|---|---|---|
| Unit | `cargo test` | Pure functions: naming, config merge, UUID, command generation |
| Integration | `cargo test` | Git worktree ops on temp repos, session store CRUD |
| CLI | `assert_cmd` | Binary invocation: `af version`, `af create --help` |
| Mocked | Trait objects | Multiplexer, agent, remote provider (no real tmux/claude/SSH) |

### Logging

- `tracing` with `RUST_LOG` env var
- Stderr only (stdout reserved for machine-readable output)
- Debug-level logs for every external command executed (`git`, `tmux`, `ssh`, etc.)
