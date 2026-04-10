//! `af agent` — manage agents within a workstream.
//!
//! Supports adding, stopping, and listing agents in the current
//! workstream's multiplexer session. Each agent runs in its own pane.

use anyhow::{Context, Result, bail};

use crate::cli::{AgentAction, AgentAddArgs, AgentArgs, AgentStopArgs};
use crate::mux::Multiplexer;
use crate::mux::tmux::TmuxMultiplexer;
use crate::session::ledger::{Ledger, LedgerEvent};
use crate::session::store::SessionStore;
use crate::session::types::{AgentSlot, AgentStatus};

/// Execute the `af agent` command.
pub fn run(args: &AgentArgs) -> Result<()> {
    match &args.action {
        AgentAction::Add(add_args) => run_add(add_args),
        AgentAction::Stop(stop_args) => run_stop(stop_args),
        AgentAction::List => run_list(),
    }
}

/// Add an agent to the current workstream in a new pane.
fn run_add(args: &AgentAddArgs) -> Result<()> {
    let mux = TmuxMultiplexer;
    let cfg = crate::config::load(None).context("failed to load configuration")?;

    let session_name = resolve_session_name(&mux, args.session.as_deref())?;
    let store = SessionStore::default_location().context("cannot determine data directory")?;
    let mut state = store
        .load(&session_name)
        .context("session not found — use 'af list' to see active sessions")?;

    // Determine agent provider.
    let agent_name = args.agent.as_deref().unwrap_or(&cfg.general.default_agent);
    let agent_provider = crate::agent::resolve(agent_name).ok_or_else(|| {
        anyhow::anyhow!(
            "unknown agent '{agent_name}'. Supported: {}",
            crate::agent::KNOWN_AGENTS.join(", ")
        )
    })?;

    // Determine slot name.
    let slot_name = args
        .slot
        .clone()
        .unwrap_or_else(|| auto_slot_name(agent_name, &state.agents));

    // Guard: slot name must be unique.
    if state.agents.iter().any(|a| a.slot == slot_name) {
        bail!(
            "slot '{slot_name}' already exists. Use a different name or stop the existing agent first."
        );
    }

    // Determine working directory for the new pane.
    let cwd = match state.worktree.as_ref() {
        Some(wt) => std::path::PathBuf::from(&wt.path),
        None => std::env::current_dir().unwrap_or_default(),
    };

    // Create a new pane.
    let pane_id = mux
        .create_pane(&session_name, &cwd)
        .context("failed to create new pane")?;

    // Build and send launch command.
    let session_id =
        crate::util::uuid::session_id(&session_name, &format!("{session_name}-{slot_name}"));
    let launch_opts = crate::agent::LaunchOpts {
        session_id: session_id.to_string(),
        approval_mode: crate::agent::ApprovalMode::Default,
    };
    let cmd_parts = agent_provider.launch_cmd(&launch_opts);
    let launch_cmd_str = cmd_parts.join(" ");

    mux.send_keys_to_pane(&session_name, &pane_id, &launch_cmd_str)
        .context("failed to send launch command to pane")?;

    // Update session state.
    state.agents.push(AgentSlot {
        slot: slot_name.clone(),
        provider: agent_name.to_owned(),
        session_ids: vec![session_id.to_string()],
        pane: pane_id.clone(),
        status: AgentStatus::Running,
    });
    store.save(&state).context("failed to save session state")?;

    // Write ledger events.
    let session_dir = store.session_dir_path(&session_name);
    let ledger = Ledger::new(&session_dir);
    let _ = ledger.append(
        &LedgerEvent::new("agent_added")
            .with_field("slot", &slot_name)
            .with_field("agent", agent_name)
            .with_field("pane", &pane_id)
            .with_field("cmd", &launch_cmd_str),
    );
    let _ = ledger.append(
        &LedgerEvent::new("agent_launched")
            .with_field("slot", &slot_name)
            .with_field("agent", agent_name)
            .with_field("session_id", &session_id.to_string())
            .with_field("pane", &pane_id)
            .with_field("cmd", &launch_cmd_str),
    );

    #[allow(clippy::print_stderr)]
    {
        eprintln!("Added {agent_name} agent in slot '{slot_name}' (pane {pane_id}).");
    }

    Ok(())
}

