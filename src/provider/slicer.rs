//! Slicer sandbox provider.
//!
//! Implements [`SandboxProvider`] for the Slicer Firecracker microVM runtime.
//! Shells out to the `slicer` CLI for VM lifecycle management.
//! See ADR-005 for the design rationale and composition model.

use std::path::{Path, PathBuf};
use std::process::Command;

use tracing::debug;

use crate::provider::{ProvisionOpts, RemoteDaemon, SandboxConfig, SandboxHandle, SandboxProvider};

/// Default VM group when none is specified in the config.
const DEFAULT_GROUP: &str = "default";

/// Guest mount point for `VirtioFS` host workspace.
const GUEST_HOST_PREFIX: &str = "/home/ubuntu/host";

/// Slicer Firecracker microVM sandbox provider.
///
/// Manages isolated execution environments via the `slicer` CLI.
/// Sandboxes can run locally (`VirtioFS` mounts the worktree) or on a
/// remote host (repo synced via tar). Currently local-only; remote
/// slicer is not yet supported.
pub struct SlicerProvider;

/// Build the command to launch an agent sandbox.
///
/// Uses `slicer claude`, `slicer codex`, etc. based on the agent name.
/// Returns `None` only when the workdir cannot be converted to a UTF-8 string.
/// Unknown agents fall back to `slicer workspace`.
pub fn agent_sandbox_cmd(agent: &str, workdir: &Path) -> Option<Vec<String>> {
    let workdir_str = workdir.to_str()?;
    let subcommand = match agent {
        "claude" => "claude",
        "codex" => "codex",
        "amp" => "amp",
        "copilot" => "copilot",
        _ => "workspace",
    };
    Some(vec![
        "slicer".to_owned(),
        subcommand.to_owned(),
        workdir_str.to_owned(),
    ])
}

/// Build the command to launch an agent sandbox via a remote daemon.
///
/// Like [`agent_sandbox_cmd`], but prepends `--url <url>` (and optionally
/// `--token <token>`) as global slicer flags before the subcommand and workdir.
///
/// ```text
/// slicer [--url <url>] [--token <tok>] <subcommand> <workdir>
/// ```
///
/// `resolved_token` is the already-resolved token string (e.g. from
/// [`RemoteDaemon::resolve_token`]). Pass `None` when no token is available.
///
/// Returns `None` only when `workdir` cannot be converted to a UTF-8 string.
pub fn agent_sandbox_cmd_with_daemon(
    agent: &str,
    workdir: &std::path::Path,
    daemon: &RemoteDaemon,
    resolved_token: Option<&str>,
) -> Option<Vec<String>> {
    let workdir_str = workdir.to_str()?;
    let subcommand = match agent {
        "claude" => "claude",
        "codex" => "codex",
        "amp" => "amp",
        "copilot" => "copilot",
        _ => "workspace",
    };

    let daemon_args = daemon.slicer_args(resolved_token);

    // Layout: slicer <daemon_flags…> <subcommand> <workdir>
    let mut cmd = Vec::with_capacity(1 + daemon_args.len() + 2);
    cmd.push("slicer".to_owned());
    cmd.extend(daemon_args);
    cmd.push(subcommand.to_owned());
    cmd.push(workdir_str.to_owned());
    Some(cmd)
}

/// Parse JSON output from `slicer vm list --json`.
///
/// Expects a JSON array of objects with at least `hostname` and optionally
/// `id` / `status` fields. Each VM becomes a [`SandboxHandle`].
fn parse_vm_list(json_str: &str) -> anyhow::Result<Vec<SandboxHandle>> {
    let vms: Vec<serde_json::Value> = serde_json::from_str(json_str)
        .map_err(|e| anyhow::anyhow!("failed to parse slicer vm list JSON: {e}"))?;

    let mut handles = Vec::with_capacity(vms.len());
    for vm in &vms {
        let hostname = vm
            .get("hostname")
            .and_then(serde_json::Value::as_str)
            .unwrap_or("")
            .to_owned();
        if hostname.is_empty() {
            continue;
        }
        let id = vm
            .get("id")
            .and_then(serde_json::Value::as_str)
            .unwrap_or(&hostname)
            .to_owned();
        handles.push(SandboxHandle {
            id,
            hostname,
            provider: "slicer".to_owned(),
        });
    }
    Ok(handles)
}

