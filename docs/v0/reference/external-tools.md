# External Tool Reference — CLI Surfaces

**Purpose:** a durable snapshot of the `slicer`, `sbx`, `workspaces`, and `cmux`
command surfaces that `af` wraps. Lanes stubbing these tools in tests, and
authors wrapping subcommands in provider code, should reference this page
rather than re-deriving the signatures. ADR-029 removes the workspace-wide
`CommandRunner` trait; provider tests assert on argv directly, so an accurate
argv reference matters.

**Verification date:** 2026-04-21, on the developer's macOS machine. See the
appendix for version stamps.

---

## slicer (installed)

```
Commands:
  amp|claude|codex|copilot|opencode    # Launch agent-specific sandbox, attach
  env                                   # Declarative VM envs from slicer.yaml
  secret {create,list,remove,update}    # Slicer-native secret store
  vm {add,list,delete,shell,health}     # Low-level VM lifecycle
  workspace [./path|<vm-name>]          # Provision VM, sync workspace, open shell
  install, update, info, eula, version
```

**Critical daemon flags (apply to every subcommand):**

- `--url <URL>` / `SLICER_URL` — point at a slicer daemon (local or remote)
- `--token <T>` / `--token-file <F>` / `SLICER_TOKEN` / `SLICER_TOKEN_FILE`

**Implication:** "remote slicer" is *just* "pass `--url`" — no SSH install
step, no separate provisioning pipeline. The user's shell already wraps this:
`slicer --remote=<host> <cmd>` → `command slicer <cmd> --url <resolved-url>
--token-file ...`.

## sbx (installed)

```
Commands:
  create <agent> <path>                 # Create sandbox, don't attach
  run <agent> [path]                    # Create (if needed) + attach
  exec <sandbox> <cmd>                  # Run command inside sandbox
  ls, rm, stop                          # List/destroy/pause
  secret {set,ls,rm}                    # Secret proxy (never exposed to agent)
  ports, policy, template, diagnose
  login, logout                         # Docker Hub auth
```

**Agents supported by `sbx run`:** `claude, codex, copilot, docker-agent,
droid, gemini, kiro, opencode, shell`.

**Critical quote from `sbx secret --help`:**

> When a sandbox starts, the proxy uses stored secrets to authenticate API
> requests on behalf of the agent. **The secret is never exposed directly to
> the agent.**

## workspaces (DD, installed)

```
Commands:
  create [name] --branch --dotfiles --repo --vscode-template ...
  connect <name> --editor vscode|cursor|pycharm|intellij|goland|rustrover
  list, delete, restart, diagnostics
  daemon {start,stop,status}            # Workspaces daemon
  dotfiles {...}                        # Dotfiles config mgmt
  proxy {...}                           # Build service proxies
  secrets {set,get,list,remove,sync,set-flags}   # Workspace-scoped secrets
  settings {...}
  ssh-config <name>                     # Updates ~/.ssh/config for workspace
```

## cmux (installed)

```
Binary: /Applications/cmux.app/Contents/Resources/bin/cmux
Socket-based: Unix socket + password auth (CMUX_SOCKET_PASSWORD)

Commands:
  workspace {new, list, action}, window {new, focus, close}, pane, surface
  ssh <dest> [--name --port --identity --no-focus]    # Remote workspace
  claude-teams, omc, omo, omx, codex install-hooks
  capabilities, rpc <method> [json-params]
  ping, version, identify, themes, welcome, shortcuts, feedback
```

---

## Appendix — Version Stamps (reproducibility)

Probed on 2026-04-21 on the developer's macOS machine:

- `slicer` — installed; `/opt/homebrew/bin/slicer`; supports `--url` daemon mode
- `sbx` — installed; `/opt/homebrew/bin/sbx`; 9 agents including `shell`
- `workspaces` — installed; `/opt/homebrew/bin/workspaces`; DD-internal
- `cmux` — installed; `/Applications/cmux.app/Contents/Resources/bin/cmux`

The exact subcommand surfaces above are verified against these versions.
