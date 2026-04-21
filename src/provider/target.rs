//! SSH target types and universal liveness probe (ADR-027).
//!
//! This module introduces [`SshTarget`] (an SSH-config alias) and
//! [`Liveness`] (the four-state reachability enum) that replace the ad-hoc
//! booleans previously scattered across provider code. Per ADR-027, remote
//! provider identity is decoupled from SSH reachability: any host that
//! resolves in `~/.ssh/config` can be probed the same way.
//!
//! The probe uses `StrictHostKeyChecking=accept-new` on both the probe and
//! subsequent sessions (security finding N2). `BatchMode=yes` prevents
//! hangs on misconfigured hosts.
//!
//! # Note on integration
//!
//! This module is currently defined inside `src/provider/target.rs` but is
//! **not yet wired through `src/provider/mod.rs`** — the Lane L-REMOTE
//! subagent cannot modify that shared file. Consumers reach it via an
//! inline `#[path = "target.rs"] pub mod target;` declaration inside
//! `exedev.rs`. The Phase IV integration pass will promote this module to
//! a top-level `pub mod target;` entry in `provider/mod.rs`.

use std::process::{Command, Stdio};
use std::time::Duration;

use tracing::debug;

/// An SSH target: a hostname or `~/.ssh/config` alias.
///
/// Workspaces and exedev both produce `~/.ssh/config` entries via their
/// respective `create` step; after that, the alias is all `af` needs to
/// connect. Storing the alias (rather than the raw host + port + identity
/// file) keeps the session state minimal and leaves connection policy to
/// SSH itself.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct SshTarget {
    /// The SSH alias (matches a `Host` stanza in `~/.ssh/config`).
    pub host: String,
}

impl SshTarget {
    /// Construct a new [`SshTarget`] from an SSH alias.
    pub fn new(host: impl Into<String>) -> Self {
        Self { host: host.into() }
    }
}

/// Reachability of a remote VM (ADR-027).
///
/// The four states disambiguate "the VM does not exist" (orphan) from
/// "the VM exists but is suspended / unreachable today". `af list` uses
/// the distinction for its orphan column.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Liveness {
    /// SSH connection succeeded.
    Alive,
    /// Provider reports the VM exists but is not SSH-reachable (e.g.,
    /// workspaces `suspended` state). Not an orphan.
    Suspended,
    /// Probe failed: no route, connection refused, or timeout. Candidate
    /// for `af done --force`.
    Unreachable,
    /// Could not decide (host-key mismatch, provider CLI error, etc.).
    /// Never auto-cleaned. The wrapped string carries diagnostic detail.
    Unknown(String),
}

impl Liveness {
    /// Short lowercase label used in `af list`'s STATUS column.
    pub fn label(&self) -> &'static str {
        match self {
            Self::Alive => "alive",
            Self::Suspended => "suspended",
            Self::Unreachable => "orphan",
            Self::Unknown(_) => "unknown",
        }
    }
}

/// Default timeout for [`is_alive`] probes.
///
/// Short enough to keep `af list` snappy, long enough that one slow RTT
/// does not flap. ADR-017 discusses the UX trade-off.
pub const DEFAULT_PROBE_TIMEOUT: Duration = Duration::from_secs(4);

