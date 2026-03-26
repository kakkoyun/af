//! `OpenAI` Codex agent provider.
//!
//! Implements [`AgentProvider`] for `OpenAI`'s Codex CLI.
//! Codex supports `--full-auto` for sandboxed automatic execution,
//! `resume` subcommand for session continuation, and `--session-id` for
//! deterministic sessions.

use std::path::{Path, PathBuf};

use crate::agent::{AgentProvider, LaunchOpts, ResumeOpts};

/// `OpenAI` Codex agent provider.
///
/// Shells out to the `codex` binary. Supports:
/// - `--full-auto` for sandboxed automatic execution (yolo equivalent)
/// - `codex resume <session-id>` for resuming sessions
/// - `codex resume --last` for continuing the most recent session
pub struct CodexProvider;

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
        if opts.yolo {
            cmd.push("--full-auto".to_owned());
        }
        // Codex doesn't have --session-id on launch; af tracks the ID externally.
        let _ = &opts.session_id;
        cmd
    }

    fn resume_cmd(&self, opts: &ResumeOpts) -> Vec<String> {
        let mut cmd = vec!["codex".to_owned()];
        if opts.yolo {
            cmd.push("--full-auto".to_owned());
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
    fn test_codex_launch_cmd_basic() {
        let p = CodexProvider;
        let opts = LaunchOpts {
            session_id: "x".to_owned(),
            yolo: false,
        };
        assert_eq!(p.launch_cmd(&opts), vec!["codex"]);
    }

    #[test]
    fn test_codex_launch_cmd_yolo() {
        let p = CodexProvider;
        let opts = LaunchOpts {
            session_id: "x".to_owned(),
            yolo: true,
        };
        assert_eq!(p.launch_cmd(&opts), vec!["codex", "--full-auto"]);
    }

    #[test]
    fn test_codex_resume_cmd_basic() {
        let p = CodexProvider;
        let cmd = p.resume_cmd(&ResumeOpts { yolo: false });
        assert_eq!(cmd, vec!["codex", "resume", "--last"]);
    }

    #[test]
    fn test_codex_resume_cmd_yolo() {
        let p = CodexProvider;
        let cmd = p.resume_cmd(&ResumeOpts { yolo: true });
        assert_eq!(cmd, vec!["codex", "--full-auto", "resume", "--last"]);
    }

    #[test]
    fn test_codex_pr_cmd_returns_none() {
        let p = CodexProvider;
        let opts = LaunchOpts {
            session_id: "x".to_owned(),
            yolo: false,
        };
        assert!(p.pr_cmd(42, &opts).is_none());
    }

    #[test]
    fn test_codex_is_available() {
        let p = CodexProvider;
        // Should find codex now that it's installed.
        let _available = p.is_available();
    }
}
