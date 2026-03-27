//! DD Workspaces remote provider.
//!
//! Implements [`RemoteProvider`] for Datadog's internal Workspaces platform.
//! This is a stub — all lifecycle methods bail with "not yet implemented".

use std::path::Path;

use crate::provider::{RemoteInstance, RemoteProvider};

/// DD Workspaces remote development provider.
///
/// Manages cloud development environments via the Workspaces CLI/API.
/// Currently a stub; full implementation will shell out to the
/// Workspaces CLI for instance lifecycle management.
pub struct WorkspacesProvider;

impl RemoteProvider for WorkspacesProvider {
    fn name(&self) -> &'static str {
        "DD Workspaces"
    }

    fn detect(&self, _org: &str) -> bool {
        // TODO(kakkoyun): detect Datadog org membership via CLI or API
        false
    }

    fn create(&self, _name: &str, _repo: &str, _branch: Option<&str>) -> anyhow::Result<String> {
        anyhow::bail!("workspaces provider not yet implemented")
    }

    fn setup(
        &self,
        _ssh_host: &str,
        _repo: &str,
        _branch: Option<&str>,
        _git_root: &Path,
    ) -> anyhow::Result<()> {
        anyhow::bail!("workspaces provider not yet implemented")
    }

    fn teardown(&self, _name: &str) -> anyhow::Result<()> {
        anyhow::bail!("workspaces provider not yet implemented")
    }

    fn list(&self) -> anyhow::Result<Vec<RemoteInstance>> {
        anyhow::bail!("workspaces provider not yet implemented")
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_workspaces_name() {
        let provider = WorkspacesProvider;
        assert_eq!(provider.name(), "DD Workspaces");
    }

    #[test]
    fn test_workspaces_detect_returns_false() {
        let provider = WorkspacesProvider;
        assert!(!provider.detect("datadog"));
        assert!(!provider.detect(""));
    }

    #[test]
    fn test_workspaces_create_returns_not_implemented() {
        let provider = WorkspacesProvider;
        let result = provider.create("test", "repo", None);
        assert!(result.is_err());
        let err = result.unwrap_err().to_string();
        assert!(
            err.contains("not yet implemented"),
            "expected 'not yet implemented', got: {err}"
        );
    }

    #[test]
    fn test_workspaces_setup_returns_not_implemented() {
        let provider = WorkspacesProvider;
        let result = provider.setup("host", "repo", Some("main"), Path::new("/tmp/repo"));
        assert!(result.is_err());
        let err = result.unwrap_err().to_string();
        assert!(
            err.contains("not yet implemented"),
            "expected 'not yet implemented', got: {err}"
        );
    }

    #[test]
    fn test_workspaces_teardown_returns_not_implemented() {
        let provider = WorkspacesProvider;
        let result = provider.teardown("test-instance");
        assert!(result.is_err());
        let err = result.unwrap_err().to_string();
        assert!(
            err.contains("not yet implemented"),
            "expected 'not yet implemented', got: {err}"
        );
    }

    #[test]
    fn test_workspaces_list_returns_not_implemented() {
        let provider = WorkspacesProvider;
        let result = provider.list();
        assert!(result.is_err());
        let err = result.unwrap_err().to_string();
        assert!(
            err.contains("not yet implemented"),
            "expected 'not yet implemented', got: {err}"
        );
    }

    #[test]
    fn test_workspaces_as_trait_object() {
        let provider: Box<dyn RemoteProvider> = Box::new(WorkspacesProvider);
        assert_eq!(provider.name(), "DD Workspaces");
    }
}