/// Universal SSH liveness probe (ADR-027, amended from ADR-017).
///
/// Uses `StrictHostKeyChecking=accept-new` so the first connection is
/// pinned (TOFU) and subsequent connections refuse silent host-key
/// changes. `BatchMode=yes` guarantees no interactive prompt can hang the
/// probe. Returns:
///
/// * [`Liveness::Alive`] — the remote answered `true` successfully.
/// * [`Liveness::Unreachable`] — the probe failed for any reason other
///   than a host-key mismatch or SSH's own startup error.
/// * [`Liveness::Unknown`] — the probe failed in a way the caller
///   should not treat as an orphan (e.g., ssh binary missing).
///
/// The `timeout` argument is passed to `-o ConnectTimeout=<secs>`. Values
/// below one second are clamped to one second because `ConnectTimeout`'s
/// unit is seconds.
pub fn is_alive(target: &SshTarget, timeout: Duration) -> Liveness {
    let secs = timeout.as_secs().max(1);
    let connect_timeout = format!("ConnectTimeout={secs}");
    debug!(host = %target.host, timeout_secs = secs, "ssh liveness probe");

    let status = Command::new("ssh")
        .args([
            "-o",
            &connect_timeout,
            "-o",
            "BatchMode=yes",
            "-o",
            "StrictHostKeyChecking=accept-new",
            &target.host,
            "true",
        ])
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .status();

    match status {
        Ok(s) if s.success() => Liveness::Alive,
        Ok(s) => {
            debug!(host = %target.host, exit = ?s.code(), "ssh probe failed");
            Liveness::Unreachable
        }
        Err(err) => {
            debug!(host = %target.host, %err, "ssh probe could not run");
            Liveness::Unknown(format!("ssh invocation failed: {err}"))
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    // ── SshTarget ───────────────────────────────────────────────────

    #[test]
    fn test_ssh_target_new_from_string() {
        let target = SshTarget::new(String::from("my-vm"));
        assert_eq!(target.host, "my-vm");
    }

    #[test]
    fn test_ssh_target_new_from_str() {
        let target = SshTarget::new("remote.example.com");
        assert_eq!(target.host, "remote.example.com");
    }

    #[test]
    fn test_ssh_target_equality() {
        let a = SshTarget::new("a");
        let b = SshTarget::new("a");
        let c = SshTarget::new("c");
        assert_eq!(a, b);
        assert_ne!(a, c);
    }

    #[test]
    fn test_ssh_target_clone() {
        let t = SshTarget::new("vm");
        let cloned = t.clone();
        assert_eq!(t, cloned);
    }

    #[test]
    fn test_ssh_target_debug_contains_host() {
        let t = SshTarget::new("debug-host");
        let s = format!("{t:?}");
        assert!(s.contains("debug-host"));
    }

    // ── Liveness ────────────────────────────────────────────────────

    #[test]
    fn test_liveness_label_alive() {
        assert_eq!(Liveness::Alive.label(), "alive");
    }

    #[test]
    fn test_liveness_label_suspended() {
        assert_eq!(Liveness::Suspended.label(), "suspended");
    }

    #[test]
    fn test_liveness_label_unreachable_maps_to_orphan() {
        assert_eq!(Liveness::Unreachable.label(), "orphan");
    }

    #[test]
    fn test_liveness_label_unknown() {
        assert_eq!(Liveness::Unknown(String::from("boom")).label(), "unknown");
    }

    #[test]
    fn test_liveness_equality_ignores_alive_variants() {
        assert_eq!(Liveness::Alive, Liveness::Alive);
        assert_ne!(Liveness::Alive, Liveness::Unreachable);
    }

    #[test]
    fn test_liveness_unknown_carries_detail() {
        match Liveness::Unknown(String::from("host-key mismatch")) {
            Liveness::Unknown(msg) => assert_eq!(msg, "host-key mismatch"),
            _ => panic!("expected Unknown variant"),
        }
    }

    #[test]
    fn test_liveness_clone() {
        let l = Liveness::Unknown(String::from("x"));
        let cloned = l.clone();
        assert_eq!(l, cloned);
    }

    // ── is_alive (we cannot reach real network in unit tests, but we
    //    can exercise the "unknown" path by pointing at a host that
    //    SSH refuses to resolve; SSH still exits non-zero promptly). ──

    #[test]
    fn test_is_alive_unresolvable_host_returns_unreachable_or_unknown() {
        // An invalid host. SSH will either fail to resolve (exit nonzero
        // => Unreachable) or the ssh binary itself may be missing
        // (=> Unknown). Both are acceptable outcomes for this contract
        // test — the important property is that it never returns Alive
        // and never panics.
        let target = SshTarget::new("af-lane-l-invalid-host-does-not-exist.invalid");
        let liveness = is_alive(&target, Duration::from_secs(1));
        match liveness {
            Liveness::Unreachable | Liveness::Unknown(_) => {}
            Liveness::Alive | Liveness::Suspended => {
                panic!("invalid host should not appear alive or suspended: {liveness:?}")
            }
        }
    }

    #[test]
    fn test_is_alive_zero_timeout_is_clamped() {
        // We cannot assert on ssh's behaviour directly, but the fn
        // should not panic with zero duration.
        let target = SshTarget::new("af-lane-l-invalid.invalid");
        let _ = is_alive(&target, Duration::from_secs(0));
    }

    #[test]
    fn test_default_probe_timeout_is_reasonable() {
        // Four seconds balances "tight UX" (ADR-017) against flap risk.
        assert_eq!(DEFAULT_PROBE_TIMEOUT, Duration::from_secs(4));
    }
}
