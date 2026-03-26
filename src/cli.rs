//! Command-line interface definition.

use clap::{Parser, Subcommand};

/// af — agentic-flow, automatic-flow, or as-fuck.
///
/// Workflow tooling for agentic/automatic programming.
#[derive(Debug, Parser)]
#[command(
    name = "af",
    version,
    about,
    long_about = None,
    propagate_version = true,
)]
pub(crate) struct Cli {
    /// Enable verbose output (-v, -vv, -vvv).
    #[arg(short, long, action = clap::ArgAction::Count, global = true)]
    pub verbose: u8,

    #[command(subcommand)]
    pub command: Commands,
}

/// Top-level subcommands.
#[derive(Debug, Subcommand)]
pub(crate) enum Commands {
    /// Print version and build information.
    Version,
}

impl Cli {
    /// Dispatch to the appropriate subcommand.
    #[allow(clippy::unnecessary_wraps)] // Will do fallible work once subcommands are added.
    pub(crate) fn run(&self) -> anyhow::Result<()> {
        match &self.command {
            Commands::Version => {
                let version = env!("CARGO_PKG_VERSION");
                #[allow(clippy::print_stdout)]
                {
                    println!("af {version}");
                }
                Ok(())
            }
        }
    }
}
