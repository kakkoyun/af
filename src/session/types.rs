//! Core session types shared across the session module.
//!
//! These types represent the `state.toml` schema defined in ADR-011.

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};

/// Execution mode for a session.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum ExecutionMode {
    /// Local git worktree, agent runs on host.
    Local,
    /// Non-git directory workspace.
    Workspace,
    /// Agent runs on a remote VM.
    Remote,
    /// Agent runs inside a Firecracker sandbox.
    Sandbox,
    /// Agent runs locally on host worktree (review mode).
    Bare,
}

/// Session lifecycle status.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum SessionStatus {
    /// Workstream is in progress.
    Active,
    /// User detached, session paused.
    Paused,
    /// `af done` completed successfully.
    Completed,
    /// `af done --force` on unmerged work.
    Abandoned,
}

/// Status of an agent slot within a session.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum AgentStatus {
    /// Agent is running.
    Running,
    /// Agent was intentionally stopped.
    Stopped,
    /// Agent process died unexpectedly.
    Crashed,
}

/// An agent slot within a session (one pane in the multiplexer).
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct AgentSlot {
    /// Slot name (e.g., "primary", "review", "tests").
    pub slot: String,
    /// Agent provider name (e.g., "claude", "pi").
    pub provider: String,
    /// Agent session IDs (the agent's own session identifiers).
    pub session_ids: Vec<String>,
    /// Multiplexer pane identifier.
    pub pane: String,
    /// Current status of this agent.
    pub status: AgentStatus,
}

/// PR tracking information.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Default)]
pub struct PrInfo {
    /// PR number (0 = no PR).
    #[serde(default)]
    pub number: u64,
    /// PR URL.
    #[serde(default)]
    pub url: String,
    /// PR state (empty, "open", "merged", "closed").
    #[serde(default)]
    pub state: String,
}

/// Version information captured at session creation.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct VersionInfo {
    /// af version at session creation.
    pub af: String,
    /// Hash of the agent configuration at session creation.
    #[serde(default)]
    pub agent_config_hash: String,
}

/// The complete session state, serialized as `state.toml`.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct SessionState {
    /// Session identity and metadata.
    pub session: SessionMeta,
    /// Git worktree information (absent in workspace mode).
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub worktree: Option<WorktreeInfo>,
    /// Execution environment.
    pub execution: ExecutionInfo,
    /// Active agents (one or more).
    pub agents: Vec<AgentSlot>,
    /// Associated pull request.
    #[serde(default)]
    pub pr: PrInfo,
    /// Version pins.
    pub versions: VersionInfo,
}

/// Session identity fields.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct SessionMeta {
    /// Sanitized session name (tmux-safe).
    pub name: String,
    /// Deterministic UUID v5.
    pub id: String,
    /// When the session was created.
    pub created_at: DateTime<Utc>,
    /// Current lifecycle status.
    pub status: SessionStatus,
}

/// Git worktree information.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct WorktreeInfo {
    /// Absolute path to the worktree directory.
    pub path: String,
    /// Git branch name.
    pub branch: String,
    /// Branch this session forked from.
    pub base_branch: String,
    /// Absolute path to the repository root.
    pub git_root: String,
}

/// Execution environment information.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct ExecutionInfo {
    /// Session execution mode.
    pub mode: ExecutionMode,
    /// Multiplexer in use (e.g., "tmux").
    pub multiplexer: String,
    /// Multiplexer session name.
    pub multiplexer_session: String,
}

#[cfg(test)]
mod tests {
    use super::*;
    use chrono::TimeZone;

    fn sample_session_state() -> SessionState {
        SessionState {
            session: SessionMeta {
                name: String::from("kakkoyun--issue-42"),
                id: String::from("550e8400-e29b-41d4-a716-446655440000"),
                created_at: Utc.with_ymd_and_hms(2026, 3, 26, 14, 30, 0).unwrap(),
                status: SessionStatus::Active,
            },
            worktree: Some(WorktreeInfo {
                path: String::from("/home/user/Workspace/.worktrees/myrepo/kakkoyun/issue-42"),
                branch: String::from("kakkoyun/issue-42"),
                base_branch: String::from("upstream/main"),
                git_root: String::from("/home/user/Work/myrepo"),
            }),
            execution: ExecutionInfo {
                mode: ExecutionMode::Local,
                multiplexer: String::from("tmux"),
                multiplexer_session: String::from("kakkoyun--issue-42"),
            },
            agents: vec![AgentSlot {
                slot: String::from("primary"),
                provider: String::from("claude"),
                session_ids: vec![String::from("550e8400-e29b-41d4-a716-446655440000")],
                pane: String::from("0"),
                status: AgentStatus::Running,
            }],
            pr: PrInfo::default(),
            versions: VersionInfo {
                af: String::from("0.1.0"),
                agent_config_hash: String::from("a1b2c3d4"),
            },
        }
    }

    #[test]
    fn test_session_state_roundtrip_toml() {
        let state = sample_session_state();
        let toml_str = toml::to_string_pretty(&state).unwrap();
        let parsed: SessionState = toml::from_str(&toml_str).unwrap();
        assert_eq!(state, parsed);
    }

    #[test]
    fn test_session_state_without_worktree_roundtrip() {
        let mut state = sample_session_state();
        state.worktree = None;
        state.execution.mode = ExecutionMode::Workspace;

        let toml_str = toml::to_string_pretty(&state).unwrap();
        let parsed: SessionState = toml::from_str(&toml_str).unwrap();
        assert_eq!(state, parsed);
    }

    #[test]
    fn test_session_state_multiple_agents_roundtrip() {
        let mut state = sample_session_state();
        state.agents.push(AgentSlot {
            slot: String::from("review"),
            provider: String::from("pi"),
            session_ids: vec![String::from("7a1b2c3d-0000-0000-0000-000000000000")],
            pane: String::from("1"),
            status: AgentStatus::Running,
        });

        let toml_str = toml::to_string_pretty(&state).unwrap();
        let parsed: SessionState = toml::from_str(&toml_str).unwrap();
        assert_eq!(state, parsed);
        assert_eq!(parsed.agents.len(), 2);
    }

    #[test]
    fn test_execution_mode_serializes_lowercase() {
        let json = serde_json::to_string(&ExecutionMode::Local).unwrap();
        assert_eq!(json, "\"local\"");

        let json = serde_json::to_string(&ExecutionMode::Sandbox).unwrap();
        assert_eq!(json, "\"sandbox\"");
    }

    #[test]
    fn test_session_status_serializes_lowercase() {
        let json = serde_json::to_string(&SessionStatus::Active).unwrap();
        assert_eq!(json, "\"active\"");

        let json = serde_json::to_string(&SessionStatus::Completed).unwrap();
        assert_eq!(json, "\"completed\"");
    }

    #[test]
    fn test_pr_info_default_is_empty() {
        let pr = PrInfo::default();
        assert_eq!(pr.number, 0);
        assert!(pr.url.is_empty());
        assert!(pr.state.is_empty());
    }
}
