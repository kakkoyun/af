//! Git operations: worktree management, branch operations, remote detection.
//!
//! All git operations shell out to the `git` binary via [`std::process::Command`].
//! This avoids linking `libgit2` and ensures behaviour matches the user's installed git.

pub mod branch;
pub mod remote;
