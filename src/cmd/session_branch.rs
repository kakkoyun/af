//! `af session-branch` — launch agent with a session ID tied to the current branch.
//!
//! A lightweight alternative to `af create` — no worktree, no multiplexer session management.
//! Just launches the agent with a deterministic session ID derived from the branch name.

use anyhow::{Context, Result, bail};

use crate::config;
use crate::util::uuid::session_id;

/// Execute the `af session-branch` command.
pub fn run() -> Result<()> {
    let cfg = config::load(None).context("failed to load config")?;

    // Get current branch.
    let output = std::process::Command::new("git")
        .args(["branch", "--show-current"])
        .output()
        .context("failed to run git")?;

    if !output.status.success() {
        bail!("not a git repository");
    }

    let branch = String::from_utf8_lossy(&output.stdout).trim().to_owned();
    if branch.is_empty() {
        bail!("detached HEAD — switch to a named branch first");
    }

    let sid = session_id(&branch, &branch);

    // Resolve agent.
    let agent_name = &cfg.general.default_agent;
    let agent = crate::agent::resolve(agent_name)
        .ok_or_else(|| anyhow::anyhow!("unknown agent '{agent_name}'"))?;

    let opts = crate::agent::LaunchOpts {
        session_id: sid.to_string(),
        yolo: false,
    };
    let cmd = agent.launch_cmd(&opts);

    if cmd.is_empty() {
        bail!("agent produced empty launch command");
    }

    #[allow(clippy::print_stderr)]
    {
        eprintln!("af: launching {} with session {sid}", agent.name());
    }

    // Spawn the agent process and wait for it.
    let status = std::process::Command::new(&cmd[0])
        .args(&cmd[1..])
        .status()
        .with_context(|| format!("failed to run {}", cmd[0]))?;

    if !status.success() {
        let code = status.code().unwrap_or(1);
        bail!("agent exited with code {code}");
    }

    Ok(())
}
