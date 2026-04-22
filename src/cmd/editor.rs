//! `af editor` — open the workstream codebase in an editor.
//!
//! Opens the worktree path in either a terminal editor (via tmux split) or
//! a visual editor (VS Code / Cursor / Zed).
//!
//! Editor selection priority:
//! 1. Config file (`[editor]` section) — highest precedence
//! 2. Environment variables (`$EDITOR`, `$AF_VISUAL_EDITOR`, `$CF_VISUAL_EDITOR`)
//! 3. Auto-detection / compiled defaults
//!
//! # Remote sessions (ADR-019)
//!
//! When a session's [`ExecutionMode`] is `Remote`, `af editor --visual`
//! constructs an editor-specific remote URI (VS Code / Cursor SSH-remote
//! scheme, or a `zed ssh://...` argument) and launches the editor against
//! the remote host. For the `workspaces` provider, `workspaces connect
//! <name>` is used instead of a URI scheme.
//!
//! Remote dispatch is driven by [`dispatch_remote_visual`], which reads the
//! `ssh_host` / `remote_path` / `remote_provider` fields populated by
//! `af create` into [`ExecutionInfo`]. The pure URL-builder functions
//! [`build_remote_open_args`] and [`build_workspaces_connect_args`] remain
//! exported so external integrators (editor plugins, tests) can reuse them.
//!
//! [`ExecutionMode`]: crate::session::types::ExecutionMode
//! [`ExecutionInfo`]: crate::session::types::ExecutionInfo

use std::str::FromStr;

use anyhow::{Context, Result, bail};
use tracing::{debug, info};

use crate::cli::EditorArgs;
use crate::config::EditorConfig;
use crate::mux::Multiplexer;
use crate::mux::tmux::TmuxMultiplexer;
use crate::session::store::SessionStore;
use crate::session::types::ExecutionMode;

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

    // Remote-session branch (ADR-019).
    //
    // A Remote session whose ExecutionInfo carries an ssh_host launches the
    // editor against the remote host via its native URI scheme (or
    // `workspaces connect` for Datadog Workspaces sessions) and returns
    // without touching the local worktree. If the metadata is incomplete
    // — e.g. ssh_host is missing because the session was created before
    // the schema extension — fall through with an info log.
    if matches!(state.execution.mode, ExecutionMode::Remote) && (args.visual || !args.terminal) {
        if let Some(ssh_host) = state.execution.ssh_host.as_deref() {
            let remote_path = state.execution.remote_path.as_deref().unwrap_or("");
            if let Some(open_args) = dispatch_remote_visual(
                &cfg.editor,
                &session_name,
                state.execution.remote_provider.as_deref(),
                ssh_host,
                remote_path,
            ) {
                debug!(
                    binary = %open_args.binary,
                    host = %ssh_host,
                    "launching remote editor"
                );
                std::process::Command::new(&open_args.binary)
                    .args(&open_args.argv)
                    .spawn()
                    .with_context(|| format!("failed to launch {}", open_args.binary))?;
                return Ok(());
            }
            info!(
                session = %session_name,
                host = %ssh_host,
                "remote session has ssh_host but no visual editor resolvable \
                 from config/env/PATH; falling back to local worktree path"
            );
        } else {
            info!(
                session = %session_name,
                "remote session is missing ssh_host metadata (pre-Phase-IV state.toml); \
                 falling back to local worktree path (ADR-019)"
            );
        }
    }

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

// ── Remote editor URL builders (ADR-019) ───────────────────────────────────
//
// These are pure functions: they take a host string and a remote-path
// string and produce the command invocation (binary + argv) that a caller
// should spawn. They perform no I/O and are fully unit-testable.

/// Which visual editor to launch for a remote session.
///
/// Returned by [`EditorKind::from_str`] so callers can map
/// `config.editor.visual` to a concrete URI scheme builder.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum EditorKind {
    /// Visual Studio Code — uses the `vscode-remote://ssh-remote+<host>/...` URI.
    VSCode,
    /// Cursor — VS Code fork with the `cursor://vscode-remote/ssh-remote+<host>/...` URI.
    Cursor,
    /// Zed — launched with a positional `ssh://<host>/<path>` argument.
    Zed,
}

