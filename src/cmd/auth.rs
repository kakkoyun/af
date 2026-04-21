//! `af auth` — manage provider API secrets (ADR-016 + ADR-025).
//!
//! Subcommands:
//!
//! | Action  | Description                                                        |
//! |---------|--------------------------------------------------------------------|
//! | setup   | Prompt on stdin for an API key and store it in the OS keyring.     |
//! | reroll  | Replace an existing stored secret (same UX as setup).              |
//! | status  | List providers with a stored secret. **Never prints values.**      |
//! | clear   | Delete the stored secret. Warns about live sessions (ledger scan). |
//!
//! # Why stdin and not argv
//!
//! A secret passed on the command line (`af auth setup --key sk-...`) shows
//! up in `/proc/<pid>/cmdline` and shell history. Reading from stdin (the
//! shell's "read -s" equivalent) keeps the secret out of both.
//!
//! # Phase IV wiring note
//!
//! This module defines its own `AuthArgs` / `AuthAction` clap-derive enum
//! rather than adding a variant to `crate::cli::Commands`, because ADR-015
//! lists `src/cli.rs` as a shared-file ownership. The lead adds the
//! variant + dispatch in `cli.rs` / `cmd/mod.rs` during Phase IV
//! integration; see `handback` notes in the lane report.
//!
//! # Keystore selection
//!
//! `run()` takes a `&dyn Keystore` so the top-level dispatcher can wire in
//! the real backend (or a fake for integration tests). The public
//! [`AuthCtx`] groups the dependencies the command needs (keystore +
//! session store) so Phase IV wiring is a single factory call.

use std::io::{self, BufRead, Write};

use anyhow::{Context, Result};
use clap::{Args, Subcommand};

use crate::auth::keyring::{Keystore, KeystoreError};
use crate::auth::redact::SecretString;
use crate::session::store::SessionStore;
use crate::session::types::SessionState;

/// Arguments for `af auth`.
#[derive(Debug, Args)]
pub struct AuthArgs {
    /// Auth action to perform.
    #[command(subcommand)]
    pub action: AuthAction,
}

/// `af auth` subcommands.
#[derive(Debug, Subcommand)]
pub enum AuthAction {
    /// Prompt for and store a provider API secret.
    Setup(AuthSetArgs),
    /// Replace an existing stored secret. Re-prompts on stdin.
    Reroll(AuthSetArgs),
    /// List providers that have a stored secret. Never prints values.
    Status,
    /// Delete the stored secret for a provider.
    Clear(AuthClearArgs),
}

/// Shared arguments for `setup` and `reroll`.
#[derive(Debug, Args, Clone)]
pub struct AuthSetArgs {
    /// Provider name (e.g., `claude`, `codex`, `gemini`, `amp`, `copilot`, `pi`).
    #[arg(long, value_name = "PROVIDER")]
    pub provider: String,
}

/// Arguments for `af auth clear`.
#[derive(Debug, Args, Clone)]
pub struct AuthClearArgs {
    /// Provider name whose secret should be deleted.
    #[arg(long, value_name = "PROVIDER")]
    pub provider: String,

    /// Terminate sessions that are using this provider before clearing.
    ///
    /// Without this flag `af auth clear` warns about live sessions and still
    /// deletes the keyring entry — but the running agents hold the secret
    /// in memory until they terminate naturally.
    #[arg(long)]
    pub kill_sessions: bool,
}

// ── Context + entry point ────────────────────────────────────────────────────

/// Runtime context for `af auth`.
///
/// Phase IV builds this with the real keyring + the default session store;
/// integration tests build it with a [`FakeKeystore`](crate::auth::keyring::FakeKeystore)
/// and a temp-dir-backed session store.
pub struct AuthCtx<'a> {
    /// The keystore backend to read/write secrets.
    pub keystore: &'a dyn Keystore,
    /// The session store used to find live sessions during `clear`.
    pub sessions: &'a SessionStore,
}

