//! DD Workspaces remote provider.
//!
//! Implements [`RemoteProvider`] for Datadog's internal Workspaces
//! platform by shelling out to the `workspaces` CLI. Per ADR-027, the
//! provider additionally exposes `ssh_target` and `is_alive` as inherent
//! methods (the matching trait migration happens in Phase IV, owned by
//! the lead agent).
//!
//! The full `workspaces` surface is documented in
//! `docs/reference/external-tools.md`. This module wraps the five
//! lifecycle subcommands `af` actually uses:
//!
//! | Operation            | CLI                               |
//! |----------------------|-----------------------------------|
//! | Create a workspace   | `workspaces create <name> …`      |
//! | List workspaces      | `workspaces list`                 |
//! | Update `~/.ssh/config` | `workspaces ssh-config <name>`  |
//! | Delete a workspace   | `workspaces delete <name>`        |
//! | Restart a workspace  | `workspaces restart <name>`       |
//!
//! Every shell-out captures `stdout` + `stderr` so failure messages
//! surface to the caller with context, and no test in this module
//! executes the real binary.

use std::path::Path;
use std::process::Command;

use tracing::debug;

use crate::provider::target::{self, DEFAULT_PROBE_TIMEOUT, Liveness, SshTarget};
use crate::provider::{RemoteInstance, RemoteProvider};

/// DD Workspaces remote development provider.
///
/// Manages cloud development environments via the `workspaces` CLI. See
/// [module-level docs](self) for the subcommand mapping.
pub struct WorkspacesProvider;

/// State reported by `workspaces list` for a given workspace.
///
/// The enum collapses provider-specific strings into the four cases
/// `af` actually cares about. Unknown values become [`WorkspaceState::Other`]
/// so future CLI additions do not silently become "Unreachable".
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum WorkspaceState {
    /// Workspace VM is running and SSH-reachable.
    Running,
    /// VM exists but is paused / stopped.
    Stopped,
    /// VM has been terminated but the CLI may still list it briefly.
    Terminated,
    /// Any other state reported by the CLI.
    Other(String),
}

impl WorkspaceState {
    /// Map a free-form state string (e.g., `"running"`, `"stopped"`) to
    /// the canonical enum. Comparison is case-insensitive.
    pub fn from_raw(raw: &str) -> Self {
        match raw.trim().to_ascii_lowercase().as_str() {
            "running" | "ready" | "active" => Self::Running,
            "stopped" | "paused" | "suspended" => Self::Stopped,
            "terminated" | "deleted" => Self::Terminated,
            other => Self::Other(other.to_owned()),
        }
    }
}

/// Build the argv for a `workspaces` CLI invocation.
///
/// Centralising argv construction keeps tests simple (assert on argv,
/// don't run the real binary). Exposed publicly so the integration test
/// in `tests/provider_remote.rs` can reuse it.
pub fn workspaces_argv(args: &[&str]) -> Vec<String> {
    let mut argv = Vec::with_capacity(args.len() + 1);
    argv.push(String::from("workspaces"));
    for a in args {
        argv.push((*a).to_owned());
    }
    argv
}

/// Parse `workspaces list` plain-text output into [`RemoteInstance`] values.
///
/// The `workspaces list` command prints one workspace per line with
/// whitespace-separated fields; the first token is the workspace name
/// and the last is the state. Lines starting with `NAME` (the header)
/// or `#` (comments) are skipped.
///
/// This is deliberately permissive: the DD CLI sometimes adds extra
/// columns between runs. As long as the name + state positions hold, we
/// stay compatible.
pub fn parse_list_output(text: &str) -> Vec<RemoteInstance> {
    let mut instances = Vec::new();
    for line in text.lines() {
        let trimmed = line.trim();
        if trimmed.is_empty() || trimmed.starts_with('#') {
            continue;
        }
        // Skip header lines (case-insensitive "NAME" as first token).
        let first = trimmed.split_whitespace().next().unwrap_or("");
        if first.eq_ignore_ascii_case("name") {
            continue;
        }
        let tokens: Vec<&str> = trimmed.split_whitespace().collect();
        if tokens.len() < 2 {
            debug!(line = trimmed, "skipping malformed workspaces list line");
            continue;
        }
        let name = tokens[0];
        let status = tokens[tokens.len() - 1];
        instances.push(RemoteInstance {
            id: name.to_owned(),
            name: name.to_owned(),
            ssh_host: name.to_owned(),
            status: status.to_owned(),
        });
    }
    instances
}

/// Find a workspace by name in parsed list output and return its state.
///
/// Returns `None` if the workspace is not present.
pub fn state_for(instances: &[RemoteInstance], name: &str) -> Option<WorkspaceState> {
    instances
        .iter()
        .find(|i| i.name == name)
        .map(|i| WorkspaceState::from_raw(&i.status))
}

