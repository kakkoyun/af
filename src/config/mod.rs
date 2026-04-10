//! Layered configuration system (ADR-003).
//!
//! Configuration is loaded from multiple sources with increasing precedence:
//! compiled defaults → user config → project config → env vars → CLI flags.
//!
//! Config files are TOML. User config lives at `~/.config/af/config.toml`,
//! project config at `<repo>/.af/config.toml`.

use serde::{Deserialize, Serialize};
use std::path::{Path, PathBuf};

/// Top-level configuration.
#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq)]
#[serde(default)]
pub struct Config {
    /// General settings.
    pub general: GeneralConfig,
    /// Branch naming settings.
    pub branch: BranchConfig,
    /// Editor settings.
    pub editor: EditorConfig,
    /// Session lifecycle settings.
    pub lifecycle: LifecycleConfig,
    /// Remote provisioning settings.
    pub provisioning: ProvisioningConfig,
    /// Obsidian integration settings.
    pub obsidian: ObsidianConfig,
}

/// General settings.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
#[serde(default)]
pub struct GeneralConfig {
    /// Default AI agent to launch (e.g., "claude", "pi").
    pub default_agent: String,
    /// Terminal multiplexer to use (e.g., "tmux", "zellij").
    pub multiplexer: String,
    /// Maximum concurrent af sessions.
    pub max_sessions: u32,
    /// Root directory for worktrees.
    pub worktree_root: String,
}

/// Branch naming settings.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
#[serde(default)]
pub struct BranchConfig {
    /// Username prefix for branches in fork repos (e.g., "kakkoyun").
    pub prefix: String,
    /// Only apply prefix when an `upstream` remote exists.
    pub prefix_on_fork_only: bool,
}

/// Editor settings.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
#[serde(default)]
pub struct EditorConfig {
    /// Terminal editor (fallback: $EDITOR, then "nvim").
    pub terminal: String,
    /// Visual editor (empty = auto-detect: code > zed).
    pub visual: String,
}

/// Session lifecycle settings.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
#[serde(default)]
pub struct LifecycleConfig {
    /// Days to retain archived session data after `af done`.
    pub retention_days: u32,
    /// Automatically archive completed sessions.
    pub auto_archive: bool,
}

/// Remote provisioning settings (ADR-009).
#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq)]
#[serde(default)]
pub struct ProvisioningConfig {
    /// Dotfiles configuration for remote VMs.
    pub dotfiles: DotfilesConfig,
}

/// Dotfiles provisioning configuration.
#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq)]
#[serde(default)]
pub struct DotfilesConfig {
    /// Git repo to clone on remote VMs (e.g., `https://github.com/you/dotfiles.git`).
    pub repo: String,
    /// Directory to clone into on the remote (default: "~/.dotfiles").
    pub target: String,
    /// Command to run after cloning (within the cloned directory).
    pub install_cmd: String,
}

/// Obsidian integration settings (ADR-007).
#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq)]
#[serde(default)]
pub struct ObsidianConfig {
    /// Path to the Obsidian vault (e.g., "~/Vaults/work").
    pub vault: String,
    /// Subfolder within the vault for workstream notes.
    pub folder: String,
    /// Enable Obsidian note creation on `af create`.
    pub enabled: bool,
}

// ── Defaults ────────────────────────────────────────────────────────────────

impl Default for GeneralConfig {
    fn default() -> Self {
        Self {
            default_agent: String::from("claude"),
            multiplexer: String::from("tmux"),
            max_sessions: 10,
            worktree_root: String::from("~/Workspace/.worktrees"),
        }
    }
}

impl Default for BranchConfig {
    fn default() -> Self {
        Self {
            prefix: String::new(),
            prefix_on_fork_only: true,
        }
    }
}

impl Default for EditorConfig {
    fn default() -> Self {
        Self {
            terminal: String::from("nvim"),
            visual: String::new(),
        }
    }
}

impl Default for LifecycleConfig {
    fn default() -> Self {
        Self {
            retention_days: 90,
            auto_archive: true,
        }
    }
}

// ── Loading ─────────────────────────────────────────────────────────────────

/// Errors that can occur when loading configuration.
#[derive(Debug, thiserror::Error)]
pub enum ConfigError {
    /// Failed to read a config file.
    #[error("failed to read config file {path}: {source}")]
    ReadFile {
        /// Path that could not be read.
        path: PathBuf,
        /// Underlying I/O error.
        source: std::io::Error,
    },
    /// Failed to parse TOML content.
    #[error("failed to parse config file {path}: {source}")]
    ParseToml {
        /// Path that could not be parsed.
        path: PathBuf,
        /// Underlying TOML error.
        source: toml::de::Error,
    },
}

/// Return the user config file path: `~/.config/af/config.toml`.
///
/// Uses `dirs::config_dir()` for XDG compliance.
/// Returns `None` if the home directory cannot be determined.
pub fn user_config_path() -> Option<PathBuf> {
    dirs::config_dir().map(|d| d.join("af").join("config.toml"))
}

