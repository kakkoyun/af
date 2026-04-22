//! `af create` — create a new workstream.
//!
//! Creates a git worktree, a multiplexer session, and launches an AI agent.
//! This is the primary entry point for starting isolated work.

use anyhow::{Context, Result, bail};
use chrono::Utc;

use crate::cli::CreateArgs;
use crate::config;
use crate::git::{resolve, worktree};
use crate::mux::Multiplexer;
use crate::mux::tmux::TmuxMultiplexer;
use crate::session::ledger::{Ledger, LedgerEvent};
use crate::session::naming::{apply_prefix, sanitize_session_name};
use crate::session::store::SessionStore;
use crate::session::types::{
    AgentSlot, AgentStatus, ExecutionInfo, ExecutionMode, PrInfo, SessionMeta, SessionState,
    SessionStatus, VersionInfo, WorktreeInfo,
};
use crate::util::uuid::session_id;

/// Execute the `af create` command.
pub fn run(args: &CreateArgs) -> Result<()> {
    let mux = TmuxMultiplexer;
    let cfg = config::load(None).context("failed to load configuration")?;

    // Detect git root — if not in a git repo, use workspace mode.
    let git_root = detect_git_root();

    let Some(git_root) = git_root else {
        return run_workspace_mode(args, &cfg, &mux);
    };

    // Resolve base branch.
    let base_branch = resolve_base_branch(args, &git_root)?;

    let repo_name = repo_name_from_path(&git_root);
    let (task_name, branch_pinned) = resolve_task_name(args, &git_root, &repo_name);

    // Apply branch prefix for fork repos.
    // Skip prefix when: name is pinned to an existing branch, prefix is empty,
    // or prefix_on_fork_only is set and there's no upstream remote.
    let skip_prefix = branch_pinned
        || cfg.branch.prefix.is_empty()
        || (cfg.branch.prefix_on_fork_only && !resolve::has_upstream(&git_root));
    let branch_name = if skip_prefix {
        task_name
    } else {
        apply_prefix(&task_name, &cfg.branch.prefix)
    };

    let session_name = sanitize_session_name(&branch_name);

    // Guards.
    let store = SessionStore::default_location().context("cannot determine data directory")?;
    if store.exists(&session_name) {
        bail!(
            "session '{session_name}' already exists. Use 'af resume {session_name}' to reattach."
        );
    }
    if mux.session_exists(&session_name) {
        bail!(
            "multiplexer session '{session_name}' already exists. Use 'af resume {session_name}'."
        );
    }
    guard_session_limit(&cfg, &store)?;

    // Create worktree.
    let worktree_root = shellexpand_tilde(&cfg.general.worktree_root);
    let worktree_path = std::path::PathBuf::from(&worktree_root)
        .join(&repo_name)
        .join(&branch_name);

    if let Some(parent) = worktree_path.parent() {
        std::fs::create_dir_all(parent)
            .with_context(|| format!("failed to create directory {}", parent.display()))?;
    }

    worktree::create(&git_root, &worktree_path, &branch_name, &base_branch)
        .with_context(|| format!("failed to create worktree at {}", worktree_path.display()))?;

    // Create multiplexer session.
    mux.create_session(&session_name, &worktree_path)
        .context("failed to create multiplexer session")?;
    mux.set_option(&session_name, "@AF_SESSION", "1")
        .context("failed to tag session")?;

    // Determine agent.
    let agent_name = args.agent.as_deref().unwrap_or(&cfg.general.default_agent);
    let sid = session_id(&repo_name, &branch_name);

    // Determine execution mode and launch command.
    //
    // The three concerns compose orthogonally:
    //   --remote [host]        → get a machine (exe.dev, SSH)
    //   --sandbox              → wrap in isolation (slicer, docker/sbx)
    //   --agent <name>         → which AI runs inside
    //
    // Composition: --sandbox --remote host = remote machine + sandbox on it + agent inside.
    let agent_provider = resolve_agent(agent_name)?;
    let approval_mode = if args.yolo {
        crate::agent::ApprovalMode::Yolo
    } else {
        crate::agent::ApprovalMode::Default
    };
    let launch_opts = crate::agent::LaunchOpts {
        session_id: sid.to_string(),
        approval_mode,
    };

    // Resolve the SSH host once so both the launch command and the session
    // state agree on which machine the work lives on. `--remote` with no
    // argument resolves to the literal alias "auto" — the exe.dev
    // convention for "pick any available VM".
    let ssh_host = args
        .remote
        .as_ref()
        .map(|opt| opt.as_deref().unwrap_or("auto").to_owned());
    // Every remote session `af create` can currently produce is an
    // exe.dev VM (ADR-027 §5 folds workspaces support into a later
    // Phase IV step). Recording the provider lets `af list` / `af done`
    // pick the right probe and teardown without a second detection pass.
    let remote_provider = ssh_host.as_ref().map(|_| String::from("exedev"));

    let (exec_mode, launch_cmd_str) = match (args.sandbox, ssh_host.as_deref()) {
        // Remote + sandbox: SSH to host, run sandbox provider there.
        (true, Some(host)) => {
            let sandbox_cmd =
                crate::provider::slicer::agent_sandbox_cmd(agent_name, &worktree_path)
                    .ok_or_else(|| anyhow::anyhow!("failed to build sandbox command"))?;
            let inner_cmd = sandbox_cmd.join(" ");
            let ssh_cmd = format!("ssh {host} {inner_cmd}");
            (ExecutionMode::Sandbox, ssh_cmd)
        }
        // Local sandbox: run sandbox provider on this machine.
        (true, None) => {
            let sandbox_cmd = crate::provider::slicer::agent_sandbox_cmd(
                agent_name,
                &worktree_path,
            )
            .ok_or_else(|| {
                anyhow::anyhow!("sandbox provider required for --sandbox. Install slicer or sbx.")
            })?;
            (ExecutionMode::Sandbox, sandbox_cmd.join(" "))
        }
        // Remote only: launch agent directly on the remote via SSH.
        (false, Some(host)) => {
            let cmd_parts = agent_provider.launch_cmd(&launch_opts);
            let ssh_cmd = format!("ssh {host} {}", cmd_parts.join(" "));
            (ExecutionMode::Remote, ssh_cmd)
        }
        // Local: agent runs on this machine.
        (false, None) => {
            let cmd_parts = agent_provider.launch_cmd(&launch_opts);
            let mode = if args.bare {
                ExecutionMode::Bare
            } else {
                ExecutionMode::Local
            };
            (mode, cmd_parts.join(" "))
        }
    };

    // Send launch command to the multiplexer pane.
    mux.send_keys(&session_name, &format!("{launch_cmd_str}\n"))
        .context("failed to launch agent")?;

    // Store session metadata.
    //
    // `remote_path` is intentionally left `None` for now: the current
    // Remote branch of `af create` SSHes in and launches the agent
    // directly, without a `git clone` step, so we do not yet know which
    // absolute path on the VM holds the working tree. Populating this
    // field is deferred to the provisioning pass that will call
    // `ExedevProvider::bootstrap` from `af create` (ADR-019 §2).
    let state = build_state(
        &session_name,
        &sid.to_string(),
        Some(&worktree_path),
        Some(&branch_name),
        Some(&base_branch),
        Some(&git_root),
        exec_mode,
        agent_name,
        ssh_host.as_deref(),
        None,
        remote_provider.as_deref(),
    );
    store.save(&state).context("failed to save session state")?;

    // Write ledger events.
    let session_dir = store.session_dir_path(&session_name);
    let ledger = Ledger::new(&session_dir);
    ledger
        .append(
            &LedgerEvent::new("session_created")
                .with_field("af_version", crate::VERSION)
                .with_field("agent", agent_name)
                .with_field("mode", execution_mode_str(args))
                .with_field("branch", &branch_name)
                .with_field("base", &base_branch),
        )
        .context("failed to write ledger")?;
    ledger
        .append(
            &LedgerEvent::new("agent_launched")
                .with_field("slot", "primary")
                .with_field("agent", agent_name)
                .with_field("session_id", &sid.to_string())
                .with_field("cmd", &launch_cmd_str),
        )
        .context("failed to write ledger")?;

    // Inject metadata into multiplexer env for debugging.
    let _ = mux.set_env(
        &session_name,
        "AF_WORKTREE_PATH",
        &worktree_path.display().to_string(),
    );
    let _ = mux.set_env(&session_name, "AF_BRANCH_NAME", &branch_name);
    let _ = mux.set_env(
        &session_name,
        "AF_GIT_ROOT",
        &git_root.display().to_string(),
    );
    let _ = mux.set_env(&session_name, "AF_SESSION_ID", &sid.to_string());

    // Create Obsidian note (best-effort, only if enabled).
    if cfg.obsidian.enabled {
        if let Ok(note_path) = crate::obsidian::note_path(&cfg.obsidian, &session_name) {
            let meta = crate::obsidian::NoteMeta {
                session: session_name.clone(),
                branch: branch_name.clone(),
                base_branch,
                repo: repo_name,
                agent: agent_name.to_owned(),
                status: "active".to_owned(),
                created_at: chrono::Utc::now(),
                completed_at: None,
            };
            let _ = crate::obsidian::create_note(&note_path, &meta);
        }
    }

    // Notify superterm (best-effort).
    crate::util::notify::notify(
        &session_name,
        "Workstream created",
        &format!("Agent: {agent_name}, Branch: {branch_name}"),
    );

    // Attach.
    mux.attach_or_switch(&session_name)
        .context("failed to attach to session")?;

    Ok(())
}

