---
adr: 035
title: "CLI Framework — cobra + pflag"
status: proposed
implementation: in-progress
date: 2026-05-06
last_modified: 2026-05-21
supersedes: []
superseded_by: null
related:
  ["031", "034", "044", "045", "046", "054", "055", "056", "057", "058", "059"]
tags: ["go", "cli", "cobra"]
---

# ADR-035: CLI Framework — cobra + pflag

## Context

The owner asked for shell-completion support as a hard requirement. The
stdlib `flag` package does not generate completions; we'd have to hand-
roll one per shell (bash, zsh, fish, powershell), and keep them in sync
with the command tree. That's a maintenance burden out of scope for a
single-user tool.

Three Go CLI libraries have first-class completion generation:

1. **`spf13/cobra`** + `pflag` — most widely used, used by `kubectl`, `helm`, `gh`, `hugo`. Generates completions for bash/zsh/fish/powershell out of the box.
2. **`urfave/cli/v2`** — lighter footprint, also has completion support but with smaller ecosystem and slightly different idioms.
3. **`peterbourgon/ff/v3`** — minimalist, no completions out of the box.

## Decision

Adopt **`github.com/spf13/cobra`** (transitive `github.com/spf13/pflag`)
as the CLI framework.

### Why cobra

- **Completions are a single line**: `rootCmd.GenBashCompletion(os.Stdout)`, etc. ADR-045 (`af setup`) and ADR-035 here both depend on this generator.
- **Mature ecosystem**: every Go agent the owner uses (`gh`, `kubectl`, etc.) is built on cobra; the idioms are familiar.
- **Single dep tree**: `cobra` + `pflag` + `spf13/cast` (transitive) is the entire chain. No further indirect runtime deps.
- **Help text is automatic**: `--help` on every subcommand without writing it.

### Command tree

**ADR-035 is the authoritative CLI contract.** SPEC §3, README, and any
per-command ADR must agree with this tree; if a per-command ADR adds a
flag, this tree is updated in the same commit batch (see
`docs/CONVENTIONS.md` §"Command-surface drift"). Currently:

```
af [--verbose|-v] [--config PATH] [--session NAME]
├── version
│
│   # Lifecycle (ADR-046)
├── create [name] [--from BRANCH] [--current] [--from-pr N] [--bare]
│         [--remote HOST] [--sandbox PROVIDER] [--agent NAME]
│         [--yolo] [--auto]
├── done [session] [--force]
├── suspend [session]
├── resume [session] [--bare] [--respawn]
├── session-branch
│
│   # Multi-agent (ADR-039)
├── agent
│   ├── add [--slot NAME] --agent PROVIDER [--session NAME]
│   ├── stop SLOT [--remove-worktree] [--session NAME]
│   └── list [--session NAME]
│
│   # Inspection (ADR-054, ADR-055)
├── list                           # one-line per workstream, current repo first
├── status [--json] [--all] [--filter STATE]
├── info [session] [--json] [--ledger N]
│
│   # Reaping (ADR-056) — supersedes the v0/early-v1 `af gc`
├── clean [--dry-run] [--include-abandoned] [--max-age DURATION]
│         [--force [<name>...]]
│
│   # Stacking (ADR-059)
├── stack [session] [--parent PARENT]
├── unstack [session]
├── sync [session]
│
│   # Environment / setup (ADR-044, ADR-045)
├── setup [--force] [--shell SHELL] [--skip-completions] [--skip-gitignore]
├── doctor [--remote HOST] [--verbose]
│
│   # Notes / Obsidian (ADR-047, ADR-058)
├── note [session] [--append TEXT]
├── retro [--since DURATION] [--tag TAG]... [--search QUERY] [--ai]
│         [--limit N]
│
│   # Proxy commands (ADR-048, ADR-057)
├── editor [--terminal|-t|--visual|-v] [session]
├── diff [session] [--base REF]
├── pr [session] [--title T] [--draft] [--web] [--ai] [--ai-model MODEL]
│
│   # Secrets (ADR-049)
├── auth
│   ├── set <key>
│   ├── get <key>
│   ├── status
│   ├── clear <key>
│   └── list
│
│   # Config + completions
├── config
│   ├── show
│   └── init
└── completions <bash|zsh|fish|powershell>
```

