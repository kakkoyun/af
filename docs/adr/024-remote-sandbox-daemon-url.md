# ADR-024: Remote Sandbox via Daemon URL

**Status:** Accepted
**Date:** 2026-04-21
**Supersedes:** ADR-014 §"Composition model" L37–41 for slicer.

## Context

Slicer exposes `--url <URL>` / `SLICER_URL` plus `--token <T>` / `SLICER_TOKEN`
daemon mode (verified against the installed CLI). The user's shell already
wraps this: `slicer --remote=<host> <cmd>` resolves to
`command slicer <cmd> --url <resolved> --token-file <path>`.

ADR-014's four-step composition pipeline (SSH → provision → sandbox → launch)
is correct for the sbx+exedev path but wrong for slicer: daemon mode has no
SSH install step and no provisioning pipeline. Per the architect review,
"provisioning is the bridge" becomes "connection is the bridge" for
daemon-based providers.

## Decision

- Add a `remote_daemon: Option<RemoteDaemon>` field to `SandboxConfig`
  (`src/provider/mod.rs`):

  ```rust
  pub struct RemoteDaemon {
      pub url: String,
      pub token_source: TokenSource,  // File(PathBuf) | Env(String) | Keyring
  }
  ```

- The slicer provider reads `remote_daemon` and appends `--url` and
  `--token-file` (or `--token` when `Env`) to every invocation.
- The sbx provider **ignores** `remote_daemon` for 0.1.0 (sbx has no daemon
  mode). Future sbx daemon support reuses the same field without trait change.
- `TokenSource::Keyring` integrates with ADR-025's scope (host-only keyring);
  daemon tokens are per-host, stored as `af/slicer/<host-alias>`.
- Feature-gated behind `slicer-remote` (already scaffolded in Lane D).

## Alternatives considered

- **Wrap exedev's SSH path for slicer too.** Rejected: duplicates provisioning
  that the slicer daemon already handles natively.
- **Treat slicer-local and slicer-remote as separate providers.** Rejected:
  the agent and workflow are identical; the only delta is URL presence.

## Consequences

- Lane L-SBX-DAEMON shrinks to "plumb `remote_daemon` through one struct plus
  one test."
- The `SandboxProvider` trait is unchanged.
- Future providers with daemon modes (hypothetical: remote `sbx`, remote
  `modal`) reuse the same field.
- ADR-014's composition pipeline remains authoritative for the sbx+exedev
  path; only the slicer-specific prose in §"Composition model" L37–41 is
  superseded.
