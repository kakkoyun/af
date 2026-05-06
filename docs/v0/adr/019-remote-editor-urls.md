# ADR-019: Remote Editor URL Scheme Strategy

**Status:** Accepted
**Date:** 2026-04-21

## Context

`af editor --visual` currently opens VS Code or Zed for local worktrees. For remote
sessions (exe.dev), opening the editor should connect to the remote host's working
directory, not a local path.

Three editors are in scope based on the current codebase's `editor.rs` and the target
environment (macOS + Arch Linux development machines):

| Editor | Local `af editor` | Remote target URL |
|---|---|---|
| VS Code | `code <path>` | `vscode://vscode-remote/ssh-remote+<host>/<path>` |
| Cursor | `cursor <path>` | `cursor://vscode-remote/ssh-remote+<host>/<path>` |
| Zed | `zed <path>` | `zed ssh://<host>/<path>` |

The URL schemes open the editor and connect to the remote working directory via
the editor's built-in SSH remote extension. No additional daemon is required beyond
a reachable SSH host.

## Decision

### URL format per editor

**VS Code:**
```
code --folder-uri vscode-remote://ssh-remote+<host>/<remote-path>
```
Uses the `--folder-uri` flag which VS Code handles via its remote extension protocol.
Alternatively: `open "vscode://vscode-remote/ssh-remote+<host>/<remote-path>"` on
macOS. The `--folder-uri` approach works cross-platform.

**Cursor:**
```
cursor --folder-uri cursor://vscode-remote/ssh-remote+<host>/<remote-path>
```
Cursor forks VS Code's remote extension; the URL scheme is `cursor://` instead of
`vscode://`, but the path format is identical.

**Zed:**
```
zed ssh://<user>@<host>/<remote-path>
```
Zed SSH remote is a native feature (no extension). The URI scheme is `zed ssh://`.
The user can be omitted if the SSH config specifies it for the host.

### Editor detection and fallback chain

Same detection as local mode: check `config.editor.visual` first, then walk
`which code`, `which cursor`, `which zed`. First found wins. If none found, print
an actionable error.

### Implementation in `src/cmd/editor.rs`

```rust
fn remote_editor_uri(editor: &EditorKind, host: &str, path: &str) -> String {
    match editor {
        EditorKind::VSCode  => format!("vscode-remote://ssh-remote+{host}/{path}"),
        EditorKind::Cursor  => format!("cursor://vscode-remote/ssh-remote+{host}/{path}"),
        EditorKind::Zed     => format!("zed ssh://{host}/{path}"),
    }
}
```

For VS Code and Cursor, pass the URI via `--folder-uri`. For Zed, pass it as a
positional argument.

### Remote path source

The remote working directory is stored in session metadata at `af create --remote`
time (`session.remote_path`). `af editor` reads it from the session store. No SSH
roundtrip is needed at editor-open time.

### Liveness guard

Before opening the URI, `is_alive` (ADR-017) is called. If the VM is unreachable,
print an error rather than opening an editor to a dead host.

### macOS `open` vs direct invocation

On macOS, `open "<uri>"` works for all three schemes and respects the user's default
application for each URL type. On Linux (Arch), direct invocation (`code --folder-uri
<uri>`) is more reliable since `xdg-open` may not handle custom protocol schemes
without `.desktop` file registration. Implement both: try direct first, fall back
to `open`/`xdg-open`.

## Consequences

- `af editor` becomes useful for remote sessions without requiring a new tool or
  daemon.
- The URL schemes depend on each editor's SSH remote extension being installed.
  VS Code Remote — SSH is the only mandatory external dep; it's bundled in VS Code
  Server (auto-installed on first connection). Cursor behaves identically. Zed SSH
  remote is native.
- `session.remote_path` must be populated at create time. Sessions created before
  this field was added will fall back to `~` with a warning.
- No terminal editor support for remote (`af editor --terminal` with a remote
  session starts a plain `ssh <host>` and opens `$EDITOR` there — straightforward,
  no URI scheme needed).
