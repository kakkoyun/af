//! Session name sanitization and branch prefix logic.
//!
//! tmux session names cannot contain `/`, `.`, or `:`. These characters are
//! replaced with `--`. Branch names may be prefixed with a username when
//! working on fork repositories.

/// Sanitize a branch name into a valid tmux session name.
///
/// Replaces `/`, `.`, and `:` with `--` so the name is safe for use as a
/// tmux (or zellij) session identifier.
///
/// # Examples
///
/// ```
/// use af::session::naming::sanitize_session_name;
///
/// assert_eq!(sanitize_session_name("kakkoyun/issue-42"), "kakkoyun--issue-42");
/// assert_eq!(sanitize_session_name("v1.2.3:hotfix"), "v1--2--3--hotfix");
/// ```
pub fn sanitize_session_name(name: &str) -> String {
    let mut result = String::with_capacity(name.len());
    for c in name.chars() {
        if c == '/' || c == '.' || c == ':' {
            result.push_str("--");
        } else {
            result.push(c);
        }
    }
    result
}

/// Determine if a branch name should be prefixed.
///
/// Returns `true` when the name doesn't already start with `prefix/`.
/// Returns `false` when the prefix is empty (no fork detected) or the name
/// already begins with the prefix.
///
/// # Examples
///
/// ```
/// use af::session::naming::should_prefix;
///
/// assert!(should_prefix("my-task", "kakkoyun"));
/// assert!(!should_prefix("kakkoyun/my-task", "kakkoyun"));
/// assert!(!should_prefix("my-task", ""));
/// ```
pub fn should_prefix(name: &str, prefix: &str) -> bool {
    if prefix.is_empty() {
        return false;
    }
    let with_slash = format!("{prefix}/");
    !name.starts_with(&with_slash)
}

/// Apply the branch prefix if needed.
///
/// Returns the name unchanged if it already starts with `prefix/` or if
/// the prefix is empty.
///
/// # Examples
///
/// ```
/// use af::session::naming::apply_prefix;
///
/// assert_eq!(apply_prefix("my-task", "kakkoyun"), "kakkoyun/my-task");
/// assert_eq!(apply_prefix("kakkoyun/my-task", "kakkoyun"), "kakkoyun/my-task");
/// assert_eq!(apply_prefix("my-task", ""), "my-task");
/// ```
pub fn apply_prefix(name: &str, prefix: &str) -> String {
    if prefix.is_empty() || !should_prefix(name, prefix) {
        return name.to_owned();
    }
    format!("{prefix}/{name}")
}

#[cfg(test)]
mod tests {
    use super::*;

    // ── sanitize_session_name ─────────────────────────────────────────────

    #[test]
    fn test_sanitize_replaces_slash_with_double_dash() {
        assert_eq!(
            sanitize_session_name("kakkoyun/issue-42"),
            "kakkoyun--issue-42"
        );
    }

    #[test]
    fn test_sanitize_replaces_dot_with_double_dash() {
        assert_eq!(sanitize_session_name("v1.2.3"), "v1--2--3");
    }

    #[test]
    fn test_sanitize_replaces_colon_with_double_dash() {
        assert_eq!(sanitize_session_name("v1:hotfix"), "v1--hotfix");
    }

    #[test]
    fn test_sanitize_multiple_special_chars() {
        assert_eq!(sanitize_session_name("v1.2.3:hotfix"), "v1--2--3--hotfix");
    }

    #[test]
    fn test_sanitize_no_special_chars_unchanged() {
        assert_eq!(sanitize_session_name("my-task"), "my-task");
    }

    #[test]
    fn test_sanitize_empty_string() {
        assert_eq!(sanitize_session_name(""), "");
    }

    #[test]
    fn test_sanitize_only_special_chars() {
        assert_eq!(sanitize_session_name("/.:"), "------");
    }

    // ── should_prefix ─────────────────────────────────────────────────────

    #[test]
    fn test_should_prefix_plain_name_returns_true() {
        assert!(should_prefix("my-task", "kakkoyun"));
    }

    #[test]
    fn test_should_prefix_already_prefixed_returns_false() {
        assert!(!should_prefix("kakkoyun/my-task", "kakkoyun"));
    }

    #[test]
    fn test_should_prefix_empty_prefix_returns_false() {
        assert!(!should_prefix("my-task", ""));
    }

    // ── apply_prefix ──────────────────────────────────────────────────────

    #[test]
    fn test_apply_prefix_adds_prefix() {
        assert_eq!(apply_prefix("my-task", "kakkoyun"), "kakkoyun/my-task");
    }

    #[test]
    fn test_apply_prefix_skips_if_already_prefixed() {
        assert_eq!(
            apply_prefix("kakkoyun/my-task", "kakkoyun"),
            "kakkoyun/my-task"
        );
    }

    #[test]
    fn test_apply_prefix_empty_prefix_returns_name() {
        assert_eq!(apply_prefix("my-task", ""), "my-task");
    }
}
