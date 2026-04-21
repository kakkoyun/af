//! cmux multiplexer implementation.
//!
//! Implements [`Multiplexer`] by shelling out to the `cmux` CLI binary.
//!
//! ## Conceptual mapping
//!
//! cmux's object model differs from tmux's. The table below shows how each
//! tmux concept maps to cmux primitives:
//!
//! | tmux concept    | cmux equivalent                                    |
//! |-----------------|-----------------------------------------------------|
//! | session         | workspace (`new-workspace`, `list-workspaces`)      |
//! | window          | pane (a group of surfaces inside a workspace)       |
//! | pane            | surface (a terminal surface inside a pane)          |
//! | session name    | workspace `--name`                                  |
//! | send-keys       | `cmux send "text\n"` (with `\n` for Enter)          |
//! | set-environment | sidecar JSON file in `$XDG_DATA_HOME/af/cmux/`      |
//! | attach-session  | `cmux workspace-action move-top` + `open -a cmux`   |
//! | kill-session    | `cmux close-workspace --workspace <ref>`            |
//!
//! ## Environment variables
//!
//! cmux injects three environment variables into every terminal surface it
//! manages:
//! - `CMUX_WORKSPACE_ID` — UUID of the current workspace
//! - `CMUX_SURFACE_ID`   — UUID of the current surface
//! - `CMUX_TAB_ID`       — UUID of the current tab/surface alias
//!
//! `is_inside_session` tests for a non-empty `CMUX_WORKSPACE_ID`.
//!
//! ## Environment variable persistence
//!
//! cmux has no native `set-environment` / `get-environment` equivalent.
//! `CmuxMultiplexer` persists session-scoped environment key/value pairs in a
//! JSON sidecar file at:
//!
//! ```text
//! $XDG_DATA_HOME/af/cmux/<session-name>.env.json
//! ```
//!
//! (`$XDG_DATA_HOME` defaults to `~/.local/share` on all platforms.)
//!
//! ## `attach_or_switch` behaviour
//!
//! cmux is a macOS GUI application; there is no `attach` concept. Instead,
//! `attach_or_switch`:
//! 1. Resolves the workspace ref from the session name via `list-workspaces`.
//! 2. Calls `workspace-action move-top` to promote the workspace to the top.
//! 3. Calls `open -a cmux` to bring the application to the foreground.
//!
//! ## `set_option` / `get_env`
//!
//! Both delegate to the sidecar file. Options are stored in the same map as
//! environment variables, using an `@` prefix on the key to mirror tmux's
//! `@option` naming convention.
//!
//! ## Phase IV wiring
//!
//! The lead must add to `src/mux/mod.rs`:
//! ```rust
//! pub mod cmux;
//! ```
//! and wire the factory auto-select logic (ADR-022 §Decision).

use std::collections::HashMap;
use std::fs;
use std::path::{Path, PathBuf};
use std::process::Command;

use anyhow::Context as _;
use serde::{Deserialize, Serialize};

use crate::mux::{Multiplexer, SessionInfo};

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

/// Default cmux binary name. Resolved via PATH at runtime unless overridden.
const DEFAULT_BINARY: &str = "cmux";

/// Absolute path to the cmux binary bundled with the macOS app.
const MACOS_APP_BINARY: &str =
    "/Applications/cmux.app/Contents/Resources/bin/cmux";

// ---------------------------------------------------------------------------
// Sidecar (env persistence)
// ---------------------------------------------------------------------------

/// On-disk format for the per-session environment sidecar file.
///
/// Keys are plain env var names or `@option` names (with the `@` prefix).
#[derive(Debug, Default, Serialize, Deserialize)]
struct EnvSidecar {
    /// Key → value pairs stored for this session.
    vars: HashMap<String, String>,
}

// ---------------------------------------------------------------------------
// CmuxMultiplexer
// ---------------------------------------------------------------------------

/// cmux-based multiplexer implementation.
///
/// All methods shell out to the `cmux` CLI. Requires cmux to be installed
/// and the cmux application to be running (its Unix socket must be active).
///
/// The binary is resolved in this order:
/// 1. The value returned by [`CmuxMultiplexer::binary`] — configurable at
///    construction time.
/// 2. `cmux` on `$PATH`.
/// 3. The macOS app bundle path (`/Applications/cmux.app/…/cmux`).
pub struct CmuxMultiplexer {
    /// Path to the cmux binary. Defaults to `cmux` (PATH lookup).
    binary: String,
    /// Base directory for sidecar env files.
    data_dir: PathBuf,
}

