//! GitHub PR helpers via the `gh` CLI.
//!
//! Provides functions to query PR information using `gh pr view` and
//! `gh pr list`. All operations require the `gh` CLI to be installed
//! and authenticated.

use std::path::Path;
use tracing::debug;

/// Information about a GitHub pull request.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct PrBranchInfo {
    /// The PR's head (source) branch name.
    pub head_branch: String,
    /// The PR's base (target) branch name.
    pub base_branch: String,
}

/// Information about a PR's current state.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct PrStateInfo {
    /// PR number.
    pub number: u64,
    /// PR URL.
    pub url: String,
    /// PR state: "OPEN", "CLOSED", or "MERGED".
    pub state: String,
}

/// Check if the `gh` CLI is available on PATH.
pub fn gh_available() -> bool {
    which::which("gh").is_ok()
}

/// Resolve the head and base branch for a PR number.
///
/// Shells out to `gh pr view <number> --json headRefName,baseRefName`.
pub fn resolve_pr_branches(pr_number: u64, git_root: &Path) -> anyhow::Result<PrBranchInfo> {
    if !gh_available() {
        anyhow::bail!("'gh' CLI is required for --from-pr. Install it: https://cli.github.com/");
    }

    debug!(pr_number, "resolving PR branches via gh pr view");

    let output = std::process::Command::new("gh")
        .args([
            "pr",
            "view",
            &pr_number.to_string(),
            "--json",
            "headRefName,baseRefName",
        ])
        .current_dir(git_root)
        .output()
        .map_err(|e| anyhow::anyhow!("failed to run gh: {e}"))?;

    if !output.status.success() {
        let stderr = String::from_utf8_lossy(&output.stderr);
        anyhow::bail!("gh pr view #{pr_number} failed: {}", stderr.trim());
    }

    let json: serde_json::Value = serde_json::from_slice(&output.stdout)
        .map_err(|e| anyhow::anyhow!("failed to parse gh output: {e}"))?;

    debug!(?json, "gh pr view response");

    let head = json["headRefName"]
        .as_str()
        .ok_or_else(|| anyhow::anyhow!("gh pr view: missing headRefName"))?
        .to_owned();

    let base = json["baseRefName"]
        .as_str()
        .ok_or_else(|| anyhow::anyhow!("gh pr view: missing baseRefName"))?
        .to_owned();

    Ok(PrBranchInfo {
        head_branch: head,
        base_branch: base,
    })
}

/// Look up PR info for a branch in the current repo.
///
/// Shells out to `gh pr list --head <branch> --json number,url,state --limit 1`.
/// Returns `None` if no PR exists for the branch.
pub fn find_pr_for_branch(branch: &str, git_root: &Path) -> anyhow::Result<Option<PrStateInfo>> {
    if !gh_available() {
        debug!("gh not available, skipping PR lookup");
        return Ok(None);
    }

    debug!(branch, "looking up PR for branch via gh pr list");

    let output = std::process::Command::new("gh")
        .args([
            "pr",
            "list",
            "--head",
            branch,
            "--json",
            "number,url,state",
            "--limit",
            "1",
            "--state",
            "all",
        ])
        .current_dir(git_root)
        .output()
        .map_err(|e| anyhow::anyhow!("failed to run gh: {e}"))?;

    if !output.status.success() {
        // gh pr list fails if not in a GitHub repo — not an error for us.
        return Ok(None);
    }

    let json: serde_json::Value = serde_json::from_slice(&output.stdout)
        .map_err(|e| anyhow::anyhow!("failed to parse gh output: {e}"))?;

    let arr = json
        .as_array()
        .ok_or_else(|| anyhow::anyhow!("expected array from gh pr list"))?;

    let Some(pr) = arr.first() else {
        return Ok(None);
    };

    let number = pr["number"].as_u64().unwrap_or(0);
    let url = pr["url"].as_str().unwrap_or("").to_owned();
    let state = pr["state"].as_str().unwrap_or("").to_owned();

    Ok(Some(PrStateInfo { number, url, state }))
}

