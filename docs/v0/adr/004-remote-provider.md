# ADR-004: Remote Execution Provider

**Status:** Accepted
**Date:** 2026-03-26

## Context

`cf` supports running agents on remote machines via two providers (DD Workspaces and exe.dev),
selected by git remote org. The provider interface is implicit — zsh functions following a naming
convention (`_cf_remote_provider_<name>_detect`, `_cf_remote_create_<name>`, etc.).

The Rust rewrite needs:

- A clean trait boundary for remote providers
- Org-based routing logic extracted into configuration (not hardcoded)
- SSH bootstrap and provisioning as reusable operations
- Provider-specific teardown

## Decision

### Remote Provider trait

```rust
pub trait RemoteProvider {
    /// Provider identifier (e.g., "workspaces", "exedev")
    fn name(&self) -> &str;

    /// Check if this provider is available and should handle the given context
    fn detect(&self, ctx: &RepoContext, config: &Config) -> bool;

    /// Create a remote environment, return SSH host
    fn create(&self, name: &str, repo: &str, branch: Option<&str>) -> Result<String>;

    /// Post-bootstrap setup (clone repo, auth forwarding, etc.)
    fn setup(&self, ssh_host: &str, repo: &str, branch: Option<&str>, git_root: &Path)
        -> Result<()>;

    /// Tear down a remote environment
    fn teardown(&self, name: &str) -> Result<()>;

    /// List active environments (for orphan detection)
    fn list(&self) -> Result<Vec<RemoteInstance>>;
}
```

### Provider routing (configured, not hardcoded)

```toml
# Route by org — first matching provider wins
[providers.workspaces]
enabled = true
orgs = ["DataDog", "ddoghq", "open-telemetry"]

[providers.exedev]
enabled = true
# Matches any org not claimed by another provider
fallback = true
```

### Bootstrap pipeline

Bootstrap is a sequence of steps executed via SSH. Instead of shipping bash scripts inside the
binary, `af` shells out to configurable bootstrap scripts:

```toml
[providers.exedev]
bootstrap_script = "~/.config/af/scripts/bootstrap-remote.sh"
provision_script = "~/.config/af/scripts/provision-dotfiles.sh"
```

Default scripts are embedded in the binary (via `include_str!`) for zero-config operation,
but can be overridden.

## Consequences

- Provider selection moves from implicit org-matching in zsh to explicit config.
- Bootstrap scripts are decoupled — users can customize without forking `af`.
- Default embedded scripts match `cf`'s current bootstrap behaviour.
- New providers (e.g., Gitpod, Codespaces) are a trait impl + config entry.
- SSH operations (`ssh`, `scp`) are invoked via `std::process::Command` — we don't embed an SSH library.
