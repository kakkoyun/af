---
adr: 057
title: "af pr --ai — Agent-Authored PR Body"
status: proposed
implementation: pending
date: 2026-05-08
last_modified: 2026-05-08
supersedes: []
superseded_by: null
related: ["031", "037", "043", "047", "048"]
tags: ["go", "command", "agent", "pr", "ai"]
---

# ADR-057: `af pr --ai` — Agent-Authored PR Body

## Context

ADR-048 defines `af pr` as a thin wrapper around `gh pr create` with token interpolation.
The body comes from the user's editor or a `.github/PULL_REQUEST_TEMPLATE.md`. The owner
already has an agent attached to the workstream that knows the change intimately — the
diff, the workstream note (per ADR-047), the agent's own session log.

Datadog's `gv submit --ai` proves the value: the agent drafts the PR body from diff +
context, the user edits if needed. For af this is a small extension to ADR-048 and a
new method on the Agent interface (ADR-043).

## Decision

### Flag

```
af pr --ai [--ai-model MODEL] [other ADR-048 flags]
```

When `--ai` is set:

1. af gathers context:
   - Diff: `git diff <base>...HEAD` from the worktree.
   - Workstream note body (per ADR-047) — the markdown after frontmatter.
   - Recent ledger events (last 20, ledger.jsonl tail).
2. af invokes the **primary slot's** agent in non-interactive print mode.
3. Captures stdout as the PR body.
4. Substitutes `{body}` in the configured `[pr].cmd` (per ADR-036).
5. Pipes through the existing `af pr` flow — `gh pr create --body "<body>"`.

### Agent interface extension

ADR-043's `Agent` interface gains one method:

```go
// BodyCmd returns argv for invoking the agent in non-interactive print mode.
// The prompt is provided on stdin; the agent's PR-body draft is read from
// stdout. The bool indicates whether the agent supports this mode.
BodyCmd(opts BodyOpts) ([]string, bool)

type BodyOpts struct {
    Cwd   string // worktree path
    Model string // optional model override; "" = agent default
}
```

Per-agent argv (subject to verification at impl time):

| Agent  | argv pattern                                                  |
| ------ | ------------------------------------------------------------- |
| pi     | `pi --print` (TBD — verify with `pi --help`)                  |
| claude | `claude -p` with `--model {Model}` if non-empty               |
| codex  | `codex exec` (TBD — verify with `codex --help`)               |

If `BodyCmd` returns `false`, `af pr --ai` errors with: `agent <name> does not support
non-interactive body generation; use 'af pr' without --ai or attach a different agent`.

### Prompt template

The prompt fed on stdin is **not configurable in v1** (keeps the contract small):

```
You are drafting a pull request body for the change below.

# Workstream note
{note-body}

# Recent activity
{ledger-tail-pretty-printed}

# Diff
{diff}

Write a PR body in markdown with these sections: Summary, Why, What changed, Test plan.
Be concise. Do not include a title — only the body. Do not wrap in code fences.
```

Future ADRs may externalize this template if the owner finds it limiting.

### Failure modes

| Failure                                  | Behaviour                                                                           |
| ---------------------------------------- | ----------------------------------------------------------------------------------- |
| Agent binary not on PATH                 | Error with `af doctor` hint                                                          |
| Agent prints empty body                  | Error: `agent returned empty body — re-run with 'af pr' (no --ai) and write manually` |
| Agent exits non-zero                     | Print agent stderr, error                                                           |
| Diff is empty (no commits to base)       | Error before invoking agent: `no commits to draft from`                             |
| `--ai` and `--web` together              | Reject: `--web defers body to gh's web flow; --ai is incompatible`                  |

### `[pr]` config addition

```toml
[pr]
ai_model = ""    # default model override for --ai (empty = agent default)
```

`--ai-model` flag overrides this per-invocation.

## Consequences

- `af pr --ai` is one flag away from existing `af pr` — no new command surface.
- The Agent interface gains one method; each provider implements it once.
- Prompt template is hard-coded for v1; externalising later is non-breaking.
- The agent decides body quality; af's role stays "context aggregator + tool runner."

## Alternatives Considered

- **Invoke the agent inside its tmux pane via send-keys.** Rejected; brittle, captures
  the user's session output, can't read stdout reliably.
- **Use the agent's PRCmd (per ADR-043).** Rejected; PRCmd is for "open the PR via the
  agent's own UI"; --ai is "give me a body, I'll open the PR myself."
- **External prompt template file.** Rejected for v1; one less knob, can be added later.
- **Run on the active slot's agent rather than `primary`.** Rejected; `primary` is the
  contract anchor and other slots may be specialized (review, tests).

## References

- ADR-031 — v1 master.
- ADR-037 — ledger tail data source.
- ADR-043 — Agent interface (extended by this ADR).
- ADR-047 — workstream note body input.
- ADR-048 — base `af pr` flow.
