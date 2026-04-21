//! Claude Code agent provider.
//!
//! Implements [`AgentProvider`] for Anthropic's Claude Code CLI (`claude`).
//! Claude Code supports deterministic session IDs, continuation, and PR review.
//!
//! # OS sandbox (ADR-028)
//!
//! [`AgentSandbox::Os`] is a **documented no-op** for Claude Code. Claude
//! defers OS sandboxing to the caller — its `--dangerously-skip-permissions`
//! help text says *"Recommended only for sandboxes with no internet access"*,
//! meaning Claude expects the **host** to provide the sandbox, not Claude
//! itself. Claude's built-in file-approval flow (`--permission-mode auto`) is
//! the functional equivalent for interactive sessions.
//!
//! Passing an additional sandbox flag would be harmful (no such flag exists on
//! the `claude` binary). `apply_sandbox` is therefore a no-op for all values
//! of [`AgentSandbox`].

pub use crate::agent::codex::AgentSandbox;

use std::path::{Path, PathBuf};

use crate::agent::{AgentProvider, ApprovalMode, LaunchOpts, ResumeOpts};

/// Claude Code agent provider.
///
/// Shells out to the `claude` binary. Supports:
/// - `--session-id <uuid>` for deterministic session tracking
/// - `--continue` for resuming sessions
/// - `--from-pr <number>` for PR review sessions
/// - `--dangerously-skip-permissions` for yolo/unattended mode
pub struct ClaudeProvider;

/// Apply the OS sandbox policy for Claude Code — always a no-op.
///
/// Claude Code does not expose an OS-level sandbox flag. It defers isolation
/// to the host (VM, Docker, Seatbelt wrapper). This function exists for
/// documentation symmetry with other agent providers.
///
/// | `sandbox`            | effect |
/// |----------------------|--------|
/// | `AgentSandbox::None` | no-op  |
/// | `AgentSandbox::Os`   | no-op  |
pub fn apply_sandbox(_cmd: &mut Vec<String>, _sandbox: AgentSandbox) {
    // Intentional no-op. See module-level documentation for rationale.
}

