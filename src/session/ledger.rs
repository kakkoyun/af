//! Append-only event ledger for session lifecycle tracking (ADR-011).
//!
//! Each session has a `ledger.jsonl` file — one JSON object per line,
//! chronologically ordered. The ledger is never edited, only appended.
//! It captures what happened during a workstream for crash recovery
//! and pattern analysis.

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use std::io::{BufRead, Write};
use std::path::{Path, PathBuf};

/// A single event in the session ledger.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct LedgerEvent {
    /// When the event occurred.
    pub ts: DateTime<Utc>,
    /// Event type identifier.
    pub event: String,
    /// Additional key-value data for this event.
    #[serde(flatten)]
    pub data: serde_json::Map<String, serde_json::Value>,
}

impl LedgerEvent {
    /// Create a new event with the current timestamp.
    pub fn new(event: &str) -> Self {
        Self {
            ts: Utc::now(),
            event: event.to_owned(),
            data: serde_json::Map::new(),
        }
    }

    /// Add a string field to the event data.
    #[must_use]
    pub fn with_field(mut self, key: &str, value: &str) -> Self {
        self.data
            .insert(key.to_owned(), serde_json::Value::String(value.to_owned()));
        self
    }

    /// Add a numeric field to the event data.
    #[must_use]
    pub fn with_number(mut self, key: &str, value: u64) -> Self {
        self.data.insert(
            key.to_owned(),
            serde_json::Value::Number(serde_json::Number::from(value)),
        );
        self
    }

    /// Add a string list field to the event data.
    #[must_use]
    pub fn with_list(mut self, key: &str, values: &[&str]) -> Self {
        let arr: Vec<serde_json::Value> = values
            .iter()
            .map(|v| serde_json::Value::String((*v).to_owned()))
            .collect();
        self.data
            .insert(key.to_owned(), serde_json::Value::Array(arr));
        self
    }
}

/// Errors from ledger operations.
#[derive(Debug, thiserror::Error)]
pub enum LedgerError {
    /// Failed to write to the ledger file.
    #[error("failed to write ledger at {path}: {source}")]
    Write {
        /// Path that failed.
        path: PathBuf,
        /// Underlying I/O error.
        source: std::io::Error,
    },
    /// Failed to read the ledger file.
    #[error("failed to read ledger at {path}: {source}")]
    Read {
        /// Path that failed.
        path: PathBuf,
        /// Underlying I/O error.
        source: std::io::Error,
    },
    /// Failed to serialize an event to JSON.
    #[error("failed to serialize ledger event: {source}")]
    Serialize {
        /// Underlying serialization error.
        source: serde_json::Error,
    },
    /// Failed to parse a ledger line.
    #[error("failed to parse ledger line {line_number}: {source}")]
    ParseLine {
        /// 1-indexed line number.
        line_number: usize,
        /// Underlying parse error.
        source: serde_json::Error,
    },
}

/// Append-only ledger writer/reader for a single session.
#[derive(Debug, Clone)]
pub struct Ledger {
    /// Path to the `ledger.jsonl` file.
    path: PathBuf,
}

impl Ledger {
    /// Create a ledger for a session stored under the given directory.
    ///
    /// The ledger file will be at `<session_dir>/ledger.jsonl`.
    pub fn new(session_dir: &Path) -> Self {
        Self {
            path: session_dir.join("ledger.jsonl"),
        }
    }

    /// Append an event to the ledger. Creates the file if it doesn't exist.
    pub fn append(&self, event: &LedgerEvent) -> Result<(), LedgerError> {
        if let Some(parent) = self.path.parent() {
            std::fs::create_dir_all(parent).map_err(|e| LedgerError::Write {
                path: self.path.clone(),
                source: e,
            })?;
        }

        let mut line =
            serde_json::to_string(event).map_err(|e| LedgerError::Serialize { source: e })?;
        line.push('\n');

        let mut file = std::fs::OpenOptions::new()
            .create(true)
            .append(true)
            .open(&self.path)
            .map_err(|e| LedgerError::Write {
                path: self.path.clone(),
                source: e,
            })?;

        file.write_all(line.as_bytes())
            .map_err(|e| LedgerError::Write {
                path: self.path.clone(),
                source: e,
            })
    }

