//! Keyring backend abstraction (ADR-016, narrowed by ADR-025).
//!
//! Defines [`Keystore`], the trait all secret backends implement, plus an
//! in-memory [`FakeKeystore`] used in tests and a real backend stub gated
//! behind `#[cfg(feature = "keyring")]`.
//!
//! # Account naming
//!
//! Per ADR-016 amendment (2026-04-21) and ADR-025 [C 2.2]:
//! - Service name: `"af"` (shared across all providers).
//! - Account name: `<provider>` (no `af/` prefix — the service already scopes).
//!
//! # Platform-specific backends
//!
//! The real backend is not yet wired because `Cargo.toml` cannot be
//! modified by this lane per ADR-015 file-ownership rules. When Phase IV
//! adds `keyring = "3"` + `dep:keyring` to the `keyring` feature, swap
//! [`KeyringKeystore`] to a `keyring::Entry`-backed implementation that:
//!
//! - **macOS**: uses `keyring::Entry::new("af", provider)`. macOS Keychain is
//!   user-scoped and hardware-backed on Apple Silicon.
//! - **Linux**: creates a **dedicated, non-default collection** via D-Bus
//!   `CreateCollection` with label `af-<uuid>` and auto-lock on idle, then
//!   stores entries in *that* collection (not the default). This is the
//!   ADR-025 §Decision §1 requirement and is non-negotiable for security.
//!
//! The trait surface is stable across the stub → real switch.

use std::collections::HashMap;
use std::sync::Mutex;

use super::redact::SecretString;

/// Errors from keystore operations.
#[derive(Debug, thiserror::Error)]
pub enum KeystoreError {
    /// The requested provider has no stored secret.
    #[error("no secret stored for provider {0:?}")]
    NotFound(String),

    /// The platform has no available secret service (Linux with no
    /// kwallet / gnome-keyring; macOS with Keychain locked).
    #[error(
        "no secret service available — install and unlock kwallet or gnome-keyring.\n  \
         Arch: pacman -S kwallet kwallet-pam  (or: gnome-keyring libsecret)"
    )]
    ServiceUnavailable,

    /// The backend returned an unexpected error.
    #[error("keystore backend error: {0}")]
    Backend(String),
}

/// Abstract keyring-like storage for provider API secrets.
///
/// Implementors provide platform-specific persistence. All values are
/// wrapped in [`SecretString`] for redaction safety. The provider name is
/// plain (e.g., `"claude"`, `"openai"`, `"gemini"`).
///
/// # Required invariants
///
/// - `set(p, v)` followed by `get(p)` returns a value equal to `v`.
/// - `set(p, v1)` followed by `set(p, v2)` and `get(p)` returns `v2`.
/// - `delete(p)` makes subsequent `get(p)` return `NotFound`.
/// - `list()` returns only provider names; never secret values.
pub trait Keystore {
    /// Fetch the secret for a provider.
    fn get(&self, provider: &str) -> Result<SecretString, KeystoreError>;

    /// Store a secret for a provider. Overwrites any existing entry.
    fn set(&self, provider: &str, secret: SecretString) -> Result<(), KeystoreError>;

    /// Delete the stored secret for a provider.
    ///
    /// Returns `Ok(())` on success. Returns `KeystoreError::NotFound` if no
    /// entry existed (callers that don't care can ignore the error).
    fn delete(&self, provider: &str) -> Result<(), KeystoreError>;

    /// List provider names that have a stored secret.
    ///
    /// Never returns secret *values* — labels only.
    fn list(&self) -> Result<Vec<String>, KeystoreError>;
}

// ── FakeKeystore ─────────────────────────────────────────────────────────────

/// In-memory [`Keystore`] implementation for unit tests.
///
/// Uses a `Mutex<HashMap>` internally for interior mutability through `&self`
/// (to match the real backend's signature: D-Bus / Security Framework
/// calls don't require `&mut`).
#[derive(Debug, Default)]
pub struct FakeKeystore {
    store: Mutex<HashMap<String, String>>,
}

impl FakeKeystore {
    /// Create an empty fake keystore.
    #[must_use]
    pub fn new() -> Self {
        Self::default()
    }
}

