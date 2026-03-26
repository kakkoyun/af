//! Deterministic UUID v5 generation for session IDs.
//!
//! Sessions get a stable UUID derived from `"<repo>/<branch>"` using the DNS namespace.
//! This ensures that resuming a session uses the same agent session ID.

use uuid::Uuid;

/// Generate a deterministic session ID from a repo name and branch name.
///
/// Uses UUID v5 with the DNS namespace: `uuid5(NAMESPACE_DNS, "<repo>/<branch>")`.
/// This matches the Python `uuid.uuid5(uuid.NAMESPACE_DNS, "<repo>/<branch>")` output,
/// ensuring cross-language compatibility with the original shell implementation.
///
/// # Examples
///
/// ```
/// use af::util::uuid::session_id;
///
/// let id = session_id("myrepo", "mybranch");
/// assert_eq!(id.to_string(), "47413589-5b22-5008-b34e-a569b1920d6f");
/// ```
pub fn session_id(repo: &str, branch: &str) -> Uuid {
    let name = format!("{repo}/{branch}");
    Uuid::new_v5(&Uuid::NAMESPACE_DNS, name.as_bytes())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_session_id_deterministic() {
        let id1 = session_id("myrepo", "mybranch");
        let id2 = session_id("myrepo", "mybranch");
        assert_eq!(id1, id2, "same inputs must produce the same UUID");
    }

    #[test]
    fn test_session_id_different_inputs_differ() {
        let id1 = session_id("repo-a", "branch-a");
        let id2 = session_id("repo-b", "branch-b");
        assert_ne!(id1, id2, "different inputs must produce different UUIDs");
    }

    #[test]
    fn test_session_id_matches_python_output() {
        // Python: uuid.uuid5(uuid.NAMESPACE_DNS, "myrepo/mybranch")
        //       = 47413589-5b22-5008-b34e-a569b1920d6f
        let id = session_id("myrepo", "mybranch");
        assert_eq!(
            id.to_string(),
            "47413589-5b22-5008-b34e-a569b1920d6f",
            "must match Python uuid5 output for cross-language compatibility"
        );
    }

    #[test]
    fn test_session_id_empty_repo() {
        // Empty repo string should still produce a valid UUID without panicking.
        let id = session_id("", "main");
        assert_eq!(id.get_version_num(), 5);
        // Verify it's deterministic even with empty repo.
        assert_eq!(id, session_id("", "main"));
    }

    #[test]
    fn test_session_id_unicode_branch() {
        // Unicode branch names should work (e.g., feature/日本語).
        let id = session_id("repo", "feature/日本語");
        assert_eq!(id.get_version_num(), 5);
        assert_eq!(id, session_id("repo", "feature/日本語"));
    }

    #[test]
    fn test_session_id_slash_in_branch() {
        // Branches like `kakkoyun/feature` contain slashes and must work correctly.
        let id = session_id("myrepo", "kakkoyun/feature");
        assert_eq!(id.get_version_num(), 5);
        // Must be deterministic even with slashes in the branch name.
        assert_eq!(id, session_id("myrepo", "kakkoyun/feature"));
        // Different branch with slash produces different ID.
        assert_ne!(id, session_id("myrepo", "kakkoyun/other"));
    }
}
