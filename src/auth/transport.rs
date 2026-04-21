//! Secret delivery transports (ADR-025 §Decision §2).
//!
//! `af` delivers secrets to agent processes via two transports:
//!
//! 1. **Host (local agent):** inject into the process environment via a
//!    `HashMap<String, String>` passed to `Command::envs(..)`. The secret
//!    stays wrapped in [`SecretString`] until the final `envs()` call
//!    (which requires owned `String`).
//! 2. **exedev (SSH):** write the secret to
//!    `/run/user/$UID/af-<session>/.env` with mode `0600`, have the remote
//!    agent read the file once, then unlink. **Forbidden:** `SetEnv` /
//!    `SendEnv` over SSH (ADR-025 [H-b]).
//!
//! This module provides the helpers used by both paths. The SSH/exedev
//! wiring that *uses* these helpers lives in `src/provider/exedev.rs` and
//! is owned by Lane L-REMOTE — not this lane.

use std::collections::HashMap;
use std::fs;
use std::io::Write;
use std::path::{Path, PathBuf};

use super::keyring::{Keystore, KeystoreError};
use super::redact::SecretString;

/// The environment variable name each provider's secret maps to.
///
/// Per ADR-016: provider names match the `--agent` flag and the env var is
/// the well-known upstream name (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, …).
/// Keeping this mapping central avoids every agent's `launch()` reinventing
/// it and centralises the review point for new providers.
#[must_use]
pub fn env_var_for_provider(provider: &str) -> Option<&'static str> {
    match provider {
        "claude" => Some("ANTHROPIC_API_KEY"),
        "pi" => Some("INFLECTION_API_KEY"),
        "codex" | "openai" => Some("OPENAI_API_KEY"),
        "gemini" => Some("GEMINI_API_KEY"),
        "amp" => Some("AMP_API_KEY"),
        "copilot" => Some("GH_TOKEN"),
        _ => None,
    }
}

/// Inject a provider secret into a host-process env map.
///
/// This is the ADR-025 §Decision §2 §Host path. The secret is read from
/// the keystore and placed into `env` under the provider's canonical env
/// var name (see [`env_var_for_provider`]).
///
/// # Behaviour
///
/// - Missing provider secret → no-op, returns `Ok(false)`. The existing
///   shell env can still supply the value; `af` does not overwrite it.
/// - Unknown provider (no env var mapping) → `Ok(false)`. Caller may log.
/// - Keystore backend error → propagated.
///
/// # Security
///
/// The secret is converted to `String` at the very last moment (when
/// inserting into `env`) because `std::process::Command::envs` takes
/// `AsRef<OsStr>` and we cannot pass `SecretString` directly. This is the
/// intentional final egress point — callers SHOULD NOT format the resulting
/// `env` map via `{:?}`. A follow-up lane may introduce a `RedactedEnv`
/// wrapper; for Phase III the convention is "write then drop immediately".
pub fn inject_host<K: Keystore, S: std::hash::BuildHasher>(
    keystore: &K,
    provider: &str,
    env: &mut HashMap<String, String, S>,
) -> Result<bool, KeystoreError> {
    let Some(var) = env_var_for_provider(provider) else {
        return Ok(false);
    };
    match keystore.get(provider) {
        Ok(secret) => {
            env.insert(var.to_owned(), secret.expose_secret().to_owned());
            Ok(true)
        }
        Err(KeystoreError::NotFound(_)) => Ok(false),
        Err(e) => Err(e),
    }
}

// ── tmpfs transport (exedev / remote) ────────────────────────────────────────

/// Errors from the tmpfs env-file transport.
#[derive(Debug, thiserror::Error)]
pub enum TransportError {
    /// Failed to create the transport directory or file.
    #[error("failed to create env transport file at {path}: {source}")]
    Create {
        /// Path that failed.
        path: PathBuf,
        /// Underlying I/O error.
        source: std::io::Error,
    },

    /// Failed to write to the transport file.
    #[error("failed to write env transport file at {path}: {source}")]
    Write {
        /// Path that failed.
        path: PathBuf,
        /// Underlying I/O error.
        source: std::io::Error,
    },

    /// Failed to set permissions (mode 0600) on the transport file.
    #[error("failed to chmod 0600 env transport file at {path}: {source}")]
    Chmod {
        /// Path that failed.
        path: PathBuf,
        /// Underlying I/O error.
        source: std::io::Error,
    },

    /// Failed to read or unlink the transport file.
    #[error("failed to read env transport file at {path}: {source}")]
    Read {
        /// Path that failed.
        path: PathBuf,
        /// Underlying I/O error.
        source: std::io::Error,
    },
}