impl Keystore for FakeKeystore {
    fn get(&self, provider: &str) -> Result<SecretString, KeystoreError> {
        let guard = self
            .store
            .lock()
            .map_err(|e| KeystoreError::Backend(format!("lock poisoned: {e}")))?;
        guard
            .get(provider)
            .map(|s| SecretString::new(s.clone()))
            .ok_or_else(|| KeystoreError::NotFound(provider.to_owned()))
    }

    fn set(&self, provider: &str, secret: SecretString) -> Result<(), KeystoreError> {
        let mut guard = self
            .store
            .lock()
            .map_err(|e| KeystoreError::Backend(format!("lock poisoned: {e}")))?;
        guard.insert(provider.to_owned(), secret.expose_secret().to_owned());
        Ok(())
    }

    fn delete(&self, provider: &str) -> Result<(), KeystoreError> {
        let mut guard = self
            .store
            .lock()
            .map_err(|e| KeystoreError::Backend(format!("lock poisoned: {e}")))?;
        guard
            .remove(provider)
            .map(|_| ())
            .ok_or_else(|| KeystoreError::NotFound(provider.to_owned()))
    }

    fn list(&self) -> Result<Vec<String>, KeystoreError> {
        let guard = self
            .store
            .lock()
            .map_err(|e| KeystoreError::Backend(format!("lock poisoned: {e}")))?;
        let mut names: Vec<String> = guard.keys().cloned().collect();
        names.sort();
        Ok(names)
    }
}

// ── KeyringKeystore (real backend, feature-gated) ────────────────────────────

/// The real OS-keyring backend.
///
/// Currently compiled out because `keyring` is not yet wired as a Cargo
/// dependency (blocked by ADR-015 file-ownership rules on `Cargo.toml`).
/// When Phase IV wires `keyring = "3"` under `dep:keyring`, the
/// `#[cfg(feature = "keyring")]` block becomes the production backend.
///
/// **Phase IV wiring checklist** (lead-only):
///
/// ```toml
/// [dependencies]
/// keyring = { version = "3", optional = true, default-features = false,
///             features = ["apple-native", "linux-native-sync-persistent"] }
/// secrecy = { version = "0.10", optional = true }
/// zeroize = { version = "1", optional = true }
///
/// [features]
/// keyring = ["dep:keyring", "dep:secrecy", "dep:zeroize"]
/// ```
///
/// Then replace this stub with a `keyring::Entry`-backed impl plus the
/// Linux-dedicated-collection logic described in the module docs.
#[cfg(feature = "keyring")]
pub struct KeyringKeystore {
    /// Service name used for every entry. Per ADR-016: `"af"`.
    service: &'static str,
}

#[cfg(feature = "keyring")]
impl KeyringKeystore {
    /// Construct a new keyring-backed keystore.
    ///
    /// Until the real `keyring` crate is wired into `Cargo.toml` in Phase IV,
    /// all methods return [`KeystoreError::ServiceUnavailable`] so the crate
    /// compiles cleanly without `dep:keyring`. The trait shape is the final
    /// API and will not change when the real backend lands.
    #[must_use]
    pub fn new() -> Self {
        Self { service: "af" }
    }

    /// Service name used for all entries (read-only accessor; intentionally
    /// exposed so tests and diagnostics can verify the ADR-016 invariant).
    #[must_use]
    pub fn service(&self) -> &'static str {
        self.service
    }
}

#[cfg(feature = "keyring")]
impl Default for KeyringKeystore {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(feature = "keyring")]
impl Keystore for KeyringKeystore {
    fn get(&self, _provider: &str) -> Result<SecretString, KeystoreError> {
        Err(KeystoreError::ServiceUnavailable)
    }

    fn set(&self, _provider: &str, _secret: SecretString) -> Result<(), KeystoreError> {
        Err(KeystoreError::ServiceUnavailable)
    }

    fn delete(&self, _provider: &str) -> Result<(), KeystoreError> {
        Err(KeystoreError::ServiceUnavailable)
    }