impl CmuxMultiplexer {
    /// Create a new `CmuxMultiplexer` using `cmux` from PATH.
    ///
    /// The sidecar directory is `$XDG_DATA_HOME/af/cmux/` (falling back to
    /// `~/.local/share/af/cmux/`).
    pub fn new() -> Self {
        Self {
            binary: DEFAULT_BINARY.to_owned(),
            data_dir: default_data_dir(),
        }
    }

    /// Create a `CmuxMultiplexer` with a custom binary path and data directory.
    ///
    /// Useful for testing or when the binary lives outside `$PATH`.
    pub fn with_binary(binary: impl Into<String>, data_dir: impl Into<PathBuf>) -> Self {
        Self {
            binary: binary.into(),
            data_dir: data_dir.into(),
        }
    }

    /// Return the binary path to use, falling back to the macOS app bundle.
    fn resolved_binary(&self) -> String {
        if self.binary != DEFAULT_BINARY {
            return self.binary.clone();
        }
        // If `cmux` is not on PATH, try the macOS app bundle.
        if which::which(&self.binary).is_ok() {
            self.binary.clone()
        } else if Path::new(MACOS_APP_BINARY).exists() {
            MACOS_APP_BINARY.to_owned()
        } else {
            self.binary.clone()
        }
    }

    /// Run a cmux command and return its stdout as a trimmed string.
    fn run_output(&self, args: &[&str]) -> anyhow::Result<String> {
        let output = Command::new(self.resolved_binary())
            .args(args)
            .output()
            .with_context(|| format!("spawn cmux {:?}", args.first()))?;
        if output.status.success() {
            Ok(String::from_utf8_lossy(&output.stdout).trim().to_owned())
        } else {
            let stderr = String::from_utf8_lossy(&output.stderr);
            anyhow::bail!(
                "cmux {} failed: {}",
                args.first().unwrap_or(&""),
                stderr.trim()
            )
        }
    }

    /// Run a cmux command and check for success (stdout discarded).
    fn run_status(&self, args: &[&str]) -> anyhow::Result<()> {
        let status = Command::new(self.resolved_binary())
            .args(args)
            .status()
            .with_context(|| format!("spawn cmux {:?}", args.first()))?;
        if status.success() {
            Ok(())
        } else {
            anyhow::bail!("cmux {} failed", args.first().unwrap_or(&""))
        }
    }

    /// Resolve a workspace ref string (e.g. `workspace:3`) from a session name.
    ///
    /// Returns `None` if no workspace with that name is found.
    fn workspace_ref_for(&self, name: &str) -> anyhow::Result<Option<String>> {
        let raw = self.run_output(&["list-workspaces"])?;
        Ok(parse_workspace_ref(&raw, name))
    }

    // ------------------------------------------------------------------
    // Sidecar helpers
    // ------------------------------------------------------------------

    /// Return the path of the sidecar env file for the given session name.
    fn sidecar_path(&self, session: &str) -> PathBuf {
        self.data_dir.join(format!("{session}.env.json"))
    }

    /// Load the sidecar for a session (returns a default empty map on missing).
    fn load_sidecar(&self, session: &str) -> anyhow::Result<EnvSidecar> {
        let path = self.sidecar_path(session);
        if !path.exists() {
            return Ok(EnvSidecar::default());
        }
        let raw = fs::read_to_string(&path)
            .with_context(|| format!("read cmux env sidecar {}", path.display()))?;
        serde_json::from_str(&raw)
            .with_context(|| format!("parse cmux env sidecar {}", path.display()))
    }

    /// Persist a sidecar file for the given session.
    fn save_sidecar(&self, session: &str, sidecar: &EnvSidecar) -> anyhow::Result<()> {
        let path = self.sidecar_path(session);
        fs::create_dir_all(path.parent().unwrap_or(&self.data_dir))
            .with_context(|| format!("create cmux data dir {}", self.data_dir.display()))?;
        let json = serde_json::to_string_pretty(sidecar)
            .context("serialise cmux env sidecar")?;
        fs::write(&path, json)
            .with_context(|| format!("write cmux env sidecar {}", path.display()))
    }
}

