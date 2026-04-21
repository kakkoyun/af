//! Docker AI Sandbox provider.
//!
//! Implements [`SandboxProvider`] using the `sbx` CLI (Docker AI Sandboxes).
//! Sandboxes run agents in isolated microVMs with dedicated Docker daemons,
//! filesystems, and network isolation. See <https://docs.docker.com/ai/sandboxes/>.
//!
//! The `sbx` CLI provides:
//! - `sbx run <agent> [path]` — create + attach to sandbox
//! - `sbx create <agent> [path]` — create without attaching
//! - `sbx ls` — list sandboxes
//! - `sbx stop <name>` — pause a sandbox
//! - `sbx rm <name>` — destroy a sandbox
//! - `sbx exec -it <name> bash` — shell into a sandbox

use std::path::{Path, PathBuf};

use tracing::debug;

use crate::provider::{ProvisionOpts, SandboxConfig, SandboxHandle, SandboxProvider};

/// Docker AI Sandbox provider via the `sbx` CLI.
///
/// Manages isolated microVM sandboxes for AI coding agents. Each sandbox
/// gets its own Docker daemon, filesystem, and network. Supports all
/// sbx-native agents: claude, codex, copilot, docker-agent, droid,
/// gemini, kiro, opencode, and shell.
pub struct DockerSandboxProvider;

/// Known agents supported by `sbx run`.
///
/// Full list from <https://docs.docker.com/ai/sandboxes/> CLI surface reference.
const KNOWN_SBX_AGENTS: &[&str] = &[
    "claude",
    "codex",
    "copilot",
    "docker-agent",
    "droid",
    "gemini",
    "kiro",
    "opencode",
    "shell",
];

impl SandboxProvider for DockerSandboxProvider {
    fn name(&self) -> &'static str {
        "docker"
    }

    fn is_available(&self) -> bool {
        which::which("sbx").is_ok()
    }

    fn prepare(&self, _config: &SandboxConfig) -> anyhow::Result<()> {
        if !self.is_available() {
            anyhow::bail!(
                "'sbx' CLI not found. Install Docker AI Sandboxes: https://docs.docker.com/ai/sandboxes/"
            );
        }
        debug!("docker sandbox provider: sbx CLI available");
        Ok(())
    }

    fn create(&self, name: &str, _host: Option<&str>) -> anyhow::Result<SandboxHandle> {
        // sbx create claude <path> --name <name>
        // For now, create without a specific agent — the agent is launched separately.
        debug!(name, "creating docker sandbox");

        let output = std::process::Command::new("sbx")
            .args(["create", "claude", ".", "--name", name])
            .output()
            .map_err(|e| anyhow::anyhow!("failed to run sbx create: {e}"))?;

        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr);
            anyhow::bail!("sbx create failed: {}", stderr.trim());
        }

        Ok(SandboxHandle {
            id: name.to_owned(),
            hostname: name.to_owned(),
            provider: "docker".to_owned(),
        })
    }

    fn provision(&self, _handle: &SandboxHandle, _opts: &ProvisionOpts) -> anyhow::Result<()> {
        // sbx handles provisioning internally (workspace mounting, agent installation).
        debug!("docker sandbox provision: no-op (sbx handles internally)");
        Ok(())
    }

    fn map_path(&self, host_path: &Path) -> anyhow::Result<PathBuf> {
        // sbx mounts the workspace directly — path is preserved.
        Ok(host_path.to_path_buf())
    }

    fn shell_cmd(&self, handle: &SandboxHandle, _bootstrap_cmd: &str) -> Vec<String> {
        vec![
            "sbx".to_owned(),
            "exec".to_owned(),
            "-it".to_owned(),
            handle.hostname.clone(),
            "bash".to_owned(),
        ]
    }

    fn is_healthy(&self, handle: &SandboxHandle) -> bool {
        // Check if the sandbox appears in `sbx ls` output.
        let output = std::process::Command::new("sbx").args(["ls"]).output();

        match output {
            Ok(o) if o.status.success() => {
                let stdout = String::from_utf8_lossy(&o.stdout);
                stdout.contains(&handle.hostname)
            }
            _ => false,
        }
    }

    fn teardown(&self, handle: &SandboxHandle) -> anyhow::Result<()> {
        debug!(name = %handle.hostname, "tearing down docker sandbox");

        let status = std::process::Command::new("sbx")
            .args(["rm", &handle.hostname])
            .status()
            .map_err(|e| anyhow::anyhow!("failed to run sbx rm: {e}"))?;

        if !status.success() {
            anyhow::bail!("sbx rm '{}' failed", handle.hostname);
        }

        Ok(())
    }

    fn list(&self) -> anyhow::Result<Vec<SandboxHandle>> {
        let output = std::process::Command::new("sbx")
            .args(["ls"])
            .output()
            .map_err(|e| anyhow::anyhow!("failed to run sbx ls: {e}"))?;

        if !output.status.success() {
            return Ok(vec![]);
        }

        let stdout = String::from_utf8_lossy(&output.stdout);
        Ok(parse_sbx_ls(&stdout))
    }
}