/// Return the project config file path: `<dir>/.af/config.toml`.
pub fn project_config_path(project_dir: &Path) -> PathBuf {
    project_dir.join(".af").join("config.toml")
}

/// Load configuration from a TOML file.
///
/// Returns `Ok(None)` if the file does not exist.
/// Returns `Err` if the file exists but cannot be read or parsed.
pub fn load_from_file(path: &Path) -> Result<Option<Config>, ConfigError> {
    if !path.exists() {
        return Ok(None);
    }
    let content = std::fs::read_to_string(path).map_err(|e| ConfigError::ReadFile {
        path: path.to_path_buf(),
        source: e,
    })?;
    let config: Config = toml::from_str(&content).map_err(|e| ConfigError::ParseToml {
        path: path.to_path_buf(),
        source: e,
    })?;
    Ok(Some(config))
}

/// Load the effective configuration by merging layers.
///
/// Precedence (highest wins):
/// 1. Project config (`.af/config.toml` in `project_dir`)
/// 2. User config (`~/.config/af/config.toml`)
/// 3. Compiled defaults
///
/// Missing files are silently skipped. Parse errors are propagated.
pub fn load(project_dir: Option<&Path>) -> Result<Config, ConfigError> {
    let mut config = Config::default();

    // Layer 1: user config
    if let Some(user_path) = user_config_path() {
        if let Some(user_config) = load_from_file(&user_path)? {
            merge(&mut config, &user_config);
        }
    }

    // Layer 2: project config
    if let Some(dir) = project_dir {
        let project_path = project_config_path(dir);
        if let Some(project_config) = load_from_file(&project_path)? {
            merge(&mut config, &project_config);
        }
    }

    Ok(config)
}