/// A prepared command invocation for opening an editor.
///
/// Describes the `binary` to spawn and the ordered `argv` to pass. The
/// binary is intentionally a bare name (e.g. `code`, `cursor`, `zed`,
/// `workspaces`) — PATH resolution is left to the caller.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct OpenArgs {
    /// Program name to spawn (looked up on `$PATH`).
    pub binary: String,
    /// Ordered argument list.
    pub argv: Vec<String>,
}

impl FromStr for EditorKind {
    type Err = EditorKindParseError;

    fn from_str(s: &str) -> Result<Self, Self::Err> {
        match s.trim().to_ascii_lowercase().as_str() {
            "code" | "vscode" => Ok(Self::VSCode),
            "cursor" => Ok(Self::Cursor),
            "zed" => Ok(Self::Zed),
            _ => Err(EditorKindParseError(s.to_owned())),
        }
    }
}

/// Error returned when an editor name cannot be mapped to an [`EditorKind`].
#[derive(Debug, thiserror::Error)]
#[error("unknown visual editor {0:?}: expected one of `code`, `cursor`, `zed`")]
pub struct EditorKindParseError(String);

/// Build the [`OpenArgs`] for opening `host:path` in the given editor.
///
/// Per ADR-019 §"URL format per editor":
///
/// | Editor | Invocation |
/// |---|---|
/// | VS Code | `code --folder-uri vscode-remote://ssh-remote+<host>/<path>` |
/// | Cursor  | `cursor --folder-uri cursor://vscode-remote/ssh-remote+<host>/<path>` |
/// | Zed     | `zed ssh://<host>/<path>` (positional arg, not a URI scheme) |
///
/// The `remote_path` is normalised so the URI contains exactly one `/`
/// between the host alias and the path segment, regardless of whether the
/// caller passed a leading slash.
#[must_use]
pub fn build_remote_open_args(kind: &EditorKind, host: &str, remote_path: &str) -> OpenArgs {
    let path = remote_path.trim_start_matches('/');
    match kind {
        EditorKind::VSCode => OpenArgs {
            binary: String::from("code"),
            argv: vec![
                String::from("--folder-uri"),
                format!("vscode-remote://ssh-remote+{host}/{path}"),
            ],
        },
        EditorKind::Cursor => OpenArgs {
            binary: String::from("cursor"),
            argv: vec![
                String::from("--folder-uri"),
                format!("cursor://vscode-remote/ssh-remote+{host}/{path}"),
            ],
        },
        EditorKind::Zed => OpenArgs {
            binary: String::from("zed"),
            argv: vec![format!("ssh://{host}/{path}")],
        },
    }
}

/// Build the [`OpenArgs`] for connecting to a Datadog Workspaces VM.
///
/// Workspaces does not expose a URI scheme; the Workspaces CLI owns the
/// connection. Per ADR-019, for sessions whose `remote_provider` is
/// `workspaces`, `af editor` invokes `workspaces connect <name>` instead of
/// constructing an editor URI.
#[must_use]
pub fn build_workspaces_connect_args(session_name: &str) -> OpenArgs {
    OpenArgs {
        binary: String::from("workspaces"),
        argv: vec![String::from("connect"), session_name.to_owned()],
    }
}

