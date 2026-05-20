# Changelog

All notable changes to `af` (v1) are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

> **v0 history.** The Rust implementation's changelog is archived at
> [`docs/v0/CHANGELOG.md`](docs/v0/CHANGELOG.md). v1 starts fresh — no
> version-number continuity is implied.

---

## [Unreleased]

### Added

#### Documentation pass (v0 → v1)

- v0 (Rust era) docs archived under `docs/v0/` (PROGRESS, TODO, CHANGELOG, SPEC, PLAN, CONVENTIONS, 30 ADRs, mdBook scaffold, planning, reference).
- `docs/v0/README.md` indexes the archive and explains the v1 boundary.

> v1 ADRs (031–053), v1 spec/plan/conventions, and the new top-level
> README/CLAUDE/AGENTS land in subsequent commits in this same
> Unreleased block. Each ADR will be listed once `accepted`.

#### Go implementation

- Added the initial Go module scaffold: `go.mod`, `cmd/af/`, the planned
  `internal/...` package tree, and `examples/` placeholders.
- Added the minimal cobra root command with persistent `--verbose`,
  `--config`, and `--session` flags plus `af version` backed by
  `internal/version` build metadata.
- Added pinned Go build tooling: `Makefile`, `.golangci.yml`,
  `.goreleaser.yml`, gofumpt/goimports format checks, pedantic
  `golangci-lint`, race-test `make test`, `make check`, and local
  snapshot build targets.
- Added the initial `testscript` harness, `internal/testutil` helpers,
  fake-command PATH hooks, and smoke scripts for `af version` and
  `af --help`.
- Added property-test scaffolds for lifecycle transitions and workstream
  naming invariants using stdlib `testing/quick`.
- Added the `internal/config` layered TOML loader with compiled defaults,
  user/repo merges, global-only section handling, `~` path expansion, and
  proxy command shape validation.
- Added the shared `internal/duration` grammar for `d` / `w` shorthand
  plus stdlib duration units, with table and property tests.
- Expanded `internal/workstream` naming helpers for double-dash session
  sanitization, branch prefix rules, sub-branch names, auto session names,
  and deterministic UUID session IDs.
- Added `internal/session` state persistence with atomic `state.toml`
  writes, append-only `ledger.jsonl`, flock-backed locks, repo slug
  parsing, and current-workstream discovery helpers.
- Added `internal/git` worktree planning helpers for stable primary and
  sub-worktree paths, discovery symlinks, and safe cleanup plans.
- Added `internal/secret` redacting `slog` handler plus fake-backed
  keyring interface for future `af auth` and launch-secret work.
- Added `internal/obsidian` frontmatter parse/emit helpers, note path
  resolution, and an in-memory note store for future note commands.
- Added `internal/agent` provider interfaces, pi/claude/codex command
  builders, availability checks, registry fallback, and fake provider.
- Added `internal/mux` tmux command construction, runner seams,
  recording runner, and fake multiplexer for tests.
- Added `internal/remote` SSH command construction, remote clone path
  mapping, probe command construction, and fake executor.
- Added `internal/sandbox` provider interfaces, slicer/sbx command
  builders, recording runner, and fake sandbox.
- Added testscript fake-command PATH wiring for tmux, ssh, slicer, sbx,
  pi, claude, and codex.
- Added `af completions <bash|zsh|fish|powershell>`: emits the shell-specific completion script to stdout using cobra's built-in generators (`GenBashCompletion`, `GenZshCompletion`, `GenFishCompletion`, `GenPowerShellCompletionWithDesc`).
- Added `af doctor` (local) and `af doctor --remote <host>` per ADR-044. Probes git, tmux, the agent trio (pi/claude/codex, OR-group), gh, fzf, slicer, sbx, delta, and any `[doctor].extra_tools` entries against the local PATH or an SSH remote. Renders install hints per platform (macOS/Arch/Debian/Other) detected via `/etc/os-release`. Exits 1 when any TierMust requirement is missing. Backed by `internal/doctor` with `SystemLookup`, `RemoteLookup`, and OS-release parsing fakes.
- Added `af config init` and `af config show`: `init` scaffolds the
  annotated user config at `$HOME/.config/af/config.toml` (or the path
  given via `--config`) and refuses to overwrite an existing file;
  `show` prints the effective layered configuration as canonical TOML.
  Backed by reusable `internal/config.WriteUserConfig`,
  `UserConfigTemplate`, and `Render` helpers.

### Removed

> v1 removes the Rust-era implementation surface and several v0 features.
> The v0 documentation archive remains under `docs/v0/`; deleted source
> remains available through git history.

- Rust v0 source, integration tests, Cargo files, `.cargo/`, `justfile`,
  and Rust tool configs (`src/`, `tests/`, `Cargo.toml`, `Cargo.lock`,
  `clippy.toml`, `deny.toml`, `rust-toolchain.toml`, `rustfmt.toml`).
- DD Workspaces remote provider (replaced by generic SSH-host model).
- exe.dev remote provider special-casing (subsumed by generic SSH-host model).
- cmux multiplexer; zellij/Ghostty multiplexer scaffolding.
- Three-layer composition (`agent × remote × sandbox`) as a runtime model — replaced by an explicit `--remote <ssh-host>` + `--sandbox <slicer|sbx>` flag pair on `af create`.
- Provisioning pipeline (`provision/`, embedded bootstrap scripts, dotfiles install).
- Skill bundle installer (v0 ADR-030).
- `af migrate` (v0 cf-sessions migration); v0 was never released to anyone other than the owner.
- Agent providers: gemini, amp, copilot.
- mdBook user guide.
- `clap_complete` machinery (replaced by `cobra` completion generator).
- `keyring`/`secrecy`/`zeroize` Rust dependencies (replaced by `zalando/go-keyring`).

### Reduced surface (compared to v0)

- Multiplexer providers: 1 (`tmux`). Was 2 in v0.
- Agent providers: 3 (`pi` default, `claude`, `codex`). Was 6 in v0.
- Remote providers: 1 (generic SSH host). Was 2 in v0 with a plugin layer.
- Sandbox providers: 2 (`slicer`, `sbx`). Unchanged from v0 but with simpler composition.
- Top-level commands: ~14 (TBD per ADR-031). Was 19 in v0.

### Build & distribution

- Build tool: `Make`. Was `just`.
- Release tool: `goreleaser`. Was a hand-written GitHub Actions workflow.
- Distribution targets: `linux/{amd64,arm64}`, `darwin/{amd64,arm64}` cross-compiled by default, plus `go install github.com/kakkoyun/af@latest`.
- No Homebrew tap planned for v1 (single-user project).
- No GitHub Releases planned for v1 (single-user project).

[Unreleased]: https://github.com/kakkoyun/af/compare/main...HEAD
