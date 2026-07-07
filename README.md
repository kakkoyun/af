# af

**af** manages isolated AI-agent workstreams across git worktrees, tmux sessions,
sandboxes, and SSH remotes. Give it a task name, and it creates a branch, a
dedicated worktree, a tmux session, and launches a primary agent (pi, claude, or
codex) — all tied together under a single durable state file. When the task is
done, everything is cleaned up with one command.

> **Status — v1 (single-user).** Stages 0–14 are implemented and all
> v1 ADRs are closed: ADRs 031–073 are `implementation: complete`
> (ADR-032 is `implementation: n/a`). Stage 15 is release prep.
> `make check` is
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
| `af done [session] [--force] [--discard]`                                                                            | Tear down and archive a workstream. `--force` marks it abandoned; `--discard` skips the ADR-067 automatic transcript sync.      |
| `af suspend [session] [--force] [--discard]`                                                                         | Record suspension in state; tmux stays alive. `--force` overrides a held slicer lease; `--discard` skips the automatic sync.    |
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

| Command                                                                                  | Description                                                                        |
| ------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------- |
| `af clean [--dry-run] [--include-abandoned] [--max-age DURATION] [--force] [--discard]` | Remove state dirs for terminal workstreams. `--max-age` accepts `7d`, `2w`, `24h`. For each VM-leased target, auto-runs the ADR-067 `session-data sync` before removal; a sync failure skips (keeps) that target and makes `clean` exit non-zero, but other targets in the same run still get reaped. `--discard` skips the sync and acknowledges transcript loss. `--dry-run` prints `would sync + remove NAME` for leased targets. |

### Stacking

| Command                              | Description                                                                                           |
| ------------------------------------ | ----------------------------------------------------------------------------------------------------- |
| `af stack [session] --parent PARENT` | Link this workstream as a child in the stack model ([ADR-059](docs/adr/059-stack-aware-branches.md)). |
| `af unstack [session]`               | Remove the stack parent link.                                                                         |
| `af sync [session]`                  | Fetch the parent (when an origin exists) and rebase this branch onto its current head; on conflict git is left mid-rebase for manual resolution. |

### Environment / setup

| Command                                                                      | Description                                                                                                |
| ---------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| `af setup [--force] [--shell SHELL] [--skip-completions] [--skip-gitignore]` | One-shot user-scope setup: state dirs, config file, global gitignore entry, shell completions. Idempotent. |
| `af doctor [--remote HOST] [--verbose]`                                      | Probe required tools locally or on an SSH host; print install hints. Never auto-installs.                  |
| `af doctor --all [--report] [--report-dir DIR] [--issue]`                     | Host self-smoke (ADR-074): run real af commands in an isolated scratch HOME, clean up, and summarise. `--report` writes paste-ready markdown + JSON; `--issue` files failures on GitHub via `gh`. Exits non-zero when any step fails. |

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
| ------------------------------------------------------------------------- | ------------------------------------------------------------------------ |
| `af diff [session] [--base REF] [--web] [--interactive]`                  | Run the configured diff command in the workstream worktree; `--web` opens the range via diffity (ADR-064). |
| `af pr [session] [--title T] [--body B] [--draft] [--web] [--ai] [--ai-model MODEL] [--refresh]` | Run the PR-create command; `--ai` builds the body from the worktree diff via `agent.BodyCmd` (rejects `--ai` + `--web`); `--refresh` force-refreshes the cached PR state (ADR-071) without opening anything. |
| `af editor [session] [--terminal\|-t] [--visual]`                          | Open the configured editor at the workstream worktree path.              |




### State schema roll-up (ADR-072)

`state.toml` remains `schema_version = 1`. The canonical consolidated
schema is in ADR-072 and includes the Stage 12/13 additions:
`[session_export]` with `[[session_export.sources]]`, PR cache fields
`last_refreshed_at` / `last_refresh_error`, `[slicer_wt]`, stack,
control, and slicer resource capture fields.

### Operational UX contracts (ADR-068)

- `--json` commands emit a versioned envelope: `{ "schema": 1, "data": ... }`.
- `af` maps common failure classes to the full sysexits-style exit-code
  table (`EX_OK` 0, `EX_GENERAL` 1, `EX_USAGE_COBRA` 2, `EX_USAGE` 64,
  `EX_DATAERR` 65, `EX_NOINPUT` 66, `EX_UNAVAILABLE` 69, `EX_SOFTWARE`
  70, `EX_TEMPFAIL` 75, `EX_NOPERM` 77, `EX_INTERRUPTED` 130). A missing
  external tool (`gh`, `slicer`, ...) exits `EX_UNAVAILABLE`; a
  lock-acquisition timeout exits `EX_TEMPFAIL` with a retry hint; a
  caught internal panic exits `EX_SOFTWARE` after printing the panic
  and a stack trace to stderr.
- Mutating commands acquire a per-session `.af.lock` (exclusive flock)
  across their full read-modify-write span via `session.WithLock` /
  `session.Update`. Acquisition is bounded: it retries for up to
  `AF_LOCK_TIMEOUT` (default 30s) before failing with `EX_TEMPFAIL`.
- Completions include workstream names for `[session]` / `--session` and
  lifecycle states for `af status --filter`.

### Session resolution (ADR-070)

Every command that accepts `[session]` resolves it in this order:
positional arg → root `--session NAME` (warns when overriding a
positional arg) → `AF_SESSION` → cwd `.af/state.toml` discovery symlink
(walking up parent directories) → interactive `fzf` picker when stdin and
stderr are TTYs → a deterministic `EX_NOINPUT`-style error with recovery
hints. `af create` sets `AF_SESSION` in the tmux session environment so
agents launched inside af panes can run commands like `af note --append`
without repeating the session name.

