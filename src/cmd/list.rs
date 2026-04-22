//! `af list` — show active workstreams.
//!
//! Lists all sessions from the session store, grouped by repository.
//!
//! The STATUS column (ADR-027) labels each session with `alive`,
//! `suspended`, `orphan`, `unknown`, or `local` so the user can spot
//! dead remote VMs at a glance and run `af done --force` on them. Only
//! sessions whose [`ExecutionMode`] is `Remote` incur a provider probe;
//! everything else renders as `local` without network I/O.

use anyhow::{Context, Result};
use std::collections::BTreeMap;

use crate::provider::exedev::ExedevProvider;
use crate::provider::target::Liveness;
use crate::session::store::SessionStore;
use crate::session::types::{ExecutionMode, SessionState};

/// Short label used in `af list`'s STATUS column for a non-remote session.
pub const STATUS_LOCAL: &str = "local";

/// Resolve the STATUS column text for a session.
///
/// For remote sessions, delegates to `probe(&session_name)` so tests can
/// inject a deterministic outcome. For every other mode the label is a
/// constant `local` — see [`STATUS_LOCAL`] — because the local host
/// either works or the entire CLI would have failed earlier.
///
/// # Example
///
/// ```ignore
/// use af::cmd::list::status_label;
/// use af::provider::exedev::Liveness;
/// let label = status_label(&state, |_| Liveness::Alive);
/// ```
pub fn status_label<F>(state: &SessionState, probe: F) -> String
where
    F: FnOnce(&str) -> Liveness,
{
    if matches!(state.execution.mode, ExecutionMode::Remote) {
        probe(&state.session.name).label().to_owned()
    } else {
        STATUS_LOCAL.to_owned()
    }
}

