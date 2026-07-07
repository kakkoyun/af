---
adr: 060
title: "Slicer-Only Sandbox Provider (drop sbx)"
status: accepted
implementation: complete
date: 2026-05-20
last_modified: 2026-07-03
supersedes: ["042"]
superseded_by: null
related: ["031", "036", "041", "044", "049", "062"]
tags: ["go", "sandbox", "slicer", "scope"]
---

# ADR-060: Slicer-Only Sandbox Provider

## Context

ADR-042 kept two sandbox providers: `slicer` and Docker `sbx`. The
provider abstraction was attractive when the v1 rewrite still assumed a
pluggable sandbox layer, but it cuts against ADR-031's scope reduction.
Every additional sandbox provider multiplies launch, attach, health,
secrets, remote, teardown, doctor, and test cases.

External research shows that `sbx` is technically feasible, not dead:
Docker documents Sandboxes as isolated microVM environments for AI
coding agents with `sbx run`, `sbx ls`, `sbx stop`, and `sbx rm`, and
the release repository is active. The owner-corrected constraints are
more important for `af`: `sbx` is closed source, and its VMs cannot be
resized to fit a repo's workload. It is also a separate Docker product
surface with its own login, subscription / entitlement model, daemon
behaviour, and organizational policy layer. That may be useful for
Docker users, but it is not worth carrying as a first-class v1 provider
for this single-user tool.

Slicer is a better fit for the owner's stated direction: Firecracker on
Linux, Apple's Virtualization Framework on macOS, explicit host-group
configuration, and documented resource fields (`vcpu`, `ram_gb`,
`storage_size`, optional `gpu_count`). Keeping slicer only also makes
ADR-062 possible without designing a least-common-denominator resource
API across two unrelated sandbox products.

Because ADRs are append-only, this ADR supersedes ADR-042's provider set
without editing ADR-042 in place. Where older v1 ADRs mention `sbx` as a
supported provider, read that as historical context; new implementation
work follows this ADR.

## Decision

`af` v1 supports **slicer only** as a sandbox provider.

### Provider set

The sandbox provider enum has exactly two runtime states:

| Value    | Meaning                                      |
| -------- | -------------------------------------------- |
| `""`     | No sandbox; launch directly in tmux / remote |
| `slicer` | Launch inside a Slicer VM                    |

`sbx` is not a valid provider for new workstreams. If an existing
`state.toml` from an experimental build records `sandbox_provider =
"sbx"`, `af` fails closed with a migration hint instead of trying to
attach or delete an unknown sandbox:

```text
sandbox provider "sbx" is no longer supported by af v1; use slicer or
manage the sbx sandbox manually with the Docker sbx CLI.
```

### Configuration delta

ADR-036's `[sandbox]` schema is narrowed by this ADR:

```toml
[sandbox]
default_provider = ""        # empty or "slicer" only

[sandbox.slicer]
group = ""                   # slicer host group; empty = af default
```

`[sandbox.sbx]` is obsolete. If encountered in user or repo config, the
loader ignores it and emits a warning. The warning is not fatal so old
config files do not block unrelated commands.

### CLI delta

`af create --sandbox` accepts only:

```text
af create --sandbox slicer
af create --sandbox          # uses [sandbox].default_provider; must resolve to slicer
```

`af doctor` probes `slicer` when sandbox support is requested. It no
longer probes, installs, or prints hints for `sbx`.

### Code shape

The implementation keeps a small `Sandbox` interface only if it pays for
it in tests. The interface no longer exists to support multiple
providers; it exists to isolate command execution from command logic.
A concrete `Slicer` type may satisfy it, but there is no registry or
plugin lookup.

### Migration stance

No automatic migration from `sbx` to slicer is attempted. The two tools
use different images, filesystems, lifecycle handles, and resource
models. Users should finish or manually tear down any old `sbx`
workstream before upgrading.

## Consequences

### Pros

- One launch path, one attach path, one teardown path, and one set of
  tests.
- Fewer external dependencies: no Docker-specific login, daemon, paid
  entitlement, closed-source sandbox runtime, or organization policy
  integration in `af`.
- Slicer resource sizing can be modeled directly in ADR-062 instead of
  squeezed through a cross-provider abstraction.
- Remote composition stays simpler: SSH to the host, then invoke slicer
  there.
- Secret injection has one provider-specific implementation to harden.

### Cons / risks

- Users who prefer Docker's sandbox UX lose first-class support.
- Slicer must be installed and working on every host where sandboxed
  workstreams run.
- Docker Sandboxes may evolve faster than slicer in some areas; this ADR
  deliberately ignores that optionality because closed-source,
  fixed-size VMs are the wrong fit for repo-scoped control.
- Existing experimental `sbx` workstreams become manual cleanup tasks.

## Alternatives Considered

- **Keep ADR-042's slicer + sbx pair.** Feasible, but rejected for v1
  scope. The second provider doubles test and lifecycle surface without
  a concrete owner need.
- **Keep a generic sandbox plugin registry.** Rejected per ADR-031. One
  provider does not justify dynamic discovery or provider contracts.
- **Make `sbx` the only provider.** Rejected. Docker Sandboxes are
  real, but `sbx` is closed source and does not let `af` resize VMs per
  repo. Slicer exposes the VM resource model this project wants to
  control.
- **Keep `sbx` as an undocumented escape hatch.** Rejected. Hidden
  support still needs tests and can destroy user data if teardown
  semantics drift.

## References

- ADR-031 — v1 scope reduction.
- ADR-036 — configuration schema narrowed by this ADR.
- ADR-041 — SSH remote model; slicer still composes over SSH.
- ADR-042 — superseded provider set.
- ADR-049 — secret injection into sandboxes.
- ADR-062 — repo-scoped slicer VM resource profiles.
- Docker Sandboxes docs: <https://docs.docker.com/ai/sandboxes/>
- Docker `sbx` usage docs: <https://docs.docker.com/ai/sandboxes/usage/>
- Docker `sbx` releases: <https://github.com/docker/sbx-releases>
- Slicer introduction: <https://docs.slicervm.com/>
- Slicer walkthrough: <https://docs.slicervm.com/getting-started/walkthrough/>
