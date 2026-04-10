//! `af export` — export ledger data for external analysis.
//!
//! Reads ledger events from sessions and outputs them as JSON or CSV to stdout.

use anyhow::{Context, Result, bail};

use crate::cli::ExportArgs;
use crate::session::ledger::{Ledger, LedgerEvent};
use crate::session::store::SessionStore;

/// Execute the `af export` command.
pub fn run(args: &ExportArgs) -> Result<()> {
    // Validate format before doing any I/O.
    if !matches!(args.format.as_str(), "json" | "csv") {
        bail!(
            "unsupported format: {:?} (supported: json, csv)",
            args.format
        );
    }

    let store = SessionStore::default_location().context("cannot determine data directory")?;

    let events = collect_events(&store, args.session.as_deref())?;

    match args.format.as_str() {
        "json" => export_json(&events)?,
        "csv" => export_csv(&events)?,
        _ => unreachable!("format validated above"),
    }

    Ok(())
}

/// Collect ledger events for the requested session(s).
fn collect_events(store: &SessionStore, session: Option<&str>) -> Result<Vec<LedgerEvent>> {
    let session_names = if let Some(name) = session {
        let all = store.list().unwrap_or_default();
        if !all.iter().any(|s| s == name) {
            bail!("session not found: {name}");
        }
        vec![name.to_owned()]
    } else {
        store.list().unwrap_or_default()
    };

    if session_names.is_empty() {
        #[allow(clippy::print_stderr)]
        {
            eprintln!("No sessions found. Use 'af create' to start one.");
        }
        return Ok(Vec::new());
    }

    let mut all_events = Vec::new();
    for name in &session_names {
        let session_dir = store.session_dir_path(name);
        let ledger = Ledger::new(&session_dir);
        if let Ok(events) = ledger.read_all() {
            all_events.extend(events);
        }
    }

    Ok(all_events)
}

/// Write events as a JSON array to stdout.
fn export_json(events: &[LedgerEvent]) -> Result<()> {
    let json = serde_json::to_string_pretty(events).context("failed to serialize events")?;
    #[allow(clippy::print_stdout)]
    {
        println!("{json}");
    }
    Ok(())
}

/// Write events as CSV (`ts,event,data` columns) to stdout.
fn export_csv(events: &[LedgerEvent]) -> Result<()> {
    #[allow(clippy::print_stdout)]
    {
        println!("ts,event,data");
        for event in events {
            let data =
                serde_json::to_string(&event.data).context("failed to serialize event data")?;
            // Escape the data field for CSV: wrap in quotes and double any internal quotes.
            let escaped = data.replace('"', "\"\"");
            println!("{},{},\"{}\"", event.ts.to_rfc3339(), event.event, escaped);
        }
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use chrono::Utc;

    fn sample_event(name: &str) -> LedgerEvent {
        LedgerEvent {
            ts: Utc::now(),
            event: name.to_owned(),
            data: serde_json::Map::new(),
        }
    }

    #[test]
    fn test_export_json_empty() {
        let result = export_json(&[]);
        assert!(result.is_ok());
    }

    #[test]
    fn test_export_json_with_events() {
        let events = vec![
            sample_event("session_created"),
            sample_event("agent_launched"),
        ];
        let result = export_json(&events);
        assert!(result.is_ok());
    }

    #[test]
    fn test_export_csv_empty() {
        let result = export_csv(&[]);
        assert!(result.is_ok());
    }

    #[test]
    fn test_export_csv_with_events() {
        let events = vec![
            sample_event("session_created"),
            sample_event("agent_launched"),
        ];
        let result = export_csv(&events);
        assert!(result.is_ok());
    }

    #[test]
    fn test_collect_events_no_sessions() {
        let dir = tempfile::TempDir::new().unwrap();
        let store = SessionStore::new(dir.path());
        let events = collect_events(&store, None).unwrap();
        assert!(events.is_empty());
    }

    #[test]
    fn test_collect_events_missing_session_returns_error() {
        let dir = tempfile::TempDir::new().unwrap();
        let store = SessionStore::new(dir.path());
        let result = collect_events(&store, Some("nonexistent"));
        assert!(result.is_err());
    }

    #[test]
    fn test_unsupported_format_returns_error() {
        let args = ExportArgs {
            format: "xml".to_owned(),
            session: Some("nonexistent".to_owned()),
        };
        let err = run(&args).unwrap_err();
        let msg = err.to_string();
        assert!(
            msg.contains("xml") || msg.contains("unsupported"),
            "error should mention the unsupported format: {msg}"
        );
    }
}
