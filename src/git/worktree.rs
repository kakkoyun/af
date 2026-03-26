//! Git worktree operations.
//!
//! Creates and removes worktrees by shelling out to `git worktree`.
//! Worktree paths follow the convention: `<worktree_root>/<repo>/<branch>/`.

use std::path::Path;
use std::process::Command;

/// Errors from git worktree operations.
#[derive(Debug, thiserror::Error)]
pub enum WorktreeError {
    /// `git worktree add` failed.
    #[error("failed to create worktree at {path}: {detail}")]
    CreateFailed {
        /// Intended worktree path.
        path: String,
        /// stderr or exit code detail.
        detail: String,
    },
    /// `git worktree remove` failed.
    #[error("failed to remove worktree at {path}: {detail}")]
    RemoveFailed {
        /// Worktree path.
        path: String,
        /// stderr or exit code detail.
        detail: String,
    },
    /// Failed to spawn the git process.
    #[error("failed to run git: {0}")]
    Spawn(#[from] std::io::Error),
}

/// Create a new worktree, reusing an existing branch or creating a new one.
///
/// If a local branch named `branch` exists, checks it out into the worktree.
/// Otherwise, creates a new branch from `base_branch`.
///
/// Equivalent to:
/// ```sh
/// git worktree add <path> <branch>          # if branch exists
/// git worktree add -b <branch> <path> <base> # if branch is new
/// ```
pub fn create(
    git_root: &Path,
    worktree_path: &Path,
    branch: &str,
    base_branch: &str,
) -> Result<(), WorktreeError> {
    // Check if the branch already exists locally.
    let branch_exists = Command::new("git")
        .args(["show-ref", "--verify", "--quiet"])
        .arg(format!("refs/heads/{branch}"))
        .current_dir(git_root)
        .status()?
        .success();

    let output = if branch_exists {
        Command::new("git")
            .args(["worktree", "add"])
            .arg(worktree_path)
            .arg(branch)
            .current_dir(git_root)
            .output()?
    } else {
        Command::new("git")
            .args(["worktree", "add", "-b", branch])
            .arg(worktree_path)
            .arg(base_branch)
            .current_dir(git_root)
            .output()?
    };

    if output.status.success() {
        Ok(())
    } else {
        Err(WorktreeError::CreateFailed {
            path: worktree_path.display().to_string(),
            detail: String::from_utf8_lossy(&output.stderr).into_owned(),
        })
    }
}

/// Remove a worktree (forcefully).
///
/// Equivalent to `git worktree remove --force <path>`.
pub fn remove(git_root: &Path, worktree_path: &Path) -> Result<(), WorktreeError> {
    let output = Command::new("git")
        .args(["worktree", "remove", "--force"])
        .arg(worktree_path)
        .current_dir(git_root)
        .output()?;

    if output.status.success() {
        Ok(())
    } else {
        Err(WorktreeError::RemoveFailed {
            path: worktree_path.display().to_string(),
            detail: String::from_utf8_lossy(&output.stderr).into_owned(),
        })
    }
}

/// Delete a branch. If `force` is true, uses `-D` (force delete even if unmerged).
///
/// Equivalent to `git branch -d <branch>` or `git branch -D <branch>`.
pub fn delete_branch(git_root: &Path, branch: &str, force: bool) -> Result<(), WorktreeError> {
    let flag = if force { "-D" } else { "-d" };
    let output = Command::new("git")
        .args(["branch", flag, branch])
        .current_dir(git_root)
        .output()?;

    if output.status.success() {
        Ok(())
    } else {
        Err(WorktreeError::RemoveFailed {
            path: branch.to_owned(),
            detail: String::from_utf8_lossy(&output.stderr).into_owned(),
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::process::Command as StdCommand;
    use tempfile::TempDir;

    /// Create a bare-minimum git repo in a temp dir with one commit on `main`.
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

    #[test]
    fn test_create_worktree_new_branch() {
        let repo_dir = TempDir::new().unwrap();
        init_repo(repo_dir.path());

        let wt_dir = TempDir::new().unwrap();
        let wt_path = wt_dir.path().join("my-feature");

        create(repo_dir.path(), &wt_path, "my-feature", "main").unwrap();

        assert!(wt_path.exists());
        // Verify the branch was created
        let output = StdCommand::new("git")
            .args(["branch", "--list", "my-feature"])
            .current_dir(repo_dir.path())
            .output()
            .unwrap();
        let branches = String::from_utf8_lossy(&output.stdout);
        assert!(branches.contains("my-feature"));
    }

    #[test]
    fn test_create_worktree_existing_branch() {
        let repo_dir = TempDir::new().unwrap();
        init_repo(repo_dir.path());

        // Create the branch first
        StdCommand::new("git")
            .args(["branch", "existing-branch"])
            .current_dir(repo_dir.path())
            .status()
            .unwrap();

        let wt_dir = TempDir::new().unwrap();
        let wt_path = wt_dir.path().join("existing-branch");

        create(repo_dir.path(), &wt_path, "existing-branch", "main").unwrap();

        assert!(wt_path.exists());
    }

    #[test]
    fn test_remove_worktree() {
        let repo_dir = TempDir::new().unwrap();
        init_repo(repo_dir.path());

        let wt_dir = TempDir::new().unwrap();
        let wt_path = wt_dir.path().join("to-remove");

        create(repo_dir.path(), &wt_path, "to-remove", "main").unwrap();
        assert!(wt_path.exists());

        remove(repo_dir.path(), &wt_path).unwrap();
        assert!(!wt_path.exists());
    }

    #[test]
    fn test_remove_nonexistent_worktree_fails() {
        let repo_dir = TempDir::new().unwrap();
        init_repo(repo_dir.path());

        let result = remove(repo_dir.path(), Path::new("/tmp/nonexistent-af-wt-test"));
        assert!(result.is_err());
    }

    #[test]
    fn test_delete_branch_after_worktree_removal() {
        let repo_dir = TempDir::new().unwrap();
        init_repo(repo_dir.path());

        let wt_dir = TempDir::new().unwrap();
        let wt_path = wt_dir.path().join("delete-me");

        create(repo_dir.path(), &wt_path, "delete-me", "main").unwrap();
        remove(repo_dir.path(), &wt_path).unwrap();
        delete_branch(repo_dir.path(), "delete-me", true).unwrap();

        // Verify branch is gone
        let output = StdCommand::new("git")
            .args(["branch", "--list", "delete-me"])
            .current_dir(repo_dir.path())
            .output()
            .unwrap();
        let branches = String::from_utf8_lossy(&output.stdout);
        assert!(!branches.contains("delete-me"));
    }

    #[test]
    fn test_delete_branch_non_force_on_unmerged_fails() {
        let repo_dir = TempDir::new().unwrap();
        init_repo(repo_dir.path());

        // Create a branch with a commit not on main
        StdCommand::new("git")
            .args(["checkout", "-q", "-b", "unmerged"])
            .current_dir(repo_dir.path())
            .status()
            .unwrap();
        StdCommand::new("git")
            .args(["commit", "--allow-empty", "-q", "-m", "diverge"])
            .current_dir(repo_dir.path())
            .status()
            .unwrap();
        StdCommand::new("git")
            .args(["checkout", "-q", "main"])
            .current_dir(repo_dir.path())
            .status()
            .unwrap();

        let result = delete_branch(repo_dir.path(), "unmerged", false);
        assert!(result.is_err());
    }

    #[test]
    fn test_create_worktree_invalid_base_fails() {
        let repo_dir = TempDir::new().unwrap();
        init_repo(repo_dir.path());

        let wt_dir = TempDir::new().unwrap();
        let wt_path = wt_dir.path().join("bad-base");

        let result = create(repo_dir.path(), &wt_path, "bad-base", "nonexistent-branch");
        assert!(result.is_err());
    }
}
