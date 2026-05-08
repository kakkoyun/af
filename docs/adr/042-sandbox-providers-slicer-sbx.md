---
adr: 042
title: "Sandbox Providers (slicer + sbx)"
status: proposed
implementation: pending
date: 2026-05-06
last_modified: 2026-05-08
supersedes: []
superseded_by: null
related: ["031", "041", "043", "049"]
tags: ["go", "sandbox", "slicer", "sbx"]
---

# ADR-042: Sandbox Providers (slicer + sbx)

## Context

The owner uses two sandbox tools: `slicer` (Firecracker microVM) and
`sbx` (Docker AI Sandboxes). Both isolate the agent's filesystem and
network from the host. v1 keeps both, behind a single `Sandbox`
interface.

Composition with `--remote` (ADR-041) means the sandbox provider can
run **on a remote SSH host** — `af` SSHes in, then invokes the
provider's CLI there.

## Decision

### Interface

```go
// internal/sandbox/sandbox.go

type LaunchOpts struct {
    Workstream string  // session name
    Worktree   string  // host path to mount inside the sandbox
    AgentArgv  []string // command to run inside the sandbox
}

type Handle struct {
    ID        string  // provider-specific identifier (slicer hostname, sbx ID)
    AttachCmd []string // command to attach to the running sandbox (e.g. "slicer vm shell <id>")
}

type Sandbox interface {
    Name() string                     // "slicer" | "sbx"
    IsAvailable(ctx context.Context) bool
    Launch(ctx context.Context, opts LaunchOpts) (*Handle, error)
    Attach(ctx context.Context, h *Handle) error
    IsHealthy(ctx context.Context, h *Handle) (bool, error)
    Teardown(ctx context.Context, h *Handle) error
    List(ctx context.Context) ([]Handle, error)
}
```

### Implementations

| Provider                     | Binary   | Backend             | Local | Remote |
| ---------------------------- | -------- | ------------------- | ----- | ------ |
| `internal/sandbox/slicer.go` | `slicer` | Firecracker microVM | ✅    | ✅     |
| `internal/sandbox/sbx.go`    | `sbx`    | Docker AI Sandboxes | ✅    | ✅     |

Both shell out to their CLI via `exec.CommandContext`.

### Selection

`af create --sandbox <provider>` picks the provider explicitly. If
omitted but `--sandbox` is set without an arg, the provider comes from
`[sandbox].default_provider` in config. If both are unset, `--sandbox`
is rejected with a hint.

```
af create --sandbox slicer
af create --sandbox sbx
af create --sandbox            # uses [sandbox].default_provider; errors if empty
```

### Composition with `--remote`

`af create --remote <host> --sandbox <provider>`:

1. SSH to `<host>`.
2. Probe that `<provider>`'s binary is installed on the remote (per ADR-044).
3. Clone repo on the remote (per ADR-041).
4. Invoke `<provider>` CLI on the remote with arguments to mount the cloned worktree and launch the agent inside.
5. Record `state.toml` `[execution]`: `mode = "sandbox"`, `ssh_host`, `remote_path`, `sandbox_provider`, `sandbox_id`.

Two SSH hops: the user's local `af` → remote shell → sandbox VM. The
user attaches by:

```
af resume <name>
  → ssh <host>
    → tmux attach -t <name>      (the launching shell on the remote)
       → already running: slicer vm shell <id>  (or sbx attach <id>)
```

The remote tmux pane's command is the sandbox-attach command itself,
so reattaching to the tmux session lands the user inside the sandbox.

### Path mapping (slicer)

slicer mounts `~/af-worktrees/<repo>/<branch>/` from the host (or
remote, when composed) into the VM at
`/home/ubuntu/host/<repo>/<branch>/`. This is the same VirtioFS
behaviour v0 ADR-005 used; v1 keeps it.

### Path mapping (sbx)

sbx mounts the workstream's worktree as the container's working
directory. No special path-mapping shim needed; the `Workdir` arg to
`sbx run` is `/workstream` inside the container.

### Teardown

`af done` on a sandboxed workstream calls `Sandbox.Teardown(handle)`
which:

- slicer: `slicer vm delete <hostname>`
- sbx: `sbx rm <id>`

Sandbox teardown happens **before** worktree removal. Failures don't
abort the rest of teardown; they're logged and the user is told to
clean up manually.

### Health check + respawn

`af resume --respawn` on a sandboxed workstream:

1. Calls `Sandbox.IsHealthy(handle)` to check VM liveness.
2. If unhealthy and `--respawn` set: tear down the dead VM, launch a
   new one, update `state.toml` with the new handle.
3. If unhealthy and `--respawn` not set: error with a hint.

### Secrets injection

Per ADR-049: secrets reach the sandbox via an **ephemeral envelope**
file. Path varies by sandbox image:

- Image with tmpfs at `/run/user/$UID/`: `/run/user/$UID/af-<session>/.env` (mounted in for slicer; `docker cp` for sbx).
- Image without tmpfs: `~/.local/share/af/v1/secrets/af-<session>/.env` on the sandbox's persistent FS (covered by the in-sandbox lazy 60-min sweep when `af` runs there, plus normal sandbox teardown).

The envelope is sourced once and deleted immediately by the launch
wrapper (per ADR-049 §"Source-and-delete invariant"). Never via env
vars on the provider CLI's command line.

## Consequences

- The same workstream can target slicer or sbx with a flag flip.
- Composition with `--remote` is one extra SSH layer; no special
  "remote sandbox" provider needed.
- VM teardown failures don't strand workstream state; `af done`
  completes locally even if the remote VM linger.

## Alternatives Considered

- **Keep slicer only.** Rejected; sbx is the owner's preferred Docker-based fallback.
- **Add a sandbox plugin layer.** Rejected; same reasoning as ADR-041 — two providers don't justify a plugin system.
- **Run the sandbox provider's daemon embedded.** Rejected; both `slicer` and `sbx` ship CLIs; reuse them.

## References

- v0 ADR-005, v0 ADR-014, v0 ADR-024 — superseded for v1.
- ADR-031 — v1 master.
- ADR-041 — SSH remote (composition).
- ADR-043 — agent providers (sandbox launches an agent inside).
- ADR-044 — doctor probes for sandbox CLIs.
- ADR-049 — secret transport into sandboxes.