/// Map a host path to its guest-side `VirtioFS` equivalent.
///
/// Strips the `$HOME` prefix (e.g. `/home/user`) and replaces it with
/// [`GUEST_HOST_PREFIX`] (`/home/ubuntu/host`).
fn map_host_to_guest(host_path: &Path) -> anyhow::Result<PathBuf> {
    let home = dirs::home_dir()
        .ok_or_else(|| anyhow::anyhow!("cannot determine home directory for path mapping"))?;

    let relative = host_path.strip_prefix(&home).map_err(|_| {
        anyhow::anyhow!(
            "host path {} is not under home directory {}",
            host_path.display(),
            home.display()
        )
    })?;

    Ok(PathBuf::from(GUEST_HOST_PREFIX).join(relative))
}

impl SandboxProvider for SlicerProvider {
    fn name(&self) -> &'static str {
        "slicer"
    }

    fn is_available(&self) -> bool {
        which::which("slicer").is_ok()
    }

    fn prepare(&self, _config: &SandboxConfig) -> anyhow::Result<()> {
        debug!("checking slicer daemon health via `slicer vm list`");

        let output = Command::new("slicer")
            .args(["vm", "list"])
            .output()
            .map_err(|e| anyhow::anyhow!("failed to run `slicer vm list`: {e}"))?;

        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr);
            anyhow::bail!(
                "slicer daemon is not running or not healthy: {}",
                stderr.trim()
            );
        }

        debug!("slicer daemon is healthy");
        Ok(())
    }

    fn create(&self, name: &str, host: Option<&str>) -> anyhow::Result<SandboxHandle> {
        if let Some(h) = host {
            anyhow::bail!("remote slicer not yet supported (host={h:?}); use local slicer instead");
        }

        let group = DEFAULT_GROUP;
        let tag = format!("af-session={name}");

        debug!(name, group, tag = tag.as_str(), "creating slicer VM");

        let output = Command::new("slicer")
            .args(["vm", "add", group, "--tag", &tag])
            .output()
            .map_err(|e| anyhow::anyhow!("failed to run `slicer vm add`: {e}"))?;

        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr);
            anyhow::bail!("slicer vm add failed: {}", stderr.trim());
        }

        let stdout = String::from_utf8_lossy(&output.stdout);
        let hostname = stdout.trim().to_owned();
        if hostname.is_empty() {
            anyhow::bail!("slicer vm add returned empty hostname");
        }

        debug!(hostname = hostname.as_str(), "slicer VM created");

        Ok(SandboxHandle {
            id: hostname.clone(),
            hostname,
            provider: "slicer".to_owned(),
        })
    }

    fn provision(&self, _handle: &SandboxHandle, _opts: &ProvisionOpts) -> anyhow::Result<()> {
        // Slicer handles provisioning internally during `slicer vm add`.
        // No additional steps required.
        debug!("slicer provision is a no-op (handled internally by slicer)");
        Ok(())
    }

    fn map_path(&self, host_path: &Path) -> anyhow::Result<PathBuf> {
        map_host_to_guest(host_path)
    }

    fn shell_cmd(&self, handle: &SandboxHandle, bootstrap_cmd: &str) -> Vec<String> {
        vec![
            "slicer".to_owned(),
            "vm".to_owned(),
            "shell".to_owned(),
            handle.hostname.clone(),
            "--uid".to_owned(),
            "1000".to_owned(),
            "--bootstrap".to_owned(),
            bootstrap_cmd.to_owned(),
        ]
    }

    fn is_healthy(&self, handle: &SandboxHandle) -> bool {
        debug!(
            hostname = handle.hostname.as_str(),
            "checking slicer VM health"
        );

        let result = Command::new("slicer")
            .args(["vm", "health", &handle.hostname])
            .output();

        match result {
            Ok(output) => {
                let healthy = output.status.success();
                debug!(hostname = handle.hostname.as_str(), healthy, "health check");
                healthy
            }
            Err(e) => {
                debug!(
                    hostname = handle.hostname.as_str(),
                    error = %e,
                    "health check failed to execute"
                );
                false
            }
        }
    }

    fn teardown(&self, handle: &SandboxHandle) -> anyhow::Result<()> {
        debug!(
            hostname = handle.hostname.as_str(),
            "tearing down slicer VM"
        );

        let output = Command::new("slicer")
            .args(["vm", "delete", &handle.hostname])
            .output()
            .map_err(|e| anyhow::anyhow!("failed to run `slicer vm delete`: {e}"))?;

        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr);
            anyhow::bail!(
                "slicer vm delete {} failed: {}",
                handle.hostname,
                stderr.trim()
            );
        }

        debug!(
            hostname = handle.hostname.as_str(),
            "slicer VM deleted successfully"
        );
        Ok(())
    }

    fn list(&self) -> anyhow::Result<Vec<SandboxHandle>> {
        debug!("listing slicer VMs via `slicer vm list --json`");

        let output = Command::new("slicer")
            .args(["vm", "list", "--json"])
            .output()
            .map_err(|e| anyhow::anyhow!("failed to run `slicer vm list --json`: {e}"))?;

        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr);
            anyhow::bail!("slicer vm list --json failed: {}", stderr.trim());
        }

        let stdout = String::from_utf8_lossy(&output.stdout);
        parse_vm_list(&stdout)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    // ── name / availability ─────────────────────────────────────────

    #[test]
    fn test_slicer_name() {
        let provider = SlicerProvider;
        assert_eq!(provider.name(), "slicer");
    }

    #[test]
    fn test_slicer_is_available() {
        // Verifies the method runs without panic. Result depends on env.
        let provider = SlicerProvider;
        let _available = provider.is_available();
    }

    // ── agent_sandbox_cmd ───────────────────────────────────────────

    #[test]
    fn test_agent_sandbox_cmd_claude() {
        let cmd = agent_sandbox_cmd("claude", Path::new("/tmp/project"));
        assert_eq!(
            cmd,
            Some(vec![
                "slicer".to_owned(),
                "claude".to_owned(),
                "/tmp/project".to_owned(),
            ])
        );
    }

    #[test]
    fn test_agent_sandbox_cmd_codex() {
        let cmd = agent_sandbox_cmd("codex", Path::new("/home/user/code"));
        assert_eq!(
            cmd,
            Some(vec![
                "slicer".to_owned(),
                "codex".to_owned(),
                "/home/user/code".to_owned(),
            ])
        );
    }

    #[test]
    fn test_agent_sandbox_cmd_amp() {
        let cmd = agent_sandbox_cmd("amp", Path::new("/workspace"));
        assert_eq!(
            cmd,
            Some(vec![
                "slicer".to_owned(),
                "amp".to_owned(),
                "/workspace".to_owned(),
            ])
        );
    }

    #[test]
    fn test_agent_sandbox_cmd_unknown_falls_back_to_workspace() {
        let cmd = agent_sandbox_cmd("gemini", Path::new("/tmp/proj"));
        assert_eq!(
            cmd,
            Some(vec![
                "slicer".to_owned(),
                "workspace".to_owned(),
                "/tmp/proj".to_owned(),
            ])
        );
    }

    #[test]
    fn test_agent_sandbox_cmd_empty_agent_falls_back_to_workspace() {
        let cmd = agent_sandbox_cmd("", Path::new("/tmp/proj"));
        assert_eq!(
            cmd,
            Some(vec![
                "slicer".to_owned(),
                "workspace".to_owned(),
                "/tmp/proj".to_owned(),
            ])
        );
    }

    // ── map_path (VirtioFS mapping) ─────────────────────────────────

    #[test]
    fn test_map_path_under_home() {
        // Uses the real home directory so this works on any machine.
        let home = dirs::home_dir().expect("home dir must exist for test");
        let host_path = home.join("Workspace/.worktrees/my-project/feature-branch");
        let provider = SlicerProvider;
        let guest = provider.map_path(&host_path).expect("map_path should work");
        assert_eq!(
            guest,
            PathBuf::from("/home/ubuntu/host/Workspace/.worktrees/my-project/feature-branch")
        );
    }

    #[test]
    fn test_map_path_simple_subdir() {
        let home = dirs::home_dir().expect("home dir must exist for test");
        let host_path = home.join("code/repo");
        let provider = SlicerProvider;
        let guest = provider.map_path(&host_path).expect("map_path should work");
        assert_eq!(guest, PathBuf::from("/home/ubuntu/host/code/repo"));
    }

    #[test]
    fn test_map_path_outside_home_fails() {
        let provider = SlicerProvider;
        let result = provider.map_path(Path::new("/tmp/not-under-home"));
        assert!(result.is_err());
        let err = result.unwrap_err().to_string();
        assert!(
            err.contains("not under home directory"),
            "expected 'not under home directory', got: {err}"
        );
    }

    // ── shell_cmd ───────────────────────────────────────────────────

    #[test]
    fn test_slicer_shell_cmd_format() {
        let provider = SlicerProvider;
        let handle = SandboxHandle {
            id: "vm-456".to_owned(),
            hostname: "my-slicer-vm".to_owned(),
            provider: "slicer".to_owned(),
        };
        let cmd = provider.shell_cmd(&handle, "bash -l");
        assert_eq!(
            cmd,
            vec![
                "slicer",
                "vm",
                "shell",
                "my-slicer-vm",
                "--uid",
                "1000",
                "--bootstrap",
                "bash -l"
            ]
        );
    }

    // ── create command generation ───────────────────────────────────

    #[test]
    fn test_create_with_remote_host_errors() {
        let provider = SlicerProvider;
        let result = provider.create("test-session", Some("remote-host"));
        assert!(result.is_err());
        let err = result.unwrap_err().to_string();
        assert!(
            err.contains("remote slicer not yet supported"),
            "expected remote error, got: {err}"
        );
    }

    // ── provision is a no-op ────────────────────────────────────────

    #[test]
    fn test_provision_is_noop() {
        let provider = SlicerProvider;
        let handle = SandboxHandle {
            id: "vm-123".to_owned(),
            hostname: "slicer-vm".to_owned(),
            provider: "slicer".to_owned(),
        };
        let opts = ProvisionOpts {
            inject_ssh_keys: true,
            install_tools: true,
        };
        let result = provider.provision(&handle, &opts);
        assert!(result.is_ok(), "provision should be a no-op success");
    }

    // ── parse_vm_list ───────────────────────────────────────────────

    #[test]
    fn test_parse_vm_list_empty_array() {
        let handles = parse_vm_list("[]").expect("empty array should parse");
        assert!(handles.is_empty());
    }

    #[test]
    fn test_parse_vm_list_single_vm() {
        let json = r#"[{"hostname": "slicer-abc123", "id": "vm-1", "status": "running"}]"#;
        let handles = parse_vm_list(json).expect("single VM should parse");
        assert_eq!(handles.len(), 1);
        assert_eq!(handles[0].hostname, "slicer-abc123");
        assert_eq!(handles[0].id, "vm-1");
        assert_eq!(handles[0].provider, "slicer");
    }

    #[test]
    fn test_parse_vm_list_multiple_vms() {
        let json = r#"[
            {"hostname": "vm-a", "id": "id-a"},
            {"hostname": "vm-b", "id": "id-b"},
            {"hostname": "vm-c", "id": "id-c"}
        ]"#;
        let handles = parse_vm_list(json).expect("multiple VMs should parse");
        assert_eq!(handles.len(), 3);
        assert_eq!(handles[0].hostname, "vm-a");
        assert_eq!(handles[1].hostname, "vm-b");
        assert_eq!(handles[2].hostname, "vm-c");
    }

    #[test]
    fn test_parse_vm_list_missing_id_uses_hostname() {
        let json = r#"[{"hostname": "slicer-no-id"}]"#;
        let handles = parse_vm_list(json).expect("VM without id should parse");
        assert_eq!(handles.len(), 1);
        assert_eq!(handles[0].id, "slicer-no-id");
        assert_eq!(handles[0].hostname, "slicer-no-id");
    }

    #[test]
    fn test_parse_vm_list_skips_empty_hostname() {
        let json = r#"[{"hostname": ""}, {"hostname": "valid-vm"}]"#;
        let handles = parse_vm_list(json).expect("should skip empty hostname");
        assert_eq!(handles.len(), 1);
        assert_eq!(handles[0].hostname, "valid-vm");
    }

    #[test]
    fn test_parse_vm_list_skips_missing_hostname() {
        let json = r#"[{"id": "orphan"}, {"hostname": "good-vm", "id": "id-1"}]"#;
        let handles = parse_vm_list(json).expect("should skip missing hostname");
        assert_eq!(handles.len(), 1);
        assert_eq!(handles[0].hostname, "good-vm");
    }

    #[test]
    fn test_parse_vm_list_invalid_json() {
        let result = parse_vm_list("not json at all");
        assert!(result.is_err());
        let err = result.unwrap_err().to_string();
        assert!(
            err.contains("failed to parse"),
            "expected parse error, got: {err}"
        );
    }

    // ── teardown command args ───────────────────────────────────────
    // (We cannot run the real command in tests, but we verify the
    //  error path for non-existent binary.)

    #[test]
    fn test_teardown_builds_correct_command() {
        // Verify the implementation path by testing the error case:
        // when slicer is not installed, teardown should fail with a
        // descriptive error (not panic).
        if which::which("slicer").is_ok() {
            // If slicer happens to be installed, skip this test.
            return;
        }
        let provider = SlicerProvider;
        let handle = SandboxHandle {
            id: "vm-tear".to_owned(),
            hostname: "teardown-vm".to_owned(),
            provider: "slicer".to_owned(),
        };
        let result = provider.teardown(&handle);
        assert!(result.is_err());
        let err = result.unwrap_err().to_string();
        assert!(
            err.contains("slicer vm delete"),
            "expected slicer vm delete error, got: {err}"
        );
    }

    // ── is_healthy without slicer ───────────────────────────────────

    #[test]
    fn test_is_healthy_returns_false_without_slicer() {
        if which::which("slicer").is_ok() {
            return;
        }
        let provider = SlicerProvider;
        let handle = SandboxHandle {
            id: "vm-789".to_owned(),
            hostname: "healthy-vm".to_owned(),
            provider: "slicer".to_owned(),
        };
        assert!(!provider.is_healthy(&handle));
    }

    // ── trait object ────────────────────────────────────────────────

    #[test]
    fn test_slicer_as_trait_object() {
        let provider: Box<dyn SandboxProvider> = Box::new(SlicerProvider);
        assert_eq!(provider.name(), "slicer");
    }

    // ── default group constant ──────────────────────────────────────

    #[test]
    fn test_default_group_is_set() {
        assert_eq!(DEFAULT_GROUP, "default");
    }

    #[test]
    fn test_guest_host_prefix_is_set() {
        assert_eq!(GUEST_HOST_PREFIX, "/home/ubuntu/host");
    }
}
