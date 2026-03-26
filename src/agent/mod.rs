//! AI agent provider abstraction (ADR-001).
//!
//! Defines the [`AgentProvider`] trait that encapsulates agent-specific behaviour
//! (launch commands, session resumption, permission bypass). Built-in providers:
//! Claude Code, pi, Codex, Gemini CLI, Amp.

pub mod claude;

use std::path::{Path, PathBuf};

/// Options for launching a new agent session.
#[derive(Debug, Clone)]
pub struct LaunchOpts {
    /// Deterministic session ID (UUID v5).
    pub session_id: String,
    /// Skip permission prompts (--yolo mode).
    pub yolo: bool,
}

/// Options for resuming an agent session.
#[derive(Debug, Clone)]
pub struct ResumeOpts {
    /// Whether to use yolo/unattended mode on resume.
    pub yolo: bool,
}

/// Abstraction over AI coding agents (ADR-001).
///
/// Each implementation encapsulates the CLI surface of a specific agent binary
/// (flags, session management, log paths). New agents are added by implementing
/// this trait on a new struct.
pub trait AgentProvider {
    /// Display name (e.g., "Claude Code").
    fn name(&self) -> &str;

    /// Binary name to invoke (e.g., "claude").
    fn binary(&self) -> &str;

    /// Check if the agent binary is available on PATH.
    fn is_available(&self) -> bool;

    /// Build the command arguments to launch a new session.
    /// Returns the full command as a `Vec` of strings (binary + args).
    fn launch_cmd(&self, opts: &LaunchOpts) -> Vec<String>;

    /// Build the command arguments to resume/continue a session.
    fn resume_cmd(&self, opts: &ResumeOpts) -> Vec<String>;

    /// Build the command for a PR-review session (if supported).
    fn pr_cmd(&self, pr_number: u64, opts: &LaunchOpts) -> Option<Vec<String>>;

    /// Locate the agent's own session log files for a given session ID.
    /// Used for analysis — af never deletes these files.
    fn session_log_paths(&self, session_id: &str, project_path: &Path) -> Vec<PathBuf>;
}
