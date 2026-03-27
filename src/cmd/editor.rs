//! `af editor` — open the workstream codebase in an editor.
//!
//! Opens the worktree path in either a terminal editor (via tmux split) or
//! a visual editor (VS Code / Zed).
//!
//! Editor selection priority:
//! 1. Config file (`[editor]` section) — highest precedence
//! 2. Environment variables (`$EDITOR`, `$AF_VISUAL_EDITOR`, `$CF_VISUAL_EDITOR`)
//! 3. Auto-detection / compiled defaults

use anyhow::{Context, Result, bail};
use tracing::debug;

use crate::cli::EditorArgs;
use crate::config::EditorConfig;
use crate::mux::Multiplexer;
use crate::mux::tmux::TmuxMultiplexer;
use crate::session::store::SessionStore;

/// Execute the `af editor` command.
pub fn run(args: &EditorArgs) -> Result<()> {
    let mux = TmuxMultiplexer;
    let cfg = crate::config::load(None).context("failed to load configuration")?;

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
        let editor = detect_visual_editor(&cfg.editor);
        if let Some(editor) = editor {
            debug!(editor = %editor, "launching visual editor");
            std::process::Command::new(&editor)
                .arg(&wt_path)
                .spawn()
                .with_context(|| format!("failed to launch {editor}"))?;
        } else {
            debug!("no visual editor found, falling back to terminal mode");
            // Fall back to terminal mode.
            open_terminal_editor(&mux, &session_name, &wt_path, &cfg.editor)?;
        }
    } else {
        open_terminal_editor(&mux, &session_name, &wt_path, &cfg.editor)?;
    }

    Ok(())
}

/// Open a terminal editor in a tmux horizontal split pane.
///
/// Resolution order: config `editor.terminal` → `$EDITOR` → `"nvim"`.
fn open_terminal_editor(
    mux: &TmuxMultiplexer,
    session_name: &str,
    wt_path: &std::path::Path,
    editor_cfg: &EditorConfig,
) -> Result<()> {
    let editor = resolve_terminal_editor(editor_cfg);
    debug!(editor = %editor, "opening terminal editor in tmux split");
    let cmd = format!("{editor} .");
    mux.split_horizontal(session_name, &cmd, wt_path)
        .context("failed to split pane for editor")
}

/// Resolve the terminal editor to use.
///
/// Priority: config `editor.terminal` (if non-empty and differs from the
/// compiled default) → `$EDITOR` env var → `"nvim"`.
fn resolve_terminal_editor(editor_cfg: &EditorConfig) -> String {
    let default_terminal = EditorConfig::default().terminal;

    // Config value takes precedence if the user explicitly set it
    // (i.e., it differs from the compiled default).
    if !editor_cfg.terminal.is_empty() && editor_cfg.terminal != default_terminal {
        debug!(
            source = "config",
            editor = %editor_cfg.terminal,
            "terminal editor from config"
        );
        return editor_cfg.terminal.clone();
    }

    // Next: $EDITOR environment variable.
    if let Ok(env_editor) = std::env::var("EDITOR") {
        if !env_editor.is_empty() {
            debug!(
                source = "env",
                editor = %env_editor,
                "terminal editor from $EDITOR"
            );
            return env_editor;
        }
    }

    // Fall back to the compiled default (nvim).
    debug!(
        source = "default",
        editor = %default_terminal,
        "terminal editor from compiled default"
    );
    default_terminal
}

/// Detect the visual editor.
///
/// Priority: config `editor.visual` → `$AF_VISUAL_EDITOR` →
/// `$CF_VISUAL_EDITOR` → `code` on PATH → `zed` on PATH.
fn detect_visual_editor(editor_cfg: &EditorConfig) -> Option<String> {
    // Config value takes highest precedence.
    if !editor_cfg.visual.is_empty() {
        debug!(
            source = "config",
            editor = %editor_cfg.visual,
            "visual editor from config"
        );
        return Some(editor_cfg.visual.clone());
    }

    if let Ok(editor) = std::env::var("AF_VISUAL_EDITOR") {
        if !editor.is_empty() {
            debug!(source = "env", editor = %editor, "visual editor from $AF_VISUAL_EDITOR");
            return Some(editor);
        }
    }
    if let Ok(editor) = std::env::var("CF_VISUAL_EDITOR") {
        if !editor.is_empty() {
            debug!(source = "env", editor = %editor, "visual editor from $CF_VISUAL_EDITOR");
            return Some(editor);
        }
    }
    if which::which("code").is_ok() {
        debug!(
            source = "auto-detect",
            editor = "code",
            "found VS Code on PATH"
        );
        return Some("code".to_owned());
    }
    if which::which("zed").is_ok() {
        debug!(source = "auto-detect", editor = "zed", "found Zed on PATH");
        return Some("zed".to_owned());
    }
    debug!("no visual editor found");
    None
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_resolve_terminal_editor_uses_config_when_non_default() {
        let cfg = EditorConfig {
            terminal: String::from("hx"),
            visual: String::new(),
        };
        // Config overrides everything when it differs from the compiled default.
        let editor = resolve_terminal_editor(&cfg);
        assert_eq!(editor, "hx");
    }

    #[test]
    fn test_resolve_terminal_editor_config_empty_falls_through() {
        // An empty terminal config is not selected; the function
        // falls through to $EDITOR or the compiled default.
        let cfg = EditorConfig {
            terminal: String::new(),
            visual: String::new(),
        };
        let editor = resolve_terminal_editor(&cfg);
        // Either $EDITOR from the environment or "nvim" — both are valid.
        assert!(!editor.is_empty());
    }

    #[test]
    fn test_resolve_terminal_editor_default_config_returns_non_empty() {
        // With default config, result is $EDITOR (if set) or "nvim".
        let cfg = EditorConfig::default();
        let editor = resolve_terminal_editor(&cfg);
        assert!(!editor.is_empty());
    }

    #[test]
    fn test_detect_visual_editor_uses_config_first() {
        let cfg = EditorConfig {
            terminal: String::new(),
            visual: String::from("idea"),
        };
        let editor = detect_visual_editor(&cfg);
        assert_eq!(editor, Some(String::from("idea")));
    }

    #[test]
    fn test_detect_visual_editor_config_takes_precedence_over_path() {
        // Even if code/zed are on PATH, a non-empty config value wins.
        let cfg = EditorConfig {
            terminal: String::new(),
            visual: String::from("emacs"),
        };
        let editor = detect_visual_editor(&cfg);
        assert_eq!(editor, Some(String::from("emacs")));
    }

    #[test]
    fn test_detect_visual_editor_empty_config_does_not_panic() {
        let cfg = EditorConfig {
            terminal: String::new(),
            visual: String::new(),
        };
        // Result depends on env vars and PATH — just verify no panic.
        let _ = detect_visual_editor(&cfg);
    }
}
