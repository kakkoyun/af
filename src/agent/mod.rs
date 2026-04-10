//! AI agent provider abstraction (ADR-001).
//!
//! Defines the [`AgentProvider`] trait that encapsulates agent-specific behaviour
//! (launch commands, session resumption, permission bypass). Built-in providers:
//! Claude Code, pi, Codex, Gemini CLI, Amp, Copilot CLI.

pub mod amp;
pub mod claude;
pub mod codex;
pub mod copilot;
pub mod gemini;
pub mod pi;

use std::path::{Path, PathBuf};

/// Agent approval mode (ADR-012).
///
/// Controls how the agent handles permission prompts for tool use,
/// file edits, and destructive operations.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum ApprovalMode {
    /// Prompt for approval on tool use (agent default behaviour).
    #[default]
    Default,
    /// Auto-approve edits and safe tools, prompt for destructive operations.
    Auto,
    /// Skip all permission prompts (sandbox/unattended mode).
    Yolo,
}

/// Options for launching a new agent session.
#[derive(Debug, Clone)]
pub struct LaunchOpts {
    /// Deterministic session ID (UUID v5).
    pub session_id: String,
    /// Approval mode for permission prompts.
    pub approval_mode: ApprovalMode,
}

/// Options for resuming an agent session.
#[derive(Debug, Clone)]
pub struct ResumeOpts {
    /// Approval mode for permission prompts.
    pub approval_mode: ApprovalMode,
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

/// All known agent provider names.
pub const KNOWN_AGENTS: &[&str] = &["claude", "pi", "codex", "gemini", "amp", "copilot"];

/// Resolve an agent provider by name.
///
/// Returns `None` if the name is not recognized.
pub fn resolve(name: &str) -> Option<Box<dyn AgentProvider>> {
    match name {
        "claude" => Some(Box::new(claude::ClaudeProvider)),
        "pi" => Some(Box::new(pi::PiProvider)),
        "codex" => Some(Box::new(codex::CodexProvider)),
        "gemini" => Some(Box::new(gemini::GeminiProvider)),
        "amp" => Some(Box::new(amp::AmpProvider)),
        "copilot" => Some(Box::new(copilot::CopilotProvider)),
        _ => None,
    }
}

/// Find the first available agent from a priority list.
///
/// Default priority: claude > pi > codex > gemini > amp > copilot.
pub fn first_available() -> Option<Box<dyn AgentProvider>> {
    for &name in KNOWN_AGENTS {
        if let Some(provider) = resolve(name) {
            if provider.is_available() {
                return Some(provider);
            }
        }
    }
    None
}
