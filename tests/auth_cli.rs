//! Integration tests for `af auth` command surface.
//!
//! These tests exercise the command handlers directly through the public
//! `af::cmd::auth` API rather than spawning the binary, because the clap
//! enum is not yet registered in `src/cli.rs` (Phase IV integration step).
//! When the lead wires `Commands::Auth(AuthArgs)` into `src/cli.rs`, a
//! second integration test suite using `assert_cmd` can exercise the
//! binary surface end-to-end.
//!
//! ADRs: 016 (keyring), 025 (secret boundaries).

use af::auth::keyring::{FakeKeystore, Keystore, KeystoreError};
use af::auth::redact::SecretString;
use af::cmd::auth::{AuthClearArgs, AuthCtx};
use af::session::store::SessionStore;
use af::session::types::{
    AgentSlot, AgentStatus, ExecutionInfo, ExecutionMode, PrInfo, SessionMeta, SessionState,
    SessionStatus, VersionInfo, WorktreeInfo,
};
use chrono::Utc;
use tempfile::TempDir;

fn sample_state(name: &str, provider: &str) -> SessionState {
    SessionState {
        session: SessionMeta {
            name: name.to_owned(),
            id: String::from("550e8400-e29b-41d4-a716-446655440000"),
            created_at: Utc::now(),
            status: SessionStatus::Active,
        },
        worktree: Some(WorktreeInfo {
            path: format!("/tmp/{name}"),
            branch: format!("k/{name}"),
            base_branch: String::from("main"),
            git_root: String::from("/tmp"),
        }),
        execution: ExecutionInfo {
            mode: ExecutionMode::Local,
            multiplexer: String::from("tmux"),
            multiplexer_session: name.to_owned(),
            ssh_host: None,
            remote_path: None,
            remote_provider: None,
        },
        agents: vec![AgentSlot {
            slot: String::from("primary"),
            provider: provider.to_owned(),
            session_ids: vec![],
            pane: String::from("0"),
            status: AgentStatus::Running,
        }],
        pr: PrInfo::default(),
        versions: VersionInfo {
            af: String::from("0.1.0"),
            agent_config_hash: String::new(),
        },
    }
}

#[test]
fn integration_setup_then_status_then_clear_happy_path() {
    let tmp = TempDir::new().unwrap();
    let sessions = SessionStore::new(tmp.path());
    let keystore = FakeKeystore::new();
    let ctx = AuthCtx {
        keystore: &keystore as &dyn Keystore,
        sessions: &sessions,
    };

    // Seed via the keystore directly (public API — the CLI-driven stdin
    // path is covered in cmd/auth.rs unit tests).
    keystore
        .set(
            "claude",
            SecretString::new(String::from("sk-ant-integration")),
        )
        .unwrap();
    keystore
        .set(
            "gemini",
            SecretString::new(String::from("sk-gem-integration")),
        )
        .unwrap();

    // ── status: labels only, no values ──
    af::cmd::auth::run(
        &af::cmd::auth::AuthArgs {
            action: af::cmd::auth::AuthAction::Status,
        },
        &ctx,
    )
    .unwrap_or_else(|e| panic!("status failed: {e}"));
    // Can't easily capture stdout from `run` directly — re-read via keystore.
    let listed = keystore.list().unwrap();
    assert!(listed.contains(&String::from("claude")));
    assert!(listed.contains(&String::from("gemini")));
    // Scrub sentinel check:
    for name in &listed {
        assert!(!name.contains("sk-ant"));
        assert!(!name.contains("sk-gem"));
    }

    // ── clear: should delete and be idempotent-friendly ──
    let clear_args = af::cmd::auth::AuthArgs {
        action: af::cmd::auth::AuthAction::Clear(AuthClearArgs {
            provider: String::from("claude"),
            kill_sessions: false,
        }),
    };
    af::cmd::auth::run(&clear_args, &ctx).unwrap();
    assert!(matches!(
        keystore.get("claude"),
        Err(KeystoreError::NotFound(_))
    ));
    // Gemini untouched.
    assert_eq!(
        keystore.get("gemini").unwrap().expose_secret(),
        "sk-gem-integration"
    );

    // Clearing the already-cleared entry is friendly (prints "no secret
    // stored" but returns Ok).
    af::cmd::auth::run(&clear_args, &ctx).unwrap();
}

