//! `OpenAI` Codex agent provider.
//!
//! Implements [`AgentProvider`] for `OpenAI`'s Codex CLI.
//! Codex supports `--full-auto` for sandboxed automatic execution,
//! `resume` subcommand for session continuation, and `--session-id` for
//! deterministic sessions.
//!
//! # OS sandbox (ADR-028)
//!
//! [`AgentSandbox::Os`] maps to `-s workspace-write`. Codex handles the OS
//! detail: Seatbelt on macOS, bubblewrap/Landlock on Linux.
//!
//! # Phase IV integration notes
//!
//! `AgentSandbox` is defined here temporarily (Phase III). In Phase IV it moves
//! to `src/agent/mod.rs` as part of `LaunchOpts`. The per-agent files that
//! re-export it (`pub use crate::agent::codex::AgentSandbox`) will be updated to
//! point at the canonical location instead.

use std::path::{Path, PathBuf};

use crate::agent::{AgentProvider, ApprovalMode, LaunchOpts, ResumeOpts};

/// Per-agent OS sandbox mode (ADR-028).
///
/// Controls whether `af` asks the agent binary to enable its own OS-level
/// sandbox. This is orthogonal to af's VM/container isolation layer
/// (`--sandbox` / `provider/slicer`, `provider/docker`).
///
/// # Temporary location
///
/// Defined here for Phase III. Phase IV moves this to `src/agent/mod.rs` and
/// adds a `sandbox: AgentSandbox` field to `LaunchOpts`.
///
/// # Default
///
/// `None`. The CLI layer (Phase IV) will make `Os` the effective default when
/// the selected agent supports it.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum AgentSandbox {
    /// No agent-level OS sandbox. Agent is launched without any additional
    /// sandbox flag.
    #[default]
    None,
    /// Request the agent's native OS sandbox.
    ///
    /// - **codex:** appends `-s workspace-write` (Seatbelt on macOS,
    ///   bubblewrap/Landlock on Linux — codex resolves the OS detail).
    /// - **claude:** no-op. Claude defers sandboxing to the caller; it has
    ///   a built-in file-approval flow that is the functional equivalent.
    /// - **amp, gemini, copilot, pi:** no OS sandbox flag available; degrades
    ///   to `None` with a `tracing::info!` log.
    Os,
}

/// `OpenAI` Codex agent provider.
///
/// Shells out to the `codex` binary. Supports:
/// - `--full-auto` for sandboxed automatic execution (yolo equivalent)
/// - `codex resume <session-id>` for resuming sessions
/// - `codex resume --last` for continuing the most recent session
pub struct CodexProvider;

/// Apply the OS sandbox policy to an in-progress Codex argv vector.
///
/// Must be called **after** all approval-mode flags and **before** any
/// subcommand token (e.g. `resume`).
///
/// | `sandbox`            | effect                       |
/// |----------------------|------------------------------|
/// | `AgentSandbox::None` | no-op                        |
/// | `AgentSandbox::Os`   | appends `-s workspace-write` |
pub fn apply_sandbox(cmd: &mut Vec<String>, sandbox: AgentSandbox) {
    if sandbox == AgentSandbox::Os {
        cmd.push("-s".to_owned());
        cmd.push("workspace-write".to_owned());
    }
}

