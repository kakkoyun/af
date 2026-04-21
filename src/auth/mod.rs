//! Secret management (ADR-016 as narrowed by ADR-025).
//!
//! This module owns the `af auth` surface: a small [`Keystore`] trait for
//! platform-specific secret backends, a [`redact::SecretString`] newtype
//! that prevents accidental `{:?}` leaks, and a tmpfs delivery transport
//! for remote (exedev) sessions.
//!
//! # Boundary rule (ADR-025 §Decision)
//!
//! > af's keyring stores secrets for **host agents and exedev SSH sessions
//! > only**; every other path (sbx, slicer, workspaces) defers to the
//! > provider-native secret store.
//!
//! This module therefore exposes two transports:
//!
//! 1. [`transport::inject_host`] — in-process env map, for local launches.
//! 2. [`transport::write_env_tmpfs`] — `/run/user/$UID/af-<session>/.env`
//!    mode 0600, read-once-then-unlink, for exedev.
//!
//! SSH `SetEnv` / `SendEnv` is **forbidden** (ADR-025 [H-b]).
//!
//! # Module map
//!
//! - [`redact`] — [`SecretString`], redaction helpers, panic hook.
//! - [`keyring`] — [`Keystore`] trait + in-memory fake. Real backend is
//!   stubbed behind `#[cfg(feature = "keyring")]` until Phase IV wires
//!   `dep:keyring`.
//! - [`transport`] — host env injection + tmpfs env-file delivery.
//!
//! # Feature gating
//!
//! - No feature required: trait, fake, and tmpfs transport all compile.
//! - `keyring` feature: real OS keyring backend (currently stubbed).
//!
//! # Example
//!
//! ```
//! use af::auth::keyring::{FakeKeystore, Keystore};
//! use af::auth::redact::SecretString;
//!
//! let ks = FakeKeystore::new();
//! ks.set("claude", SecretString::new(String::from("sk-..."))).unwrap();
//! let s = ks.get("claude").unwrap();
//! assert_eq!(s.expose_secret(), "sk-...");
//! // Debug output is redacted — safe to log.
//! assert_eq!(format!("{s:?}"), "SecretString([REDACTED])");
//! ```

pub mod keyring;
pub mod redact;
pub mod transport;

pub use keyring::{FakeKeystore, Keystore, KeystoreError};
pub use redact::{SecretString, install_redacting_panic_hook, redact_env_like};
pub use transport::{
    TransportError, default_tmpfs_base, env_var_for_provider, inject_host,
    read_env_tmpfs_and_unlink, session_transport_dir, write_env_tmpfs,
};