/// Group sessions by repo (`git_root` basename, or `"workspace"`).
///
/// Pure helper so unit tests can assert on the grouping without touching
/// the filesystem.
pub fn group_by_repo(sessions: Vec<SessionState>) -> BTreeMap<String, Vec<SessionState>> {
    let mut by_repo: BTreeMap<String, Vec<SessionState>> = BTreeMap::new();
    for state in sessions {
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
    by_repo
}

/// Render a single session line. Exposed for tests.
///
/// The layout is `name<padding>branch=<branch><pad>[mode] agents=<a,b> status=<label>`.
/// The STATUS column lives at the end so existing scripts piping on the
/// earlier columns keep working.
pub fn render_session_line(state: &SessionState, status: &str) -> String {
    let branch = state.worktree.as_ref().map_or("-", |wt| &wt.branch);
    let mode = format!("{:?}", state.execution.mode).to_lowercase();
    let agents: Vec<&str> = state.agents.iter().map(|a| a.provider.as_str()).collect();
    format!(
        "  {:<28} branch={:<24} [{}] agents={} status={}",
        state.session.name,
        branch,
        mode,
        agents.join(","),
        status,
    )
}

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

    let mut sessions: Vec<SessionState> = Vec::new();
    for name in &names {
        if let Ok(state) = store.load(name) {
            sessions.push(state);
        }
    }
    let by_repo = group_by_repo(sessions);

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

    // Remote provider is exedev for every remote session `af create`
    // can currently produce (ADR-027 §5 folds workspaces integration
    // into Phase IV). The liveness probe runs per remote session, so
    // `af list` is lazy for local-only users.
    let provider = ExedevProvider;
    let probe = move |name: &str| {
        provider
            .is_alive(name)
            .unwrap_or_else(|err| Liveness::Unknown(err.to_string()))
    };

    #[allow(clippy::print_stdout)]
    for (repo, sessions) in &by_repo {
        let marker = if current_repo.as_deref() == Some(repo.as_str()) {
            " (current)"
        } else {
            ""
        };
        println!("── {repo}{marker} ──────────────────────────────────");
        for state in sessions {
            let status = status_label(state, &probe);
            println!("{}", render_session_line(state, &status));
        }
    }

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::session::types::{
        AgentSlot, AgentStatus, ExecutionInfo, PrInfo, SessionMeta, SessionStatus, VersionInfo,
        WorktreeInfo,
    };
    use chrono::{TimeZone, Utc};

    fn make_session(name: &str, mode: ExecutionMode, branch: &str) -> SessionState {
        SessionState {
            session: SessionMeta {
                name: name.to_owned(),
                id: String::from("00000000-0000-0000-0000-000000000000"),
                created_at: Utc.with_ymd_and_hms(2026, 4, 21, 0, 0, 0).unwrap(),
                status: SessionStatus::Active,
            },
            worktree: Some(WorktreeInfo {
                path: format!("/tmp/worktrees/{name}"),
                branch: branch.to_owned(),
                base_branch: String::from("main"),
                git_root: String::from("/tmp/repo/myrepo"),
            }),
            execution: ExecutionInfo {
                mode,
                multiplexer: String::from("tmux"),
                multiplexer_session: name.to_owned(),
                ssh_host: None,
                remote_path: None,
                remote_provider: None,
            },
            agents: vec![AgentSlot {
                slot: String::from("primary"),
                provider: String::from("claude"),
                session_ids: vec![String::from("00000000-0000-0000-0000-000000000000")],
                pane: String::from("0"),
                status: AgentStatus::Running,
            }],
            pr: PrInfo::default(),
            versions: VersionInfo {
                af: String::from("0.1.0"),
                agent_config_hash: String::from("abc"),
            },
        }
    }

    // ── status_label ────────────────────────────────────────────────

    #[test]
    fn test_status_label_local_mode_returns_local_without_probe() {
        let state = make_session("s", ExecutionMode::Local, "b");
        let label = status_label(&state, |_| panic!("probe must not be called for Local"));
        assert_eq!(label, "local");
    }

    #[test]
    fn test_status_label_sandbox_mode_returns_local_without_probe() {
        let state = make_session("s", ExecutionMode::Sandbox, "b");
        let label = status_label(&state, |_| panic!("probe must not be called for Sandbox"));
        assert_eq!(label, "local");
    }

    #[test]
    fn test_status_label_bare_mode_returns_local_without_probe() {
        let state = make_session("s", ExecutionMode::Bare, "b");
        let label = status_label(&state, |_| panic!("probe must not be called for Bare"));
        assert_eq!(label, "local");
    }

    #[test]
    fn test_status_label_workspace_mode_returns_local_without_probe() {
        let state = make_session("s", ExecutionMode::Workspace, "b");
        let label = status_label(&state, |_| panic!("probe must not be called for Workspace"));
        assert_eq!(label, "local");
    }

    #[test]
    fn test_status_label_remote_mode_invokes_probe_and_renders_alive() {
        let state = make_session("remote-1", ExecutionMode::Remote, "b");
        let label = status_label(&state, |name| {
            assert_eq!(name, "remote-1");
            Liveness::Alive
        });
        assert_eq!(label, "alive");
    }

    #[test]
    fn test_status_label_remote_mode_renders_orphan_for_unreachable() {
        let state = make_session("ghost", ExecutionMode::Remote, "b");
        let label = status_label(&state, |_| Liveness::Unreachable);
        assert_eq!(label, "orphan");
    }

    #[test]
    fn test_status_label_remote_mode_renders_suspended() {
        let state = make_session("paused", ExecutionMode::Remote, "b");
        let label = status_label(&state, |_| Liveness::Suspended);
        assert_eq!(label, "suspended");
    }

    #[test]
    fn test_status_label_remote_mode_renders_unknown() {
        let state = make_session("mystery", ExecutionMode::Remote, "b");
        let label = status_label(&state, |_| Liveness::Unknown(String::from("boom")));
        assert_eq!(label, "unknown");
    }

    // ── group_by_repo ───────────────────────────────────────────────

    #[test]
    fn test_group_by_repo_extracts_basename() {
        let s = make_session("x", ExecutionMode::Local, "b");
        let grouped = group_by_repo(vec![s]);
        assert!(grouped.contains_key("myrepo"));
    }

    #[test]
    fn test_group_by_repo_workspace_fallback() {
        let mut s = make_session("x", ExecutionMode::Workspace, "b");
        s.worktree = None;
        let grouped = group_by_repo(vec![s]);
        assert!(grouped.contains_key("workspace"));
    }

    #[test]
    fn test_group_by_repo_preserves_sessions() {
        let a = make_session("a", ExecutionMode::Local, "b1");
        let b = make_session("b", ExecutionMode::Local, "b2");
        let grouped = group_by_repo(vec![a, b]);
        let entry = grouped.get("myrepo").expect("myrepo group exists");
        assert_eq!(entry.len(), 2);
    }

    // ── render_session_line ─────────────────────────────────────────

    #[test]
    fn test_render_session_line_includes_status_column() {
        let state = make_session("sess-1", ExecutionMode::Remote, "feature/x");
        let line = render_session_line(&state, "alive");
        assert!(line.contains("sess-1"));
        assert!(line.contains("branch=feature/x"));
        assert!(line.contains("[remote]"));
        assert!(line.contains("agents=claude"));
        assert!(line.contains("status=alive"));
    }

    #[test]
    fn test_render_session_line_status_suffix_position() {
        let state = make_session("sess-2", ExecutionMode::Local, "b");
        let line = render_session_line(&state, "local");
        // status is the last field — critical for scripts piping on the
        // earlier columns.
        assert!(line.ends_with("status=local"));
    }

    #[test]
    fn test_render_session_line_without_worktree_shows_dash() {
        let mut state = make_session("sess-3", ExecutionMode::Workspace, "b");
        state.worktree = None;
        let line = render_session_line(&state, "local");
        assert!(line.contains("branch=-"));
    }
}