impl Default for CmuxMultiplexer {
    fn default() -> Self {
        Self::new()
    }
}

// ---------------------------------------------------------------------------
// Multiplexer impl
// ---------------------------------------------------------------------------

impl Multiplexer for CmuxMultiplexer {
    /// Check if the cmux binary is available (on PATH or in the app bundle).
    fn is_available(&self) -> bool {
        which::which(&self.binary).is_ok() || Path::new(MACOS_APP_BINARY).exists()
    }

    /// Return `true` when running inside a cmux-managed terminal surface.
    ///
    /// cmux sets `CMUX_WORKSPACE_ID` automatically in every terminal it manages.
    fn is_inside_session(&self) -> bool {
        std::env::var("CMUX_WORKSPACE_ID").is_ok_and(|v| !v.is_empty())
    }

    /// Return the name of the current cmux workspace, if inside one.
    ///
    /// Calls `cmux identify` (JSON output) to find the current `workspace_ref`,
    /// then cross-references `cmux list-workspaces` to resolve the human-readable
    /// name.
    fn current_session_name(&self) -> anyhow::Result<Option<String>> {
        if !self.is_inside_session() {
            return Ok(None);
        }
        let json_raw = self
            .run_output(&["identify", "--no-caller"])
            .context("cmux identify")?;
        let identity: serde_json::Value =
            serde_json::from_str(&json_raw).context("parse cmux identify output")?;
        let workspace_ref = identity
            .pointer("/focused/workspace_ref")
            .and_then(|v| v.as_str())
            .map(ToOwned::to_owned);

        let Some(ws_ref) = workspace_ref else {
            return Ok(None);
        };

        // Resolve the ref to a name via list-workspaces.
        let raw = self.run_output(&["list-workspaces"])?;
        Ok(parse_workspace_name_for_ref(&raw, &ws_ref))
    }

    /// Create a new detached cmux workspace with the given name and working directory.
    ///
    /// Maps to `cmux new-workspace --name <name> --cwd <cwd>`.
    ///
    /// # Note
    ///
    /// cmux workspaces are created in the currently active window. Unlike tmux
    /// sessions, they are always visible in the GUI immediately after creation.
    fn create_session(&self, name: &str, cwd: &Path) -> anyhow::Result<()> {
        let cwd_str = cwd
            .to_str()
            .with_context(|| format!("non-UTF-8 path: {}", cwd.display()))?;
        self.run_status(&[
            "new-workspace",
            "--name",
            name,
            "--cwd",
            cwd_str,
        ])
        .with_context(|| format!("cmux new-workspace failed for {name:?}"))
    }

    /// Close (kill) the cmux workspace with the given name.
    ///
    /// Resolves the workspace ref from the name via `list-workspaces`, then
    /// calls `cmux close-workspace --workspace <ref>`.
    fn kill_session(&self, name: &str) -> anyhow::Result<()> {
        let ws_ref = self
            .workspace_ref_for(name)?
            .with_context(|| format!("cmux workspace {name:?} not found"))?;
        self.run_status(&["close-workspace", "--workspace", &ws_ref])
            .with_context(|| format!("cmux close-workspace failed for {name:?}"))
    }

    /// Return `true` if a cmux workspace with the given name exists.
    fn session_exists(&self, name: &str) -> bool {
        self.workspace_ref_for(name)
            .ok()
            .flatten()
            .is_some()
    }

    /// Bring the cmux workspace with the given name to the foreground.
    ///
    /// ## cmux deviation from tmux
    ///
    /// cmux is a macOS GUI application; there is no terminal `attach-session`
    /// concept. Instead this method:
    /// 1. Promotes the workspace to the top of the list via
    ///    `workspace-action move-top`.
    /// 2. Calls `open -a cmux` to raise the application window (macOS only).
    ///
    /// Running from inside a cmux surface will promote the workspace; outside
    /// cmux it will also bring the GUI to the front.
    fn attach_or_switch(&self, name: &str) -> anyhow::Result<()> {
        let ws_ref = self
            .workspace_ref_for(name)?
            .with_context(|| format!("cmux workspace {name:?} not found"))?;
        self.run_status(&[
            "workspace-action",
            "--workspace",
            &ws_ref,
            "--action",
            "move-top",
        ])
        .with_context(|| format!("cmux workspace-action move-top failed for {name:?}"))?;
        // Best-effort: raise the cmux GUI window. Non-fatal if `open` is absent
        // (e.g. in CI or Linux).
        let _ = Command::new("open").args(["-a", "cmux"]).status();
        Ok(())
    }

