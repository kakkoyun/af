//! Merge detection for garbage collection.
//!
//! Determines if a worktree branch has been merged into the main branch using
//! three strategies: PR state (via `gh`), git ancestry, and squash-merge
//! fingerprint matching.

use std::path::Path;
use std::process::Command;

use super::branch;

/// The merge status of a branch.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum MergeStatus {
    /// Branch has been merged (regular or squash).
    Merged,
    /// Branch PR was closed without merging.
    Closed,
    /// Branch is still active / unmerged.
    Open,
}

/// Detect the merge status of a branch using multiple strategies.
///
/// Priority:
/// 1. GitHub PR state via `gh` (if available)
/// 2. Git ancestry (`merge-base --is-ancestor`)
/// 3. Squash-merge fingerprint matching
/// 4. Fallback: [`MergeStatus::Open`]
pub fn detect_merge_status(git_root: &Path, branch: &str, main_branch: &str) -> MergeStatus {
    // Strategy 1: GitHub PR state.
    if let Some(status) = pr_state(git_root, branch) {
        return status;
    }

    // Strategy 2: Git ancestry.
    if is_ancestor(git_root, branch, main_branch) {
        return MergeStatus::Merged;
    }

    // Strategy 3: Squash-merge fingerprint.
    if is_squash_merged(git_root, branch, main_branch) {
        return MergeStatus::Merged;
    }

    MergeStatus::Open
}

/// Check GitHub PR state via the `gh` CLI.
///
/// Returns `None` if `gh` is not available or the branch has no PR.
fn pr_state(git_root: &Path, branch: &str) -> Option<MergeStatus> {
    let output = Command::new("gh")
        .args(["pr", "view", branch, "--json", "state", "--jq", ".state"])
        .current_dir(git_root)
        .output()
        .ok()?;

    if !output.status.success() {
        return None;
    }

    let state = String::from_utf8_lossy(&output.stdout)
        .trim()
        .to_uppercase();
    match state.as_str() {
        "MERGED" => Some(MergeStatus::Merged),
        "CLOSED" => Some(MergeStatus::Closed),
        "OPEN" => Some(MergeStatus::Open),
        _ => None,
    }
}

/// Check if `branch` is an ancestor of `main_branch` (regular merge detection).
fn is_ancestor(git_root: &Path, branch: &str, main_branch: &str) -> bool {
    Command::new("git")
        .args(["merge-base", "--is-ancestor", branch, main_branch])
        .current_dir(git_root)
        .status()
        .is_ok_and(|s| s.success())
}

/// Detect if a branch was squash-merged by comparing diff fingerprints.
///
/// Compares the cumulative diff of the feature branch against each non-merge
/// commit on main since the merge-base. If any commit has the same diff
/// fingerprint, the branch was squash-merged.
///
/// This mirrors `cf`'s `_cf_is_squash_merged` heuristic.
fn is_squash_merged(git_root: &Path, branch: &str, main_branch: &str) -> bool {
    // Get the merge base.
    let merge_base = Command::new("git")
        .args(["merge-base", main_branch, branch])
        .current_dir(git_root)
        .output()
        .ok()
        .filter(|o| o.status.success())
        .map(|o| String::from_utf8_lossy(&o.stdout).trim().to_owned());

    let Some(merge_base) = merge_base else {
        return false;
    };

    // Get the combined diff of the feature branch.
    let feature_diff = Command::new("git")
        .args(["diff", &merge_base, branch])
        .current_dir(git_root)
        .output()
        .ok()
        .filter(|o| o.status.success())
        .map(|o| o.stdout);

    let Some(feature_diff) = feature_diff else {
        return false;
    };

    if feature_diff.is_empty() {
        return false;
    }

    let feature_hash = simple_hash(&feature_diff);

    // Compare against each non-merge commit on main since merge-base.
    let commits = Command::new("git")
        .args([
            "log",
            "--format=%H",
            "--no-merges",
            &format!("{merge_base}..{main_branch}"),
        ])
        .current_dir(git_root)
        .output()
        .ok()
        .filter(|o| o.status.success())
        .map(|o| String::from_utf8_lossy(&o.stdout).to_string());

    let Some(commits) = commits else {
        return false;
    };

    for commit in commits.lines().take(100) {
        let commit = commit.trim();
        if commit.is_empty() {
            continue;
        }

        let parent = format!("{commit}^");
        let commit_diff = Command::new("git")
            .args(["diff", &parent, commit])
            .current_dir(git_root)
            .output()
            .ok()
            .filter(|o| o.status.success())
            .map(|o| o.stdout);

        if let Some(diff) = commit_diff {
            if simple_hash(&diff) == feature_hash {
                return true;
            }
        }
    }

    false
}

