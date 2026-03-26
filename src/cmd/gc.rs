//! `af gc` — garbage collect merged/closed workstreams.
//!
//! Scans worktrees and session metadata for branches that have been merged
//! or had their PR closed. Offers to clean up the worktree, branch, and
//! session metadata.

use anyhow::{Context, Result};
use std::io::Write;

use crate::cli::GcArgs;
use crate::git::gc::{MergeStatus, detect_main_for_worktree, detect_merge_status};
use crate::git::worktree;
use crate::session::store::SessionStore;

/// Execute the `af gc` command.
pub fn run(args: &GcArgs) -> Result<()> {
    let store = SessionStore::default_location().context("cannot determine data directory")?;
    let sessions = store.list().unwrap_or_default();

    if sessions.is_empty() {
        #[allow(clippy::print_stdout)]
        {
            println!("No sessions to garbage collect.");
        }
        return Ok(());
    }

    let mut found = 0u32;
    let mut cleaned = 0u32;

    for name in &sessions {
        let Ok(state) = store.load(name) else {
            continue;
        };

        let Some(ref wt) = state.worktree else {
            continue;
        };

        let wt_path = std::path::Path::new(&wt.path);
        let git_root = std::path::Path::new(&wt.git_root);

        if !wt_path.exists() {
            continue;
        }

        let main_branch = detect_main_for_worktree(wt_path);
        let status = detect_merge_status(git_root, &wt.branch, &main_branch);

        found += 1;

        match status {
            MergeStatus::Merged => {
                #[allow(clippy::print_stdout)]
                {
                    println!("  \x1b[32m✓ merged\x1b[0m  {:<30}  {}", name, wt.branch);
                }
            }
            MergeStatus::Closed => {
                #[allow(clippy::print_stdout)]
                {
                    println!("  \x1b[33m⊘ closed\x1b[0m  {:<30}  {}", name, wt.branch);
                }
            }
            MergeStatus::Open => {
                #[allow(clippy::print_stdout)]
                {
                    println!("  \x1b[90m· open\x1b[0m    {:<30}  {}", name, wt.branch);
                }
                continue;
            }
        }

        if args.dry_run {
            #[allow(clippy::print_stdout)]
            {
                println!("    [dry-run] would clean: worktree + branch + session");
            }
            continue;
        }

        let do_clean = if args.all {
            true
        } else {
            #[allow(clippy::print_stderr)]
            {
                eprint!("    Clean up? [y/N] ");
            }
            std::io::stderr().flush()?;
            let mut reply = String::new();
            std::io::stdin().read_line(&mut reply)?;
            reply.trim().eq_ignore_ascii_case("y")
        };

        if do_clean {
            // Remove worktree and branch.
            if wt_path.exists() {
                let _ = worktree::remove(git_root, wt_path);
            }
            let _ = worktree::delete_branch(git_root, &wt.branch, true);

            // Clean up empty parent directory.
            if let Some(parent) = wt_path.parent() {
                let _ = std::fs::remove_dir(parent);
            }

            // Archive the session.
            let _ = store.archive(name);

            #[allow(clippy::print_stdout)]
            {
                println!("    Cleaned.");
            }
            cleaned += 1;
        }
    }

    // Clean orphan metadata files.
    let orphans = clean_orphan_metadata(&store);

    #[allow(clippy::print_stdout)]
    {
        println!();
        println!("gc: {found} worktree(s) scanned, {cleaned} cleaned.");
        if orphans > 0 {
            println!("gc: {orphans} orphan metadata file(s) removed.");
        }
    }

    Ok(())
}

/// Remove metadata for sessions whose worktree no longer exists.
fn clean_orphan_metadata(store: &SessionStore) -> u32 {
    let mut count = 0;
    let sessions = store.list().unwrap_or_default();
    for name in &sessions {
        if let Ok(state) = store.load(name) {
            if let Some(ref wt) = state.worktree {
                if !std::path::Path::new(&wt.path).exists() {
                    let _ = store.archive(name);
                    count += 1;
                }
            }
        }
    }
    count
}
