# af by example

Worked, copy-pasteable recipes for every major feature. Each block is a
real command sequence against the current `af` surface — start with the
[README quickstart](../README.md#quickstart) if you haven't run
`af setup` yet.

Everything here assumes your shell is inside a git repository unless a
recipe says otherwise.

---

## The daily driver

One task, one workstream, from idea to archive:

```bash
af create fix-auth                 # branch + worktree + tmux + agent, then attaches
# ... work with the agent inside the tmux session, detach when done ...

af status                          # dashboard: every workstream, PR state, age
af note fix-auth --append "auth middleware done; tests pending"

af resume fix-auth                 # jump back in any time (attaches)
af done fix-auth                   # tear down + archive when merged
```

If you skip the attach (`--no-attach`, or a non-interactive shell),
`af create` prints where to go next:

```
  → to attach:   af resume fix-auth     (or: tmux attach -t af-fix-auth)
  → to check in: af status
  → to finish:   af done fix-auth
```

Common create variations:

```bash
af create hotfix --from release-1.2    # base on a specific branch
af create spike --current              # branch off whatever HEAD is now
af create docs-pass --agent claude     # override [general].default_agent
af create infra --bare                 # worktree + state only, no agent, no tmux attach
```

## Resuming: names, hints, and the picker

`af` commands take the **workstream name**, not the tmux session name —
and they tell you when you mix them up:

```bash
af resume af-fix-auth
# session 'af-fix-auth' not found
# hint: 'af-fix-auth' looks like a tmux session name; did you mean: af resume fix-auth
```

Resume is safe to run on anything non-terminal:

```bash
af resume fix-auth        # suspended → restores + attaches; active → just attaches
af resume fix-auth --bare # restore state only; prints the manual tmux attach hint
```

With no name at all, session resolution walks: positional → `--session`
→ `$AF_SESSION` → the `.af/state.toml` discovery symlink in your cwd →
an interactive `fzf` picker (when on a TTY):

```bash
cd ~/Workspace/.worktrees/myrepo/fix-auth
af info            # no name needed — discovered from the worktree
af note --append "found the root cause"
```

## Several agents on one task

Give a second agent its own sibling worktree and branch so the two
can't trample each other:

```bash
af create big-refactor
af agent add --slot reviewer --agent claude --session big-refactor
af agent list --session big-refactor

# later: stop the extra slot and clean up its worktree + branch
af agent stop reviewer --remove-worktree --session big-refactor
```

## Stacked workstreams

Build a child change on top of a parent that hasn't merged yet, and
keep it rebased as the parent moves:

```bash
af create api-v2 --bare
af create api-v2-docs --bare
af stack api-v2-docs --parent api-v2

# parent moved? re-sync the child (fetches origin when one exists,
# then rebases; on conflict git is left mid-rebase for you to resolve)
af sync api-v2-docs

af unstack api-v2-docs             # detach from the stack when done
```

## Diff, PR, and review

The `[diff]` / `[pr]` / `[editor]` sections of `~/.config/af/config.toml`
define *your* tools; af runs them with tokens substituted:

```toml
[diff]
shell = false
cmd   = ["git", "diff", "{base}...HEAD"]

[pr]
shell = false
cmd   = ["gh", "pr", "create", "--base", "{base}", "--head", "{head}"]
```

```bash
af diff fix-auth                   # your diff command, in the right worktree
af diff fix-auth --web             # open the range in the browser via diffity

af pr fix-auth --draft             # your PR command with --draft appended
af pr fix-auth --ai                # body drafted by your agent from the diff
af pr fix-auth --refresh           # re-sync the cached PR state from gh

af review fix-auth --pr 42         # draft review report; read-only, never posts
# → writes <worktree>/.af/reviews/<timestamp>-pr42.md

af editor fix-auth                 # open your editor at the worktree
```

`af status` and `af info` show cached PR state and refresh it when it's
older than `[pr].refresh_ttl`; add `--refresh` to force it.

## Sandboxed agents (slicer VMs)

Run the agent inside an isolated Linux VM that leases your worktree,
then pull the branches and the agent's own session transcripts back:

```toml
[sandbox.slicer]
group = "sbox"                     # your slicer host group
```

```bash
af create risky-migration --sandbox slicer   # worktree pushed into a fresh VM

af session-data list risky-migration         # inventory transcripts inside the VM
af session-data sync risky-migration         # copy + merge them into your host home
af session-data sync risky-migration --continue-host
# ↑ additionally rewrites VM paths in the transcripts so
#   `claude --resume` works from the HOST worktree

af pull risky-migration            # import VM branches, fast-forward, release the lease
af done risky-migration            # auto-syncs transcripts before teardown
af done risky-migration --discard  # ...or skip the sync and accept transcript loss
```

## Remote hosts

```bash
af create gpu-task --remote buildbox        # minimal SSH setup: mkdir + clone

af control up --remote buildbox             # tmux web UI over your tailnet
af control status --json
af control down --remote buildbox
```

## Notes, Obsidian, and retro mining

Every workstream has an append-only ledger; `af note` adds to it and
`af retro` mines the archive:

```bash
af note fix-auth --append "workaround: token refresh races the proxy"

af retro --since 2w                # everything archived in the last two weeks
af retro --search "token refresh" --limit 5
af retro --since 1w --ai           # narrative synthesis via your agent
```

To also get a per-workstream Obsidian note on `af create`, point
`notes_vault` at a vault (it's opt-in and off by default):

```toml
[obsidian]
notes_vault  = "personal"
notes_folder = "00 - af"

[obsidian.vaults]
personal = "/Users/you/Vaults/personal"
```

See [`examples/obsidian/`](../examples/obsidian/README.md) for the full
worked example including the resulting note layout.

## Secrets

Credentials live in your OS keyring (macOS Keychain / Linux Secret
Service) — never in config files:

```bash
af auth set anthropic_api_key      # prompts with echo off
af auth status                     # the curated trio + extras, values redacted
af auth get github_token           # plain on a TTY, redacted when piped
af auth clear openai_api_key
```

## Housekeeping

```bash
af clean --dry-run                 # what would be reaped, including VM-leased targets
af clean --max-age 2w              # only terminal workstreams older than two weeks
af clean --include-abandoned --force
```

And the one-command health check — run it after install, after upgrades,
and before filing a bug:

```bash
af doctor                          # tool inventory + install hints
af doctor --all --report           # full self-smoke in an isolated scratch HOME
# doctor self-smoke: 21 pass, 0 fail, 0 skip (af v1.0.0 on darwin/arm64)
# report: ./af-doctor-smoke-20260709-120000.md (json: ...)

af doctor --all --issue            # failures? file them on the repo via gh
```

The markdown report is the ideal bug-report attachment: it carries the
exact failing commands, exit codes, and environment.

## Scripting af

Machine-readable output is a versioned envelope, and failures follow a
sysexits-style exit-code contract:

```bash
af status --json | jq '.data'      # unwrap the {"schema": 1, "data": ...} envelope

af note ghost --append hi
echo $?                            # 66  (EX_NOINPUT: no such workstream)

# simulate a missing gh — resolve af's own path first, since a
# restricted PATH would otherwise hide af (usually in $GOPATH/bin) too
AF=$(command -v af)
PATH=/usr/bin:/bin "$AF" pr fix-auth --title t; echo $?   # 69 (EX_UNAVAILABLE: gh missing)

AF_LOCK_TIMEOUT=2s af note busy --append x    # 75 (EX_TEMPFAIL) instead of hanging
```

Inside a workstream's tmux session, `AF_SESSION` is pre-set, so agents
and scripts can call af without naming the session:

```bash
af note --append "checkpoint: tests green"    # no name needed inside the session
```

## Shell completions

```bash
af completions --install           # auto-detects your shell from $SHELL
af completions zsh --install --dry-run
```

Completions cover command names, flags, workstream names for
`[session]` arguments, and lifecycle states for `af status --filter`.