/// Execute the `af auth` command against the given context.
///
/// # Errors
///
/// Returns an error if the keystore backend fails, stdin cannot be read
/// during `setup`/`reroll`, or the session store cannot be scanned during
/// `clear`.
pub fn run(args: &AuthArgs, ctx: &AuthCtx<'_>) -> Result<()> {
    match &args.action {
        AuthAction::Setup(a) | AuthAction::Reroll(a) => {
            set_secret_from_stdin(&mut io::stdin().lock(), &mut io::stdout().lock(), ctx, a)
        }
        AuthAction::Status => status(&mut io::stdout().lock(), ctx),
        AuthAction::Clear(a) => clear(&mut io::stdout().lock(), ctx, a),
    }
}

// ── Setup / reroll ───────────────────────────────────────────────────────────

/// Core setup/reroll impl, parameterised over `stdin` and `stdout` for tests.
///
/// Reads one line from `stdin` (without echoing — callers running under a
/// tty SHOULD use `rpassword` or `termios` to disable echo; the MVP here
/// trusts the caller's tty configuration and documents the caveat).
fn set_secret_from_stdin<R: BufRead, W: Write>(
    stdin: &mut R,
    stdout: &mut W,
    ctx: &AuthCtx<'_>,
    args: &AuthSetArgs,
) -> Result<()> {
    validate_provider(&args.provider)?;

    writeln!(
        stdout,
        "Enter API key for {} (input is read from stdin — use a pipe or paste; \
         echo is NOT disabled by af in this MVP):",
        args.provider
    )?;
    stdout.flush()?;

    let mut line = String::new();
    let n = stdin
        .read_line(&mut line)
        .context("failed to read secret from stdin")?;
    if n == 0 {
        anyhow::bail!("no input read from stdin; aborting");
    }
    let trimmed = line.trim_end_matches(['\r', '\n']).to_owned();
    // Overwrite `line` as soon as we can — belt+suspenders alongside
    // SecretString's Drop.
    line.clear();

    if trimmed.is_empty() {
        anyhow::bail!("empty secret; aborting");
    }

    let secret = SecretString::new(trimmed);
    ctx.keystore
        .set(&args.provider, secret)
        .with_context(|| format!("failed to store secret for {:?}", args.provider))?;

    writeln!(stdout, "stored secret for {:?}", args.provider)?;
    Ok(())
}

// ── Status ───────────────────────────────────────────────────────────────────

/// List provider labels only. Never prints secret values.
fn status<W: Write>(stdout: &mut W, ctx: &AuthCtx<'_>) -> Result<()> {
    let names = ctx
        .keystore
        .list()
        .context("failed to list keystore entries")?;

    if names.is_empty() {
        writeln!(
            stdout,
            "No secrets stored.\n\
             Run 'af auth setup --provider <name>' to add one."
        )?;
        return Ok(());
    }

    writeln!(stdout, "Providers with stored secrets:")?;
    for name in &names {
        writeln!(stdout, "  {name}")?;
    }
    Ok(())
}

// ── Clear ────────────────────────────────────────────────────────────────────

/// Delete the stored secret for a provider.
///
/// Warns about live sessions using that provider (as surfaced from the
/// session store). `--kill-sessions` is honoured by marking those sessions
/// for termination; the actual kill-path is provider-specific and is
/// deferred to Phase IV. For now we **warn and proceed**: the keyring
/// entry is deleted, and the user is told which sessions still hold the
/// key in memory.
fn clear<W: Write>(stdout: &mut W, ctx: &AuthCtx<'_>, args: &AuthClearArgs) -> Result<()> {
    validate_provider(&args.provider)?;

    let live = scan_live_sessions(ctx.sessions, &args.provider);
    if !live.is_empty() {
        writeln!(
            stdout,
            "warning: {count} live session(s) currently use {provider:?}:",
            count = live.len(),
            provider = args.provider,
        )?;
        for name in &live {
            writeln!(stdout, "  - {name}")?;
        }
        if args.kill_sessions {
            writeln!(
                stdout,
                "note: --kill-sessions was requested. Agent termination is \
                 tracked in Phase IV; for now the keyring entry is deleted \
                 and each listed session must be closed with 'af done' to \
                 drop the secret from memory."
            )?;
        } else {
            writeln!(
                stdout,
                "note: keyring change takes effect for NEW launches only. \
                 Running agents hold the key in memory until terminated. \
                 Re-run with --kill-sessions to mark them for termination."
            )?;
        }
    }

    match ctx.keystore.delete(&args.provider) {
        Ok(()) => {
            writeln!(stdout, "cleared secret for {:?}", args.provider)?;
            Ok(())
        }
        Err(KeystoreError::NotFound(_)) => {
            writeln!(stdout, "no secret stored for {:?}", args.provider)?;
            Ok(())
        }
        Err(e) => Err(anyhow::anyhow!(e)),
    }
}

