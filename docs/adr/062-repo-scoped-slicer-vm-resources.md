---
adr: 062
title: "Repo-Scoped Slicer VM Resource Profiles"
status: proposed
implementation: pending
date: 2026-05-20
last_modified: 2026-05-20
supersedes: []
superseded_by: null
related: ["036", "041", "044", "060", "061"]
tags: ["go", "sandbox", "slicer", "resources", "config"]
---

# ADR-062: Repo-Scoped Slicer VM Resource Profiles

## Context

Once ADR-060 drops `sbx`, slicer becomes the only sandbox resource model
`af` needs to understand. This matters because `sbx` is closed source and
its VMs cannot be resized. The owner wants repo-specific sizing when a
project needs it: small repos should not inherit oversized VMs, and
large repos should be able to request enough CPU, memory, disk, or GPU
capacity without changing global defaults for every other repo.

External research confirms this is feasible with Slicer. Slicer host
groups are configured in YAML and documented with fields such as
`vcpu`, `ram_gb`, `storage_size`, `count`, `network`, `hypervisor`, and
optional `gpu_count`. The walkthrough shows a default Firecracker host
group with `vcpu: 2`, `ram_gb: 4`, and `storage_size: 25G`; the macOS
installation docs include persistent and ephemeral host groups; GPU
examples customize vCPU, RAM, disk, GPU count, and hypervisor. Slicer can
run fixed host groups or start with `count: 0` and create disposable VMs
on demand through its CLI/API surface.

The implementation risk is not whether resources can be expressed; they
can. The risk is who owns host capacity and daemon configuration. `af`
should let a repo request resources, but it must not silently rewrite a
machine's global Slicer config or overcommit the host without a clear
error.

## Decision

Repo config may define an optional slicer resource profile under
`[sandbox.slicer.resources]`.

### Schema

```toml
[sandbox.slicer]
group = ""                    # optional existing host group override

[sandbox.slicer.resources]
name         = ""             # optional profile name; empty = derived from repo
vcpu         = 0              # 0 = slicer / group default
ram_gb       = 0              # 0 = slicer / group default
storage_size = ""             # e.g. "25G"; empty = slicer / group default
gpu_count    = 0              # 0 = no explicit GPU request
image        = ""             # optional slicer image override
hypervisor   = ""             # empty = slicer default; e.g. "firecracker" or "qemu"
```

All fields are optional. An empty `[sandbox.slicer.resources]` is a
no-op. If no resource field is set, `af` behaves as ADR-060 describes:
launch in the configured slicer group or in the default group.

### Resolution

When launching `af create --sandbox slicer`:

1. Resolve repo control settings per ADR-061.
2. Resolve slicer group and resources from merged config.
3. If `group` is set and no resource fields are set, launch in that
   existing group.
4. If any resource field is set, derive a deterministic managed group
   name:

   ```text
   af-<repo-slug>-<profile-or-default>
   ```

5. Ask Slicer for an existing group with that name.
6. If it exists, verify its resource shape matches the requested shape.
   A mismatch is an error with a hint to choose a new profile name or
   update the Slicer group manually.
7. If it does not exist, create it through Slicer's CLI/API with
   `count: 0` so VMs are disposable and created on demand.
8. Launch the sandbox in that group and record the effective resource
   profile in `state.toml`.

`af` never edits `/etc/slicer` or a user's checked-in Slicer YAML file in
place. The Slicer daemon remains the authority for whether a requested
host group can be created on the current machine.

### Validation

- `vcpu`, `ram_gb`, and `gpu_count` must be non-negative integers.
- `storage_size` must match Slicer's size grammar as a string (for
  example `15G`, `25G`, `30G`). `af` validates obvious mistakes but lets
  Slicer reject provider-specific edge cases.
- `hypervisor` is optional. `qemu` is required for GPU passthrough in
  documented Slicer examples; Firecracker remains the normal Linux path.
- A repo cannot request both a fixed `group` and resource fields that
  would imply creating a different managed group. That is a parse error:
  either point at a pre-existing group or let `af` manage a profile.

### Existing workstreams

Resource changes affect new sandboxes only. `af resume --respawn` uses
the resource profile captured in `state.toml` for that workstream, not
whatever the repo config says today. This prevents a repo config edit
from unexpectedly changing an active VM's size.

### Remote composition

With `af create --remote <host> --sandbox slicer`, resource resolution
happens locally, but Slicer group probing and creation happen on the
remote host. Capacity errors are remote errors and must name the remote
host in the diagnostic.

## Consequences

### Pros

- Repos can right-size VMs without global slicer changes.
- Small projects avoid wasting RAM/CPU; large projects can request the
  resources needed for builds, tests, or local services.
- Slicer-specific fields are explicit instead of hidden behind a generic
  provider abstraction.
- Capturing the profile in state makes respawn deterministic.
- The model composes with remote hosts because slicer already exposes
  CLI/API management on the machine that owns the VMs.

### Cons / risks

- Repo config can ask for resources the current machine cannot provide.
- Provider-specific config now appears in repo config. That is acceptable
  only because ADR-060 makes slicer the sole sandbox provider.
- Automatically creating Slicer groups depends on installed Slicer CLI/API
  capabilities; older installations may need manual group creation.
- Changing a resource profile does not resize existing VMs. Users must
  suspend/respawn or create a new workstream.
- GPU and QEMU cases are more host-specific than normal Firecracker VMs;
  diagnostics must be explicit.

## Alternatives Considered

- **Use one global slicer group for every repo.** Rejected. Simple, but
  either wastes resources or under-powers large repos.
- **Put resources in `[control]`.** Rejected. ADR-061 controls launch
  policy; resource shape is provider-specific and belongs under
  `[sandbox.slicer]`.
- **Let users manage all Slicer YAML manually.** Rejected as the only
  path. Manual groups remain supported via `group`, but repo defaults
  should be able to request common sizes.
- **Abstract resources across slicer and sbx.** Rejected by ADR-060;
  there is no second provider to normalize against, and `sbx` cannot
  resize VMs anyway.
- **Resize existing VMs in place.** Rejected. Disposable respawn is safer
  and matches the workstream lifecycle.

## References

- ADR-036 — config schema and repo config layer.
- ADR-041 — remote execution; slicer management runs on the remote host.
- ADR-044 — doctor should validate slicer availability and version.
- ADR-060 — slicer-only sandbox provider.
- ADR-061 — repo-scoped control settings.
- Slicer introduction: <https://docs.slicervm.com/>
- Slicer walkthrough host-group example: <https://docs.slicervm.com/getting-started/walkthrough/>
- Slicer macOS installation and sandbox groups: <https://docs.slicervm.com/mac/installation>
- Slicer storage overview: <https://docs.slicervm.com/storage/overview/>
- Slicer GPU examples: <https://docs.slicervm.com/examples/k3s-gpu/>
