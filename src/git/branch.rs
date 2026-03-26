//! Branch detection utilities.
//!
//! Provides logic for detecting the main/default branch name from a list
//! of local branches, following the convention of checking `main`, `master`,
//! and `trunk` in priority order.

/// The well-known main branch names, in priority order.
pub const MAIN_BRANCH_CANDIDATES: &[&str] = &["main", "master", "trunk"];

/// Detect the main branch name from a list of local branch names.
///
/// Checks for `"main"`, `"master"`, `"trunk"` in that priority order.
/// Returns `"main"` as the fallback if none of the candidates match.
///
/// # Examples
///
/// ```
/// use af::git::branch::detect_main_branch;
///
/// assert_eq!(detect_main_branch(&["main", "feature-x"]), "main");
/// assert_eq!(detect_main_branch(&["master", "develop"]), "master");
/// assert_eq!(detect_main_branch(&["release", "develop"]), "main"); // fallback
/// ```
pub fn detect_main_branch(branches: &[&str]) -> &'static str {
    for candidate in MAIN_BRANCH_CANDIDATES {
        if branches.contains(candidate) {
            return candidate;
        }
    }
    // Fallback: assume "main" is the convention.
    "main"
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_detect_main_branch_prefers_main() {
        assert_eq!(
            detect_main_branch(&["main", "feature-x", "develop"]),
            "main"
        );
    }

    #[test]
    fn test_detect_main_branch_falls_back_to_master() {
        assert_eq!(
            detect_main_branch(&["master", "develop", "feature-x"]),
            "master"
        );
    }

    #[test]
    fn test_detect_main_branch_falls_back_to_trunk() {
        assert_eq!(
            detect_main_branch(&["trunk", "develop", "feature-x"]),
            "trunk"
        );
    }

    #[test]
    fn test_detect_main_branch_returns_main_when_none_match() {
        assert_eq!(
            detect_main_branch(&["develop", "feature-x", "release"]),
            "main"
        );
    }

    #[test]
    fn test_detect_main_branch_empty_list_returns_main() {
        assert_eq!(detect_main_branch(&[]), "main");
    }

    #[test]
    fn test_detect_main_branch_main_and_master_prefers_main() {
        assert_eq!(detect_main_branch(&["master", "main", "trunk"]), "main");
    }
}
