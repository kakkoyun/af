//! Session state persistence — TOML files on disk (ADR-006, ADR-011).
//!
//! Each active session lives at `~/.local/share/af/sessions/<name>/state.toml`.
//! Completed sessions are archived to `~/.local/share/af/archive/<name>/state.toml`.

use std::path::{Path, PathBuf};

use super::types::SessionState;

/// Errors from session store operations.
#[derive(Debug, thiserror::Error)]
pub enum StoreError {
    /// Failed to read session state from disk.
    #[error("failed to read session state at {path}: {source}")]
    Read {
        /// Path that failed.
        path: PathBuf,
        /// Underlying I/O error.
        source: std::io::Error,
    },
    /// Failed to write session state to disk.
    #[error("failed to write session state at {path}: {source}")]
    Write {
        /// Path that failed.
        path: PathBuf,
        /// Underlying I/O error.
        source: std::io::Error,
    },
    /// Failed to parse TOML content.
    #[error("failed to parse session state at {path}: {source}")]
    Parse {
        /// Path that failed.
        path: PathBuf,
        /// Underlying parse error.
        source: toml::de::Error,
    },
    /// Failed to serialize to TOML.
    #[error("failed to serialize session state: {source}")]
    Serialize {
        /// Underlying serialization error.
        source: toml::ser::Error,
    },
    /// Session not found.
    #[error("session not found: {name}")]
    NotFound {
        /// Session name.
        name: String,
    },
}

/// Filesystem-based session store.
///
/// Sessions are stored as directories under a root path, each containing
/// a `state.toml` file.
#[derive(Debug, Clone)]
pub struct SessionStore {
    /// Root directory for active sessions (e.g., `~/.local/share/af/sessions`).
    sessions_dir: PathBuf,
    /// Root directory for archived sessions (e.g., `~/.local/share/af/archive`).
    archive_dir: PathBuf,
}

impl SessionStore {
    /// Create a store rooted at the given data directory.
    ///
    /// The directory structure will be:
    /// ```text
    /// <data_dir>/
    /// ├── sessions/<name>/state.toml
    /// └── archive/<name>/state.toml
    /// ```
    pub fn new(data_dir: &Path) -> Self {
        Self {
            sessions_dir: data_dir.join("sessions"),
            archive_dir: data_dir.join("archive"),
        }
    }

    /// Create a store using the default XDG data directory.
    ///
    /// Returns `None` if the home directory cannot be determined.
    pub fn default_location() -> Option<Self> {
        dirs::data_dir().map(|d| Self::new(&d.join("af")))
    }

    /// Path to a session's state file.
    fn state_path(&self, name: &str) -> PathBuf {
        self.sessions_dir.join(name).join("state.toml")
    }

    /// Path to a session's directory.
    fn session_dir(&self, name: &str) -> PathBuf {
        self.sessions_dir.join(name)
    }

    /// Path to an archived session's directory.
    fn archive_session_dir(&self, name: &str) -> PathBuf {
        self.archive_dir.join(name)
    }

    /// Save a session state to disk. Creates directories as needed.
    pub fn save(&self, state: &SessionState) -> Result<(), StoreError> {
        let dir = self.session_dir(&state.session.name);
        std::fs::create_dir_all(&dir).map_err(|e| StoreError::Write {
            path: dir.clone(),
            source: e,
        })?;

        let toml_str =
            toml::to_string_pretty(state).map_err(|e| StoreError::Serialize { source: e })?;

        let path = self.state_path(&state.session.name);
        std::fs::write(&path, toml_str).map_err(|e| StoreError::Write { path, source: e })
    }

    /// Load a session state by name.
    pub fn load(&self, name: &str) -> Result<SessionState, StoreError> {
        let path = self.state_path(name);
        if !path.exists() {
            return Err(StoreError::NotFound {
                name: name.to_owned(),
            });
        }
        let content = std::fs::read_to_string(&path).map_err(|e| StoreError::Read {
            path: path.clone(),
            source: e,
        })?;
        toml::from_str(&content).map_err(|e| StoreError::Parse { path, source: e })
    }

    /// Delete a session from disk (removes the entire session directory).
    pub fn delete(&self, name: &str) -> Result<(), StoreError> {
        let dir = self.session_dir(name);
        if dir.exists() {
            std::fs::remove_dir_all(&dir).map_err(|e| StoreError::Write {
                path: dir,
                source: e,
            })?;
        }
        Ok(())
    }

    /// List all active session names.
    pub fn list(&self) -> Result<Vec<String>, StoreError> {
        if !self.sessions_dir.exists() {
            return Ok(Vec::new());
        }
        let mut names = Vec::new();
        let entries = std::fs::read_dir(&self.sessions_dir).map_err(|e| StoreError::Read {
            path: self.sessions_dir.clone(),
            source: e,
        })?;
        for entry in entries {
            let entry = entry.map_err(|e| StoreError::Read {
                path: self.sessions_dir.clone(),
                source: e,
            })?;
            if entry.path().join("state.toml").exists() {
                if let Some(name) = entry.file_name().to_str() {
                    names.push(name.to_owned());
                }
            }
        }
        names.sort();
        Ok(names)
    }