    /// Send text followed by Enter to the active surface of the named workspace.
    ///
    /// Maps to `cmux send --workspace <ref> "<keys>\n"`.
    ///
    /// The `\n` suffix tells cmux to synthesise an Enter keystroke, matching
    /// the behaviour of `tmux send-keys … Enter`.
    fn send_keys(&self, session: &str, keys: &str) -> anyhow::Result<()> {
        let ws_ref = self
            .workspace_ref_for(session)?
            .with_context(|| format!("cmux workspace {session:?} not found"))?;
        // cmux `send` interprets `\n` as Enter.
        let text = format!("{keys}\\n");
        self.run_status(&["send", "--workspace", &ws_ref, "--", &text])
            .with_context(|| format!("cmux send failed for workspace {session:?}"))
    }

    /// Persist a session-scoped environment variable in the sidecar file.
    ///
    /// cmux has no native `set-environment` equivalent. Values are stored in a
    /// JSON file at `$XDG_DATA_HOME/af/cmux/<session>.env.json`.
    fn set_env(&self, session: &str, key: &str, value: &str) -> anyhow::Result<()> {
        let mut sidecar = self.load_sidecar(session)?;
        sidecar.vars.insert(key.to_owned(), value.to_owned());
        self.save_sidecar(session, &sidecar)
    }

    /// Retrieve a session-scoped environment variable from the sidecar file.
    ///
    /// Returns `Ok(None)` if the key is not set (not an error).
    fn get_env(&self, session: &str, key: &str) -> anyhow::Result<Option<String>> {
        let sidecar = self.load_sidecar(session)?;
        Ok(sidecar.vars.get(key).cloned())
    }

    /// Store a session option in the sidecar file with an `@` prefix.
    ///
    /// This mirrors tmux's `@option` naming convention (`@AF_SESSION`, etc.).
    /// The option is retrievable via [`get_env`] using the same `@`-prefixed key.
    ///
    /// [`get_env`]: CmuxMultiplexer::get_env
    fn set_option(&self, session: &str, key: &str, value: &str) -> anyhow::Result<()> {
        // Store options with their key as-is; callers that follow tmux convention
        // already prefix with `@` (e.g. `@AF_SESSION`).
        self.set_env(session, key, value)
    }

    /// List all cmux workspaces in the current window.
    ///
    /// Maps to `cmux list-workspaces`. The `attached` field is `true` for the
    /// workspace currently marked with `[selected]` in the cmux output.
    fn list_sessions(&self) -> anyhow::Result<Vec<SessionInfo>> {
        let raw = self
            .run_output(&["list-workspaces"])
            .context("cmux list-workspaces")?;
        Ok(parse_list_workspaces(&raw))
    }

    /// Split the active pane of the named workspace horizontally (right split).
    ///
    /// Maps to `cmux new-split right --workspace <ref> && cmux send … <cmd>\n`.
    ///
    /// The new split is created to the right of the current surface, then the
    /// given command is sent as keystrokes.
    fn split_horizontal(&self, session: &str, cmd: &str, _cwd: &Path) -> anyhow::Result<()> {
        let ws_ref = self
            .workspace_ref_for(session)?
            .with_context(|| format!("cmux workspace {session:?} not found"))?;
        self.run_status(&["new-split", "right", "--workspace", &ws_ref])
            .with_context(|| format!("cmux new-split right failed for {session:?}"))?;
        // Send the command to the newly-focused surface.
        let text = format!("{cmd}\\n");
        self.run_status(&["send", "--workspace", &ws_ref, "--", &text])
            .with_context(|| format!("cmux send command failed for {session:?}"))
    }

