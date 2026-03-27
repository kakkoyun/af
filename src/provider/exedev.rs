//! exe.dev remote provider.
//!
//! Implements [`RemoteProvider`] for the exe.dev cloud development platform.
//! All lifecycle operations shell out to `ssh exe.dev <command>`.

use std::path::Path;
use std::process::Command;
use std::sync::OnceLock;

use tracing::debug;

use crate::provider::{RemoteInstance, RemoteProvider};

/// Cached result of `ssh exe.dev whoami` for the lifetime of the process.
static DETECT_CACHE: OnceLock<bool> = OnceLock::new();

/// exe.dev remote development provider.
///
/// Manages cloud development environments via `ssh exe.dev <command>`.
/// Uses SSH-based CLI surface for all instance lifecycle operations.
pub struct ExedevProvider;

/// Parse the output of `ssh exe.dev ls` into a list of [`RemoteInstance`] values.
///
/// Each non-empty line is expected to contain whitespace-separated fields:
/// `<hostname> <status>`. Lines that do not contain at least two tokens are
/// skipped with a debug log.
pub fn parse_ls_output(text: &str) -> Vec<RemoteInstance> {
    let mut instances = Vec::new();
    for line in text.lines() {
        let trimmed = line.trim();
        if trimmed.is_empty() {
            continue;
        }
        let mut parts = trimmed.split_whitespace();
        let Some(hostname) = parts.next() else {
            continue;
        };
        let Some(status) = parts.next() else {
            debug!(line = trimmed, "skipping malformed ls line: missing status");
            continue;
        };
        instances.push(RemoteInstance {
            id: hostname.to_owned(),
            name: hostname.to_owned(),
            ssh_host: hostname.to_owned(),
            status: status.to_owned(),
        });
    }
    instances
}

/// Build an SSH command targeting `exe.dev` with the given subcommand arguments.
fn ssh_exedev(args: &[&str]) -> Command {
    let mut cmd = Command::new("ssh");
    cmd.arg("exe.dev");
    for arg in args {
        cmd.arg(arg);
    }
    cmd
}

/// Extract the repository name from a full repo URL or path.
///
/// Handles patterns like `git@github.com:user/repo.git`, `https://github.com/user/repo`,
/// and bare names like `my-repo`.
fn repo_name(repo: &str) -> &str {
    let base = repo.rsplit('/').next().unwrap_or(repo);
    // Also handle git@host:user/repo.git — the last segment after ':' and '/'
    let base = base.rsplit(':').next().unwrap_or(base);
    base.strip_suffix(".git").unwrap_or(base)
}

