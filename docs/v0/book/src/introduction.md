# af — User Guide

**af** (agentic-flow · automatic-flow · as-fuck) gives you isolated development sessions
for AI coding agents. One command creates a git worktree, a multiplexer session, and
launches your agent. When you are done, one command tears everything down.

## What it does

You are working on a repo. You want Claude (or pi, or Codex, or Gemini, or Amp) to focus
on one task without touching your main checkout. You want the branch, the worktree, and
the agent session tied together. When the PR merges, you want everything cleaned up.

`af` does that.

## The 30-second version

```bash
af create fix-auth-bug      # worktree + tmux session + Claude
af agent add --slot review  # second agent in a new pane
af list                     # see all active workstreams
af done                     # tear it all down
```

## Where to go next

- New to af? Start with [Installation](install.md) then [Quickstart](quickstart.md).
- Understand the model first? Read [Three-Layer Architecture](concepts/providers.md).
- Looking up a flag? Jump to [Command Reference](commands/index.md).
