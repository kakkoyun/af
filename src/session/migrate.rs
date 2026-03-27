//! Migration from `cf-sessions/*.env` to `af` session TOML format.
//!
//! The original `cf` tool stored session metadata as shell env files at
//! `~/.local/share/cf-sessions/<name>.env`. Each file contains `KEY=VALUE`
//! lines with `CF_*` prefixed variables. This module converts those files
//! to `af`'s `SessionState` TOML format.

use std::collections::HashMap;
use std::path::{Path, PathBuf};

use tracing::debug;

use super::types::{
    AgentSlot, AgentStatus, ExecutionInfo, ExecutionMode, PrInfo, SessionMeta, SessionState,
    SessionStatus, VersionInfo, WorktreeInfo,
};

/// Parse a `cf-sessions/*.env` file into a key-value map.
///
/// Lines are `KEY=VALUE` format. Empty lines and comments (`#`) are skipped.
/// Values may be optionally quoted with single or double quotes.
pub fn parse_env_file(content: &str) -> HashMap<String, String> {
    let mut map = HashMap::new();
    for line in content.lines() {
        let line = line.trim();
        if line.is_empty() || line.starts_with('#') {
            continue;
        }
        if let Some((key, value)) = line.split_once('=') {
            let key = key.trim().to_owned();
            let value = value.trim();
            // Strip optional surrounding quotes.
            let value = value
                .strip_prefix('"')
                .and_then(|v| v.strip_suffix('"'))
                .or_else(|| value.strip_prefix('\'').and_then(|v| v.strip_suffix('\'')))
                .unwrap_or(value)
                .to_owned();
            map.insert(key, value);
        }
    }
    map
}

/// Convert a parsed env map to a `SessionState`.
///
/// The session name is derived from the filename (without `.env` extension).
pub fn env_to_session_state(name: &str, env: &HashMap<String, String>) -> SessionState {
    let session_id = env.get("CF_SESSION_ID").cloned().unwrap_or_default();

    let worktree_path = env.get("CF_WORKTREE_PATH").cloned().unwrap_or_default();
    let branch = env.get("CF_BRANCH_NAME").cloned().unwrap_or_default();
    let base_branch = env.get("CF_BASE_BRANCH").cloned().unwrap_or_default();
    let git_root = env.get("CF_GIT_ROOT").cloned().unwrap_or_default();

    let is_workspace = env.get("CF_WORKSPACE_MODE").is_some_and(|v| v == "1");
    let is_bare = env.get("CF_VM_MODE").is_some_and(|v| v == "bare");

    let mode = if is_workspace {
        ExecutionMode::Workspace
    } else if is_bare {
        ExecutionMode::Bare
    } else {
        ExecutionMode::Local
    };

    let worktree = if !worktree_path.is_empty() && !is_workspace {
        Some(WorktreeInfo {
            path: worktree_path,
            branch,
            base_branch,
            git_root,
        })
    } else {
        None
    };

    debug!(name, ?mode, "converted cf env to session state");

    SessionState {
        session: SessionMeta {
            name: name.to_owned(),
            id: session_id,
            created_at: chrono::Utc::now(),
            status: SessionStatus::Active,
        },
        worktree,
        execution: ExecutionInfo {
            mode,
            multiplexer: "tmux".to_owned(),
            multiplexer_session: name.to_owned(),
        },
        agents: vec![AgentSlot {
            slot: "primary".to_owned(),
            provider: "claude".to_owned(),
            session_ids: vec![],
            pane: "0".to_owned(),
            status: AgentStatus::Stopped,
        }],
        pr: PrInfo::default(),
        versions: VersionInfo {
            af: "migrated".to_owned(),
            agent_config_hash: String::new(),
        },
    }
}

/// Default path to the cf-sessions directory.
pub fn cf_sessions_dir() -> Option<PathBuf> {
    dirs::data_dir().map(|d| d.join("cf-sessions"))
}

/// Discover all `.env` files in the cf-sessions directory.
pub fn discover_env_files(dir: &Path) -> Vec<PathBuf> {
    if !dir.exists() {
        return vec![];
    }
    let Ok(entries) = std::fs::read_dir(dir) else {
        return vec![];
    };
    let mut files: Vec<PathBuf> = entries
        .filter_map(Result::ok)
        .map(|e| e.path())
        .filter(|p| p.extension().is_some_and(|ext| ext == "env"))
        .collect();
    files.sort();
    files
}

