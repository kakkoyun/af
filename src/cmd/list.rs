//! `af list` — show active workstreams.
//!
//! Lists all sessions from the session store, grouped by repository.

use anyhow::{Context, Result};
use std::collections::BTreeMap;

use crate::session::store::SessionStore;
use crate::session::types::SessionState;

/// Execute the `af list` command.
pub fn run() -> Result<()> {
    let store = SessionStore::default_location().context("cannot determine data directory")?;

    let names = store.list().unwrap_or_default();
    if names.is_empty() {
        #[allow(clippy::print_stdout)]
        {
            println!("No active workstreams.");
        }
        return Ok(());
    }

    // Group sessions by repo (from git_root basename, or "workspace").
    let mut by_repo: BTreeMap<String, Vec<SessionState>> = BTreeMap::new();
    for name in &names {
        if let Ok(state) = store.load(name) {
            let repo = state
                .worktree
                .as_ref()
                .and_then(|wt| {
                    std::path::Path::new(&wt.git_root)
                        .file_name()
                        .map(|n| n.to_string_lossy().into_owned())
                })
                .unwrap_or_else(|| "workspace".to_owned());
            by_repo.entry(repo).or_default().push(state);
        }
    }

    // Detect current repo for marking.
    let current_repo = std::process::Command::new("git")
        .args(["rev-parse", "--show-toplevel"])
        .output()
        .ok()
        .filter(|o| o.status.success())
        .and_then(|o| {
            let s = String::from_utf8_lossy(&o.stdout).trim().to_owned();
            std::path::Path::new(&s)
                .file_name()
                .map(|n| n.to_string_lossy().into_owned())
        });

    #[allow(clippy::print_stdout)]
    for (repo, sessions) in &by_repo {
        let marker = if current_repo.as_deref() == Some(repo.as_str()) {
            " (current)"
        } else {
            ""
        };
        println!("── {repo}{marker} ──────────────────────────────────");
        for state in sessions {
            let branch = state.worktree.as_ref().map_or("-", |wt| &wt.branch);
            let mode = format!("{:?}", state.execution.mode).to_lowercase();
            let agents: Vec<&str> = state.agents.iter().map(|a| a.provider.as_str()).collect();
            println!(
                "  {:<28} branch={:<24} [{}] agents={}",
                state.session.name,
                branch,
                mode,
                agents.join(","),
            );
        }
    }

    Ok(())
}
