//! Session lifecycle notifications via superterm.
//!
//! Sends notifications to the superterm dashboard when session events occur.
//! Falls back silently if superterm is not available — notifications are
//! best-effort and never block the main workflow.

use tracing::debug;

/// Check if the `superterm` binary is available on PATH.
pub fn superterm_available() -> bool {
    which::which("superterm").is_ok()
}

/// Send a notification via `superterm notify`.
///
/// Silently returns if superterm is not available.
pub fn notify(session: &str, title: &str, body: &str) {
    if !superterm_available() {
        debug!("superterm not available, skipping notification");
        return;
    }

    debug!(session, title, "sending superterm notification");

    let mut cmd = std::process::Command::new("superterm");
    cmd.args(["notify", session, "--title", title]);

    if !body.is_empty() {
        cmd.args(["--body", body]);
    }

    match cmd.output() {
        Ok(output) => {
            if !output.status.success() {
                debug!(
                    status = %output.status,
                    "superterm notify returned non-zero (non-fatal)"
                );
            }
        }
        Err(e) => {
            debug!(error = %e, "superterm notify failed (non-fatal)");
        }
    }
}

/// Signal that an agent session has ended via `superterm agent-hook stop`.
///
/// Silently returns if superterm is not available.
pub fn agent_hook_stop(source: &str) {
    if !superterm_available() {
        return;
    }

    debug!(source, "sending superterm agent-hook stop");

    let _ = std::process::Command::new("superterm")
        .args(["agent-hook", "stop", "--source", source])
        .output();
}

/// Build the `superterm notify` command arguments.
///
/// Exposed for testing without running the command.
pub fn build_notify_args(session: &str, title: &str, body: &str) -> Vec<String> {
    let mut args = vec![
        "notify".to_owned(),
        session.to_owned(),
        "--title".to_owned(),
        title.to_owned(),
    ];
    if !body.is_empty() {
        args.push("--body".to_owned());
        args.push(body.to_owned());
    }
    args
}

/// Build the `superterm agent-hook stop` command arguments.
///
/// Exposed for testing without running the command.
pub fn build_agent_hook_stop_args(source: &str) -> Vec<String> {
    vec![
        "agent-hook".to_owned(),
        "stop".to_owned(),
        "--source".to_owned(),
        source.to_owned(),
    ]
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_build_notify_args_with_body() {
        let args = build_notify_args("my-session", "Build complete", "All tests passed");
        assert_eq!(
            args,
            vec![
                "notify",
                "my-session",
                "--title",
                "Build complete",
                "--body",
                "All tests passed"
            ]
        );
    }

    #[test]
    fn test_build_notify_args_without_body() {
        let args = build_notify_args("my-session", "Session started", "");
        assert_eq!(
            args,
            vec!["notify", "my-session", "--title", "Session started"]
        );
    }

    #[test]
    fn test_build_agent_hook_stop_args() {
        let args = build_agent_hook_stop_args("claude");
        assert_eq!(args, vec!["agent-hook", "stop", "--source", "claude"]);
    }

    #[test]
    fn test_build_agent_hook_stop_args_codex() {
        let args = build_agent_hook_stop_args("codex");
        assert_eq!(args, vec!["agent-hook", "stop", "--source", "codex"]);
    }

    #[test]
    fn test_superterm_available_returns_bool() {
        // Just verify it doesn't panic — result depends on env.
        let _available = superterm_available();
    }
}