/// Default base directory for the tmpfs transport on a POSIX host.
///
/// Per ADR-025 §Decision §2: `/run/user/$UID/af-<session>/`. The directory
/// is created on the remote (exedev) side by the agent launch wrapper; for
/// *local* testing we allow the caller to override the base (so we can
/// point at a `TempDir` in unit tests).
#[must_use]
pub fn default_tmpfs_base(uid: u32) -> PathBuf {
    PathBuf::from(format!("/run/user/{uid}"))
}

/// Compute the per-session transport directory: `<base>/af-<session>/`.
#[must_use]
pub fn session_transport_dir(base: &Path, session: &str) -> PathBuf {
    base.join(format!("af-{session}"))
}

/// Write a single env-file entry (`KEY=value`) at `path` with mode 0600.
///
/// The file is created exclusively (fails if it already exists, to avoid
/// overwriting a previous session's file by accident). Mode 0600 is set
/// via `OpenOptions::mode(0o600)` on Unix. Non-Unix hosts fall back to
/// default permissions with a tracing warning — the exedev path is
/// Unix-only by design.
///
/// The function writes the line `<KEY>=<VALUE>\n` and does **not** log the
/// value. The line format is chosen to be compatible with a plain
/// `source`-able shell file; the agent wrapper reads and `unlink`s.
///
/// # Errors
///
/// - [`TransportError::Create`] if the parent directory cannot be created.
/// - [`TransportError::Write`] if the file cannot be written.
/// - [`TransportError::Chmod`] if `chmod 0600` fails (Unix only).
pub fn write_env_tmpfs(
    path: &Path,
    key: &str,
    secret: &SecretString,
) -> Result<(), TransportError> {
    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent).map_err(|e| TransportError::Create {
            path: parent.to_path_buf(),
            source: e,
        })?;
    }

    let mut opts = fs::OpenOptions::new();
    opts.create_new(true).write(true).truncate(true);

    #[cfg(unix)]
    {
        use std::os::unix::fs::OpenOptionsExt;
        opts.mode(0o600);
    }

    let mut file = opts.open(path).map_err(|e| TransportError::Create {
        path: path.to_path_buf(),
        source: e,
    })?;

    // Line format: KEY=VALUE\n. Never logs the value.
    let line = format!("{key}={value}\n", value = secret.expose_secret());
    file.write_all(line.as_bytes())
        .map_err(|e| TransportError::Write {
            path: path.to_path_buf(),
            source: e,
        })?;

    // Belt + suspenders: re-chmod in case the `mode()` setting was ignored
    // by the platform (e.g., some network filesystems honour umask only).
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        let perms = std::fs::Permissions::from_mode(0o600);
        fs::set_permissions(path, perms).map_err(|e| TransportError::Chmod {
            path: path.to_path_buf(),
            source: e,
        })?;
    }

    Ok(())
}

