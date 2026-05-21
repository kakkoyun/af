---
adr: 070
title: "Session Selection & Inference"
status: proposed
implementation: pending
date: 2026-05-21
last_modified: 2026-05-21
supersedes: []
superseded_by: null
related: ["035", "038", "044", "046", "054", "068"]
tags: ["go", "ux", "session", "inference", "fzf"]
---

# ADR-070: Session Selection & Inference

## Context

Most v1 commands accept `[session]` as an optional positional
argument (ADR-035 command tree; e.g. `af resume [session]`,
`af suspend [session]`, `af done [session]`, `af info [session]`,
`af pr [session]`, `af note [session]`). ADR-038 introduces a
per-repo discovery symlink at `<repo>/.af/state.toml` that points at
the canonical session for the current worktree, so commands run
inside a workstream usually need no explicit argument.

But the rules for **what happens when `[session]` is omitted and the
cwd does not resolve a workstream** are scattered and incomplete:

- `af doctor` (ADR-044) probes for `fzf` but no command actually
  uses it.
- `af suspend` and `af resume` (ADR-046) say "default to the
  current workstream" without defining the fallback.
- Read-only commands (`af list`, `af status`) deliberately ignore
  the argument; the rule is implicit.
- No ADR specifies the `AF_SESSION` environment variable, although
  it's a natural fit for tmux-pane-level `setenv`.

This ADR consolidates the resolution order into one contract.

## Decision

### Resolution order

Every command that accepts `[session]` resolves it in this order;
the first source that yields a non-empty name wins:

1. **Positional arg** `[session]` — taken verbatim.
2. **`--session NAME` flag** — overrides positional arg with a
   warning to stderr if both are provided (`--session` wins).
3. **`AF_SESSION` env var** — read once at the start of the
   command. tmux panes started by `af create` set this via tmux
   `setenv` (per ADR-040), so any `af` invocation inside such a
   pane resolves the right workstream for free.
4. **cwd inference** — walk up from `os.Getwd()` to find
   `.af/state.toml`. If present, dereference the symlink and use
   the session it points to. This is ADR-038's existing behaviour.
5. **Interactive picker fallback** — only if **all** of:
   - none of 1–4 resolved a session,
   - stdin **and** stderr are TTYs,
   - `fzf` is on `PATH`,
   - at least one workstream exists (`sessions/` is non-empty),

   then run `fzf` against the workstream list, prompt on stderr.
   Ctrl-C aborts → `EX_INTERRUPTED` (130). Selection becomes the
   resolved session.
6. **Hard error.** Otherwise, exit with `EX_NOINPUT` (66) and:

   ```text
   no session specified and none could be inferred.
   pass [session], set --session NAME, set AF_SESSION, or run inside
   a workstream worktree (cwd contains a .af/state.toml symlink).
   ```

### fzf picker shape

The picker prompts:

```
af> session
```

Columns (tab-separated, fzf rendered):

```
<session>    <status>    <repo>      <branch>        <last_touched>
mytask       active      kakkoyun/af kakkoyun/x      2m
otherwork    suspended   kakkoyun/dd kakkoyun/y      3h
```

- Sorted by `last_touched_at` descending (most recently touched
  first), then alphabetical for ties.
- `--multi` is not used; one selection per invocation.
- The picker is invoked via `exec.CommandContext` and its stdout
  pipe; `af` reads back the first column.
- The picker honours `FZF_DEFAULT_OPTS` from the environment. `af`
  does **not** force its own theme.

If the user has `FZF_DEFAULT_OPTS` set or aliases `fzf`, the
picker behaviour follows that. `af` ships no `fzf` config.

### Read-only commands skip the contract

`af list` and `af status` (without `--session`) **always** list all
sessions. They do not invoke the picker. The `[session]` flag on
`af status` is for the `--filter` use case and is parsed by cobra
without going through this resolution path.

`af session-branch` (ADR-035) is unchanged: it never accepts
`[session]` and derives its identity from the current branch.

### Tmux env propagation

`af create` calls `tmux setenv -t <session> AF_SESSION <session>`
(per ADR-040). Sub-commands run inside that tmux session
(including `af` invocations from an agent's pane) inherit
`AF_SESSION` automatically.

This means an agent running inside `af`'s tmux pane can call
`af note --append "fixed bug"` with no `--session` flag and the
right workstream is resolved.

### Non-TTY discipline

The picker fires **only** when the user is plausibly watching.
Non-TTY invocations (`af foo < /dev/null`, `af foo | jq`, CI runs)
fall through to step 6 and error out deterministically. This
preserves the script-friendliness of ADR-068's exit-code contract.

## Consequences

- One contract instead of per-command rules; users stop wondering
  "does *this* command default to current?"
- Tmux panes "just work": agents in af-launched panes resolve the
  right session without flags.
- The picker fallback rescues humans who type `af resume<enter>`
  outside a worktree.
- CI and scripted invocations stay deterministic; no surprise
  blocking on an interactive picker.
- `fzf` becomes a recommended-but-optional dependency; `af doctor`
  probes for it (ADR-044) and the hint becomes truthful: "install
  fzf to enable the interactive session picker."

## Alternatives Considered

- **No picker; always error on missing session.** Rejected: makes
  ad-hoc commands (`af note`) friction-heavy when you're not in a
  worktree.
- **Default to the most-recently-touched session.** Rejected:
  invisible state, easy to nuke the wrong workstream with `af done`.
- **Always prompt with a built-in picker (Bubble Tea / promptui).**
  Rejected: vendoring a TUI library for one prompt; `fzf` is
  already in the owner's toolchain and the most ergonomic choice.
- **Picker also on TTY-but-no-fzf path** (built-in `select`-style
  fallback). Rejected for v1: adds non-trivial code for an edge
  case; "install fzf" is a fine hint.
- **`AF_SESSION` above `--session`.** Rejected: explicit flags
  should beat ambient env. Env wins over cwd inference because env
  is more deliberate than directory.
- **Picker on stdout** (so the user can `af resume | grep ...`).
  Rejected: stdout-stderr discipline from ADR-068 §3 explicitly
  reserves stdout for command data.

## References

- ADR-035 — cobra command tree; positional `[session]` and
  persistent `--session` flag definitions.
- ADR-038 — per-repo discovery symlink; cwd inference.
- ADR-040 — tmux `setenv` propagates `AF_SESSION`.
- ADR-044 — `af doctor` probes for `fzf`; this ADR makes the probe
  load-bearing.
- ADR-046 — `af suspend` / `af resume` rely on this resolution
  contract.
- ADR-068 §3 — stdout/stderr/TTY discipline; picker writes to
  stderr.
- ADR-068 §2 — `EX_NOINPUT` (66) and `EX_INTERRUPTED` (130) exit
  codes used here.
- `fzf` documentation: <https://github.com/junegunn/fzf>