/// Merge `overlay` into `base`. Non-default values in `overlay` overwrite `base`.
///
/// A value is considered "non-default" if it differs from the type's `Default`.
/// This allows partial config files to override only the fields they specify.
fn merge(base: &mut Config, overlay: &Config) {
    let default = Config::default();

    // General
    if overlay.general.default_agent != default.general.default_agent {
        base.general
            .default_agent
            .clone_from(&overlay.general.default_agent);
    }
    if overlay.general.multiplexer != default.general.multiplexer {
        base.general
            .multiplexer
            .clone_from(&overlay.general.multiplexer);
    }
    if overlay.general.max_sessions != default.general.max_sessions {
        base.general.max_sessions = overlay.general.max_sessions;
    }
    if overlay.general.worktree_root != default.general.worktree_root {
        base.general
            .worktree_root
            .clone_from(&overlay.general.worktree_root);
    }

    // Branch
    if overlay.branch.prefix != default.branch.prefix {
        base.branch.prefix.clone_from(&overlay.branch.prefix);
    }
    if overlay.branch.prefix_on_fork_only != default.branch.prefix_on_fork_only {
        base.branch.prefix_on_fork_only = overlay.branch.prefix_on_fork_only;
    }

    // Editor
    if overlay.editor.terminal != default.editor.terminal {
        base.editor.terminal.clone_from(&overlay.editor.terminal);
    }
    if overlay.editor.visual != default.editor.visual {
        base.editor.visual.clone_from(&overlay.editor.visual);
    }

    // Lifecycle
    if overlay.lifecycle.retention_days != default.lifecycle.retention_days {
        base.lifecycle.retention_days = overlay.lifecycle.retention_days;
    }
    if overlay.lifecycle.auto_archive != default.lifecycle.auto_archive {
        base.lifecycle.auto_archive = overlay.lifecycle.auto_archive;
    }

    // Provisioning
    if overlay.provisioning.dotfiles.repo != default.provisioning.dotfiles.repo {
        base.provisioning
            .dotfiles
            .repo
            .clone_from(&overlay.provisioning.dotfiles.repo);
    }
    if overlay.provisioning.dotfiles.target != default.provisioning.dotfiles.target {
        base.provisioning
            .dotfiles
            .target
            .clone_from(&overlay.provisioning.dotfiles.target);
    }
    if overlay.provisioning.dotfiles.install_cmd != default.provisioning.dotfiles.install_cmd {
        base.provisioning
            .dotfiles
            .install_cmd
            .clone_from(&overlay.provisioning.dotfiles.install_cmd);
    }

    // Obsidian
    if overlay.obsidian.vault != default.obsidian.vault {
        base.obsidian.vault.clone_from(&overlay.obsidian.vault);
    }
    if overlay.obsidian.folder != default.obsidian.folder {
        base.obsidian.folder.clone_from(&overlay.obsidian.folder);
    }
    if overlay.obsidian.enabled != default.obsidian.enabled {
        base.obsidian.enabled = overlay.obsidian.enabled;
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::TempDir;

    #[test]
    fn test_default_config_has_expected_values() {
        let config = Config::default();
        assert_eq!(config.general.default_agent, "claude");
        assert_eq!(config.general.multiplexer, "tmux");
        assert_eq!(config.general.max_sessions, 10);
        assert_eq!(config.general.worktree_root, "~/Workspace/.worktrees");
        assert!(config.branch.prefix.is_empty());
        assert!(config.branch.prefix_on_fork_only);
        assert_eq!(config.editor.terminal, "nvim");
        assert!(config.editor.visual.is_empty());
        assert_eq!(config.lifecycle.retention_days, 90);
        assert!(config.lifecycle.auto_archive);
    }

    #[test]
    fn test_load_from_nonexistent_file_returns_none() {
        let result = load_from_file(Path::new("/tmp/af-test-nonexistent-config.toml"));
        assert!(result.is_ok());
        assert!(result.unwrap().is_none());
    }

    #[test]
    fn test_load_from_valid_toml_file() {
        let dir = TempDir::new().unwrap();
        let path = dir.path().join("config.toml");
        std::fs::write(
            &path,
            r#"
[general]
default_agent = "pi"
max_sessions = 5
"#,
        )
        .unwrap();

        let config = load_from_file(&path).unwrap().unwrap();
        assert_eq!(config.general.default_agent, "pi");
        assert_eq!(config.general.max_sessions, 5);
        // Fields not in the file should get defaults
        assert_eq!(config.general.multiplexer, "tmux");
    }

    #[test]
    fn test_load_from_invalid_toml_returns_error() {
        let dir = TempDir::new().unwrap();
        let path = dir.path().join("bad.toml");
        std::fs::write(&path, "this is not valid toml [[[").unwrap();

        let result = load_from_file(&path);
        assert!(result.is_err());
        let err = result.unwrap_err();
        assert!(matches!(err, ConfigError::ParseToml { .. }));
    }

    #[test]
    fn test_load_from_empty_toml_returns_defaults() {
        let dir = TempDir::new().unwrap();
        let path = dir.path().join("empty.toml");
        std::fs::write(&path, "").unwrap();

        let config = load_from_file(&path).unwrap().unwrap();
        assert_eq!(config, Config::default());
    }

    #[test]
    fn test_merge_overlay_overrides_non_default_values() {
        let mut base = Config::default();
        let mut overlay = Config::default();
        overlay.general.default_agent = String::from("codex");
        overlay.branch.prefix = String::from("myuser");
        overlay.lifecycle.retention_days = 30;

        merge(&mut base, &overlay);

        assert_eq!(base.general.default_agent, "codex");
        assert_eq!(base.branch.prefix, "myuser");
        assert_eq!(base.lifecycle.retention_days, 30);
        // Unmodified fields remain at base defaults
        assert_eq!(base.general.multiplexer, "tmux");
        assert_eq!(base.general.max_sessions, 10);
    }

    #[test]
    fn test_merge_default_overlay_does_not_change_base() {
        let mut base = Config::default();
        base.general.default_agent = String::from("pi");
        let original = base.clone();

        let overlay = Config::default();
        merge(&mut base, &overlay);

        assert_eq!(base, original);
    }

    #[test]
    fn test_load_merges_user_and_project_configs() {
        let dir = TempDir::new().unwrap();

        // Simulate a project config
        let project_dir = dir.path().join("myrepo");
        let af_dir = project_dir.join(".af");
        std::fs::create_dir_all(&af_dir).unwrap();
        std::fs::write(
            af_dir.join("config.toml"),
            r#"
[general]
default_agent = "pi"
"#,
        )
        .unwrap();

        // load() with project_dir — user config likely doesn't exist, so
        // we get defaults + project overlay.
        let config = load(Some(&project_dir)).unwrap();
        assert_eq!(config.general.default_agent, "pi");
        assert_eq!(config.general.multiplexer, "tmux"); // default preserved
    }

    #[test]
    fn test_load_no_project_returns_defaults_or_user_config() {
        // Without a project dir, we get defaults (or user config if it exists).
        let config = load(None).unwrap();
        // Can't assert exact values since user config may exist,
        // but it should not error.
        assert!(!config.general.default_agent.is_empty());
    }

    #[test]
    fn test_roundtrip_serialize_deserialize() {
        let config = Config::default();
        let toml_str = toml::to_string_pretty(&config).unwrap();
        let parsed: Config = toml::from_str(&toml_str).unwrap();
        assert_eq!(config, parsed);
    }

    #[test]
    fn test_partial_toml_preserves_unset_defaults() {
        let toml_str = r#"
[editor]
terminal = "vim"
"#;
        let config: Config = toml::from_str(toml_str).unwrap();
        assert_eq!(config.editor.terminal, "vim");
        assert_eq!(config.editor.visual, ""); // default
        assert_eq!(config.general.default_agent, "claude"); // default
        assert_eq!(config.lifecycle.retention_days, 90); // default
    }

    #[test]
    fn test_user_config_path_returns_some() {
        // On any system with a home dir, this should return Some
        let path = user_config_path();
        if let Some(p) = &path {
            assert!(p.ends_with("af/config.toml"));
        }
        // On CI without a home dir, None is acceptable
    }

    #[test]
    fn test_project_config_path() {
        let path = project_config_path(Path::new("/home/user/myrepo"));
        assert_eq!(path, PathBuf::from("/home/user/myrepo/.af/config.toml"));
    }
}