impl AgentProvider for ClaudeProvider {
    fn name(&self) -> &'static str {
        "Claude Code"
    }

    fn binary(&self) -> &'static str {
        "claude"
    }

    fn is_available(&self) -> bool {
        which::which("claude").is_ok()
    }

    fn launch_cmd(&self, opts: &LaunchOpts) -> Vec<String> {
        let mut cmd = vec!["claude".to_owned()];
        match opts.approval_mode {
            ApprovalMode::Default => {}
            ApprovalMode::Auto => {
                cmd.push("--permission-mode".to_owned());
                cmd.push("auto".to_owned());
            }
            ApprovalMode::Yolo => cmd.push("--dangerously-skip-permissions".to_owned()),
        }
        cmd.push("--session-id".to_owned());
        cmd.push(opts.session_id.clone());
        cmd
    }

    fn resume_cmd(&self, opts: &ResumeOpts) -> Vec<String> {
        let mut cmd = vec!["claude".to_owned()];
        match opts.approval_mode {
            ApprovalMode::Default => {}
            ApprovalMode::Auto => {
                cmd.push("--permission-mode".to_owned());
                cmd.push("auto".to_owned());
            }
            ApprovalMode::Yolo => cmd.push("--dangerously-skip-permissions".to_owned()),
        }
        cmd.push("--continue".to_owned());
        cmd
    }

    fn pr_cmd(&self, pr_number: u64, opts: &LaunchOpts) -> Option<Vec<String>> {
        let mut cmd = vec!["claude".to_owned()];
        match opts.approval_mode {
            ApprovalMode::Default => {}
            ApprovalMode::Auto => {
                cmd.push("--permission-mode".to_owned());
                cmd.push("auto".to_owned());
            }
            ApprovalMode::Yolo => cmd.push("--dangerously-skip-permissions".to_owned()),
        }
        cmd.push("--from-pr".to_owned());
        cmd.push(pr_number.to_string());
        Some(cmd)
    }

    fn session_log_paths(&self, session_id: &str, project_path: &Path) -> Vec<PathBuf> {
        // Claude stores logs at ~/.claude/projects/<encoded_path>/<session_id>.jsonl
        // The path encoding replaces `/` with `%2F`.
        let Some(home) = dirs::home_dir() else {
            return vec![];
        };

        let encoded_path = project_path.to_string_lossy().replace('/', "%2F");

        let log_path = home
            .join(".claude")
            .join("projects")
            .join(&encoded_path)
            .join(format!("{session_id}.jsonl"));

        vec![log_path]
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_claude_name_and_binary() {
        let provider = ClaudeProvider;
        assert_eq!(provider.name(), "Claude Code");
        assert_eq!(provider.binary(), "claude");
    }

    #[test]
    fn test_claude_is_available() {
        // This test verifies the method runs without panic.
        // The result depends on whether `claude` is on PATH in the test env.
        let provider = ClaudeProvider;
        let _available = provider.is_available();
    }

    #[test]
    fn test_claude_launch_cmd_default() {
        let provider = ClaudeProvider;
        let opts = LaunchOpts {
            session_id: "abc-123".to_owned(),
            approval_mode: ApprovalMode::Default,
        };
        let cmd = provider.launch_cmd(&opts);
        assert_eq!(cmd, vec!["claude", "--session-id", "abc-123"]);
    }

    #[test]
    fn test_claude_launch_cmd_auto() {
        let provider = ClaudeProvider;
        let opts = LaunchOpts {
            session_id: "abc-123".to_owned(),
            approval_mode: ApprovalMode::Auto,
        };
        let cmd = provider.launch_cmd(&opts);
        assert_eq!(
            cmd,
            vec![
                "claude",
                "--permission-mode",
                "auto",
                "--session-id",
                "abc-123"
            ]
        );
    }

    #[test]
    fn test_claude_launch_cmd_yolo() {
        let provider = ClaudeProvider;
        let opts = LaunchOpts {
            session_id: "abc-123".to_owned(),
            approval_mode: ApprovalMode::Yolo,
        };
        let cmd = provider.launch_cmd(&opts);
        assert_eq!(
            cmd,
            vec![
                "claude",
                "--dangerously-skip-permissions",
                "--session-id",
                "abc-123"
            ]
        );
    }

    #[test]
    fn test_claude_resume_cmd_default() {
        let provider = ClaudeProvider;
        let opts = ResumeOpts {
            approval_mode: ApprovalMode::Default,
        };
        let cmd = provider.resume_cmd(&opts);
        assert_eq!(cmd, vec!["claude", "--continue"]);
    }

    #[test]
    fn test_claude_resume_cmd_auto() {
        let provider = ClaudeProvider;
        let opts = ResumeOpts {
            approval_mode: ApprovalMode::Auto,
        };
        let cmd = provider.resume_cmd(&opts);
        assert_eq!(
            cmd,
            vec!["claude", "--permission-mode", "auto", "--continue"]
        );
    }

    #[test]
    fn test_claude_resume_cmd_yolo() {
        let provider = ClaudeProvider;
        let opts = ResumeOpts {
            approval_mode: ApprovalMode::Yolo,
        };
        let cmd = provider.resume_cmd(&opts);
        assert_eq!(
            cmd,
            vec!["claude", "--dangerously-skip-permissions", "--continue"]
        );
    }

    #[test]
    fn test_claude_pr_cmd_default() {
        let provider = ClaudeProvider;
        let opts = LaunchOpts {
            session_id: "abc-123".to_owned(),
            approval_mode: ApprovalMode::Default,
        };
        let cmd = provider.pr_cmd(42, &opts);
        assert_eq!(
            cmd,
            Some(vec![
                "claude".to_owned(),
                "--from-pr".to_owned(),
                "42".to_owned()
            ])
        );
    }

    #[test]
    fn test_claude_pr_cmd_auto() {
        let provider = ClaudeProvider;
        let opts = LaunchOpts {
            session_id: "abc-123".to_owned(),
            approval_mode: ApprovalMode::Auto,
        };
        let cmd = provider.pr_cmd(42, &opts);
        assert_eq!(
            cmd,
            Some(vec![
                "claude".to_owned(),
                "--permission-mode".to_owned(),
                "auto".to_owned(),
                "--from-pr".to_owned(),
                "42".to_owned()
            ])
        );
    }

    #[test]
    fn test_claude_pr_cmd_yolo() {
        let provider = ClaudeProvider;
        let opts = LaunchOpts {
            session_id: "abc-123".to_owned(),
            approval_mode: ApprovalMode::Yolo,
        };
        let cmd = provider.pr_cmd(42, &opts);
        assert_eq!(
            cmd,
            Some(vec![
                "claude".to_owned(),
                "--dangerously-skip-permissions".to_owned(),
                "--from-pr".to_owned(),
                "42".to_owned()
            ])
        );
    }

    #[test]
    fn test_claude_session_log_paths() {
        let provider = ClaudeProvider;
        let paths = provider.session_log_paths("session-abc", Path::new("/home/user/project"));

        // Should return at least one path
        assert_eq!(paths.len(), 1);

        let path = &paths[0];
        let path_str = path.to_string_lossy();

        // Should contain the .claude/projects directory
        assert!(
            path_str.contains(".claude/projects"),
            "path should contain .claude/projects: {path_str}"
        );

        // Should contain the encoded project path
        assert!(
            path_str.contains("%2F"),
            "path should contain URL-encoded separators: {path_str}"
        );

        // Should end with the session ID + .jsonl
        assert!(
            path_str.ends_with("session-abc.jsonl"),
            "path should end with session-abc.jsonl: {path_str}"
        );
    }

    // --- AgentSandbox tests (ADR-028) ---

    #[test]
    fn test_claude_apply_sandbox_none_is_noop() {
        let mut cmd = vec![
            "claude".to_owned(),
            "--session-id".to_owned(),
            "x".to_owned(),
        ];
        let before = cmd.clone();
        apply_sandbox(&mut cmd, AgentSandbox::None);
        assert_eq!(
            cmd, before,
            "AgentSandbox::None must not change claude argv"
        );
    }

    #[test]
    fn test_claude_apply_sandbox_os_is_noop() {
        // Claude has no OS sandbox flag — AgentSandbox::Os is a documented no-op.
        let mut cmd = vec![
            "claude".to_owned(),
            "--session-id".to_owned(),
            "x".to_owned(),
        ];
        let before = cmd.clone();
        apply_sandbox(&mut cmd, AgentSandbox::Os);
        assert_eq!(
            cmd, before,
            "AgentSandbox::Os must not change claude argv (documented no-op)"
        );
    }
}
