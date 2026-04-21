//! Integration tests for `SandboxConfig.remote_daemon` (ADR-024).
//!
//! These tests verify:
//! 1. TOML with `[sandbox.remote_daemon]` parses correctly into `RemoteDaemon`.
//! 2. `SlicerProvider` arg-building includes `--url` when `remote_daemon` is set.
//! 3. `--token` is NOT appended when only `url` is configured (no `token_ref`).
//! 4. Token resolution via `AF_SLICER_TOKEN_<UPPERCASE_REF>` env var works.

use af::provider::{RemoteDaemon, SandboxConfig};

// ── TOML parsing ─────────────────────────────────────────────────────────────

/// Verify that a TOML block with `[sandbox.remote_daemon]` deserialises into
/// `SandboxConfig` with the `remote_daemon` field populated.
#[test]
fn test_sandbox_config_remote_daemon_parses_url_and_token_ref() {
    let toml = r#"
[sandbox]
group = "default"

[sandbox.remote_daemon]
url = "https://slicer.example.com:8443"
token_ref = "slicer/example"
"#;

    let config: SandboxConfigToml = toml::from_str(toml).expect("TOML must parse");
    let daemon = config
        .sandbox
        .remote_daemon
        .expect("remote_daemon must be present");

    assert_eq!(daemon.url, "https://slicer.example.com:8443");
    assert_eq!(daemon.token_ref.as_deref(), Some("slicer/example"));
}

/// Verify that `[sandbox.remote_daemon]` with only a URL (no `token_ref`) parses.
#[test]
fn test_sandbox_config_remote_daemon_url_only() {
    let toml = r#"
[sandbox.remote_daemon]
url = "https://slicer.local:9090"
"#;

    let config: SandboxConfigToml = toml::from_str(toml).expect("TOML must parse");
    let daemon = config
        .sandbox
        .remote_daemon
        .expect("remote_daemon must be present");

    assert_eq!(daemon.url, "https://slicer.local:9090");
    assert!(
        daemon.token_ref.is_none(),
        "token_ref should be None when omitted"
    );
}

/// Verify that a `[sandbox]` block without `[sandbox.remote_daemon]` results in `None`.
#[test]
fn test_sandbox_config_remote_daemon_absent_is_none() {
    let toml = r#"
[sandbox]
group = "default"
"#;

    let config: SandboxConfigToml = toml::from_str(toml).expect("TOML must parse");
    assert!(
        config.sandbox.remote_daemon.is_none(),
        "remote_daemon must be None when not configured"
    );
}

/// Verify that an empty TOML document results in `remote_daemon = None`.
#[test]
fn test_sandbox_config_remote_daemon_empty_toml_is_none() {
    let toml = "";
    let config: SandboxConfigToml = toml::from_str(toml).expect("empty TOML must parse");
    assert!(config.sandbox.remote_daemon.is_none());
}

// ── Slicer arg building ───────────────────────────────────────────────────────

/// When `remote_daemon` carries a URL, `slicer_daemon_args` returns `["--url", "<url>"]`.
#[test]
fn test_slicer_daemon_args_with_url_only() {
    let daemon = RemoteDaemon {
        url: "https://slicer.example.com:8443".to_owned(),
        token_ref: None,
    };

    let args = daemon.slicer_args(None);
    assert_eq!(
        args,
        vec![
            "--url".to_owned(),
            "https://slicer.example.com:8443".to_owned()
        ]
    );
}

/// When `remote_daemon` has `token_ref` and the env var is set, `slicer_args` includes
/// `--token <value>`.
#[test]
fn test_slicer_daemon_args_with_token_from_env() {
    let daemon = RemoteDaemon {
        url: "https://slicer.example.com:8443".to_owned(),
        token_ref: Some("slicer/example".to_owned()),
    };

    // AF_SLICER_TOKEN_SLICER_EXAMPLE (/ → _, lowered ref uppercased)
    let token_value = "secret-token-value";
    let args = daemon.slicer_args(Some(token_value));

    assert!(
        args.contains(&"--url".to_owned()),
        "args must contain --url"
    );
    assert!(
        args.contains(&"https://slicer.example.com:8443".to_owned()),
        "args must contain the url value"
    );
    assert!(
        args.contains(&"--token".to_owned()),
        "args must contain --token when token is resolved"
    );
    assert!(
        args.contains(&token_value.to_owned()),
        "args must contain the token value"
    );
}

/// When `token_ref` is set but no token value is supplied, `--token` is NOT added.
#[test]
fn test_slicer_daemon_args_token_ref_without_resolved_value_omits_token() {
    let daemon = RemoteDaemon {
        url: "https://slicer.example.com:8443".to_owned(),
        token_ref: Some("slicer/example".to_owned()),
    };

    let args = daemon.slicer_args(None);

    assert!(
        args.contains(&"--url".to_owned()),
        "args must contain --url"
    );
    assert!(
        !args.contains(&"--token".to_owned()),
        "--token must NOT be present when no token value is resolved"
    );
}

/// `SandboxConfig::slicer_daemon_args` is `None` when `remote_daemon` is `None`.
#[test]
fn test_sandbox_config_slicer_daemon_args_none_when_no_daemon() {
    let config = SandboxConfig {
        group: "default".to_owned(),
        share_home: None,
        remote_daemon: None,
    };

    assert!(config.slicer_daemon_args().is_none());
}

