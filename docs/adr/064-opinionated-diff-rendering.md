---
adr: 064
title: "Opinionated Diff Rendering (hunk + diffity)"
status: proposed
implementation: pending
date: 2026-05-20
last_modified: 2026-05-20
supersedes: []
superseded_by: null
related: ["035", "036", "037", "048", "059"]
tags: ["go", "command", "diff", "hunk", "diffity"]
---

# ADR-064: Opinionated Diff Rendering

## Context

ADR-048 made `af diff` a minimal proxy around user-configured
`[diff].cmd`. That was simple, but it under-specifies the owner's actual
workflow. The dotfiles already have a `diff-render` skill/hook that
chooses the best available viewer: hunk in tmux when installed, diffity
for a browser view, and plain `git diff` as the fallback. The owner wants
that workflow promoted into `af` itself.

External research and local skills confirm the tool fit:

- `hunk` is a review-first terminal diff viewer for agent-authored
  changesets, with file navigation and inline annotation support. It is a
  good default only when already installed; `af` should not install it or
  make it mandatory.
- `diffity` is a browser diff viewer. Its CLI accepts refs/ranges,
  reuses or starts a local server, opens the browser, and exposes
  `diffity list --json` for machine-readable running-session discovery.
- `git diff` is the universal fallback and should remain sufficient when
  neither richer viewer is installed.

Because ADRs are append-only, this ADR supersedes ADR-048's `af diff`
subsection without editing ADR-048 in place. ADR-048 still governs
`af editor`, `af pr`, and the general proxy-command parsing model.

## Decision

`af diff` becomes an opinionated renderer with two modes: terminal by
default, browser when `--web` is supplied.

### CLI surface

```text
af diff [session] [--base REF] [--web]
```

`--base` keeps the ADR-048 / ADR-059 semantics:

1. Explicit `--base REF` wins.
2. Else, if `state.toml.[stack].parent_branch` is non-empty, use it.
3. Else use `state.toml.[worktree].base_branch`.

The command runs from the target workstream's worktree. The head ref is
the workstream branch recorded in state, falling back to `HEAD` if the
state is unavailable but the current directory is inside a worktree.

### Default terminal mode

`af diff` without `--web` resolves the diff scope, then dispatches:

| Condition                        | Renderer | Command shape                                              |
| -------------------------------- | -------- | ---------------------------------------------------------- |
| `hunk` is on `PATH`              | hunk     | pipe `git diff --no-color <base>...HEAD` to `hunk patch -` |
| `hunk` is not on `PATH`          | git diff | `git diff <base>...HEAD`                                   |
| stdout is not an interactive TTY | git diff | compact `git diff --stat <base>...HEAD` summary            |

The hunk command is expressed as a pipeline here for clarity: `af`
should implement it with `exec.CommandContext` and an explicit pipe, not
by shelling out through `sh -c`. Hunk receives a patch on stdin so the
same base resolution works whether or not hunk's own Git-range syntax
changes.

`af` does **not** install hunk. If hunk is absent, the fallback is plain
Git. If hunk exits non-zero because the terminal is unsupported, `af`
prints the error and falls back to Git only when no hunk UI was shown.

### Web mode

`af diff --web` uses diffity and opens the diff in a browser. It does not
fall back to hunk or plain Git because the user explicitly requested a
web UI.

The range passed to diffity is explicit and worktree-safe:

```text
<base>..<head>
```

For example:

```text
diffity upstream/main..feature/auth-flow
```

Implementation steps:

1. Probe that `diffity` is on `PATH`. If missing, fail with an install
   hint (`npm i -g diffity`) and do not fall back silently.
2. Compute `range = <base>..<head>`.
3. Run `diffity list --json` when available. If an existing diffity
   instance for this repo already has the same range, open/reuse it.
4. If no matching instance exists, run `diffity <range>` from the
   worktree so diffity starts or reuses its local server and opens the
   browser.
5. Print the localhost URL if it can be discovered from
   `diffity list --json`; otherwise print that diffity was launched and
   leave browser opening to diffity.

`af` should not use `diffity --new` by default because that may discard a
running browser review and its comments. If future UX needs a forced
fresh browser session, add a separate flag in a later ADR.

### Configuration role

ADR-036's `[diff].cmd` remains useful as an escape hatch, but it is no
longer the default `af diff` contract. The default path is:

```text
hunk if installed -> git diff fallback
```

A later ADR may add an explicit `af diff --custom` or `[diff].mode =
"custom"` if the owner still wants arbitrary command execution. v1's
normal path should stay predictable and documented here.

## Consequences

### Pros

- Matches the owner's existing dotfiles workflow instead of requiring
  every repo to configure `[diff].cmd`.
- Gives the best terminal review experience when hunk is already
  installed, with zero mandatory dependency for other machines.
- Keeps a reliable Git fallback that works everywhere.
- Makes browser diff review explicit via `--web` instead of hidden behind
  environment detection.
- Passing explicit diffity ranges avoids ambiguous browser sessions in
  worktrees and stacked branches.

### Cons / risks

- `af diff` is no longer a purely generic proxy; it now knows about hunk
  and diffity.
- The hunk path depends on terminal capabilities. Non-interactive callers
  must receive a Git summary instead of a TUI.
- diffity is another optional external CLI whose flags and registry JSON
  may change.
- The ADR leaves `[diff].cmd` in a demoted/escape-hatch position, so
  ADR-036 and ADR-048 need a future cleanup ADR if arbitrary diff command
  execution is removed completely.
- `git diff <base>...HEAD` reflects branch-vs-base state; users who want
  a different scope must pass `--base` or use Git directly.

## Alternatives Considered

- **Keep ADR-048's pure `[diff].cmd` proxy.** Rejected. It is flexible,
  but it fails to encode the workflow the owner actually wants as the
  default.
- **Always require hunk.** Rejected. Hunk is a good default when present,
  but `af diff` must work on clean machines and remote hosts without
  extra installation.
- **Use diffity as the default renderer.** Rejected. Browser diff review
  is valuable but should be explicit with `--web`; the default terminal
  path should stay fast.
- **Auto-install missing hunk or diffity.** Rejected. `af doctor` may
  print install hints, but command execution should not mutate the user's
  toolchain.
- **Use `diffity --new` every time.** Rejected. It guarantees a fresh
  range but can destroy a browser review in progress.

## References

- ADR-035 — command tree includes `af diff`.
- ADR-036 — existing `[diff]` configuration and token interpolation.
- ADR-037 — workstream state provides base/head/worktree.
- ADR-048 — superseded for `af diff`; still governs editor/pr proxies.
- ADR-059 — stacked workstreams affect default base selection.
- `~/.agents/skills/diff/SKILL.md` — current dotfiles dispatch workflow.
- `~/.agents/skills/diffity-diff/SKILL.md` — current diffity browser workflow.
- `~/.claude/hooks/diff-render.sh` — current hunk/diffity/git fallback script.
- hunk: <https://github.com/modem-dev/hunk>
- diffity: <https://github.com/kamranahmedse/diffity>
