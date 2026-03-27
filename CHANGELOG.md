# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - Unreleased

### Added

#### Phase 0 — Foundation

- Layered TOML configuration system: compiled defaults, user config (`~/.config/af/config.toml`), project config (`.af/config.toml`)
- Session state store: file-per-session TOML with save/load/delete/list/archive
- Append-only JSONL session event ledger with builder pattern
- Session name sanitization (`/.:` replaced with `--`) and prefix logic for fork repos
- UUID v5 generation for deterministic session IDs
- Git helpers: worktree create/remove, branch create/delete
- Git helpers: main branch detection (main/master/trunk)
- Git helpers: org and repo name parsing from SSH and HTTPS remote URLs
- Git helpers: fetch and resolve base branch with upstream preference
- Platform detection for macOS, Arch Linux, and Debian
- Package manager abstraction (brew, pacman, apt)
- Dependency tier system (Must/Should/Nice) for pre-flight checks
- Multiplexer trait with full tmux implementation (create/kill/attach/env/send-keys)
- Agent provider trait with Claude Code provider (launch/resume/pr commands)

#### Phase 1 — Local MVP

- `af create [task-name]` with local worktree mode, workspace mode for non-git directories
- `af create --from <branch>` to fork from a specific branch
- `af create --current` to fork from the current branch
- `af create --from-pr <number>` for PR-based worktrees via `gh` CLI
- `af create --bare` for bare mode (no VM, host worktree)
- `af create --agent <name>` for agent provider selection
- Session limit guard via `max_sessions` config
- `af done [session]` with confirmation prompt, worktree removal, branch deletion, and archival
- `af done --force` to skip confirmation and force-delete unmerged branches
- `af list` with sessions grouped by repo, current repo highlighted
- `af resume [session]` with fzf picker for interactive selection
- `af resume` session recovery: recreate dead mux sessions from disk metadata
- `af session-branch` for branch-tied agent launch with deterministic UUID
- `af doctor` for pre-flight dependency checking with tier-based reporting
- Ledger event emission from create/done/resume commands

#### Phase 2 — Multi-Agent + Config

- Pi agent provider (`--continue` for resume)
- Codex agent provider (`--full-auto` for yolo, `resume --last`)
- Gemini agent provider (`--yolo`, `--resume latest`)
- Amp agent provider (`--dangerously-allow-all`, `threads continue --last`)
- Agent availability check with fallback chain via `first_available()`
- `af agent add --slot <name> --agent <provider>` to add agents in new tmux panes
- `af agent stop <slot>` to stop a running agent
- `af agent list` with tabular output of slot, agent, status, and pane
- Multi-agent resume: restore all running agent panes on `af resume`
- Multi-agent teardown: stop all agents with ledger events on `af done`
- `af config show` to dump effective TOML configuration with source path
- `af config init` to create default user config file
- `af completions bash|zsh|fish` via clap_complete

#### Phase 5 — GC + Editor + Polish (partial)

- `af gc` to list merged/closed worktrees eligible for cleanup
- `af gc --dry-run` to preview cleanup without action
- `af gc --all` to clean all without per-session prompts
- Merge detection via GitHub PR state (`gh` CLI)
- Merge detection via git ancestry (`merge-base --is-ancestor`)
- Merge detection via squash-merge fingerprint (diff checksum comparison)
- `af gc` prunes expired archives older than `retention_days`
- `af editor --terminal` to open `$EDITOR` in a tmux split pane
- `af editor --visual` to open VS Code or Zed GUI editor
- Editor config integration: `[editor]` section in config controls terminal and visual editor selection
- Session archival: move to archive/ on `af done`, retain for configurable days
- PR tracking: detect and record PR number and URL from branch
- Agent session log discovery for claude and pi file path conventions
- Comprehensive `--help` text for all commands
- Error messages with actionable suggestions
