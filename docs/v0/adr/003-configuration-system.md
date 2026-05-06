# ADR-003: Layered Configuration System

**Status:** Accepted
**Date:** 2026-03-26

## Context

`cf` uses scattered env vars (`CF_MAX_SESSIONS`, `CF_WORKSPACES_ORGS`, `CF_VISUAL_EDITOR`, etc.)
and hardcoded values (branch prefix `kakkoyun/`, worktree root `~/Workspace/.worktrees/`).
Configuration is inseparable from the dotfiles that ship it.

Since `af` is a standalone binary (not embedded in dotfiles), it needs its own configuration
system. Requirements:

- **Global/user config** — defaults, preferred agent, multiplexer, editor
- **Project config** — per-repo overrides (agent, branch prefix, remote provider)
- **Env var overrides** — for CI and scripting
- **Dotfiles decoupled** — `af` should not manage or sync dotfiles; that's orthogonal

## Decision

Use **TOML** as the config format (Rust ecosystem standard, human-friendly).

### Layer precedence (highest wins)

1. CLI flags (`--agent codex`)
2. Environment variables (`AF_AGENT=codex`)
3. Project config (`.af/config.toml` in repo root)
4. User config (`~/.config/af/config.toml`)
5. Compiled defaults

### Config locations

| Scope | Path | Purpose |
|---|---|---|
| User | `~/.config/af/config.toml` | Global defaults |
| Project | `<repo>/.af/config.toml` | Per-repo overrides |

### User config schema (initial)

```toml
# ~/.config/af/config.toml

[general]
# Default agent to launch
default_agent = "claude"
# Terminal multiplexer
multiplexer = "tmux"
# Maximum concurrent sessions
max_sessions = 10
# Worktree root directory
worktree_root = "~/Workspace/.worktrees"

[branch]
# Prefix for branches in fork repos (empty = no prefix)
prefix = "kakkoyun"
# Only prefix when 'upstream' remote exists
prefix_on_fork_only = true

[editor]
# Terminal editor ($EDITOR fallback: "nvim")
terminal = "nvim"
# Visual editor (auto-detect: code > zed)
visual = ""

[agents.claude]
# binary = "claude"            # default
# session_flag = "--session-id" # default
# resume_flag = "--continue"    # default

[agents.pi]
# binary = "pi"

[providers.workspaces]
enabled = true
orgs = ["DataDog", "ddoghq", "open-telemetry"]

[providers.exedev]
enabled = true

[providers.slicer]
enabled = true
# group = ""  # auto-select
# share_home = "~/Workspace/.worktrees/"

[obsidian]
enabled = false
# vault_path = "~/Obsidian/Work"
# template = "workstream"
```

### Project config schema

```toml
# <repo>/.af/config.toml

[general]
default_agent = "pi"

[branch]
prefix = "kakkoyun"
```

### Environment variable mapping

All config keys map to env vars with `AF_` prefix and `__` for nesting:

- `AF_GENERAL__DEFAULT_AGENT=codex`
- `AF_GENERAL__MAX_SESSIONS=20`
- `AF_BRANCH__PREFIX=myuser`

## Consequences

- Config is the **single source of truth** — no more scattered shell exports.
- TOML parsing adds a dependency (`toml` crate) but it's tiny and standard.
- Project config (`.af/config.toml`) can be committed to repos or gitignored.
- The `[agents.*]` table allows future per-agent configuration without schema changes.
- Bootstrap/provisioning scripts are **not** part of `af` — they remain in dotfiles or
  are handled by the agent's own setup mechanism.