/// Workspace mode: non-git directory, no worktree.
fn run_workspace_mode(
    args: &CreateArgs,
    cfg: &config::Config,
    mux: &TmuxMultiplexer,
) -> Result<()> {
    if args.current || args.from.is_some() || args.from_pr.is_some() {
        bail!(
            "--current, --from, and --from-pr are not available in workspace mode (no git repository)"
        );
    }

    let cwd = std::env::current_dir().context("cannot determine current directory")?;
    let dir_name = repo_name_from_path(&cwd);

    let name = args
        .name
        .clone()
        .unwrap_or_else(|| format!("{dir_name}-{}", Utc::now().format("%Y%m%d-%H%M%S")));
    let session_name = sanitize_session_name(&name);

    let store = SessionStore::default_location().context("cannot determine data directory")?;
    if store.exists(&session_name) {
        bail!("session '{session_name}' already exists.");
    }
    guard_session_limit(cfg, &store)?;

    mux.create_session(&session_name, &cwd)
        .context("failed to create multiplexer session")?;
    mux.set_option(&session_name, "@AF_SESSION", "1")?;

    let agent_name = args.agent.as_deref().unwrap_or(&cfg.general.default_agent);
    let sid = session_id(&dir_name, &name);
    let agent_provider = resolve_agent(agent_name)?;
    let launch_opts = crate::agent::LaunchOpts {
        session_id: sid.to_string(),
        approval_mode: crate::agent::ApprovalMode::Default,
    };
    let cmd_parts = agent_provider.launch_cmd(&launch_opts);
    mux.send_keys(&session_name, &format!("{}\n", cmd_parts.join(" ")))?;

    let state = build_state(
        &session_name,
        &sid.to_string(),
        None,
        None,
        None,
        None,
        ExecutionMode::Workspace,
        agent_name,
        None,
        None,
        None,
    );
    store.save(&state)?;

    let session_dir = store.session_dir_path(&session_name);
    let ledger = Ledger::new(&session_dir);
    ledger.append(
        &LedgerEvent::new("session_created")
            .with_field("af_version", crate::VERSION)
            .with_field("agent", agent_name)
            .with_field("mode", "workspace"),
    )?;

    mux.attach_or_switch(&session_name)?;
    Ok(())
}

