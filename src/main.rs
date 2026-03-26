//! `af` — agentic-flow, automatic-flow, or as-fuck.
//!
//! Workflow tooling for agentic/automatic programming.

use clap::Parser;
use tracing_subscriber::EnvFilter;

mod cli;

fn main() -> anyhow::Result<()> {
    // Initialize tracing (respects RUST_LOG env var).
    tracing_subscriber::fmt()
        .with_env_filter(
            EnvFilter::try_from_default_env().unwrap_or_else(|_| EnvFilter::new("warn")),
        )
        .with_writer(std::io::stderr)
        .init();

    let cli = cli::Cli::parse();
    cli.run()
}