/// Parse `gh pr view` JSON output into `PrBranchInfo`.
///
/// Exposed for testing without calling `gh`.
pub fn parse_pr_view_json(json_str: &str) -> anyhow::Result<PrBranchInfo> {
    let json: serde_json::Value =
        serde_json::from_str(json_str).map_err(|e| anyhow::anyhow!("invalid JSON: {e}"))?;

    let head = json["headRefName"]
        .as_str()
        .ok_or_else(|| anyhow::anyhow!("missing headRefName"))?
        .to_owned();

    let base = json["baseRefName"]
        .as_str()
        .ok_or_else(|| anyhow::anyhow!("missing baseRefName"))?
        .to_owned();

    Ok(PrBranchInfo {
        head_branch: head,
        base_branch: base,
    })
}

/// Parse `gh pr list` JSON output into a `PrStateInfo`.
///
/// Exposed for testing without calling `gh`.
pub fn parse_pr_list_json(json_str: &str) -> anyhow::Result<Option<PrStateInfo>> {
    let json: serde_json::Value =
        serde_json::from_str(json_str).map_err(|e| anyhow::anyhow!("invalid JSON: {e}"))?;

    let arr = json
        .as_array()
        .ok_or_else(|| anyhow::anyhow!("expected array"))?;

    let Some(pr) = arr.first() else {
        return Ok(None);
    };

    let number = pr["number"].as_u64().unwrap_or(0);
    let url = pr["url"].as_str().unwrap_or("").to_owned();
    let state = pr["state"].as_str().unwrap_or("").to_owned();

    Ok(Some(PrStateInfo { number, url, state }))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_pr_view_json_valid() {
        let json = r#"{"headRefName":"fix/bug-123","baseRefName":"main"}"#;
        let info = parse_pr_view_json(json).unwrap();
        assert_eq!(info.head_branch, "fix/bug-123");
        assert_eq!(info.base_branch, "main");
    }

    #[test]
    fn test_parse_pr_view_json_missing_head() {
        let json = r#"{"baseRefName":"main"}"#;
        let result = parse_pr_view_json(json);
        assert!(result.is_err());
        assert!(result.unwrap_err().to_string().contains("headRefName"));
    }

    #[test]
    fn test_parse_pr_view_json_missing_base() {
        let json = r#"{"headRefName":"fix/bug"}"#;
        let result = parse_pr_view_json(json);
        assert!(result.is_err());
        assert!(result.unwrap_err().to_string().contains("baseRefName"));
    }

    #[test]
    fn test_parse_pr_view_json_invalid() {
        let result = parse_pr_view_json("not json");
        assert!(result.is_err());
    }

    #[test]
    fn test_parse_pr_list_json_with_pr() {
        let json = r#"[{"number":42,"url":"https://github.com/org/repo/pull/42","state":"OPEN"}]"#;
        let info = parse_pr_list_json(json).unwrap().unwrap();
        assert_eq!(info.number, 42);
        assert_eq!(info.url, "https://github.com/org/repo/pull/42");
        assert_eq!(info.state, "OPEN");
    }

    #[test]
    fn test_parse_pr_list_json_empty_array() {
        let json = "[]";
        let result = parse_pr_list_json(json).unwrap();
        assert!(result.is_none());
    }

    #[test]
    fn test_parse_pr_list_json_merged() {
        let json =
            r#"[{"number":99,"url":"https://github.com/org/repo/pull/99","state":"MERGED"}]"#;
        let info = parse_pr_list_json(json).unwrap().unwrap();
        assert_eq!(info.state, "MERGED");
    }

    #[test]
    fn test_parse_pr_list_json_invalid() {
        let result = parse_pr_list_json("not json");
        assert!(result.is_err());
    }

    #[test]
    fn test_pr_branch_info_debug_clone() {
        let info = PrBranchInfo {
            head_branch: "feat/x".to_owned(),
            base_branch: "main".to_owned(),
        };
        let cloned = info.clone();
        assert_eq!(info, cloned);
        let _debug = format!("{info:?}");
    }

    #[test]
    fn test_pr_state_info_debug_clone() {
        let info = PrStateInfo {
            number: 1,
            url: "https://example.com".to_owned(),
            state: "OPEN".to_owned(),
        };
        let cloned = info.clone();
        assert_eq!(info, cloned);
        let _debug = format!("{info:?}");
    }
}
