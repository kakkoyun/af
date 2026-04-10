//! `af pr` — create a GitHub PR from the current workstream.
//!
//! Uses session metadata (branch, base branch, worktree path) to call
//! `gh pr create` with the correct arguments. Requires `gh` CLI.

use anyhow::{Context, Result, bail};
use tracing::debug;

use crate::cli::PrArgs;
use crate::mux::Multiplexer;
use crate::mux::tmux::TmuxMultiplexer;
use crate::session::ledger::{Ledger, LedgerEvent};
use crate::session::store::SessionStore;

/// Execute the `af pr` command.
pub fn run(args: &PrArgs) -> Result<()> {
    if !crate::git::pr::gh_available() {
        bail!("'gh' CLI is required for 'af pr'. Install it: https://cli.github.com/");
    }

    let mux = TmuxMultiplexer;
    let session_name = resolve_session_name(&mux, args.session.as_deref())?;

    let store = SessionStore::default_location().context("cannot determine data directory")?;
    let state = store
        .load(&session_name)
        .context("session not found — use 'af list' to see active sessions")?;

    let wt = state.worktree.as_ref().ok_or_else(|| {
        anyhow::anyhow!("session '{session_name}' has no worktree (workspace mode)")
    })?;

    let wt_path = std::path::Path::new(&wt.path);
    if !wt_path.exists() {
        bail!("worktree path {} does not exist", wt_path.display());
    }

    // Push branch first.
    debug!(branch = %wt.branch, "pushing branch before PR creation");
    let push_status = std::process::Command::new("git")
        .args(["push", "-u", "origin", &wt.branch])
        .current_dir(wt_path)
        .status()
        .context("failed to push branch")?;

    if !push_status.success() {
        bail!("git push failed — ensure you have write access to the remote");
    }

    // Build gh pr create command.
    let cmd_args = build_gh_pr_args(&wt.branch, &wt.base_branch, args);
    debug!(?cmd_args, "running gh pr create");

    let output = std::process::Command::new("gh")
        .args(&cmd_args)
        .current_dir(wt_path)
        .output()
        .context("failed to run gh pr create")?;

    if !output.status.success() {
        let stderr = String::from_utf8_lossy(&output.stderr);
        bail!("gh pr create failed: {}", stderr.trim());
    }

    let pr_url = String::from_utf8_lossy(&output.stdout).trim().to_owned();

    #[allow(clippy::print_stdout)]
    {
        println!("{pr_url}");
    }

    // Write ledger event.
    let session_dir = store.session_dir_path(&session_name);
    let ledger = Ledger::new(&session_dir);
    let _ = ledger.append(
        &LedgerEvent::new("pr_opened")
            .with_field("url", &pr_url)
            .with_field("branch", &wt.branch)
            .with_field("base", &wt.base_branch),
    );

    Ok(())
}

/// Build the `gh pr create` arguments from session metadata and CLI flags.
///
/// Exposed for testing without running the actual command.
pub fn build_gh_pr_args(branch: &str, base: &str, args: &PrArgs) -> Vec<String> {
    let mut cmd_args = vec![
        "pr".to_owned(),
        "create".to_owned(),
        "--head".to_owned(),
        branch.to_owned(),
        "--base".to_owned(),
        base.to_owned(),
    ];

    if let Some(ref title) = args.title {
        cmd_args.push("--title".to_owned());
        cmd_args.push(title.clone());
    }

    if args.draft {
        cmd_args.push("--draft".to_owned());
    }

    if args.web {
        cmd_args.push("--web".to_owned());
    }

    cmd_args
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
    use crate::cli::PrArgs;

    fn default_args() -> PrArgs {
        PrArgs {
            session: None,
            title: None,
            draft: false,
            web: false,
        }
    }

    #[test]
    fn test_build_gh_pr_args_basic() {
        let args = default_args();
        let cmd = build_gh_pr_args("feat/x", "main", &args);
        assert_eq!(
            cmd,
            vec!["pr", "create", "--head", "feat/x", "--base", "main"]
        );
    }

    #[test]
    fn test_build_gh_pr_args_with_title() {
        let args = PrArgs {
            title: Some("Fix the bug".to_owned()),
            ..default_args()
        };
        let cmd = build_gh_pr_args("fix/bug", "main", &args);
        assert!(cmd.contains(&"--title".to_owned()));
        assert!(cmd.contains(&"Fix the bug".to_owned()));
    }

    #[test]
    fn test_build_gh_pr_args_draft() {
        let args = PrArgs {
            draft: true,
            ..default_args()
        };
        let cmd = build_gh_pr_args("feat/x", "main", &args);
        assert!(cmd.contains(&"--draft".to_owned()));
    }

    #[test]
    fn test_build_gh_pr_args_web() {
        let args = PrArgs {
            web: true,
            ..default_args()
        };
        let cmd = build_gh_pr_args("feat/x", "main", &args);
        assert!(cmd.contains(&"--web".to_owned()));
    }

    #[test]
    fn test_build_gh_pr_args_all_flags() {
        let args = PrArgs {
            session: None,
            title: Some("All flags".to_owned()),
            draft: true,
            web: true,
        };
        let cmd = build_gh_pr_args("feat/all", "develop", &args);
        assert_eq!(cmd[0], "pr");
        assert_eq!(cmd[1], "create");
        assert!(cmd.contains(&"--head".to_owned()));
        assert!(cmd.contains(&"feat/all".to_owned()));
        assert!(cmd.contains(&"--base".to_owned()));
        assert!(cmd.contains(&"develop".to_owned()));
        assert!(cmd.contains(&"--title".to_owned()));
        assert!(cmd.contains(&"All flags".to_owned()));
        assert!(cmd.contains(&"--draft".to_owned()));
        assert!(cmd.contains(&"--web".to_owned()));
    }
}
