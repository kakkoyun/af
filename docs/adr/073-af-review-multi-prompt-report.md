---
adr: 073
title: "af review — Repo-Aware PR Review Report"
status: proposed
implementation: pending
date: 2026-05-21
last_modified: 2026-05-21
supersedes: []
superseded_by: null
related: ["031", "035", "036", "037", "043", "051", "057", "058"]
tags: ["go", "command", "agent", "review", "pr", "ai"]
---

# ADR-073: `af review` — Repo-Aware PR Review Report

## Context

ADR-057 defines `af pr --ai` and ADR-058 defines `af retro --ai`. Both ship a
hard-coded prompt to the configured agent's `BodyCmd` and capture stdout into an
artifact. Together they establish the pattern: af owns the orchestration, the agent
owns the prose.

The owner wants a complementary read-only operation: a draft PR review report
written to disk (never posted) that respects the repo's own review conventions,
skills, and contribution norms — without af carrying a hard-coded list of slash
commands to invoke.

The design challenge is tone governance. If af bakes in a prompt that tells the
agent to use severity tags, emoji, or verdict lines, every run produces
structurally similar output regardless of the repo's norms. The solution: af owns
an **immutable system prompt** that controls tone and scope, and the repo controls
content via its own AGENTS.md, CLAUDE.md, and `.claude/commands/` skills. The
two layers combine but never compete — the af prefix always runs first and cannot
be replaced, only extended.

Closest v1 precedents are ADR-057 (`BodyCmd` mechanism) and ADR-058 (agent
invoked with `BodyOpts.Cwd = ""`). `af review` is the same execution shape with
a different prompt strategy.

## Decision

### 1. Immutable system prompt

Stored as `internal/review/system_prompt.md`, embedded into the binary at build
time:

```go
//go:embed system_prompt.md
var systemPromptBytes []byte

func SystemPrompt() string { return string(systemPromptBytes) }
```

The file is plain markdown, diff-reviewable, and ADR-versionable. Runtime code
never allows config to overwrite it — config may only append (see §2). Updating
the system prompt requires a code change **and** an ADR amendment or a new ADR
superseding this one.

The verbatim content of `internal/review/system_prompt.md` is:

```
You are running inside `af review`. Produce a draft PR review report.
Do not post comments. Do not modify any files. Output is a single
markdown document that I will read before deciding what to publish.

Before writing the review, discover the repo's conventions:
- Read AGENTS.md (if present) at the repo root.
- Read CLAUDE.md (if present) at the repo root.
- Read .agents/ and .claude/ files at the repo root.
- Read .claude/commands/*.md — these are this repo's review skills.
  Prefer using a `/review` skill if one is defined; otherwise apply
  any review-oriented skills you find (e.g. /go-review, /simplify).
- If no review skills exist, fall back to the repo's general
  contribution conventions found in AGENTS.md / CLAUDE.md / README.

Style for the report:
- Write as a thoughtful human reviewer: friendly, constructive, kind.
- Do not use severity tags (CRITICAL, HIGH, MED, LOW, P0, etc.).
- Do not use emoji.
- Do not produce a verdict line (no "approved", "blocked",
  "ship it", etc.).
- Group feedback by area or file when natural; quote line numbers
  where useful.
- It is a draft — be specific, but acknowledge uncertainty when
  appropriate.

You will receive the PR diff and a small context block. Use your
tools (read, search, etc.) to inspect the repo where needed.
```

This text is also reproduced here so the ADR is the contract. If the embedded
file and this ADR diverge, the embedded file in the binary is authoritative.

### 2. Per-repo append

Config and CLI may extend the system prompt with repo-specific review notes. The
af-owned prefix always runs first. Non-empty contributions are appended in order,
separated by blank lines, under the heading `# Repo-specific review notes`:

Resolution order:

1. `[review].system_prompt_append` from `~/.config/af/config.toml` (user level).
2. `[review].system_prompt_append` from `<repo>/.af/config.toml` (repo level;
   repo overrides user per the existing layering in ADR-036).