    /// Create a new surface below the current one in the named workspace.
    ///
    /// Maps to `cmux new-split down --workspace <ref>`.
    ///
    /// Returns the new surface ref (e.g. `surface:7`) as the pane identifier.
    ///
    /// ## cmux deviation
    ///
    /// cmux's `new-split` does not output the new surface ref. This implementation
    /// calls `cmux identify` immediately after the split to retrieve the focused
    /// surface ref (which cmux focuses automatically after a split). This is
    /// inherently racy in concurrent usage but matches tmux's best-effort
    /// pane-ID capture.
    fn create_pane(&self, session: &str, _cwd: &Path) -> anyhow::Result<String> {
        let ws_ref = self
            .workspace_ref_for(session)?
            .with_context(|| format!("cmux workspace {session:?} not found"))?;
        self.run_status(&["new-split", "down", "--workspace", &ws_ref])
            .with_context(|| format!("cmux new-split down failed for {session:?}"))?;
        // Retrieve the newly-focused surface ref.
        let json_raw = self.run_output(&["identify", "--no-caller"])?;
        let identity: serde_json::Value =
            serde_json::from_str(&json_raw).context("parse cmux identify output")?;
        identity
            .pointer("/focused/surface_ref")
            .and_then(|v| v.as_str())
            .map(ToOwned::to_owned)
            .context("cmux identify did not return focused surface_ref after new-split")
    }

    /// Send text followed by Enter to a specific surface within a workspace.
    ///
    /// Maps to `cmux send --workspace <ref> --surface <pane> "<keys>\n"`.
    fn send_keys_to_pane(
        &self,
        session: &str,
        pane: &str,
        keys: &str,
    ) -> anyhow::Result<()> {
        let ws_ref = self
            .workspace_ref_for(session)?
            .with_context(|| format!("cmux workspace {session:?} not found"))?;
        let text = format!("{keys}\\n");
        self.run_status(&[
            "send",
            "--workspace",
            &ws_ref,
            "--surface",
            pane,
            "--",
            &text,
        ])
        .with_context(|| format!("cmux send to surface {pane:?} failed for {session:?}"))
    }

    /// Close a specific surface within a workspace.
    ///
    /// Maps to `cmux close-surface --workspace <ref> --surface <pane>`.
    fn kill_pane(&self, session: &str, pane: &str) -> anyhow::Result<()> {
        let ws_ref = self
            .workspace_ref_for(session)?
            .with_context(|| format!("cmux workspace {session:?} not found"))?;
        self.run_status(&[
            "close-surface",
            "--workspace",
            &ws_ref,
            "--surface",
            pane,
        ])
        .with_context(|| format!("cmux close-surface {pane:?} failed for {session:?}"))
    }

    /// List all pane refs in a workspace.
    ///
    /// Maps to `cmux list-panes --workspace <ref>`.
    ///
    /// Returns pane refs such as `["pane:1", "pane:2"]`.
    fn list_panes(&self, session: &str) -> anyhow::Result<Vec<String>> {
        let ws_ref = self
            .workspace_ref_for(session)?
            .with_context(|| format!("cmux workspace {session:?} not found"))?;
        let raw = self
            .run_output(&["list-panes", "--workspace", &ws_ref])
            .context("cmux list-panes")?;
        Ok(parse_list_panes(&raw))
    }
}

// ---------------------------------------------------------------------------
// Parsing helpers (pure functions — easily unit-tested without spawning cmux)
// ---------------------------------------------------------------------------

/// Parse `cmux list-workspaces` output into a workspace name → ref map.
///
/// Output format (one line per workspace):
/// ```text
///   workspace:2  af
/// * workspace:5  my session  [selected]
///   workspace:1  main
/// ```
///
/// Lines begin with `*` (focused) or spaces. The ref is the first token; the
/// name is everything between the ref and optional trailing flags like
/// `[selected]`.
pub(crate) fn parse_list_workspaces(output: &str) -> Vec<SessionInfo> {
    output
        .lines()
        .filter_map(|raw_line| {
            let line = raw_line.trim_start_matches(['*', ' ']);
            // Split on two-or-more spaces to separate ref from name.
            let mut parts = line.splitn(2, "  ");
            let _ref_token = parts.next()?.trim();
            let rest = parts.next()?.trim();
            // Strip trailing flags such as `[selected]`, `⠐ …`
            let name = rest
                .split_first_token_if('[')
                .unwrap_or(rest)
                .trim()
                .to_owned();
            let attached = raw_line.trim_start().starts_with('*');
            if name.is_empty() {
                return None;
            }
            Some(SessionInfo { name, attached })
        })
        .collect()
}

