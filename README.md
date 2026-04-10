# af

**af** (agentic-flow · automatic-flow · as-fuck) — isolated development sessions for AI coding agents.

Create a worktree, launch an agent, track everything. One command.

## Why

You're working on a repo. You want Claude (or pi, or Codex, or Gemini, or Amp) to focus on
one task without touching your main checkout. You want the branch, the worktree, and the agent
session tied together. When the PR merges, you want everything cleaned up.

`af` does that.

## Quickstart

```bash
# Install
cargo install --locked --git https://github.com/kakkoyun/af

# Check your environment
af doctor

# Create a workstream — worktree + tmux session + agent
af create fix-auth-bug

# You're now in a tmux session with Claude running.
# The worktree is at ~/Workspace/.worktrees/<repo>/fix-auth-bug/

# Add a second agent for review
af agent add --slot review --agent pi

# List active workstreams
af list

# Resume a workstream after detaching
af resume fix-auth-bug

# Done — tears down tmux, removes worktree, deletes branch
af done
```

## Commands

| Command | What it does |
|---|---|
| `af create [task]` | New workstream: worktree + mux session + agent |
| `af done [session]` | Tear down a workstream |
| `af list` | Show active workstreams |
| `af resume [session]` | Re-attach to a workstream |
| `af gc` | Clean up merged/closed workstreams |
| `af agent add` | Add another agent to the current workstream |
| `af agent stop` | Stop an agent slot |
| `af agent list` | List agents in the current workstream |
| `af editor` | Open codebase in editor (terminal or GUI) |
| `af doctor` | Check dependencies, optionally install them |
| `af config` | Show or initialize configuration |
| `af completions` | Generate shell completions (bash/zsh/fish) |
| `af session-branch` | Launch agent tied to current branch |

### `af create` options

```bash
af create fix-bug                    # Fork from main, launch default agent
af create --agent pi fix-bug         # Use pi instead of claude
af create --from develop hotfix      # Fork from a specific branch
af create --current spike            # Fork from current branch
af create --from-pr 42               # Work on an existing PR (requires gh CLI)
af create --bare review-pr           # Run agent on host worktree (no VM)
```

Planned (not yet implemented):

```bash
af create --remote fix-infra         # Agent runs on a remote VM
af create --sandbox untrusted-code   # Agent runs in a Firecracker VM
af create --yolo --sandbox fast-fix  # Skip permission prompts
```

### `af done` options

```bash
af done                    # Tear down current workstream (with confirmation)
af done fix-bug            # Tear down a named workstream
af done --force fix-bug    # Skip confirmation, force-delete unmerged branch
```

## Multi-Agent Workstreams

A single workstream can run multiple agents concurrently in separate panes:

```bash
af create implement-feature          # Claude in pane 0 (primary)
af agent add --slot review --agent pi    # pi in pane 1
af agent add --slot tests --agent codex  # Codex in pane 2
af agent list                        # Show all running agents
af agent stop review                 # Stop pi, keep others
```

All agents share the same worktree and branch. Each gets its own multiplexer pane
and independent session state.

## Three-Layer Architecture

`af` composes three independent concerns:

| Layer | What it does | Options |
|---|---|---|
| **Agent** | The AI coding agent | claude, pi, codex, gemini, amp, copilot |
| **Remote** | Where the machine lives | local (default), exe.dev, DD Workspaces |
| **Sandbox** | Isolation around the agent | none (default), slicer (Firecracker), docker (sbx) |

These compose orthogonally:

```bash
af create task                              # local + no sandbox + claude
af create --agent codex task                # local + no sandbox + codex
af create --sandbox task                    # local + slicer sandbox + claude
af create --remote task                     # exe.dev remote + no sandbox + claude
af create --remote --sandbox task           # exe.dev remote + slicer sandbox + claude
af create --remote host --agent pi task     # specific remote + no sandbox + pi
```

## Supported Agents