/// Scan the session store for active sessions whose agent list includes
/// `provider`.
///
/// Failures to load an individual session are skipped with a tracing
/// warning — a single corrupt `state.toml` should not prevent `clear` from
/// surfacing the rest. Returns session names.
fn scan_live_sessions(sessions: &SessionStore, provider: &str) -> Vec<String> {
    let names = sessions.list().unwrap_or_default();
    let mut out = Vec::new();
    for name in names {
        match sessions.load(&name) {
            Ok(state) => {
                if session_uses_provider(&state, provider) {
                    out.push(state.session.name);
                }
            }
            Err(e) => {
                tracing::warn!(session = %name, error = %e, "failed to load session state");
            }
        }
    }
    out
}

/// Does the given session have a running agent for `provider`?
fn session_uses_provider(state: &SessionState, provider: &str) -> bool {
    state.agents.iter().any(|a| a.provider == provider)
}

// ── Shared helpers ───────────────────────────────────────────────────────────

/// Validate that a provider name is a recognised agent.
///
/// Accepts the same set of names as the `--agent` flag (ADR-001) plus
/// `"openai"` as an alias for `codex`.
fn validate_provider(name: &str) -> Result<()> {
    const KNOWN: &[&str] = &[
        "claude", "pi", "codex", "openai", "gemini", "amp", "copilot",
    ];
    if KNOWN.contains(&name) {
        Ok(())
    } else {
        anyhow::bail!(
            "unknown provider {name:?}; expected one of: {list}",
            list = KNOWN.join(", ")
        )
    }
}