3. Contents of the file at `[review].system_prompt_append_file` (repo-relative).
   When unset, af looks for `.af/review-system-prompt.md` at the repo root and
   uses it if present; otherwise empty.
4. `--append-prompt <text>` CLI flag (one-shot, highest priority).

### 3. Subcommand surface

```
af review [session]
  --pr <n>                 explicit PR number (overrides auto-detect)
  --agent <name>           override [review].agent
  --model <id>             override [review].model
  --base <ref>             override base ref for the diff
  --out <path>             override report path
                           (default <worktree>/.af/reviews/<UTC-ts>-pr<n>.md)
  --stdout                 print report to stdout instead of writing a file
  --append-prompt <text>   one-shot append to the system prompt
  --skill <name>           override [review].suggested_skills; repeatable
  --print-system-prompt    print the resolved system prompt and exit
```

`--print-system-prompt` is a debugging aid: it prints the full resolved prompt
(af prefix + all appended layers + suggested skills block) without invoking the
agent or touching the PR.

### 4. PR resolution

`--pr <n>` wins. Otherwise af runs:

```
gh pr view --json number,title,headRefName,baseRefName
```

against the current branch. If neither resolves, af returns `errReviewNoPR` with
the remediation hint: _run `gh pr checkout <n>` or pass `--pr <n>`_. v1 requires
`gh` on PATH.

### 5. Diff resolution

```
gh pr diff <n>
```

This matches the PR UI diff. An empty diff is `errReviewEmptyDiff` (hard error;
do not invoke the agent).

### 6. Suggested skills

A configurable list of skill names is folded into the prompt as an advisory hint:

```
# Suggested skills
If any of the following skills are defined in this repo's
.claude/commands/ directory, prefer using them where appropriate:
/review, /go-review, /simplify
```

The agent is free to ignore names that do not map to any file. The list exists
because skill discovery can miss files (case sensitivity, nested directories), and
naming the canonical set up-front improves consistency across runs.

`--skill <name>` (repeatable) replaces the config list for one invocation.
`--skill ""` (empty string) suppresses the hints entirely.

Default config value:
```toml
[review]
suggested_skills = ["/review", "/go-review", "/simplify"]
```

### 7. Agent invocation

Single non-interactive call per `af review`. Uses `agent.BodyCmd` (same path as
`af pr --ai`, ADR-057). The full prompt is delivered on stdin:

```
<immutable af system prompt>

# Repo-specific review notes
<user-config append, if any>

<repo-config append, if any>

<repo-file append, if any>

<CLI append, if any>

# Suggested skills
<rendered list, if any>

# PR
PR #<n> — <title>
Base: <base>
Head: <head>
Worktree: <abs path>

# Diff
<full unified diff from `gh pr diff <n>`>
```

`claude -p` and equivalent `pi --print` / `codex exec` argv treat the entire
stdin payload as one body — there is no true system/user channel split when
invoking these CLIs. "Immutable" means immutable inside af's process: af always
prepends the same prefix; config and CLI can only append after it.

If a real system-prompt channel becomes available (e.g. `claude --system-prompt`
flag or an SDK migration), the af-owned text moves there and user content becomes
the PR block. That migration is non-breaking from af's API surface.

### 8. Report layout

af writes a header; the agent provides the body:

```markdown
# Review draft — PR #<n> <title>

_Generated by af review at <UTC timestamp> — do not post as-is._

Base: <base>  Head: <head>
Agent: <provider> <model>

<agent output verbatim>
```

### 9. Output

Default path: `<worktree>/.af/reviews/<UTC-ts>-pr<n>.md`.

Written atomically: `0o600` file under a `0o750` directory, `.tmp` + rename —
matching `session.WriteState` style from ADR-037. The resolved path is printed to
stdout on success.

`--stdout` skips the file and prints the full report to stdout. When `--stdout`
is set, no path is printed and no file is written.

If a session is active, a ledger entry of kind `review.report.written` is appended
with fields `{pr, path, agent, model}`. No other state mutation occurs.

