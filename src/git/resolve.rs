//! Base branch resolution and remote fetching.
//!
//! Resolves which branch a new worktree should fork from. Prefers `upstream`
//! remote (forks), falls back to `origin`. Detects the main branch name
//! (main/master/trunk) and returns the fully qualified remote-tracking ref.

use std::path::Path;
use std::process::Command;

use super::branch;

/// Errors from resolution operations.
#[derive(Debug, thiserror::Error)]
pub enum ResolveError {
    /// Failed to run a git command.
    #[error("git command failed: {0}")]
    GitCommand(String),
    /// Failed to spawn git.
    #[error("failed to run git: {0}")]
    Spawn(#[from] std::io::Error),
}

/// Detect the preferred remote: `upstream` if it exists, else `origin`.
pub fn preferred_remote(git_root: &Path) -> Result<String, ResolveError> {
    let output = Command::new("git")
        .args(["remote"])
        .current_dir(git_root)
        .output()?;

    let remotes = String::from_utf8_lossy(&output.stdout);
    for remote in remotes.lines() {
        if remote.trim() == "upstream" {
            return Ok("upstream".to_owned());
        }
    }
    Ok("origin".to_owned())
}

/// Check if a remote exists.
pub fn remote_exists(git_root: &Path, remote: &str) -> bool {
    Command::new("git")
        .args(["remote", "get-url", remote])
        .current_dir(git_root)
        .output()
        .is_ok_and(|o| o.status.success())
}

/// Check if the repo has an `upstream` remote (i.e., is a fork).
pub fn has_upstream(git_root: &Path) -> bool {
    remote_exists(git_root, "upstream")
}

/// Fetch from a remote (quietly).
pub fn fetch(git_root: &Path, remote: &str) -> Result<(), ResolveError> {
    let output = Command::new("git")
        .args(["fetch", remote, "--quiet"])
        .current_dir(git_root)
        .output()?;

    if output.status.success() {
        Ok(())
    } else {
        Err(ResolveError::GitCommand(format!(
            "git fetch {remote} failed: {}",
            String::from_utf8_lossy(&output.stderr)
        )))
    }
}

/// List local branch names.
pub fn list_local_branches(git_root: &Path) -> Result<Vec<String>, ResolveError> {
    let output = Command::new("git")
        .args(["branch", "--format=%(refname:short)"])
        .current_dir(git_root)
        .output()?;

    if !output.status.success() {
        return Err(ResolveError::GitCommand(
            "git branch --format failed".to_owned(),
        ));
    }

    Ok(String::from_utf8_lossy(&output.stdout)
        .lines()
        .map(|l| l.trim().to_owned())
        .filter(|l| !l.is_empty())
        .collect())
}

/// Detect the main branch from local branches.
///
/// Uses [`branch::detect_main_branch`] against the local branch list.
pub fn detect_main_branch_local(git_root: &Path) -> Result<String, ResolveError> {
    let branches = list_local_branches(git_root)?;
    let branch_refs: Vec<&str> = branches.iter().map(String::as_str).collect();
    Ok(branch::detect_main_branch(&branch_refs).to_owned())
}

/// Check if a remote-tracking ref exists (e.g., `refs/remotes/upstream/main`).
fn remote_ref_exists(git_root: &Path, remote: &str, branch: &str) -> bool {
    Command::new("git")
        .args([
            "show-ref",
            "--verify",
            "--quiet",
            &format!("refs/remotes/{remote}/{branch}"),
        ])
        .current_dir(git_root)
        .status()
        .is_ok_and(|s| s.success())
}

/// Resolve the base branch ref to fork from.
///
/// Strategy:
/// 1. Determine the preferred remote (`upstream` > `origin`).
/// 2. Detect the main branch name (`main` > `master` > `trunk`).
/// 3. If the remote-tracking ref exists, return `<remote>/<main>`.
/// 4. Otherwise fall back to the local branch name.
///
/// Does **not** fetch — call [`fetch`] first if you want fresh data.
pub fn resolve_base_branch(git_root: &Path) -> Result<String, ResolveError> {
    let remote = preferred_remote(git_root)?;
    let main_branch = detect_main_branch_local(git_root)?;

    if remote_ref_exists(git_root, &remote, &main_branch) {
        Ok(format!("{remote}/{main_branch}"))
    } else {
        Ok(main_branch)
    }
}

/// Fetch from the preferred remote and resolve the base branch.
///
/// Combines [`fetch`] + [`resolve_base_branch`] for the common case.
pub fn fetch_and_resolve_base(git_root: &Path) -> Result<String, ResolveError> {
    let remote = preferred_remote(git_root)?;
    fetch(git_root, &remote)?;
    resolve_base_branch(git_root)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::process::Command as StdCommand;
    use tempfile::TempDir;

    fn init_repo(dir: &Path) {
        StdCommand::new("git")
            .args(["init", "-q", "-b", "main"])
            .current_dir(dir)
            .status()
            .unwrap();
        StdCommand::new("git")
            .args(["config", "user.email", "test@test.com"])
            .current_dir(dir)
            .status()
            .unwrap();
        StdCommand::new("git")
            .args(["config", "user.name", "Test"])
            .current_dir(dir)
            .status()
            .unwrap();
        StdCommand::new("git")
            .args(["commit", "--allow-empty", "-q", "-m", "init"])
            .current_dir(dir)
            .status()
            .unwrap();
    }

    fn add_remote(dir: &Path, name: &str, url: &str) {
        StdCommand::new("git")
            .args(["remote", "add", name, url])
            .current_dir(dir)
            .status()
            .unwrap();
    }

    #[test]
    fn test_preferred_remote_defaults_to_origin() {
        let dir = TempDir::new().unwrap();
        init_repo(dir.path());
        add_remote(dir.path(), "origin", "https://github.com/org/repo.git");

        let remote = preferred_remote(dir.path()).unwrap();
        assert_eq!(remote, "origin");
    }

    #[test]
    fn test_preferred_remote_prefers_upstream() {
        let dir = TempDir::new().unwrap();
        init_repo(dir.path());
        add_remote(dir.path(), "origin", "https://github.com/fork/repo.git");
        add_remote(dir.path(), "upstream", "https://github.com/org/repo.git");

        let remote = preferred_remote(dir.path()).unwrap();
        assert_eq!(remote, "upstream");
    }

    #[test]
    fn test_has_upstream_true() {
        let dir = TempDir::new().unwrap();
        init_repo(dir.path());
        add_remote(dir.path(), "upstream", "https://github.com/org/repo.git");

        assert!(has_upstream(dir.path()));
    }

    #[test]
    fn test_has_upstream_false() {
        let dir = TempDir::new().unwrap();
        init_repo(dir.path());
        add_remote(dir.path(), "origin", "https://github.com/org/repo.git");

        assert!(!has_upstream(dir.path()));
    }

    #[test]
    fn test_list_local_branches() {
        let dir = TempDir::new().unwrap();
        init_repo(dir.path());
        StdCommand::new("git")
            .args(["branch", "feature-a"])
            .current_dir(dir.path())
            .status()
            .unwrap();
        StdCommand::new("git")
            .args(["branch", "feature-b"])
            .current_dir(dir.path())
            .status()
            .unwrap();

        let branches = list_local_branches(dir.path()).unwrap();
        assert!(branches.contains(&"main".to_owned()));
        assert!(branches.contains(&"feature-a".to_owned()));
        assert!(branches.contains(&"feature-b".to_owned()));
        assert_eq!(branches.len(), 3);
    }

    #[test]
    fn test_detect_main_branch_local_finds_main() {
        let dir = TempDir::new().unwrap();
        init_repo(dir.path());

        let main = detect_main_branch_local(dir.path()).unwrap();
        assert_eq!(main, "main");
    }

    #[test]
    fn test_detect_main_branch_local_finds_master() {
        let dir = TempDir::new().unwrap();
        // Init with master as default branch
        StdCommand::new("git")
            .args(["init", "-q", "-b", "master"])
            .current_dir(dir.path())
            .status()
            .unwrap();
        StdCommand::new("git")
            .args(["config", "user.email", "test@test.com"])
            .current_dir(dir.path())
            .status()
            .unwrap();
        StdCommand::new("git")
            .args(["config", "user.name", "Test"])
            .current_dir(dir.path())
            .status()
            .unwrap();
        StdCommand::new("git")
            .args(["commit", "--allow-empty", "-q", "-m", "init"])
            .current_dir(dir.path())
            .status()
            .unwrap();

        let main = detect_main_branch_local(dir.path()).unwrap();
        assert_eq!(main, "master");
    }

    #[test]
    fn test_resolve_base_branch_local_only() {
        let dir = TempDir::new().unwrap();
        init_repo(dir.path());
        // No remotes — should fall back to local main branch
        let base = resolve_base_branch(dir.path()).unwrap();
        assert_eq!(base, "main");
    }

    #[test]
    fn test_resolve_base_branch_with_remote_tracking() {
        let dir = TempDir::new().unwrap();
        init_repo(dir.path());

        // Create a bare upstream repo to fetch from
        let upstream_dir = TempDir::new().unwrap();
        StdCommand::new("git")
            .args(["clone", "--bare", "-q"])
            .arg(dir.path())
            .arg(upstream_dir.path().join("repo.git"))
            .status()
            .unwrap();

        add_remote(
            dir.path(),
            "origin",
            upstream_dir.path().join("repo.git").to_str().unwrap(),
        );

        // Fetch to populate remote-tracking refs
        fetch(dir.path(), "origin").unwrap();

        let base = resolve_base_branch(dir.path()).unwrap();
        assert_eq!(base, "origin/main");
    }

    #[test]
    fn test_fetch_nonexistent_remote_fails() {
        let dir = TempDir::new().unwrap();
        init_repo(dir.path());

        let result = fetch(dir.path(), "nonexistent");
        assert!(result.is_err());
    }

    #[test]
    fn test_remote_exists_true() {
        let dir = TempDir::new().unwrap();
        init_repo(dir.path());
        add_remote(dir.path(), "origin", "https://example.com/repo.git");

        assert!(remote_exists(dir.path(), "origin"));
    }

    #[test]
    fn test_remote_exists_false() {
        let dir = TempDir::new().unwrap();
        init_repo(dir.path());

        assert!(!remote_exists(dir.path(), "nonexistent"));
    }
}
