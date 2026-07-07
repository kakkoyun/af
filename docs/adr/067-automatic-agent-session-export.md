---
adr: 067
title: "Automatic Agent Session Export and Sync State"
status: accepted
implementation: complete
date: 2026-05-21
last_modified: 2026-07-03
supersedes: []
superseded_by: null
related: ["037", "046", "049", "065", "066"]
tags: ["go", "sandbox", "slicer", "session", "export", "state"]
---

# ADR-067: Automatic Agent Session Export and Sync State

## Context

ADR-066 introduced copying Claude, Codex, pi, and harness session data
out of slicer VMs into the matching host-side home directories. The
missing requirement is priority: exporting is not merely a manual rescue
command. It must happen automatically before `af` tears down or discards
a VM, because once the microVM is gone the transcript state is gone too.

A manual command is still useful for mid-session analysis or for
continuing work on the host before the VM lifecycle ends. That creates an
idempotency requirement: if the user ran the explicit export earlier,
the automatic export at teardown must not skip the session. It must
sync the latest VM-side transcript data into the existing host-side copy.

The right place to track this is `af` state plus import manifests, not
Git and not agent config files. The host `state.toml` already records the
workstream, VM handle, lifecycle, and slicer worktree lease. The ledger
records lifecycle events. Session-export sync should extend that state
with per-source cursors and hashes so `af` knows what was copied, what
changed, and what still needs reconciliation before a VM can be deleted.

## Decision

Agent session export from slicer VMs is **automatic by default** at any
VM-destroying lifecycle boundary, with an explicit command available for
on-demand sync. Both paths use the same sync engine and state records.

### Lifecycle rule

Before any command deletes, discards, suspends, or otherwise makes a
slicer VM unreachable, `af` must run session export:

```text
af session-data sync <session> --agent all
```

This applies to at least:

- `af suspend [session]`
- `af done [session]`
- `af clean --force <session>` when it would delete a VM
- any future repair/teardown command that removes a slicer VM

VM teardown may proceed only after one of these outcomes:

1. session-data sync succeeded;
2. there was no reachable VM / no allowlisted session data; or
3. the user passed an explicit destructive flag acknowledging transcript
   loss.

A plain failure to copy or merge session data blocks VM deletion and
prints recovery commands. This is intentional: transcripts are part of
the workstream output.

### Explicit command

ADR-066's command is renamed around sync semantics:

```text
af session-data sync [session] [--agent all|claude|codex|pi|harness] [--continue-host] [--dry-run]
af session-data list [session] [--vm VM]
```

`sync` is safe to run repeatedly. It always re-inventories the VM and
copies the latest allowlisted files. Running it manually does not
suppress the automatic sync later; it only advances the recorded cursors.

### State schema

`state.toml` gains an optional `[session_export]` section:

```toml
[session_export]
last_sync_at = null
last_sync_status = "never" # never | ok | blocked | discarded
last_manifest = ""         # path under ~/.local/share/af/v1/session-import/...

[[session_export.sources]]
agent       = "pi"          # claude | codex | pi | harness
vm          = "sbox-abc123"
source_path = "/home/ubuntu/.pi/agent/sessions/2026-05-21.jsonl"
dest_path   = "/Users/me/.pi/agent/sessions/2026-05-21.jsonl"
mode        = "append-jsonl" # copy | append-jsonl | directory
size        = 123456
hash        = "sha256:..."
mtime       = "2026-05-21T...Z"
last_offset = 123456         # for append-jsonl when dest is a verified prefix
status      = "ok"           # ok | conflict | skipped | blocked
```

The ledger also records each sync attempt as `agent_sessions_synced` with
counts, conflicts, and whether the sync was automatic or explicit.

State is an optimization and an audit trail, not the only source of
truth. Every sync still inventories the VM before deciding what to copy,
because session files may have grown since the last explicit sync.

### Latest-sync merge rules

When a VM-side session file maps to a host-side file that already exists,
`af` applies these rules:

1. **Same hash**: skip; host already has the latest copy.
2. **Destination is a verified prefix of the VM file**: append only the
   missing tail atomically, then update `last_offset`, `size`, and
   `hash`. This is the expected case for JSONL transcript files that grew
   after an earlier explicit sync.
3. **VM file is smaller than destination with matching prefix**: keep the
   host file and report a warning; do not truncate.
4. **Divergence**: do not overwrite. Copy the VM file into the conflict
   quarantine under `~/.local/share/af/v1/session-import/conflicts/...`
   and mark the source `conflict` in state.
5. **Directory sources**: merge files individually using the same rules;
   never delete host files just because they are absent in the VM.

For known append-only formats (`*.jsonl` Claude/Codex/pi transcript
files), prefix verification is byte-for-byte. If verification fails, the
file is divergent even if names match.

### Host continuation

`--continue-host` remains opt-in. Automatic lifecycle sync does not
rewrite transcripts for host continuation by default; it preserves and
updates the latest analysis copy. If `--continue-host` was used earlier,
the automatic sync still uses the recorded mapping and appends the latest
VM transcript tail when the destination is a verified prefix.

If continuation normalization produces conflicts, VM teardown is blocked
unless the user discards the conflict explicitly.

### What state may contain

State files may contain paths, hashes, byte offsets, timestamps, agent
names, VM names, and import manifest paths. They must not contain raw
transcript content, prompts, tokens, API keys, or credential material.
Raw transcripts live only in the host agent/harness directories and the
private import staging/quarantine directories with `0700` / `0600`
permissions as required by ADR-066.

## Consequences

### Pros

- VM teardown cannot silently delete the only copy of agent transcripts.
- Manual mid-session exports and automatic teardown exports compose: the
  later sync appends or merges the latest data.
- State gives `af` an auditable answer to "what did we sync, from where,
  and when?"
- Repeated syncs are cheap for unchanged files and safe for append-only
  transcript files.
- The design preserves VM isolation: no host home directories are mounted
  into the VM.

### Cons / risks

- `af suspend` / `af done` can now block on transcript export problems.
  That is safer, but it makes lifecycle commands less fire-and-forget.
- Prefix-append logic must be correct; a bad implementation could corrupt
  host transcripts. Hash and prefix checks are mandatory.
- State can become stale if users manually copy or edit imported session
  files. Re-inventorying and hash checks reduce but do not eliminate this
  complexity.
- Automatic export may surprise users who expected VM transcripts to be
  destroyed with the VM. Documentation must make transcript retention
  explicit.

## Alternatives Considered

- **Manual export only.** Rejected. Users will forget, and VM teardown
  would destroy valuable session data.
- **Automatic export only, no explicit command.** Rejected. Mid-session
  analysis and host continuation are useful before teardown.
- **Use only timestamps to skip copied files.** Rejected. Session files
  are important enough to require hashes and prefix verification.
- **Overwrite host files with VM files on every sync.** Rejected. It can
  destroy host-side appended data or normalized continuation copies.
- **Store transcript bodies in `state.toml`.** Rejected. State should
  track sync metadata only; transcripts remain private files.

## References

- ADR-037 — state and ledger event model.
- ADR-046 — suspend/done lifecycle boundaries.
- ADR-049 — secrets must not be copied as session data.
- ADR-065 — slicer VM worktree lease and VM teardown constraints.
- ADR-066 — source/destination allowlist and host import locations.
