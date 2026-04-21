# ADR-027: Remote = SSH Target

**Status:** Accepted
**Date:** 2026-04-21
**Supersedes:** ADR-004 §30–44 (`RemoteProvider` trait surface) and ADR-017
§"probe" prose (provider identity conflation).

## Context

Per directive D2, "remote = SSH-able host" — exe.dev is not special. Both
exedev VMs and DD Workspaces VMs resolve to entries in `~/.ssh/config` after
their respective `create` call. The distinction between them is **lifecycle**
(create / list / teardown / suspend), not **connection**.

The architect review observed that `RemoteProvider` currently mixes lifecycle
with an implied "produces an SSH target" contract. `setup(ssh_host, repo,
branch, git_root)` on the trait only applies to exedev — workspaces' own CLI
owns provisioning.

Security finding N2: `StrictHostKeyChecking=no` on ADR-017's probe enables
MITM credential capture when combined with the session's `accept-new`.

## Decision

### Narrowed `RemoteProvider` trait

```rust
pub trait RemoteProvider: Send + Sync {
    fn name(&self) -> &str;
    fn create(&self, req: &CreateRequest) -> Result<SshTarget>;
    fn list(&self) -> Result<Vec<RemoteSession>>;
    fn teardown(&self, name: &str) -> Result<()>;
    fn detect(&self) -> Result<()>;
    fn ssh_target(&self, name: &str) -> Result<SshTarget>;
    fn is_alive(&self, name: &str) -> Result<Liveness>;
}

pub enum Liveness { Alive, Suspended, Unreachable, Unknown }
pub struct SshTarget { pub host: String /* alias in ~/.ssh/config */ }
```

- `setup(…)` is **removed from the trait** and moves to
  `ExedevProvider::bootstrap(…)` as a concrete method. Workspaces does not
  need it; the workspaces CLI owns bootstrap.
- `is_alive` returns the four-state `Liveness` enum. Workspaces' `Suspended`
  (VM exists but not SSH-reachable) is not an orphan. `af list` uses the
  distinction for its orphan column.
- **Universal probe** lives in a new free function
  `src/provider/ssh.rs::is_alive(target, timeout)`. It uses
  `StrictHostKeyChecking=accept-new` on **both** probe and session — per N2,
  `no` is never safe on paths that precede key transit.

### Per-provider liveness

- **exedev:** `SshTarget` → free-function SSH probe. 4-second connect timeout;
  returns `Alive` or `Unreachable`.
- **workspaces:** `workspaces list | grep <name>` first. If present and status
  is suspended → `Suspended`. Else fall through to the SSH probe.

### Orphan detection rule (Lane L-REMOTE)

- `Alive` / `Suspended` → not orphan.
- `Unreachable` → orphan (the user can `af done --force`).
- `Unknown` → display as-is; do not auto-clean.

## Alternatives considered

- **Keep `setup()` on the trait; stub it for workspaces.** Rejected: stubbing
  a trait method is the "lies about what it does" smell the architect
  flagged.
- **New `VmLifecycle` trait separate from a `SshReachable` trait.** Rejected:
  real cost (two trait bounds everywhere a remote is used) with no benefit at
  two providers.
- **`is_alive -> bool` with workspaces treating `Suspended` as `true`.**
  Rejected: loses the UX distinction in `af list`. The four-state enum is
  cheap.

## Consequences

- The `RemoteProvider` trait is narrower and more honest.
- `accept-new` on the probe closes security finding N2.
- Lane L-REMOTE owns the probe, orphan, and liveness changes in one lane
  (folding former A1 + A2 + B3 + B4).
- ADR-004 §30–44 and ADR-017 §"probe" (L33 and L80–83) are superseded by this
  ADR. ADR-017 carries an amendment pointer.
- ADR-026 was drafted as a provider-specific-liveness ADR but folded into
  this ADR during the Phase II.5 revision round; it never landed as an
  independent ADR.