### Review (ADR-073)

| Command                                                                            | Description                                                                                                                                                                                                                |
| ---------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `af review [session] [--pr N] [--agent X] [--model Y] [--append-prompt T] [--skill S] [--stdout] [--out PATH]` | Generate a draft PR review report. Read-only; never posts. Embedded immutable af system prompt + four-layer append (user / repo / file / CLI) + suggested skills + PR diff. Writes `<worktree>/.af/reviews/<UTC>-pr<n>.md`. |

### `af pr --refresh` (ADR-071)

`af status --refresh`, `af info --refresh`, and `af pr --refresh [session]`
force-refresh cached PR state via `gh pr view --json` without opening
anything. `af status` / `af info` otherwise refresh outside the configured
`[pr].refresh_ttl` window. Correctness-critical commands (`af clean`,
`af sync`, `af done`) always force-refresh before acting. Updates
`[pr].state`, `[pr].last_refreshed_at`, and emits `pr_state_changed` on a
flip. `af pr --refresh` with no PR exits with `EX_DATAERR`-style error.

### Slicer VM session sync (ADR-066 + ADR-067)

| Command                                                                            | Description                                                                                                                  |
| ---------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------- |
| `af session-data sync [session] [--agent KIND] [--dry-run] [--continue-host]`     | Copy allowlisted agent transcripts (`~/.claude/projects/**`, `~/.codex/sessions/**`, `~/.pi/agent/...`, harness teams) out of the slicer VM and merge into the host home dir with SHA-256 dedup + JSONL prefix-append. Writes `[session_export]` cursors to state.toml. |
| `af session-data list [session] [--vm VM] [--agent KIND]`                          | Inventory the allowlisted session files in the VM without copying.                                                          |
| `af pull [session]`                                                                | Run `slicer wt pull`: import VM branches under `refs/slicer/<vm>/*`, fast-forward the host branch, release the host-worktree lease. Requires `lease_state=held_by_vm`. |
| `af suspend [session] [--force] [--discard]`                                       | Auto-runs `session-data sync` before VM teardown when slicer-backed. `--discard` skips the sync and acknowledges transcript loss; `--force` is the ADR-065 lease bypass and the two compose. |
| `af done [session] [--force] [--discard]`                                          | Same auto-sync + `--discard` semantics as `af suspend`.                                                                      |
| `af clean [--force] [--discard] [...]`                                             | Same auto-sync before removal for any VM-leased target it reaps; `--discard` skips the sync. A sync failure keeps that target's state dir and fails the command, without blocking removal of other targets. |

### Remote control (ADR-063)

| Command                                                        | Description                                                                                       |
| -------------------------------------------------------------- | ------------------------------------------------------------------------------------------------- |
| `af control up [--port N] [--provider P] [--remote HOST]`      | Start the remote-control helper: superterm tmux web UI exposed over the tailnet via Tailscale Serve. |
| `af control down [--remote HOST]`                              | Stop remote control: remove the Tailscale Serve mapping and stop superterm.                        |
| `af control status [--json] [--remote HOST]`                   | Report remote-control status.                                                                      |

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

**ADR status.** ADRs 031–073 are ratified for v1 (`status: accepted`,
`implementation: complete`, except ADR-032 which is `n/a`). Per the
constitution, `docs/SPEC.md` and `docs/PLAN.md` are frozen; design
changes go through new ADRs (074+).

**`af session-data sync --continue-host` rewrites staged transcripts
before merge.** Per ADR-066 §Host continuation mode, `--continue-host`
runs a per-kind rewrite over the staged copy (before the merge/dedup
step) using `state.SlicerWT.Path` as the VM-side workspace path and
`state.Worktree.Path` as the host-side path:

- **claude**: renames the staged `~/.claude/projects/<vm-slug>`
  directory to `~/.claude/projects/<host-slug>` (the Claude Code
  project-directory slug is the workspace path with `/` and `.`
  replaced by `-`), then rewrites `cwd`-style path fields inside its
  `*.jsonl` records so the host destination is discoverable by `claude
  --resume` / `/resume` from the host worktree.
- **codex**: rewrites `cwd`-style path fields inside
  `~/.codex/sessions/**/*.jsonl` in place (no rename — Codex keys
  sessions by date + rollout ID, not by workspace path).
- **pi**: pi's on-disk session-index format is not reverse-engineered
  in this codebase; as a conservative fallback, exact `vmPath` string
  occurrences inside `~/.pi/agent/sessions/**/*.json(l)` are rewritten
  in place.
- **harness**: no host-continuation rewrite is defined; harness files
  import for analysis only.

The rewrite only ever touches whole JSON string values equal to, or
prefixed by, the VM path — unrecognized fields and non-JSON lines
round-trip untouched. Normalization runs before the SHA-256 dedup step,
so re-running `--continue-host` against unchanged VM content imports 0
new files. `--dry-run --continue-host` cannot inspect file content
(ADR-066: dry-run never copies) and instead reports per-kind candidate
file counts from the manifest alone. See `internal/sandbox/sessiondata/normalize.go`.

**Caveat:** `state.SlicerWT.Path` records the same `<worktree-path>`
argument `slicer wt push`/`pull` operate on (ADR-065), which today is
the *host* worktree lease path — in the common case it is identical to
`state.Worktree.Path`, so normalization is a safe no-op unless the two
diverge. `af` does not currently track a distinct VM-internal working
directory in `state.toml`; if a future slicer convention places the
VM-side clone at a different path, `--continue-host` needs that path
threaded through before it can rewrite anything in practice.

## Building

```bash
make build          # ./bin/af with build metadata ldflags
make install        # build first, warn if dirty, then go install ./cmd/af
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
