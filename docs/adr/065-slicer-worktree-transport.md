---
adr: 065
title: "Slicer Worktree Transport (`slicer wt`)"
status: proposed
implementation: pending
date: 2026-05-21
last_modified: 2026-05-21
supersedes: []
superseded_by: null
related: ["037", "038", "041", "043", "046", "049", "060", "062"]
tags: ["go", "sandbox", "slicer", "worktree", "git"]
---

# ADR-065: Slicer Worktree Transport (`slicer wt`)

## Context

ADR-060 made slicer the only sandbox provider, and ADR-062 added
repo-scoped slicer VM resource profiles. Those ADRs still assumed the
older model: `af` launches a slicer VM and makes the host worktree
available to it. Slicer's new `wt` API changes the safer default.

The new flow is:

```text
slicer wt push --launch .        # launch a VM, push the current worktree in
slicer vm shell <vm>             # work in it, or point an agent at it
slicer wt pull <vm> .            # pull commits + files back; branch fast-forwarded
git push                         # from the host, under the user's identity
```

`slicer wt` pushes a worktree (or repo) into a VM with a working,
self-contained `.git`. It stages a fresh, sanitised Git directory: no
host hooks, no foreign config, and no host repository mount. The VM
cannot reach or corrupt the host repo. It sets `origin` to the HTTPS
upstream so VM-side Git can push through `slicer-proxy`; it syncs safe
Git identity/preferences such as `user.name` and `user.email`; and it
never copies credentials. Pulling imports VM branches under
`refs/slicer/<vm>/*` and fast-forwards the host branch so VM work comes
back as real commits.

The crucial operational rule is that the host worktree is **leased to the
VM** while the VM holds it. The user must not edit the host worktree
until `slicer wt pull` completes. Pull overwrites host files with the VM
copy; host-side edits made after push are lost.

## Decision

`af` adopts `slicer wt` as the slicer sandbox transport. A slicer-backed
workstream is no longer a mounted host worktree; it is a VM-local clone
created by `slicer wt push` and reconciled by `slicer wt pull`.

### Launch flow

`af create --sandbox slicer` runs from the host worktree (or remote clone
when composed with ADR-041) and launches a fresh VM with `slicer wt`:

```text
slicer wt push --launch [--hostgroup GROUP] [--depth N] \
  --tag af --tag af-session=<name> --tag af-repo=<repo> <worktree-path>
```

- `<worktree-path>` is the target workstream path; it defaults to the
  current directory when the user is already inside that workstream.
- `--hostgroup GROUP` is derived from ADR-062's slicer resource/profile
  resolution when present.
- `--depth N` is optional and exists for large repos; `0` / unset means
  Slicer's default full history behaviour.
- `--tag` records enough metadata for `slicer wt list` and `slicer vm
  list` to identify `af`-managed VMs.

`af` parses and records the VM name printed by slicer. After push, `af`
launches or attaches the agent inside the VM using the slicer VM command
surface (`slicer vm shell <vm>` for interactive attach, or slicer exec / agent
entrypoint support when available). Exact agent argv still comes from
ADR-043.

### Existing VM push and force

For recovery or explicit re-push into an existing VM, `af` may use:

```text
slicer wt push <vm> <worktree-path>
```

`--force` / `-f` is never used implicitly. It wipes VM-side state before
re-pushing and may discard unpulled VM work, so it requires an explicit
user action such as a future `af repair --force` or a manual slicer
command.

### Pull flow

When the user wants VM work back on the host, `af` runs:

```text
slicer wt pull <vm> <worktree-path>
```

This imports VM branches under `refs/slicer/<vm>/*`, fast-forwards the
host branch when possible, and writes VM files into the host worktree.
After a successful pull the host worktree should be clean and can be
pushed normally by the user:

```text
git push
```

`af done` and `af suspend` must not delete a slicer-wt VM before either:

1. `slicer wt pull <vm> <worktree-path>` succeeds, or
2. the user passes an explicit destructive/discard flag.

`af resume` attaches to the VM and does not pull automatically. Pull is a
boundary operation: it moves ownership of the worktree state back to the
host.

### Host worktree lease rule

On successful push, `af` records a host-worktree lease in `state.toml`:

```toml
[slicer_wt]
vm          = "sbox-abc123"
path        = "/abs/path/to/worktree"
pushed_at   = "2026-05-21T...Z"
pulled_at   = null
lease_state = "held_by_vm" # held_by_vm | pulled | discarded
```