impl WorkspacesProvider {
    /// ADR-027 narrowed surface — resolve a workspace name to an SSH target.
    ///
    /// Convention: `workspaces create <name>` + `workspaces ssh-config
    /// <name>` leaves an entry in `~/.ssh/config` with the workspace name
    /// as the alias. Storing anything richer is wasted state.
    pub fn ssh_target(&self, name: &str) -> anyhow::Result<SshTarget> {
        if name.is_empty() {
            anyhow::bail!("workspaces ssh_target: session name must not be empty");
        }
        Ok(SshTarget::new(name))
    }

    /// ADR-027 narrowed surface — liveness for a workspace.
    ///
    /// Strategy: consult `workspaces list` first. If the workspace is
    /// present:
    ///
    /// * `Running` → fall through to the SSH probe (only then can we
    ///   distinguish `Alive` from a VM that is booted but unreachable).
    /// * `Stopped` → [`Liveness::Suspended`] (not an orphan).
    /// * `Terminated` → [`Liveness::Unreachable`] (orphan candidate).
    /// * any other state → [`Liveness::Unknown`] with the raw string.
    ///
    /// If the workspace is not in the list at all, it has been destroyed
    /// behind `af`'s back → [`Liveness::Unreachable`].
    ///
    /// Returns `Err` only on CLI invocation failure (missing binary).
    pub fn is_alive(&self, name: &str) -> anyhow::Result<Liveness> {
        let instances = self.list()?;
        let state = state_for(&instances, name);
        match state {
            Some(WorkspaceState::Running) => {
                let t = self.ssh_target(name)?;
                Ok(target::is_alive(&t, DEFAULT_PROBE_TIMEOUT))
            }
            Some(WorkspaceState::Stopped) => Ok(Liveness::Suspended),
            // Terminated VM or absent entry: both mean the workspace is
            // gone from the provider's registry → orphan candidate.
            Some(WorkspaceState::Terminated) | None => Ok(Liveness::Unreachable),
            Some(WorkspaceState::Other(s)) => {
                Ok(Liveness::Unknown(format!("workspaces reports state {s:?}")))
            }
        }
    }

    /// Run `workspaces ssh-config <name>` to write / refresh the
    /// workspace's entry in `~/.ssh/config`.
    ///
    /// Called once after `create` so subsequent SSH operations do not
    /// need to re-derive the hostname + identity file.
    pub fn ssh_config(&self, name: &str) -> anyhow::Result<()> {
        if name.is_empty() {
            anyhow::bail!("workspaces ssh-config: name must not be empty");
        }
        debug!(name, "refreshing ~/.ssh/config via workspaces ssh-config");
        let output = Command::new("workspaces")
            .args(["ssh-config", name])
            .output()
            .map_err(|err| anyhow::anyhow!("failed to run workspaces ssh-config: {err}"))?;
        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr);
            anyhow::bail!("workspaces ssh-config {name} failed: {stderr}");
        }
        Ok(())
    }

    /// Run `workspaces restart <name>` on an existing workspace.
    ///
    /// Surfaces the CLI's own exit status so callers can decide whether
    /// to retry. `af resume` currently does not call this, but a future
    /// `--respawn` path will.
    pub fn restart(&self, name: &str) -> anyhow::Result<()> {
        if name.is_empty() {
            anyhow::bail!("workspaces restart: name must not be empty");
        }
        debug!(name, "restarting workspace");
        let output = Command::new("workspaces")
            .args(["restart", name])
            .output()
            .map_err(|err| anyhow::anyhow!("failed to run workspaces restart: {err}"))?;
        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr);
            anyhow::bail!("workspaces restart {name} failed: {stderr}");
        }
        Ok(())
    }
}

