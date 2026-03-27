//! tmux multiplexer implementation.
//!
//! Implements [`Multiplexer`] by shelling out to the `tmux` binary.
//! This is the primary (and currently only) multiplexer backend.

use std::path::Path;
use std::process::Command;

use crate::mux::{Multiplexer, SessionInfo};

/// tmux-based multiplexer implementation.
///
/// All methods shell out to the `tmux` CLI. Requires tmux to be installed
/// and available on `$PATH`.
pub struct TmuxMultiplexer;

impl TmuxMultiplexer {
    /// Run a tmux command and return its stdout as a trimmed string.
    fn run_output(args: &[&str]) -> anyhow::Result<String> {
        let output = Command::new("tmux").args(args).output()?;
        if output.status.success() {
            Ok(String::from_utf8_lossy(&output.stdout).trim().to_owned())
        } else {
            let stderr = String::from_utf8_lossy(&output.stderr);
            anyhow::bail!(
                "tmux {} failed: {}",
                args.first().unwrap_or(&""),
                stderr.trim()
            );
        }
    }

    /// Run a tmux command and check for success (discard stdout).
    fn run_status(args: &[&str]) -> anyhow::Result<()> {
        let status = Command::new("tmux").args(args).status()?;
        if status.success() {
            Ok(())
        } else {
            anyhow::bail!("tmux {} failed", args.first().unwrap_or(&""));
        }
    }
}

impl Multiplexer for TmuxMultiplexer {
    fn is_available(&self) -> bool {
        which::which("tmux").is_ok()
    }

    fn is_inside_session(&self) -> bool {
        std::env::var("TMUX").is_ok_and(|v| !v.is_empty())
    }

    fn current_session_name(&self) -> anyhow::Result<Option<String>> {
        if !self.is_inside_session() {
            return Ok(None);
        }
        let name = Self::run_output(&["display-message", "-p", "#{session_name}"])?;
        if name.is_empty() {
            Ok(None)
        } else {
            Ok(Some(name))
        }
    }

    fn create_session(&self, name: &str, cwd: &Path) -> anyhow::Result<()> {
        let status = Command::new("tmux")
            .args(["new-session", "-d", "-s", name, "-c"])
            .arg(cwd)
            .status()?;
        if status.success() {
            Ok(())
        } else {
            anyhow::bail!("tmux new-session failed for '{name}'");
        }
    }

    fn kill_session(&self, name: &str) -> anyhow::Result<()> {
        Self::run_status(&["kill-session", "-t", name])
    }

    fn session_exists(&self, name: &str) -> bool {
        Command::new("tmux")
            .args(["has-session", "-t", name])
            .output()
            .is_ok_and(|o| o.status.success())
    }

    fn attach_or_switch(&self, name: &str) -> anyhow::Result<()> {
        if self.is_inside_session() {
            // Inside tmux — switch client
            Self::run_status(&["switch-client", "-t", name])
        } else {
            // Outside tmux — attach
            Self::run_status(&["attach-session", "-t", name])
        }
    }

    fn send_keys(&self, session: &str, keys: &str) -> anyhow::Result<()> {
        Self::run_status(&["send-keys", "-t", session, keys, "Enter"])
    }

    fn set_env(&self, session: &str, key: &str, value: &str) -> anyhow::Result<()> {
        Self::run_status(&["set-environment", "-t", session, key, value])
    }

    fn get_env(&self, session: &str, key: &str) -> anyhow::Result<Option<String>> {
        let result = Command::new("tmux")
            .args(["show-environment", "-t", session, key])
            .output()?;

        if !result.status.success() {
            // Variable not set — not an error, just absent
            return Ok(None);
        }

        let output = String::from_utf8_lossy(&result.stdout);
        let output = output.trim();

        // tmux outputs "KEY=VALUE" or "-KEY" (if unset)
        if let Some(value) = output.strip_prefix(&format!("{key}=")) {
            Ok(Some(value.to_owned()))
        } else {
            Ok(None)
        }
    }

    fn set_option(&self, session: &str, key: &str, value: &str) -> anyhow::Result<()> {
        Self::run_status(&["set-option", "-t", session, key, value])
    }

    fn list_sessions(&self) -> anyhow::Result<Vec<SessionInfo>> {
        let output =
            Self::run_output(&["list-sessions", "-F", "#{session_name}:#{session_attached}"])?;

        if output.is_empty() {
            return Ok(vec![]);
        }

        let sessions = output
            .lines()
            .filter_map(|line| {
                let (name, attached_str) = line.rsplit_once(':')?;
                Some(SessionInfo {
                    name: name.to_owned(),
                    attached: attached_str == "1",
                })
            })
            .collect();

        Ok(sessions)
    }

    fn split_horizontal(&self, session: &str, cmd: &str, cwd: &Path) -> anyhow::Result<()> {
        let status = Command::new("tmux")
            .args(["split-window", "-h", "-t", session, "-c"])
            .arg(cwd)
            .arg(cmd)
            .status()?;
        if status.success() {
            Ok(())
        } else {
            anyhow::bail!("tmux split-window failed for session '{session}'");
        }
    }

