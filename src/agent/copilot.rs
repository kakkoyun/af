//! GitHub Copilot CLI agent provider.
//!
//! Implements [`AgentProvider`] for GitHub Copilot CLI.
//! Copilot uses `--continue` for session resumption,
//! `--allow-all --autopilot` for unattended mode.

use std::path::{Path, PathBuf};

use crate::agent::{AgentProvider, ApprovalMode, LaunchOpts, ResumeOpts};

/// GitHub Copilot CLI agent provider.
///
/// Shells out to the `copilot` binary. Supports:
/// - `copilot --continue` for resuming the most recent session
/// - `--allow-all --autopilot` for yolo/unattended mode
/// - Interactive chat with file editing, shell commands, codebase search
pub struct CopilotProvider;

impl AgentProvider for CopilotProvider {
    fn name(&self) -> &'static str {
        "Copilot CLI"
    }

    fn binary(&self) -> &'static str {
        "copilot"
    }

    fn is_available(&self) -> bool {
        which::which("copilot").is_ok()
    }

    fn launch_cmd(&self, opts: &LaunchOpts) -> Vec<String> {
        let mut cmd = vec!["copilot".to_owned()];
        match opts.approval_mode {
            ApprovalMode::Default => {}
            ApprovalMode::Auto => cmd.push("--allow-all-tools".to_owned()),
            ApprovalMode::Yolo => {
                cmd.push("--allow-all".to_owned());
                cmd.push("--autopilot".to_owned());
            }
        }
        // Copilot doesn't support explicit session IDs.
        let _ = &opts.session_id;
        cmd
    }

    fn resume_cmd(&self, opts: &ResumeOpts) -> Vec<String> {
        let mut cmd = vec!["copilot".to_owned()];
        match opts.approval_mode {
            ApprovalMode::Default => {}
            ApprovalMode::Auto => cmd.push("--allow-all-tools".to_owned()),
            ApprovalMode::Yolo => {
                cmd.push("--allow-all".to_owned());
                cmd.push("--autopilot".to_owned());
            }
        }
        cmd.push("--continue".to_owned());
        cmd
    }

    fn pr_cmd(&self, _pr_number: u64, _opts: &LaunchOpts) -> Option<Vec<String>> {
        // Copilot CLI doesn't have a dedicated PR review mode.
        None
    }

    fn session_log_paths(&self, _session_id: &str, _project_path: &Path) -> Vec<PathBuf> {
        // Copilot stores sessions in ~/.copilot/sessions/
        if let Some(home) = dirs::home_dir() {
            let sessions_dir = home.join(".copilot").join("sessions");
            if sessions_dir.exists() {
                return vec![sessions_dir];
            }
        }
        vec![]
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::agent::{ApprovalMode, LaunchOpts, ResumeOpts};

    #[test]
    fn test_copilot_name_and_binary() {
        let provider = CopilotProvider;
        assert_eq!(provider.name(), "Copilot CLI");
        assert_eq!(provider.binary(), "copilot");
    }

    #[test]
    fn test_copilot_is_available() {
        let provider = CopilotProvider;
        // Result depends on env — just verify no panic.
        let _available = provider.is_available();
    }

    #[test]
    fn test_copilot_launch_cmd_default() {
        let provider = CopilotProvider;
        let opts = LaunchOpts {
            session_id: "test-uuid".to_owned(),
            approval_mode: ApprovalMode::Default,
        };
        let cmd = provider.launch_cmd(&opts);
        assert_eq!(cmd, vec!["copilot"]);
    }

    #[test]
    fn test_copilot_launch_cmd_auto() {
        let provider = CopilotProvider;
        let opts = LaunchOpts {
            session_id: "test-uuid".to_owned(),
            approval_mode: ApprovalMode::Auto,
        };
        let cmd = provider.launch_cmd(&opts);
        assert_eq!(cmd, vec!["copilot", "--allow-all-tools"]);
    }

    #[test]
    fn test_copilot_launch_cmd_yolo() {
        let provider = CopilotProvider;
        let opts = LaunchOpts {
            session_id: "test-uuid".to_owned(),
            approval_mode: ApprovalMode::Yolo,
        };
        let cmd = provider.launch_cmd(&opts);
        assert_eq!(cmd, vec!["copilot", "--allow-all", "--autopilot"]);
    }

    #[test]
    fn test_copilot_resume_cmd_default() {
        let provider = CopilotProvider;
        let opts = ResumeOpts {
            approval_mode: ApprovalMode::Default,
        };
        let cmd = provider.resume_cmd(&opts);
        assert_eq!(cmd, vec!["copilot", "--continue"]);
    }

    #[test]
    fn test_copilot_resume_cmd_auto() {
        let provider = CopilotProvider;
        let opts = ResumeOpts {
            approval_mode: ApprovalMode::Auto,
        };
        let cmd = provider.resume_cmd(&opts);
        assert_eq!(cmd, vec!["copilot", "--allow-all-tools", "--continue"]);
    }

    #[test]
    fn test_copilot_resume_cmd_yolo() {
        let provider = CopilotProvider;
        let opts = ResumeOpts {
            approval_mode: ApprovalMode::Yolo,
        };
        let cmd = provider.resume_cmd(&opts);
        assert_eq!(
            cmd,
            vec!["copilot", "--allow-all", "--autopilot", "--continue"]
        );
    }

    #[test]
    fn test_copilot_pr_cmd_returns_none() {
        let provider = CopilotProvider;
        let opts = LaunchOpts {
            session_id: "test".to_owned(),
            approval_mode: ApprovalMode::Default,
        };
        assert!(provider.pr_cmd(42, &opts).is_none());
    }
}
