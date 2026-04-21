# create

> Generated from `af create --help`. Do not edit by hand — re-run `just book-gen`.

```text
Create a new workstream: worktree + multiplexer session + agent

Usage: af create [OPTIONS] [NAME]

Arguments:
  [NAME]  Task name (becomes the branch and session name). If omitted, auto-generates from repo name
          + timestamp

Options:
      --from <FROM>       Fork from a specific branch instead of the default
  -v, --verbose...        Enable verbose output (-v, -vv, -vvv)
      --current           Fork from the current branch
      --from-pr <NUMBER>  Create worktree from a GitHub PR number
      --bare              Run agent locally on the host worktree (review/PR mode)
      --remote [<HOST>]   Run agent on a remote VM (via exe.dev or configured provider)
      --sandbox           Run agent inside a Firecracker sandbox (via slicer)
      --auto              Auto-approve edits and safe tools, prompt for destructive operations
      --yolo              Skip all permission prompts (sandbox/unattended mode)
      --agent <AGENT>     Select the AI agent to launch (e.g., "claude", "pi")
  -h, --help              Print help
  -V, --version           Print version
```
