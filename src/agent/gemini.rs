//! Gemini CLI agent provider.
//!
//! Implements [`AgentProvider`] for Google's Gemini CLI.
//! Gemini supports resume by index, yolo mode, and sandbox mode.
//!
//! # OS sandbox (ADR-028)
//!
//! Gemini CLI has no `-s`/`--sandbox` flag in the sense that codex does.
//! [`AgentSandbox::Os`] degrades silently to [`AgentSandbox::None`] with a
//! `tracing::info!` log.

pub use crate::agent::codex::AgentSandbox;

use std::path::{Path, PathBuf};

use crate::agent::{AgentProvider, ApprovalMode, LaunchOpts, ResumeOpts};

/// Gemini CLI agent provider.
///
/// Shells out to the `gemini` binary. Supports:
/// - `--resume latest` for resuming the most recent session
/// - `--yolo` / `-y` for auto-approve all actions
/// - `--sandbox` for sandboxed execution
/// - `--approval-mode yolo` as alternative to `--yolo`
pub struct GeminiProvider;

/// Apply the OS sandbox policy for Gemini CLI — degrades to none with an info log.
///
/// Gemini CLI does not expose an OS-level sandbox flag compatible with af's
/// `AgentSandbox::Os` semantics. When `Os` is requested, `af` logs an
/// informational message and proceeds without a sandbox flag.
///
/// | `sandbox`            | effect                                         |
/// |----------------------|------------------------------------------------|
/// | `AgentSandbox::None` | no-op                                          |
/// | `AgentSandbox::Os`   | no-op + `tracing::info!` degrade-to-none log   |
pub fn apply_sandbox(_cmd: &mut Vec<String>, sandbox: AgentSandbox) {
    if sandbox == AgentSandbox::Os {
        tracing::info!(
            agent = "gemini",
            "agent gemini does not support OS sandbox; running without"
        );
    }
}

impl AgentProvider for GeminiProvider {
    fn name(&self) -> &'static str {
        "Gemini CLI"
    }

    fn binary(&self) -> &'static str {
        "gemini"
    }

    fn is_available(&self) -> bool {
        which::which("gemini").is_ok()
    }

    fn launch_cmd(&self, opts: &LaunchOpts) -> Vec<String> {
        let mut cmd = vec!["gemini".to_owned()];
        match opts.approval_mode {
            ApprovalMode::Default => {}
            ApprovalMode::Auto => {
                cmd.push("--approval-mode".to_owned());
                cmd.push("auto_edit".to_owned());
            }
            ApprovalMode::Yolo => cmd.push("--yolo".to_owned()),
        }
        // Gemini doesn't have a --session-id flag.
        // Session ID is tracked in af's metadata only.
        let _ = &opts.session_id;
        cmd
    }

    fn resume_cmd(&self, opts: &ResumeOpts) -> Vec<String> {
        let mut cmd = vec!["gemini".to_owned()];
        match opts.approval_mode {
            ApprovalMode::Default => {}
            ApprovalMode::Auto => {
                cmd.push("--approval-mode".to_owned());
                cmd.push("auto_edit".to_owned());
            }
            ApprovalMode::Yolo => cmd.push("--yolo".to_owned()),
        }
        cmd.push("--resume".to_owned());
        cmd.push("latest".to_owned());
        cmd
    }

    fn pr_cmd(&self, _pr_number: u64, _opts: &LaunchOpts) -> Option<Vec<String>> {
        // Gemini CLI doesn't have a --from-pr equivalent.
        None
    }

    fn session_log_paths(&self, _session_id: &str, _project_path: &Path) -> Vec<PathBuf> {
        // Gemini stores sessions under ~/.gemini/ but the exact structure
        // varies and isn't well-documented. Return empty for now.
        vec![]
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_gemini_name_and_binary() {
        let p = GeminiProvider;
        assert_eq!(p.name(), "Gemini CLI");
        assert_eq!(p.binary(), "gemini");
    }

    #[test]
    fn test_gemini_launch_cmd_default() {
        let p = GeminiProvider;
        let opts = LaunchOpts {
            session_id: "x".to_owned(),
            approval_mode: ApprovalMode::Default,
        };
        assert_eq!(p.launch_cmd(&opts), vec!["gemini"]);
    }

    #[test]
    fn test_gemini_launch_cmd_auto() {
        let p = GeminiProvider;
        let opts = LaunchOpts {
            session_id: "x".to_owned(),
            approval_mode: ApprovalMode::Auto,
        };
        assert_eq!(
            p.launch_cmd(&opts),
            vec!["gemini", "--approval-mode", "auto_edit"]
        );
    }

    #[test]
    fn test_gemini_launch_cmd_yolo() {
        let p = GeminiProvider;
        let opts = LaunchOpts {
            session_id: "x".to_owned(),
            approval_mode: ApprovalMode::Yolo,
        };
        assert_eq!(p.launch_cmd(&opts), vec!["gemini", "--yolo"]);
    }

    #[test]
    fn test_gemini_resume_cmd_default() {
        let p = GeminiProvider;
        let cmd = p.resume_cmd(&ResumeOpts {
            approval_mode: ApprovalMode::Default,
        });
        assert_eq!(cmd, vec!["gemini", "--resume", "latest"]);
    }

    #[test]
    fn test_gemini_resume_cmd_auto() {
        let p = GeminiProvider;
        let cmd = p.resume_cmd(&ResumeOpts {
            approval_mode: ApprovalMode::Auto,
        });
        assert_eq!(
            cmd,
            vec![
                "gemini",
                "--approval-mode",
                "auto_edit",
                "--resume",
                "latest"
            ]
        );
    }

    #[test]
    fn test_gemini_resume_cmd_yolo() {
        let p = GeminiProvider;
        let cmd = p.resume_cmd(&ResumeOpts {
            approval_mode: ApprovalMode::Yolo,
        });
        assert_eq!(cmd, vec!["gemini", "--yolo", "--resume", "latest"]);
    }

    #[test]
    fn test_gemini_pr_cmd_returns_none() {
        let p = GeminiProvider;
        let opts = LaunchOpts {
            session_id: "x".to_owned(),
            approval_mode: ApprovalMode::Default,
        };
        assert!(p.pr_cmd(42, &opts).is_none());
    }

    // --- AgentSandbox tests (ADR-028) ---

    #[test]
    fn test_gemini_apply_sandbox_none_is_noop() {
        let mut cmd = vec!["gemini".to_owned()];
        let before = cmd.clone();
        apply_sandbox(&mut cmd, AgentSandbox::None);
        assert_eq!(cmd, before);
    }

    #[test]
    fn test_gemini_apply_sandbox_os_does_not_modify_argv() {
        // gemini has no OS sandbox flag; argv must be unchanged.
        let mut cmd = vec!["gemini".to_owned()];
        let before = cmd.clone();
        apply_sandbox(&mut cmd, AgentSandbox::Os);
        assert_eq!(
            cmd, before,
            "gemini apply_sandbox(Os) must not modify argv (degrade-to-none)"
        );
    }
}