impl RemoteProvider for ExedevProvider {
    fn name(&self) -> &'static str {
        "exe.dev"
    }

    fn detect(&self, _org: &str) -> bool {
        *DETECT_CACHE.get_or_init(|| {
            debug!("probing exe.dev auth via ssh exe.dev whoami");
            let result = ssh_exedev(&["whoami"])
                .stdout(std::process::Stdio::null())
                .stderr(std::process::Stdio::null())
                .status();
            match result {
                Ok(status) => {
                    let ok = status.success();
                    debug!(success = ok, "exe.dev whoami probe completed");
                    ok
                }
                Err(err) => {
                    debug!(%err, "exe.dev whoami probe failed");
                    false
                }
            }
        })
    }

    fn create(&self, name: &str, repo: &str, branch: Option<&str>) -> anyhow::Result<String> {
        debug!(name, repo, ?branch, "creating exe.dev VM");
        let output = ssh_exedev(&["new"])
            .output()
            .map_err(|err| anyhow::anyhow!("failed to run ssh exe.dev new: {err}"))?;

        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr);
            anyhow::bail!("ssh exe.dev new failed: {stderr}");
        }

        let stdout = String::from_utf8_lossy(&output.stdout);
        let hostname = stdout
            .lines()
            .rfind(|l| !l.trim().is_empty())
            .map(str::trim)
            .ok_or_else(|| anyhow::anyhow!("ssh exe.dev new returned no hostname"))?
            .to_owned();

        debug!(%hostname, "exe.dev VM created");
        Ok(hostname)
    }

    fn setup(
        &self,
        ssh_host: &str,
        repo: &str,
        branch: Option<&str>,
        _git_root: &Path,
    ) -> anyhow::Result<()> {
        let name = repo_name(repo);
        let checkout = branch
            .map(|b| format!(" && cd {name} && git checkout {b}"))
            .unwrap_or_default();
        let remote_cmd = format!("git clone {repo}{checkout}");

        debug!(ssh_host, %remote_cmd, "setting up repo on exe.dev VM");
        let status = Command::new("ssh")
            .args([ssh_host, &remote_cmd])
            .status()
            .map_err(|err| anyhow::anyhow!("failed to run ssh {ssh_host}: {err}"))?;

        if !status.success() {
            anyhow::bail!("remote setup on {ssh_host} failed (exit {status})");
        }
        debug!(ssh_host, "exe.dev VM setup complete");
        Ok(())
    }

    fn teardown(&self, name: &str) -> anyhow::Result<()> {
        debug!(name, "tearing down exe.dev VM");
        let output = ssh_exedev(&["rm", name])
            .output()
            .map_err(|err| anyhow::anyhow!("failed to run ssh exe.dev rm: {err}"))?;

        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr);
            anyhow::bail!("ssh exe.dev rm {name} failed: {stderr}");
        }
        debug!(name, "exe.dev VM torn down");
        Ok(())
    }

    fn list(&self) -> anyhow::Result<Vec<RemoteInstance>> {
        debug!("listing exe.dev VMs");
        let output = ssh_exedev(&["ls"])
            .output()
            .map_err(|err| anyhow::anyhow!("failed to run ssh exe.dev ls: {err}"))?;

        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr);
            anyhow::bail!("ssh exe.dev ls failed: {stderr}");
        }

        let stdout = String::from_utf8_lossy(&output.stdout);
        let instances = parse_ls_output(&stdout);
        debug!(count = instances.len(), "exe.dev VMs listed");
        Ok(instances)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    // ── name ────────────────────────────────────────────────────────

    #[test]
    fn test_exedev_name() {
        let provider = ExedevProvider;
        assert_eq!(provider.name(), "exe.dev");
    }

    // ── parse_ls_output ─────────────────────────────────────────────

    #[test]
    fn test_parse_ls_output_valid() {
        let text = "vm-hostname-1    running\nvm-hostname-2    stopped\n";
        let instances = parse_ls_output(text);
        assert_eq!(instances.len(), 2);

        assert_eq!(instances[0].id, "vm-hostname-1");
        assert_eq!(instances[0].name, "vm-hostname-1");
        assert_eq!(instances[0].ssh_host, "vm-hostname-1");
        assert_eq!(instances[0].status, "running");

        assert_eq!(instances[1].id, "vm-hostname-2");
        assert_eq!(instances[1].name, "vm-hostname-2");
        assert_eq!(instances[1].ssh_host, "vm-hostname-2");
        assert_eq!(instances[1].status, "stopped");
    }

    #[test]
    fn test_parse_ls_output_empty() {
        assert!(parse_ls_output("").is_empty());
        assert!(parse_ls_output("   \n  \n").is_empty());
    }

    #[test]
    fn test_parse_ls_output_malformed_skips_bad_lines() {
        let text = "good-host running\nbad-line\n\nanother-host stopped\n";
        let instances = parse_ls_output(text);
        assert_eq!(instances.len(), 2);
        assert_eq!(instances[0].id, "good-host");
        assert_eq!(instances[0].status, "running");
        assert_eq!(instances[1].id, "another-host");
        assert_eq!(instances[1].status, "stopped");
    }

    #[test]
    fn test_parse_ls_output_extra_whitespace() {
        let text = "  host-1   running   extra-field  \n";
        let instances = parse_ls_output(text);
        assert_eq!(instances.len(), 1);
        assert_eq!(instances[0].id, "host-1");
        assert_eq!(instances[0].status, "running");
    }

    #[test]
    fn test_parse_ls_output_single_line() {
        let text = "my-vm running";
        let instances = parse_ls_output(text);
        assert_eq!(instances.len(), 1);
        assert_eq!(instances[0].ssh_host, "my-vm");
        assert_eq!(instances[0].status, "running");
    }

    #[test]
    fn test_parse_ls_output_tabs() {
        let text = "host-a\trunning\nhost-b\tstopped\n";
        let instances = parse_ls_output(text);
        assert_eq!(instances.len(), 2);
        assert_eq!(instances[0].id, "host-a");
        assert_eq!(instances[1].status, "stopped");
    }

    // ── repo_name ───────────────────────────────────────────────────

    #[test]
    fn test_repo_name_https_url() {
        assert_eq!(repo_name("https://github.com/user/my-repo.git"), "my-repo");
    }

    #[test]
    fn test_repo_name_https_no_git_suffix() {
        assert_eq!(repo_name("https://github.com/user/my-repo"), "my-repo");
    }

    #[test]
    fn test_repo_name_ssh_url() {
        assert_eq!(repo_name("git@github.com:user/my-repo.git"), "my-repo");
    }

    #[test]
    fn test_repo_name_bare_name() {
        assert_eq!(repo_name("my-repo"), "my-repo");
    }

    #[test]
    fn test_repo_name_bare_name_with_git_suffix() {
        assert_eq!(repo_name("my-repo.git"), "my-repo");
    }

    // ── ssh_exedev command builder ──────────────────────────────────

    #[test]
    fn test_ssh_exedev_builds_correct_command() {
        let cmd = ssh_exedev(&["ls"]);
        let args: Vec<&std::ffi::OsStr> = cmd.get_args().collect();
        assert_eq!(cmd.get_program(), "ssh");
        assert_eq!(args, &["exe.dev", "ls"]);
    }

    #[test]
    fn test_ssh_exedev_rm_command() {
        let cmd = ssh_exedev(&["rm", "my-vm"]);
        let args: Vec<&std::ffi::OsStr> = cmd.get_args().collect();
        assert_eq!(args, &["exe.dev", "rm", "my-vm"]);
    }

    #[test]
    fn test_ssh_exedev_whoami_command() {
        let cmd = ssh_exedev(&["whoami"]);
        let args: Vec<&std::ffi::OsStr> = cmd.get_args().collect();
        assert_eq!(args, &["exe.dev", "whoami"]);
    }

    #[test]
    fn test_ssh_exedev_new_command() {
        let cmd = ssh_exedev(&["new"]);
        let args: Vec<&std::ffi::OsStr> = cmd.get_args().collect();
        assert_eq!(args, &["exe.dev", "new"]);
    }

    // ── detect does not panic ───────────────────────────────────────

    // Note: detect() calls a real SSH command, so we only verify it
    // returns a bool without panicking. The OnceLock cache means the
    // result is fixed for the process lifetime.
    #[test]
    fn test_exedev_detect_returns_bool() {
        let provider = ExedevProvider;
        // org is ignored for exe.dev — just verify no panic
        let _ = provider.detect("anything");
        let _ = provider.detect("");
    }

    // ── trait object ────────────────────────────────────────────────

    #[test]
    fn test_exedev_as_trait_object() {
        let provider: Box<dyn RemoteProvider> = Box::new(ExedevProvider);
        assert_eq!(provider.name(), "exe.dev");
    }
}