    /// Move a session to the archive directory.
    pub fn archive(&self, name: &str) -> Result<(), StoreError> {
        let src = self.session_dir(name);
        if !src.exists() {
            return Err(StoreError::NotFound {
                name: name.to_owned(),
            });
        }
        let dst = self.archive_session_dir(name);
        if let Some(parent) = dst.parent() {
            std::fs::create_dir_all(parent).map_err(|e| StoreError::Write {
                path: parent.to_path_buf(),
                source: e,
            })?;
        }
        std::fs::rename(&src, &dst).map_err(|e| StoreError::Write {
            path: dst,
            source: e,
        })
    }

    /// Check if a session exists (active, not archived).
    pub fn exists(&self, name: &str) -> bool {
        self.state_path(name).exists()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::session::types::*;
    use chrono::{TimeZone, Utc};
    use tempfile::TempDir;

    fn sample_state(name: &str) -> SessionState {
        SessionState {
            session: SessionMeta {
                name: name.to_owned(),
                id: String::from("550e8400-e29b-41d4-a716-446655440000"),
                created_at: Utc.with_ymd_and_hms(2026, 3, 26, 14, 30, 0).unwrap(),
                status: SessionStatus::Active,
            },
            worktree: Some(WorktreeInfo {
                path: format!("/tmp/worktrees/{name}"),
                branch: format!("kakkoyun/{name}"),
                base_branch: String::from("main"),
                git_root: String::from("/tmp/repo"),
            }),
            execution: ExecutionInfo {
                mode: ExecutionMode::Local,
                multiplexer: String::from("tmux"),
                multiplexer_session: name.to_owned(),
            },
            agents: vec![AgentSlot {
                slot: String::from("primary"),
                provider: String::from("claude"),
                session_ids: vec![String::from("550e8400-e29b-41d4-a716-446655440000")],
                pane: String::from("0"),
                status: AgentStatus::Running,
            }],
            pr: PrInfo::default(),
            versions: VersionInfo {
                af: String::from("0.1.0"),
                agent_config_hash: String::from("abcd1234"),
            },
        }
    }

    #[test]
    fn test_save_and_load_roundtrip() {
        let dir = TempDir::new().unwrap();
        let store = SessionStore::new(dir.path());
        let state = sample_state("test-session");

        store.save(&state).unwrap();
        let loaded = store.load("test-session").unwrap();
        assert_eq!(state, loaded);
    }

    #[test]
    fn test_load_nonexistent_returns_not_found() {
        let dir = TempDir::new().unwrap();
        let store = SessionStore::new(dir.path());

        let result = store.load("nonexistent");
        assert!(result.is_err());
        assert!(matches!(result.unwrap_err(), StoreError::NotFound { .. }));
    }

    #[test]
    fn test_delete_removes_session() {
        let dir = TempDir::new().unwrap();
        let store = SessionStore::new(dir.path());
        let state = sample_state("to-delete");

        store.save(&state).unwrap();
        assert!(store.exists("to-delete"));

        store.delete("to-delete").unwrap();
        assert!(!store.exists("to-delete"));
    }

    #[test]
    fn test_delete_nonexistent_is_ok() {
        let dir = TempDir::new().unwrap();
        let store = SessionStore::new(dir.path());

        // Should not error
        store.delete("nonexistent").unwrap();
    }

    #[test]
    fn test_list_returns_sorted_names() {
        let dir = TempDir::new().unwrap();
        let store = SessionStore::new(dir.path());

        store.save(&sample_state("charlie")).unwrap();
        store.save(&sample_state("alice")).unwrap();
        store.save(&sample_state("bob")).unwrap();

        let names = store.list().unwrap();
        assert_eq!(names, vec!["alice", "bob", "charlie"]);
    }

    #[test]
    fn test_list_empty_store_returns_empty_vec() {
        let dir = TempDir::new().unwrap();
        let store = SessionStore::new(dir.path());

        let names = store.list().unwrap();
        assert!(names.is_empty());
    }

    #[test]
    fn test_exists_returns_true_for_saved_session() {
        let dir = TempDir::new().unwrap();
        let store = SessionStore::new(dir.path());

        assert!(!store.exists("test"));
        store.save(&sample_state("test")).unwrap();
        assert!(store.exists("test"));
    }

    #[test]
    fn test_archive_moves_session() {
        let dir = TempDir::new().unwrap();
        let store = SessionStore::new(dir.path());
        let state = sample_state("to-archive");

        store.save(&state).unwrap();
        assert!(store.exists("to-archive"));

        store.archive("to-archive").unwrap();
        assert!(!store.exists("to-archive"));

        // Verify the archive directory exists
        assert!(store.archive_session_dir("to-archive").exists());
    }

    #[test]
    fn test_archive_nonexistent_returns_not_found() {
        let dir = TempDir::new().unwrap();
        let store = SessionStore::new(dir.path());

        let result = store.archive("nonexistent");
        assert!(result.is_err());
        assert!(matches!(result.unwrap_err(), StoreError::NotFound { .. }));
    }

    #[test]
    fn test_save_overwrites_existing() {
        let dir = TempDir::new().unwrap();
        let store = SessionStore::new(dir.path());

        let mut state = sample_state("overwrite-me");
        store.save(&state).unwrap();

        state.session.status = SessionStatus::Completed;
        store.save(&state).unwrap();

        let loaded = store.load("overwrite-me").unwrap();
        assert_eq!(loaded.session.status, SessionStatus::Completed);
    }
}
