//! `af doctor` — check dependencies and optionally install missing ones.
//!
//! Validates that all required tools are available on the current system.
//! Reports their status using the dependency tier system (ADR-009, ADR-010).

use anyhow::{Context, Result};

use crate::cli::DoctorArgs;
use crate::config;
use crate::platform::Platform;
use crate::platform::deps::{CheckMethod, Dependency, Tier};

/// Execute the `af doctor` command.
pub fn run(args: &DoctorArgs) -> Result<()> {
    let cfg = config::load(None).context("failed to load config")?;
    let platform = Platform::detect().context("failed to detect platform")?;

    #[allow(clippy::print_stdout)]
    {
        println!(
            "Platform: {} ({})",
            platform.display_name(),
            platform.package_manager()
        );
        println!("Agent:    {} (default)", cfg.general.default_agent);
        println!("Mux:      {} (default)", cfg.general.multiplexer);
        println!();
    }

    let deps = build_dependency_list(&cfg);

    let mut missing_must = Vec::new();
    let mut missing_should = Vec::new();

    #[allow(clippy::print_stdout)]
    {
        println!("Dependencies:");
    }

    for dep in &deps {
        let satisfied = dep.is_satisfied();
        let symbol = if satisfied {
            "✓"
        } else {
            match dep.tier {
                Tier::Must => "✗",
                Tier::Should => "⚠",
                Tier::Nice => "·",
            }
        };

        #[allow(clippy::print_stdout)]
        if satisfied {
            println!("  {symbol} {:<14} found", dep.name);
        } else {
            println!("  {symbol} {:<14} not found", dep.name);
            if !satisfied {
                match dep.tier {
                    Tier::Must => missing_must.push(dep),
                    Tier::Should => missing_should.push(dep),
                    Tier::Nice => {} // silent
                }
            }
        }
    }

    if args.fix && (!missing_must.is_empty() || !missing_should.is_empty()) {
        let pkg_mgr = platform.package_manager();
        let all_missing: Vec<&Dependency> = missing_must
            .iter()
            .chain(missing_should.iter())
            .copied()
            .collect();

        #[allow(clippy::print_stderr)]
        {
            eprintln!();
            eprintln!(
                "Attempting to install {} missing dependencies via {pkg_mgr}...",
                all_missing.len()
            );
        }

        for dep in &all_missing {
            let binary = match &dep.check {
                CheckMethod::Binary(b) => b.as_str(),
            };
            let pkg_name = pkg_mgr.package_name(binary);

            // Skip npm-distributed agents — can't install via system package manager.
            if matches!(binary, "claude" | "codex" | "pi" | "gemini" | "amp") {
                #[allow(clippy::print_stderr)]
                {
                    eprintln!(
                        "  ⏭ {:<14} (install manually — not a system package)",
                        dep.name
                    );
                }
                continue;
            }

            let cmd_parts = pkg_mgr.install_cmd(pkg_name);
            let cmd_str = cmd_parts.join(" ");
            #[allow(clippy::print_stderr)]
            {
                eprintln!("  ▶ {cmd_str}");
            }

            let status = std::process::Command::new(&cmd_parts[0])
                .args(&cmd_parts[1..])
                .status();

            match status {
                Ok(s) if s.success() => {
                    #[allow(clippy::print_stderr)]
                    {
                        eprintln!("  ✓ {:<14} installed", dep.name);
                    }
                }
                Ok(s) => {
                    #[allow(clippy::print_stderr)]
                    {
                        eprintln!("  ✗ {:<14} install failed (exit {})", dep.name, s);
                    }
                }
                Err(e) => {
                    #[allow(clippy::print_stderr)]
                    {
                        eprintln!("  ✗ {:<14} install failed: {e}", dep.name);
                    }
                }
            }
        }

        // Re-check after install attempt.
        let still_missing: Vec<&str> = all_missing
            .iter()
            .filter(|d| !d.is_satisfied())
            .map(|d| d.name.as_str())
            .collect();
        if still_missing.is_empty() {
            #[allow(clippy::print_stderr)]
            {
                eprintln!();
                eprintln!("All dependencies now satisfied.");
            }
            return Ok(());
        }
    }

    if !missing_must.is_empty() {
        let names: Vec<&str> = missing_must.iter().map(|d| d.name.as_str()).collect();
        anyhow::bail!(
            "required dependencies missing: {}. Install them and re-run 'af doctor'.",
            names.join(", ")
        );
    }

    if missing_should.is_empty() {
        #[allow(clippy::print_stdout)]
        {
            println!();
            println!("All dependencies satisfied.");
        }
    }

    Ok(())
}

/// Build the dependency list based on current configuration.
fn build_dependency_list(cfg: &config::Config) -> Vec<Dependency> {
    let mut deps = vec![Dependency {
        name: "git".to_owned(),
        tier: Tier::Must,
        check: CheckMethod::Binary("git".to_owned()),
    }];

    // Multiplexer.
    let mux_binary = match cfg.general.multiplexer.as_str() {
        "zellij" => "zellij",
        _ => "tmux",
    };
    deps.push(Dependency {
        name: mux_binary.to_owned(),
        tier: Tier::Must,
        check: CheckMethod::Binary(mux_binary.to_owned()),
    });

    // Agent.
    let agent_binary = match cfg.general.default_agent.as_str() {
        "pi" => "pi",
        "codex" => "codex",
        "gemini" => "gemini",
        "amp" => "amp",
        _ => "claude",
    };
    deps.push(Dependency {
        name: agent_binary.to_owned(),
        tier: Tier::Must,
        check: CheckMethod::Binary(agent_binary.to_owned()),
    });

    // Node.js (required if agent is claude — it's an npm package).
    if matches!(cfg.general.default_agent.as_str(), "claude") {
        deps.push(Dependency {
            name: "node".to_owned(),
            tier: Tier::Should,
            check: CheckMethod::Binary("node".to_owned()),
        });
    }

    // Optional tools.
    deps.push(Dependency {
        name: "gh".to_owned(),
        tier: Tier::Should,
        check: CheckMethod::Binary("gh".to_owned()),
    });
    deps.push(Dependency {
        name: "fzf".to_owned(),
        tier: Tier::Nice,
        check: CheckMethod::Binary("fzf".to_owned()),
    });

    deps
}