    fn list(&self) -> Result<Vec<String>, KeystoreError> {
        Err(KeystoreError::ServiceUnavailable)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_fake_set_then_get_roundtrip() {
        let ks = FakeKeystore::new();
        ks.set("claude", SecretString::new(String::from("sk-abc")))
            .unwrap();
        let got = ks.get("claude").unwrap();
        assert_eq!(got.expose_secret(), "sk-abc");
    }

    #[test]
    fn test_fake_get_missing_is_not_found() {
        let ks = FakeKeystore::new();
        let err = ks.get("gemini").unwrap_err();
        assert!(matches!(err, KeystoreError::NotFound(p) if p == "gemini"));
    }

    #[test]
    fn test_fake_set_overwrites_existing() {
        let ks = FakeKeystore::new();
        ks.set("claude", SecretString::new(String::from("v1")))
            .unwrap();
        ks.set("claude", SecretString::new(String::from("v2")))
            .unwrap();
        assert_eq!(ks.get("claude").unwrap().expose_secret(), "v2");
    }

    #[test]
    fn test_fake_delete_removes_entry() {
        let ks = FakeKeystore::new();
        ks.set("claude", SecretString::new(String::from("sk")))
            .unwrap();
        ks.delete("claude").unwrap();
        assert!(matches!(ks.get("claude"), Err(KeystoreError::NotFound(_))));
    }

    #[test]
    fn test_fake_delete_missing_is_not_found() {
        let ks = FakeKeystore::new();
        let err = ks.delete("copilot").unwrap_err();
        assert!(matches!(err, KeystoreError::NotFound(p) if p == "copilot"));
    }

    #[test]
    fn test_fake_list_returns_sorted_names() {
        let ks = FakeKeystore::new();
        ks.set("gemini", SecretString::new(String::from("a")))
            .unwrap();
        ks.set("claude", SecretString::new(String::from("b")))
            .unwrap();
        ks.set("amp", SecretString::new(String::from("c"))).unwrap();

        let names = ks.list().unwrap();
        assert_eq!(names, vec!["amp", "claude", "gemini"]);
    }

    #[test]
    fn test_fake_list_never_contains_values() {
        // Explicit invariant test: `list()` returns labels only.
        let ks = FakeKeystore::new();
        ks.set("claude", SecretString::new(String::from("sk-SECRET-VALUE")))
            .unwrap();
        let names = ks.list().unwrap();
        for name in &names {
            assert!(!name.contains("sk-SECRET-VALUE"));
            assert!(!name.contains("SECRET"));
        }
    }

    #[test]
    fn test_fake_list_empty_when_no_entries() {
        let ks = FakeKeystore::new();
        assert!(ks.list().unwrap().is_empty());
    }

    #[test]
    fn test_account_naming_is_bare_provider() {
        // ADR-016 amendment + ADR-025 [C 2.2]: account is the provider name,
        // NOT prefixed with "af/". The FakeKeystore uses the provider string
        // directly as the key, which enforces this at the type level.
        let ks = FakeKeystore::new();
        ks.set("claude", SecretString::new(String::from("v")))
            .unwrap();
        let names = ks.list().unwrap();
        assert_eq!(names, vec!["claude"]);
        assert!(!names.iter().any(|n| n.starts_with("af/")));
    }

    #[cfg(feature = "keyring")]
    #[test]
    fn test_keyring_stub_service_is_af() {
        let ks = KeyringKeystore::new();
        assert_eq!(ks.service(), "af");
    }

    #[cfg(feature = "keyring")]
    #[test]
    fn test_keyring_stub_returns_service_unavailable() {
        // Until Phase IV wires `dep:keyring`, the stub returns
        // `ServiceUnavailable` so the crate compiles and the boundary is
        // testable without the OS daemon.
        let ks = KeyringKeystore::new();
        assert!(matches!(
            ks.get("claude"),
            Err(KeystoreError::ServiceUnavailable)
        ));
        assert!(matches!(
            ks.set("claude", SecretString::new(String::from("v"))),
            Err(KeystoreError::ServiceUnavailable)
        ));
        assert!(matches!(
            ks.delete("claude"),
            Err(KeystoreError::ServiceUnavailable)
        ));
        assert!(matches!(ks.list(), Err(KeystoreError::ServiceUnavailable)));
    }
}