#[test]
fn integration_clear_warns_on_live_session_via_ledger_scan() {
    // ADR-025 §Decision §3 rotation protocol: clear should surface live
    // sessions that still use the provider.
    let tmp = TempDir::new().unwrap();
    let sessions = SessionStore::new(tmp.path());
    sessions
        .save(&sample_state("live-workstream", "claude"))
        .expect("save session");

    let keystore = FakeKeystore::new();
    keystore
        .set("claude", SecretString::new(String::from("sk")))
        .unwrap();

    let ctx = AuthCtx {
        keystore: &keystore as &dyn Keystore,
        sessions: &sessions,
    };

    // Drive the entry point — exit code is what matters; the warning text
    // is covered in cmd/auth.rs unit tests (which redirect stdout). Here
    // we verify the observable side-effect: the keyring entry is gone and
    // the session is preserved (not auto-killed).
    af::cmd::auth::run(
        &af::cmd::auth::AuthArgs {
            action: af::cmd::auth::AuthAction::Clear(AuthClearArgs {
                provider: String::from("claude"),
                kill_sessions: false,
            }),
        },
        &ctx,
    )
    .unwrap();

    assert!(matches!(
        keystore.get("claude"),
        Err(KeystoreError::NotFound(_))
    ));
    // Session is still there — we didn't kill it.
    assert!(sessions.exists("live-workstream"));
}

#[test]
fn integration_secret_never_appears_in_debug_output() {
    // Regression guard for ADR-025 §Decision §4 redaction: the
    // SecretString must not surface its value through Debug, even when
    // composed into an error chain.
    let s = SecretString::new(String::from("sk-ultra-secret-TOKEN"));
    let rendered_debug = format!("{s:?}");
    let rendered_display = format!("{s}");

    // Error chain check: wrap it in an anyhow::Error and re-print.
    let err: anyhow::Error = anyhow::anyhow!("wrapped: {s:?} and {s}");
    let chain = format!("{err:?}");

    for haystack in [&rendered_debug, &rendered_display, &chain] {
        assert!(
            !haystack.contains("sk-ultra-secret-TOKEN"),
            "secret leaked in: {haystack}"
        );
        assert!(
            haystack.contains("REDACTED"),
            "redaction marker missing in: {haystack}"
        );
    }
}

#[test]
fn integration_env_transport_creates_0600_file() {
    // ADR-025 §Decision §2 tmpfs transport — end-to-end roundtrip.
    use af::auth::transport::{read_env_tmpfs_and_unlink, write_env_tmpfs};

    let tmp = TempDir::new().unwrap();
    let path = tmp.path().join("af-abc").join(".env");
    let secret = SecretString::new(String::from("sk-tmpfs-roundtrip"));
    write_env_tmpfs(&path, "ANTHROPIC_API_KEY", &secret).unwrap();

    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        let mode = std::fs::metadata(&path).unwrap().permissions().mode() & 0o777;
        assert_eq!(mode, 0o600, "env file must be 0600");
    }

    let (k, v) = read_env_tmpfs_and_unlink(&path).unwrap();
    assert_eq!(k, "ANTHROPIC_API_KEY");
    assert_eq!(v.expose_secret(), "sk-tmpfs-roundtrip");
    assert!(!path.exists(), "file must be unlinked after read");
}

#[test]
fn integration_inject_host_round_trips_provider_secret() {
    // ADR-025 §Decision §2 host path.
    use af::auth::transport::inject_host;
    use std::collections::HashMap;

    let keystore = FakeKeystore::new();
    keystore
        .set("gemini", SecretString::new(String::from("sk-gem-inject")))
        .unwrap();

    let mut env: HashMap<String, String> = HashMap::new();
    let injected = inject_host(&keystore, "gemini", &mut env).unwrap();
    assert!(injected);
    assert_eq!(
        env.get("GEMINI_API_KEY").map(String::as_str),
        Some("sk-gem-inject")
    );
}