/// Build the `sbx run` command for launching an agent in a sandbox.
///
/// Maps agent names to sbx-supported agents. Unknown agents fall back
/// to creating a workspace sandbox without a specific agent.
pub fn agent_sandbox_cmd(agent: &str, workdir: &Path) -> Vec<String> {
    let sbx_agent = if KNOWN_SBX_AGENTS.contains(&agent) {
        agent
    } else {
        // Fall back to claude for unknown agents.
        "claude"
    };

    vec![
        "sbx".to_owned(),
        "run".to_owned(),
        sbx_agent.to_owned(),
        workdir.display().to_string(),
        "--branch".to_owned(),
        "auto".to_owned(),
    ]
}

/// Parse `sbx ls` output into sandbox handles.
///
/// The output format is a table with headers. Each data line has the
/// sandbox name in the first column. We skip the header line.
pub fn parse_sbx_ls(output: &str) -> Vec<SandboxHandle> {
    output
        .lines()
        .skip(1) // skip header line
        .filter_map(|line| {
            let name = line.split_whitespace().next()?;
            if name.is_empty() {
                return None;
            }
            Some(SandboxHandle {
                id: name.to_owned(),
                hostname: name.to_owned(),
                provider: "docker".to_owned(),
            })
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_docker_provider_name() {
        let provider = DockerSandboxProvider;
        assert_eq!(provider.name(), "docker");
    }

    #[test]
    fn test_docker_provider_is_available() {
        let provider = DockerSandboxProvider;
        // Result depends on env — just verify no panic.
        let _available = provider.is_available();
    }

    #[test]
    fn test_docker_provider_as_trait_object() {
        let provider: Box<dyn SandboxProvider> = Box::new(DockerSandboxProvider);
        assert_eq!(provider.name(), "docker");
    }

    #[test]
    fn test_docker_provision_is_noop() {
        let provider = DockerSandboxProvider;
        let handle = SandboxHandle {
            id: "test".to_owned(),
            hostname: "test".to_owned(),
            provider: "docker".to_owned(),
        };
        let opts = ProvisionOpts {
            inject_ssh_keys: false,
            install_tools: false,
        };
        assert!(provider.provision(&handle, &opts).is_ok());
    }

    #[test]
    fn test_docker_map_path_is_identity() {
        let provider = DockerSandboxProvider;
        let path = Path::new("/home/user/project");
        let mapped = provider.map_path(path).unwrap();
        assert_eq!(mapped, path);
    }

    #[test]
    fn test_docker_shell_cmd() {
        let provider = DockerSandboxProvider;
        let handle = SandboxHandle {
            id: "my-sandbox".to_owned(),
            hostname: "my-sandbox".to_owned(),
            provider: "docker".to_owned(),
        };
        let cmd = provider.shell_cmd(&handle, "");
        assert_eq!(cmd, vec!["sbx", "exec", "-it", "my-sandbox", "bash"]);
    }

    #[test]
    fn test_agent_sandbox_cmd_claude() {
        let cmd = agent_sandbox_cmd("claude", Path::new("/tmp/project"));
        assert_eq!(
            cmd,
            vec!["sbx", "run", "claude", "/tmp/project", "--branch", "auto"]
        );
    }

    #[test]
    fn test_agent_sandbox_cmd_codex() {
        let cmd = agent_sandbox_cmd("codex", Path::new("/tmp/project"));
        assert_eq!(
            cmd,
            vec!["sbx", "run", "codex", "/tmp/project", "--branch", "auto"]
        );
    }

    #[test]
    fn test_agent_sandbox_cmd_unknown_falls_back_to_claude() {
        let cmd = agent_sandbox_cmd("nonexistent-agent", Path::new("/tmp/project"));
        assert_eq!(cmd[2], "claude");
    }

    #[test]
    fn test_known_sbx_agents_includes_full_sbx_set() {
        let expected = &[
            "claude",
            "codex",
            "copilot",
            "docker-agent",
            "droid",
            "gemini",
            "kiro",
            "opencode",
            "shell",
        ];
        for agent in expected {
            assert!(
                KNOWN_SBX_AGENTS.contains(agent),
                "KNOWN_SBX_AGENTS missing expected agent: {agent}"
            );
        }
        assert_eq!(
            KNOWN_SBX_AGENTS.len(),
            expected.len(),
            "KNOWN_SBX_AGENTS has unexpected agents"
        );
    }

    #[test]
    fn test_parse_sbx_ls_empty() {
        let result = parse_sbx_ls("");
        assert!(result.is_empty());
    }

    #[test]
    fn test_parse_sbx_ls_with_header_only() {
        let result = parse_sbx_ls("NAME  STATUS  AGENT  PORTS\n");
        assert!(result.is_empty());
    }

    #[test]
    fn test_parse_sbx_ls_with_entries() {
        let output = "NAME      STATUS   AGENT   PORTS\nmy-proj   running  claude  8080:3000\nother     stopped  codex   \n";
        let result = parse_sbx_ls(output);
        assert_eq!(result.len(), 2);
        assert_eq!(result[0].hostname, "my-proj");
        assert_eq!(result[0].provider, "docker");
        assert_eq!(result[1].hostname, "other");
    }

    #[test]
    fn test_parse_sbx_ls_skips_empty_lines() {
        let output = "NAME  STATUS\nfoo   running\n\n\nbar   stopped\n";
        let result = parse_sbx_ls(output);
        assert_eq!(result.len(), 2);
    }
}
