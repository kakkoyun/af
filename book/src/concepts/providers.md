# Three-Layer Architecture

`af` composes three independent concerns. You can mix and match any combination.

## The three layers

| Layer | What it controls | Default | Options |
|---|---|---|---|
| **Agent** | The AI coding agent that runs inside the session | `claude` | claude, pi, codex, gemini, amp, copilot |
| **Remote** | Where the machine lives | local host | local, exe.dev, DD Workspaces |
| **Sandbox** | Isolation around the agent | none | none, slicer (Firecracker), docker (sbx) |

## How they compose

The layers are orthogonal. Any combination works:

```bash
af create task                              # local + no sandbox + claude
af create --agent codex task                # local + no sandbox + codex
af create --sandbox task                    # local + slicer sandbox + claude
af create --remote task                     # exe.dev remote + no sandbox + claude
af create --remote --sandbox task           # exe.dev remote + slicer sandbox + claude
af create --remote host --agent pi task     # specific remote host + no sandbox + pi
```

## Agent layer

The agent is the AI process that writes code. af launches the agent binary inside
the multiplexer session and passes it the worktree path and session ID.

Select an agent with `--agent`:

```bash
af create --agent gemini refactor-api
```

The default agent is configured in `~/.config/af/config.toml` under
`[general] default_agent`. See [Configuration](../commands/config.md).

Available agents: `claude`, `pi`, `codex`, `gemini`, `amp`, `copilot`. Each
has a dedicated CLI binary and a distinct set of native flags; `af` maps the
[approval mode](approval-modes.md) to each agent's equivalent.

## Remote layer

By default the agent runs on your local machine. `--remote` shifts it to a remote VM.

```bash
af create --remote fix-infra      # uses configured default remote (exe.dev)
af create --remote myhost task    # ssh target: myhost
```

Supported remotes: [**exe.dev**](https://exe.dev) (SSH-reachable VMs, full
SSH bootstrap pipeline) and **DD Workspaces** (internal to Datadog; managed
via the `workspaces` CLI). Provider selection is config-driven under
`[remote]`.

## Sandbox layer

`--sandbox` wraps the agent in a Firecracker microVM (via slicer) for filesystem
and process isolation. Useful for untrusted code or `--yolo` runs.

```bash
af create --sandbox risky-refactor
af create --yolo --sandbox fully-automated
```

Two sandbox providers ship in 0.1.0:

- **Slicer** — Firecracker-based microVMs, `--sandbox`. Heavier, best
  isolation.
- **Docker AI Sandboxes** (via `sbx`) — container-based, fast. Selected
  via config `[sandbox] provider = "docker"`.

Per-agent OS-level sandboxes (codex's Seatbelt/bwrap profile) are a separate
knob: `--agent-sandbox=os`. Orthogonal to the VM sandbox above.
