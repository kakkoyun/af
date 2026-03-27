//! `af` — agentic-flow, automatic-flow, or as-fuck.
//!
//! Core library for the `af` CLI. All meaningful logic lives here so it can be
//! tested independently of the CLI surface.
//!
//! # Architecture
//!
//! ```text
//! ┌─────────────────────────────────────────────────┐
//! │                   af CLI                         │
//! ├────────┬─────────┬──────────┬──────────┬────────┤
//! │ config │ session │   git    │ platform │  util  │
//! ├────────┴─────────┴──────────┴──────────┴────────┤
//! │              Provider Layer                       │
//! │  ┌────────┐  ┌──────────┐  ┌────────┐           │
//! │  │ agent  │  │   mux    │  │ remote │           │
//! │  └────────┘  └──────────┘  └────────┘           │
//! └─────────────────────────────────────────────────┘
//! ```
//!
//! See `docs/PLAN.md` for the full module map and `docs/adr/` for architecture decisions.

pub mod agent;
pub mod cli;
pub mod cmd;
pub mod config;
pub mod git;
pub mod mux;
pub mod platform;
pub mod provider;
pub mod session;
pub mod util;

/// The version of `af` at compile time.
pub const VERSION: &str = env!("CARGO_PKG_VERSION");