// ── Helpers ─────────────────────────────────────────────────────────────────

/// Detect the git root of the current directory, or `None` if not in a repo.
fn detect_git_root() -> Option<std::path::PathBuf> {
    std::process::Command::new("git")
        .args(["rev-parse", "--show-toplevel"])
        .output()
        .ok()
        .filter(|o| o.status.success())
        .and_then(|o| {
            let s = String::from_utf8_lossy(&o.stdout).trim().to_owned();
            if s.is_empty() {
                None
            } else {
                Some(std::path::PathBuf::from(s))
            }
        })
}

/// Extract the repo/directory name from a path.
fn repo_name_from_path(path: &std::path::Path) -> String {
    path.file_name()
        .map_or_else(|| "repo".to_owned(), |n| n.to_string_lossy().into_owned())
}

/// Resolve the base branch from CLI args and git state.
fn resolve_base_branch(args: &CreateArgs, git_root: &std::path::Path) -> Result<String> {
    if args.current {
        let output = std::process::Command::new("git")
            .args(["branch", "--show-current"])
            .current_dir(git_root)
            .output()
            .context("failed to get current branch")?;
        let branch = String::from_utf8_lossy(&output.stdout).trim().to_owned();
        if branch.is_empty() {
            bail!("--current requires a named branch (not detached HEAD)");
        }
        Ok(branch)
    } else if let Some(ref from) = args.from {
        Ok(from.clone())
    } else if let Some(pr_number) = args.from_pr {
        let pr_info = crate::git::pr::resolve_pr_branches(pr_number, git_root)
            .with_context(|| format!("failed to resolve PR #{pr_number}"))?;
        // Fetch the PR's head branch so it's available locally.
        let _ = std::process::Command::new("git")
            .args(["fetch", "origin", &pr_info.head_branch])
            .current_dir(git_root)
            .status();
        Ok(pr_info.base_branch)
    } else {
        resolve::fetch_and_resolve_base(git_root).context("failed to resolve base branch")
    }
}