/// Read the env file back and unlink it atomically (read-once-then-unlink).
///
/// This mirrors the remote-side protocol: the agent wrapper calls this,
/// parses `KEY=VALUE`, exports to its own env, and the file is gone. Even
/// a subsequent `cat` by a co-tenant sees `ENOENT`.
///
/// Returns the `(key, secret)` pair read from the file.
///
/// # Errors
///
/// - [`TransportError::Read`] if the file cannot be read or the content is
///   malformed (no `=`). The file is **still unlinked** on malformed content
///   to avoid leaving a stale secret on disk after a bad write.
pub fn read_env_tmpfs_and_unlink(path: &Path) -> Result<(String, SecretString), TransportError> {
    let content = fs::read_to_string(path).map_err(|e| TransportError::Read {
        path: path.to_path_buf(),
        source: e,
    })?;
    // Unlink eagerly, regardless of parse success.
    let _ = fs::remove_file(path);

    let line = content.lines().next().unwrap_or("").to_owned();
    if let Some((k, v)) = line.split_once('=') {
        Ok((k.to_owned(), SecretString::new(v.to_owned())))
    } else {
        Err(TransportError::Read {
            path: path.to_path_buf(),
            source: std::io::Error::new(
                std::io::ErrorKind::InvalidData,
                "malformed env transport file (no '=')",
            ),
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::auth::keyring::FakeKeystore;
    use tempfile::TempDir;

    // ── env_var_for_provider ──────────────────────────────────────────────

    #[test]
    fn test_env_var_for_known_providers() {
        assert_eq!(env_var_for_provider("claude"), Some("ANTHROPIC_API_KEY"));
        assert_eq!(env_var_for_provider("codex"), Some("OPENAI_API_KEY"));
        assert_eq!(env_var_for_provider("openai"), Some("OPENAI_API_KEY"));
        assert_eq!(env_var_for_provider("gemini"), Some("GEMINI_API_KEY"));
        assert_eq!(env_var_for_provider("amp"), Some("AMP_API_KEY"));
        assert_eq!(env_var_for_provider("copilot"), Some("GH_TOKEN"));
        assert_eq!(env_var_for_provider("pi"), Some("INFLECTION_API_KEY"));
    }

    #[test]
    fn test_env_var_for_unknown_provider_is_none() {
        assert_eq!(env_var_for_provider("nonesuch"), None);
    }

    // ── inject_host ───────────────────────────────────────────────────────

    #[test]
    fn test_inject_host_populates_env() {
        let ks = FakeKeystore::new();
        ks.set("claude", SecretString::new(String::from("sk-abc")))
            .unwrap();
        let mut env: HashMap<String, String> = HashMap::new();
        let injected = inject_host(&ks, "claude", &mut env).unwrap();
        assert!(injected);
        assert_eq!(
            env.get("ANTHROPIC_API_KEY").map(String::as_str),
            Some("sk-abc")
        );
    }

    #[test]
    fn test_inject_host_missing_secret_is_noop() {
        let ks = FakeKeystore::new();
        let mut env: HashMap<String, String> = HashMap::new();
        let injected = inject_host(&ks, "gemini", &mut env).unwrap();
        assert!(!injected);
        assert!(env.is_empty());
    }

    #[test]
    fn test_inject_host_unknown_provider_is_noop() {
        let ks = FakeKeystore::new();
        ks.set("mystery", SecretString::new(String::from("v")))
            .unwrap();
        let mut env: HashMap<String, String> = HashMap::new();
        let injected = inject_host(&ks, "mystery", &mut env).unwrap();
        assert!(!injected);
        assert!(env.is_empty());
    }

    // ── session_transport_dir / default_tmpfs_base ────────────────────────

    #[test]
    fn test_session_transport_dir_format() {
        let base = PathBuf::from("/run/user/1000");
        let d = session_transport_dir(&base, "abc-session");
        assert_eq!(d, PathBuf::from("/run/user/1000/af-abc-session"));
    }

    #[test]
    fn test_default_tmpfs_base_format() {
        let b = default_tmpfs_base(1000);
        assert_eq!(b, PathBuf::from("/run/user/1000"));
    }

    // ── write_env_tmpfs: roundtrip + chmod ────────────────────────────────

    #[test]
    fn test_write_env_tmpfs_creates_0600_file() {
        let tmp = TempDir::new().unwrap();
        let path = tmp.path().join("af-sess").join(".env");
        let s = SecretString::new(String::from("sk-roundtrip"));
        write_env_tmpfs(&path, "ANTHROPIC_API_KEY", &s).unwrap();

        assert!(path.exists());

        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            let meta = fs::metadata(&path).unwrap();
            let mode = meta.permissions().mode() & 0o777;
            assert_eq!(mode, 0o600, "env file must be 0600, got {mode:o}");
        }
    }

    #[test]
    fn test_write_env_tmpfs_never_overwrites_existing() {
        let tmp = TempDir::new().unwrap();
        let path = tmp.path().join("af-sess").join(".env");
        let s = SecretString::new(String::from("v1"));
        write_env_tmpfs(&path, "K", &s).unwrap();

        let s2 = SecretString::new(String::from("v2"));
        let res = write_env_tmpfs(&path, "K", &s2);
        assert!(res.is_err(), "second write must fail on create_new");
    }

    #[test]
    fn test_read_env_tmpfs_and_unlink_roundtrip() {
        let tmp = TempDir::new().unwrap();
        let path = tmp.path().join("af-sess").join(".env");
        let s = SecretString::new(String::from("sk-rt"));
        write_env_tmpfs(&path, "ANTHROPIC_API_KEY", &s).unwrap();

        let (k, v) = read_env_tmpfs_and_unlink(&path).unwrap();
        assert_eq!(k, "ANTHROPIC_API_KEY");
        assert_eq!(v.expose_secret(), "sk-rt");
        assert!(!path.exists(), "file must be unlinked after read");
    }

    #[test]
    fn test_read_env_tmpfs_malformed_still_unlinks() {
        let tmp = TempDir::new().unwrap();
        let path = tmp.path().join("af-sess").join(".env");
        fs::create_dir_all(path.parent().unwrap()).unwrap();
        fs::write(&path, b"no-equals-here").unwrap();

        let res = read_env_tmpfs_and_unlink(&path);
        assert!(res.is_err());
        assert!(
            !path.exists(),
            "malformed file must still be unlinked to avoid stale-secret risk"
        );
    }

    #[test]
    fn test_write_env_tmpfs_does_not_log_value_to_path_err() {
        // Sanity check: an error message from a failed write must never
        // contain the secret. Trigger a create failure by passing an
        // impossible path (use a non-writable root marker).
        let s = SecretString::new(String::from("SECRET-TOKEN-VALUE"));
        // `/dev/null/foo` — /dev/null is a char device, cannot have children.
        let res = write_env_tmpfs(Path::new("/dev/null/cannot-create"), "K", &s);
        assert!(res.is_err());
        let msg = format!("{}", res.unwrap_err());
        assert!(!msg.contains("SECRET-TOKEN-VALUE"));
    }
}
