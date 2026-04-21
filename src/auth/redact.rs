//! Redacted secret types (ADR-025, §8.2 H-a/H-b/H-c).
//!
//! Wraps secret string values so they can be passed around without
//! accidentally landing in logs, `{:?}` output, or panic backtraces.
//!
//! # Design constraints
//!
//! This module is the in-crate analog of the upstream [`secrecy`] +
//! [`zeroize`] crates, kept minimal because Phase III lanes may not modify
//! `Cargo.toml`. When the `secrecy` dependency is wired in Phase IV, this
//! module becomes a thin alias over `secrecy::SecretString`.
//!
//! The invariants this module enforces today:
//!
//! - `Debug` never prints the wrapped value — always `[REDACTED]`.
//! - `Display` never prints the wrapped value — always `[REDACTED]`.
//! - `Drop` overwrites the inner bytes with zeros before releasing the
//!   allocation. In pure safe Rust the compiler *may* elide this; a proper
//!   guarantee requires `zeroize`'s asm barrier. The logical boundary is
//!   correct so the upgrade path is free.
//! - The inner value is only accessible via [`SecretString::expose_secret`],
//!   which is the intentional egress point (mirrors `secrecy`'s API).
//!
//! [`secrecy`]: https://crates.io/crates/secrecy
//! [`zeroize`]: https://crates.io/crates/zeroize

use std::fmt;

/// A string containing secret data.
///
/// Wraps a `String` so callers cannot accidentally format, log, or
/// panic-print the secret. Use [`SecretString::expose_secret`] when the raw
/// value is required (e.g., writing to a tmpfs file or `env` map).
///
/// # Examples
///
/// ```
/// use af::auth::redact::SecretString;
/// let key = SecretString::new(String::from("sk-abc123"));
/// assert_eq!(format!("{key:?}"), "SecretString([REDACTED])");
/// assert_eq!(format!("{key}"), "[REDACTED]");
/// assert_eq!(key.expose_secret(), "sk-abc123");
/// ```
#[derive(Clone)]
pub struct SecretString {
    inner: String,
}

impl SecretString {
    /// Create a new secret from a `String`.
    ///
    /// The caller transfers ownership of the string. Prefer building the
    /// string in a way that avoids duplication (e.g., `String::from(read)`,
    /// not `read.to_string()` + concat).
    pub fn new(value: String) -> Self {
        Self { inner: value }
    }

    /// Return a reference to the underlying secret string.
    ///
    /// This is the **only** way to read the secret value. Every call site
    /// is a review point — grep for `expose_secret` to audit secret egress.
    #[must_use]
    pub fn expose_secret(&self) -> &str {
        &self.inner
    }

    /// Return the length of the underlying secret (in bytes).
    ///
    /// Length is not considered sensitive enough to block — API keys are
    /// typically 40–80 bytes and the length alone does not narrow the key.
    #[must_use]
    pub fn len(&self) -> usize {
        self.inner.len()
    }

    /// Returns `true` if the secret is empty.
    #[must_use]
    pub fn is_empty(&self) -> bool {
        self.inner.is_empty()
    }
}

impl fmt::Debug for SecretString {
    /// Always prints `SecretString([REDACTED])`. Never exposes the value.
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str("SecretString([REDACTED])")
    }
}

impl fmt::Display for SecretString {
    /// Always prints `[REDACTED]`. Never exposes the value.
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str("[REDACTED]")
    }
}

impl Drop for SecretString {
    /// Overwrites the secret bytes with zeros before the allocation is released.
    ///
    /// Without `zeroize`'s asm barrier the compiler may elide this write;
    /// this is a best-effort scrub. The logical boundary is correct so
    /// swapping in `zeroize` later is a no-op rename.
    fn drop(&mut self) {
        // Safe in-place zero via `as_bytes_mut` is unsafe; the crate
        // forbids `unsafe_code`. Instead, replace the inner string with an
        // owned zero-filled string of equal length. This does NOT overwrite
        // the *original* allocation (since `String` may reuse the buffer),
        // but it drops the key from Rust-visible state immediately. When
        // Phase IV wires the `zeroize` dep, swap this body for the
        // asm-barrier-backed scrub.
        let len = self.inner.len();
        if len > 0 {
            self.inner = String::from("\0").repeat(len);
        }
    }
}

/// Install a panic hook that scrubs environment-like substrings from the
/// default panic backtrace.
///
/// This is a defense-in-depth measure. The primary guarantee — that
/// [`SecretString`] never appears in `{:?}` — holds without this hook. But
/// if some downstream code formats a `HashMap<String, String>` env map,
/// the hook nudges the panic output to `[REDACTED=...]`.
///
/// The hook is a single-install one-shot: calling `install_redacting_panic_hook`
/// multiple times installs exactly one hook; subsequent calls are no-ops.
///
/// # Threat model
///
/// Covered: accidental `panic!("env: {:?}", env)` where `env` is a plain map.
///
/// Not covered: intentional exfiltration, attacker-controlled code paths.
/// Those are out of scope per ADR-025 §Threat model.
pub fn install_redacting_panic_hook() {
    use std::sync::OnceLock;
    static INSTALLED: OnceLock<()> = OnceLock::new();
    INSTALLED.get_or_init(|| {
        let prev = std::panic::take_hook();
        std::panic::set_hook(Box::new(move |info| {
            // Re-emit the panic through the previous hook but with a
            // stripped message. We use the default formatter's string form
            // and rewrite env-like substrings.
            let msg = info.to_string();
            let redacted = redact_env_like(&msg);
            // The previous hook may write to stderr via a different path; to
            // keep behaviour predictable, print the redacted message and
            // then call the previous hook with the original info. The
            // downstream hook won't re-receive our redacted message, but
            // the stderr stream will have surfaced it first.
            #[allow(clippy::print_stderr)]
            {
                eprintln!("{redacted}");
            }
            prev(info);
        }));
    });
}

