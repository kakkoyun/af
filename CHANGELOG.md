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
  `internal/...` package tree, and `examples/` placeholders. The binary is
  scaffold-only until the cobra command tree lands in TODO item I0.2.

### Removed

> The following items will be **explicitly removed** in v1 once Go
> implementation begins. They are listed here as a forward-looking
> contract, not as already-removed in this changelog entry. Each is
> ratified in an ADR (see `docs/adr/` once written).

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
- Rust toolchain entirely (`Cargo.toml`, `Cargo.lock`, `clippy.toml`, `deny.toml`, `rust-toolchain.toml`, `rustfmt.toml`, `.cargo/`, `justfile`).

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
