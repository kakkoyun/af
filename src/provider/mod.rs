//! Remote and sandbox provider abstractions (ADR-004, ADR-005).
//!
//! Defines the [`RemoteProvider`] and [`SandboxProvider`] traits that encapsulate
//! remote development environment and sandbox lifecycle management. Built-in
//! providers: DD Workspaces, exe.dev (remote); Slicer (sandbox).
//!
//! Remote providers handle spinning up cloud development machines and syncing
//! repositories. Sandbox providers wrap isolation runtimes (Firecracker microVMs,
//! containers) that agents run inside for safety.
//!
//! The two concerns are orthogonal and composable: `--sandbox --remote host`
//! creates a sandbox on a remote machine.

pub mod exedev;
pub mod slicer;
pub mod workspaces;

use std::path::{Path, PathBuf};

/// Metadata about a running remote development instance.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RemoteInstance {
    /// Unique identifier for this instance (provider-specific).
    pub id: String,
    /// Human-readable name.
    pub name: String,
    /// SSH hostname or connection string.
    pub ssh_host: String,
    /// Current status (e.g., "running", "stopped").
    pub status: String,
}

/// Handle to a running sandbox instance.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct SandboxHandle {
    /// Unique identifier for this sandbox (provider-specific).
    pub id: String,
    /// Hostname inside the sandbox (for SSH or shell access).
    pub hostname: String,
    /// Provider that created this sandbox.
    pub provider: String,
}

/// Configuration for sandbox pre-flight setup.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct SandboxConfig {
    /// Group or template name for the sandbox VM.
    pub group: String,
    /// Optional share-home path for `VirtioFS` mounts.
    pub share_home: Option<PathBuf>,
}

/// Options for provisioning a sandbox after creation.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ProvisionOpts {
    /// Whether to inject SSH keys into the sandbox.
    pub inject_ssh_keys: bool,
    /// Whether to install development tooling.
    pub install_tools: bool,
}

/// Abstraction over remote development providers (ADR-004).
///
/// Each implementation manages the lifecycle of remote development machines
/// (create, sync, teardown). Providers shell out to their respective CLIs
/// or APIs.
pub trait RemoteProvider {
    /// Display name (e.g., "DD Workspaces").
    fn name(&self) -> &str;

    /// Check if this provider manages the given organization.
    fn detect(&self, org: &str) -> bool;

    /// Create a new remote instance, returning its SSH hostname.
    fn create(&self, name: &str, repo: &str, branch: Option<&str>) -> anyhow::Result<String>;

    /// Set up repository sync on an existing remote host.
    fn setup(
        &self,
        ssh_host: &str,
        repo: &str,
        branch: Option<&str>,
        git_root: &Path,
    ) -> anyhow::Result<()>;

    /// Tear down a remote instance by name.
    fn teardown(&self, name: &str) -> anyhow::Result<()>;

    /// List all active remote instances.
    fn list(&self) -> anyhow::Result<Vec<RemoteInstance>>;
}

/// Abstraction over sandbox providers (ADR-005).
///
/// Each implementation manages the lifecycle of isolated execution environments
/// (Firecracker microVMs, containers, etc.) that agents run inside for safety.
/// Sandbox providers can be composed with remote providers.
pub trait SandboxProvider {
    /// Provider identifier (e.g., "slicer").
    fn name(&self) -> &str;

    /// Check if the sandbox runtime is available on this machine.
    fn is_available(&self) -> bool;

    /// Pre-flight setup (ensure daemon running, config correct, etc.).
    fn prepare(&self, config: &SandboxConfig) -> anyhow::Result<()>;

    /// Create a sandbox, returning a handle for further operations.
    fn create(&self, name: &str, host: Option<&str>) -> anyhow::Result<SandboxHandle>;

    /// Provision the sandbox (install tools, inject auth, etc.).
    fn provision(&self, handle: &SandboxHandle, opts: &ProvisionOpts) -> anyhow::Result<()>;

    /// Get the in-sandbox path for a host worktree path.
    fn map_path(&self, host_path: &Path) -> anyhow::Result<PathBuf>;

    /// Build command to launch a shell inside the sandbox.
    fn shell_cmd(&self, handle: &SandboxHandle, bootstrap_cmd: &str) -> Vec<String>;

