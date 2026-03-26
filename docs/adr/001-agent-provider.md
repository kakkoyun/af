# ADR-001: Agent Provider Abstraction

**Status:** Accepted
**Date:** 2026-03-26

## Context

`cf` is hardcoded to Claude Code. The Rust rewrite must support multiple AI coding agents:

- **Claude Code** (`claude`) — current default, most mature
- **pi** (`pi`) — already installed, used daily
- **Codex** (`codex`) — OpenAI's agent, available on work machine
- **Gemini CLI** (`gemini`) — Google's agent, available on work machine
- **Amp** (`amp`) — Sourcegraph's agent, available on work machine

Each agent has different:

- Binary names and CLI flags
- Session resumption mechanisms (`--session-id`, `--continue`, etc.)
- Permission bypass flags (`--dangerously-skip-permissions`, `--yolo`, etc.)
- Configuration file locations and formats

The user should be able to choose the agent per-session, per-project, or globally.

## Decision

Introduce an **Agent Provider** trait that encapsulates agent-specific behaviour:

```rust
pub trait AgentProvider {
    /// Display name (e.g., "Claude Code")
    fn name(&self) -> &str;

    /// Binary name to invoke (e.g., "claude")
    fn binary(&self) -> &str;

    /// Check if the agent binary is available on $PATH
    fn is_available(&self) -> bool;

    /// Build the command to launch a new session
    fn launch_cmd(&self, opts: &LaunchOpts) -> Vec<String>;

    /// Build the command to resume/continue a session
    fn resume_cmd(&self, opts: &ResumeOpts) -> Vec<String>;

    /// Build the command for a PR-review session (if supported)
    fn pr_cmd(&self, pr_number: u64, opts: &LaunchOpts) -> Option<Vec<String>>;

    /// Locate the agent's own session log files for a given session ID.
    /// Used for analysis — af never deletes these files. (See ADR-011)
    fn session_log_paths(&self, session_id: &str, project_path: &Path) -> Vec<PathBuf>;
}
```

### Built-in providers (compiled in, feature-gated where sensible)

| Provider | Binary | Session flag | Resume | Yolo flag |
|---|---|---|---|---|
| `claude` | `claude` | `--session-id <uuid>` | `--continue` | `--dangerously-skip-permissions` |
| `pi` | `pi` | *(tbd — research)* | *(tbd)* | *(tbd)* |
| `codex` | `codex` | *(tbd)* | *(tbd)* | `--full-auto` |
| `gemini` | `gemini` | *(tbd)* | *(tbd)* | *(tbd)* |
| `amp` | `amp` | *(tbd)* | *(tbd)* | *(tbd)* |

### Multi-agent model

A workstream can run **multiple agents concurrently**, each in its own multiplexer pane.
Agents are identified by a **slot name** (user-chosen or auto-assigned).

```
af create fix-bug                        # launches primary agent (from config)
af agent add --slot review --agent pi    # adds pi in a new pane
af agent add --slot tests --agent codex  # adds codex in another pane
af agent stop review                     # stops the pi instance
af agent list                            # show all agents in the workstream
```

#### Primary agent selection

1. `af create --agent codex` — explicit per-session flag
2. Project config (`.af/config.toml` → `agent = "pi"`)
3. User config (`~/.config/af/config.toml` → `default_agent = "claude"`)
4. First available from a priority list: `claude > pi > codex > gemini > amp`

#### Slot model

- Each slot maps to one agent provider + one multiplexer pane
- The `primary` slot is created by `af create`
- Additional slots are created by `af agent add`
- Each agent has its own session IDs, tracked independently in `state.toml`
- All agents share the same worktree, branch, and multiplexer session
- `af done` tears down all agents in all slots

## Consequences

- Each agent's CLI surface must be researched and encoded (some may change frequently).
- Agents that don't support session IDs will get a degraded experience (no deterministic resume).
- The trait boundary keeps agent-specific logic contained — new agents are a struct + trait impl.
- `--yolo` semantics vary per agent; we abstract it as "unattended mode" in the trait.
- Multi-agent adds complexity to resume (must restore each pane) and teardown (must stop all).
- The slot model avoids conflicts — each agent has its own pane, its own session state.
- Ledger events carry `slot` to distinguish which agent did what (see [ADR-011](011-workstream-lifecycle.md)).
