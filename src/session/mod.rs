//! Session types, metadata persistence, naming, and event ledger.
//!
//! A session (workstream) ties together a git worktree, a multiplexer session,
//! and one or more AI agents. See ADR-006 and ADR-011.

pub mod ledger;
pub mod naming;
pub mod store;
pub mod types;
