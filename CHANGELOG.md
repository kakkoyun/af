# Changelog

All notable changes to `af` (v1) are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

> **v0 history.** The Rust implementation's changelog is archived at
> [`docs/v0/CHANGELOG.md`](docs/v0/CHANGELOG.md). v1 starts fresh — no
> version-number continuity is implied.

---

## [Unreleased]

### Added

#### Stage 12 — ADR-066 + ADR-067 slicer VM agent-session sync

- **ADR-066** (VM agent-session export): new `af session-data sync
  [session]` and `af session-data list [session]` commands. The sync
  command copies allowlisted transcripts (`~/.claude/projects/**`,
  `~/.codex/sessions/**`, pi `sessionDir`, harness `~/.pi/agent/teams`)
  out of the slicer VM via `slicer vm exec` + `slicer vm cp --mode=tar`
  and merges into the matching host directories.
- **SHA-256 dedup + conflict quarantine**: identical files are skipped;
  divergent files are routed to `<staging>/conflicts/` rather than
  overwriting host state. Imports use `0o600` files and `0o700` parent
  dirs per the ADR-066 privacy contract.
- **Append-aware JSONL merge** (ADR-067 §Latest-sync merge rules):
  when a `*.jsonl` destination is a byte-for-byte prefix of the VM
  source, sync appends only the missing tail to the existing host
  file. Divergent or shrunken JSONLs still quarantine.
- **ADR-067** state schema: `state.toml.[session_export]` records
  `last_sync_at`, `last_sync_status` (never/ok/blocked/discarded),
  `last_manifest` (staging path), and per-file `[[session_export.sources]]`
  cursors with `agent`, `vm`, `source_path`, `dest_path`, `mode`
  (copy or append-jsonl), `hash`, `size`, `last_offset`, `mtime`, and
  `status`. Empty sessions omit the section entirely.
- **`agent_sessions_synced` ledger event** captures every sync attempt
  with kinds + imported/skipped/conflict counts.
- **Auto-sync hooks on lifecycle boundaries** (ADR-067 §Lifecycle rule):
  `af suspend` and `af done` now run `session-data sync` for any
  slicer-backed workstream before the destructive step. A failed or
  conflicting sync blocks teardown and prints a recovery hint pointing
  to `af session-data sync <name>` or `--discard`. The new `--discard`
  flag on both commands acknowledges transcript loss and records
  `last_sync_status=discarded` in state.toml.
- **`af doctor` wt API probe** (ADR-065 carry-over): slicer's
  `wt push --help --launch` is consulted when the slicer probe finds
  the binary; a missing `--launch` flag surfaces as a non-blocking
  warning sub-line.
- **`TestEditor_LeaseWarning` + editorCommand seam** (ADR-065
  carry-over): the lease warning path is now covered without spawning
  a real editor.
- **Pre-existing bug fix in `internal/session/ledger_tail.go`**: the
  writer wrote `Event.Type` to JSON key `"event"` but the parser only
  matched `"type"`, so round-tripped events lost their Type. Fixed
  the parser to accept both keys; `TestLedger_EventTypeRoundTrip`
  regression guard added. No on-disk format change.

Deferrals carried into Stage 13/14 (called out inline in code):

- `af session-data sync --continue-host` path-normalization (per-agent
  format knowledge).
- `af clean --force` ADR-067 hook (clean's slicer-VM interaction is
  uncommon; adding when the reaper learns about VMs).

#### Stage 11 — ADR-065 slicer worktree transport (Session 29)

- **ADR-065** (slicer worktree transport): `af create --sandbox slicer`
  now invokes `slicer wt push --launch [--hostgroup G] [--depth N]
  --tag af --tag af-session=NAME <worktree-path>` instead of the
  earlier `slicer vm run` (which mounted the host worktree). The VM
  receives a sanitised, self-contained `.git` clone; the host worktree
  is **leased to the VM** while the VM holds it.
- New `af pull [session]` command runs `slicer wt pull <vm>
  <worktree-path>`, fast-forwards the host branch, and releases the
  lease (`lease_state: pulled`, `pulled_at` stamped).