While `lease_state = "held_by_vm"`, `af` treats the host worktree as
checked out to the VM:

- `af editor` should open the VM/session, not the stale host files.
- `af diff` should warn that the host may be stale unless a pull has
  occurred.
- `af pr` should refuse until the VM work has been pulled back and the
  host branch contains the commits.
- `af status` / `af info` should show the VM name and lease state.

`af` cannot prevent the user from editing files with external tools, but
it must make the lease visible and avoid encouraging host-side edits.

### `slicer wt list` integration

`af` may use `slicer wt list` for recovery and diagnostics. The `*` marker
for the current directory helps map a host worktree back to its VM when
state exists but the user is unsure which VM owns it.

`af doctor` should probe for `slicer wt push --help` to distinguish new
Slicer builds from older slicer installs that lack the worktree API.

### Git identity and credentials

`af` relies on slicer wt's sanitised `.git` staging:

- Host Git hooks are not copied and do not run in the VM.
- Host Git credentials are not copied.
- Safe identity/preferences (`user.name`, `user.email`, safe aliases) are
  synced into the VM.
- `origin` is set to the HTTPS upstream so VM-side Git operations use
  `slicer-proxy` rather than direct host credential material.

Secrets for agents remain governed by ADR-049. This ADR covers Git
transport and repository safety, not API-key delivery to the agent.

### Remote composition

For `af create --remote <host> --sandbox slicer`, all `slicer wt` commands
run on `<host>` against the remote clone/worktree that ADR-041 created.
The remote machine owns the VM, the slicer daemon, and the VM-local Git
copy. Pulling from the VM returns commits to the remote clone; pushing to
the canonical upstream remains an explicit user action under the user's
normal Git identity.

## Consequences

### Pros

- The host repository is never mounted into the VM, eliminating a major
  corruption and hook-execution risk.
- The VM gets a real `.git`, so agents can use normal Git commands,
  commit locally, inspect history, and work with branches.
- Pulling imports VM work as commits and refs, not as an opaque file copy.
- Git credentials stay out of the VM; slicer-proxy handles upstream
  access without copying host secrets.
- ADR-062 resource profiles still compose via `--hostgroup`.
- The workflow matches the owner's intended loop: push to VM, work there,
  pull back, then push from the host.

### Cons / risks

- The host worktree must be treated as read-only while leased to the VM;
  `slicer wt pull` overwrites host files with the VM copy.
- `af` can warn and refuse its own host-side commands, but it cannot stop
  a user or editor from modifying the host path directly.
- Commands that inspect host files (`af diff`, `af pr`, tests run on the
  host) are stale until pull.
- The implementation depends on new slicer CLI behaviour and output
  parsing; older slicer versions need a doctor hint instead of fallback
  to unsafe mounts.
- Pull conflicts or non-fast-forward cases need clear recovery guidance.
- Remote composition has two reconciliation steps: VM to remote clone via
  `slicer wt pull`, then remote/host/upstream Git workflow as usual.

## Alternatives Considered

- **Keep mounting the host worktree into the VM.** Rejected. It exposes
  the host repo, hooks, and config to VM-side tools and lets the VM
  corrupt host state directly.
- **Use `slicer vm cp` / tarballs instead of `slicer wt`.** Rejected.
  File copies lose Git branch/ref semantics and make commit import a
  custom `af` problem.
- **Clone from origin inside the VM manually.** Rejected. It requires
  credentials or custom auth setup in the VM and does not preserve the
  exact host worktree state the user launched from.
- **Let the VM push directly as the final publication path.** Rejected.
  The owner wants `git push` from the host under the normal host identity
  after pulling VM commits back.
- **Use `--force` automatically when re-pushing.** Rejected. It can wipe
  VM-side work and must stay an explicit recovery action.

## References

- ADR-037 — session state records VM handles and lease metadata.
- ADR-038 — host worktree layout remains the launch/pull anchor.
- ADR-041 — remote composition; commands run where the slicer daemon runs.
- ADR-043 — agent argv still comes from the selected agent provider.
- ADR-046 — suspend/done lifecycle must pull or explicitly discard.
- ADR-049 — non-Git secret transport remains separate.
- ADR-060 — slicer-only sandbox provider.
- ADR-062 — slicer resource profiles map to `slicer wt push --hostgroup`.
- Owner-provided Slicer `wt` API note, 2026-05-20.