// ── Tests ────────────────────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;
    use crate::auth::keyring::FakeKeystore;
    use crate::session::types::{
        AgentSlot, AgentStatus, ExecutionInfo, ExecutionMode, PrInfo, SessionMeta, SessionStatus,
        VersionInfo, WorktreeInfo,
    };
    use chrono::Utc;
    use std::io::Cursor;
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

    fn mk_ctx<'a>(keystore: &'a dyn Keystore, store: &'a SessionStore) -> AuthCtx<'a> {
        AuthCtx {
            keystore,
            sessions: store,
        }
    }

    // ── validate_provider ─────────────────────────────────────────────────

    #[test]
    fn test_validate_known_providers() {
        for p in [
            "claude", "pi", "codex", "openai", "gemini", "amp", "copilot",
        ] {
            assert!(validate_provider(p).is_ok(), "{p} should be accepted");
        }
    }

    #[test]
    fn test_validate_rejects_unknown() {
        let err = validate_provider("bogus").unwrap_err();
        let msg = format!("{err}");
        assert!(msg.contains("bogus"));
        assert!(msg.contains("expected one of"));
    }

    // ── setup / reroll ────────────────────────────────────────────────────

    #[test]
    fn test_setup_reads_secret_from_stdin_and_stores() {
        let tmp = TempDir::new().unwrap();
        let sessions = SessionStore::new(tmp.path());
        let keystore = FakeKeystore::new();
        let ctx = mk_ctx(&keystore, &sessions);

        let mut stdin = Cursor::new(b"sk-ant-stdin\n".to_vec());
        let mut stdout: Vec<u8> = Vec::new();
        let args = AuthSetArgs {
            provider: String::from("claude"),
        };
        set_secret_from_stdin(&mut stdin, &mut stdout, &ctx, &args).unwrap();

        let got = keystore.get("claude").unwrap();
        assert_eq!(got.expose_secret(), "sk-ant-stdin");
        let out = String::from_utf8(stdout).unwrap();
        assert!(out.contains("stored secret for \"claude\""));
        // Defense-in-depth: the value never appears in user-visible stdout.
        assert!(!out.contains("sk-ant-stdin"));
    }

    #[test]
    fn test_setup_rejects_empty_input() {
        let tmp = TempDir::new().unwrap();
        let sessions = SessionStore::new(tmp.path());
        let keystore = FakeKeystore::new();
        let ctx = mk_ctx(&keystore, &sessions);

        let mut stdin = Cursor::new(b"\n".to_vec());
        let mut stdout: Vec<u8> = Vec::new();
        let args = AuthSetArgs {
            provider: String::from("claude"),
        };
        let err = set_secret_from_stdin(&mut stdin, &mut stdout, &ctx, &args).unwrap_err();
        let msg = format!("{err}");
        assert!(msg.contains("empty secret"));
    }

    #[test]
    fn test_setup_rejects_unknown_provider() {
        let tmp = TempDir::new().unwrap();
        let sessions = SessionStore::new(tmp.path());
        let keystore = FakeKeystore::new();
        let ctx = mk_ctx(&keystore, &sessions);

        let mut stdin = Cursor::new(b"v\n".to_vec());
        let mut stdout: Vec<u8> = Vec::new();
        let args = AuthSetArgs {
            provider: String::from("not-a-provider"),
        };
        let err = set_secret_from_stdin(&mut stdin, &mut stdout, &ctx, &args).unwrap_err();
        let msg = format!("{err}");
        assert!(msg.contains("unknown provider"));
    }

    #[test]
    fn test_reroll_overwrites_existing_secret() {
        let tmp = TempDir::new().unwrap();
        let sessions = SessionStore::new(tmp.path());
        let keystore = FakeKeystore::new();
        keystore
            .set("claude", SecretString::new(String::from("sk-old")))
            .unwrap();
        let ctx = mk_ctx(&keystore, &sessions);

        let mut stdin = Cursor::new(b"sk-new\n".to_vec());
        let mut stdout: Vec<u8> = Vec::new();
        let args = AuthSetArgs {
            provider: String::from("claude"),
        };
        set_secret_from_stdin(&mut stdin, &mut stdout, &ctx, &args).unwrap();
        assert_eq!(keystore.get("claude").unwrap().expose_secret(), "sk-new");
    }

    // ── status ────────────────────────────────────────────────────────────

    #[test]
    fn test_status_prints_provider_labels_only() {
        let tmp = TempDir::new().unwrap();
        let sessions = SessionStore::new(tmp.path());
        let keystore = FakeKeystore::new();
        keystore
            .set("claude", SecretString::new(String::from("sk-SECRET-1")))
            .unwrap();
        keystore
            .set("gemini", SecretString::new(String::from("sk-SECRET-2")))
            .unwrap();
        let ctx = mk_ctx(&keystore, &sessions);

        let mut stdout: Vec<u8> = Vec::new();
        status(&mut stdout, &ctx).unwrap();
        let out = String::from_utf8(stdout).unwrap();
        assert!(out.contains("claude"));
        assert!(out.contains("gemini"));
        // Critical: no secret values appear.
        assert!(!out.contains("sk-SECRET-1"));
        assert!(!out.contains("sk-SECRET-2"));
    }

    #[test]
    fn test_status_empty_suggests_setup() {
        let tmp = TempDir::new().unwrap();
        let sessions = SessionStore::new(tmp.path());
        let keystore = FakeKeystore::new();
        let ctx = mk_ctx(&keystore, &sessions);

        let mut stdout: Vec<u8> = Vec::new();
        status(&mut stdout, &ctx).unwrap();
        let out = String::from_utf8(stdout).unwrap();
        assert!(out.contains("No secrets stored"));
        assert!(out.contains("af auth setup"));
    }

    // ── clear ─────────────────────────────────────────────────────────────

    #[test]
    fn test_clear_removes_entry() {
        let tmp = TempDir::new().unwrap();
        let sessions = SessionStore::new(tmp.path());
        let keystore = FakeKeystore::new();
        keystore
            .set("claude", SecretString::new(String::from("sk")))
            .unwrap();
        let ctx = mk_ctx(&keystore, &sessions);

        let mut stdout: Vec<u8> = Vec::new();
        let args = AuthClearArgs {
            provider: String::from("claude"),
            kill_sessions: false,
        };
        clear(&mut stdout, &ctx, &args).unwrap();
        assert!(matches!(
            keystore.get("claude"),
            Err(KeystoreError::NotFound(_))
        ));
    }

    #[test]
    fn test_clear_missing_is_friendly() {
        let tmp = TempDir::new().unwrap();
        let sessions = SessionStore::new(tmp.path());
        let keystore = FakeKeystore::new();
        let ctx = mk_ctx(&keystore, &sessions);

        let mut stdout: Vec<u8> = Vec::new();
        let args = AuthClearArgs {
            provider: String::from("claude"),
            kill_sessions: false,
        };
        clear(&mut stdout, &ctx, &args).unwrap();
        let out = String::from_utf8(stdout).unwrap();
        assert!(out.contains("no secret stored"));
    }

    #[test]
    fn test_clear_warns_about_live_sessions() {
        let tmp = TempDir::new().unwrap();
        let sessions = SessionStore::new(tmp.path());
        sessions.save(&sample_state("live-one", "claude")).unwrap();
        sessions.save(&sample_state("live-two", "gemini")).unwrap();

        let keystore = FakeKeystore::new();
        keystore
            .set("claude", SecretString::new(String::from("sk")))
            .unwrap();
        let ctx = mk_ctx(&keystore, &sessions);

        let mut stdout: Vec<u8> = Vec::new();
        let args = AuthClearArgs {
            provider: String::from("claude"),
            kill_sessions: false,
        };
        clear(&mut stdout, &ctx, &args).unwrap();
        let out = String::from_utf8(stdout).unwrap();
        assert!(out.contains("live-one"), "should mention live session");
        assert!(
            !out.contains("live-two"),
            "unrelated session should not appear"
        );
        assert!(out.contains("keyring change takes effect for NEW launches"));
    }

    #[test]
    fn test_clear_with_kill_sessions_prints_followup() {
        let tmp = TempDir::new().unwrap();
        let sessions = SessionStore::new(tmp.path());
        sessions.save(&sample_state("active", "claude")).unwrap();
        let keystore = FakeKeystore::new();
        keystore
            .set("claude", SecretString::new(String::from("sk")))
            .unwrap();
        let ctx = mk_ctx(&keystore, &sessions);

        let mut stdout: Vec<u8> = Vec::new();
        let args = AuthClearArgs {
            provider: String::from("claude"),
            kill_sessions: true,
        };
        clear(&mut stdout, &ctx, &args).unwrap();
        let out = String::from_utf8(stdout).unwrap();
        assert!(out.contains("--kill-sessions was requested"));
    }

    #[test]
    fn test_clear_rejects_unknown_provider() {
        let tmp = TempDir::new().unwrap();
        let sessions = SessionStore::new(tmp.path());
        let keystore = FakeKeystore::new();
        let ctx = mk_ctx(&keystore, &sessions);

        let mut stdout: Vec<u8> = Vec::new();
        let args = AuthClearArgs {
            provider: String::from("bogus"),
            kill_sessions: false,
        };
        let err = clear(&mut stdout, &ctx, &args).unwrap_err();
        assert!(format!("{err}").contains("unknown provider"));
    }

    #[test]
    fn test_session_uses_provider_matches_running_agent() {
        let s = sample_state("x", "claude");
        assert!(session_uses_provider(&s, "claude"));
        assert!(!session_uses_provider(&s, "gemini"));
    }
}
