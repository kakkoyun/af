//! exe.dev remote provider.
//!
//! Implements [`RemoteProvider`] for the exe.dev cloud development platform.
//! This is a stub — all lifecycle methods bail with "not yet implemented".

use std::path::Path;

use crate::provider::{RemoteInstance, RemoteProvider};

/// exe.dev remote development provider.
///
/// Manages cloud development environments via the exe.dev CLI/API.
/// Currently a stub; full implementation will integrate with the
/// exe.dev platform for instance lifecycle management.
pub struct ExedevProvider;

impl RemoteProvider for ExedevProvider {
    fn name(&self) -> &'static str {
        "exe.dev"
    }

    fn detect(&self, _org: &str) -> bool {
        // TODO(kakkoyun): detect exe.dev org membership via CLI or API
        false
    }

    fn create(&self, _name: &str, _repo: &str, _branch: Option<&str>) -> anyhow::Result<String> {
        anyhow::bail!("exedev provider not yet implemented")
    }

    fn setup(
        &self,
        _ssh_host: &str,
        _repo: &str,
        _branch: Option<&str>,
        _git_root: &Path,
    ) -> anyhow::Result<()> {
        anyhow::bail!("exedev provider not yet implemented")
    }

    fn teardown(&self, _name: &str) -> anyhow::Result<()> {
        anyhow::bail!("exedev provider not yet implemented")
    }

    fn list(&self) -> anyhow::Result<Vec<RemoteInstance>> {
        anyhow::bail!("exedev provider not yet implemented")
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_exedev_name() {
        let provider = ExedevProvider;
        assert_eq!(provider.name(), "exe.dev");
    }

    #[test]
    fn test_exedev_detect_returns_false() {
        let provider = ExedevProvider;
        assert!(!provider.detect("some-org"));
        assert!(!provider.detect(""));
    }

    #[test]
    fn test_exedev_create_returns_not_implemented() {
        let provider = ExedevProvider;
        let result = provider.create("test", "repo", None);
        assert!(result.is_err());
        let err = result.unwrap_err().to_string();
        assert!(
            err.contains("not yet implemented"),
            "expected 'not yet implemented', got: {err}"
        );
    }

    #[test]
    fn test_exedev_setup_returns_not_implemented() {
        let provider = ExedevProvider;
        let result = provider.setup("host", "repo", Some("main"), Path::new("/tmp/repo"));
        assert!(result.is_err());
        let err = result.unwrap_err().to_string();
        assert!(
            err.contains("not yet implemented"),
            "expected 'not yet implemented', got: {err}"
        );
    }

    #[test]
    fn test_exedev_teardown_returns_not_implemented() {
        let provider = ExedevProvider;
        let result = provider.teardown("test-instance");
        assert!(result.is_err());
        let err = result.unwrap_err().to_string();
        assert!(
            err.contains("not yet implemented"),
            "expected 'not yet implemented', got: {err}"
        );
    }

    #[test]
    fn test_exedev_list_returns_not_implemented() {
        let provider = ExedevProvider;
        let result = provider.list();
        assert!(result.is_err());
        let err = result.unwrap_err().to_string();
        assert!(
            err.contains("not yet implemented"),
            "expected 'not yet implemented', got: {err}"
        );
    }

    #[test]
    fn test_exedev_as_trait_object() {
        let provider: Box<dyn RemoteProvider> = Box::new(ExedevProvider);
        assert_eq!(provider.name(), "exe.dev");
    }
}
