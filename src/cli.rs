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
    /// Open codebase in an editor.
    Editor(EditorArgs),
    /// Garbage collect merged/closed workstreams.
    Gc(GcArgs),
    /// List active workstreams.
    List,
    /// Re-attach to an existing workstream.
    Resume(ResumeArgs),
    /// Launch agent with a session ID tied to the current branch.
    SessionBranch,
    /// Generate shell completions.
    Completions(CompletionsArgs),
    /// Manage configuration.
    Config(ConfigArgs),
    /// Manage agents within a workstream.
    Agent(AgentArgs),
    /// Open a visual diff of the workstream changes.
    Diff(DiffArgs),
    /// Create a GitHub PR from the current workstream.
    Pr(PrArgs),
    /// Open the Obsidian note for a workstream.
    Note(NoteArgs),
    /// Show workstream analytics from ledger data.
    Stats,
    /// Export ledger data for external analysis.
    Export(ExportArgs),
    /// Check dependencies, optionally install missing ones.
    Doctor(DoctorArgs),
    /// Generate man page and write to stdout.
    #[command(hide = true)]
    Mangen,
    /// Print version and build information.
    Version,
}

/// Arguments for `af create`.
#[derive(Debug, clap::Args)]
#[allow(clippy::struct_excessive_bools)]
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

    /// Run agent on a remote VM (via exe.dev or configured provider).
    #[arg(long, value_name = "HOST")]
    pub remote: Option<Option<String>>,

    /// Run agent inside a Firecracker sandbox (via slicer).
    #[arg(long)]
    pub sandbox: bool,

    /// Skip permission prompts (unattended mode).
    #[arg(long)]
    pub yolo: bool,

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

    /// Respawn dead sandbox VMs instead of failing.
    #[arg(long)]
    pub respawn: bool,
}

/// Arguments for `af editor`.
#[derive(Debug, clap::Args)]
pub struct EditorArgs {
    /// Session name. Defaults to the current session.
    pub session: Option<String>,

    /// Open in terminal editor (`$EDITOR` in a tmux split).
    #[arg(long, short = 't')]
    pub terminal: bool,

    /// Open in visual editor (VS Code or Zed).
    #[arg(long, conflicts_with = "terminal")]
    pub visual: bool,
}

/// Arguments for `af gc`.
#[derive(Debug, clap::Args)]
pub struct GcArgs {
    /// List candidates without cleaning.
    #[arg(long)]
    pub dry_run: bool,

    /// Clean all merged/closed without prompting.
    #[arg(long)]
    pub all: bool,
}

/// Arguments for `af completions`.
#[derive(Debug, clap::Args)]
pub struct CompletionsArgs {
    /// Shell to generate completions for.
    #[arg(value_enum)]
    pub shell: clap_complete::Shell,
}

/// Arguments for `af config`.
#[derive(Debug, clap::Args)]
pub struct ConfigArgs {
    /// Config action to perform.
    #[command(subcommand)]
    pub action: ConfigAction,
}

/// Config subcommands.
#[derive(Debug, Subcommand)]
pub enum ConfigAction {
    /// Dump the effective configuration.
    Show,
    /// Create a default config file.
    Init,
}

/// Arguments for `af diff`.
#[derive(Debug, clap::Args)]
pub struct DiffArgs {
    /// Session name. Defaults to the current multiplexer session.
    pub session: Option<String>,

    /// Base ref to compare from (defaults to session's base branch).
    #[arg(long)]
    pub base: Option<String>,

    /// Open in dark mode.
    #[arg(long)]
    pub dark: bool,

    /// Open in unified view (default: split).
    #[arg(long)]
    pub unified: bool,

    /// Do not open browser automatically.
    #[arg(long)]
    pub no_open: bool,
}

/// Arguments for `af note`.
#[derive(Debug, clap::Args)]
pub struct NoteArgs {
    /// Session name. Defaults to the current multiplexer session.
    pub session: Option<String>,
}

