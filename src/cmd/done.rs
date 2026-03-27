//! `af done` — tear down a workstream.
//!
//! Kills the multiplexer session, removes the worktree, deletes the branch,
//! and archives the session metadata.

use anyhow::{Context, Result, bail};
use std::io::Write;

use crate::cli::DoneArgs;
use crate::git::worktree;
use crate::mux::Multiplexer;
use crate::mux::tmux::TmuxMultiplexer;
use crate::session::ledger::{Ledger, LedgerEvent};
use crate::session::store::SessionStore;
use crate::session::types::{ExecutionMode, SessionStatus};

/// Execute the `af done` command.
pub fn run(args: &DoneArgs) -> Result<()> {
    let mux = TmuxMultiplexer;

    // Resolve session name: explicit arg, or current mux session.
    let session_name = if let Some(ref name) = args.session {
        name.clone()
    } else if mux.is_inside_session() {
        mux.current_session_name()?
            .context("cannot determine current session name")?
    } else {
        bail!("specify a session name, or run inside a multiplexer session");
    };

    let store = SessionStore::default_location().context("cannot determine data directory")?;

    // Load session state.
    let state = store
        .load(&session_name)
        .context("session not found — use 'af list' to see active sessions")?;

    // Confirmation prompt (unless --force).
    if !args.force {
        #[allow(clippy::print_stderr)]
        {
            eprint!("Tear down session '{session_name}'");
            if let Some(ref wt) = state.worktree {
                eprint!(" (branch: {}, worktree: {})", wt.branch, wt.path);
            }
            eprintln!("?");
            eprint!("Continue? [y/N] ");
        }
        std::io::stderr().flush()?;
        let mut reply = String::new();
        std::io::stdin().read_line(&mut reply)?;
        if !reply.trim().eq_ignore_ascii_case("y") {
            #[allow(clippy::print_stderr)]
            {
                eprintln!("Aborted.");
            }
            return Ok(());
        }
    }

    // Kill multiplexer session.
    if mux.session_exists(&session_name) {
        let _ = mux.kill_session(&session_name);
    }

    // Clean up worktree and branch (local/bare modes only).
    if let Some(ref wt) = state.worktree {
        if matches!(
            state.execution.mode,
            ExecutionMode::Local | ExecutionMode::Bare
        ) {
            let git_root = std::path::Path::new(&wt.git_root);
            let wt_path = std::path::Path::new(&wt.path);

            if wt_path.exists() {
                let _ = worktree::remove(git_root, wt_path);
            }

            let _ = worktree::delete_branch(git_root, &wt.branch, args.force);

            // Clean up empty parent directory.
            if let Some(parent) = wt_path.parent() {
                let _ = std::fs::remove_dir(parent); // only succeeds if empty
            }
        }
    }

    // Detect PR state and emit ledger event (best-effort).
    let session_dir = store.session_dir_path(&session_name);
    let ledger = Ledger::new(&session_dir);
    if let Some(ref wt) = state.worktree {
        let git_root = std::path::Path::new(&wt.git_root);
        if let Ok(Some(pr_info)) = crate::git::pr::find_pr_for_branch(&wt.branch, git_root) {
            let event_name = match pr_info.state.as_str() {
                "MERGED" => "pr_merged",
                "CLOSED" => "pr_closed",
                _ => "pr_opened",
            };
            let _ = ledger.append(
                &LedgerEvent::new(event_name)
                    .with_field("number", &pr_info.number.to_string())
                    .with_field("url", &pr_info.url)
                    .with_field("state", &pr_info.state),
            );
        }
    }

    // Write ledger events for each running agent.
    for agent in &state.agents {
        if agent.status == crate::session::types::AgentStatus::Running {
            let _ = ledger.append(
                &LedgerEvent::new("agent_stopped")
                    .with_field("slot", &agent.slot)
                    .with_field("agent", &agent.provider)
                    .with_field("reason", "session_teardown"),
            );
        }
    }

    let session_event = if args.force {
        LedgerEvent::new("session_abandoned").with_field("reason", "force")
    } else {
        LedgerEvent::new("session_completed")
    };
    let _ = ledger.append(&session_event);

    // Update all agent statuses and session status, then archive.
    let mut final_state = state;
    for agent in &mut final_state.agents {
        agent.status = crate::session::types::AgentStatus::Stopped;
    }
    final_state.session.status = if args.force {
        SessionStatus::Abandoned
    } else {
        SessionStatus::Completed
    };
    let _ = store.save(&final_state);
    let _ = store.archive(&session_name);

    // Notify superterm (best-effort).
    crate::util::notify::notify(&session_name, "Workstream torn down", "");
    for agent in &final_state.agents {
        crate::util::notify::agent_hook_stop(&agent.provider);
    }

    #[allow(clippy::print_stderr)]
    {
        eprintln!("Session '{session_name}' cleaned up.");
    }

    Ok(())
}