    /// Check sandbox health.
    fn is_healthy(&self, handle: &SandboxHandle) -> bool;

    /// Destroy a sandbox.
    fn teardown(&self, handle: &SandboxHandle) -> anyhow::Result<()>;

    /// List active sandboxes (for orphan detection).
    fn list(&self) -> anyhow::Result<Vec<SandboxHandle>>;
}

/// All known remote provider names.
pub const KNOWN_REMOTE_PROVIDERS: &[&str] = &["workspaces", "exedev"];

/// All known sandbox provider names.
pub const KNOWN_SANDBOX_PROVIDERS: &[&str] = &["slicer"];

/// Resolve a remote provider by name.
///
/// Returns `None` if the name is not recognized.
pub fn resolve_remote(name: &str) -> Option<Box<dyn RemoteProvider>> {
    match name {
        "workspaces" => Some(Box::new(workspaces::WorkspacesProvider)),
        "exedev" => Some(Box::new(exedev::ExedevProvider)),
        _ => None,
    }
}

/// Resolve a sandbox provider by name.
///
/// Returns `None` if the name is not recognized.
pub fn resolve_sandbox(name: &str) -> Option<Box<dyn SandboxProvider>> {
    match name {
        "slicer" => Some(Box::new(slicer::SlicerProvider)),
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    // ── RemoteInstance tests ─────────────────────────────────────────

    #[test]
    fn test_remote_instance_construction() {
        let instance = RemoteInstance {
            id: "inst-123".to_owned(),
            name: "my-workspace".to_owned(),
            ssh_host: "dev-host.example.com".to_owned(),
            status: "running".to_owned(),
        };
        assert_eq!(instance.id, "inst-123");
        assert_eq!(instance.name, "my-workspace");
        assert_eq!(instance.ssh_host, "dev-host.example.com");
        assert_eq!(instance.status, "running");
    }

    #[test]
    fn test_remote_instance_clone() {
        let instance = RemoteInstance {
            id: "inst-456".to_owned(),
            name: "cloned".to_owned(),
            ssh_host: "host.example.com".to_owned(),
            status: "stopped".to_owned(),
        };
        let cloned = instance.clone();
        assert_eq!(cloned, instance);
    }

    #[test]
    fn test_remote_instance_debug() {
        let instance = RemoteInstance {
            id: "inst-789".to_owned(),
            name: "debug-test".to_owned(),
            ssh_host: "debug.example.com".to_owned(),
            status: "running".to_owned(),
        };
        let debug = format!("{instance:?}");
        assert!(debug.contains("inst-789"));
        assert!(debug.contains("debug-test"));
        assert!(debug.contains("debug.example.com"));
    }

    #[test]
    fn test_remote_instance_equality() {
        let a = RemoteInstance {
            id: "same".to_owned(),
            name: "same".to_owned(),
            ssh_host: "same".to_owned(),
            status: "same".to_owned(),
        };
        let b = a.clone();
        assert_eq!(a, b);

        let c = RemoteInstance {
            id: "different".to_owned(),
            name: "same".to_owned(),
            ssh_host: "same".to_owned(),
            status: "same".to_owned(),
        };
        assert_ne!(a, c);
    }

    // ── SandboxHandle tests ─────────────────────────────────────────

    #[test]
    fn test_sandbox_handle_construction() {
        let handle = SandboxHandle {
            id: "vm-abc".to_owned(),
            hostname: "slicer-vm-1".to_owned(),
            provider: "slicer".to_owned(),
        };
        assert_eq!(handle.id, "vm-abc");
        assert_eq!(handle.hostname, "slicer-vm-1");
        assert_eq!(handle.provider, "slicer");
    }

    #[test]
    fn test_sandbox_handle_clone() {
        let handle = SandboxHandle {
            id: "vm-def".to_owned(),
            hostname: "clone-vm".to_owned(),
            provider: "slicer".to_owned(),
        };
        let cloned = handle.clone();
        assert_eq!(cloned, handle);
    }

    #[test]
    fn test_sandbox_handle_debug() {
        let handle = SandboxHandle {
            id: "vm-ghi".to_owned(),
            hostname: "debug-vm".to_owned(),
            provider: "slicer".to_owned(),
        };
        let debug = format!("{handle:?}");
        assert!(debug.contains("vm-ghi"));
        assert!(debug.contains("debug-vm"));
        assert!(debug.contains("slicer"));
    }

    #[test]
    fn test_sandbox_handle_equality() {
        let a = SandboxHandle {
            id: "same".to_owned(),
            hostname: "same".to_owned(),
            provider: "same".to_owned(),
        };
        let b = a.clone();
        assert_eq!(a, b);

        let c = SandboxHandle {
            id: "different".to_owned(),
            hostname: "same".to_owned(),
            provider: "same".to_owned(),
        };
        assert_ne!(a, c);
    }

    // ── SandboxConfig tests ─────────────────────────────────────────

    #[test]
    fn test_sandbox_config_construction() {
        let config = SandboxConfig {
            group: "default".to_owned(),
            share_home: Some(PathBuf::from("/home/user/Workspace")),
        };
        assert_eq!(config.group, "default");
        assert_eq!(
            config.share_home,
            Some(PathBuf::from("/home/user/Workspace"))
        );
    }

    #[test]
    fn test_sandbox_config_without_share_home() {
        let config = SandboxConfig {
            group: "minimal".to_owned(),
            share_home: None,
        };
        assert_eq!(config.group, "minimal");
        assert!(config.share_home.is_none());
    }

    // ── ProvisionOpts tests ─────────────────────────────────────────

    #[test]
    fn test_provision_opts_construction() {
        let opts = ProvisionOpts {
            inject_ssh_keys: true,
            install_tools: false,
        };
        assert!(opts.inject_ssh_keys);
        assert!(!opts.install_tools);
    }

    #[test]
    fn test_provision_opts_clone() {
        let opts = ProvisionOpts {
            inject_ssh_keys: true,
            install_tools: true,
        };
        let cloned = opts.clone();
        assert_eq!(cloned, opts);
    }

    // ── Trait object creation tests ─────────────────────────────────

    #[test]
    fn test_remote_provider_trait_object_creation() {
        let provider: Box<dyn RemoteProvider> = Box::new(workspaces::WorkspacesProvider);
        assert_eq!(provider.name(), "DD Workspaces");
    }

    #[test]
    fn test_sandbox_provider_trait_object_creation() {
        let provider: Box<dyn SandboxProvider> = Box::new(slicer::SlicerProvider);
        assert_eq!(provider.name(), "slicer");
    }

    #[test]
    fn test_remote_provider_trait_object_exedev() {
        let provider: Box<dyn RemoteProvider> = Box::new(exedev::ExedevProvider);
        assert_eq!(provider.name(), "exe.dev");
    }

    // ── resolve_remote tests ────────────────────────────────────────

    #[test]
    fn test_resolve_remote_workspaces() {
        let provider = resolve_remote("workspaces");
        assert!(provider.is_some());
        assert_eq!(provider.unwrap().name(), "DD Workspaces");
    }

    #[test]
    fn test_resolve_remote_exedev() {
        let provider = resolve_remote("exedev");
        assert!(provider.is_some());
        assert_eq!(provider.unwrap().name(), "exe.dev");
    }

    #[test]
    fn test_resolve_remote_unknown_returns_none() {
        assert!(resolve_remote("nonexistent").is_none());
        assert!(resolve_remote("").is_none());
    }

    // ── resolve_sandbox tests ───────────────────────────────────────

    #[test]
    fn test_resolve_sandbox_slicer() {
        let provider = resolve_sandbox("slicer");
        assert!(provider.is_some());
        assert_eq!(provider.unwrap().name(), "slicer");
    }

    #[test]
    fn test_resolve_sandbox_unknown_returns_none() {
        assert!(resolve_sandbox("docker").is_none());
        assert!(resolve_sandbox("").is_none());
    }

    // ── Known provider constants ────────────────────────────────────

    #[test]
    fn test_known_remote_providers() {
        assert_eq!(KNOWN_REMOTE_PROVIDERS.len(), 2);
        assert!(KNOWN_REMOTE_PROVIDERS.contains(&"workspaces"));
        assert!(KNOWN_REMOTE_PROVIDERS.contains(&"exedev"));
    }

    #[test]
    fn test_known_sandbox_providers() {
        assert_eq!(KNOWN_SANDBOX_PROVIDERS.len(), 1);
        assert!(KNOWN_SANDBOX_PROVIDERS.contains(&"slicer"));
    }
}
