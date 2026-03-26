//! `af resume` — re-attach to an existing workstream.
//!
//! If no session name is given, attempts to show a picker via fzf.
//! Falls back to listing available sessions.

use anyhow::{Context, Result, bail};

use crate::cli::ResumeArgs;
use crate::mux::Multiplexer;
use crate::mux::tmux::TmuxMultiplexer;
use crate::session::ledger::{Ledger, LedgerEvent};
use crate::session::store::SessionStore;

/// Execute the `af resume` command.
pub fn run(args: &ResumeArgs) -> Result<()> {
    let mux = TmuxMultiplexer;
    let store = SessionStore::default_location().context("cannot determine data directory")?;

    let session_name = if let Some(ref name) = args.session {
        name.clone()
    } else {
        // Try fzf picker.
        let sessions = store.list().unwrap_or_default();
        if sessions.is_empty() {
            bail!("no active sessions. Use 'af create' to start one.");
        }
        pick_session(&sessions)?
    };

    // Verify session exists in store.
    if !store.exists(&session_name) {
        bail!("session '{session_name}' not found. Use 'af list' to see active sessions.");
    }

    // If mux session is gone, recreate it from stored metadata.
    if !mux.session_exists(&session_name) {
        let state = store.load(&session_name)?;
        let cwd = match state.worktree.as_ref() {
            Some(wt) => std::path::PathBuf::from(&wt.path),
            None => std::env::current_dir().unwrap_or_default(),
        };

        if !cwd.exists() {
            bail!(
                "worktree path {} no longer exists. Session cannot be recovered.",
                cwd.display()
            );
        }

        mux.create_session(&session_name, &cwd)
            .context("failed to recreate multiplexer session")?;
        mux.set_option(&session_name, "@AF_SESSION", "1")?;

        // Relaunch agent with --continue.
        if let Some(agent) = state.agents.first() {
            let agent_provider = crate::agent::resolve(&agent.provider)
                .ok_or_else(|| anyhow::anyhow!("unknown agent '{}'", agent.provider))?;
            let agent_provider: Box<dyn crate::agent::AgentProvider> = agent_provider;
            let resume_opts = crate::agent::ResumeOpts { yolo: false };
            let cmd_parts = agent_provider.resume_cmd(&resume_opts);
            mux.send_keys(&session_name, &format!("{}\n", cmd_parts.join(" ")))?;
        }

        // Log recovery event.
        let session_dir = store.session_dir_path(&session_name);
        let ledger = Ledger::new(&session_dir);
        let _ = ledger.append(
            &LedgerEvent::new("session_resumed").with_field("recovery", "metadata_restore"),
        );
    }

    mux.attach_or_switch(&session_name)
        .context("failed to attach to session")?;

    Ok(())
}

/// Show a session picker. Uses fzf if available, otherwise prompts.
fn pick_session(sessions: &[String]) -> Result<String> {
    // Try fzf.
    if which::which("fzf").is_ok() {
        let input = sessions.join("\n");
        let output = std::process::Command::new("fzf")
            .args(["--prompt=Session> "])
            .stdin(std::process::Stdio::piped())
            .stdout(std::process::Stdio::piped())
            .stderr(std::process::Stdio::inherit())
            .spawn()
            .and_then(|mut child| {
                use std::io::Write;
                if let Some(ref mut stdin) = child.stdin {
                    let _ = stdin.write_all(input.as_bytes());
                }
                child.wait_with_output()
            });

        if let Ok(output) = output {
            if output.status.success() {
                let choice = String::from_utf8_lossy(&output.stdout).trim().to_owned();
                if !choice.is_empty() {
                    return Ok(choice);
                }
            }
        }
        // fzf cancelled or failed — fall through.
    }

    // Fallback: print list and ask.
    #[allow(clippy::print_stderr)]
    {
        eprintln!("Active sessions:");
        for (i, s) in sessions.iter().enumerate() {
            eprintln!("  [{i}] {s}");
        }
        eprint!("Enter number or name: ");
    }
    let mut reply = String::new();
    std::io::stdin().read_line(&mut reply)?;
    let reply = reply.trim();

    // Try as index first.
    if let Ok(idx) = reply.parse::<usize>() {
        if idx < sessions.len() {
            return Ok(sessions[idx].clone());
        }
    }

    // Try as name.
    if sessions.contains(&reply.to_owned()) {
        return Ok(reply.to_owned());
    }

    bail!("invalid selection: {reply}");
}