/// Extract session name from an env file path.
pub fn session_name_from_path(path: &Path) -> Option<String> {
    path.file_stem()
        .and_then(|s| s.to_str())
        .map(ToOwned::to_owned)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_env_file_basic() {
        let content = "CF_WORKTREE_PATH=/tmp/worktree\nCF_BRANCH_NAME=fix/bug-42\n";
        let env = parse_env_file(content);
        assert_eq!(env.get("CF_WORKTREE_PATH").unwrap(), "/tmp/worktree");
        assert_eq!(env.get("CF_BRANCH_NAME").unwrap(), "fix/bug-42");
    }

    #[test]
    fn test_parse_env_file_quoted_values() {
        let content = "CF_GIT_ROOT=\"/home/user/Work/repo\"\nCF_SESSION_ID='abc-123'\n";
        let env = parse_env_file(content);
        assert_eq!(env.get("CF_GIT_ROOT").unwrap(), "/home/user/Work/repo");
        assert_eq!(env.get("CF_SESSION_ID").unwrap(), "abc-123");
    }

    #[test]
    fn test_parse_env_file_skips_comments_and_empty() {
        let content = "# comment\n\nCF_KEY=value\n  # another comment\n";
        let env = parse_env_file(content);
        assert_eq!(env.len(), 1);
        assert_eq!(env.get("CF_KEY").unwrap(), "value");
    }

    #[test]
    fn test_parse_env_file_empty() {
        let env = parse_env_file("");
        assert!(env.is_empty());
    }

    #[test]
    fn test_env_to_session_state_local_mode() {
        let mut env = HashMap::new();
        env.insert("CF_SESSION_ID".to_owned(), "uuid-123".to_owned());
        env.insert("CF_WORKTREE_PATH".to_owned(), "/tmp/wt".to_owned());
        env.insert("CF_BRANCH_NAME".to_owned(), "feat/x".to_owned());
        env.insert("CF_BASE_BRANCH".to_owned(), "main".to_owned());
        env.insert("CF_GIT_ROOT".to_owned(), "/tmp/repo".to_owned());

        let state = env_to_session_state("my-session", &env);
        assert_eq!(state.session.name, "my-session");
        assert_eq!(state.session.id, "uuid-123");
        assert_eq!(state.execution.mode, ExecutionMode::Local);
        assert!(state.worktree.is_some());
        let wt = state.worktree.unwrap();
        assert_eq!(wt.path, "/tmp/wt");
        assert_eq!(wt.branch, "feat/x");
        assert_eq!(wt.base_branch, "main");
    }

    #[test]
    fn test_env_to_session_state_workspace_mode() {
        let mut env = HashMap::new();
        env.insert("CF_WORKSPACE_MODE".to_owned(), "1".to_owned());

        let state = env_to_session_state("ws-session", &env);
        assert_eq!(state.execution.mode, ExecutionMode::Workspace);
        assert!(state.worktree.is_none());
    }

    #[test]
    fn test_env_to_session_state_bare_mode() {
        let mut env = HashMap::new();
        env.insert("CF_VM_MODE".to_owned(), "bare".to_owned());
        env.insert("CF_WORKTREE_PATH".to_owned(), "/tmp/wt".to_owned());
        env.insert("CF_BRANCH_NAME".to_owned(), "fix/y".to_owned());

        let state = env_to_session_state("bare-session", &env);
        assert_eq!(state.execution.mode, ExecutionMode::Bare);
    }

    #[test]
    fn test_env_to_session_state_migrated_version() {
        let env = HashMap::new();
        let state = env_to_session_state("test", &env);
        assert_eq!(state.versions.af, "migrated");
    }

    #[test]
    fn test_session_name_from_path() {
        let path = PathBuf::from("/home/user/.local/share/cf-sessions/my-session.env");
        assert_eq!(session_name_from_path(&path), Some("my-session".to_owned()));
    }

    #[test]
    fn test_session_name_from_path_no_extension() {
        let path = PathBuf::from("/tmp/nosuffix");
        assert_eq!(session_name_from_path(&path), Some("nosuffix".to_owned()));
    }

    #[test]
    fn test_discover_env_files_nonexistent_dir() {
        let files = discover_env_files(Path::new("/nonexistent/dir"));
        assert!(files.is_empty());
    }

    #[test]
    fn test_discover_env_files_in_temp_dir() {
        let dir = tempfile::TempDir::new().unwrap();
        std::fs::write(dir.path().join("session-a.env"), "CF_KEY=a").unwrap();
        std::fs::write(dir.path().join("session-b.env"), "CF_KEY=b").unwrap();
        std::fs::write(dir.path().join("not-env.txt"), "ignored").unwrap();

        let files = discover_env_files(dir.path());
        assert_eq!(files.len(), 2);
        assert!(files[0].to_string_lossy().contains("session-a"));
        assert!(files[1].to_string_lossy().contains("session-b"));
    }
}