/// `SandboxConfig::slicer_daemon_args` returns `Some(vec)` with `--url` when daemon is set.
#[test]
fn test_sandbox_config_slicer_daemon_args_some_when_daemon_url_set() {
    let config = SandboxConfig {
        group: "default".to_owned(),
        share_home: None,
        remote_daemon: Some(RemoteDaemon {
            url: "https://slicer.example.com:8443".to_owned(),
            token_ref: None,
        }),
    };

    let args = config.slicer_daemon_args().expect("should return Some");
    assert!(args.contains(&"--url".to_owned()));
    assert!(args.contains(&"https://slicer.example.com:8443".to_owned()));
}

// ── Token env-var resolution helper ──────────────────────────────────────────

/// `RemoteDaemon::token_env_var` produces the expected env var name from `token_ref`.
///
/// Conversion rule: replace `/` and `-` with `_`, uppercase all.
/// `"slicer/example"` → `"AF_SLICER_TOKEN_SLICER_EXAMPLE"`
#[test]
fn test_remote_daemon_token_env_var_name() {
    let daemon = RemoteDaemon {
        url: "https://slicer.example.com:8443".to_owned(),
        token_ref: Some("slicer/example".to_owned()),
    };

    let env_var = daemon
        .token_env_var()
        .expect("should return Some for Some token_ref");
    assert_eq!(env_var, "AF_SLICER_TOKEN_SLICER_EXAMPLE");
}

/// `RemoteDaemon::token_env_var` returns `None` when `token_ref` is `None`.
#[test]
fn test_remote_daemon_token_env_var_name_none_when_no_ref() {
    let daemon = RemoteDaemon {
        url: "https://slicer.example.com:8443".to_owned(),
        token_ref: None,
    };

    assert!(daemon.token_env_var().is_none());
}

/// Hyphens in `token_ref` are also replaced by underscores.
#[test]
fn test_remote_daemon_token_env_var_hyphen_replaced() {
    let daemon = RemoteDaemon {
        url: "https://slicer.example.com:8443".to_owned(),
        token_ref: Some("slicer/my-host".to_owned()),
    };

    let env_var = daemon.token_env_var().expect("should return Some");
    assert_eq!(env_var, "AF_SLICER_TOKEN_SLICER_MY_HOST");
}

// ── SlicerProvider agent_sandbox_cmd with daemon args ────────────────────────

/// `agent_sandbox_cmd_with_daemon` prepends `--url` before the subcommand when daemon
/// is configured.
#[test]
fn test_agent_sandbox_cmd_with_daemon_url_prepends_flags() {
    use af::provider::slicer::agent_sandbox_cmd_with_daemon;
    use std::path::Path;

    let daemon = RemoteDaemon {
        url: "https://slicer.example.com:8443".to_owned(),
        token_ref: None,
    };

    let cmd = agent_sandbox_cmd_with_daemon("claude", Path::new("/tmp/project"), &daemon, None)
        .expect("cmd must be Some");

    // Expected: ["slicer", "--url", "<url>", "claude", "/tmp/project"]
    assert_eq!(cmd[0], "slicer");
    assert_eq!(cmd[1], "--url");
    assert_eq!(cmd[2], "https://slicer.example.com:8443");
    assert_eq!(cmd[3], "claude");
    assert_eq!(cmd[4], "/tmp/project");
}

/// When token is also resolved, it appears between `--url` and the subcommand.
#[test]
fn test_agent_sandbox_cmd_with_daemon_url_and_token() {
    use af::provider::slicer::agent_sandbox_cmd_with_daemon;
    use std::path::Path;

    let daemon = RemoteDaemon {
        url: "https://slicer.example.com:8443".to_owned(),
        token_ref: Some("slicer/example".to_owned()),
    };

    let cmd =
        agent_sandbox_cmd_with_daemon("codex", Path::new("/workspace"), &daemon, Some("tok123"))
            .expect("cmd must be Some");

    // Expected: ["slicer", "--url", "<url>", "--token", "tok123", "codex", "/workspace"]
    let url_pos = cmd
        .iter()
        .position(|s| s == "--url")
        .expect("--url missing");
    let tok_pos = cmd
        .iter()
        .position(|s| s == "--token")
        .expect("--token missing");

    // Flags must come before the subcommand
    let sub_pos = cmd
        .iter()
        .position(|s| s == "codex")
        .expect("codex missing");
    assert!(url_pos < sub_pos, "--url must come before subcommand");
    assert!(tok_pos < sub_pos, "--token must come before subcommand");

    // Values follow immediately
    assert_eq!(cmd[url_pos + 1], "https://slicer.example.com:8443");
    assert_eq!(cmd[tok_pos + 1], "tok123");
}

// ── Helper wrapper for TOML deserialisation ───────────────────────────────────

/// A thin TOML wrapper so we can deserialise `[sandbox]` sections in tests
/// without touching `src/config/mod.rs`.
#[derive(Debug, serde::Deserialize, Default)]
#[serde(default)]
struct SandboxConfigToml {
    sandbox: SandboxConfig,
}