/// Simple hash of a byte slice for fingerprint comparison.
/// Uses a basic FNV-like hash — not cryptographic, just for equality checks.
fn simple_hash(data: &[u8]) -> u64 {
    let mut hash: u64 = 0xcbf2_9ce4_8422_2325;
    for &byte in data {
        hash ^= u64::from(byte);
        hash = hash.wrapping_mul(0x0100_0000_01b3);
    }
    hash
}

/// Detect the main branch for a worktree directory.
///
/// Reads local branches from the worktree and applies the standard
/// main/master/trunk detection.
pub fn detect_main_for_worktree(wt_path: &Path) -> String {
    let output = Command::new("git")
        .args(["branch", "--format=%(refname:short)"])
        .current_dir(wt_path)
        .output();

    let branches: Vec<String> = output
        .ok()
        .filter(|o| o.status.success())
        .map(|o| {
            String::from_utf8_lossy(&o.stdout)
                .lines()
                .map(|l| l.trim().to_owned())
                .filter(|l| !l.is_empty())
                .collect()
        })
        .unwrap_or_default();

    let refs: Vec<&str> = branches.iter().map(String::as_str).collect();
    branch::detect_main_branch(&refs).to_owned()
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
            .args(["config", "commit.gpgsign", "false"])
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
    fn test_simple_hash_deterministic() {
        let data = b"hello world";
        assert_eq!(simple_hash(data), simple_hash(data));
    }

    #[test]
    fn test_simple_hash_different_inputs_differ() {
        assert_ne!(simple_hash(b"hello"), simple_hash(b"world"));
    }

    #[test]
    fn test_is_ancestor_after_merge() {
        let dir = TempDir::new().unwrap();
        init_repo(dir.path());

        StdCommand::new("git")
            .args(["checkout", "-q", "-b", "feature"])
            .current_dir(dir.path())
            .status()
            .unwrap();
        StdCommand::new("git")
            .args(["commit", "--allow-empty", "-q", "-m", "feature work"])
            .current_dir(dir.path())
            .status()
            .unwrap();
        StdCommand::new("git")
            .args(["checkout", "-q", "main"])
            .current_dir(dir.path())
            .status()
            .unwrap();
        StdCommand::new("git")
            .args(["merge", "--no-ff", "-q", "-m", "merge", "feature"])
            .current_dir(dir.path())
            .status()
            .unwrap();

        assert!(is_ancestor(dir.path(), "feature", "main"));
    }

    #[test]
    fn test_is_ancestor_unmerged_returns_false() {
        let dir = TempDir::new().unwrap();
        init_repo(dir.path());

        StdCommand::new("git")
            .args(["checkout", "-q", "-b", "unmerged"])
            .current_dir(dir.path())
            .status()
            .unwrap();
        StdCommand::new("git")
            .args(["commit", "--allow-empty", "-q", "-m", "diverge"])
            .current_dir(dir.path())
            .status()
            .unwrap();
        StdCommand::new("git")
            .args(["checkout", "-q", "main"])
            .current_dir(dir.path())
            .status()
            .unwrap();

        assert!(!is_ancestor(dir.path(), "unmerged", "main"));
    }

    #[test]
    fn test_squash_merge_detection() {
        let dir = TempDir::new().unwrap();
        init_repo(dir.path());

        // Create a feature branch with real file changes.
        StdCommand::new("git")
            .args(["checkout", "-q", "-b", "feature-squash"])
            .current_dir(dir.path())
            .status()
            .unwrap();
        std::fs::write(dir.path().join("feature.txt"), "hello\n").unwrap();
        StdCommand::new("git")
            .args(["add", "feature.txt"])
            .current_dir(dir.path())
            .status()
            .unwrap();
        StdCommand::new("git")
            .args(["commit", "-q", "-m", "add feature"])
            .current_dir(dir.path())
            .status()
            .unwrap();

        // Squash merge onto main.
        StdCommand::new("git")
            .args(["checkout", "-q", "main"])
            .current_dir(dir.path())
            .status()
            .unwrap();
        StdCommand::new("git")
            .args(["merge", "--squash", "-q", "feature-squash"])
            .current_dir(dir.path())
            .status()
            .unwrap();
        StdCommand::new("git")
            .args(["commit", "-q", "-m", "squash merge feature"])
            .current_dir(dir.path())
            .status()
            .unwrap();

        assert!(is_squash_merged(dir.path(), "feature-squash", "main"));
    }

    #[test]
    fn test_squash_merge_unmerged_returns_false() {
        let dir = TempDir::new().unwrap();
        init_repo(dir.path());

        StdCommand::new("git")
            .args(["checkout", "-q", "-b", "not-squashed"])
            .current_dir(dir.path())
            .status()
            .unwrap();
        std::fs::write(dir.path().join("other.txt"), "different\n").unwrap();
        StdCommand::new("git")
            .args(["add", "other.txt"])
            .current_dir(dir.path())
            .status()
            .unwrap();
        StdCommand::new("git")
            .args(["commit", "-q", "-m", "unrelated"])
            .current_dir(dir.path())
            .status()
            .unwrap();
        StdCommand::new("git")
            .args(["checkout", "-q", "main"])
            .current_dir(dir.path())
            .status()
            .unwrap();

        assert!(!is_squash_merged(dir.path(), "not-squashed", "main"));
    }

    #[test]
    fn test_detect_merge_status_merged_ancestor() {
        let dir = TempDir::new().unwrap();
        init_repo(dir.path());

        StdCommand::new("git")
            .args(["checkout", "-q", "-b", "merged-feature"])
            .current_dir(dir.path())
            .status()
            .unwrap();
        StdCommand::new("git")
            .args(["commit", "--allow-empty", "-q", "-m", "work"])
            .current_dir(dir.path())
            .status()
            .unwrap();
        StdCommand::new("git")
            .args(["checkout", "-q", "main"])
            .current_dir(dir.path())
            .status()
            .unwrap();
        StdCommand::new("git")
            .args(["merge", "--no-ff", "-q", "-m", "merge", "merged-feature"])
            .current_dir(dir.path())
            .status()
            .unwrap();

        assert_eq!(
            detect_merge_status(dir.path(), "merged-feature", "main"),
            MergeStatus::Merged
        );
    }

    #[test]
    fn test_detect_merge_status_open() {
        let dir = TempDir::new().unwrap();
        init_repo(dir.path());

        StdCommand::new("git")
            .args(["checkout", "-q", "-b", "open-feature"])
            .current_dir(dir.path())
            .status()
            .unwrap();
        StdCommand::new("git")
            .args(["commit", "--allow-empty", "-q", "-m", "diverge"])
            .current_dir(dir.path())
            .status()
            .unwrap();
        StdCommand::new("git")
            .args(["checkout", "-q", "main"])
            .current_dir(dir.path())
            .status()
            .unwrap();

        assert_eq!(
            detect_merge_status(dir.path(), "open-feature", "main"),
            MergeStatus::Open
        );
    }

    #[test]
    fn test_detect_main_for_worktree() {
        let dir = TempDir::new().unwrap();
        init_repo(dir.path());

        assert_eq!(detect_main_for_worktree(dir.path()), "main");
    }
}