/// Resolve a workspace `workspace:N` ref from the `list-workspaces` output
/// for the given session name.
pub(crate) fn parse_workspace_ref(output: &str, name: &str) -> Option<String> {
    for raw_line in output.lines() {
        let line = raw_line.trim_start_matches(['*', ' ']);
        let mut parts = line.splitn(2, "  ");
        let ref_token = parts.next()?.trim().to_owned();
        let rest = parts.next()?.trim();
        let ws_name = rest
            .split_first_token_if('[')
            .unwrap_or(rest)
            .trim();
        if ws_name == name {
            return Some(ref_token);
        }
    }
    None
}

/// Resolve the workspace name for a given ref (e.g. `workspace:5`).
pub(crate) fn parse_workspace_name_for_ref(output: &str, ws_ref: &str) -> Option<String> {
    for raw_line in output.lines() {
        let line = raw_line.trim_start_matches(['*', ' ']);
        let mut parts = line.splitn(2, "  ");
        let ref_token = parts.next()?.trim();
        let rest = parts.next()?.trim();
        if ref_token == ws_ref {
            let name = rest
                .split_first_token_if('[')
                .unwrap_or(rest)
                .trim()
                .to_owned();
            return if name.is_empty() { None } else { Some(name) };
        }
    }
    None
}

/// Parse `cmux list-panes` output into a list of pane ref strings.
///
/// Output format:
/// ```text
/// * pane:3  [2 surfaces]  [focused]
///   pane:1  [1 surface]
/// ```
pub(crate) fn parse_list_panes(output: &str) -> Vec<String> {
    output
        .lines()
        .filter_map(|raw_line| {
            let line = raw_line.trim_start_matches(['*', ' ']);
            // First token before whitespace is the pane ref.
            line.split_whitespace().next().map(ToOwned::to_owned)
        })
        .filter(|s| !s.is_empty())
        .collect()
}

// ---------------------------------------------------------------------------
// Utility
// ---------------------------------------------------------------------------

/// Split `s` at the first `[` and return the left portion (without the `[`),
/// or `None` if the delimiter is not present.
trait SplitFirstTokenIf {
    fn split_first_token_if(&self, delimiter: char) -> Option<&str>;
}

impl SplitFirstTokenIf for str {
    fn split_first_token_if(&self, delimiter: char) -> Option<&str> {
        self.split_once(delimiter).map(|(left, _)| left)
    }
}

/// Compute the default sidecar data directory.
///
/// Uses `$XDG_DATA_HOME/af/cmux/` if set; otherwise `~/.local/share/af/cmux/`.
fn default_data_dir() -> PathBuf {
    if let Ok(xdg) = std::env::var("XDG_DATA_HOME") {
        PathBuf::from(xdg).join("af").join("cmux")
    } else {
        dirs::data_local_dir()
            .unwrap_or_else(|| PathBuf::from("~/.local/share"))
            .join("af")
            .join("cmux")
    }
}

