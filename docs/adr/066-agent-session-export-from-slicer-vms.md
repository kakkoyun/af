---
adr: 066
title: "Agent Session Export from Slicer VMs"
status: proposed
implementation: in-progress
date: 2026-05-21
last_modified: 2026-05-22
supersedes: []
superseded_by: null
related: ["037", "039", "043", "046", "047", "049", "065"]
tags: ["go", "sandbox", "slicer", "session", "agent", "transcript"]
---

# ADR-066: Agent Session Export from Slicer VMs

## Context

ADR-065 moves work into slicer VMs through `slicer wt`. Git commits and
files can be pulled back with `slicer wt pull`, but agent conversation
state is not part of Git. If a Claude, Codex, pi, or harness process runs
inside the VM, its transcripts and session metadata stay in the VM's home
directory. If the VM is deleted before those files are harvested, we lose
valuable debugging context, recall/retro material, and the ability to
continue an agent conversation on the host when needed.

Research confirms this is feasible and useful:

- Claude Code sessions are local JSONL transcripts under
  `~/.claude/projects/<project-key>/...` and can be resumed with session
  commands such as `--resume` / `--continue`.
- Codex stores local session JSONL files under
  `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl`, and `codex resume`
  can reopen previous conversations.
- pi exposes a configurable `sessionDir`; by default the user's recall
  tooling indexes `~/.pi/agent/sessions/`. pi team/harness state also
  lives under `~/.pi/agent/...`.
- Slicer supports copying files and directories out of a VM with
  `slicer vm cp` (including tar mode), and executing commands inside a VM
  with `slicer vm exec` when `slicer-agent` is present.

Session files can contain prompts, tool output, file snippets, and
possibly secrets accidentally pasted into conversations. They belong in
the user's home agent directories, not in the repo and not in Obsidian by
default.

## Decision

`af` will harvest agent and harness session data from slicer VMs before
VM teardown and on demand. The harvested files are staged, validated, and
merged into the corresponding host home directories for analysis. When
safe and supported, `af` can also normalize the imported data so the user
can continue the conversation on the host.

### CLI surface

```text
af session-data pull [session] [--agent all|claude|codex|pi|harness] [--continue-host] [--dry-run]
af session-data list [session] [--vm VM]
```

- `pull` copies session data out of the VM and imports it into host-side
  agent/harness directories.
- `list` inventories session files found in the VM without importing.
- `--continue-host` requests path normalization for supported agents so a
  host-side resume command can find the imported session from the host
  worktree.
- `--dry-run` prints source/destination mappings and collision decisions
  without copying.

`af suspend` and `af done` for a slicer-backed workstream run
`af session-data pull --agent all` before deleting or discarding the VM,
unless the user passes an explicit destructive/discard flag.

### Sources and destinations

`af` uses an allowlist. It does not copy whole home directories.

| Agent / harness | VM sources | Host destination |
| --------------- | ---------- | ---------------- |
| Claude Code | `~/.claude/projects/**`, plus `~/.claude/sessions/**` when present | `~/.claude/projects/**`, `~/.claude/sessions/**` |
| Codex | `~/.codex/sessions/**/rollout-*.jsonl` | `~/.codex/sessions/**/rollout-*.jsonl` |
| pi | resolved pi `sessionDir`; default `~/.pi/agent/sessions/**`; project `.pi/sessions/**` when configured | host resolved pi `sessionDir`; default `~/.pi/agent/sessions/**`; host project `.pi/sessions/**` |
| Harness / teams | `~/.pi/agent/teams/**` and other explicitly configured harness session roots | matching host harness roots |

The allowlist explicitly excludes auth/config/cache material such as
`~/.claude/settings.json`, `~/.codex/auth.json`, `~/.pi/agent/settings.json`,
model caches, API tokens, and package installs. Secrets used by agents
remain governed by ADR-049 and are not exported as session data.

### Transport

For a slicer VM `<vm>`, `af` performs a two-phase import:

1. Inventory inside the VM:

   ```text
   slicer vm exec <vm> -- af-session-export-manifest
   ```

   The implementation may ship this helper inline as a POSIX shell script
   rather than requiring an installed binary in the guest. It resolves
   existing allowlisted paths, records file sizes, mtimes, and hashes when
   cheap, and emits a JSON manifest.

2. Copy into a host staging directory:

   ```text
   slicer vm cp --mode=tar <vm>:<allowlisted-dir> <host-staging-dir>
   ```

   Staging lives under:

   ```text
   ~/.local/share/af/v1/session-import/<af-session>/<vm>/<timestamp>/
   ```

3. Validate and merge from staging into host agent directories.

The merge is append-only by default:

- If the destination file is absent, create it with mode `0600` and
  parent directories `0700`.