/// Resolve the task name from args, applying auto-generation and branch pinning.
///
/// Returns `(name, branch_pinned)`. When `branch_pinned` is true, the name
/// matches an existing local branch and should not be prefixed.
fn resolve_task_name(
    args: &CreateArgs,
    git_root: &std::path::Path,
    repo_name: &str,
) -> (String, bool) {
    if let Some(ref name) = args.name {
        return (name.clone(), false);
    }

    // --from-pr: default name to the PR's head branch.
    if let Some(pr_number) = args.from_pr {
        if let Ok(pr_info) = crate::git::pr::resolve_pr_branches(pr_number, git_root) {
            return (pr_info.head_branch, true);
        }
    }

    // --from with an existing local branch: default name to that branch.
    if let Some(ref from) = args.from {
        let branch_exists = std::process::Command::new("git")
            .args(["show-ref", "--verify", "--quiet"])
            .arg(format!("refs/heads/{from}"))
            .current_dir(git_root)
            .status()
            .is_ok_and(|s| s.success());
        if branch_exists {
            return (from.clone(), true);
        }
    }

    // Auto-generate from repo name + timestamp.
    (
        format!("{repo_name}-{}", Utc::now().format("%Y%m%d-%H%M%S")),
        false,
    )
}

/// Check the session count against the configured limit.
fn guard_session_limit(cfg: &config::Config, store: &SessionStore) -> Result<()> {
    let sessions = store.list().unwrap_or_default();
    let count = sessions.len();
    if count >= cfg.general.max_sessions as usize {
        bail!(
            "session limit reached ({count}/{max}). Run 'af gc' or 'af done' to free slots.",
            max = cfg.general.max_sessions
        );
    }
    Ok(())
}

/// Map CLI args to a mode string for ledger events.
fn execution_mode_str(args: &CreateArgs) -> &'static str {
    if args.sandbox {
        "sandbox"
    } else if args.remote.is_some() {
        "remote"
    } else if args.bare {
        "bare"
    } else {
        "local"
    }
}

