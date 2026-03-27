//! Slicer sandbox provider.
//!
//! Implements [`SandboxProvider`] for the Slicer Firecracker microVM runtime.
//! This is a stub — all lifecycle methods bail with "not yet implemented".
//! See ADR-005 for the design rationale and composition model.

use std::path::{Path, PathBuf};

use crate::provider::{ProvisionOpts, SandboxConfig, SandboxHandle, SandboxProvider};

/// Slicer Firecracker microVM sandbox provider.
///
/// Manages isolated execution environments via the `slicer` CLI.
/// Sandboxes can run locally (`VirtioFS` mounts the worktree) or on a
/// remote host (repo synced via tar). Currently a stub; full
/// implementation will shell out to `slicer vm` commands.
pub struct SlicerProvider;

impl SandboxProvider for SlicerProvider {
    fn name(&self) -> &'static str {
        "slicer"
    }

    fn is_available(&self) -> bool {
        which::which("slicer").is_ok()
    }

    fn prepare(&self, _config: &SandboxConfig) -> anyhow::Result<()> {
        anyhow::bail!("slicer provider not yet implemented")
    }

    fn create(&self, _name: &str, _host: Option<&str>) -> anyhow::Result<SandboxHandle> {
        anyhow::bail!("slicer provider not yet implemented")
    }

    fn provision(&self, _handle: &SandboxHandle, _opts: &ProvisionOpts) -> anyhow::Result<()> {
        anyhow::bail!("slicer provider not yet implemented")
    }

    fn map_path(&self, _host_path: &Path) -> anyhow::Result<PathBuf> {
        anyhow::bail!("slicer provider not yet implemented")
    }

    fn shell_cmd(&self, handle: &SandboxHandle, bootstrap_cmd: &str) -> Vec<String> {
        vec![
            "slicer".to_owned(),
            "vm".to_owned(),
            "shell".to_owned(),
            handle.hostname.clone(),
            "--uid".to_owned(),
            "1000".to_owned(),
            "--bootstrap".to_owned(),
            bootstrap_cmd.to_owned(),
        ]
    }

    fn is_healthy(&self, _handle: &SandboxHandle) -> bool {
        // Stub: cannot check health without a running sandbox
        false
    }

    fn teardown(&self, _handle: &SandboxHandle) -> anyhow::Result<()> {
        anyhow::bail!("slicer provider not yet implemented")
    }

    fn list(&self) -> anyhow::Result<Vec<SandboxHandle>> {
        anyhow::bail!("slicer provider not yet implemented")
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_slicer_name() {
        let provider = SlicerProvider;
        assert_eq!(provider.name(), "slicer");
    }

    #[test]
    fn test_slicer_is_available() {
        // Verifies the method runs without panic. Result depends on env.
        let provider = SlicerProvider;
        let _available = provider.is_available();
    }

    #[test]
    fn test_slicer_prepare_returns_not_implemented() {
        let provider = SlicerProvider;
        let config = SandboxConfig {
            group: "default".to_owned(),
            share_home: None,
        };
        let result = provider.prepare(&config);
        assert!(result.is_err());
        let err = result.unwrap_err().to_string();
        assert!(
            err.contains("not yet implemented"),
            "expected 'not yet implemented', got: {err}"
        );
    }

    #[test]
    fn test_slicer_create_returns_not_implemented() {
        let provider = SlicerProvider;
        let result = provider.create("test-sandbox", None);
        assert!(result.is_err());
        let err = result.unwrap_err().to_string();
        assert!(
            err.contains("not yet implemented"),
            "expected 'not yet implemented', got: {err}"
        );
    }

    #[test]
    fn test_slicer_create_with_host_returns_not_implemented() {
        let provider = SlicerProvider;
        let result = provider.create("test-sandbox", Some("remote-host"));
        assert!(result.is_err());
        let err = result.unwrap_err().to_string();
        assert!(
            err.contains("not yet implemented"),
            "expected 'not yet implemented', got: {err}"
        );
    }

    #[test]
    fn test_slicer_provision_returns_not_implemented() {
        let provider = SlicerProvider;
        let handle = SandboxHandle {
            id: "vm-123".to_owned(),
            hostname: "slicer-vm".to_owned(),
            provider: "slicer".to_owned(),
        };
        let opts = ProvisionOpts {
            inject_ssh_keys: true,
            install_tools: true,
        };
        let result = provider.provision(&handle, &opts);
        assert!(result.is_err());
        let err = result.unwrap_err().to_string();
        assert!(
            err.contains("not yet implemented"),
            "expected 'not yet implemented', got: {err}"
        );
    }

    #[test]
    fn test_slicer_map_path_returns_not_implemented() {
        let provider = SlicerProvider;
        let result = provider.map_path(Path::new("/home/user/project"));
        assert!(result.is_err());
        let err = result.unwrap_err().to_string();
        assert!(
            err.contains("not yet implemented"),
            "expected 'not yet implemented', got: {err}"
        );
    }

    #[test]
    fn test_slicer_shell_cmd_format() {
        let provider = SlicerProvider;
        let handle = SandboxHandle {
            id: "vm-456".to_owned(),
            hostname: "my-slicer-vm".to_owned(),
            provider: "slicer".to_owned(),
        };
        let cmd = provider.shell_cmd(&handle, "bash -l");
        assert_eq!(
            cmd,
            vec![
                "slicer",
                "vm",
                "shell",
                "my-slicer-vm",
                "--uid",
                "1000",
                "--bootstrap",
                "bash -l"
            ]
        );
    }

    #[test]
    fn test_slicer_is_healthy_returns_false_for_stub() {
        let provider = SlicerProvider;
        let handle = SandboxHandle {
            id: "vm-789".to_owned(),
            hostname: "healthy-vm".to_owned(),
            provider: "slicer".to_owned(),
        };
        assert!(!provider.is_healthy(&handle));
    }

    #[test]
    fn test_slicer_teardown_returns_not_implemented() {
        let provider = SlicerProvider;
        let handle = SandboxHandle {
            id: "vm-tear".to_owned(),
            hostname: "teardown-vm".to_owned(),
            provider: "slicer".to_owned(),
        };
        let result = provider.teardown(&handle);
        assert!(result.is_err());
        let err = result.unwrap_err().to_string();
        assert!(
            err.contains("not yet implemented"),
            "expected 'not yet implemented', got: {err}"
        );
    }

    #[test]
    fn test_slicer_list_returns_not_implemented() {
        let provider = SlicerProvider;
        let result = provider.list();
        assert!(result.is_err());
        let err = result.unwrap_err().to_string();
        assert!(
            err.contains("not yet implemented"),
            "expected 'not yet implemented', got: {err}"
        );
    }

    #[test]
    fn test_slicer_as_trait_object() {
        let provider: Box<dyn SandboxProvider> = Box::new(SlicerProvider);
        assert_eq!(provider.name(), "slicer");
    }
}