### 10. Failure modes

| Failure                    | Behaviour                                                                          |
| -------------------------- | ---------------------------------------------------------------------------------- |
| No PR resolvable           | `errReviewNoPR` with remediation hint                                              |
| Empty diff                 | `errReviewEmptyDiff`                                                               |
| `gh` missing               | Clear error pointing at `gh auth login`                                            |
| Agent unavailable          | `errReviewAgentUnavailable`                                                        |
| Agent returns empty body   | `errReviewEmptyBody`                                                               |
| Agent exits non-zero       | Wrap agent error; do not partial-write                                             |

### 11. `[review]` config addition

```toml
[review]
# Agent slot used for review. Defaults to the workstream's primary
# agent, or "claude" when no session is loaded.
agent = ""

# Model override forwarded to BodyCmd. Empty means agent default.
model = ""

# Appended to the af-owned immutable system prompt. The af prefix
# always runs first; this content cannot replace it, only extend it.
# Repo-level overrides user-level (existing config layering).
system_prompt_append = ""

# Optional path (repo-relative) to a markdown file whose contents are
# appended to the system prompt after `system_prompt_append`. When
# unset, af looks for ".af/review-system-prompt.md" at the repo root
# and uses it if present.
system_prompt_append_file = ""

# Skill names hinted to the agent. The agent reads .claude/commands/
# to discover the actual skill definitions. This list is advisory.
suggested_skills = ["/review", "/go-review", "/simplify"]
```

Implementation follows the existing five-touchpoint config pattern (struct, layer,
parser, merge, defaults — see `internal/config/config.go`).

## Consequences

- `af review` is read-only by design. It never posts, never modifies files, and
  never touches remote services beyond `gh pr view` and `gh pr diff`.
- Tone and structural constraints are enforced in code (the embedded prompt), not
  in documentation or convention. Drift requires a code change + ADR amendment.
- The repo's own review skills (`.claude/commands/`) are discovered by the agent
  at runtime rather than being enumerated by af. af's role is context aggregation;
  the agent decides which skills to apply.
- The `--print-system-prompt` flag makes the resolved prompt auditable without
  running the agent, which is useful when debugging unexpected tone drift.
- Agent quality varies. The report header marks the output as a draft. The file is
  written to `.af/reviews/` (gitignored) rather than staged or committed.

## Alternatives Considered

- **Hard-code a list of slash-command steps** (like an early draft explored). Rejected:
  it couples af to specific skill names and ordering, and breaks silently when the
  repo renames or removes skills. The immutable-system-prompt + advisory-skills
  approach lets the agent adapt.
- **Let config fully replace the system prompt.** Rejected: af would have no
  governance over tone. Two repos configured differently could produce structurally
  incompatible output. The append-only constraint keeps the af contract stable.
- **Write the system prompt inline in Go source.** Rejected: a Go string literal
  is harder to diff-review and awkward to read. An embedded markdown file is
  viewable in the repo without building.
- **Invoke the agent inside its tmux pane via send-keys.** Rejected; same reason
  as ADR-057 — brittle, captures user session output, cannot read stdout reliably.
- **Two-pass model: one pass for findings, one pass to humanize.** Deferred; adds
  latency and complexity for uncertain quality gain. A single well-prompted pass
  is sufficient for a draft.
- **Post the report as a GitHub PR review.** Explicitly out of scope. The owner
  wants to read the draft before deciding what to publish. Posting is a separate
  concern that may never be added.

## References

- ADR-031 — v1 master.
- ADR-036 — configuration layering (five-touchpoint pattern).
- ADR-037 — `session.WriteState` atomic write style; ledger events.
- ADR-043 — `Agent.BodyCmd` and `BodyOpts`; provider argv patterns.
- ADR-051 — testscript harness; `gh` fake requirement.
- ADR-057 — `af pr --ai`; originator of `BodyCmd` pattern; `--ai` + `--web`
  incompatibility precedent.
- ADR-058 — `af retro --ai`; `BodyOpts.Cwd = ""` contract.
