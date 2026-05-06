# ADR-002: Terminal Multiplexer Abstraction

**Status:** Accepted
**Date:** 2026-03-26

## Context

`cf` is deeply coupled to tmux — it uses tmux sessions, environment variables, pane commands,
split windows, and session lifecycle management. However:

- **Zellij** is a modern alternative with better defaults and a plugin system.
- The user wants to experiment with zellij in the future without rewriting session logic.
- Both tmux and zellij support named sessions, environment variables, and pane management, but
  with completely different CLIs.

## Decision

Introduce a **Multiplexer** trait that abstracts terminal multiplexer operations:

```rust
pub trait Multiplexer {
    /// Check if the multiplexer binary is available
    fn is_available(&self) -> bool;

    /// Check if we're currently inside a multiplexer session
    fn is_inside_session(&self) -> bool;

    /// Get the current session name (if inside one)
    fn current_session_name(&self) -> Result<Option<String>>;

    /// Create a new detached session
    fn create_session(&self, name: &str, cwd: &Path) -> Result<()>;

    /// Kill/destroy a session
    fn kill_session(&self, name: &str) -> Result<()>;

    /// Check if a session exists
    fn session_exists(&self, name: &str) -> bool;

    /// Attach to or switch to a session
    fn attach_or_switch(&self, name: &str) -> Result<()>;

    /// Send keystrokes to a session (for launching commands)
    fn send_keys(&self, session: &str, keys: &str) -> Result<()>;

    /// Set a session-scoped environment variable
    fn set_env(&self, session: &str, key: &str, value: &str) -> Result<()>;

    /// Get a session-scoped environment variable
    fn get_env(&self, session: &str, key: &str) -> Result<Option<String>>;

    /// Set a session option/tag (e.g., @AF_SESSION marker)
    fn set_option(&self, session: &str, key: &str, value: &str) -> Result<()>;

    /// List all sessions
    fn list_sessions(&self) -> Result<Vec<SessionInfo>>;

    /// Split window horizontally (for editor integration)
    fn split_horizontal(&self, session: &str, cmd: &str, cwd: &Path) -> Result<()>;

    /// Get the current pane's running command
    fn pane_command(&self, session: &str) -> Result<Option<String>>;
}
```

### Phase 1: tmux only

The `TmuxMultiplexer` implements this trait using `tmux` CLI commands — the exact same operations
`cf` performs today. This is the only implementation for the MVP.

### Phase 2+: zellij (optional)

A `ZellijMultiplexer` can be added later. Zellij's session model maps reasonably well:

- Named sessions → `zellij attach --create <name>`
- Environment → zellij env injection at session creation
- Key sending → `zellij action write` / `zellij run`

### Multiplexer selection

1. `af create --mux zellij` — explicit
2. Config: `multiplexer = "tmux"` (user or project)
3. Auto-detect: tmux preferred if inside tmux, else first available

## Consequences

- The trait boundary is an investment for future flexibility.
- tmux's `@option` (used for `@CF_SESSION` marker) has no direct zellij equivalent — the zellij
  impl will need a different tagging mechanism (metadata file or session name prefix).
- `send_keys` is inherently fragile (typing into a terminal). Both tmux and zellij have this
  limitation. We accept it as the mechanism to launch agents.
- Disk-persisted metadata (ADR-006) reduces reliance on multiplexer env vars.
