---
adr: 043
title: "Agent Providers (claude, pi, codex; pi default)"
status: proposed
implementation: in-progress
date: 2026-05-06
last_modified: 2026-05-20
supersedes: []
superseded_by: null
related: ["031", "036", "039", "044", "057"]
tags: ["go", "agent", "pi", "claude", "codex"]
---

# ADR-043: Agent Providers (claude, pi, codex; pi default)

## Context

v0 supported six AI agent CLIs: `claude`, `pi`, `codex`, `gemini`,
`amp`, `copilot`. The owner uses three of them in practice; the others
were added for completeness and never exercised. Per scope cut
(ADR-031), v1 keeps only the three.

Pi is the **default**. The owner's daily driver is pi; claude and codex
are added to a workstream on demand via `af agent add --agent <name>`.

## Decision

### Interface

```go
// internal/agent/agent.go

type ApprovalMode int

const (
    ApprovalDefault ApprovalMode = iota // agent's default behaviour
    ApprovalAuto                        // auto-approve safe ops
    ApprovalYolo                        // skip all permission prompts
)

type LaunchOpts struct {
    SessionID    string       // UUID v5 from ADR-039
    ApprovalMode ApprovalMode
    Cwd          string       // worktree path
}

type ResumeOpts struct {
    SessionID    string
    ApprovalMode ApprovalMode
    Cwd          string
}

type Agent interface {
    Name() string                                                // "pi" | "claude" | "codex"
    Binary() string                                              // "pi" | "claude" | "codex"
    IsAvailable(ctx context.Context) bool
    LaunchCmd(opts LaunchOpts) []string                          // argv for new session
    ResumeCmd(opts ResumeOpts) []string                          // argv for resumed session
    PRCmd(prNumber int, opts LaunchOpts) ([]string, bool)        // argv if supported
    BodyCmd(opts BodyOpts) ([]string, bool)                      // argv for non-interactive print mode (ADR-057)
    SessionLogPaths(sessionID, projectPath string) []string      // for analysis only
}

type BodyOpts struct {
    Cwd   string // worktree path
    Model string // optional model override; "" = agent default
}
```

### CLI surfaces

Distilled from each agent's `--help`. **Subject to per-agent verification at implementation time.**

| Agent      | Launch                       | Resume                | Yolo flag                               | Auto flag |
| ---------- | ---------------------------- | --------------------- | --------------------------------------- | --------- |
| **pi**     | `pi`                         | `pi --continue`       | (none — pi has internal approval flows) | (n/a)     |
| **claude** | `claude --session-id <uuid>` | `claude --continue`   | `--dangerously-skip-permissions`        | (n/a)     |
| **codex**  | `codex`                      | `codex resume --last` | `--full-auto`                           | `--auto`  |

### Defaults

- `[general].default_agent = "pi"` (per user directive).
- `KnownAgents = ["pi", "claude", "codex"]`. Order is the
  fallback-priority for `first_available()`.

### `IsAvailable`

Each agent's `IsAvailable` shells out to `which <binary>` (or `exec.LookPath`).
Used by `af doctor` (ADR-044) and `af create`'s implicit "if no
`--agent` flag and the configured default isn't installed, fall back".

### Session log paths

For analysis only — `af` **never deletes or modifies** these files.

| Agent  | Path pattern                                                                  |
| ------ | ----------------------------------------------------------------------------- |
| pi     | `~/.pi/agent/sessions/<encoded-cwd>/<timestamp>_<session-id>.jsonl`           |
| claude | `~/.claude/projects/<encoded-cwd>/<session-id>.jsonl`                         |
| codex  | (TBD per codex's session-log convention; impl researches at integration time) |

`SessionLogPaths` returns `[]string` so callers (debugging, future
analytics) can locate them without the agent's own `--list-sessions`
command.

### Excluded agents

| Agent   | Why excluded from v1            |
| ------- | ------------------------------- |
| gemini  | Not used by the owner currently |
| amp     | Not used by the owner currently |
| copilot | Not used by the owner currently |

If any of these comes back, it's a new ADR + a new file in
`internal/agent/`. The interface was deliberately kept simple to make
that mechanical.

### Subagent dispatch

Per ADR-039: subagents run in additional slots, possibly with different
agent providers. A workstream might have:

- slot `primary` running `pi`
- slot `review` running `claude`
- slot `tests` running `codex`

Each slot uses its agent's `LaunchCmd`, gets its own session ID, and
writes to the agent's own log file pattern.

## Consequences

- Three small files in `internal/agent/` (one per provider) plus the interface and a `Resolve(name) Agent` factory.
- Adding/removing agents is a focused PR.
- The owner's pi-first workflow is honored without per-command flags.

## Alternatives Considered

- **Keep all six agents.** Rejected per scope cut.
- **Drop the interface; one type per command.** Rejected; the multi-agent slot model (ADR-039) requires a polymorphic dispatch.
- **Auto-detect agent from binary in PATH.** Rejected; explicit `--agent <name>` or `default_agent` config is clearer.

## References

- v0 ADR-001 — superseded for v1.
- ADR-031 — v1 master, dropped agents.
- ADR-036 — `[general].default_agent` config.
- ADR-039 — multi-agent slot model.
- ADR-044 — doctor probes agent binaries.
