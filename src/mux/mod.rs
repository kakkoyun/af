//! Terminal multiplexer abstraction (ADR-002).
//!
//! Defines the [`Multiplexer`] trait for session management. The default
//! implementation uses tmux; a zellij implementation is planned.

pub mod tmux;

use std::path::Path;

/// Information about a multiplexer session.
#[derive(Debug, Clone)]
pub struct SessionInfo {
    /// Session name.
    pub name: String,
    /// Whether this session is attached.
    pub attached: bool,
}

/// Abstraction over terminal multiplexers (tmux, zellij).
///
/// All operations are fallible — the multiplexer binary may not be installed
/// or may return unexpected output. Implementations shell out to the
/// respective multiplexer's CLI.
pub trait Multiplexer {
    /// Check if the multiplexer binary is available on PATH.
    fn is_available(&self) -> bool;

    /// Check if we are currently inside a multiplexer session.
    fn is_inside_session(&self) -> bool;

    /// Get the current session name (if inside one).
    fn current_session_name(&self) -> anyhow::Result<Option<String>>;

    /// Create a new detached session with the given name and working directory.
    fn create_session(&self, name: &str, cwd: &Path) -> anyhow::Result<()>;

    /// Kill/destroy a session by name.
    fn kill_session(&self, name: &str) -> anyhow::Result<()>;

    /// Check if a session with the given name exists.
    fn session_exists(&self, name: &str) -> bool;

    /// Attach to or switch to a session.
    fn attach_or_switch(&self, name: &str) -> anyhow::Result<()>;

    /// Send keystrokes to a session's active pane.
    fn send_keys(&self, session: &str, keys: &str) -> anyhow::Result<()>;

    /// Set a session-scoped environment variable.
    fn set_env(&self, session: &str, key: &str, value: &str) -> anyhow::Result<()>;

    /// Get a session-scoped environment variable.
    fn get_env(&self, session: &str, key: &str) -> anyhow::Result<Option<String>>;

    /// Set a session option (e.g., `@AF_SESSION` marker).
    fn set_option(&self, session: &str, key: &str, value: &str) -> anyhow::Result<()>;

    /// List all sessions.
    fn list_sessions(&self) -> anyhow::Result<Vec<SessionInfo>>;

    /// Split window horizontally with a command.
    fn split_horizontal(&self, session: &str, cmd: &str, cwd: &Path) -> anyhow::Result<()>;
}
