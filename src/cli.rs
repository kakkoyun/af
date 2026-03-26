//! Command-line interface definition.
//!
//! All subcommands are defined here using clap derive. Each subcommand
//! dispatches to its handler in `src/cmd/`.

use clap::{Parser, Subcommand};

/// af — agentic-flow, automatic-flow, or as-fuck.
///
/// Isolated development sessions for AI coding agents. Create a worktree,
/// launch an agent, track everything.
#[derive(Debug, Parser)]
#[command(
    name = "af",
    version,
    about,
    long_about = None,
    propagate_version = true,
)]
pub struct Cli {
    /// Enable verbose output (-v, -vv, -vvv).
    #[arg(short, long, action = clap::ArgAction::Count, global = true)]
    pub verbose: u8,

    /// Subcommand to execute.
    #[command(subcommand)]
    pub command: Commands,
}

/// Top-level subcommands.
#[derive(Debug, Subcommand)]
pub enum Commands {
    /// Create a new workstream: worktree + multiplexer session + agent.
    Create(CreateArgs),
    /// Tear down a workstream.
    Done(DoneArgs),
    /// List active workstreams.
    List,
    /// Re-attach to an existing workstream.
    Resume(ResumeArgs),
    /// Launch agent with a session ID tied to the current branch.
    SessionBranch,
    /// Check dependencies, optionally install missing ones.
    Doctor(DoctorArgs),
    /// Print version and build information.
    Version,
}

/// Arguments for `af create`.
#[derive(Debug, clap::Args)]
pub struct CreateArgs {
    /// Task name (becomes the branch and session name).
    /// If omitted, auto-generates from repo name + timestamp.
    pub name: Option<String>,

    /// Fork from a specific branch instead of the default.
    #[arg(long)]
    pub from: Option<String>,

    /// Fork from the current branch.
    #[arg(long, conflicts_with = "from")]
    pub current: bool,

    /// Create worktree from a GitHub PR number.
    #[arg(long, value_name = "NUMBER")]
    pub from_pr: Option<u64>,

    /// Run agent locally on the host worktree (review/PR mode).
    #[arg(long)]
    pub bare: bool,

    /// Select the AI agent to launch (e.g., "claude", "pi").
    #[arg(long, value_name = "AGENT")]
    pub agent: Option<String>,
}

/// Arguments for `af done`.
#[derive(Debug, clap::Args)]
pub struct DoneArgs {
    /// Session name to tear down. Defaults to the current session.
    pub session: Option<String>,

    /// Force-delete even if the branch is unmerged. Skip confirmation.
    #[arg(long)]
    pub force: bool,
}

/// Arguments for `af resume`.
#[derive(Debug, clap::Args)]
pub struct ResumeArgs {
    /// Session name to resume. If omitted, shows a picker.
    pub session: Option<String>,

    /// Resume in bare mode (skip VM, run agent locally).
    #[arg(long)]
    pub bare: bool,
}

/// Arguments for `af doctor`.
#[derive(Debug, clap::Args)]
pub struct DoctorArgs {
    /// Auto-install missing dependencies.
    #[arg(long)]
    pub fix: bool,

    /// Skip confirmation prompts (use with --fix).
    #[arg(long, requires = "fix")]
    pub yes: bool,
}

impl Cli {
    /// Dispatch to the appropriate subcommand handler.
    pub fn run(&self) -> anyhow::Result<()> {
        match &self.command {
            Commands::Create(args) => crate::cmd::create::run(args),
            Commands::Done(args) => crate::cmd::done::run(args),
            Commands::List => crate::cmd::list::run(),
            Commands::Resume(args) => crate::cmd::resume::run(args),
            Commands::SessionBranch => crate::cmd::session_branch::run(),
            Commands::Doctor(args) => crate::cmd::doctor::run(args),
            Commands::Version => {
                #[allow(clippy::print_stdout)]
                {
                    println!("af {}", env!("CARGO_PKG_VERSION"));
                }
                Ok(())
            }
        }
    }
}
