//! Remote provisioning pipeline (ADR-009).
//!
//! Three-stage provisioning for remote VMs and sandboxes:
//! 1. **Bootstrap** — install core dependencies (git, tmux, agent) using platform package manager
//! 2. **Dotfiles** — clone and run user's dotfiles repo
//! 3. **Auth** — inject agent credentials
//!
//! Default bootstrap scripts are embedded via `include_str!()` for zero-config operation.
//! Users can override by setting `[provisioning.dotfiles]` in config.

use anyhow::{Context, Result};
use tracing::debug;

use crate::config::ProvisioningConfig;

/// Embedded default bootstrap script for Debian/Ubuntu remotes.
pub const DEFAULT_BOOTSTRAP_DEBIAN: &str = include_str!("scripts/bootstrap-debian.sh");

/// Embedded default bootstrap script for Arch Linux remotes.
pub const DEFAULT_BOOTSTRAP_ARCH: &str = include_str!("scripts/bootstrap-arch.sh");

/// Run the full provisioning pipeline on a remote host via SSH.
///
/// Stages: bootstrap → dotfiles → auth (auth is handled separately by the agent provider).
pub fn provision_remote(ssh_host: &str, config: &ProvisioningConfig) -> Result<()> {
    debug!(host = ssh_host, "starting remote provisioning pipeline");

    // Stage 1: Bootstrap — install core deps.
    run_bootstrap(ssh_host)?;

    // Stage 2: Dotfiles — clone and install user's dotfiles.
    if !config.dotfiles.repo.is_empty() {
        run_dotfiles(ssh_host, config)?;
    } else {
        debug!("no dotfiles repo configured, skipping dotfiles stage");
    }

    debug!(host = ssh_host, "provisioning complete");
    Ok(())
}

/// Run the bootstrap stage: install core dependencies on the remote.
fn run_bootstrap(ssh_host: &str) -> Result<()> {
    debug!(host = ssh_host, "running bootstrap stage");

    // Detect remote platform and select appropriate script.
    let script = detect_remote_platform_script(ssh_host);

    let status = std::process::Command::new("ssh")
        .args([ssh_host, "bash", "-s"])
        .stdin(std::process::Stdio::piped())
        .spawn()
        .and_then(|mut child| {
            use std::io::Write;
            if let Some(ref mut stdin) = child.stdin {
                let _ = stdin.write_all(script.as_bytes());
            }
            child.wait()
        })
        .context("failed to run bootstrap script via SSH")?;

    if !status.success() {
        anyhow::bail!("bootstrap script failed on {ssh_host} (exit {status})");
    }

    Ok(())
}

/// Run the dotfiles stage: clone repo and run install command.
fn run_dotfiles(ssh_host: &str, config: &ProvisioningConfig) -> Result<()> {
    let repo = &config.dotfiles.repo;
    let target = if config.dotfiles.target.is_empty() {
        "~/.dotfiles"
    } else {
        &config.dotfiles.target
    };
    let install_cmd = if config.dotfiles.install_cmd.is_empty() {
        "./install.sh"
    } else {
        &config.dotfiles.install_cmd
    };

    debug!(
        host = ssh_host,
        repo, target, install_cmd, "running dotfiles stage"
    );

    let remote_cmd = format!(
        "git clone {repo} {target} 2>/dev/null || (cd {target} && git pull) && cd {target} && {install_cmd}"
    );

    let status = std::process::Command::new("ssh")
        .args([ssh_host, "bash", "-c", &remote_cmd])
        .status()
        .context("failed to run dotfiles provisioning via SSH")?;

    if !status.success() {
        anyhow::bail!("dotfiles provisioning failed on {ssh_host} (exit {status})");
    }

    Ok(())
}

/// Detect the remote platform and return the appropriate bootstrap script.
///
/// Checks for the presence of `apt-get` vs `pacman` on the remote.
fn detect_remote_platform_script(ssh_host: &str) -> &'static str {
    let output = std::process::Command::new("ssh")
        .args([ssh_host, "which", "pacman"])
        .output();

    if output.is_ok_and(|o| o.status.success()) {
        debug!(
            host = ssh_host,
            platform = "arch",
            "detected remote platform"
        );
        DEFAULT_BOOTSTRAP_ARCH
    } else {
        debug!(
            host = ssh_host,
            platform = "debian",
            "detected remote platform (fallback)"
        );
        DEFAULT_BOOTSTRAP_DEBIAN
    }
}

/// Build the SSH command to run a provisioning script on a remote host.
///
/// Exposed for testing without running the actual command.
pub fn build_ssh_provision_cmd(ssh_host: &str, script_content: &str) -> Vec<String> {
    vec![
        "ssh".to_owned(),
        ssh_host.to_owned(),
        "bash".to_owned(),
        "-c".to_owned(),
        script_content.to_owned(),
    ]
}

/// Build the dotfiles clone + install command string.
///
/// Exposed for testing.
pub fn build_dotfiles_cmd(repo: &str, target: &str, install_cmd: &str) -> String {
    format!(
        "git clone {repo} {target} 2>/dev/null || (cd {target} && git pull) && cd {target} && {install_cmd}"
    )
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_build_ssh_provision_cmd() {
        let cmd = build_ssh_provision_cmd("myhost", "echo hello");
        assert_eq!(cmd[0], "ssh");
        assert_eq!(cmd[1], "myhost");
        assert!(cmd.contains(&"echo hello".to_owned()));
    }

    #[test]
    fn test_build_dotfiles_cmd_basic() {
        let cmd = build_dotfiles_cmd(
            "https://github.com/user/dotfiles.git",
            "~/.dotfiles",
            "./install.sh --minimal",
        );
        assert!(cmd.contains("git clone https://github.com/user/dotfiles.git ~/.dotfiles"));
        assert!(cmd.contains("./install.sh --minimal"));
    }

    #[test]
    fn test_build_dotfiles_cmd_includes_pull_fallback() {
        let cmd = build_dotfiles_cmd("repo", "target", "make install");
        assert!(cmd.contains("git pull"));
        assert!(cmd.contains("make install"));
    }

    #[test]
    fn test_default_bootstrap_debian_is_non_empty() {
        assert!(!DEFAULT_BOOTSTRAP_DEBIAN.is_empty());
        assert!(
            DEFAULT_BOOTSTRAP_DEBIAN.contains("apt-get")
                || DEFAULT_BOOTSTRAP_DEBIAN.contains("apt")
        );
    }

    #[test]
    fn test_default_bootstrap_arch_is_non_empty() {
        assert!(!DEFAULT_BOOTSTRAP_ARCH.is_empty());
        assert!(DEFAULT_BOOTSTRAP_ARCH.contains("pacman"));
    }
}
