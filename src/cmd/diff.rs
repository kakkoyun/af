//! `af diff` — open a visual diff of the workstream changes.
//!
//! Launches `diffity` (a browser-based GitHub-style diff viewer) against
//! the session's base branch, showing what the agent changed. Falls back
//! to `delta` or plain `git diff` if `diffity` is not available.

use anyhow::{Context, Result, bail};
use tracing::debug;

use crate::cli::DiffArgs;
use crate::mux::Multiplexer;
use crate::mux::tmux::TmuxMultiplexer;
use crate::session::store::SessionStore;

/// Execute the `af diff` command.
pub fn run(args: &DiffArgs) -> Result<()> {
    let mux = TmuxMultiplexer;

    // Resolve session name.
    let session_name = resolve_session_name(&mux, args.session.as_deref())?;

    let store = SessionStore::default_location().context("cannot determine data directory")?;
    let state = store
        .load(&session_name)
        .context("session not found — use 'af list' to see active sessions")?;

    // Determine worktree path.
    let wt_path = match state.worktree.as_ref() {
        Some(wt) => std::path::PathBuf::from(&wt.path),
        None => std::env::current_dir().unwrap_or_default(),
    };

    if !wt_path.exists() {
        bail!("worktree path {} does not exist", wt_path.display());
    }

    // Determine base ref.
    let base_ref = args
        .base
        .clone()
        .or_else(|| state.worktree.as_ref().map(|wt| wt.base_branch.clone()));

    let base_ref = base_ref.unwrap_or_else(|| "HEAD~1".to_owned());
    debug!(base = %base_ref, worktree = %wt_path.display(), "launching diff viewer");

    // Try diffity first, then fall back.
    if diffity_available() {
        launch_diffity(&wt_path, &base_ref, args)
    } else if delta_available() {
        launch_git_diff_with_pager(&wt_path, &base_ref, "delta")
    } else {
        launch_git_diff_with_pager(&wt_path, &base_ref, "less")
    }
}

/// Check if `diffity` is available on PATH.
pub fn diffity_available() -> bool {
    which::which("diffity").is_ok()
}

/// Check if `delta` is available on PATH.
pub fn delta_available() -> bool {
    which::which("delta").is_ok()
}

/// Launch `diffity` with the given base ref and flags.
fn launch_diffity(wt_path: &std::path::Path, base_ref: &str, args: &DiffArgs) -> Result<()> {
    let mut cmd = std::process::Command::new("diffity");
    cmd.arg("--base").arg(base_ref);
    cmd.current_dir(wt_path);

    if args.dark {
        cmd.arg("--dark");
    }
    if args.unified {
        cmd.arg("--unified");
    }
    if args.no_open {
        cmd.arg("--no-open");
    }

    debug!(cmd = ?cmd, "running diffity");

    let status = cmd
        .status()
        .context("failed to launch diffity — is it installed?")?;

    if !status.success() {
        bail!("diffity exited with status {status}");
    }

    Ok(())
}

/// Fall back to `git diff` piped through a pager.
fn launch_git_diff_with_pager(
    wt_path: &std::path::Path,
    base_ref: &str,
    pager: &str,
) -> Result<()> {
    debug!(base = %base_ref, pager, "falling back to git diff");

    #[allow(clippy::print_stderr)]
    {
        eprintln!("diffity not found, using git diff with {pager}");
    }

    let status = std::process::Command::new("git")
        .args(["diff", base_ref])
        .env("GIT_PAGER", pager)
        .current_dir(wt_path)
        .status()
        .context("failed to run git diff")?;

    if !status.success() {
        bail!("git diff exited with status {status}");
    }

    Ok(())
}

/// Build the diffity command arguments for a given configuration.
///
/// Exposed for testing without running the actual command.
pub fn build_diffity_args(base_ref: &str, dark: bool, unified: bool, no_open: bool) -> Vec<String> {
    let mut args = vec!["--base".to_owned(), base_ref.to_owned()];
    if dark {
        args.push("--dark".to_owned());
    }
    if unified {
        args.push("--unified".to_owned());
    }
    if no_open {
        args.push("--no-open".to_owned());
    }
    args
}

// ── Helpers ─────────────────────────────────────────────────────────────────

/// Resolve the session name from an explicit argument or the current mux session.
fn resolve_session_name(mux: &TmuxMultiplexer, explicit: Option<&str>) -> Result<String> {
    if let Some(name) = explicit {
        return Ok(name.to_owned());
    }
    if mux.is_inside_session() {
        if let Some(name) = mux.current_session_name()? {
            return Ok(name);
        }
    }
    bail!("specify a session name, or run inside a multiplexer session");
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_build_diffity_args_basic() {
        let args = build_diffity_args("main", false, false, false);
        assert_eq!(args, vec!["--base", "main"]);
    }

    #[test]
    fn test_build_diffity_args_dark() {
        let args = build_diffity_args("main", true, false, false);
        assert_eq!(args, vec!["--base", "main", "--dark"]);
    }

    #[test]
    fn test_build_diffity_args_unified() {
        let args = build_diffity_args("main", false, true, false);
        assert_eq!(args, vec!["--base", "main", "--unified"]);
    }

    #[test]
    fn test_build_diffity_args_no_open() {
        let args = build_diffity_args("main", false, false, true);
        assert_eq!(args, vec!["--base", "main", "--no-open"]);
    }

    #[test]
    fn test_build_diffity_args_all_flags() {
        let args = build_diffity_args("upstream/main", true, true, true);
        assert_eq!(
            args,
            vec![
                "--base",
                "upstream/main",
                "--dark",
                "--unified",
                "--no-open"
            ]
        );
    }

    #[test]
    fn test_diffity_available_returns_bool() {
        // Just verify it doesn't panic — result depends on env.
        let _available = diffity_available();
    }

    #[test]
    fn test_delta_available_returns_bool() {
        let _available = delta_available();
    }
}
