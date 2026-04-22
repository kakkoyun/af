//! Sourcegraph Amp agent provider.
//!
//! Implements [`AgentProvider`] for Sourcegraph's Amp CLI.
//! Amp uses a thread-based model: `amp threads continue --last` for resumption,
//! `--dangerously-allow-all` for unattended mode.
//!
//! # OS sandbox (ADR-028)
//!
//! Amp has no CLI sandbox flag. [`AgentSandbox::Os`] degrades silently to
//! [`AgentSandbox::None`] with a `tracing::info!` log.

use std::path::{Path, PathBuf};

use crate::agent::{AgentProvider, AgentSandbox, ApprovalMode, LaunchOpts, ResumeOpts};

/// Sourcegraph Amp agent provider.
///
/// Shells out to the `amp` binary. Supports:
/// - `amp threads continue --last` for resuming the most recent thread
/// - `--dangerously-allow-all` for yolo/unattended mode
/// - Thread-based session model (threads new/continue/list)
pub struct AmpProvider;

/// Apply the OS sandbox policy for Amp — degrades to none with an info log.
///
/// Amp exposes no OS-level sandbox flag. When [`AgentSandbox::Os`] is
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
            agent = "amp",
            "agent amp does not support OS sandbox; running without"
        );
    }
}

impl AgentProvider for AmpProvider {
    fn name(&self) -> &'static str {
        "Amp"
    }

    fn binary(&self) -> &'static str {
        "amp"
    }

    fn is_available(&self) -> bool {
        which::which("amp").is_ok()
    }

    fn launch_cmd(&self, opts: &LaunchOpts) -> Vec<String> {
        let mut cmd = vec!["amp".to_owned()];
        match opts.approval_mode {
            // Amp has no auto mode; fall through to default.
            ApprovalMode::Default | ApprovalMode::Auto => {}
            ApprovalMode::Yolo => cmd.push("--dangerously-allow-all".to_owned()),
        }
        // Amp uses its own thread-based session system.
        let _ = &opts.session_id;
        cmd
    }

    fn resume_cmd(&self, opts: &ResumeOpts) -> Vec<String> {
        let mut cmd = vec!["amp".to_owned()];
        match opts.approval_mode {
            // Amp has no auto mode; fall through to default.
            ApprovalMode::Default | ApprovalMode::Auto => {}
            ApprovalMode::Yolo => cmd.push("--dangerously-allow-all".to_owned()),
        }
        cmd.extend([
            "threads".to_owned(),
            "continue".to_owned(),
            "--last".to_owned(),
        ]);
        cmd
    }

    fn pr_cmd(&self, _pr_number: u64, _opts: &LaunchOpts) -> Option<Vec<String>> {
        // Amp has `amp review` but not --from-pr.
        None
    }

    fn session_log_paths(&self, _session_id: &str, _project_path: &Path) -> Vec<PathBuf> {
        // Amp stores threads internally; path structure not documented.
        vec![]
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_amp_name_and_binary() {
        let p = AmpProvider;
        assert_eq!(p.name(), "Amp");
        assert_eq!(p.binary(), "amp");
    }

    #[test]
    fn test_amp_launch_cmd_default() {
        let p = AmpProvider;
        let opts = LaunchOpts {
            session_id: "x".to_owned(),
            approval_mode: ApprovalMode::Default,
        };
        assert_eq!(p.launch_cmd(&opts), vec!["amp"]);
    }

    #[test]
    fn test_amp_launch_cmd_auto() {
        let p = AmpProvider;
        let opts = LaunchOpts {
            session_id: "x".to_owned(),
            approval_mode: ApprovalMode::Auto,
        };
        // Amp has no auto mode; falls through to default.
        assert_eq!(p.launch_cmd(&opts), vec!["amp"]);
    }

    #[test]
    fn test_amp_launch_cmd_yolo() {
        let p = AmpProvider;
        let opts = LaunchOpts {
            session_id: "x".to_owned(),
            approval_mode: ApprovalMode::Yolo,
        };
        assert_eq!(p.launch_cmd(&opts), vec!["amp", "--dangerously-allow-all"]);
    }

    #[test]
    fn test_amp_resume_cmd_default() {
        let p = AmpProvider;
        let cmd = p.resume_cmd(&ResumeOpts {
            approval_mode: ApprovalMode::Default,
        });
        assert_eq!(cmd, vec!["amp", "threads", "continue", "--last"]);
    }

    #[test]
    fn test_amp_resume_cmd_auto() {
        let p = AmpProvider;
        let cmd = p.resume_cmd(&ResumeOpts {
            approval_mode: ApprovalMode::Auto,
        });
        // Amp has no auto mode; falls through to default.
        assert_eq!(cmd, vec!["amp", "threads", "continue", "--last"]);
    }

    #[test]
    fn test_amp_resume_cmd_yolo() {
        let p = AmpProvider;
        let cmd = p.resume_cmd(&ResumeOpts {
            approval_mode: ApprovalMode::Yolo,
        });
        assert_eq!(
            cmd,
            vec![
                "amp",
                "--dangerously-allow-all",
                "threads",
                "continue",
                "--last"
            ]
        );
    }

    #[test]
    fn test_amp_pr_cmd_returns_none() {
        let p = AmpProvider;
        let opts = LaunchOpts {
            session_id: "x".to_owned(),
            approval_mode: ApprovalMode::Default,
        };
        assert!(p.pr_cmd(42, &opts).is_none());
    }

    #[test]
    fn test_amp_is_available() {
        let p = AmpProvider;
        let _available = p.is_available();
    }

    // --- AgentSandbox tests (ADR-028) ---

    #[test]
    fn test_amp_apply_sandbox_none_is_noop() {
        let mut cmd = vec!["amp".to_owned()];
        let before = cmd.clone();
        apply_sandbox(&mut cmd, AgentSandbox::None);
        assert_eq!(cmd, before);
    }

    #[test]
    fn test_amp_apply_sandbox_os_does_not_modify_argv() {
        // amp has no OS sandbox flag; argv must be unchanged.
        let mut cmd = vec!["amp".to_owned()];
        let before = cmd.clone();
        apply_sandbox(&mut cmd, AgentSandbox::Os);
        assert_eq!(
            cmd, before,
            "amp apply_sandbox(Os) must not modify argv (degrade-to-none)"
        );
    }
}
