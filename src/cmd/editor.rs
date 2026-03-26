//! `af editor` — open the workstream codebase in an editor.
//!
//! Opens the worktree path in either a terminal editor (via tmux split) or
//! a visual editor (VS Code / Zed).

use anyhow::{Context, Result, bail};

use crate::cli::EditorArgs;
use crate::mux::Multiplexer;
use crate::mux::tmux::TmuxMultiplexer;
use crate::session::store::SessionStore;

/// Execute the `af editor` command.
pub fn run(args: &EditorArgs) -> Result<()> {
    let mux = TmuxMultiplexer;

    // Resolve session name.
    let session_name = if let Some(ref name) = args.session {
        name.clone()
    } else if mux.is_inside_session() {
        mux.current_session_name()?
            .context("cannot determine current session")?
    } else {
        bail!("specify a session name, or run inside a multiplexer session");
    };

    let store = SessionStore::default_location().context("cannot determine data directory")?;
    let state = store.load(&session_name).context("session not found")?;

    let wt_path = match state.worktree.as_ref() {
        Some(wt) => std::path::PathBuf::from(&wt.path),
        None => std::env::current_dir().unwrap_or_default(),
    };

    if !wt_path.exists() {
        bail!("worktree path {} does not exist", wt_path.display());
    }

    if args.visual || !args.terminal {
        // Visual mode: detect editor and open.
        let editor = detect_visual_editor();
        if let Some(editor) = editor {
            std::process::Command::new(&editor)
                .arg(&wt_path)
                .spawn()
                .with_context(|| format!("failed to launch {editor}"))?;
        } else {
            // Fall back to terminal mode.
            open_terminal_editor(&mux, &session_name, &wt_path)?;
        }
    } else {
        open_terminal_editor(&mux, &session_name, &wt_path)?;
    }

    Ok(())
}

/// Open `$EDITOR` in a tmux horizontal split pane.
fn open_terminal_editor(
    mux: &TmuxMultiplexer,
    session_name: &str,
    wt_path: &std::path::Path,
) -> Result<()> {
    let editor = std::env::var("EDITOR").unwrap_or_else(|_| "nvim".to_owned());
    let cmd = format!("{editor} .");
    mux.split_horizontal(session_name, &cmd, wt_path)
        .context("failed to split pane for editor")
}

/// Detect the visual editor. Priority: `$CF_VISUAL_EDITOR` → `code` → `zed`.
fn detect_visual_editor() -> Option<String> {
    if let Ok(editor) = std::env::var("AF_VISUAL_EDITOR") {
        if !editor.is_empty() {
            return Some(editor);
        }
    }
    if let Ok(editor) = std::env::var("CF_VISUAL_EDITOR") {
        if !editor.is_empty() {
            return Some(editor);
        }
    }
    if which::which("code").is_ok() {
        return Some("code".to_owned());
    }
    if which::which("zed").is_ok() {
        return Some("zed".to_owned());
    }
    None
}