/// Stop an agent running in a slot.
fn run_stop(args: &AgentStopArgs) -> Result<()> {
    let mux = TmuxMultiplexer;

    let session_name = resolve_session_name(&mux, args.session.as_deref())?;
    let store = SessionStore::default_location().context("cannot determine data directory")?;
    let mut state = store
        .load(&session_name)
        .context("session not found — use 'af list' to see active sessions")?;

    // Find the agent slot.
    let slot_idx = state
        .agents
        .iter()
        .position(|a| a.slot == args.slot)
        .ok_or_else(|| {
            let available: Vec<&str> = state.agents.iter().map(|a| a.slot.as_str()).collect();
            anyhow::anyhow!(
                "slot '{}' not found. Available slots: {}",
                args.slot,
                available.join(", ")
            )
        })?;

    let slot = &state.agents[slot_idx];
    let pane_id = slot.pane.clone();
    let provider = slot.provider.clone();

    // Guard: don't stop an already-stopped agent.
    if slot.status == AgentStatus::Stopped {
        bail!("agent in slot '{}' is already stopped.", args.slot);
    }

    // Kill the pane.
    let _ = mux.kill_pane(&session_name, &pane_id);

    // Update status.
    state.agents[slot_idx].status = AgentStatus::Stopped;
    store.save(&state).context("failed to save session state")?;

    // Write ledger event.
    let session_dir = store.session_dir_path(&session_name);
    let ledger = Ledger::new(&session_dir);
    let _ = ledger.append(
        &LedgerEvent::new("agent_stopped")
            .with_field("slot", &args.slot)
            .with_field("agent", &provider)
            .with_field("reason", "user_request"),
    );

    #[allow(clippy::print_stderr)]
    {
        eprintln!("Stopped {provider} agent in slot '{}'.", args.slot);
    }

    Ok(())
}

/// List agents in the current workstream.
fn run_list() -> Result<()> {
    let mux = TmuxMultiplexer;

    let session_name = resolve_session_name(&mux, None)?;
    let store = SessionStore::default_location().context("cannot determine data directory")?;
    let state = store
        .load(&session_name)
        .context("session not found — use 'af list' to see active sessions")?;

    if state.agents.is_empty() {
        #[allow(clippy::print_stderr)]
        {
            eprintln!("No agents in session '{session_name}'.");
        }
        return Ok(());
    }

    #[allow(clippy::print_stdout)]
    {
        println!(
            "{:<12} {:<10} {:<8} {:<8}",
            "SLOT", "AGENT", "STATUS", "PANE"
        );
        for agent in &state.agents {
            let status = match agent.status {
                AgentStatus::Running => "running",
                AgentStatus::Stopped => "stopped",
                AgentStatus::Crashed => "crashed",
            };
            println!(
                "{:<12} {:<10} {:<8} {:<8}",
                agent.slot, agent.provider, status, agent.pane
            );
        }
    }

    Ok(())
}

// ── Helpers ─────────────────────────────────────────────────────────────────

/// Resolve the session name from an explicit argument or the current mux session.
fn resolve_session_name(mux: &TmuxMultiplexer, explicit: Option<&str>) -> Result<String> {
    if let Some(name) = explicit {
        return Ok(name.to_owned());
    }
    if mux.is_inside_session() {
        if let Some(name) = mux.current_session_name()? {
            return Ok(name);
        }
    }
    bail!("specify a session with --session, or run inside a multiplexer session");
}

/// Auto-generate a slot name from the agent provider name.
///
/// If "pi" is taken, tries "pi-2", "pi-3", etc.
pub fn auto_slot_name(agent_name: &str, existing: &[AgentSlot]) -> String {
    if !existing.iter().any(|a| a.slot == agent_name) {
        return agent_name.to_owned();
    }
    let mut n = 2;
    loop {
        let candidate = format!("{agent_name}-{n}");
        if !existing.iter().any(|a| a.slot == candidate) {
            return candidate;
        }
        n += 1;
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::session::types::{AgentSlot, AgentStatus};

    fn slot(name: &str, provider: &str) -> AgentSlot {
        AgentSlot {
            slot: name.to_owned(),
            provider: provider.to_owned(),
            session_ids: vec![],
            pane: String::from("0"),
            status: AgentStatus::Running,
        }
    }

    #[test]
    fn test_auto_slot_name_uses_agent_name_when_available() {
        let existing = vec![slot("primary", "claude")];
        assert_eq!(auto_slot_name("pi", &existing), "pi");
    }

    #[test]
    fn test_auto_slot_name_appends_suffix_on_collision() {
        let existing = vec![slot("primary", "claude"), slot("pi", "pi")];
        assert_eq!(auto_slot_name("pi", &existing), "pi-2");
    }

    #[test]
    fn test_auto_slot_name_increments_suffix() {
        let existing = vec![
            slot("primary", "claude"),
            slot("pi", "pi"),
            slot("pi-2", "pi"),
        ];
        assert_eq!(auto_slot_name("pi", &existing), "pi-3");
    }

    #[test]
    fn test_auto_slot_name_empty_existing() {
        let existing: Vec<AgentSlot> = vec![];
        assert_eq!(auto_slot_name("claude", &existing), "claude");
    }
}
