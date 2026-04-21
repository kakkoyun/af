//! pi coding agent provider.
//!
//! Implements [`AgentProvider`] for pi (`@mariozechner/pi-coding-agent`).
//! pi supports session continuation, session files, and fork-based resume.
//!
//! # OS sandbox (ADR-028)
//!
//! pi has no CLI sandbox flag. [`AgentSandbox::Os`] degrades silently to
//! [`AgentSandbox::None`] with a `tracing::info!` log.

pub use crate::agent::codex::AgentSandbox;

use std::path::{Path, PathBuf};

use crate::agent::{AgentProvider, LaunchOpts, ResumeOpts};

/// pi coding agent provider.
///
/// Shells out to the `pi` binary. Supports:
/// - `--session <path>` for specific session files
/// - `--continue` / `-c` for resuming the previous session
/// - No native session-ID concept — uses session file paths
/// - No yolo/unattended mode equivalent
pub struct PiProvider;

/// Apply the OS sandbox policy for pi — degrades to none with an info log.
///
/// pi exposes no OS-level sandbox flag. When [`AgentSandbox::Os`] is
/// requested, `af` logs an informational message and proceeds without a
/// sandbox flag.
///
/// | `sandbox`            | effect                                         |
/// |----------------------|------------------------------------------------|
/// | `AgentSandbox::None` | no-op                                          |
/// | `AgentSandbox::Os`   | no-op + `tracing::info!` degrade-to-none log   |
pub fn apply_sandbox(_cmd: &mut Vec<String>, sandbox: AgentSandbox) {
    if sandbox == AgentSandbox::Os {
        tracing::info!(
            agent = "pi",
            "agent pi does not support OS sandbox; running without"
        );
    }
}

impl AgentProvider for PiProvider {
    fn name(&self) -> &'static str {
        "pi"
    }

    fn binary(&self) -> &'static str {
        "pi"
    }

    fn is_available(&self) -> bool {
        which::which("pi").is_ok()
    }

    fn launch_cmd(&self, opts: &LaunchOpts) -> Vec<String> {
        // pi doesn't have a --session-id flag like Claude.
        // We launch it plain; session tracking is by pi's own file-based sessions.
        // The session_id is stored in af's metadata for correlation only.
        let _ = opts;
        vec!["pi".to_owned()]
    }

    fn resume_cmd(&self, _opts: &ResumeOpts) -> Vec<String> {
        vec!["pi".to_owned(), "--continue".to_owned()]
    }

    fn pr_cmd(&self, _pr_number: u64, _opts: &LaunchOpts) -> Option<Vec<String>> {
        // pi doesn't have a --from-pr equivalent.
        None
    }

    fn session_log_paths(&self, _session_id: &str, project_path: &Path) -> Vec<PathBuf> {
        // pi stores sessions at ~/.pi/agent/sessions/<encoded_path>/<timestamp>_<uuid>.jsonl
        let Some(home) = dirs::home_dir() else {
            return vec![];
        };

        let encoded_path = format!("--{}--", project_path.to_string_lossy().replace('/', "-"));

        let session_dir = home
            .join(".pi")
            .join("agent")
            .join("sessions")
            .join(&encoded_path);

        // Return the directory — actual files have timestamp prefixes we can't predict.
        if session_dir.exists() {
            vec![session_dir]
        } else {
            vec![]
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::agent::ApprovalMode;

    #[test]
    fn test_pi_name_and_binary() {
        let p = PiProvider;
        assert_eq!(p.name(), "pi");
        assert_eq!(p.binary(), "pi");
    }

    #[test]
    fn test_pi_launch_cmd_default() {
        let p = PiProvider;
        let opts = LaunchOpts {
            session_id: "ignored".to_owned(),
            approval_mode: ApprovalMode::Default,
        };
        let cmd = p.launch_cmd(&opts);
        assert_eq!(cmd, vec!["pi"]);
    }

    #[test]
    fn test_pi_launch_cmd_auto() {
        let p = PiProvider;
        let opts = LaunchOpts {
            session_id: "ignored".to_owned(),
            approval_mode: ApprovalMode::Auto,
        };
        // pi has no approval modes; always launches plain.
        let cmd = p.launch_cmd(&opts);
        assert_eq!(cmd, vec!["pi"]);
    }

    #[test]
    fn test_pi_launch_cmd_yolo() {
        let p = PiProvider;
        let opts = LaunchOpts {
            session_id: "ignored".to_owned(),
            approval_mode: ApprovalMode::Yolo,
        };
        // pi has no approval modes; always launches plain.
        let cmd = p.launch_cmd(&opts);
        assert_eq!(cmd, vec!["pi"]);
    }

    #[test]
    fn test_pi_resume_cmd_default() {
        let p = PiProvider;
        let cmd = p.resume_cmd(&ResumeOpts {
            approval_mode: ApprovalMode::Default,
        });
        assert_eq!(cmd, vec!["pi", "--continue"]);
    }

    #[test]
    fn test_pi_resume_cmd_auto() {
        let p = PiProvider;
        let cmd = p.resume_cmd(&ResumeOpts {
            approval_mode: ApprovalMode::Auto,
        });
        // pi has no approval modes; resumes the same way.
        assert_eq!(cmd, vec!["pi", "--continue"]);
    }

    #[test]
    fn test_pi_resume_cmd_yolo() {
        let p = PiProvider;
        let cmd = p.resume_cmd(&ResumeOpts {
            approval_mode: ApprovalMode::Yolo,
        });
        // pi has no approval modes; resumes the same way.
        assert_eq!(cmd, vec!["pi", "--continue"]);
    }

    #[test]
    fn test_pi_pr_cmd_returns_none() {
        let p = PiProvider;
        let opts = LaunchOpts {
            session_id: "x".to_owned(),
            approval_mode: ApprovalMode::Default,
        };
        assert!(p.pr_cmd(42, &opts).is_none());
    }

    #[test]
    fn test_pi_is_available() {
        let p = PiProvider;
        // Just verify it doesn't panic. Result depends on environment.
        let _available = p.is_available();
    }

    // --- AgentSandbox tests (ADR-028) ---

    #[test]
    fn test_pi_apply_sandbox_none_is_noop() {
        let mut cmd = vec!["pi".to_owned()];
        let before = cmd.clone();
        apply_sandbox(&mut cmd, AgentSandbox::None);
        assert_eq!(cmd, before);
    }

    #[test]
    fn test_pi_apply_sandbox_os_does_not_modify_argv() {
        // pi has no sandbox flag; argv must be unchanged regardless of sandbox mode.
        let mut cmd = vec!["pi".to_owned()];
        let before = cmd.clone();
        apply_sandbox(&mut cmd, AgentSandbox::Os);
        assert_eq!(
            cmd, before,
            "pi apply_sandbox(Os) must not modify argv (degrade-to-none)"
        );
    }
}