| Agent | Binary | Status |
|---|---|---|
| [Claude Code](https://claude.ai) | `claude` | ✅ Default |
| [pi](https://github.com/mariozechner/pi-coding-agent) | `pi` | ✅ Supported |
| [Codex](https://openai.com/codex) | `codex` | ✅ Supported |
| [Gemini CLI](https://ai.google.dev) | `gemini` | ✅ Supported |
| [Amp](https://amp.dev) | `amp` | ✅ Supported |
| [Copilot CLI](https://githubnext.com/projects/copilot-cli) | `copilot` | ✅ Supported |

## Sandbox Providers

| Provider | Binary | Isolation | Status |
|---|---|---|---|
| [Slicer](https://slicervm.com) | `slicer` | Firecracker microVM | ✅ Supported |
| [Docker AI Sandboxes](https://docs.docker.com/ai/sandboxes/) | `sbx` | microVM + Docker daemon | ✅ Supported |

## Remote Providers

| Provider | Access | Status |
|---|---|---|
| [exe.dev](https://exe.dev) | `ssh exe.dev` | ✅ Supported |
| DD Workspaces | `workspaces` CLI | Planned |

## Configuration

```bash
af config init    # Create default config at ~/.config/af/config.toml
af config show    # Show effective configuration
```

```toml
# ~/.config/af/config.toml

[general]
default_agent = "claude"
multiplexer = "tmux"
max_sessions = 10
worktree_root = "~/Workspace/.worktrees"

[branch]
prefix = "kakkoyun"
prefix_on_fork_only = true

[editor]
terminal = "nvim"
visual = ""          # auto-detect: code > zed

[lifecycle]
retention_days = 90
```

Project-level overrides go in `<repo>/.af/config.toml`.

## Installation

### From source

```bash
cargo install --locked --git https://github.com/kakkoyun/af
```

### From release binaries

Download from [GitHub Releases](https://github.com/kakkoyun/af/releases):

| Target | Description |
|---|---|
| `x86_64-unknown-linux-gnu` | Linux x86_64 (glibc) |
| `x86_64-unknown-linux-musl` | Linux x86_64 (static) |
| `aarch64-unknown-linux-gnu` | Linux ARM64 (glibc) |
| `aarch64-unknown-linux-musl` | Linux ARM64 (static) |
| `x86_64-apple-darwin` | macOS Intel |
| `aarch64-apple-darwin` | macOS Apple Silicon |

### Prerequisites

```bash
af doctor        # Shows what's missing
af doctor --fix  # Installs missing dependencies
```

Required: `git`, a terminal multiplexer (`tmux`), at least one AI agent (`claude`, `pi`, etc.)

## How It Works

```
af create fix-bug
│
├── 1. Detect repo, resolve base branch (fetch upstream/origin)
├── 2. Create git worktree at ~/Workspace/.worktrees/<repo>/fix-bug
├── 3. Create tmux session "fix-bug"
├── 4. Generate deterministic session ID (UUID v5 of repo/branch)
├── 5. Launch agent: claude --session-id <uuid>
├── 6. Write session state to ~/.local/share/af/sessions/fix-bug/
│      ├── state.toml    (live snapshot)
│      └── ledger.jsonl  (append-only event log)
└── 7. Attach to tmux session
```

## Documentation

| Resource | Description |
|---|---|
| [`docs/SPEC.md`](docs/SPEC.md) | Full specification |
| [`docs/PLAN.md`](docs/PLAN.md) | Implementation plan & architecture |
| [`docs/adr/`](docs/adr/) | Architecture Decision Records (11 ADRs) |
| [GitHub Pages](https://kakkoyun.github.io/af/) | API docs & guides *(coming soon)* |

## Development

Requires: Rust 1.85+ (edition 2024)

```bash
# With just (recommended)
just check            # fmt + clippy + test + deny (run before every commit)
just test             # Run tests
just lint             # Run clippy (pedantic)
just doc              # Generate and open rustdoc

# Without just (raw cargo)
cargo fmt --check && cargo clippy --all-targets -- -D warnings && cargo test
```

See [`AGENTS.md`](AGENTS.md) for the full working agreement (TDD workflow, code standards).

## License

[MIT](LICENSE)
