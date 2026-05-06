# ADR-017: Remote Session Resume & Reconnect Strategy

**Status:** Accepted
**Date:** 2026-04-21
**Amended:** 2026-04-21 (probe now uses `accept-new`, per ADR-027 and security finding N2)

## Context

When a remote session's VM reboots, the network drops, or credentials rotate,
`af resume` currently fails with an opaque SSH error. There is no liveness check,
no reconnect attempt, and no actionable guidance.

Two distinct problems need separate solutions:

1. **Liveness detection**: Can we reach the VM? Is the session still alive?
2. **Reconnect/recovery**: If the VM is live but the mux session is dead (agent
   crashed, mux died), can we reattach or respawn?

The existing `af resume --respawn` (Phase 4) handles the local sandbox case (slicer
VM health + respawn). This ADR extends the pattern to remote (exe.dev) sessions.

## Decision

### Liveness check: SSH probe

Before any reconnect attempt, run a lightweight SSH probe:

```rust
pub fn is_alive(host: &str, timeout_secs: u64) -> bool {
    Command::new("ssh")
        .args([
            "-o", "ConnectTimeout=5",
            "-o", "BatchMode=yes",           // no interactive prompts
            "-o", "StrictHostKeyChecking=accept-new", // record key on first connect; never accept silent changes
            host, "true",
        ])
        .status()
        .map(|s| s.success())
        .unwrap_or(false)
}
```

`ConnectTimeout=5` keeps the UX tight. `BatchMode=yes` ensures we never hang on a
password prompt for a misconfigured host. This is implemented in `src/provider/exedev.rs`
as `ExedevProvider::is_alive`.

### Reconnect flow in `af resume`

```
af resume <session>
  ├── is_alive(host)? ──no──→ print actionable error, suggest af done --force
  │                               "VM at <host> is not reachable. It may have been
  │                                deleted or rebooted. Run 'af done --force <session>'
  │                                to clean up, then 'af create' to start fresh."
  └── yes → attach_mux(session)
               ├── mux session exists? → attach
               └── no (mux dead) → respawn agent in new mux session
                    (same logic as --respawn for slicer, generalised here)
```

The `--respawn` flag is optional for remote sessions; the default is to attempt
reconnect and fall back gracefully. Adding `--respawn` forces mux recreation even
if the existing session could be reattached.

### Orphan detection (Lane A2)

Sessions where `is_alive` returns false and the session was last known to be remote
are marked as orphaned in `af list`. The orphan marker is a display concern; it does
not trigger automatic cleanup (the user runs `af done --force`).

### Timeout + retry policy

- Single probe attempt, 5-second timeout. No retry loop — `af resume` is interactive;
  the user can retry manually if there's transient packet loss.
- Timeout is not configurable in v0.1.0 (add to `[remote]` config section in 0.2.0
  if requested).

### SSH key change after reboot

VMs may get new host keys after a reboot (especially ephemeral exe.dev VMs).
Both the probe and the full SSH session use `StrictHostKeyChecking=accept-new`
so the key is recorded on first connection but not re-challenged. Using
`accept-new` on the probe (rather than `no`) closes the MITM gap identified
in ADR-027 and security finding N2: a hijacked first connection must present
a consistent host key, and subsequent connections refuse silent changes.

## Consequences

- `af resume` gives an actionable error when a remote VM is unreachable instead of
  hanging or failing with an SSH error.
- The probe adds ~5 seconds of latency on unreachable hosts. On reachable hosts it
  completes in <100ms and is negligible.
- Both probe and session use `StrictHostKeyChecking=accept-new`, closing the
  MITM gap identified in ADR-027 and security finding N2. A hijacked first
  connection is pinned via TOFU, and subsequent connections refuse silent
  host-key changes.
- This ADR does not cover credential rotation (rotated SSH keys): that requires
  `af auth` (ADR-016) for API keys and is out of scope for SSH key management.
