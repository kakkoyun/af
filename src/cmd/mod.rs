//! Subcommand implementations.
//!
//! Each subcommand has its own module with a `run()` entry point.
//! The CLI definition in `cli.rs` dispatches to these handlers.

pub mod agent;
pub mod auth;
pub mod config_cmd;
pub mod create;
pub mod diff;
pub mod doctor;
pub mod done;
pub mod editor;
pub mod export;
pub mod gc;
pub mod list;
pub mod note;
pub mod pr;
pub mod resume;
pub mod session_branch;
pub mod stats;