    fn create_pane(&self, session: &str, cwd: &Path) -> anyhow::Result<String> {
        let output = Command::new("tmux")
            .args(["split-window", "-v", "-t", session, "-c"])
            .arg(cwd)
            .args(["-P", "-F", "#{pane_id}"])
            .output()?;
        if output.status.success() {
            Ok(String::from_utf8_lossy(&output.stdout).trim().to_owned())
        } else {
            let stderr = String::from_utf8_lossy(&output.stderr);
            anyhow::bail!(
                "tmux split-window failed for session '{session}': {}",
                stderr.trim()
            );
        }
    }

    fn send_keys_to_pane(&self, session: &str, pane: &str, keys: &str) -> anyhow::Result<()> {
        let target = format!("{session}:{pane}");
        Self::run_status(&["send-keys", "-t", &target, keys, "Enter"])
    }

    fn kill_pane(&self, session: &str, pane: &str) -> anyhow::Result<()> {
        let target = format!("{session}:{pane}");
        Self::run_status(&["kill-pane", "-t", &target])
    }

    fn list_panes(&self, session: &str) -> anyhow::Result<Vec<String>> {
        let output = Self::run_output(&["list-panes", "-t", session, "-F", "#{pane_id}"])?;
        if output.is_empty() {
            return Ok(vec![]);
        }
        Ok(output.lines().map(ToOwned::to_owned).collect())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_tmux_is_available() {
        // Verifies the method runs without panic. Result depends on env.
        let mux = TmuxMultiplexer;
        let _available = mux.is_available();
    }

    #[test]
    fn test_tmux_is_inside_session_respects_env() {
        let mux = TmuxMultiplexer;
        // We can't control the env in a unit test safely without side effects,
        // but we verify the method doesn't panic.
        let _inside = mux.is_inside_session();
    }

    #[test]
    fn test_tmux_session_info_debug_clone() {
        let info = SessionInfo {
            name: "test-session".to_owned(),
            attached: true,
        };
        let cloned = info.clone();
        assert_eq!(cloned.name, "test-session");
        assert!(cloned.attached);
        // Debug trait is derived
        let debug = format!("{info:?}");
        assert!(debug.contains("test-session"));
    }

    #[test]
    fn test_tmux_list_sessions_parsing() {
        // Test the parsing logic used in list_sessions by simulating tmux output format.
        let output = "my-session:1\nother-session:0\ncolon:in:name:1";

        let sessions: Vec<SessionInfo> = output
            .lines()
            .filter_map(|line| {
                let (name, attached_str) = line.rsplit_once(':')?;
                Some(SessionInfo {
                    name: name.to_owned(),
                    attached: attached_str == "1",
                })
            })
            .collect();

        assert_eq!(sessions.len(), 3);
        assert_eq!(sessions[0].name, "my-session");
        assert!(sessions[0].attached);
        assert_eq!(sessions[1].name, "other-session");
        assert!(!sessions[1].attached);
        // rsplit_once handles colons in session names correctly
        assert_eq!(sessions[2].name, "colon:in:name");
        assert!(sessions[2].attached);
    }

    #[test]
    fn test_tmux_get_env_parsing() {
        // Test the KEY=VALUE parsing logic used in get_env
        let key = "AF_SESSION_ID";

        // Case: KEY=VALUE
        let output = "AF_SESSION_ID=abc-123";
        let value = output
            .strip_prefix(&format!("{key}="))
            .map(ToOwned::to_owned);
        assert_eq!(value, Some("abc-123".to_owned()));

        // Case: -KEY (unset marker)
        let output = "-AF_SESSION_ID";
        let value = output
            .strip_prefix(&format!("{key}="))
            .map(ToOwned::to_owned);
        assert_eq!(value, None);

        // Case: different key
        let output = "OTHER_KEY=xyz";
        let value = output
            .strip_prefix(&format!("{key}="))
            .map(ToOwned::to_owned);
        assert_eq!(value, None);
    }

    #[test]
    fn test_tmux_send_keys_to_pane_target_format() {
        // Verify the target format used for pane-targeted commands.
        let session = "my-session";
        let pane = "%5";
        let target = format!("{session}:{pane}");
        assert_eq!(target, "my-session:%5");
    }

    #[test]
    fn test_tmux_list_panes_parsing() {
        // Simulate the output format from `tmux list-panes -F '#{pane_id}'`.
        let output = "%0\n%1\n%2";
        let panes: Vec<String> = output.lines().map(ToOwned::to_owned).collect();
        assert_eq!(panes, vec!["%0", "%1", "%2"]);
    }

    #[test]
    fn test_tmux_list_panes_parsing_empty() {
        let output = "";
        let panes: Vec<String> = if output.is_empty() {
            vec![]
        } else {
            output.lines().map(ToOwned::to_owned).collect()
        };
        assert!(panes.is_empty());
    }

    #[test]
    fn test_tmux_pane_id_format() {
        // tmux pane IDs start with % followed by digits.
        let pane_id = "%42";
        assert!(pane_id.starts_with('%'));
        assert!(pane_id[1..].parse::<u32>().is_ok());
    }
}
