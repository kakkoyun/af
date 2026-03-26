//! Dependency tier system for `af` (ADR-009, ADR-010).
//!
//! Defines the dependency importance tiers and check methods used by `af doctor`
//! to verify that required tools are installed on the current system.
//!
//! # Tier system
//!
//! - **Must**: Session cannot start without this dependency. `af` aborts with an error.
//! - **Should**: Degraded experience without this. `af` warns and continues.
//! - **Nice**: Silent fallback if missing. No user-visible indication.
//!
//! # Examples
//!
//! ```
//! use af::platform::deps::{CheckMethod, Dependency, Tier};
//!
//! let git = Dependency {
//!     name: "git".to_owned(),
//!     tier: Tier::Must,
//!     check: CheckMethod::Binary("git".to_owned()),
//! };
//! // On most dev machines, git should be available:
//! // assert!(git.is_satisfied());
//! ```

/// Dependency importance tier.
///
/// Determines how `af` reacts when a dependency is missing during pre-flight
/// checks (`af doctor`).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Tier {
    /// Session cannot start without this. Abort with error.
    Must,
    /// Degraded experience. Warn and continue.
    Should,
    /// Silent fallback if missing.
    Nice,
}

/// How to check if a dependency is satisfied.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum CheckMethod {
    /// Check if a binary exists on `PATH`.
    Binary(String),
}

/// A dependency that `af` needs.
///
/// Each dependency has a canonical name, an importance tier, and a method to
/// check whether it is satisfied on the current system.
#[derive(Debug, Clone)]
pub struct Dependency {
    /// Canonical name (e.g., `"git"`).
    pub name: String,
    /// Importance tier.
    pub tier: Tier,
    /// How to verify it's installed.
    pub check: CheckMethod,
}

impl Dependency {
    /// Check if this dependency is satisfied on the current system.
    ///
    /// For [`CheckMethod::Binary`], this checks whether the named binary
    /// exists on `PATH` using [`which::which`].
    #[must_use]
    pub fn is_satisfied(&self) -> bool {
        match &self.check {
            CheckMethod::Binary(binary) => which::which(binary).is_ok(),
        }
    }
}

impl std::fmt::Display for Tier {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::Must => f.write_str("must"),
            Self::Should => f.write_str("should"),
            Self::Nice => f.write_str("nice"),
        }
    }
}

impl std::fmt::Display for Dependency {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{} ({})", self.name, self.tier)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_dependency_binary_git_is_satisfied() {
        let git = Dependency {
            name: "git".to_owned(),
            tier: Tier::Must,
            check: CheckMethod::Binary("git".to_owned()),
        };
        assert!(
            git.is_satisfied(),
            "git should be available on PATH in any dev environment"
        );
    }

    #[test]
    fn test_dependency_binary_nonexistent_not_satisfied() {
        let nonexistent = Dependency {
            name: "nonexistent".to_owned(),
            tier: Tier::Nice,
            check: CheckMethod::Binary("nonexistent-binary-xyz".to_owned()),
        };
        assert!(
            !nonexistent.is_satisfied(),
            "a nonexistent binary should not be found on PATH"
        );
    }

    #[test]
    fn test_tier_display() {
        assert_eq!(Tier::Must.to_string(), "must");
        assert_eq!(Tier::Should.to_string(), "should");
        assert_eq!(Tier::Nice.to_string(), "nice");
    }

    #[test]
    fn test_dependency_display() {
        let dep = Dependency {
            name: "tmux".to_owned(),
            tier: Tier::Must,
            check: CheckMethod::Binary("tmux".to_owned()),
        };
        assert_eq!(dep.to_string(), "tmux (must)");
    }
}
