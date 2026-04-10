# ADR-014: Three-Layer Composition Model

**Status:** Accepted
**Date:** 2026-04-10

## Context

The original design (ADR-004, ADR-005) described remote providers and sandbox providers
as separate concerns that could compose. In practice, the implementation had `--sandbox`
and `--remote` as mutually exclusive `if/else` branches in `af create`.

Additionally, the terminology was confused: slicer and Docker/sbx are **sandbox providers**
(isolation), not agents. exe.dev and DD Workspaces are **remote providers** (machines),
not sandboxes. The agent (Claude, Codex, etc.) is what runs inside.

## Decision

### Three orthogonal layers

| Layer | Concern | Options | CLI flag |
|---|---|---|---|
| **Agent** | Which AI coding agent | claude, pi, codex, gemini, amp, copilot | `--agent <name>` |
| **Remote** | Where the machine lives | local, exe.dev, DD Workspaces | `--remote [host]` |
| **Sandbox** | Isolation around the agent | none, slicer, docker/sbx | `--sandbox` |

### Composition matrix

```
af create task                              # local + no sandbox + default agent
af create --agent codex task                # local + no sandbox + codex
af create --sandbox task                    # local + slicer sandbox + default agent
af create --remote task                     # exe.dev + no sandbox + default agent
af create --sandbox --remote task           # exe.dev + slicer on remote + default agent
af create --sandbox --remote host task      # specific host + slicer on remote
```

When `--sandbox --remote` are combined:
1. Remote provider creates/connects to the machine
2. Provisioning ensures sandbox provider (slicer/sbx) is installed
3. Sandbox provider creates the isolated environment on the remote
4. Agent launches inside the sandbox

### Implementation

`af create` uses `match (args.sandbox, &args.remote)` with four arms:
- `(true, Some(host))` — remote + sandbox composition
- `(true, None)` — local sandbox
- `(false, Some(host))` — remote, agent runs directly
- `(false, None)` — local, agent runs directly

### Sandbox provider selection

Currently hardcoded to slicer. Future: config field `[general] sandbox_provider = "slicer"`.
The `resolve_sandbox()` factory already supports `"slicer"` and `"docker"`.

## Consequences

- The three concerns are cleanly separated in code and documentation.
- Users can combine any agent with any remote with any sandbox.
- Provisioning is the bridge: it ensures the sandbox tool is available on the remote.
- slicer and Docker/sbx are never called "agents" in user-facing text.
- The composition is explicit in the match statement — no hidden fallthrough.
