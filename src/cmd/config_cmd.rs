//! `af config` — configuration management.
//!
//! `af config show` dumps the effective configuration.
//! `af config init` creates a default config file.

use anyhow::{Context, Result};

use crate::cli::ConfigArgs;
use crate::config;

/// Execute the `af config` command.
pub fn run(args: &ConfigArgs) -> Result<()> {
    match &args.action {
        ConfigAction::Show => show(),
        ConfigAction::Init => init(),
    }
}

/// Dump the effective configuration to stdout.
fn show() -> Result<()> {
    let cfg = config::load(None).context("failed to load config")?;
    let toml_str = toml::to_string_pretty(&cfg).context("failed to serialize config")?;

    #[allow(clippy::print_stdout)]
    {
        if let Some(path) = config::user_config_path() {
            if path.exists() {
                println!("# User config: {}", path.display());
            } else {
                println!("# User config: (not found — using defaults)");
            }
        }
        println!("{toml_str}");
    }
    Ok(())
}

/// Create a default config file at the user config path.
fn init() -> Result<()> {
    let path = config::user_config_path().context("cannot determine config directory")?;

    if path.exists() {
        #[allow(clippy::print_stderr)]
        {
            eprintln!("Config already exists at {}", path.display());
            eprintln!("Edit it directly, or delete it and re-run 'af config init'.");
        }
        return Ok(());
    }

    if let Some(parent) = path.parent() {
        std::fs::create_dir_all(parent)
            .with_context(|| format!("failed to create {}", parent.display()))?;
    }

    let cfg = config::Config::default();
    let toml_str = toml::to_string_pretty(&cfg).context("failed to serialize default config")?;

    let content = format!(
        "# af configuration\n\
         # See: https://kakkoyun.github.io/af/\n\
         # Edit this file to customize af's behavior.\n\
         \n\
         {toml_str}"
    );

    std::fs::write(&path, content)
        .with_context(|| format!("failed to write {}", path.display()))?;

    #[allow(clippy::print_stderr)]
    {
        eprintln!("Created config at {}", path.display());
    }
    Ok(())
}

use crate::cli::ConfigAction;