/// Arguments for `af pr`.
#[derive(Debug, clap::Args)]
pub struct PrArgs {
    /// Session name. Defaults to the current multiplexer session.
    pub session: Option<String>,

    /// PR title. If omitted, gh prompts interactively.
    #[arg(long, short = 't')]
    pub title: Option<String>,

    /// Open as draft PR.
    #[arg(long)]
    pub draft: bool,

    /// Open the PR in the browser after creation.
    #[arg(long)]
    pub web: bool,
}

/// Arguments for `af agent`.
#[derive(Debug, clap::Args)]
pub struct AgentArgs {
    /// Agent action to perform.
    #[command(subcommand)]
    pub action: AgentAction,
}

/// Agent management subcommands.
#[derive(Debug, Subcommand)]
pub enum AgentAction {
    /// Add an agent to the current workstream in a new pane.
    Add(AgentAddArgs),
    /// Stop an agent running in a slot.
    Stop(AgentStopArgs),
    /// List agents in the current workstream.
    List,
}

/// Arguments for `af agent add`.
#[derive(Debug, clap::Args)]
pub struct AgentAddArgs {
    /// Slot name for the new agent (e.g., "review", "tests").
    /// Auto-generated if omitted.
    #[arg(long, value_name = "NAME")]
    pub slot: Option<String>,

    /// Agent provider to launch (e.g., "claude", "pi", "codex").
    #[arg(long, value_name = "PROVIDER")]
    pub agent: Option<String>,

    /// Session name. Defaults to the current multiplexer session.
    #[arg(long)]
    pub session: Option<String>,
}

/// Arguments for `af agent stop`.
#[derive(Debug, clap::Args)]
pub struct AgentStopArgs {
    /// Slot name of the agent to stop.
    pub slot: String,

    /// Session name. Defaults to the current multiplexer session.
    #[arg(long)]
    pub session: Option<String>,
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

    /// Show detailed version and path information for each dependency.
    #[arg(long)]
    pub verbose: bool,
}

/// Arguments for `af export`.
#[derive(Debug, clap::Args)]
pub struct ExportArgs {
    /// Output format.
    #[arg(long, default_value = "json")]
    pub format: String,

    /// Session name. If omitted, exports all sessions.
    pub session: Option<String>,
}

impl Cli {
    /// Dispatch to the appropriate subcommand handler.
    pub fn run(&self) -> anyhow::Result<()> {
        match &self.command {
            Commands::Create(args) => crate::cmd::create::run(args),
            Commands::Done(args) => crate::cmd::done::run(args),
            Commands::Editor(args) => crate::cmd::editor::run(args),
            Commands::Gc(args) => crate::cmd::gc::run(args),
            Commands::List => crate::cmd::list::run(),
            Commands::Resume(args) => crate::cmd::resume::run(args),
            Commands::SessionBranch => crate::cmd::session_branch::run(),
            Commands::Completions(args) => {
                let mut cmd = <Self as clap::CommandFactory>::command();
                clap_complete::generate(args.shell, &mut cmd, "af", &mut std::io::stdout());
                Ok(())
            }
            Commands::Config(args) => crate::cmd::config_cmd::run(args),
            Commands::Agent(args) => crate::cmd::agent::run(args),
            Commands::Diff(args) => crate::cmd::diff::run(args),
            Commands::Note(args) => crate::cmd::note::run(args),
            Commands::Pr(args) => crate::cmd::pr::run(args),
            Commands::Stats => crate::cmd::stats::run(),
            Commands::Export(args) => crate::cmd::export::run(args),
            Commands::Doctor(args) => crate::cmd::doctor::run(args),
            Commands::Mangen => {
                let cmd = <Self as clap::CommandFactory>::command();
                let man = clap_mangen::Man::new(cmd);
                man.render(&mut std::io::stdout())?;
                Ok(())
            }
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
