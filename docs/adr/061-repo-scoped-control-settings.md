---
adr: 061
title: "Repo-Scoped Control Settings"
status: accepted
implementation: complete
date: 2026-05-20
last_modified: 2026-07-03
supersedes: []
superseded_by: null
related: ["035", "036", "038", "043", "060", "063"]
tags: ["go", "config", "repo", "control"]
---

# ADR-061: Repo-Scoped Control Settings

## Context

ADR-036 already defines layered TOML config with an optional repo layer at
`<repo>/.af/config.toml`, but it does not say which launch-control
preferences belong there. Without a precise contract, every repo either
inherits the owner's global defaults or requires long `af create` command
lines.

The owner wants repo-level control settings: a repository should be able
to say "this project normally runs with this agent, this approval mode,
this sandbox posture, and this remote-control preference" without
changing global config for every other repository.

External practice supports this shape. Git has repository-specific
configuration in `.git/config` and conditional includes for selecting
settings by repository. `direnv` uses a directory-scoped `.envrc` model
with explicit trust. CLI configuration guidance generally recommends a
hierarchy of command-line flags over environment, repo/directory config,
user config, system config, then defaults. `af` already chose TOML and a
repo config layer, so the feasible path is to make the repo layer more
explicit, not add a new settings system.

## Decision

`<repo>/.af/config.toml` may contain a `[control]` section for
repo-scoped workstream launch defaults.

### Schema

```toml
[control]
agent          = ""       # empty = [general].default_agent / compiled default
approval_mode  = ""       # empty = agent default; values from ADR-043
sandbox        = ""       # empty = no sandbox; "slicer" only per ADR-060
remote         = ""       # empty = local; otherwise SSH host string per ADR-041
remote_control = ""       # empty/off; "superterm" per ADR-063
max_agents     = 0        # 0 = no repo cap beyond global max_sessions
```

The section is legal in user config too, but its main purpose is repo
config. A repo value wins over user config; CLI flags win over both.

### Precedence

For commands that launch or resume workstreams, effective control values
are resolved in this order:

1. CLI flags (`--agent`, `--sandbox`, `--remote`, `--yolo`, `--auto`,
   future `af control` flags).
2. Repo config `[control]` from `<repo>/.af/config.toml`.
3. User config `[control]` from `~/.config/af/config.toml`.
4. Existing subsystem defaults (`[general].default_agent`,
   `[remote].default_host`, `[sandbox].default_provider`).
5. Compiled defaults.

This ADR does not remove ADR-036's subsystem sections. `[control]` is a
small policy layer for **what to choose by default for this repo**;
subsystem sections remain the place for **how the subsystem behaves**.
For example, `[control].sandbox = "slicer"` chooses slicer for this repo,
while `[sandbox.slicer]` configures slicer details.

### Validation

- `sandbox` accepts only `""` or `"slicer"` (ADR-060).
- `remote` is passed as an opaque SSH host string (ADR-041). It must not
  contain shell metacharacters; if a value needs spaces or complex SSH
  options, use `[remote].ssh_options` instead.
- `approval_mode` uses ADR-043's mode vocabulary. Unknown values are
  parse errors, not warnings.
- `remote_control` accepts `""`, `"off"`, or `"superterm"` once ADR-063
  is implemented. Unknown values are parse errors.
- `max_agents` is a repo-local cap. It cannot exceed `[general].max_sessions`.

### Commit safety

Repo control settings may be committed when they describe project policy.
They must not contain secrets, API tokens, license keys, Tailscale auth
keys, or private command strings. Secret material remains covered by
ADR-049 and global-only config rules.

Host aliases such as `work-mini` or `hetzner-af` are allowed because SSH
configuration is already user-machine-scoped in `~/.ssh/config`. If a
host name is sensitive, keep it in user config or pass it on the CLI.

### State capture

When `af create` resolves control settings, it writes the effective
choices into `state.toml` using the existing execution and agent fields.
Later changes to repo config do not silently mutate existing workstreams.
They affect new workstreams and explicit reconfiguration commands only.

## Consequences

### Pros

- Repositories can encode their normal agent/sandbox/remote posture once.
- The existing ADR-036 config layer is reused; no new file format or
  dependency is introduced.
- CLI flags remain the highest-precedence escape hatch.
- Effective values are captured in state, making workstreams stable even
  when repo defaults change later.
- The model mirrors common per-repo / per-directory configuration
  practice without importing `git config` or `direnv` semantics.

### Cons / risks

- `[control]` partially overlaps existing sections, so documentation must
  be clear about "policy choice" vs. "subsystem configuration."
- Committed repo config can reveal host aliases or workflow preferences.
- More defaults can make `af create` less explicit; `af info` and
  `af config show` must show where each effective value came from.
- Invalid repo config can break commands for everyone who checks out the
  repository until fixed.

## Alternatives Considered

- **Use only ADR-036 subsystem fields.** Rejected. It works technically,
  but it spreads launch policy across `[general]`, `[remote]`, and
  `[sandbox]` instead of giving users one obvious repo-level section.
- **Create `<repo>/.af/control.toml`.** Rejected. A second repo config
  file adds discovery and precedence rules without enough benefit.
- **Use Git config keys (`af.control.agent`) instead of TOML.** Rejected.
  Git config is great precedent, but `af` already has a typed TOML
  config and should not split state across formats.
- **Use `.envrc` / environment variables.** Rejected. Environment is too
  implicit for durable workstream policy and can leak into agents.

## References

- ADR-035 — CLI flags remain the highest precedence surface.
- ADR-036 — layered TOML config and repo config location.
- ADR-038 — repo discovery and worktree layout.
- ADR-043 — agent providers and approval modes.
- ADR-060 — slicer-only sandbox provider.
- ADR-063 — `remote_control = "superterm"` integration.
- Git config documentation: <https://git-scm.com/docs/git-config>
- direnv documentation: <https://direnv.net/>
- Hierarchical CLI config guidance: <https://rust-cli-recommendations.sunshowers.io/hierarchical-config.html>
