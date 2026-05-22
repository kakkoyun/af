# af

**af** manages isolated AI-agent workstreams across git worktrees, tmux sessions,
sandboxes, and SSH remotes. Give it a task name, and it creates a branch, a
dedicated worktree, a tmux session, and launches a primary agent (pi, claude, or
codex) — all tied together under a single durable state file. When the task is
done, everything is cleaned up with one command.

> **Status — v1 (single-user).** Stages 0–12 + Stage 14 are implemented;
> ADRs 031–067, 069, and 073 are `implementation: complete`. ADR-071 is
> `in-progress` (engine landed; multi-cmd wire-up deferred). ADRs 068,
> 070, and 072 remain `pending`. `make check` is
> green. The proxy commands (`af editor`, `af diff`, `af pr`, `af retro`),
> suspend/resume lifecycle, stack-aware `af sync`, opinionated diff
> rendering (hunk + diffity), repo-scoped `[control]` settings,
> `af control up/down/status` remote-control via Tailscale + superterm,
> slicer-only sandbox with `[sandbox.slicer.resources]` profile capture,
> slicer worktree transport (`slicer wt push/pull`) with host-worktree
> lease enforcement and `af pull`, slicer VM agent-session export +
> automatic sync (`af session-data sync|list` with append-aware JSONL
> merge and auto-sync hooks on `af suspend` / `af done`), and
> goreleaser snapshot builds are all exercised by unit + integration
> testscripts. Remote / sandbox launches go through `secret.Envelope`
> for ephemeral env-file transport. See [Caveats](#caveats) for the
> remaining single-user assumptions.

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

### Review (ADR-073)

| Command                                                                            | Description                                                                                                                                                                                                                |
| ---------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `af review [session] [--pr N] [--agent X] [--model Y] [--append-prompt T] [--skill S] [--stdout] [--out PATH]` | Generate a draft PR review report. Read-only; never posts. Embedded immutable af system prompt + four-layer append (user / repo / file / CLI) + suggested skills + PR diff. Writes `<worktree>/.af/reviews/<UTC>-pr<n>.md`. |

### `af pr --refresh` (ADR-071)

`af pr --refresh [session]` force-refreshes the cached PR state via
`gh pr view --json` without opening anything. Updates `[pr].state`,
`[pr].last_refreshed_at`, and emits a `pr_state_changed` ledger event
on a flip. Empty PR exits with `EX_DATAERR`-style error.

### Slicer VM session sync (ADR-066 + ADR-067)

| Command                                                                            | Description                                                                                                                  |
| ---------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------- |
| `af session-data sync [session] [--agent KIND] [--dry-run] [--continue-host]`     | Copy allowlisted agent transcripts (`~/.claude/projects/**`, `~/.codex/sessions/**`, `~/.pi/agent/...`, harness teams) out of the slicer VM and merge into the host home dir with SHA-256 dedup + JSONL prefix-append. Writes `[session_export]` cursors to state.toml. |
| `af session-data list [session] [--vm VM] [--agent KIND]`                          | Inventory the allowlisted session files in the VM without copying.                                                          |
| `af suspend [session] [--force] [--discard]`                                       | Auto-runs `session-data sync` before VM teardown when slicer-backed. `--discard` skips the sync and acknowledges transcript loss; `--force` is the ADR-065 lease bypass and the two compose. |
| `af done [session] [--force] [--discard]`                                          | Same auto-sync + `--discard` semantics as `af suspend`.                                                                      |

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

**`af create --remote` runs minimal SSH setup.** `PrepareRemoteWorkstream`
performs a small set of commands (mkdir + git clone) over SSH. The companion
`af control up --remote HOST` (ADR-063) is the path for attaching a remote
tmux / agent dashboard via Tailscale Serve + superterm.

**`af pr --ai` and `af retro --ai` require a non-interactive agent.** The
primary agent must support body-generation via stdin (pi `--print`, claude
non-interactive). The empty-diff / empty-output errors surface clearly.

**`[sandbox.slicer.resources]` group-shape match is optimistic.** ADR-062
resolves a managed-group name `af-<repo-slug>-<profile>` and probes
`slicer vm group` for existence. Because slicer does not expose stable
machine-readable per-group resource metadata, shape mismatches between
the configured profile and an existing group of that name are not
strictly verified; a tightening pass lands when slicer ships such an
API. See `internal/sandbox/resources.go` (`// ADR-062 §Resolution step
6`) for the exact deferral.

**Pending ADRs.** ADRs 068 (operational UX contract: flock + JSON envelope +
exit codes + completion), 070 (session resolution + fzf picker), and 072
(state.toml schema roll-up) remain `implementation: pending`. ADR-071 (PR
state TTL cache) is `in-progress`: the core engine and `af pr --refresh`
shipped; the TTL-aware wire-up into `af status` / `af info` / `af clean` /
`af sync` / `af done` is deferred to a follow-up pass.

**`af session-data sync --continue-host` is accepted but not yet wired.**
The ADR-066 host-continuation path normalization (rewriting transcript
metadata so `claude --resume` / `codex resume` / pi can find imported
sessions from the host worktree) is deferred. The flag prints a stderr
hint and falls back to the analysis-only import. Inline TODO in
`internal/sandbox/sessiondata/pull.go`.

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
| [`TODO.md`](TODO.md)                         | Implementation checklist (Stages 0–14)                                     |
| [`docs/SPEC.md`](docs/SPEC.md)               | v1 specification                                                           |
| [`docs/PLAN.md`](docs/PLAN.md)               | Implementation plan (pointer to ADR groups)                                |
| [`docs/CONVENTIONS.md`](docs/CONVENTIONS.md) | Go style, commit format, file ownership                                    |
| [`docs/adr/INDEX.md`](docs/adr/INDEX.md)     | ADR index (031–073)                                                        |
| [`docs/v0/`](docs/v0/)                       | Frozen Rust-era archive (30 ADRs, SPEC, PLAN, eleven-session PROGRESS log) |
| [`AGENTS.md`](AGENTS.md)                     | Working agreement for AI agents                                            |
| [`CLAUDE.md`](CLAUDE.md)                     | Project constitution                                                       |

## License

[MIT](LICENSE) — Copyright (c) 2026 Kemal Akkoyun
