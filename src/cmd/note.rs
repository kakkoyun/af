//! `af note` — open the Obsidian note for a workstream.
//!
//! Resolves the session, finds its note in the configured Obsidian vault,
//! and opens it. If the note doesn't exist yet, creates it from session metadata.

use anyhow::{Context, Result, bail};

use crate::cli::NoteArgs;
use crate::mux::Multiplexer;
use crate::mux::tmux::TmuxMultiplexer;
use crate::session::store::SessionStore;

/// Execute the `af note` command.
pub fn run(args: &NoteArgs) -> Result<()> {
    let cfg = crate::config::load(None).context("failed to load configuration")?;

    if !cfg.obsidian.enabled {
        bail!(
            "Obsidian integration is disabled. Enable it in config:\n\n\
             [obsidian]\n\
             enabled = true\n\
             vault = \"~/Vaults/work\"\n"
        );
    }

    let mux = TmuxMultiplexer;
    let session_name = resolve_session_name(&mux, args.session.as_deref())?;

    let store = SessionStore::default_location().context("cannot determine data directory")?;
    let state = store
        .load(&session_name)
        .context("session not found — use 'af list' to see active sessions")?;

    let note_path = crate::obsidian::note_path(&cfg.obsidian, &session_name)?;

    // Create the note if it doesn't exist.
    if !note_path.exists() {
        let wt = state.worktree.as_ref();
        let meta = build_note_meta(&session_name, &state);
        crate::obsidian::create_note(&note_path, &meta)?;

        #[allow(clippy::print_stderr)]
        {
            eprintln!("Created note at {}", note_path.display());
        }
        let _ = wt;
    }

    crate::obsidian::open_note(&note_path)?;

    Ok(())
}

/// Build note metadata from session state.
fn build_note_meta(
    session_name: &str,
    state: &crate::session::types::SessionState,
) -> crate::obsidian::NoteMeta {
    let wt = state.worktree.as_ref();
    crate::obsidian::NoteMeta {
        session: session_name.to_owned(),
        branch: wt.map_or_else(String::new, |w| w.branch.clone()),
        base_branch: wt.map_or_else(String::new, |w| w.base_branch.clone()),
        repo: wt.map_or_else(
            || "workspace".to_owned(),
            |w| {
                std::path::Path::new(&w.git_root)
                    .file_name()
                    .map_or_else(|| "repo".to_owned(), |n| n.to_string_lossy().into_owned())
            },
        ),
        agent: state
            .agents
            .first()
            .map_or_else(|| "unknown".to_owned(), |a| a.provider.clone()),
        status: "active".to_owned(),
        created_at: state.session.created_at,
        completed_at: None,
    }
}

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