impl RemoteProvider for WorkspacesProvider {
    fn name(&self) -> &'static str {
        "DD Workspaces"
    }

    fn detect(&self, _org: &str) -> bool {
        // The workspaces CLI is DD-internal; presence on $PATH signals
        // org membership. We use `which` rather than invoking the binary
        // to avoid a daemon round-trip on every detection.
        which::which("workspaces").is_ok()
    }

    fn create(&self, name: &str, repo: &str, branch: Option<&str>) -> anyhow::Result<String> {
        if name.is_empty() {
            anyhow::bail!("workspaces create: name must not be empty");
        }
        debug!(name, repo, ?branch, "creating DD workspace");
        let mut args: Vec<String> = vec![
            String::from("create"),
            name.to_owned(),
            String::from("--repo"),
            repo.to_owned(),
        ];
        if let Some(b) = branch {
            args.push(String::from("--branch"));
            args.push(b.to_owned());
        }
        let output = Command::new("workspaces")
            .args(&args)
            .output()
            .map_err(|err| anyhow::anyhow!("failed to run workspaces create: {err}"))?;
        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr);
            anyhow::bail!("workspaces create {name} failed: {stderr}");
        }
        // Refresh ~/.ssh/config so the alias resolves immediately.
        self.ssh_config(name)?;
        Ok(name.to_owned())
    }

    fn setup(
        &self,
        _ssh_host: &str,
        _repo: &str,
        _branch: Option<&str>,
        _git_root: &Path,
    ) -> anyhow::Result<()> {
        // ADR-027: workspaces owns its own bootstrap (the `create` call
        // above clones the repo). `setup` survives on the trait only
        // until Phase IV migrates the trait; this no-op keeps the
        // contract honest in the meantime.
        Ok(())
    }

    fn teardown(&self, name: &str) -> anyhow::Result<()> {
        if name.is_empty() {
            anyhow::bail!("workspaces teardown: name must not be empty");
        }
        debug!(name, "deleting DD workspace");
        let output = Command::new("workspaces")
            .args(["delete", name])
            .output()
            .map_err(|err| anyhow::anyhow!("failed to run workspaces delete: {err}"))?;
        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr);
            anyhow::bail!("workspaces delete {name} failed: {stderr}");
        }
        Ok(())
    }

    fn list(&self) -> anyhow::Result<Vec<RemoteInstance>> {
        debug!("listing DD workspaces");
        let output = Command::new("workspaces")
            .arg("list")
            .output()
            .map_err(|err| anyhow::anyhow!("failed to run workspaces list: {err}"))?;
        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr);
            anyhow::bail!("workspaces list failed: {stderr}");
        }
        let stdout = String::from_utf8_lossy(&output.stdout);
        Ok(parse_list_output(&stdout))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    // ── name / trait object ─────────────────────────────────────────

    #[test]
    fn test_workspaces_name() {
        let provider = WorkspacesProvider;
        assert_eq!(provider.name(), "DD Workspaces");
    }

    #[test]
    fn test_workspaces_as_trait_object() {
        let provider: Box<dyn RemoteProvider> = Box::new(WorkspacesProvider);
        assert_eq!(provider.name(), "DD Workspaces");
    }

    // ── detect ──────────────────────────────────────────────────────

    #[test]
    fn test_workspaces_detect_returns_bool_without_panic() {
        let provider = WorkspacesProvider;
        let _ = provider.detect("datadog");
        let _ = provider.detect("");
    }

    // ── argv construction ───────────────────────────────────────────

    #[test]
    fn test_workspaces_argv_prepends_binary_name() {
        let argv = workspaces_argv(&["list"]);
        assert_eq!(argv, vec![String::from("workspaces"), String::from("list")]);
    }

    #[test]
    fn test_workspaces_argv_with_multiple_args() {
        let argv = workspaces_argv(&["create", "my-ws", "--repo", "github.com/org/repo"]);
        assert_eq!(
            argv,
            vec![
                String::from("workspaces"),
                String::from("create"),
                String::from("my-ws"),
                String::from("--repo"),
                String::from("github.com/org/repo"),
            ]
        );
    }

    #[test]
    fn test_workspaces_argv_empty_args() {
        let argv = workspaces_argv(&[]);
        assert_eq!(argv, vec![String::from("workspaces")]);
    }

    // ── WorkspaceState::from_raw ────────────────────────────────────

    #[test]
    fn test_workspace_state_running_variants() {
        assert_eq!(WorkspaceState::from_raw("running"), WorkspaceState::Running);
        assert_eq!(WorkspaceState::from_raw("RUNNING"), WorkspaceState::Running);
        assert_eq!(WorkspaceState::from_raw("Ready"), WorkspaceState::Running);
        assert_eq!(WorkspaceState::from_raw("active"), WorkspaceState::Running);
    }

    #[test]
    fn test_workspace_state_stopped_variants() {
        assert_eq!(WorkspaceState::from_raw("stopped"), WorkspaceState::Stopped);
        assert_eq!(WorkspaceState::from_raw("paused"), WorkspaceState::Stopped);
        assert_eq!(
            WorkspaceState::from_raw("suspended"),
            WorkspaceState::Stopped
        );
    }

    #[test]
    fn test_workspace_state_terminated_variants() {
        assert_eq!(
            WorkspaceState::from_raw("terminated"),
            WorkspaceState::Terminated
        );
        assert_eq!(
            WorkspaceState::from_raw("deleted"),
            WorkspaceState::Terminated
        );
    }

    #[test]
    fn test_workspace_state_other_preserves_value() {
        match WorkspaceState::from_raw("pending") {
            WorkspaceState::Other(s) => assert_eq!(s, "pending"),
            other => panic!("expected Other, got {other:?}"),
        }
    }

    #[test]
    fn test_workspace_state_trims_whitespace() {
        assert_eq!(
            WorkspaceState::from_raw("  running  "),
            WorkspaceState::Running
        );
    }

    // ── parse_list_output ───────────────────────────────────────────

    #[test]
    fn test_parse_list_output_simple() {
        let text = "ws-alpha    running\nws-beta     stopped\n";
        let instances = parse_list_output(text);
        assert_eq!(instances.len(), 2);
        assert_eq!(instances[0].name, "ws-alpha");
        assert_eq!(instances[0].status, "running");
        assert_eq!(instances[1].name, "ws-beta");
        assert_eq!(instances[1].status, "stopped");
    }

    #[test]
    fn test_parse_list_output_skips_header() {
        let text = "NAME       STATUS\nws-gamma   running\n";
        let instances = parse_list_output(text);
        assert_eq!(instances.len(), 1);
        assert_eq!(instances[0].name, "ws-gamma");
    }

    #[test]
    fn test_parse_list_output_skips_comments_and_blanks() {
        let text = "# comment\n\n   \nws-delta running\n";
        let instances = parse_list_output(text);
        assert_eq!(instances.len(), 1);
        assert_eq!(instances[0].name, "ws-delta");
    }

    #[test]
    fn test_parse_list_output_picks_last_token_as_status() {
        // CLI adds an extra column between versions — status is still
        // the last token.
        let text = "ws-epsilon   2026-04-21T10:00   running\n";
        let instances = parse_list_output(text);
        assert_eq!(instances.len(), 1);
        assert_eq!(instances[0].status, "running");
    }

    #[test]
    fn test_parse_list_output_ignores_single_token_lines() {
        let text = "ws-only-name\nws-zeta   running\n";
        let instances = parse_list_output(text);
        assert_eq!(instances.len(), 1);
        assert_eq!(instances[0].name, "ws-zeta");
    }

    #[test]
    fn test_parse_list_output_empty() {
        assert!(parse_list_output("").is_empty());
        assert!(parse_list_output("   \n\n").is_empty());
    }

    // ── state_for ───────────────────────────────────────────────────

    #[test]
    fn test_state_for_known_workspace() {
        let instances = vec![
            RemoteInstance {
                id: String::from("a"),
                name: String::from("a"),
                ssh_host: String::from("a"),
                status: String::from("running"),
            },
            RemoteInstance {
                id: String::from("b"),
                name: String::from("b"),
                ssh_host: String::from("b"),
                status: String::from("stopped"),
            },
        ];
        assert_eq!(state_for(&instances, "a"), Some(WorkspaceState::Running));
        assert_eq!(state_for(&instances, "b"), Some(WorkspaceState::Stopped));
        assert_eq!(state_for(&instances, "missing"), None);
    }

    // ── ssh_target ──────────────────────────────────────────────────

    #[test]
    fn test_workspaces_ssh_target_uses_name_as_alias() {
        let provider = WorkspacesProvider;
        let target = provider
            .ssh_target("ws-kakkoyun-42")
            .expect("non-empty name yields a target");
        assert_eq!(target.host, "ws-kakkoyun-42");
    }

    #[test]
    fn test_workspaces_ssh_target_rejects_empty() {
        let provider = WorkspacesProvider;
        assert!(provider.ssh_target("").is_err());
    }

    // ── lifecycle methods reject empty names ────────────────────────
    //
    // We cannot run the real binary in unit tests; what we *can* assert
    // is that invalid inputs fail fast with a helpful message rather
    // than invoking a subprocess.

    #[test]
    fn test_workspaces_create_rejects_empty_name() {
        let provider = WorkspacesProvider;
        let result = provider.create("", "repo", None);
        assert!(result.is_err());
    }

    #[test]
    fn test_workspaces_teardown_rejects_empty_name() {
        let provider = WorkspacesProvider;
        let result = provider.teardown("");
        assert!(result.is_err());
    }

    #[test]
    fn test_workspaces_ssh_config_rejects_empty_name() {
        let provider = WorkspacesProvider;
        let result = provider.ssh_config("");
        assert!(result.is_err());
    }

    #[test]
    fn test_workspaces_restart_rejects_empty_name() {
        let provider = WorkspacesProvider;
        let result = provider.restart("");
        assert!(result.is_err());
    }

    #[test]
    fn test_workspaces_setup_is_noop_for_any_input() {
        // ADR-027: workspaces does not need bootstrap. The trait impl
        // exists for compatibility and must always succeed.
        let provider = WorkspacesProvider;
        let result = provider.setup("host", "repo", Some("main"), Path::new("/tmp/repo"));
        assert!(result.is_ok());
    }
}