/// Resolve an agent provider by name.
fn resolve_agent(name: &str) -> Result<Box<dyn crate::agent::AgentProvider>> {
    crate::agent::resolve(name).ok_or_else(|| {
        anyhow::anyhow!(
            "unknown agent '{name}'. Supported: {}",
            crate::agent::KNOWN_AGENTS.join(", ")
        )
    })
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

/// Build a `SessionState` from components.
///
/// The `ssh_host`, `remote_path`, and `remote_provider` arguments map
/// directly onto the identically-named fields of
/// [`ExecutionInfo`](crate::session::types::ExecutionInfo). Pass `None`
/// for every local session so the on-disk `state.toml` stays free of
/// empty remote fields.
#[allow(clippy::too_many_arguments)]
fn build_state(
    session_name: &str,
    sid: &str,
    worktree_path: Option<&std::path::Path>,
    branch: Option<&str>,
    base_branch: Option<&str>,
    git_root: Option<&std::path::Path>,
    mode: ExecutionMode,
    agent_name: &str,
    ssh_host: Option<&str>,
    remote_path: Option<&str>,
    remote_provider: Option<&str>,
) -> SessionState {
    let worktree = match (worktree_path, branch, base_branch, git_root) {
        (Some(wt), Some(b), Some(bb), Some(gr)) => Some(WorktreeInfo {
            path: wt.display().to_string(),
            branch: b.to_owned(),
            base_branch: bb.to_owned(),
            git_root: gr.display().to_string(),
        }),
        _ => None,
    };

    SessionState {
        session: SessionMeta {
            name: session_name.to_owned(),
            id: sid.to_owned(),
            created_at: Utc::now(),
            status: SessionStatus::Active,
        },
        worktree,
        execution: ExecutionInfo {
            mode,
            multiplexer: "tmux".to_owned(),
            multiplexer_session: session_name.to_owned(),
            ssh_host: ssh_host.map(str::to_owned),
            remote_path: remote_path.map(str::to_owned),
            remote_provider: remote_provider.map(str::to_owned),
        },
        agents: vec![AgentSlot {
            slot: "primary".to_owned(),
            provider: agent_name.to_owned(),
            session_ids: vec![sid.to_owned()],
            pane: "0".to_owned(),
            status: AgentStatus::Running,
        }],
        pr: PrInfo::default(),
        versions: VersionInfo {
            af: crate::VERSION.to_owned(),
            agent_config_hash: String::new(),
        },
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_build_state_local_leaves_remote_fields_none() {
        // Local sessions must never populate ssh_host / remote_path /
        // remote_provider — the serialized state.toml has to stay
        // indistinguishable from pre-Phase-IV files.
        let state = build_state(
            "sess-local",
            "00000000-0000-0000-0000-000000000000",
            Some(std::path::Path::new("/tmp/wt")),
            Some("feature/x"),
            Some("main"),
            Some(std::path::Path::new("/tmp/repo")),
            ExecutionMode::Local,
            "claude",
            None,
            None,
            None,
        );
        assert_eq!(state.execution.ssh_host, None);
        assert_eq!(state.execution.remote_path, None);
        assert_eq!(state.execution.remote_provider, None);
    }

    #[test]
    fn test_build_state_remote_populates_host_and_provider() {
        // Remote sessions carry the SSH host and provider so the editor,
        // pr, and done commands can target the VM without re-deriving
        // them. `remote_path` stays None until the provisioning pass
        // lands — see the note at the Remote branch in `run()`.
        let state = build_state(
            "sess-remote",
            "00000000-0000-0000-0000-000000000000",
            Some(std::path::Path::new("/tmp/wt")),
            Some("feature/x"),
            Some("main"),
            Some(std::path::Path::new("/tmp/repo")),
            ExecutionMode::Remote,
            "claude",
            Some("dev-vm-42"),
            None,
            Some("exedev"),
        );
        assert_eq!(state.execution.ssh_host.as_deref(), Some("dev-vm-42"));
        assert_eq!(state.execution.remote_path, None);
        assert_eq!(state.execution.remote_provider.as_deref(), Some("exedev"));
    }

    #[test]
    fn test_build_state_sandbox_on_remote_records_host() {
        // --sandbox --remote composes: the mode is Sandbox but the work
        // still lives on a remote VM, so ssh_host must be recorded for
        // teardown. (The mode drives liveness probing; ssh_host drives
        // editor/pr/done dispatch.)
        let state = build_state(
            "sess-sbx-remote",
            "00000000-0000-0000-0000-000000000000",
            Some(std::path::Path::new("/tmp/wt")),
            Some("feature/x"),
            Some("main"),
            Some(std::path::Path::new("/tmp/repo")),
            ExecutionMode::Sandbox,
            "codex",
            Some("auto"),
            None,
            Some("exedev"),
        );
        assert_eq!(state.execution.mode, ExecutionMode::Sandbox);
        assert_eq!(state.execution.ssh_host.as_deref(), Some("auto"));
        assert_eq!(state.execution.remote_provider.as_deref(), Some("exedev"));
    }
}
