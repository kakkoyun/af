---
adr: 031
title: "v1: Migration to Go + Scope Reduction"
status: proposed
implementation: pending
date: 2026-05-06
last_modified: 2026-05-06
supersedes: []
superseded_by: null
related: ["032", "033", "034", "043", "049", "053"]
tags: ["meta", "scope", "v1"]
---

# ADR-031: v1 — Migration to Go + Scope Reduction

## Context

The v0 (Rust) implementation reached ~16k LOC across 60 source files,
626 tests, and 30 ADRs (`docs/v0/adr/001` through `030`, with 026
retired). It was never released to anyone other than the owner. Two
forces motivate a rewrite:

1. **Scope drift.** The Rust tree drifted from "cf in Rust" into a
   30-ADR architecture covering remote provider plugins, sandbox
   composition matrices, multi-multiplexer support, skill-bundle
   installers, dotfiles provisioning, and a release pipeline aimed at
   cross-org distribution. None of this is needed for a single-user
   tool.

2. **Language fit.** The owner writes more Go than Rust day-to-day. Go's
   standard library covers most of what `af` does (process exec, TOML,
   filesystem, JSON, slog, context, testing). The reduced scope makes
   Rust's strengths (zero-cost abstraction, ownership) less compelling
   for a CLI that is mostly process orchestration.

Without a deliberate cut, the v1 rewrite would re-create v0's drift in
a different language. This ADR draws the boundary.

## Decision

This is the **master ADR for v1**. It establishes:

1. **v1 is a Go rewrite.** No Rust toolchain, no `Cargo.toml`. The Rust
   source tree (`src/`, `Cargo.*`, `clippy.toml`, `deny.toml`,
   `rust-toolchain.toml`, `rustfmt.toml`, `.cargo/`, `justfile`) stays
   in-tree as **read-only reference** until v1 has functional parity,
   then is removed in a single commit. v0 remains accessible via git
   history.

2. **No release.** Single user. Install via `go install` or
   `make install`. No tags, no Homebrew tap, no GitHub Releases.

3. **Scope cut** (full table below).

4. **Approved runtime dependency set** (5 packages):
   - `github.com/spf13/cobra` (+ transitive `pflag`) — CLI framework + completion generator (ADR-035).
   - `github.com/BurntSushi/toml` — config + state-file serialization (ADR-036, ADR-037).
   - `github.com/google/uuid` — UUID v5 derivation (ADR-037, ADR-038).
   - `gopkg.in/yaml.v3` — Obsidian frontmatter (ADR-047).
   - `github.com/zalando/go-keyring` — secret storage (ADR-049).

   Plus one dev dep: `github.com/rogpeppe/go-internal/testscript` (ADR-051).

   Plus dev tools that are not vendored: `golangci-lint`, `goreleaser`,
   `gofumpt`, `goimports`.

   **Adding any further runtime dep requires a new ADR.**

5. **Pedantic quality bar.** All `golangci-lint` linters on (ADR-050);
   TDD with `go test -race -count=1` (ADR-051); atomic commits per
   `docs/CONVENTIONS.md`.

6. **ADR sequence is append-only from 031.** v0 ADRs (001–030) are
   frozen and never modified (ADR-033).

### Scope cut table

