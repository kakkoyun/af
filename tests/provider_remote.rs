//! Integration tests for the narrowed remote-provider surface (ADR-027).
//!
//! These tests exercise the inherent methods added by Lane L-REMOTE —
//! `ssh_target`, `is_alive`, and the concrete workspaces lifecycle
//! wrappers — through the public library API. They never invoke the
//! real `workspaces` or `ssh` binary with a reachable host; every
//! assertion is about argv construction, liveness contracts, or parser
//! behaviour.

use std::time::Duration;

use af::provider::exedev::ExedevProvider;
use af::provider::target::{DEFAULT_PROBE_TIMEOUT, Liveness, SshTarget, is_alive as probe};

#[cfg(feature = "workspaces")]
use af::provider::workspaces::{
    WorkspaceState, WorkspacesProvider, parse_list_output, state_for, workspaces_argv,
};

// ── SshTarget + free probe ─────────────────────────────────────────

#[test]
fn integration_ssh_target_stores_host() {
    let t = SshTarget::new("my-remote");
    assert_eq!(t.host, "my-remote");
}

#[test]
fn integration_probe_unresolvable_host_never_returns_alive() {
    let t = SshTarget::new("af-lane-l-unresolvable-host.invalid");
    let liveness = probe(&t, Duration::from_secs(1));
    assert!(!matches!(liveness, Liveness::Alive | Liveness::Suspended));
}

#[test]
fn integration_default_probe_timeout_is_four_seconds() {
    assert_eq!(DEFAULT_PROBE_TIMEOUT, Duration::from_secs(4));
}

// ── ExedevProvider narrowed surface ────────────────────────────────

#[test]
fn integration_exedev_ssh_target_maps_name_to_alias() {
    let provider = ExedevProvider;
    let t = provider
        .ssh_target("session-abc")
        .expect("non-empty name yields a target");
    assert_eq!(t.host, "session-abc");
}

#[test]
fn integration_exedev_ssh_target_rejects_empty_name() {
    let provider = ExedevProvider;
    assert!(provider.ssh_target("").is_err());
}

#[test]
fn integration_exedev_is_alive_contract_never_panics() {
    let provider = ExedevProvider;
    let liveness = provider
        .is_alive("af-lane-l-integration-absent")
        .expect("exedev is_alive never errors");
    // We only assert on the contract: invalid hosts never appear alive.
    assert!(!matches!(liveness, Liveness::Alive | Liveness::Suspended));
}

// ── WorkspacesProvider (default feature) ───────────────────────────

#[cfg(feature = "workspaces")]
#[test]
fn integration_workspaces_argv_prepends_binary() {
    let argv = workspaces_argv(&["list"]);
    assert_eq!(argv, vec![String::from("workspaces"), String::from("list")]);
}

#[cfg(feature = "workspaces")]
#[test]
fn integration_workspaces_argv_create_shape() {
    let argv = workspaces_argv(&["create", "my-ws", "--repo", "example.com/r"]);
    assert_eq!(argv.first().map(String::as_str), Some("workspaces"));
    assert!(argv.contains(&String::from("create")));
    assert!(argv.contains(&String::from("my-ws")));
    assert!(argv.contains(&String::from("--repo")));
    assert!(argv.contains(&String::from("example.com/r")));
}

#[cfg(feature = "workspaces")]
#[test]
fn integration_workspaces_parse_list_output_and_state_for() {
    let text = concat!(
        "NAME        STATUS\n",
        "ws-alpha    running\n",
        "ws-beta     stopped\n",
        "ws-gamma    terminated\n",
    );
    let instances = parse_list_output(text);
    assert_eq!(instances.len(), 3);
    assert_eq!(
        state_for(&instances, "ws-alpha"),
        Some(WorkspaceState::Running)
    );
    assert_eq!(
        state_for(&instances, "ws-beta"),
        Some(WorkspaceState::Stopped)
    );
    assert_eq!(
        state_for(&instances, "ws-gamma"),
        Some(WorkspaceState::Terminated)
    );
    assert_eq!(state_for(&instances, "missing"), None);
}

#[cfg(feature = "workspaces")]
#[test]
fn integration_workspaces_ssh_target_rejects_empty() {
    let provider = WorkspacesProvider;
    assert!(provider.ssh_target("").is_err());
}