// ---------------------------------------------------------------------------
// Unit tests
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::*;
    use std::path::PathBuf;
    use tempfile::TempDir;

    // ------------------------------------------------------------------
    // Helpers
    // ------------------------------------------------------------------

    /// Build a `CmuxMultiplexer` that writes sidecars into a temp directory.
    fn mux_with_tempdir(tmp: &TempDir) -> CmuxMultiplexer {
        CmuxMultiplexer::with_binary("cmux", tmp.path())
    }

    // ------------------------------------------------------------------
    // parse_list_workspaces
    // ------------------------------------------------------------------

    #[test]
    fn test_parse_list_workspaces_normal() {
        let output = "  workspace:2  af\n  workspace:4  dotfiles\n* workspace:5  my session  [selected]\n  workspace:1  main";
        let sessions = parse_list_workspaces(output);
        assert_eq!(sessions.len(), 4);
        assert_eq!(sessions[0].name, "af");
        assert!(!sessions[0].attached);
        assert_eq!(sessions[1].name, "dotfiles");
        assert_eq!(sessions[2].name, "my session");
        assert!(sessions[2].attached, "starred line should be attached");
        assert_eq!(sessions[3].name, "main");
        assert!(!sessions[3].attached);
    }

    #[test]
    fn test_parse_list_workspaces_empty_output() {
        let sessions = parse_list_workspaces("");
        assert!(sessions.is_empty());
    }

    #[test]
    fn test_parse_list_workspaces_single_selected() {
        let output = "* workspace:1  only  [selected]";
        let sessions = parse_list_workspaces(output);
        assert_eq!(sessions.len(), 1);
        assert_eq!(sessions[0].name, "only");
        assert!(sessions[0].attached);
    }

    #[test]
    fn test_parse_list_workspaces_name_with_spaces() {
        let output = "  workspace:3  my complex session name";
        let sessions = parse_list_workspaces(output);
        assert_eq!(sessions.len(), 1);
        assert_eq!(sessions[0].name, "my complex session name");
    }

    // ------------------------------------------------------------------
    // parse_workspace_ref
    // ------------------------------------------------------------------

    #[test]
    fn test_parse_workspace_ref_found() {
        let output = "  workspace:2  af\n  workspace:4  dotfiles\n* workspace:5  my session  [selected]";
        assert_eq!(
            parse_workspace_ref(output, "af"),
            Some("workspace:2".to_owned())
        );
        assert_eq!(
            parse_workspace_ref(output, "dotfiles"),
            Some("workspace:4".to_owned())
        );
        assert_eq!(
            parse_workspace_ref(output, "my session"),
            Some("workspace:5".to_owned())
        );
    }

    #[test]
    fn test_parse_workspace_ref_not_found() {
        let output = "  workspace:2  af\n  workspace:4  dotfiles";
        assert_eq!(parse_workspace_ref(output, "nonexistent"), None);
    }

    #[test]
    fn test_parse_workspace_ref_empty_output() {
        assert_eq!(parse_workspace_ref("", "any"), None);
    }

    // ------------------------------------------------------------------
    // parse_workspace_name_for_ref
    // ------------------------------------------------------------------

    #[test]
    fn test_parse_workspace_name_for_ref_found() {
        let output = "  workspace:2  af\n* workspace:5  active-session  [selected]";
        assert_eq!(
            parse_workspace_name_for_ref(output, "workspace:5"),
            Some("active-session".to_owned())
        );
    }

    #[test]
    fn test_parse_workspace_name_for_ref_not_found() {
        let output = "  workspace:2  af";
        assert_eq!(parse_workspace_name_for_ref(output, "workspace:99"), None);
    }

    // ------------------------------------------------------------------
    // parse_list_panes
    // ------------------------------------------------------------------

    #[test]
    fn test_parse_list_panes_normal() {
        let output = "* pane:3  [2 surfaces]  [focused]\n  pane:1  [1 surface]";
        let panes = parse_list_panes(output);
        assert_eq!(panes, vec!["pane:3", "pane:1"]);
    }

    #[test]
    fn test_parse_list_panes_empty() {
        let panes = parse_list_panes("");
        assert!(panes.is_empty());
    }

    #[test]
    fn test_parse_list_panes_single() {
        let output = "* pane:8  [3 surfaces]  [focused]";
        let panes = parse_list_panes(output);
        assert_eq!(panes, vec!["pane:8"]);
    }

    // ------------------------------------------------------------------
    // is_inside_session
    // ------------------------------------------------------------------

    #[test]
    fn test_is_inside_session_without_env_var() {
        // CMUX_WORKSPACE_ID is not set in a normal test run.
        // We can only verify the method doesn't panic; actual value depends on env.
        let mux = CmuxMultiplexer::new();
        let _result = mux.is_inside_session();
        // No panic is the assertion.
    }

    // ------------------------------------------------------------------
    // is_available
    // ------------------------------------------------------------------

    #[test]
    fn test_is_available_detects_app_bundle() {
        let mux = CmuxMultiplexer::new();
        // On the developer's machine the app bundle is present; in CI it may not be.
        // We verify the method doesn't panic.
        let _available = mux.is_available();
    }

    // ------------------------------------------------------------------
    // Sidecar — set_env / get_env round-trip
    // ------------------------------------------------------------------

    #[test]
    fn test_set_and_get_env_roundtrip() {
        let tmp = TempDir::new().unwrap();
        let mux = mux_with_tempdir(&tmp);

        mux.set_env("my-session", "AF_SESSION_ID", "abc-123").unwrap();
        let val = mux.get_env("my-session", "AF_SESSION_ID").unwrap();
        assert_eq!(val, Some("abc-123".to_owned()));
    }

    #[test]
    fn test_get_env_returns_none_for_missing_key() {
        let tmp = TempDir::new().unwrap();
        let mux = mux_with_tempdir(&tmp);

        let val = mux.get_env("no-session", "MISSING_KEY").unwrap();
        assert_eq!(val, None);
    }

    #[test]
    fn test_set_env_overwrites_existing_value() {
        let tmp = TempDir::new().unwrap();
        let mux = mux_with_tempdir(&tmp);

        mux.set_env("sess", "KEY", "first").unwrap();
        mux.set_env("sess", "KEY", "second").unwrap();
        let val = mux.get_env("sess", "KEY").unwrap();
        assert_eq!(val, Some("second".to_owned()));
    }

    #[test]
    fn test_set_env_multiple_keys() {
        let tmp = TempDir::new().unwrap();
        let mux = mux_with_tempdir(&tmp);

        mux.set_env("sess", "A", "1").unwrap();
        mux.set_env("sess", "B", "2").unwrap();
        assert_eq!(mux.get_env("sess", "A").unwrap(), Some("1".to_owned()));
        assert_eq!(mux.get_env("sess", "B").unwrap(), Some("2".to_owned()));
    }

    #[test]
    fn test_set_option_and_get_env_roundtrip() {
        let tmp = TempDir::new().unwrap();
        let mux = mux_with_tempdir(&tmp);

        // tmux callers use @AF_SESSION as the key.
        mux.set_option("sess", "@AF_SESSION", "true").unwrap();
        let val = mux.get_env("sess", "@AF_SESSION").unwrap();
        assert_eq!(val, Some("true".to_owned()));
    }

    #[test]
    fn test_sidecar_isolated_per_session() {
        let tmp = TempDir::new().unwrap();
        let mux = mux_with_tempdir(&tmp);

        mux.set_env("session-a", "KEY", "value-a").unwrap();
        mux.set_env("session-b", "KEY", "value-b").unwrap();

        assert_eq!(
            mux.get_env("session-a", "KEY").unwrap(),
            Some("value-a".to_owned())
        );
        assert_eq!(
            mux.get_env("session-b", "KEY").unwrap(),
            Some("value-b".to_owned())
        );
    }

    // ------------------------------------------------------------------
    // Sidecar — path derivation
    // ------------------------------------------------------------------

    #[test]
    fn test_sidecar_path_uses_session_name() {
        let tmp = TempDir::new().unwrap();
        let mux = mux_with_tempdir(&tmp);
        let path = mux.sidecar_path("my-session");
        assert_eq!(path, PathBuf::from(tmp.path()).join("my-session.env.json"));
    }

    // ------------------------------------------------------------------
    // resolved_binary
    // ------------------------------------------------------------------

    #[test]
    fn test_resolved_binary_custom_path_returned_as_is() {
        let mux = CmuxMultiplexer::with_binary("/usr/local/bin/my-cmux", PathBuf::from("/tmp"));
        assert_eq!(mux.resolved_binary(), "/usr/local/bin/my-cmux");
    }

    // ------------------------------------------------------------------
    // Default constructor
    // ------------------------------------------------------------------

    #[test]
    fn test_default_constructor_matches_new() {
        let mux1 = CmuxMultiplexer::new();
        let mux2 = CmuxMultiplexer::default();
        assert_eq!(mux1.binary, mux2.binary);
        assert_eq!(mux1.data_dir, mux2.data_dir);
    }

    // ------------------------------------------------------------------
    // Split_first_token_if helper
    // ------------------------------------------------------------------

    #[test]
    fn test_split_first_token_if_present() {
        let s = "my session name  [selected]";
        assert_eq!(s.split_first_token_if('['), Some("my session name  "));
    }

    #[test]
    fn test_split_first_token_if_absent() {
        let s = "my session name";
        assert_eq!(s.split_first_token_if('['), None);
    }

    // ------------------------------------------------------------------
    // SessionInfo structural tests
    // ------------------------------------------------------------------

    #[test]
    fn test_session_info_debug_and_clone() {
        let info = SessionInfo {
            name: "cmux-test".to_owned(),
            attached: true,
        };
        let cloned = info.clone();
        assert_eq!(cloned.name, "cmux-test");
        assert!(cloned.attached);
        assert!(format!("{info:?}").contains("cmux-test"));
    }
}
