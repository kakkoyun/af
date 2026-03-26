//! `af` library — core logic for the af CLI.
//!
//! Keep the binary thin. All meaningful logic lives here so it can be
//! tested independently of the CLI surface.

/// Re-export the version for programmatic access.
pub const VERSION: &str = env!("CARGO_PKG_VERSION");