impl AgentProvider for CodexProvider {
    fn name(&self) -> &'static str {
        "Codex"
    }

    fn binary(&self) -> &'static str {
        "codex"
    }

    fn is_available(&self) -> bool {
        which::which("codex").is_ok()
    }

    fn launch_cmd(&self, opts: &LaunchOpts) -> Vec<String> {
        let mut cmd = vec!["codex".to_owned()];
        match opts.approval_mode {
            ApprovalMode::Default => {}
            ApprovalMode::Auto => {
                cmd.push("--ask-for-approval".to_owned());
                cmd.push("on-request".to_owned());
            }
            ApprovalMode::Yolo => {
                cmd.push("--full-auto".to_owned());
                cmd.push("--ask-for-approval".to_owned());
                cmd.push("never".to_owned());
            }
        }
        // Codex doesn't have --session-id on launch; af tracks the ID externally.
        let _ = &opts.session_id;
        cmd
    }

    fn resume_cmd(&self, opts: &ResumeOpts) -> Vec<String> {
        let mut cmd = vec!["codex".to_owned()];
        match opts.approval_mode {
            ApprovalMode::Default => {}
            ApprovalMode::Auto => {
                cmd.push("--ask-for-approval".to_owned());
                cmd.push("on-request".to_owned());
            }
            ApprovalMode::Yolo => {
                cmd.push("--full-auto".to_owned());
                cmd.push("--ask-for-approval".to_owned());
                cmd.push("never".to_owned());
            }
        }
        cmd.push("resume".to_owned());
        cmd.push("--last".to_owned());
        cmd
    }

    fn pr_cmd(&self, _pr_number: u64, _opts: &LaunchOpts) -> Option<Vec<String>> {
        // Codex has `codex review` but not --from-pr.
        None
    }

    fn session_log_paths(&self, _session_id: &str, _project_path: &Path) -> Vec<PathBuf> {
        // Codex stores sessions under ~/.codex/ but the structure is not documented.
        vec![]
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_codex_name_and_binary() {
        let p = CodexProvider;
        assert_eq!(p.name(), "Codex");
        assert_eq!(p.binary(), "codex");
    }

    #[test]
    fn test_codex_launch_cmd_default() {
        let p = CodexProvider;
        let opts = LaunchOpts {
            session_id: "x".to_owned(),
            approval_mode: ApprovalMode::Default,
        };
        assert_eq!(p.launch_cmd(&opts), vec!["codex"]);
    }

    #[test]
    fn test_codex_launch_cmd_auto() {
        let p = CodexProvider;
        let opts = LaunchOpts {
            session_id: "x".to_owned(),
            approval_mode: ApprovalMode::Auto,
        };
        assert_eq!(
            p.launch_cmd(&opts),
            vec!["codex", "--ask-for-approval", "on-request"]
        );
    }

    #[test]
    fn test_codex_launch_cmd_yolo() {
        let p = CodexProvider;
        let opts = LaunchOpts {
            session_id: "x".to_owned(),
            approval_mode: ApprovalMode::Yolo,
        };
        assert_eq!(
            p.launch_cmd(&opts),
            vec!["codex", "--full-auto", "--ask-for-approval", "never"]
        );
    }

    #[test]
    fn test_codex_resume_cmd_default() {
        let p = CodexProvider;
        let cmd = p.resume_cmd(&ResumeOpts {
            approval_mode: ApprovalMode::Default,
        });
        assert_eq!(cmd, vec!["codex", "resume", "--last"]);
    }

    #[test]
    fn test_codex_resume_cmd_auto() {
        let p = CodexProvider;
        let cmd = p.resume_cmd(&ResumeOpts {
            approval_mode: ApprovalMode::Auto,
        });
        assert_eq!(
            cmd,
            vec![
                "codex",
                "--ask-for-approval",
                "on-request",
                "resume",
                "--last"
            ]
        );
    }

    #[test]
    fn test_codex_resume_cmd_yolo() {
        let p = CodexProvider;
        let cmd = p.resume_cmd(&ResumeOpts {
            approval_mode: ApprovalMode::Yolo,
        });
        assert_eq!(
            cmd,
            vec![
                "codex",
                "--full-auto",
                "--ask-for-approval",
                "never",
                "resume",
                "--last"
            ]
        );
    }

    #[test]
    fn test_codex_pr_cmd_returns_none() {
        let p = CodexProvider;
        let opts = LaunchOpts {
            session_id: "x".to_owned(),
            approval_mode: ApprovalMode::Default,
        };
        assert!(p.pr_cmd(42, &opts).is_none());
    }

    #[test]
    fn test_codex_is_available() {
        let p = CodexProvider;
        // Result depends on environment.
        let _available = p.is_available();
    }

    // --- AgentSandbox tests (ADR-028) ---

    #[test]
    fn test_agent_sandbox_default_is_none() {
        assert_eq!(AgentSandbox::default(), AgentSandbox::None);
    }

    #[test]
    fn test_apply_sandbox_none_is_noop() {
        let mut cmd = vec!["codex".to_owned()];
        apply_sandbox(&mut cmd, AgentSandbox::None);
        assert_eq!(cmd, vec!["codex"]);
    }

    #[test]
    fn test_apply_sandbox_os_appends_workspace_write() {
        let mut cmd = vec!["codex".to_owned()];
        apply_sandbox(&mut cmd, AgentSandbox::Os);
        assert_eq!(cmd, vec!["codex", "-s", "workspace-write"]);
    }

    #[test]
    fn test_apply_sandbox_os_after_approval_flags() {
        // Sandbox flag appears after approval-mode flags, before any subcommand.
        let mut cmd = vec![
            "codex".to_owned(),
            "--full-auto".to_owned(),
            "--ask-for-approval".to_owned(),
            "never".to_owned(),
        ];
        apply_sandbox(&mut cmd, AgentSandbox::Os);
        assert_eq!(
            cmd,
            vec![
                "codex",
                "--full-auto",
                "--ask-for-approval",
                "never",
                "-s",
                "workspace-write",
            ]
        );
    }
}
