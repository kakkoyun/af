---
adr: 041
title: "SSH Remote Model (no provider plugins)"
status: accepted
implementation: complete
date: 2026-05-06
last_modified: 2026-07-03
supersedes: []
superseded_by: null
related: ["031", "037", "042", "044", "049"]
tags: ["go", "remote", "ssh"]
---

# ADR-041: SSH Remote Model (no provider plugins)

## Context

v0 had a `RemoteProvider` interface with two implementations
(exe.dev, DD Workspaces) and a plugin discovery mechanism for adding
more. The provisioning of those VMs required custom CLIs (`workspaces
create/delete`, `ssh exe.dev new/rm`). v1 drops this entirely.

The owner provisions remote machines **outside `af`**: by running
`workspaces create ...` manually, by using `exe.dev`'s own CLI, or
through a cloud provider's UI. The user then tells `af` to use that
machine via an SSH host string.

`af` v1 sees a remote as: **a string consumed by `ssh`**. It does not
know whether that's an alias from `~/.ssh/config`, a `user@host`, or
an IP. It does not need to. The user's `~/.ssh/config` is the
authority on how to connect.

## Decision

### Remote = SSH host string

`af create --remote <host>` accepts `<host>` as opaque. It is passed
verbatim to `ssh` invocations:

```go
exec.CommandContext(ctx, "ssh", append(sshOptions, host, remoteCmd)...)
```

`sshOptions` from `[remote].ssh_options` (ADR-036) are prepended.

No validation, no parsing, no DNS resolution by `af`. `ssh` itself
errors out if the host is unreachable.

### What `af` does on the remote

When `af create --remote <host>` runs:

1. Validate `tmux` and the chosen agent binary are available on the
   remote via a probe (`ssh <host> 'which tmux pi'`). If any are
   missing, fail with a doctor-style hint (ADR-044).
2. SSH in and **`git clone`** the repo into
   `~/af-clones/<repo>/<branch>/` on the remote. If a directory at that path already exists from a previous workstream of the same name, `af` errors with a hint to run `af done <name>` (or `af resume <name>` if the user expected this to be a resume). The remote path is recorded in `state.toml` `[execution].remote_path`.
3. Create a tmux session **on the remote** named identically to the
   local workstream name. Launch the agent in the primary pane.
4. The local `af` process exits. The user `ssh <host>` + `tmux a -t <name>` to attach, or `af resume <name>` runs that for them.

**Plain clone, not linked worktree.** Local workstreams use git
worktrees from a single repo (per ADR-038). Remote workstreams use
**plain clones** — one full clone per workstream at
`~/af-clones/<repo>/<branch>/`. Reasons:

- The remote may host workstreams from machines that don't share a
  `git worktree` parent. A plain clone is self-contained.
- Sandbox providers (slicer/sbx) mount the workstream's directory;
  mounting a worktree linked to a bare repo elsewhere on the host
  adds a layer of fragility (the bare repo must also be mounted, or
  the worktree resolves git operations through unmounted paths).
- Storage overhead is acceptable for the workstream counts the owner
  expects (single-digit concurrent remote workstreams).

The `~/af-worktrees/...` path used for **local** workstreams is
intentionally distinct from `~/af-clones/...` used for remote, to
make the two models obviously different on disk and to keep `af
clean` reasoning simple (no recursive worktree-from-worktree state).

### Why local-tmux + ssh-attach instead of running tmux locally over SSH

If `tmux` ran locally and just exec'd via SSH, every disconnect would
kill the agent. Running tmux **on the remote** lets the SSH connection
drop and reconnect without the agent noticing.

### Reconnection on SSH drop

The remote's tmux server keeps the session alive. The user reconnects
by re-running `af resume <name>`, which:

1. Resolves the remote host from `state.toml`.
2. SSHes in.
3. `tmux attach -t <name>` on the remote.