/// Pure dispatcher that picks the right remote-visual [`OpenArgs`].
///
/// Returns `None` when no visual editor can be resolved (e.g. empty
/// config, no env vars set, nothing on `$PATH`), or when the resolved
/// editor name is not one of the recognised [`EditorKind`] values. The
/// caller treats `None` as "fall through to local worktree behaviour".
///
/// Dispatch order:
///
/// 1. If `remote_provider` is `"workspaces"`, return
///    [`build_workspaces_connect_args`] regardless of editor
///    configuration — the Workspaces CLI owns the connection and is
///    incompatible with the generic SSH-remote URI schemes.
/// 2. Otherwise, resolve the visual editor with the same precedence as
///    the local path (config → env vars → PATH), parse it into an
///    [`EditorKind`], and build the appropriate URI via
///    [`build_remote_open_args`].
///
/// The function performs no I/O beyond reading environment variables
/// (via [`std::env::var`]) and consulting `$PATH` via `which`, so it is
/// safe to call from tests that control those inputs.
#[must_use]
pub fn dispatch_remote_visual(
    editor_cfg: &EditorConfig,
    session_name: &str,
    remote_provider: Option<&str>,
    ssh_host: &str,
    remote_path: &str,
) -> Option<OpenArgs> {
    // Workspaces ignores editor kind — it has its own CLI. The host
    // string is unused because the session name doubles as the
    // Workspaces identifier (ADR-027 §5).
    if remote_provider == Some("workspaces") {
        let _ = ssh_host;
        let _ = remote_path;
        return Some(build_workspaces_connect_args(session_name));
    }

    let editor_name = detect_visual_editor(editor_cfg)?;
    let kind: EditorKind = editor_name.parse().ok()?;
    Some(build_remote_open_args(&kind, ssh_host, remote_path))
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

    // ── Remote URL builders (ADR-019) ───────────────────────────────────

    #[test]
    fn test_editor_kind_parse_aliases() {
        assert_eq!("code".parse::<EditorKind>().unwrap(), EditorKind::VSCode);
        assert_eq!("vscode".parse::<EditorKind>().unwrap(), EditorKind::VSCode);
        assert_eq!("cursor".parse::<EditorKind>().unwrap(), EditorKind::Cursor);
        assert_eq!("zed".parse::<EditorKind>().unwrap(), EditorKind::Zed);
    }

    #[test]
    fn test_editor_kind_parse_trims_and_lowercases() {
        assert_eq!(
            "  CODE  ".parse::<EditorKind>().unwrap(),
            EditorKind::VSCode
        );
        assert_eq!("Zed".parse::<EditorKind>().unwrap(), EditorKind::Zed);
    }

    #[test]
    fn test_editor_kind_parse_rejects_unknown() {
        let err = "emacs".parse::<EditorKind>().unwrap_err();
        // The error message must name the offending editor so users can fix their config.
        assert!(err.to_string().contains("emacs"));
    }

    #[test]
    fn test_build_vscode_remote_open_args_shape() {
        let args = build_remote_open_args(&EditorKind::VSCode, "host", "/a/b");
        assert_eq!(args.binary, "code");
        assert_eq!(args.argv[0], "--folder-uri");
        assert_eq!(args.argv[1], "vscode-remote://ssh-remote+host/a/b");
    }

    #[test]
    fn test_build_cursor_remote_open_args_shape() {
        let args = build_remote_open_args(&EditorKind::Cursor, "host", "/a/b");
        assert_eq!(args.binary, "cursor");
        assert_eq!(args.argv[0], "--folder-uri");
        assert_eq!(args.argv[1], "cursor://vscode-remote/ssh-remote+host/a/b");
    }

    #[test]
    fn test_build_zed_remote_open_args_shape() {
        let args = build_remote_open_args(&EditorKind::Zed, "host", "/a/b");
        assert_eq!(args.binary, "zed");
        assert_eq!(args.argv, vec![String::from("ssh://host/a/b")]);
    }

    #[test]
    fn test_build_remote_open_args_normalises_leading_slash() {
        let with = build_remote_open_args(&EditorKind::VSCode, "h", "/a/b");
        let without = build_remote_open_args(&EditorKind::VSCode, "h", "a/b");
        assert_eq!(with.argv[1], without.argv[1]);
    }

    #[test]
    fn test_build_workspaces_connect_args_shape() {
        let args = build_workspaces_connect_args("my-session");
        assert_eq!(args.binary, "workspaces");
        assert_eq!(
            args.argv,
            vec![String::from("connect"), String::from("my-session")]
        );
    }

    // ── dispatch_remote_visual (ADR-019 runtime glue) ───────────────────

    #[test]
    fn test_dispatch_remote_visual_workspaces_provider_overrides_editor_config() {
        // Even if the user has VS Code configured, a workspaces session
        // must call `workspaces connect` because VS Code's ssh-remote
        // scheme does not reach Workspaces VMs.
        let cfg = EditorConfig {
            terminal: String::new(),
            visual: String::from("code"),
        };
        let args = dispatch_remote_visual(
            &cfg,
            "my-session",
            Some("workspaces"),
            "irrelevant-host",
            "/irrelevant/path",
        )
        .expect("workspaces provider must always dispatch");
        assert_eq!(args.binary, "workspaces");
        assert_eq!(
            args.argv,
            vec![String::from("connect"), String::from("my-session")]
        );
    }

    #[test]
    fn test_dispatch_remote_visual_vscode_config_builds_vscode_uri() {
        let cfg = EditorConfig {
            terminal: String::new(),
            visual: String::from("code"),
        };
        let args =
            dispatch_remote_visual(&cfg, "sess", Some("exedev"), "dev-vm", "/home/user/repo")
                .expect("exedev + code should resolve");
        assert_eq!(args.binary, "code");
        assert_eq!(
            args.argv[1],
            "vscode-remote://ssh-remote+dev-vm/home/user/repo"
        );
    }

    #[test]
    fn test_dispatch_remote_visual_cursor_config_builds_cursor_uri() {
        let cfg = EditorConfig {
            terminal: String::new(),
            visual: String::from("cursor"),
        };
        let args = dispatch_remote_visual(&cfg, "sess", Some("exedev"), "dev-vm", "/code")
            .expect("exedev + cursor should resolve");
        assert_eq!(args.binary, "cursor");
        assert_eq!(
            args.argv[1],
            "cursor://vscode-remote/ssh-remote+dev-vm/code"
        );
    }

    #[test]
    fn test_dispatch_remote_visual_zed_config_builds_zed_arg() {
        let cfg = EditorConfig {
            terminal: String::new(),
            visual: String::from("zed"),
        };
        let args = dispatch_remote_visual(&cfg, "sess", Some("exedev"), "dev-vm", "/code")
            .expect("exedev + zed should resolve");
        assert_eq!(args.binary, "zed");
        assert_eq!(args.argv, vec![String::from("ssh://dev-vm/code")]);
    }

    #[test]
    fn test_dispatch_remote_visual_unknown_editor_returns_none() {
        // An editor the URL builder does not understand falls through to
        // the local worktree path — the caller uses this as the signal
        // to log an info message and not touch the remote.
        let cfg = EditorConfig {
            terminal: String::new(),
            visual: String::from("emacs"),
        };
        let args = dispatch_remote_visual(&cfg, "sess", Some("exedev"), "dev-vm", "/code");
        assert!(args.is_none(), "unknown editor must return None");
    }

    #[test]
    fn test_dispatch_remote_visual_exedev_with_empty_remote_path_is_ok() {
        // remote_path = "" is the honest value when provisioning has not
        // yet populated the field. VS Code opens the user's home dir in
        // that case — a better UX than refusing to launch.
        let cfg = EditorConfig {
            terminal: String::new(),
            visual: String::from("code"),
        };
        let args = dispatch_remote_visual(&cfg, "sess", Some("exedev"), "dev-vm", "")
            .expect("empty remote_path is allowed");
        assert_eq!(args.argv[1], "vscode-remote://ssh-remote+dev-vm/");
    }

    #[test]
    fn test_dispatch_remote_visual_unknown_provider_falls_through_to_uri() {
        // A non-workspaces provider (exedev, or a future one) uses the
        // URI-scheme path. None means "provider not recorded" — we still
        // attempt the URI path because ssh_host is available.
        let cfg = EditorConfig {
            terminal: String::new(),
            visual: String::from("code"),
        };
        let args = dispatch_remote_visual(&cfg, "sess", None, "dev-vm", "/code")
            .expect("absent provider should still route through URI builder");
        assert_eq!(args.binary, "code");
    }
}
