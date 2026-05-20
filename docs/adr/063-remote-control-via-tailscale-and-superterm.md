---
adr: 063
title: "Remote Control via Tailscale Serve and superterm"
status: proposed
implementation: pending
date: 2026-05-20
last_modified: 2026-05-20
supersedes: []
superseded_by: null
related: ["035", "036", "040", "041", "044", "061"]
tags: ["go", "remote", "tailscale", "superterm", "tmux"]
---

# ADR-063: Remote Control via Tailscale Serve and superterm

## Context

ADR-041 intentionally models remote execution as SSH plus remote tmux.
That is enough for durable sessions, but it is not enough for the
owner's newer workflow: many long-running agents may be waiting for
permission, stuck, or finished while the owner is away from the terminal.
The requested capability is remote control and status from another
machine or phone.

External research points to a feasible composition rather than a new
`af` web UI:

- Tailscale Serve exposes a local HTTP service to devices in the same
  tailnet, with HTTPS URLs and access controlled by tailnet identity and
  ACLs. Tailscale Funnel can publish to the wider internet, but Serve is
  the safer default for this project.
- superterm is a browser/PWA dashboard over tmux sessions for agentic
  multitasking. It advertises tmux session status, attention indicators,
  mobile unblocking, agent-agnostic support, local computation, and no
  uploaded session data. It is a separate commercial product for
  non-personal/commercial use.

The feasible architecture is therefore: keep tmux as the execution
substrate, run superterm beside tmux on the machine that owns the
sessions, and use Tailscale Serve to expose superterm to the owner's
tailnet over HTTPS.

## Decision

`af` will support an optional **remote-control helper** that composes
superterm and Tailscale Serve. `af` does not implement a browser terminal
or agent dashboard itself.

### Control provider

`superterm` is the only remote-control provider in v1.

Repo/user config may request it through ADR-061:

```toml
[control]
remote_control = "superterm" # or "" / "off"
```

### CLI surface

This ADR adds a small control command group. ADR-035 remains historical
until a future command-surface batch supersedes it.

```text
af control up [session] [--remote HOST] [--provider superterm] [--port PORT] [--json]
af control down [--remote HOST]
af control status [--remote HOST] [--json]
```

`af control up` means **start / ensure remote control is available**. It
is host-level because superterm watches tmux at the host level, not just
one workstream. The optional `session` argument and root `--session` flag
may be accepted for discovery, but they do not scope the superterm UI to
one session.

### `af control up`

For a local host, `af control up`:

1. Probes `tmux`, `superterm`, and `tailscale`.
2. Starts or verifies the superterm UI server (`superterm up`) on the
   local machine.
3. Enables that local superterm endpoint through Tailscale Serve:

   ```text
   tailscale serve --bg <local-superterm-port-or-url>
   ```

4. Prints the resulting tailnet HTTPS address that reaches the superterm
   UI, for example:

   ```text
   Superterm UI: https://<node>.<tailnet>.ts.net/
   ```

5. Records the control endpoint metadata in local state.

For a remote workstream, `af control up --remote <host>` runs the same
steps over SSH on `<host>` because that host owns the tmux server and
superterm must observe local tmux sessions there.

### Binding and exposure rules

- superterm must bind to localhost or an equivalent private interface
  before Tailscale Serve exposes it.
- Tailscale Serve is the default and only automatic exposure mode.
- Tailscale Funnel (public internet) is out of scope for v1. If added
  later, it must require an explicit scary flag and a new ADR.
- `af` must not copy Tailscale auth keys, superterm license keys, or any
  browser session tokens into repo config or state.
- `af doctor --remote <host>` should report whether `tailscale status`
  works and whether HTTPS certificates are enabled for Serve.

### Parsing superterm and Tailscale startup

`superterm up` and `tailscale serve` are external CLIs. `af` should
prefer stable machine interfaces if either tool provides them. If the
installed versions only print human URLs, `af` may parse those URLs
conservatively. If parsing fails, `af control up` prints manual commands
instead of guessing. A successful start must end by printing an address
the user can open to reach the superterm UI.

### Teardown

`af control down` removes only the Tailscale Serve mapping and stops the
superterm helper if `af` started it. It never kills agent tmux sessions.
`af done` for a workstream does not automatically tear down host-level
remote control unless no other active workstreams remain on that host and
`af` owns the helper process.

## Consequences

### Pros

- Remote/mobile control arrives without building or securing a custom web
  terminal in `af`.
- Tailscale Serve keeps access inside the owner's tailnet by default and
  gives browsers HTTPS URLs.
- superterm is tmux-native, matching ADR-040's tmux-only multiplexer.
- The model works for local workstations, headless Linux boxes, mini PCs,
  and SSH remote hosts.
- `af` remains an orchestrator of existing tools, not a terminal server.

### Cons / risks

- Adds two external operational dependencies (`tailscale` and
  `superterm`) for users who enable remote control.
- Browser access to a terminal dashboard is high-impact; a compromised
  tailnet device may be able to control agents.
- Tailscale Serve configuration is machine-level and can conflict with
  manually configured Serve routes.
- superterm licensing may matter for commercial/professional use.
- The integration depends on superterm CLI stability. `af` must degrade
  to instructions instead of brittle automation when output changes.
- Mobile control is for unblocking and checking status, not full editing;
  workflows should still prefer a real terminal for heavy work.

## Alternatives Considered

- **Build an `af` web UI.** Rejected. It creates an authentication,
  authorization, terminal-emulation, and browser-security project far
  beyond v1 scope.
- **SSH only.** Rejected as the only answer. SSH remains the durable
  control plane, but it does not solve phone/tablet attention checks.
- **Tailscale Funnel by default.** Rejected. Public internet exposure is
  unnecessary and riskier than tailnet-only Serve.
- **Use a generic terminal-sharing service instead of superterm.**
  Rejected for v1. The requested product is superterm, and its tmux /
  agent attention model matches the project better than generic sharing.
- **Run superterm locally against remote tmux over SSH.** Rejected.
  superterm should run where tmux runs so disconnects do not affect
  monitoring and so remote sessions are visible without SSH tunnels.

## References

- ADR-035 — command surface; this ADR adds future `af control` commands.
- ADR-036 — config file location.
- ADR-040 — tmux-only multiplexer.
- ADR-041 — SSH remote model.
- ADR-044 — doctor probes for external tools.
- ADR-061 — repo-scoped `remote_control` setting.
- Tailscale Serve docs: <https://tailscale.com/docs/features/tailscale-serve>
- Tailscale Serve CLI reference: <https://tailscale.com/docs/reference/tailscale-cli/serve>
- Tailscale Serve examples: <https://tailscale.com/docs/reference/examples/serve>
- superterm: <https://superterm.dev/>
- superterm pricing / license notes: <https://superterm.dev/pricing/>