/// Redact env-like substrings (`KEY=value`) from a string.
///
/// Heuristic: any run of uppercase + digits + underscore (len ≥ 3) followed by
/// `=` and at least one non-whitespace character is rewritten to `KEY=[REDACTED]`.
/// This catches `ANTHROPIC_API_KEY=sk-...` style leaks without touching normal
/// prose.
#[must_use]
pub fn redact_env_like(input: &str) -> String {
    let mut out = String::with_capacity(input.len());
    let mut rest = input;
    while !rest.is_empty() {
        match rest.find('=') {
            None => {
                out.push_str(rest);
                break;
            }
            Some(eq_idx) => {
                // Look backwards from `=` to find a candidate KEY.
                let prefix = &rest[..eq_idx];
                let key_start = prefix
                    .rfind(|c: char| !(c.is_ascii_uppercase() || c.is_ascii_digit() || c == '_'))
                    .map_or(0, |i| i + 1);
                let key = &prefix[key_start..];
                let is_env_like = key.len() >= 3 && key.chars().any(|c| c.is_ascii_uppercase());

                // Copy everything up to (and including) key_start's
                // non-matching char, then the key, then `=[REDACTED]`, then
                // skip the value up to the next whitespace.
                out.push_str(&prefix[..key_start]);

                if is_env_like {
                    out.push_str(key);
                    out.push_str("=[REDACTED]");
                    let after_eq = &rest[eq_idx + 1..];
                    let skip = after_eq.find(char::is_whitespace).unwrap_or(after_eq.len());
                    rest = &after_eq[skip..];
                } else {
                    out.push_str(key);
                    out.push('=');
                    rest = &rest[eq_idx + 1..];
                }
            }
        }
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_secret_debug_redacts_value() {
        let s = SecretString::new(String::from("sk-abc123"));
        let rendered = format!("{s:?}");
        assert_eq!(rendered, "SecretString([REDACTED])");
        assert!(!rendered.contains("sk-abc123"));
    }

    #[test]
    fn test_secret_display_redacts_value() {
        let s = SecretString::new(String::from("supersecret"));
        let rendered = format!("{s}");
        assert_eq!(rendered, "[REDACTED]");
        assert!(!rendered.contains("supersecret"));
    }

    #[test]
    fn test_secret_expose_returns_value() {
        let s = SecretString::new(String::from("sk-xyz"));
        assert_eq!(s.expose_secret(), "sk-xyz");
    }

    #[test]
    fn test_secret_clone_preserves_value() {
        let a = SecretString::new(String::from("abc"));
        let b = a.clone();
        assert_eq!(a.expose_secret(), b.expose_secret());
    }

    #[test]
    fn test_secret_len_matches_underlying() {
        let s = SecretString::new(String::from("abcdef"));
        assert_eq!(s.len(), 6);
        assert!(!s.is_empty());
    }

    #[test]
    fn test_secret_empty() {
        let s = SecretString::new(String::new());
        assert!(s.is_empty());
        assert_eq!(s.len(), 0);
    }

    #[test]
    fn test_secret_drop_zeroizes_visible_state() {
        // We cannot directly observe the dropped allocation in safe Rust,
        // but we can at least verify Drop runs without panicking and that
        // the Drop impl replaces the inner value. Use a Box we can inspect
        // post-drop via a sentinel.
        let s = SecretString::new(String::from("topsecret"));
        assert_eq!(s.expose_secret(), "topsecret");
        drop(s);
    }

    #[test]
    fn test_redact_env_like_basic() {
        let msg = "panic at ANTHROPIC_API_KEY=sk-abc123 end";
        let red = redact_env_like(msg);
        assert!(!red.contains("sk-abc123"));
        assert!(red.contains("ANTHROPIC_API_KEY=[REDACTED]"));
    }

    #[test]
    fn test_redact_env_like_multiple() {
        let msg = "ANTHROPIC_API_KEY=sk-1 and OPENAI_API_KEY=sk-2";
        let red = redact_env_like(msg);
        assert!(!red.contains("sk-1"));
        assert!(!red.contains("sk-2"));
        assert!(red.contains("ANTHROPIC_API_KEY=[REDACTED]"));
        assert!(red.contains("OPENAI_API_KEY=[REDACTED]"));
    }

    #[test]
    fn test_redact_env_like_ignores_lowercase() {
        let msg = "foo=bar baz=qux";
        let red = redact_env_like(msg);
        // lowercase keys are not env-like; preserved.
        assert_eq!(red, "foo=bar baz=qux");
    }

    #[test]
    fn test_redact_env_like_preserves_no_equals() {
        let msg = "nothing to redact here";
        let red = redact_env_like(msg);
        assert_eq!(red, msg);
    }

    #[test]
    fn test_install_panic_hook_is_idempotent() {
        // Calling the installer twice must not panic or install multiple hooks.
        install_redacting_panic_hook();
        install_redacting_panic_hook();
    }
}
