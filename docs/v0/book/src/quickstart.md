# Quickstart

Five commands cover the common happy path.

## 1. Check your environment

```bash
af doctor
```

Lists every required tool and whether it is installed. Run `af doctor --fix` to
auto-install missing dependencies.

## 2. Create a workstream

```bash
af create fix-auth-bug
```

This does three things atomically:

1. Creates a git worktree at `~/Workspace/.worktrees/<repo>/fix-auth-bug`
2. Opens a tmux session named `fix-auth-bug`
3. Launches Claude Code inside the session

You are now inside the tmux session with your agent running.

## 3. Work

Let the agent work. You can detach from tmux at any time (`Ctrl-b d`) and
re-attach later without losing state.

```bash
af resume fix-auth-bug   # re-attach after detaching
```

## 4. Review and merge

When the agent has finished, review the diff:

```bash
af diff                  # visual diff vs the base branch
af pr                    # create a GitHub PR from session metadata
```

## 5. Clean up

Once the PR is merged, tear down the workstream:

```bash
af done
```

This removes the tmux session, deletes the worktree, and archives the session
ledger. The git branch is deleted after confirmation.

## Common options

```bash
af create --agent pi fix-bug         # use pi instead of claude
af create --from develop hotfix      # fork from a specific branch
af create --auto task                # auto-approve edits, prompt for destructive ops
af create --yolo --sandbox fast-fix  # skip all prompts, run in Firecracker VM
af create --remote fix-infra         # agent runs on a remote exe.dev VM
```

See [Approval Modes](concepts/approval-modes.md) for details on `--auto` and `--yolo`.
