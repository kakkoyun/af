//! Obsidian workstream note integration (ADR-007).
//!
//! Creates and manages Obsidian markdown notes for each workstream session.
//! Notes are created on `af create` and their frontmatter is updated on `af done`.
//! The `af note` command opens the note in the system editor or Obsidian.

use std::path::{Path, PathBuf};

use anyhow::{Context, Result};
use chrono::{DateTime, Utc};
use tracing::debug;

use crate::config::ObsidianConfig;

/// Frontmatter fields for a workstream note.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct NoteMeta {
    /// Session name.
    pub session: String,
    /// Git branch name.
    pub branch: String,
    /// Base branch the session forked from.
    pub base_branch: String,
    /// Repository name.
    pub repo: String,
    /// Agent provider name.
    pub agent: String,
    /// Current status: "active", "completed", "abandoned".
    pub status: String,
    /// When the session was created.
    pub created_at: DateTime<Utc>,
    /// When the session was completed (if applicable).
    pub completed_at: Option<DateTime<Utc>>,
}

/// Resolve the path where a workstream note should be created.
///
/// Returns `<vault>/<folder>/<session>.md`. Creates directories as needed.
pub fn note_path(config: &ObsidianConfig, session_name: &str) -> Result<PathBuf> {
    let vault = shellexpand_tilde(&config.vault);
    let vault_path = PathBuf::from(&vault);

    if !vault_path.exists() {
        anyhow::bail!(
            "Obsidian vault not found at {}. Set obsidian.vault in config.",
            vault_path.display()
        );
    }

    let folder = if config.folder.is_empty() {
        "Workstreams"
    } else {
        &config.folder
    };

    let dir = vault_path.join(folder);
    std::fs::create_dir_all(&dir)
        .with_context(|| format!("failed to create note directory {}", dir.display()))?;

    Ok(dir.join(format!("{session_name}.md")))
}

/// Create a workstream note with YAML frontmatter.
pub fn create_note(path: &Path, meta: &NoteMeta) -> Result<()> {
    if path.exists() {
        debug!(path = %path.display(), "note already exists, skipping creation");
        return Ok(());
    }

    let content = render_note(meta);
    debug!(path = %path.display(), "creating workstream note");
    std::fs::write(path, content)
        .with_context(|| format!("failed to write note at {}", path.display()))
}

/// Update the frontmatter status field in an existing note.
pub fn update_status(path: &Path, status: &str, completed_at: Option<DateTime<Utc>>) -> Result<()> {
    if !path.exists() {
        debug!(path = %path.display(), "note does not exist, skipping update");
        return Ok(());
    }

    let content = std::fs::read_to_string(path)
        .with_context(|| format!("failed to read note at {}", path.display()))?;

    let updated = replace_frontmatter_field(&content, "status", status);
    let updated = if let Some(ts) = completed_at {
        replace_frontmatter_field(&updated, "completed_at", &ts.to_rfc3339())
    } else {
        updated
    };

    debug!(path = %path.display(), status, "updating note frontmatter");
    std::fs::write(path, updated)
        .with_context(|| format!("failed to update note at {}", path.display()))
}

/// Open a note in the system editor or Obsidian URI scheme.
pub fn open_note(path: &Path) -> Result<()> {
    if !path.exists() {
        anyhow::bail!("note does not exist at {}", path.display());
    }

    // Try Obsidian URI scheme first, fall back to $EDITOR.
    let obsidian_uri = format!("obsidian://open?path={}", path.display());
    let opened = std::process::Command::new("xdg-open")
        .arg(&obsidian_uri)
        .status()
        .is_ok_and(|s| s.success());

    if !opened {
        // Fall back to $EDITOR.
        let editor = std::env::var("EDITOR").unwrap_or_else(|_| "nvim".to_owned());
        std::process::Command::new(&editor)
            .arg(path)
            .status()
            .with_context(|| format!("failed to open {}", path.display()))?;
    }

    Ok(())
}

