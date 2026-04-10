//! `af stats` — workstream analytics from ledger data.
//!
//! Reads ledger files across all active and archived sessions, aggregates
//! events, and prints key metrics: session count, agent usage, event
//! distribution, and session durations.

use std::collections::HashMap;

use anyhow::{Context, Result};

use crate::session::ledger::{Ledger, LedgerEvent};
use crate::session::store::SessionStore;

/// Execute the `af stats` command.
pub fn run() -> Result<()> {
    let store = SessionStore::default_location().context("cannot determine data directory")?;
    let sessions = store.list().unwrap_or_default();

    if sessions.is_empty() {
        #[allow(clippy::print_stderr)]
        {
            eprintln!("No sessions found. Use 'af create' to start one.");
        }
        return Ok(());
    }

    let mut all_events: Vec<LedgerEvent> = Vec::new();
    for name in &sessions {
        let session_dir = store.session_dir_path(name);
        let ledger = Ledger::new(&session_dir);
        if let Ok(events) = ledger.read_all() {
            all_events.extend(events);
        }
    }

    let summary = compute_stats(&all_events, sessions.len());

    #[allow(clippy::print_stdout)]
    {
        println!("Workstream Statistics");
        println!("=====================");
        println!();
        println!("Sessions:        {}", summary.session_count);
        println!("Total events:    {}", summary.total_events);
        println!();

        if !summary.agent_usage.is_empty() {
            println!("Agent usage:");
            let mut agents: Vec<_> = summary.agent_usage.iter().collect();
            agents.sort_by(|a, b| b.1.cmp(a.1));
            for (agent, count) in agents {
                println!("  {agent:<14} {count} launches");
            }
            println!();
        }

        if !summary.event_counts.is_empty() {
            println!("Event types:");
            let mut events: Vec<_> = summary.event_counts.iter().collect();
            events.sort_by(|a, b| b.1.cmp(a.1));
            for (event, count) in events {
                println!("  {event:<24} {count}");
            }
        }
    }

    Ok(())
}

/// Aggregated statistics from ledger events.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct StatsSummary {
    /// Number of active sessions.
    pub session_count: usize,
    /// Total number of ledger events.
    pub total_events: usize,
    /// Agent launch counts by provider name.
    pub agent_usage: HashMap<String, usize>,
    /// Event type counts.
    pub event_counts: HashMap<String, usize>,
}

/// Compute aggregate statistics from a collection of ledger events.
pub fn compute_stats(events: &[LedgerEvent], session_count: usize) -> StatsSummary {
    let mut agent_usage: HashMap<String, usize> = HashMap::new();
    let mut event_counts: HashMap<String, usize> = HashMap::new();

    for event in events {
        *event_counts.entry(event.event.clone()).or_insert(0) += 1;

        if event.event == "agent_launched" {
            if let Some(serde_json::Value::String(agent)) = event.data.get("agent") {
                *agent_usage.entry(agent.clone()).or_insert(0) += 1;
            }
        }
    }

    StatsSummary {
        session_count,
        total_events: events.len(),
        agent_usage,
        event_counts,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use chrono::Utc;

    fn event(name: &str) -> LedgerEvent {
        LedgerEvent {
            ts: Utc::now(),
            event: name.to_owned(),
            data: serde_json::Map::new(),
        }
    }

    fn agent_event(agent: &str) -> LedgerEvent {
        let mut e = event("agent_launched");
        e.data.insert(
            "agent".to_owned(),
            serde_json::Value::String(agent.to_owned()),
        );
        e
    }

    #[test]
    fn test_compute_stats_empty() {
        let stats = compute_stats(&[], 0);
        assert_eq!(stats.session_count, 0);
        assert_eq!(stats.total_events, 0);
        assert!(stats.agent_usage.is_empty());
        assert!(stats.event_counts.is_empty());
    }

    #[test]
    fn test_compute_stats_counts_events() {
        let events = vec![
            event("session_created"),
            event("session_created"),
            event("session_completed"),
        ];
        let stats = compute_stats(&events, 2);
        assert_eq!(stats.session_count, 2);
        assert_eq!(stats.total_events, 3);
        assert_eq!(stats.event_counts["session_created"], 2);
        assert_eq!(stats.event_counts["session_completed"], 1);
    }

    #[test]
    fn test_compute_stats_tracks_agent_usage() {
        let events = vec![
            agent_event("claude"),
            agent_event("claude"),
            agent_event("pi"),
            event("session_created"),
        ];
        let stats = compute_stats(&events, 1);
        assert_eq!(stats.agent_usage["claude"], 2);
        assert_eq!(stats.agent_usage["pi"], 1);
        assert_eq!(stats.total_events, 4);
    }

    #[test]
    fn test_compute_stats_ignores_non_launch_events_for_agent_usage() {
        let events = vec![event("agent_stopped"), event("session_created")];
        let stats = compute_stats(&events, 1);
        assert!(stats.agent_usage.is_empty());
        assert_eq!(stats.event_counts.len(), 2);
    }

    #[test]
    fn test_stats_summary_debug_clone() {
        let stats = compute_stats(&[], 0);
        let cloned = stats.clone();
        assert_eq!(stats, cloned);
        let _debug = format!("{stats:?}");
    }
}