- If the destination exists with the same hash, skip it.
- If the destination exists with different content, do **not** overwrite;
  place the new file under
  `~/.local/share/af/v1/session-import/conflicts/<af-session>/<vm>/...`
  and report the conflict.

Every import writes an `agent_sessions_pulled` ledger event with the VM,
agent kinds, counts, destination roots, conflicts, and whether
`--continue-host` normalization ran.

### Host continuation mode

Analysis-only import preserves VM transcripts byte-for-byte. That is the
default because it is safe and sufficient for recall, retro, and manual
inspection.

`--continue-host` is opt-in. It is allowed only after the worktree lease
from ADR-065 has been pulled back or otherwise marked safe, because the
host agent must resume against host files that match the VM work.

When `--continue-host` is set, `af` may perform format-specific path
normalization for known agents:

- Rewrite recognized `cwd` / project-root metadata from the VM worktree
  path to the host worktree path.
- For Claude Code, place project-scoped transcripts under the host
  project's Claude directory key so `claude --resume` / `/resume` can
  discover them from the host checkout.
- For Codex, preserve the dated `~/.codex/sessions/**/rollout-*.jsonl`
  path and print the session ID / resume hint when discoverable.
- For pi, import into the host's resolved `sessionDir` and preserve the
  original file in staging before rewriting recognized header fields.

If a format is unknown or a path rewrite is ambiguous, `af` imports the
session for analysis only and prints a manual resume hint. It must not
silently mutate transcripts it does not understand.

### Lifecycle integration

- `af create --sandbox slicer` records which agent provider launched in
  the VM and the expected session roots for that provider.
- `af resume` continues to attach to the VM while the VM owns the work.
  It does not import session data automatically.
- `af suspend` imports session data before tearing the VM down.
- `af done` imports session data before teardown, then requires the Git
  pull/discard decision from ADR-065.
- `af clean` may remove old staging bundles only after their files are
  present in host agent directories or explicitly marked discarded.

### Privacy and safety

Session transcripts are private user data. `af` must:

- Never write imported transcripts into the repo worktree.
- Never add imported transcripts to Git.
- Use `0700` directories and `0600` files for staging and merged data.
- Copy only allowlisted session/transcript paths.
- Keep an import manifest so the user can audit exactly what moved.
- Prefer conflicts/quarantine over overwrite.

## Consequences

### Pros

- VM teardown no longer destroys agent conversation history.
- Host-side recall, retro, and analysis tools can index VM-created
  sessions alongside normal host sessions.
- The user can continue selected conversations on the host when the
  format supports safe path normalization.
- The design respects ADR-065: Git work comes back through `slicer wt
  pull`, while agent session data comes back through a separate private
  transcript channel.
- Allowlisting avoids copying credentials, package installs, caches, and
  other high-risk home-directory state.

### Cons / risks

- Agent session formats are not stable APIs; resume support can break
  when Claude, Codex, or pi changes on-disk metadata.
- Transcripts may contain sensitive data. Importing them expands their
  lifetime and makes host-side retention policy important.
- Host-continuation mode requires path rewriting and must stay
  conservative to avoid corrupting transcripts.
- Copying large session trees can be slow; staging and hash checks add
  overhead.
- Automatically importing before `done` / `suspend` may surprise users
  who expected the VM to be ephemeral unless the behaviour is documented
  clearly.

## Alternatives Considered

- **Do nothing; let VM sessions die with the VM.** Rejected. It loses the
  record needed for debugging, retros, and host continuation.
- **Mount host `~/.claude`, `~/.codex`, or `~/.pi` into the VM.**
  Rejected. It breaks the isolation boundary, exposes credentials and
  caches, and lets VM-side tools mutate host agent state directly.
- **Copy the entire VM home directory.** Rejected. Too much secret and
  cache material; import must be allowlisted.
- **Rely on each agent's cloud sync.** Rejected. The relevant tools store
  local transcripts, and `af` should not require network/cloud behaviour
  for local recovery.
- **Always rewrite transcripts for host continuation.** Rejected. Exact
  import is safer; rewriting is opt-in and only for known formats.

## References

- ADR-037 — state and ledger events.
- ADR-039 — multi-agent workstreams may create multiple transcript roots.
- ADR-043 — agent provider selection.
- ADR-046 — suspend/done lifecycle integration.
- ADR-047 — Obsidian notes remain separate from raw transcripts.
- ADR-049 — secret handling; credentials are not session data.
- ADR-065 — `slicer wt` moves Git work separately from session export.
- Claude Code sessions docs: <https://code.claude.com/docs/en/sessions>
- Codex CLI session/resume docs: <https://developers.openai.com/codex/cli/features>
- Slicer copy docs: <https://docs.slicervm.com/tasks/copy-files/>
- Slicer exec docs: <https://docs.slicervm.com/tasks/execute-commands/>
- Local recall skill notes: `~/.agents/skills/recall/UPSTREAM.md`.
