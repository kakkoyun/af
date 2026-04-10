# ADR-012: Tri-State Approval Mode

**Status:** Accepted
**Date:** 2026-04-10

## Context

AI coding agents have evolved from binary permission models (allow everything or prompt for
everything) to multi-level approval modes. Each agent implements this differently:

| Agent | Default | Auto-approve edits | Approve everything |
|---|---|---|---|
| Claude | `--permission-mode default` | `--permission-mode auto` | `--dangerously-skip-permissions` |
| Codex | `--ask-for-approval on-request` | `--ask-for-approval on-request` | `--full-auto --ask-for-approval never` |
| Gemini | `--approval-mode default` | `--approval-mode auto_edit` | `--yolo` |
| Amp | *(bare)* | *(bare)* | `--dangerously-allow-all` |
| Copilot | *(bare)* | `--allow-all-tools` | `--allow-all --autopilot` |
| Pi | *(bare)* | *(bare)* | *(not supported)* |

The original `LaunchOpts` had `yolo: bool` — a binary switch. This doesn't capture the
middle ground where agents auto-approve edits but still prompt for destructive commands.

## Decision

Replace `yolo: bool` with a tri-state `ApprovalMode` enum:

```rust
pub enum ApprovalMode {
    /// Prompt for approval on tool use (agent default behaviour).
    Default,
    /// Auto-approve edits and safe tools, prompt for destructive operations.
    Auto,
    /// Skip all permission prompts (sandbox/unattended mode).
    Yolo,
}
```

### CLI mapping

- `af create task` → `ApprovalMode::Default`
- `af create --auto task` → `ApprovalMode::Auto`
- `af create --yolo task` → `ApprovalMode::Yolo`

The `--yolo` flag is preserved for backwards compatibility. The new `--auto` flag is added.

### Per-agent flag mapping

Each `AgentProvider::launch_cmd()` maps `ApprovalMode` to the agent's native flags.
Agents that don't support a mode fall through to the closest available option:

- Pi has no approval modes → all three map to bare invocation
- Amp has no "auto" → Auto falls through to Default

## Consequences

- Three distinct permission levels are available to users.
- Each agent's native flags are used correctly (no one-size-fits-all hack).
- Agents that don't support a mode degrade gracefully.
- The `--yolo` flag still works for users who already use it.
- Future agents with richer permission models can extend the mapping.