There is no automated reconnection loop in v1. tmux's persistence is
sufficient; the owner is comfortable re-running `af resume` after a
drop.

### Path mapping

Remote path: `~/af-clones/<repo>/<branch>/`. Local workstream state
still tracks the **local** worktree path (which is empty for
remote-only workstreams) and the **remote** path explicitly in
`state.toml.[execution].remote_path`. `af note` and the Obsidian
integration don't care about the remote path; the markdown note
lives in the local vault.

### Composition with `--sandbox`

`af create --remote <host> --sandbox <provider>` SSHes in, then
invokes the sandbox provider's CLI (`slicer` or `sbx`) **on the
remote**, which builds a VM there and launches the agent inside. ADR-042
details this.

The composition matrix is just two flags. There is no plugin layer
choosing among providers.

### Teardown

`af done` on a remote workstream (workstream is a plain clone, not a
linked worktree):

1. SSHes in.
2. Kills the remote tmux session.
3. **Safety-checks the clone before deletion**:
   - `git -C <remote_path> status --porcelain` must be empty (no
     uncommitted changes), unless `--force`.
   - `git -C <remote_path> log --branches --not --remotes` must be
     empty (no unpushed commits), unless `--force` or the branch is
     known-merged via the same three-strategy check ADR-056 uses.
4. Removes the clone directory: `rm -rf ~/af-clones/<repo>/<branch>/`.
   (No `git worktree remove` — the clone is not a worktree.)
5. Optionally deletes the branch on `origin` if merged or `--force`.

If the parent directory `~/af-clones/<repo>/` is empty after step 4,
`af` removes it too.

The **remote machine itself** is not torn down. The user provisioned
it externally; `af` doesn't unprovision. That keeps `af`'s scope
inside the workstream lifecycle, not the VM lifecycle.

### Doctor on remote

`af doctor --remote <host>` SSHes in and runs the same probe as local
doctor. Prints install commands for the **remote's** package manager
(detected via `/etc/os-release` over SSH). Never auto-installs. ADR-044
specifies.

### Secrets on remote

Per ADR-049, secrets are transported via an **ephemeral envelope**
file `scp`'d to the remote. The destination depends on what the
remote offers:

- Linux remote with `/run/user/$UID/` writable: `/run/user/$UID/af-<session>/.env` (tmpfs).
- Otherwise (e.g. macOS remote, or a stripped Linux without `XDG_RUNTIME_DIR`): `~/.local/share/af/v1/secrets/af-<session>/.env` (persistent disk; covered by the lazy 60-min sweep on the remote).

The envelope is sourced once and deleted immediately by the launch
wrapper (per ADR-049 §"Source-and-delete invariant"). **Never** via
`SSH SetEnv` or `SendEnv` — those leak through the env to every
command run on the remote.

## Consequences

- No provider plugin layer to maintain.
- Adding a new VM provider is "set up `~/.ssh/config`" — no code change.
- The remote feature surface stays minimal: probe, clone, launch, attach, kill.
- Users who prefer specific provisioning workflows aren't constrained by `af`'s opinions.

## Alternatives Considered

- **Keep the v0 plugin layer.** Rejected per scope cut (ADR-031); single user, no need to support multiple back-ends.
- **Run tmux locally, exec via SSH.** Rejected; SSH drops kill the session.
- **Embed an SSH client (`crypto/ssh`).** Rejected; reuses the OS's `ssh` binary so config + multiplexing + agent forwarding all just work.
- **Auto-provision via cloud APIs.** Rejected as out of scope; the user has separate tools for that.

## References

- v0 ADR-004, v0 ADR-017, v0 ADR-027 — superseded by this ADR for v1.
- ADR-031 — v1 master.
- ADR-037 — session metadata `[execution].ssh_host`, `remote_path`.
- ADR-042 — sandbox composition with remote.
- ADR-044 — doctor over SSH.
- ADR-049 — secret transport (no SSH SetEnv).
