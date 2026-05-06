# ADR-010: Platform-Aware Dependency Management

**Status:** Accepted
**Date:** 2026-03-26

## Context

`af` targets three platforms (macOS, Arch Linux, Debian/Ubuntu) each with different package
managers and package names. The existing `cf` scripts duplicate platform detection and package
installation logic across 4+ files. `af` needs a single, centralized approach.

Additionally, `af`'s own dependencies are conditional on user choices:

- Choosing `pi` as agent → `pi` binary needed, `claude` becomes optional
- Choosing `zellij` as multiplexer → `zellij` needed, `tmux` becomes optional
- Using `--from-pr` → `gh` becomes Must-tier for that operation

## Decision

### Centralized platform and package manager abstraction

```rust
pub enum Platform {
    MacOS,
    Arch,
    Debian,
}

pub enum PackageManager {
    Brew,          // macOS
    Pacman,        // Arch (+ yay for AUR if available)
    Apt,           // Debian/Ubuntu
}

impl Platform {
    /// Detect current platform from OS + /etc/os-release
    pub fn detect() -> Result<Self>;

    /// Return the primary package manager for this platform
    pub fn package_manager(&self) -> PackageManager;
}

impl PackageManager {
    /// Install one or more packages. Returns Ok if all succeed.
    pub fn install(&self, packages: &[&str]) -> Result<()>;

    /// Check if a package is installed (not just binary — the package itself)
    pub fn is_installed(&self, package: &str) -> bool;
}
```

### Package name mapping

Some tools have different package names across platforms:

```rust
/// Maps a canonical tool name to platform-specific package names.
pub struct PackageSpec {
    pub canonical: &'static str,     // "gh"
    pub binary: &'static str,        // "gh" (what to check in PATH)
    pub macos: &'static str,         // "gh"
    pub arch: &'static str,          // "github-cli"
    pub debian: InstallMethod,       // Custom (apt repo setup required)
}

pub enum InstallMethod {
    /// Simple package name for the platform's default manager
    Package(&'static str),
    /// Custom install script (e.g., add apt repo first)
    Script(&'static str),
    /// Install via npm (e.g., Claude Code)
    Npm(&'static str),
    /// Install via cargo
    Cargo(&'static str),
    /// Not available on this platform
    Unavailable,
}
```

### Conditional dependency resolution

Dependencies are resolved at runtime based on the effective configuration:

```rust
fn resolve_dependencies(config: &Config) -> Vec<Dependency> {
    let mut deps = vec![
        // Always required
        dep("git", Tier::Must),
    ];

    // Multiplexer
    match config.general.multiplexer.as_str() {
        "tmux" => deps.push(dep("tmux", Tier::Must)),
        "zellij" => deps.push(dep("zellij", Tier::Must)),
        _ => deps.push(dep("tmux", Tier::Must)), // default
    }

    // Agent
    let agent = &config.general.default_agent;
    deps.push(dep(agent, Tier::Must));

    // Agent runtime dependencies
    if agent_needs_node(agent) {
        deps.push(dep("node", Tier::Must));
    }

    // Optional but valuable
    deps.push(dep("gh", Tier::Should));
    deps.push(dep("fzf", Tier::Nice));

    deps
}
```

### `af doctor` output format

```
$ af doctor

Platform: Arch Linux (pacman)
Agent:    claude (default)
Mux:     tmux (default)

Dependencies:
  ✓ git          2.44.0
  ✓ tmux         3.4
  ✓ node         22.0.0
  ✓ claude       1.0.12
  ⚠ gh           not found
                 install: sudo pacman -S github-cli
                 needed for: af gc (PR state), af create --from-pr
  ✓ fzf          0.48.0

Provisioning:
  dotfiles repo: https://github.com/kakkoyun/dotfiles.git
  install cmd:   ./install.sh --minimal
```

### `af doctor --fix` behaviour

1. Compute missing Must + Should dependencies
2. Show what will be installed and how
3. Ask for confirmation (unless `--yes`)
4. Install using platform package manager
5. Re-run checks, report final state

Must-tier failures after `--fix` are fatal errors. Should-tier show warnings. Nice-tier
are silent.

## Consequences

- One place defines all package names for all platforms — no more duplicated `if brew / elif pacman / elif apt` blocks.
- Adding a new dependency is a single struct — platform mapping included.
- `af doctor` is the entry point for "my setup is broken, help me" — replaces scattered error messages.
- Conditional deps prevent unnecessary installs (don't install `node` if using `amp` which is Go-based).
- The `InstallMethod::Script` variant handles special cases (gh's apt repo, nodesource, etc.)
  without complicating the main install path.
- `af doctor --fix` replaces the `make deps` pattern from dotfiles with a self-contained mechanism.