/// Render a note as a markdown string with YAML frontmatter.
pub fn render_note(meta: &NoteMeta) -> String {
    use std::fmt::Write;

    let mut s = String::from("---\n");
    let _ = writeln!(s, "session: \"{}\"", meta.session);
    let _ = writeln!(s, "branch: \"{}\"", meta.branch);
    let _ = writeln!(s, "base_branch: \"{}\"", meta.base_branch);
    let _ = writeln!(s, "repo: \"{}\"", meta.repo);
    let _ = writeln!(s, "agent: \"{}\"", meta.agent);
    let _ = writeln!(s, "status: \"{}\"", meta.status);
    let _ = writeln!(s, "created_at: \"{}\"", meta.created_at.to_rfc3339());
    if let Some(completed) = meta.completed_at {
        let _ = writeln!(s, "completed_at: \"{}\"", completed.to_rfc3339());
    }
    s.push_str("---\n\n");
    let _ = writeln!(s, "# {}\n", meta.session);
    s.push_str("## Context\n\n");
    let _ = writeln!(s, "- **Repo:** {}", meta.repo);
    let _ = writeln!(s, "- **Branch:** `{}`", meta.branch);
    let _ = writeln!(s, "- **Base:** `{}`", meta.base_branch);
    let _ = writeln!(s, "- **Agent:** {}\n", meta.agent);
    s.push_str("## Notes\n\n");
    s.push_str("<!-- Add your notes here -->\n\n");
    s.push_str("## Decisions\n\n");
    s.push_str("## Outcome\n\n");
    s
}

/// Replace a frontmatter field value in a markdown string.
///
/// Finds `key: "old_value"` in the YAML frontmatter block and replaces with `key: "new_value"`.
pub fn replace_frontmatter_field(content: &str, key: &str, new_value: &str) -> String {
    let prefix = format!("{key}: \"");
    let mut result = String::new();
    let mut in_frontmatter = false;
    let mut frontmatter_ended = false;

    for line in content.lines() {
        if line == "---" && !frontmatter_ended {
            if in_frontmatter {
                frontmatter_ended = true;
            } else {
                in_frontmatter = true;
            }
            result.push_str(line);
            result.push('\n');
            continue;
        }

        if in_frontmatter && !frontmatter_ended && line.starts_with(&prefix) {
            use std::fmt::Write;
            let _ = writeln!(result, "{key}: \"{new_value}\"");
        } else {
            result.push_str(line);
            result.push('\n');
        }
    }

    result
}

/// Expand `~` at the start of a path to the home directory.
fn shellexpand_tilde(path: &str) -> String {
    if let Some(rest) = path.strip_prefix("~/") {
        if let Some(home) = dirs::home_dir() {
            return home.join(rest).display().to_string();
        }
    }
    path.to_owned()
}

#[cfg(test)]
mod tests {
    use super::*;
    use chrono::TimeZone;
    use tempfile::TempDir;

    fn sample_meta() -> NoteMeta {
        NoteMeta {
            session: "fix-auth-bug".to_owned(),
            branch: "kakkoyun/fix-auth-bug".to_owned(),
            base_branch: "main".to_owned(),
            repo: "my-repo".to_owned(),
            agent: "claude".to_owned(),
            status: "active".to_owned(),
            created_at: Utc.with_ymd_and_hms(2026, 4, 10, 14, 0, 0).unwrap(),
            completed_at: None,
        }
    }

    #[test]
    fn test_render_note_has_frontmatter() {
        let note = render_note(&sample_meta());
        assert!(note.starts_with("---\n"));
        assert!(note.contains("session: \"fix-auth-bug\""));
        assert!(note.contains("branch: \"kakkoyun/fix-auth-bug\""));
        assert!(note.contains("status: \"active\""));
        assert!(note.contains("# fix-auth-bug"));
    }

    #[test]
    fn test_render_note_with_completed_at() {
        let mut meta = sample_meta();
        meta.completed_at = Some(Utc.with_ymd_and_hms(2026, 4, 10, 18, 0, 0).unwrap());
        let note = render_note(&meta);
        assert!(note.contains("completed_at:"));
    }

    #[test]
    fn test_render_note_without_completed_at() {
        let note = render_note(&sample_meta());
        assert!(!note.contains("completed_at:"));
    }

    #[test]
    fn test_replace_frontmatter_field_status() {
        let content = "---\nsession: \"test\"\nstatus: \"active\"\n---\n\n# test\n";
        let updated = replace_frontmatter_field(content, "status", "completed");
        assert!(updated.contains("status: \"completed\""));
        assert!(!updated.contains("status: \"active\""));
    }