- Lease enforcement across destructive commands:
  - `af done` and `af suspend` refuse on `held_by_vm` unless `--force`
    is passed (in which case the lease is marked `discarded`).
  - `af pr` refuses outright on `held_by_vm` because the host branch
    may not yet contain the VM's commits.
  - `af diff` and `af editor` print a stderr warning suggesting
    `af pull` but do not refuse.
  - `af status` and `af info` surface `vm=<name> lease=<state>` in
    both text and JSON output.
- New `internal/sandbox/slicerwt.go` with `WTPush`/`WTPull` operations,
  permissive VM-name parser (matches "Launched VM <name>" / "VM:
  <name>" with a last-word fallback), sentinels `ErrSlicerWTPushFailed`,
  `ErrSlicerWTPullFailed`, `ErrSlicerWTNameNotFound`.
- Additive `state.toml` schema: new `[slicer_wt]` section with `vm`,
  `path`, `pushed_at`, `pulled_at`, `lease_state`
  (`held_by_vm`/`pulled`/`discarded`). `State.IsLeasedToVM()` helper.
- New `internal/lifecycle/pull.go` orchestrator with refusal sentinels
  `ErrPullNoLease`, `ErrPullAlreadyPulled`, `ErrPullDiscarded`,
  `ErrPullFailed`.
- `internal/doctor/system.go` gains `SlicerWTAvailable` probe that runs
  `slicer wt push --help` and confirms `--launch` is documented; wired
  into the doctor report as a non-blocking warning per the ADR.
- 1 ADR advanced to `implementation: complete` (065); every ADR from
  031 to 065 is now `complete`.

#### Stage 10 — post-v1 ADRs 060–064 (Session 27–28)

- **ADR-060** (slicer-only sandbox): dropped the Docker `sbx` provider
  end-to-end. New `sandbox.NewProvider(name) (Sandbox, error)` factory.
  `SBXConfig` deleted; `[sandbox] provider = "sbx"` is now a parse
  error. `cmd/af/create.go` now invokes `lifecycle.LaunchSandboxWorkstream`
  with a real `slicer vm run --name … --mount … -- <agent argv>`
  invocation; the previous "sandbox launch is performed at agent start"
  diagnostic is gone. Doctor probe for sbx removed.
- **ADR-061** (repo-scoped control): new `[control]` section in
  `<repo>/.af/config.toml` with `agent`, `approval_mode`, `sandbox`,
  `remote`, `remote_control`, `max_agents`. Precedence: CLI > repo >
  user > subsystem defaults > compiled. New `lifecycle.ResolveControl`
  precedence resolver. Additive state.toml fields:
  `Session.ApprovalMode`, `Session.MaxAgents`,
  `Execution.RemoteControl`. Validation rejects unknown sandbox
  providers, unknown remote-control values, shell metacharacters in
  remote, negative max_agents, unknown approval modes.
- **ADR-062** (slicer VM resource profiles): new
  `[sandbox.slicer.resources]` schema (`name, vcpu, ram_gb,
  storage_size, gpu_count, image, hypervisor`). New
  `internal/sandbox/resources.go` with `SlicerResources`,
  `ManagedGroupName(repoSlug, profile)`, `GroupProber` interface, and
  `ExecGroupProber` backed by `slicer vm group` output parsing.
  `lifecycle.CreateOptions.SandboxResources` threads the resolved
  profile into state. 8 additive `Execution.sandbox_resource_*` fields
  in state.toml. Per-VM argv flags deferred pending slicer machine-
  readable group metadata (see `// ADR-062 §Resolution step 6`).
- **ADR-063** (Tailscale + superterm remote control): new
  `internal/control` package and `af control up/down/status` cobra
  group. Composes `superterm up` for the dashboard with
  `tailscale serve --bg <url>` for tailnet exposure. Sentinels for
  missing tools, unsupported provider, unresolvable endpoint. URL
  parsing via regex `https://[a-zA-Z0-9._-]+\.ts\.net\S*`. Flags
  `--remote HOST --provider superterm --port N --json`. Testscript
  `control-up.txt` covers happy path + missing-tool.
- **ADR-064** (opinionated diff rendering): new `internal/diff`
  package. `af diff` now dispatches: `hunk patch -` piped from
  `git diff --no-color base...head` when hunk is on PATH (interactive
  TTY), plain `git diff base...head` fallback, `git diff --stat`
  when stdout is not a TTY, `diffity base..head` for `--web`. Base
  resolution: explicit `--base` > `state.Stack.ParentBranch` >
  `state.Worktree.BaseBranch`. ADR-048's `[diff].cmd` remains as a
  future escape hatch but is no longer the default contract.
- Aggregate: 5 ADRs advanced to `implementation: complete`; every ADR
  from 031 to 064 is now `complete`. 4 new internal packages
  (`control`, `diff`, plus extensions to `sandbox` and `lifecycle`).
  Test count grew from 208 to 222 functions. `make check` green at
  every wave commit; `goreleaser check` clean.

#### Stage 9 — close out in-progress ADRs (Session 26)

- `af pr --ai` now invokes `agent.BodyCmd` with the worktree diff and a
  body-generation prompt; the agent's stdout becomes the PR body.
  Rejects `--ai` with `--web`. Errors on empty diff or empty agent
  output (ADR-057).
- `af retro --ai` now synthesises a narrative via `agent.BodyCmd`
  with `BodyOpts.Cwd = ""`. Adds `--ai-model` flag for model override.
  Errors when no notes match or agent output is empty (ADR-058).
- `af sync` real rebase algorithm: detects dirty worktree, fetches
  parent ref, computes merge-base, runs `git rebase --onto parent
  base branch`, surfaces CONFLICT as `lifecycle.ErrSyncConflict`. New
  `internal/lifecycle.Sync` orchestrator (ADR-059).
- `.goreleaser.yaml` (v2 schema) plus `make snapshot` / `snapshot-all`
  / `release-check` Makefile targets. Cross-compile snapshots build
  for darwin/arm64, linux/amd64, linux/arm64. Legacy `.goreleaser.yml`
  skeleton deleted (ADR-053).
- `internal/lifecycle/remote_sandbox.go` now wires `secret.Envelope`
  into both `PrepareRemoteWorkstream` and `LaunchSandboxWorkstream`:
  envelope is written 0600 before launch and deleted via defer after
  (ADR-042 + ADR-049).
- Testscript integration coverage for proxy commands (`editor.txt`,
  `diff.txt`, `pr.txt`), tmux lifecycle (`tmux-lifecycle.txt` with a
  smart-fake state machine), SSH remote (`ssh-remote.txt` with three
  cases), bringing the testscript count from 8 to 13 (ADR-040, 041,
  046, 048).
- 11 in-progress ADRs advanced to `implementation: complete`: 031
  (v1 master), 040 (tmux), 041 (SSH), 042 (sandbox), 046 (suspend/
  resume), 048 (proxy commands), 049 (secret), 052 (formal
  verification), 053 (build/distribution), 057 (pr --ai), 058 (retro
  --ai), 059 (stack-aware branches). The only `pending` ADRs left
  are 060–064 — deliberately scoped as post-v1 work.

#### Documentation pass (v0 → v1)

- v0 (Rust era) docs archived under `docs/v0/` (PROGRESS, TODO, CHANGELOG, SPEC, PLAN, CONVENTIONS, 30 ADRs, mdBook scaffold, planning, reference).
- `docs/v0/README.md` indexes the archive and explains the v1 boundary.

> v1 ADRs (031–053), v1 spec/plan/conventions, and the new top-level
> README/CLAUDE/AGENTS land in subsequent commits in this same
> Unreleased block. Each ADR will be listed once `accepted`.

#### Go implementation

- Added the initial Go module scaffold: `go.mod`, `cmd/af/`, the planned
  `internal/...` package tree, and `examples/` placeholders.
- Added the minimal cobra root command with persistent `--verbose`,
  `--config`, and `--session` flags plus `af version` backed by
  `internal/version` build metadata.
- Added pinned Go build tooling: `Makefile`, `.golangci.yml`,
  `.goreleaser.yml`, gofumpt/goimports format checks, pedantic
  `golangci-lint`, race-test `make test`, `make check`, and local
  snapshot build targets.
- Added the initial `testscript` harness, `internal/testutil` helpers,
  fake-command PATH hooks, and smoke scripts for `af version` and
  `af --help`.
- Added property-test scaffolds for lifecycle transitions and workstream
  naming invariants using stdlib `testing/quick`.
- Added the `internal/config` layered TOML loader with compiled defaults,
  user/repo merges, global-only section handling, `~` path expansion, and
  proxy command shape validation.
- Added the shared `internal/duration` grammar for `d` / `w` shorthand
  plus stdlib duration units, with table and property tests.
- Expanded `internal/workstream` naming helpers for double-dash session
  sanitization, branch prefix rules, sub-branch names, auto session names,
  and deterministic UUID session IDs.
- Added `internal/session` state persistence with atomic `state.toml`
  writes, append-only `ledger.jsonl`, flock-backed locks, repo slug
  parsing, and current-workstream discovery helpers.
- Added `internal/git` worktree planning helpers for stable primary and
  sub-worktree paths, discovery symlinks, and safe cleanup plans.
- Added `internal/secret` redacting `slog` handler plus fake-backed
  keyring interface for future `af auth` and launch-secret work.
- Added `internal/obsidian` frontmatter parse/emit helpers, note path
  resolution, and an in-memory note store for future note commands.
- Added `internal/agent` provider interfaces, pi/claude/codex command
  builders, availability checks, registry fallback, and fake provider.
- Added `internal/mux` tmux command construction, runner seams,
  recording runner, and fake multiplexer for tests.
- Added `internal/remote` SSH command construction, remote clone path
  mapping, probe command construction, and fake executor.
- Added `internal/sandbox` provider interfaces, slicer/sbx command
  builders, recording runner, and fake sandbox.
- Added testscript fake-command PATH wiring for tmux, ssh, slicer, sbx,
  pi, claude, and codex.
- Added `af completions <bash|zsh|fish|powershell>`: emits the shell-specific completion script to stdout using cobra's built-in generators (`GenBashCompletion`, `GenZshCompletion`, `GenFishCompletion`, `GenPowerShellCompletionWithDesc`).
- Added Stage 7 proxy commands (`af editor`, `af diff`, `af pr`, `af retro`) per ADR-048 and ADR-058. Includes a new `internal/proxy` package with argv-vs-shell token interpolation ({base}, {head}, {worktree}, {title}, {body}) and the `flag_template` expansion for PR creation. `af pr --ai` writes a placeholder body — real `BodyCmd` wiring is the next TODO under ADR-057.
- Added Stage 5 lifecycle commands: `af suspend [session]`, `af resume [session] [--bare]`, `af note [session] --append TEXT`, `af clean [--dry-run --include-abandoned --max-age D --force]`, `af status [--json --all --filter STATE]`, `af stack/unstack/sync [session] [--parent NAME]`. Stack-sync writes the metadata link; the rebase algorithm itself is deferred (ADR-059).
- Added Stage 6 scaffold: `af create --remote <host>` prepares the remote worktree directory via the existing SSH seam; `af create --sandbox <provider>` announces the deferred launch. New `internal/secret.Envelope` writes ephemeral env-files for secret transport (ADR-049).
- Added Stage 4 closeout: `af agent list/add/stop`, `af done [session] [--force]`, `af session-branch` per ADR-038/ADR-039/ADR-046.
- Added Stage 4 MVP: `af create [name] [--from BRANCH] [--current] [--agent NAME] [--bare]` per ADR-038 + ADR-039: composes the full first-feature slice (branch + git worktree + state.toml + ledger.jsonl + `.af/state.toml` discovery symlink + optional Obsidian note + tmux session + primary-agent launch). Orchestration lives in `internal/lifecycle.Create`; the cmd layer detects the repo via `git rev-parse --show-toplevel`, parses the remote URL into a host/owner/repo slug, and threads `[general]`, `[branch]`, and `[obsidian]` config through. A new `internal/git.Runner` seam wraps `git` calls so tests use a `FakeRunner`.
- Added `af auth set|get|status|clear|list` per ADR-049: stores credentials in the OS keyring via `zalando/go-keyring`. `set` reads the value from a TTY with echo off (falls back to stdin), `get` prints in plain on a TTY but redacts to `[REDACTED:abcd...]` on non-TTY output, `status` lists the curated trio (anthropic_api_key, openai_api_key, github_token) plus any extras, `clear` removes one entry, and `list` enumerates all stored keys (names only). Backed by a new `internal/secret.SystemKeyring` that maintains a per-service account index so List works on top of the OS keyring API that has no native enumeration.
- Added `af setup` per ADR-045: creates `~/.local/share/af/v1/{sessions,archive,secrets}`, scaffolds `~/.config/af/config.toml` (refuses overwrite unless `--force`), appends `.af/` to the global gitignore (honouring an existing `core.excludesfile`), and installs shell completions for bash/zsh/fish (powershell prints a hint). Emits a `[obsidian.vaults]` configuration hint when the section is empty. Fully idempotent. Backed by `internal/setup` with injected `GitConfigurer` and shell-generator seams.
- Added `af doctor` (local) and `af doctor --remote <host>` per ADR-044. Probes git, tmux, the agent trio (pi/claude/codex, OR-group), gh, fzf, slicer, sbx, delta, and any `[doctor].extra_tools` entries against the local PATH or an SSH remote. Renders install hints per platform (macOS/Arch/Debian/Other) detected via `/etc/os-release`. Exits 1 when any TierMust requirement is missing. Backed by `internal/doctor` with `SystemLookup`, `RemoteLookup`, and OS-release parsing fakes.
- Added `af config init` and `af config show`: `init` scaffolds the
  annotated user config at `$HOME/.config/af/config.toml` (or the path
  given via `--config`) and refuses to overwrite an existing file;
  `show` prints the effective layered configuration as canonical TOML.
  Backed by reusable `internal/config.WriteUserConfig`,
  `UserConfigTemplate`, and `Render` helpers.

### Removed

> v1 removes the Rust-era implementation surface and several v0 features.
> The v0 documentation archive remains under `docs/v0/`; deleted source
> remains available through git history.

- Rust v0 source, integration tests, Cargo files, `.cargo/`, `justfile`,
  and Rust tool configs (`src/`, `tests/`, `Cargo.toml`, `Cargo.lock`,
  `clippy.toml`, `deny.toml`, `rust-toolchain.toml`, `rustfmt.toml`).
- DD Workspaces remote provider (replaced by generic SSH-host model).
- exe.dev remote provider special-casing (subsumed by generic SSH-host model).
- cmux multiplexer; zellij/Ghostty multiplexer scaffolding.
- Three-layer composition (`agent × remote × sandbox`) as a runtime model — replaced by an explicit `--remote <ssh-host>` + `--sandbox <slicer|sbx>` flag pair on `af create`.
- Provisioning pipeline (`provision/`, embedded bootstrap scripts, dotfiles install).
- Skill bundle installer (v0 ADR-030).
- `af migrate` (v0 cf-sessions migration); v0 was never released to anyone other than the owner.
- Agent providers: gemini, amp, copilot.
- mdBook user guide.
- `clap_complete` machinery (replaced by `cobra` completion generator).
- `keyring`/`secrecy`/`zeroize` Rust dependencies (replaced by `zalando/go-keyring`).

### Reduced surface (compared to v0)

- Multiplexer providers: 1 (`tmux`). Was 2 in v0.
- Agent providers: 3 (`pi` default, `claude`, `codex`). Was 6 in v0.
- Remote providers: 1 (generic SSH host). Was 2 in v0 with a plugin layer.
- Sandbox providers: 2 (`slicer`, `sbx`). Unchanged from v0 but with simpler composition.
- Top-level commands: ~14 (TBD per ADR-031). Was 19 in v0.

### Build & distribution

- Build tool: `Make`. Was `just`.
- Release tool: `goreleaser`. Was a hand-written GitHub Actions workflow.
- Distribution targets: `linux/{amd64,arm64}`, `darwin/{amd64,arm64}` cross-compiled by default, plus `go install github.com/kakkoyun/af@latest`.
- No Homebrew tap planned for v1 (single-user project).
- No GitHub Releases planned for v1 (single-user project).

[Unreleased]: https://github.com/kakkoyun/af/compare/main...HEAD
