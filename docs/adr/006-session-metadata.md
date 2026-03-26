# ADR-006: Session Metadata & Persistence

**Status:** Accepted
**Date:** 2026-03-26

## Context

`cf` stores session metadata in two places:

1. **tmux environment variables** — per-session `CF_*` vars, fast to read but lost on tmux restart
2. **Disk cache** — `~/.local/share/cf-sessions/<session>.env` files, survive reboots

This dual-write approach works but has problems:

- Metadata format is flat key=value (no structured data)
- Recovery logic is scattered and fragile
- Multiplexer env vars are the "source of truth" but the least durable store

## Decision

### Make **disk** the source of truth, multiplexer env vars are a cache

Session metadata is stored as **TOML files** at:

```
~/.local/share/af/sessions/<session-name>.toml
```

### Schema

```toml
# ~/.local/share/af/sessions/kakkoyun--issue-42.toml

[session]
name = "kakkoyun--issue-42"
id = "550e8400-e29b-41d4-a716-446655440000"   # UUID v5
created_at = 2026-03-26T14:30:00Z

[worktree]
path = "/home/kakkoyun/Workspace/.worktrees/myrepo/kakkoyun/issue-42"
branch = "kakkoyun/issue-42"
base_branch = "upstream/main"
git_root = "/home/kakkoyun/Work/myrepo"

[agent]
provider = "claude"
# provider-specific state (e.g., session IDs, tokens)

[execution]
mode = "local"                      # local | workspace | remote | sandbox | bare
# Remote fields (present only for remote/sandbox modes)
# remote_provider = "exedev"
# remote_host = "myvm.exe.xyz"
# remote_work_dir = "/home/ubuntu/src/myrepo"
# sandbox_provider = "slicer"
# sandbox_name = "slicer-3"
# yolo = false
```

### Lifecycle

1. **Create:** Write TOML file + inject vars into multiplexer session
2. **Resume:** Read TOML file → re-inject into multiplexer if needed
3. **Teardown:** Delete TOML file + cleanup multiplexer session
4. **GC:** Scan TOML files, cross-reference with git/multiplexer state
5. **List:** Read all TOML files + augment with live multiplexer state

### Migration from `cf`

`af` can read existing `~/.local/share/cf-sessions/*.env` files and convert them to the
new TOML format on first access. This provides seamless migration.

## Consequences

- TOML files are human-readable and debuggable (`cat` a session to see its state).
- Structured data enables richer queries (e.g., "list all sandbox sessions").
- Disk is the source of truth — multiplexer restart/crash doesn't lose metadata.
- Multiplexer env vars become a performance cache, not a requirement.
- File-per-session avoids database dependencies; directory listing = session enumeration.
- `~/.local/share/af/` follows XDG conventions (`XDG_DATA_HOME`).
