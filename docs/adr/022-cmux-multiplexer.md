# ADR-022: cmux Multiplexer Provider

**Status:** Accepted
**Date:** 2026-04-21

## Context

`af` uses tmux as the sole `Multiplexer` trait implementation (`src/mux/tmux.rs`).
cmux is a macOS-native multiplexer with Unix-socket IPC, native `cmux ssh` for
remote workspaces, an RPC channel, and a tmux-compatible primitive surface
(`send`, `send-key`, `new-workspace`, `capture-pane`, `resize-pane`, etc.). Per
user directive D3, cmux and tmux are interchangeable multiplexers selected via
`[general] multiplexer = "tmux"|"cmux"`.

A critic review recommended deferring cmux to 0.2.0 on the grounds that four
open cmux design questions remain and `CMUX_SOCKET_PASSWORD` threatens to leak
into the mux layer. The user directive is authoritative: cmux ships in 0.1.0.
The critic's reasoning is preserved in the Consequences section so a future
session sees why the open decisions still matter.

## Decision

- **First-class `CmuxMux` implementation.** No change to the `Multiplexer` trait
  (17 methods); no new capability flag. Each trait method maps to cmux's native
  primitive or its tmux-compat counterpart. Where cmux's `surface` concept is
  richer than tmux's `window`, `CmuxMux` projects down to the tmux model — the
  trait stays lossless for both backends.
- **Factory auto-select** in `src/mux/mod.rs`, in order:
  1. `[general] multiplexer = "tmux"|"cmux"` if set.
  2. Else `$CMUX_WORKSPACE_ID` non-empty → cmux.
  3. Else `$TMUX` non-empty → tmux.
  4. Else whichever binary is on PATH (tmux preferred).
- **`CMUX_SOCKET_PASSWORD` handling.** Read from the user's shell env at launch
  time. Never persisted by af. Out of scope for ADR-016 / ADR-025; the password
  is user-managed (cmux's own docs instruct the user to export it or rely on
  macOS Keychain integration that cmux itself owns).
- **cmux agent-opinionated subcommands** (`claude-teams`, `omc`, `omo`, `omx`,
  `codex install-hooks`) are **ignored** — `af` owns agent choice via
  `AgentProvider`. They are listed as non-goals.

## Alternatives considered

- **tmux-compat shim only.** Smaller surface but inherits cmux's quirks in
  `capture-pane` / `pipe-pane` timing. Rejected: first-class impl is cheaper once
  the trait boilerplate is paid once.
- **Defer to 0.2.0** per the critic. Rejected by user directive; the cost of
  mux ↔ secret coupling is judged tolerable because `CMUX_SOCKET_PASSWORD`
  lives outside af's secret store.

## Consequences

- Users on macOS can opt into cmux's richer window/pane model without losing
  feature parity with tmux users.
- If cmux adds new primitives post-ship, they land inside `CmuxMux` without
  touching the trait.
- **Risk carried from the critic:** coupling temptation. If a future
  contributor adds cmux-specific methods to the `Multiplexer` trait, tmux users
  regress. Mitigation: `src/mux/mod.rs` is on the shared-files list (ADR-015);
  changes go through the lead, who re-checks that both cmux and tmux can
  implement every new method.