    #[test]
    fn test_replace_frontmatter_field_preserves_other_fields() {
        let content = "---\nsession: \"test\"\nstatus: \"active\"\nagent: \"claude\"\n---\n";
        let updated = replace_frontmatter_field(content, "status", "completed");
        assert!(updated.contains("session: \"test\""));
        assert!(updated.contains("agent: \"claude\""));
    }

    #[test]
    fn test_replace_frontmatter_field_no_match() {
        let content = "---\nsession: \"test\"\n---\n";
        let updated = replace_frontmatter_field(content, "nonexistent", "value");
        assert_eq!(updated, content);
    }

    #[test]
    fn test_note_path_creates_directory() {
        let dir = TempDir::new().unwrap();
        let config = ObsidianConfig {
            vault: dir.path().display().to_string(),
            folder: "Workstreams".to_owned(),
            enabled: true,
        };
        let path = note_path(&config, "test-session").unwrap();
        assert!(path.parent().unwrap().exists());
        assert_eq!(path.file_name().unwrap(), "test-session.md");
    }

    #[test]
    fn test_note_path_custom_folder() {
        let dir = TempDir::new().unwrap();
        let config = ObsidianConfig {
            vault: dir.path().display().to_string(),
            folder: "Projects/AF".to_owned(),
            enabled: true,
        };
        let path = note_path(&config, "my-session").unwrap();
        assert!(path.to_string_lossy().contains("Projects/AF"));
    }

    #[test]
    fn test_note_path_nonexistent_vault_fails() {
        let config = ObsidianConfig {
            vault: "/nonexistent/vault".to_owned(),
            folder: String::new(),
            enabled: true,
        };
        let result = note_path(&config, "test");
        assert!(result.is_err());
    }

    #[test]
    fn test_create_note_writes_file() {
        let dir = TempDir::new().unwrap();
        let path = dir.path().join("test.md");
        create_note(&path, &sample_meta()).unwrap();
        assert!(path.exists());
        let content = std::fs::read_to_string(&path).unwrap();
        assert!(content.contains("fix-auth-bug"));
    }

    #[test]
    fn test_create_note_skips_if_exists() {
        let dir = TempDir::new().unwrap();
        let path = dir.path().join("existing.md");
        std::fs::write(&path, "existing content").unwrap();
        create_note(&path, &sample_meta()).unwrap();
        // Should not overwrite.
        let content = std::fs::read_to_string(&path).unwrap();
        assert_eq!(content, "existing content");
    }

    #[test]
    fn test_update_status_changes_frontmatter() {
        let dir = TempDir::new().unwrap();
        let path = dir.path().join("note.md");
        create_note(&path, &sample_meta()).unwrap();
        update_status(&path, "completed", None).unwrap();
        let content = std::fs::read_to_string(&path).unwrap();
        assert!(content.contains("status: \"completed\""));
    }

    #[test]
    fn test_update_status_with_completed_at() {
        let dir = TempDir::new().unwrap();
        let path = dir.path().join("note.md");
        create_note(&path, &sample_meta()).unwrap();
        let ts = Utc.with_ymd_and_hms(2026, 4, 10, 18, 0, 0).unwrap();
        // First need to add completed_at field to the note.
        let mut content = std::fs::read_to_string(&path).unwrap();
        content = content.replace(
            "status: \"active\"",
            "status: \"active\"\ncompleted_at: \"\"",
        );
        std::fs::write(&path, &content).unwrap();
        update_status(&path, "completed", Some(ts)).unwrap();
        let updated = std::fs::read_to_string(&path).unwrap();
        assert!(updated.contains("status: \"completed\""));
        assert!(updated.contains("completed_at: \"2026"));
    }

    #[test]
    fn test_update_status_nonexistent_note_is_ok() {
        let result = update_status(Path::new("/nonexistent/note.md"), "completed", None);
        assert!(result.is_ok());
    }

    #[test]
    fn test_note_meta_debug_clone() {
        let meta = sample_meta();
        let cloned = meta.clone();
        assert_eq!(meta, cloned);
        let _debug = format!("{meta:?}");
    }
}
