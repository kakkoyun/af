//! Remote URL parsing and org/owner detection.
//!
//! Handles both SSH (`git@github.com:ORG/repo.git`) and HTTPS
//! (`https://github.com/ORG/repo`) remote URL formats commonly used
//! with GitHub, GitLab, and similar forges.

/// Extract the org/owner from a git remote URL.
///
/// Handles both SSH (`git@github.com:ORG/repo.git`) and HTTPS
/// (`https://github.com/ORG/repo`) formats.
///
/// Returns `None` if the URL cannot be parsed.
///
/// # Examples
///
/// ```
/// use af::git::remote::parse_org;
///
/// assert_eq!(parse_org("git@github.com:DataDog/repo.git"), Some("DataDog".to_owned()));
/// assert_eq!(parse_org("https://github.com/kakkoyun/af"), Some("kakkoyun".to_owned()));
/// assert_eq!(parse_org(""), None);
/// ```
pub fn parse_org(url: &str) -> Option<String> {
    let (org, _repo) = split_owner_repo(url)?;
    Some(org)
}

/// Extract the repository name (without owner) from a git remote URL.
///
/// Strips the `.git` suffix if present. Returns `None` if the URL
/// cannot be parsed.
///
/// # Examples
///
/// ```
/// use af::git::remote::parse_repo_name;
///
/// assert_eq!(parse_repo_name("git@github.com:DataDog/repo.git"), Some("repo".to_owned()));
/// assert_eq!(parse_repo_name("https://github.com/kakkoyun/af"), Some("af".to_owned()));
/// assert_eq!(parse_repo_name(""), None);
/// ```
pub fn parse_repo_name(url: &str) -> Option<String> {
    let (_org, repo) = split_owner_repo(url)?;
    Some(repo)
}

/// Split a remote URL into `(owner, repo)`.
///
/// Handles SSH (`git@host:OWNER/REPO.git`) and HTTPS
/// (`https://host/OWNER/REPO.git`) formats. Strips the `.git` suffix
/// from the repo name if present.
fn split_owner_repo(url: &str) -> Option<(String, String)> {
    let path = if url.starts_with("https://") || url.starts_with("http://") {
        // HTTPS format: https://github.com/ORG/repo.git
        // Strip scheme + host, leaving /ORG/repo.git
        let without_scheme = url.split("://").nth(1)?;
        // Skip the host component
        let slash_pos = without_scheme.find('/')?;
        without_scheme[slash_pos + 1..].to_owned()
    } else if let Some(colon_pos) = url.find(':') {
        // SSH format: git@github.com:ORG/repo.git
        // Verify it looks like an SSH URL (contains `@` before the colon).
        let before_colon = &url[..colon_pos];
        if !before_colon.contains('@') {
            return None;
        }
        url[colon_pos + 1..].to_owned()
    } else {
        return None;
    };

    // `path` is now "ORG/repo.git" or "ORG/repo"
    let path = path.strip_suffix(".git").unwrap_or(&path);

    let mut parts = path.splitn(2, '/');
    let org = parts.next().filter(|s| !s.is_empty())?;
    let repo = parts.next().filter(|s| !s.is_empty())?;

    Some((org.to_owned(), repo.to_owned()))
}

#[cfg(test)]
mod tests {
    use super::*;

    // ── parse_org tests ───────────────────────────────────────────────────

    #[test]
    fn test_parse_org_ssh_url() {
        assert_eq!(
            parse_org("git@github.com:DataDog/repo.git"),
            Some("DataDog".to_owned())
        );
    }

    #[test]
    fn test_parse_org_https_url() {
        assert_eq!(
            parse_org("https://github.com/DataDog/repo.git"),
            Some("DataDog".to_owned())
        );
    }

    #[test]
    fn test_parse_org_ssh_no_git_suffix() {
        assert_eq!(
            parse_org("git@github.com:kakkoyun/dotfiles"),
            Some("kakkoyun".to_owned())
        );
    }

    #[test]
    fn test_parse_org_https_no_git_suffix() {
        assert_eq!(
            parse_org("https://github.com/kakkoyun/dotfiles"),
            Some("kakkoyun".to_owned())
        );
    }

    #[test]
    fn test_parse_org_empty_returns_none() {
        assert_eq!(parse_org(""), None);
    }

    #[test]
    fn test_parse_org_invalid_url_returns_none() {
        assert_eq!(parse_org("not-a-url"), None);
        assert_eq!(parse_org("ftp://example.com/repo"), None);
    }

    // ── parse_repo_name tests ─────────────────────────────────────────────

    #[test]
    fn test_parse_repo_name_ssh() {
        assert_eq!(
            parse_repo_name("git@github.com:DataDog/repo.git"),
            Some("repo".to_owned())
        );
    }

    #[test]
    fn test_parse_repo_name_https() {
        assert_eq!(
            parse_repo_name("https://github.com/kakkoyun/af.git"),
            Some("af".to_owned())
        );
    }

    #[test]
    fn test_parse_repo_name_no_git_suffix() {
        assert_eq!(
            parse_repo_name("git@github.com:org/myrepo"),
            Some("myrepo".to_owned())
        );
    }

    #[test]
    fn test_parse_repo_name_empty_returns_none() {
        assert_eq!(parse_repo_name(""), None);
    }
}
