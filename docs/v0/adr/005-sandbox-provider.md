# ADR-005: Sandbox Provider

**Status:** Accepted
**Date:** 2026-03-26

## Context

`cf --sandbox` runs agents inside Firecracker microVMs via the `slicer` CLI. The VM provides
strong isolation — agents can't damage the host. Sandboxes can run locally (VirtioFS mounts the
worktree) or on a remote host (repo synced via tar).

Currently slicer is the only sandbox provider, but the abstraction should allow alternatives
(e.g., Docker-based sandboxes, nsjail, gVisor, or future VM technologies).

A sandbox can be **composed** with a remote provider: `--sandbox --remote host` creates a slicer VM
on a remote machine. This composition must be first-class.

## Decision

### Sandbox Provider trait

```rust
pub trait SandboxProvider {
    /// Provider identifier (e.g., "slicer")
    fn name(&self) -> &str;

    /// Check if the sandbox runtime is available
    fn is_available(&self) -> bool;

    /// Pre-flight setup (ensure daemon running, config correct, etc.)
    fn prepare(&self, config: &SandboxConfig) -> Result<()>;

    /// Create a sandbox, return the sandbox handle (hostname, ID, etc.)
    fn create(&self, name: &str, host: Option<&str>) -> Result<SandboxHandle>;

    /// Provision the sandbox (install tools, inject auth, etc.)
    fn provision(&self, handle: &SandboxHandle, opts: &ProvisionOpts) -> Result<()>;

    /// Get the in-sandbox path for a host worktree path
    fn map_path(&self, host_path: &Path) -> Result<PathBuf>;

    /// Build command to launch a shell inside the sandbox
    fn shell_cmd(&self, handle: &SandboxHandle, bootstrap_cmd: &str) -> Vec<String>;

    /// Check sandbox health
    fn is_healthy(&self, handle: &SandboxHandle) -> bool;

    /// Destroy a sandbox
    fn teardown(&self, handle: &SandboxHandle) -> Result<()>;

    /// List active sandboxes (for orphan detection)
    fn list(&self) -> Result<Vec<SandboxHandle>>;
}
```

### Composition model

```
af create --sandbox                    # local sandbox (slicer on this machine)
af create --sandbox --remote host      # remote sandbox (slicer on remote host)
af create --remote host                # remote only (no sandbox, agent on bare remote)
```

When `--sandbox --remote` are combined:

1. Remote provider ensures SSH connectivity to `host`
2. Sandbox provider creates VM on `host` (via SSH)
3. Repo sync uses sandbox provider's remote sync mechanism
4. Agent launches inside the remote sandbox

### Slicer-specific implementation

- `prepare()`: ensure `slicer-mac` daemon, validate `slicer-mac.yaml` share_home
- `create()`: `slicer vm add <group> --tag af-session=<name>`, discover hostname via list diff
- `map_path()`: `~/Workspace/.worktrees/...` → `/home/ubuntu/host/...` (VirtioFS)
- `shell_cmd()`: `slicer vm shell '<hostname>' --uid 1000 --bootstrap '<cmd>'`
- `teardown()`: `slicer vm delete <hostname>`

## Consequences

- The trait cleanly separates "sandbox lifecycle" from "remote connectivity".
- Composition (`--sandbox --remote`) is explicit — the two concerns are orthogonal.
- Adding a Docker-based sandbox would be a new struct implementing `SandboxProvider`,
  no changes to session or remote logic.
- Slicer's hostname-discovery-by-diff is fragile (race condition with concurrent launches);
  acceptable for now, documented as a known limitation.
