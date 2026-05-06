# ADR-009: Provisioning System

**Status:** Accepted
**Date:** 2026-03-26

## Context

`cf` has provisioning baked into three separate shell scripts, each tightly coupled to the
dotfiles repo:

| Script | Where it runs | What it does |
|---|---|---|
| `cf-bootstrap-remote` | Remote VMs (via SSH) | Install Node.js, tmux, Claude Code, SSH agent fix |
| `cf-bootstrap-slicer` | Slicer VMs (via exec) | Same minus SSH agent fix |
| `cf-provision-dotfiles` | Remote/slicer VMs | Install nvim, stow, gh, dotfiles clone + stow |

All three are hardcoded to one dotfiles repo (`github.com/kakkoyun/dotfiles`), one install
mechanism (`make install/shared`), and one set of tools. Users of `af` will have different
dotfiles repos, different install scripts, and different tool requirements.

Additionally, the user's dotfiles provide an `install.sh` that handles everything end-to-end
(clone, stow, tools) with flags (`--minimal`, `--post-stow`, `--tools-only`). This is the
real interface — not the three bespoke scripts `cf` ships.

### Two faces of provisioning

1. **Local provisioning** — Ensure the local machine has `af`'s own dependencies (git, tmux,
   an agent). This is a pre-flight check that runs every time `af` starts a session.

2. **Remote provisioning** — Set up a remote VM or sandbox from scratch: install a multiplexer,
   install an agent, deploy dotfiles/configs. This runs once per VM creation.

Both share the same pattern: "ensure these tools exist, install if missing, deploy my configs."

## Decision

### Provisioning is a configurable pipeline, not embedded scripts

`af` defines **what needs to exist** (dependencies) and **delegates the how** to user-provided
scripts/repos.

### 1. Dependency manifest (what `af` needs)

`af` maintains a typed dependency graph. Each dependency has:

```rust
pub struct Dependency {
    pub name: &'static str,         // e.g., "git"
    pub tier: Tier,                  // Must | Should | Nice
    pub check: CheckMethod,         // Binary on PATH, version parse, etc.
    pub install: Option<InstallHint>, // Per-platform install commands
}

pub enum Tier {
    /// Session cannot start without this. Abort with clear error.
    Must,
    /// Degraded experience without this. Warn and continue.
    Should,
    /// Silent fallback if missing.
    Nice,
}

pub enum CheckMethod {
    /// `which <binary>` — just check PATH
    Binary(&'static str),
    /// `which <binary>` + parse version from `<binary> --version`
    BinaryVersion(&'static str, &'static str),  // (binary, min_version)
}
```

### Built-in dependency table

| Dependency | Tier | macOS | Arch | Debian |
|---|---|---|---|---|
| `git` | Must | `brew install git` | `pacman -S git` | `apt install git` |
| `tmux` | Must (*) | `brew install tmux` | `pacman -S tmux` | `apt install tmux` |
| `claude` | Should (*) | `npm i -g @anthropic-ai/claude-code` | same | same |
| `gh` | Should | `brew install gh` | `pacman -S github-cli` | [apt repo] |
| `fzf` | Nice | `brew install fzf` | `pacman -S fzf` | `apt install fzf` |
| `node` | Should (**) | `brew install node` | `pacman -S nodejs` | [nodesource] |

(*) "Must" for the selected multiplexer/agent — if you choose `pi`, `claude` becomes Nice.
(**) Required only if the selected agent needs it (Claude Code requires Node.js).

### 2. Pre-flight check (`af doctor`)

Runs the dependency table against the current system:

```
$ af doctor
✓ git 2.44.0
✓ tmux 3.4
✓ claude 1.0.12
✗ gh — not found (install: brew install gh)
  ↳ needed for: af gc (PR state), af create --from-pr
✓ fzf 0.48.0
```

Can also auto-install:

```
af doctor --fix
```

`--fix` calls the platform-appropriate install command from the dependency table.
Requires confirmation unless `--yes` is passed.

### 3. Dotfiles provisioning (configurable)

Instead of hardcoded provisioning scripts, `af` supports a **dotfiles provider** in config:

```toml
# ~/.config/af/config.toml

[provisioning.dotfiles]
# Git repo to clone on remote VMs
repo = "https://github.com/kakkoyun/dotfiles.git"
# Directory to clone into on the remote
target = "~/.dotfiles"
# Command to run after cloning (within the cloned directory)
install_cmd = "./install.sh --minimal"
# Alternatively: just point to a local script to pipe via SSH
# script = "~/.config/af/scripts/provision.sh"
```

When `af create --remote` provisions a VM:

1. **Bootstrap phase:** Install `af`'s Must-tier deps (git, node, tmux, agent) using the
   built-in platform-aware install table. This is `af`'s responsibility.

2. **Dotfiles phase:** If `provisioning.dotfiles.repo` is set, clone it and run
   `install_cmd`. This is the user's responsibility — `af` only triggers it.

3. **Agent auth phase:** Inject agent credentials (per ADR-001's agent provider).

### 4. Platform detection

Three-way detection matching the dotfiles pattern:

```rust
pub enum Platform {
    MacOS,
    Arch,   // Arch, EndeavourOS, Garuda, CachyOS, Artix
    Debian, // Debian, Ubuntu, and all other Linux
}
```

Detection: `uname -s` → Darwin = macOS, Linux = read `/etc/os-release` ID field.

### 5. Embedded default scripts

For zero-config operation, `af` embeds minimal bootstrap scripts (via `include_str!`).
These handle the bootstrap phase only (install core deps). They are used when no custom
`provisioning.dotfiles` is configured.

Users can override with their own provisioning by setting the config.

## Consequences

- `af doctor` gives immediate, actionable feedback on missing tools.
- `af doctor --fix` provides guided installation without requiring users to know package names.
- Dotfiles provisioning is decoupled — any dotfiles repo with an install script works.
- The three bespoke `cf-bootstrap-*` scripts collapse into one configurable pipeline.
- Platform detection is shared between `af doctor` and remote provisioning.
- `af` never installs tools without user consent (except in `--fix` mode with confirmation).
- The dependency tier system means `af` still works in degraded mode (no `gh` = no PR state in gc).