Notes on the surface:

- `--session NAME` is a **persistent flag on the root command** so any
  subcommand that reads workstream state can target a workstream other
  than the one resolved by the discovery rules in ADR-037 §"File-discovery
  rules." Subcommands that don't apply (e.g. `version`, `setup`,
  `completions`, `doctor`) ignore it.
- `agent add --slot` is **optional**. If omitted, `af` auto-assigns from
  the agent's binary name (`pi`, `pi-2`, …) per ADR-039.
- `agent stop` accepts `--remove-worktree` per ADR-039.
- `note --append <text>` is the agent-side hook surface from ADR-047.
- `clean` replaces the early-v1 `gc` per ADR-056; the rename is
  documented in ADR-056 §"Supersession of `af gc` mention in ADR-038."
- `mangen` is **not** included for v1 (no man pages — single-user, no
  distribution). If users want one, `cobra-cli gen man-page` can be
  invoked manually.

### Single-source rule for command-surface drift

When a new ADR adds, removes, or modifies a command/flag:

1. The ADR's `## Decision` section lists the exact CLI surface.
2. The same commit batch updates this command tree, `docs/SPEC.md` §3,
   and `README.md` to match. Drift between ADRs and these surfaces is
   a review blocker.
3. The new/changed ADR carries `035` in its `related:` frontmatter to
   make the dependency explicit.

This convention is also captured in `docs/CONVENTIONS.md`.

### Idioms

- **Each subcommand is a constructor function** in `cmd/af/<cmd>.go` that returns `*cobra.Command`. No `init()` registration.
- **Args bound by struct**: each subcommand defines a private `<cmd>Opts` struct; flags bind to its fields via pflag.
- **Validation in `RunE`**: subcommands return `error`, never panic. `RunE` (not `Run`) is always used.
- **Context flows from `ExecuteContext`**: the root `main()` calls `root.ExecuteContext(ctx)`; each `RunE` calls `cmd.Context()` to get the cancellation-aware context.
- **No global flags via `pflag.CommandLine`**: all flags are bound to specific subcommands or to the root via `root.PersistentFlags()`.
- **Completion bindings**: `--from` etc. attach completion functions via `cmd.RegisterFlagCompletionFunc("from", completeBranches)` so tab-completion offers branch names from the current repo.

### Sketch

```go
// cmd/af/create.go
func newCreateCmd() *cobra.Command {
    var opts createOpts
    cmd := &cobra.Command{
        Use:   "create [name]",
        Short: "Create a new workstream: branch, worktree, tmux, primary agent",
        Args:  cobra.MaximumNArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            if len(args) == 1 {
                opts.name = args[0]
            }
            return runCreate(cmd.Context(), &opts)
        },
    }
    cmd.Flags().StringVar(&opts.from, "from", "", "fork from this branch")
    cmd.Flags().BoolVar(&opts.current, "current", false, "fork from the current branch")
    // ...
    return cmd
}
```

## Consequences

- One runtime dep (`cobra`) plus its transitive `pflag` and `cast`.
- Shell completions are generated, not hand-rolled.
- `--help` output is consistent across subcommands.
- Subcommand authors have a familiar template; new commands are mechanical to add.
- The `af` binary's startup is slower than a `flag`-based equivalent by single-digit milliseconds — irrelevant for an interactive tool.

## Alternatives Considered

- **stdlib `flag`** — rejected, no completion generator, manual help text.
- **`urfave/cli/v2`** — rejected. Equally capable but smaller ecosystem and fewer idioms the owner already knows.
- **`peterbourgon/ff/v3`** — rejected. Minimalist; lacks the completion generator that's the deciding feature.
- **No framework at all** (hand-roll everything) — rejected for the same reason; we'd build a mini-cobra anyway.

## References

- [`spf13/cobra` documentation](https://github.com/spf13/cobra/blob/main/site/content/docs/concepts/intro.md)
- [`spf13/cobra` shell completions](https://github.com/spf13/cobra/blob/main/site/content/docs/concepts/completions/_index.md)
- ADR-031 — v1 master.
- ADR-034 — Go module layout (cobra registration in main, not init).
- ADR-045 — `af setup` invokes `cobra` completion generator at install time.
