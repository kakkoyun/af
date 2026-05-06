# Approval Modes

AI coding agents differ in how they ask for permission. Some prompt for every
tool invocation; some auto-approve safe edits but still confirm destructive
commands; some run without prompting at all. `af` collapses this spectrum into
three levels — **Default**, **Auto**, **Yolo** — and maps each to the agent's
native flags.

## The three levels

| Mode | CLI flag | Intent |
|---|---|---|
| `Default` | *(none)* | Prompt the user on tool use. The agent's out-of-the-box behaviour. |
| `Auto` | `--auto` | Auto-approve read/edit operations; still prompt for destructive ones (shell, deletes). |
| `Yolo` | `--yolo` | Skip every permission prompt. Reserved for sandboxed or unattended runs. |

```bash
af create task                    # Default — prompts live
af create --auto task             # Auto — approves edits silently
af create --yolo --sandbox task   # Yolo — fully autonomous, inside a VM
```

## Per-agent mapping

Each agent implements approval differently. `af` translates your mode choice
to the agent's native flag at launch time.

| Agent | `Default` | `Auto` | `Yolo` |
|---|---|---|---|
| **Claude** | `--permission-mode default` | `--permission-mode auto` | `--dangerously-skip-permissions` |
| **Codex** | `--ask-for-approval on-request` | `--ask-for-approval on-request` | `--full-auto --ask-for-approval never` |
| **Gemini** | `--approval-mode default` | `--approval-mode auto_edit` | `--yolo` |
| **Amp** | *(bare)* | *(bare — falls through to Default)* | `--dangerously-allow-all` |
| **Copilot** | *(bare)* | `--allow-all-tools` | `--allow-all --autopilot` |
| **Pi** | *(bare)* | *(bare)* | *(not supported — falls through)* |

### Graceful degradation

Agents that don't support a given mode fall through to the closest available
option rather than erroring:

- **Pi** has no approval modes; all three levels invoke `pi` bare.
- **Amp** has no "auto" middle ground; `Auto` behaves like `Default`.

A one-time `tracing::info!` announces the degradation on session launch so you
aren't surprised.

## Which one should you pick?

- **Default** — when you're watching the session interactively and want to
  review each action.
- **Auto** — when you trust the agent with file edits but want to stay in the
  loop for anything destructive. Good for supervised work.
- **Yolo** — when the session runs unattended (overnight, CI, background
  task) AND is sandboxed (`--sandbox` for a VM, or `--agent-sandbox=os` for
  agent-level OS isolation). `af` warns when `--yolo` runs without any
  sandbox active.

## Related

- [Three-Layer Architecture](providers.md) — how agent, remote, and sandbox
  layers compose.
- [`create`](../commands/create.md) — flag reference.
- ADR-012 (Tri-State Approval Mode) — the design rationale.
- ADR-028 (Agent-Level OS Sandbox) — per-agent OS sandboxing orthogonal to
  this approval axis.
