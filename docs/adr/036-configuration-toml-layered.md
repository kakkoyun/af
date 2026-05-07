---
adr: 036
title: "Configuration — TOML, layered, with global Obsidian vault paths"
status: proposed
implementation: pending
date: 2026-05-06
last_modified: 2026-05-06
supersedes: []
superseded_by: null
related: ["031", "034", "044", "045", "047", "049"]
tags: ["go", "config", "toml"]
---

# ADR-036: Configuration — TOML, Layered, with Global Obsidian Vault Paths

## Context

`af` needs configuration for: default agent, multiplexer, branch
prefix, worktree root, editor commands, diff/PR proxy commands, remote
SSH defaults, sandbox defaults, Obsidian vault paths, secret service
name, and lifecycle retention. Some fields are user-machine-scoped
(vault paths, default agent); others can be repo-overridden (branch
prefix, base branch, sandbox preference).

The owner explicitly called out that **Obsidian vault paths** are a
global concern — they describe where the user's vaults live on this
machine, unrelated to any project — and must live in the user-level
config only.

## Decision

### Layer hierarchy

Three layers, merged in order with later layers winning per-field:

1. **Compiled defaults** — built into the binary; documented in this ADR's "Schema" section.
2. **User config** — `~/.config/af/config.toml`. Created by `af setup` if missing, or `af config init` on demand.
3. **Repo config** — `<repo>/.af/config.toml`. Optional. Per-project overrides.

### Library: `github.com/BurntSushi/toml`

Sufficient for our schema. Smaller and more battle-tested than
`pelletier/go-toml/v2`. No reason for `viper` (which adds ~5 transitive
deps) — our merge order is fixed and trivial to hand-roll.

### Schema

Field grouping (sections):

```toml
schema_version = 1

[general]
default_agent  = "pi"            # one of: pi | claude | codex
multiplexer    = "tmux"          # only "tmux" in v1 (ADR-040)
max_sessions   = 10              # cap on concurrent workstreams
worktree_root  = "~/Workspace/.worktrees"

[branch]
prefix              = ""          # empty = no prefix; e.g. "kakkoyun"
prefix_on_fork_only = true        # only apply when `upstream` remote exists

[editor]
terminal = "$EDITOR"              # falls back to nvim if $EDITOR unset
visual   = ""                     # empty = auto-detect: code > zed

[diff]
cmd      = "git diff {base}...HEAD"
                                  # tokens: {base}, {head}, {worktree}
[pr]
cmd      = "gh pr create --base {base} --head {head}"
                                  # tokens: {base}, {head}, {title}, {body}
template = ""                     # path to PR template file (default: repo's .github/PULL_REQUEST_TEMPLATE.md)

[remote]
default_host = ""                 # empty = remote requires explicit --remote arg
ssh_options  = ["-o", "ServerAliveInterval=60"]

[sandbox]
default_provider = ""             # empty | "slicer" | "sbx"

[sandbox.slicer]
group = ""                        # slicer host group; empty = auto

[sandbox.sbx]
image = ""                        # sbx default image override

[obsidian]
notes_vault   = ""                # name of vault key from [obsidian.vaults]
notes_folder  = "00 - af"         # folder within the chosen vault
notes_template = ""               # optional path to a markdown template

[obsidian.vaults]
# Map of vault-name -> absolute path on this machine.
# Multiple entries supported (personal, work, etc.).
# THIS SECTION IS GLOBAL ONLY: never appears in <repo>/.af/config.toml.
# personal = "/Users/kemal/Vaults/personal"
# work     = "/Users/kemal/Vaults/work"

[doctor]
# Extra binaries to probe beyond the built-in defaults.
extra_tools = []

[secret]
keyring_service = "af"            # service name passed to zalando/go-keyring (ADR-049)

[lifecycle]
retention_days = 90               # archive expiry
auto_archive   = true             # move completed sessions to archive/ on `af done`
```

### Global-only sections

Two sections live **only** in the user-level config; they are silently
ignored if seen in a repo config:

1. `[obsidian.vaults]` — vault paths are per-machine.
2. `[secret]` — keyring service name shouldn't vary per-repo.

The TOML parser doesn't enforce this; the merge function strips these
sections from any repo-config layer with a `slog.Warn`.

### Token interpolation

`[diff].cmd` and `[pr].cmd` accept these tokens, replaced at exec time:

| Token | Value |
|---|---|
| `{base}` | base branch (e.g. `upstream/main`) |
| `{head}` | workstream branch |
| `{worktree}` | absolute worktree path |
| `{title}` | PR title (if provided via `--title`) |
| `{body}` | PR body |

Tokens are simple `strings.ReplaceAll`; no shell expansion. The command
is then split via `shlex` (or hand-rolled equivalent — TBD per ADR-048
implementation) and executed via `exec.CommandContext`.

### `~` expansion

All path-like fields (`worktree_root`, `obsidian.vaults.*`) expand `~`
at load time using `os.UserHomeDir()`. Stdlib only.

### Resolution sketch

```go
type Config struct {
    SchemaVersion int          `toml:"schema_version"`
    General       GeneralConfig `toml:"general"`
    Branch        BranchConfig  `toml:"branch"`
    Editor        EditorConfig  `toml:"editor"`
    // ...
}

func Load(ctx context.Context, repoDir string) (*Config, error) {
    cfg := defaultConfig()
    if user, err := loadFromFile(userConfigPath()); err == nil {
        merge(cfg, user, /*allowGlobalOnly=*/true)
    }
    if repoDir != "" {
        if repo, err := loadFromFile(filepath.Join(repoDir, ".af", "config.toml")); err == nil {
            merge(cfg, repo, /*allowGlobalOnly=*/false)
        }
    }
    return cfg, nil
}
```

Missing files are not errors; parse errors are.

### Schema versioning

`schema_version = 1` is recorded at the top. v1 ships only schema 1.
Future schema bumps add a migration step to `Load`. Reading a higher
schema than the binary supports is an error.

## Consequences

- One TOML library dep, no `viper`.
- Vault paths live exactly where they belong.
- Fields that need shell-style flexibility (diff/pr commands) get a
  small token interpolation surface, not full shell parsing.
- Schema version pins the file format; future migrations are trivial.

## Alternatives Considered

- **`pelletier/go-toml/v2`** — feature equivalent; rejected for being slightly larger and not adding value.
- **`spf13/viper`** — rejected; brings ~5 transitive deps for layered config that we already own.
- **YAML for config** — rejected; TOML matches v0 and is friendlier to hand-edit.
- **Per-environment configs** (e.g. `config.dev.toml`) — rejected as out of scope; single-user.

## References

- [`BurntSushi/toml`](https://github.com/BurntSushi/toml)
- v0 ADR-003 (Layered Configuration System) — superseded by this ADR for v1.
- ADR-031 — v1 master, dep set.
- ADR-044 — `af doctor` reads `[doctor].extra_tools`.
- ADR-047 — Obsidian writer reads `[obsidian.vaults]` + `[obsidian.notes]`.
- ADR-048 — proxy commands read `[editor]` `[diff]` `[pr]`.
- ADR-049 — secrets read `[secret]`.
