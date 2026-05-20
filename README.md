# af

**af** manages isolated AI-agent workstreams across git worktrees, tmux sessions,
sandboxes, and SSH remotes. Give it a task name, and it creates a branch, a
dedicated worktree, a tmux session, and launches a primary agent (pi, claude, or
codex) — all tied together under a single durable state file. When the task is
done, everything is cleaned up with one command.

> **Status — v1 (single-user).** Stages 0–9 are implemented; every v1 ADR
> (031–059) is marked `implementation: complete`. `make check` is green. The
> proxy commands (`af editor`, `af diff`, `af pr`, `af retro`), suspend/resume
> lifecycle, stack-aware `af sync`, and goreleaser snapshot builds are all
> exercised by integration testscripts. Remote / sandbox launches go through
> `secret.Envelope` for ephemeral env-file transport. See [Caveats](#caveats)
> for the remaining single-user assumptions and the post-v1 backlog
> (ADR-060–064).

## Installation

```bash
go install github.com/kakkoyun/af@latest
```

Or build from source:

```bash
git clone https://github.com/kakkoyun/af
cd af
make install   # installs to $GOPATH/bin
```

Requires Go 1.22+. Binaries are not published; this is a single-user tool.

## Quickstart

```bash
# One-time setup: state dirs, config, gitignore entry, shell completions
af setup

# Probe that required tools (git, tmux, pi/claude/codex) are present
af doctor

# Create a workstream called "fix-auth" on a new branch from upstream/main
af create fix-auth

# See all active workstreams
af list

# Detailed view of one workstream
af info fix-auth

# Complete and archive the workstream
af done fix-auth
```

## Commands

Global flags available on every command:

```
af [--verbose|-v] [--config PATH] [--session NAME] <command>
```

### Lifecycle

| Command                                                                                                              | Description                                                                                                                      |
| -------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------- |
| `af create [name] [--from BRANCH] [--current] [--agent NAME] [--bare] [--remote HOST] [--sandbox PROVIDER] [--yolo]` | Create a workstream: new branch, git worktree, `state.toml`, ledger, optional Obsidian note, tmux session, primary-agent launch. |
| `af done [session] [--force]`                                                                                        | Tear down and archive a workstream. `--force` marks it abandoned rather than completed.                                          |
| `af suspend [session]`                                                                                               | Record suspension in state; tmux stays alive.                                                                                    |
| `af resume [session] [--bare]`                                                                                       | Resume a suspended workstream; respawns the tmux session if it died.                                                             |
| `af session-branch`                                                                                                  | Create an ad-hoc workstream branch in the current checkout without a separate worktree.                                          |

### Multi-agent

| Command                                                      | Description                                                                                  |
| ------------------------------------------------------------ | -------------------------------------------------------------------------------------------- |
| `af agent add --slot NAME --agent PROVIDER [--session NAME]` | Add an agent slot; creates a sibling sub-worktree on a sibling branch for non-primary slots. |
| `af agent stop SLOT [--remove-worktree] [--session NAME]`    | Mark a slot stopped; optionally remove its sub-worktree and branch.                          |
| `af agent list [--session NAME]`                             | List agent slots and their status.                                                           |

### Inspection

| Command                                       | Description                                                         |
| --------------------------------------------- | ------------------------------------------------------------------- |
| `af list`                                     | One-line summary per workstream.                                    |
| `af status [--json] [--all] [--filter STATE]` | Dashboard view; `--all` includes completed and abandoned.           |
| `af info [session] [--json] [--ledger N]`     | Detailed state view; `--ledger N` appends the last N ledger events. |

### Reaping

| Command                                                                     | Description                                                                        |
| --------------------------------------------------------------------------- | ---------------------------------------------------------------------------------- |
| `af clean [--dry-run] [--include-abandoned] [--max-age DURATION] [--force]` | Remove state dirs for terminal workstreams. `--max-age` accepts `7d`, `2w`, `24h`. |

### Stacking

| Command                              | Description                                                                                           |
| ------------------------------------ | ----------------------------------------------------------------------------------------------------- |
| `af stack [session] --parent PARENT` | Link this workstream as a child in the stack model ([ADR-059](docs/adr/059-stack-aware-branches.md)). |
| `af unstack [session]`               | Remove the stack parent link.                                                                         |
| `af sync [session]`                  | Rebase/fast-forward onto parent's current head (stub — full implementation pending).                  |

### Environment / setup

| Command                                                                      | Description                                                                                                |
| ---------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| `af setup [--force] [--shell SHELL] [--skip-completions] [--skip-gitignore]` | One-shot user-scope setup: state dirs, config file, global gitignore entry, shell completions. Idempotent. |
| `af doctor [--remote HOST] [--verbose]`                                      | Probe required tools locally or on an SSH host; print install hints. Never auto-installs.                  |

### Notes / Obsidian

| Command                                                                       | Description                                                                       |
| ----------------------------------------------------------------------------- | --------------------------------------------------------------------------------- |
| `af note [session] --append TEXT`                                             | Append a structured note event to the workstream ledger.                          |
| `af retro [--since DURATION] [--tag TAG] [--search QUERY] [--limit N] [--ai] [--ai-model MODEL]` | Mine archived workstream notes; `--ai` synthesises a narrative via the primary agent's `BodyCmd`. |

### Proxy commands

These commands run the user-configured executables from `[diff]`, `[pr]`, and
`[editor]` in `~/.config/af/config.toml` with token substitution
(`{base}`, `{head}`, `{worktree}`, `{title}`, `{body}`).

| Command                                                                   | Description                                                              |
| ------------------------------------------------------------------------- | ------------------------------------------------------------------------ | ----------------------------------------------------------- |
| `af diff [session] [--base REF]`                                          | Run the configured diff command in the workstream worktree.              |
| `af pr [session] [--title T] [--draft] [--web] [--ai] [--ai-model MODEL]` | Run the PR-create command; `--ai` builds the body from the worktree diff via `agent.BodyCmd` (rejects `--ai` + `--web`). |
| `af editor [session] [--terminal                                          | -t] [--visual]`                                                          | Open the configured editor at the workstream worktree path. |

### Secrets

Backed by `zalando/go-keyring` (macOS Keychain / Linux Secret Service).

| Command             | Description                                                                                |
| ------------------- | ------------------------------------------------------------------------------------------ |
| `af auth set KEY`   | Store a credential (prompts with echo-off on TTY, reads stdin otherwise).                  |
| `af auth get KEY`   | Print a credential (plain on TTY, redacted as `[REDACTED:abcd...]` otherwise).             |
| `af auth status`    | Show the curated trio (`anthropic_api_key`, `openai_api_key`, `github_token`) plus extras. |
| `af auth clear KEY` | Remove a credential from the keyring.                                                      |
| `af auth list`      | List all stored key names (no values).                                                     |

### Config + completions

| Command                                        | Description                                                                              |
| ---------------------------------------------- | ---------------------------------------------------------------------------------------- |
| `af config init`                               | Write the annotated config template to `~/.config/af/config.toml`. Refuses to overwrite. |
| `af config show`                               | Print the effective merged configuration as canonical TOML.                              |
| `af completions <bash\|zsh\|fish\|powershell>` | Emit a shell completion script to stdout.                                                |

### Meta

```bash
af version
```

## Configuration

`af` uses a three-layer TOML configuration per [ADR-036](docs/adr/036-configuration-toml-layered.md):

1. Compiled defaults (agent `pi`, multiplexer `tmux`, worktree root `~/Workspace/.worktrees`).
2. User config — `~/.config/af/config.toml`. Created by `af config init` or `af setup`.
3. Repo config — `<repo>/.af/config.toml` (optional per-project overrides; no `[obsidian.vaults]`).

```bash
# Scaffold the user config with all sections and comments:
af config init

# Inspect the effective merged config:
af config show
```

Key sections:

- `[general]` — default agent, multiplexer, max sessions, worktree root.
- `[branch]` — branch prefix and `prefix_on_fork_only` gate.
- `[diff]` / `[pr]` / `[editor]` — proxy command shapes (argv or shell mode).
- `[obsidian]` — vault paths, notes folder, template path.
- `[secret]` — keyring service name and extra redact keys.

## Caveats

**Single-user.** `af` is a personal tool. There is no auth layer, no multi-user
session sharing, and no remote API.

**`af create --sandbox` not yet end-to-end.** `lifecycle.LaunchSandboxWorkstream`
is implemented and unit-tested (including envelope write/delete), but
`af create --sandbox PROVIDER` currently prints a deferred-launch notice rather
than invoking the orchestrator. Wiring the CLI to the orchestrator is tracked
under ADR-060 (slicer-only cleanup).

**`af create --remote` runs minimal SSH setup.** `PrepareRemoteWorkstream`
performs a small set of commands (mkdir + git clone) over SSH. It is not yet
batched against a real remote tmux/agent boot — that landed will be wired when
ADR-063 (Tailscale + superterm) lands.

**`af pr --ai` and `af retro --ai` require a non-interactive agent.** The
primary agent must support body-generation via stdin (pi `--print`, claude
non-interactive). The empty-diff / empty-output errors surface clearly.

## Building

```bash
make build          # ./bin/af
make check          # lint + race test
make release-snapshot  # cross-compile snapshot via goreleaser
```

## Documentation

| Resource                                     | Description                                                                |
| -------------------------------------------- | -------------------------------------------------------------------------- |
| [`CHANGELOG.md`](CHANGELOG.md)               | Full feature history (`[Unreleased]` for v1)                               |
| [`PROGRESS.md`](PROGRESS.md)                 | Narrative session log                                                      |
| [`TODO.md`](TODO.md)                         | Implementation checklist (Stages 0–8)                                      |
| [`docs/SPEC.md`](docs/SPEC.md)               | v1 specification                                                           |
| [`docs/PLAN.md`](docs/PLAN.md)               | Implementation plan (pointer to ADR groups)                                |
| [`docs/CONVENTIONS.md`](docs/CONVENTIONS.md) | Go style, commit format, file ownership                                    |
| [`docs/adr/INDEX.md`](docs/adr/INDEX.md)     | ADR index (031–059)                                                        |
| [`docs/v0/`](docs/v0/)                       | Frozen Rust-era archive (30 ADRs, SPEC, PLAN, eleven-session PROGRESS log) |
| [`AGENTS.md`](AGENTS.md)                     | Working agreement for AI agents                                            |
| [`CLAUDE.md`](CLAUDE.md)                     | Project constitution                                                       |

## License

[MIT](LICENSE) — Copyright (c) 2026 Kemal Akkoyun