    /// Read all events from the ledger, in chronological order.
    ///
    /// Returns an empty vec if the ledger file doesn't exist.
    pub fn read_all(&self) -> Result<Vec<LedgerEvent>, LedgerError> {
        if !self.path.exists() {
            return Ok(Vec::new());
        }

        let file = std::fs::File::open(&self.path).map_err(|e| LedgerError::Read {
            path: self.path.clone(),
            source: e,
        })?;

        let reader = std::io::BufReader::new(file);
        let mut events = Vec::new();

        for (i, line) in reader.lines().enumerate() {
            let line = line.map_err(|e| LedgerError::Read {
                path: self.path.clone(),
                source: e,
            })?;
            if line.trim().is_empty() {
                continue;
            }
            let event: LedgerEvent =
                serde_json::from_str(&line).map_err(|e| LedgerError::ParseLine {
                    line_number: i + 1,
                    source: e,
                })?;
            events.push(event);
        }

        Ok(events)
    }

    /// Check if the ledger file exists.
    pub fn exists(&self) -> bool {
        self.path.exists()
    }

    /// Get the path to the ledger file.
    pub fn path(&self) -> &Path {
        &self.path
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::TempDir;

    #[test]
    fn test_ledger_event_new_has_current_timestamp() {
        let event = LedgerEvent::new("session_created");
        assert_eq!(event.event, "session_created");
        let now = Utc::now();
        let diff = now - event.ts;
        assert!(diff.num_seconds() < 2);
    }

    #[test]
    fn test_ledger_event_with_field() {
        let event = LedgerEvent::new("agent_launched")
            .with_field("agent", "claude")
            .with_field("slot", "primary");
        assert_eq!(event.data["agent"], "claude");
        assert_eq!(event.data["slot"], "primary");
    }

    #[test]
    fn test_ledger_event_with_number() {
        let event = LedgerEvent::new("pr_opened").with_number("number", 42);
        assert_eq!(event.data["number"], 42);
    }

    #[test]
    fn test_ledger_event_with_list() {
        let event =
            LedgerEvent::new("session_completed").with_list("agents_used", &["claude", "pi"]);
        let agents = event.data["agents_used"].as_array().unwrap();
        assert_eq!(agents.len(), 2);
        assert_eq!(agents[0], "claude");
        assert_eq!(agents[1], "pi");
    }

    #[test]
    fn test_append_and_read_roundtrip() {
        let dir = TempDir::new().unwrap();
        let ledger = Ledger::new(dir.path());

        let event1 = LedgerEvent::new("session_created").with_field("agent", "claude");
        let event2 = LedgerEvent::new("agent_launched")
            .with_field("slot", "primary")
            .with_field("agent", "claude");

        ledger.append(&event1).unwrap();
        ledger.append(&event2).unwrap();

        let events = ledger.read_all().unwrap();
        assert_eq!(events.len(), 2);
        assert_eq!(events[0].event, "session_created");
        assert_eq!(events[1].event, "agent_launched");
        assert_eq!(events[1].data["slot"], "primary");
    }

    #[test]
    fn test_read_empty_ledger_returns_empty_vec() {
        let dir = TempDir::new().unwrap();
        let ledger = Ledger::new(dir.path());

        let events = ledger.read_all().unwrap();
        assert!(events.is_empty());
    }

    #[test]
    fn test_ledger_creates_directories() {
        let dir = TempDir::new().unwrap();
        let nested = dir.path().join("deep").join("nested").join("session");
        let ledger = Ledger::new(&nested);

        let event = LedgerEvent::new("test");
        ledger.append(&event).unwrap();
        assert!(ledger.exists());
    }

    #[test]
    fn test_ledger_append_is_truly_append() {
        let dir = TempDir::new().unwrap();
        let ledger = Ledger::new(dir.path());

        ledger.append(&LedgerEvent::new("first")).unwrap();
        ledger.append(&LedgerEvent::new("second")).unwrap();
        ledger.append(&LedgerEvent::new("third")).unwrap();

        let events = ledger.read_all().unwrap();
        assert_eq!(events.len(), 3);
        assert_eq!(events[0].event, "first");
        assert_eq!(events[1].event, "second");
        assert_eq!(events[2].event, "third");
    }

    #[test]
    fn test_ledger_event_serializes_to_single_line() {
        let event = LedgerEvent::new("test")
            .with_field("key", "value")
            .with_number("num", 42);

        let json = serde_json::to_string(&event).unwrap();
        assert!(!json.contains('\n'));
    }

    #[test]
    fn test_ledger_exists_false_when_no_file() {
        let dir = TempDir::new().unwrap();
        let ledger = Ledger::new(dir.path());
        assert!(!ledger.exists());
    }

    #[test]
    fn test_ledger_exists_true_after_append() {
        let dir = TempDir::new().unwrap();
        let ledger = Ledger::new(dir.path());

        ledger.append(&LedgerEvent::new("test")).unwrap();
        assert!(ledger.exists());
    }
}