| v0 component                                                | v1 disposition                                                   | Reason                                                      |
| ----------------------------------------------------------- | ---------------------------------------------------------------- | ----------------------------------------------------------- |
| 6 agent providers (claude, pi, codex, gemini, amp, copilot) | **Reduced to 3**: pi (default), claude, codex (ADR-043)          | Only three the owner uses                                   |
| 2 multiplexers (tmux, cmux) + zellij/ghostty stubs          | **tmux only** (ADR-040)                                          | One multiplexer is enough                                   |
| 2 remote providers (exedev, workspaces) + plugin layer      | **Generic SSH host** (ADR-041)                                   | User configures `~/.ssh/config` externally; no plugin layer |
| 2 sandbox providers (slicer, sbx)                           | **Kept** (ADR-042)                                               | Both are still used                                         |
| Three-layer composition (`agent × remote × sandbox`)        | **Replaced by explicit `--remote` + `--sandbox` flags**          | The runtime model adds complexity without clarity           |
| Provisioning pipeline (SSH bootstrap, dotfiles install)     | **Dropped**; `af doctor` prints install hints (ADR-042, ADR-044) | Provisioning belongs in dotfiles, not `af`                  |
| Skill bundle installer (v0 ADR-030)                         | **Dropped**                                                      | Claude Code skill ecosystem unproven; revisit later         |
| `af migrate` (cf-sessions import)                           | **Dropped**                                                      | v0 was never released                                       |
| Multi-tier auth + keyring/secrecy/zeroize                   | **Reduced to keyring + tmpfs envelope** (ADR-049)                | Lower complexity; same security posture                     |
| `af diff` / `af pr` / `af stats` / `af export` (rich impls) | **Reduced to thin proxies** (ADR-048)                            | Funnel to underlying tool from config                       |
| Superterm notification integration                          | **Dropped**                                                      | Not used by the owner currently                             |
| `af export`, `af stats`                                     | **Dropped**                                                      | Niche; `jq` over `ledger.jsonl` is enough                   |
| mdBook user guide                                           | **Dropped**                                                      | Single-user; no audience                                    |
| Rust toolchain (`Cargo.toml` etc.)                          | **Removed after parity**                                         | Replaced by `go.mod`, `Makefile`                            |
| `justfile`                                                  | **Replaced by `Makefile`** (ADR-053)                             | Standard Go convention                                      |

### New components in v1

| Component                                                    | ADR     |
| ------------------------------------------------------------ | ------- |
| `af setup` — user-scope environment companion to `af doctor` | ADR-045 |
| `af suspend` — pair to `af resume`, tears down resources     | ADR-046 |
| Sub-worktrees per subagent on sibling branches               | ADR-038 |
| Per-repo `.af/state.toml` discovery symlink                  | ADR-038 |
| Obsidian Bases aggregator (example `.base` file)             | ADR-047 |
| Versioned Obsidian frontmatter (`af_schema: 1`)              | ADR-047 |

### Cutover

1. v0 docs archived to `docs/v0/` (already done; commits C1–C5 of doc plan).
2. v1 ADRs 031–053 land (this commit and 22 others).
3. Implementation proceeds per `docs/PLAN.md` ADR-dependency stages.
4. Each ADR's `implementation` frontmatter advances `pending → in-progress → complete` as Go code lands.
5. When ADRs 034–048 are all `implementation: complete`, the Rust source tree is removed in a single commit.

There is no "v0 to v1 data migration." `af` was unreleased; the owner
will `af done --force` any active v0 workstreams before running the
new v1 binary.

## Consequences

- The Go tree starts truly fresh — no compatibility shim from v0 schemas.
- Removing `src/` later is a pure-deletion commit, easy to rollback via `git revert`.
- Adding a runtime dep is a deliberate ADR-amending act, not a casual `go get`.
- The 30 v0 ADRs remain useful for "how did we think about X?" but never as runtime authority.
- Single-user constraint avoids release engineering overhead. If v1 escapes that scope later, ADR-053 will be amended.

## Alternatives Considered

- **Keep Rust, prune scope.** Rejected: doesn't address the language-fit motivation; the Rust tree is so entangled that a "prune" would be a rewrite anyway.
- **Tag `v0.1.0-rust-final`** before deleting `src/`. Rejected per user directive: no releases for this tool, git history is sufficient.
- **Sibling repo `af-go`.** Rejected: the owner wants one repo to track this project's history; doc archival under `docs/v0/` is enough.

## References

- [`docs/v0/SPEC.md`](../v0/SPEC.md) — v0 specification.
- [`docs/v0/adr/README.md`](../v0/adr/README.md) — v0 ADR index (001–030).
- [`docs/SPEC.md`](../SPEC.md) — v1 specification.
- [`docs/PLAN.md`](../PLAN.md) — v1 sequencing.
- ADR-032 (conventions), ADR-033 (archival policy) — meta layer.
