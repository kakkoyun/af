---
adr: 047
title: "Obsidian Integration — Notes + Bases"
status: proposed
implementation: in-progress
date: 2026-05-06
last_modified: 2026-05-20
supersedes: []
superseded_by: null
related: ["031", "036", "037", "045", "048", "054", "058"]
tags: ["go", "obsidian", "notes"]
---

# ADR-047: Obsidian Integration — Notes + Bases

## Context

The owner keeps notes in Obsidian. Each workstream is a unit of work
worth a markdown note: what was attempted, what worked, what didn't.
v0 had Obsidian integration (ADR-007) but the agents themselves
couldn't easily append context. v1 fixes that and adds **Obsidian
Bases** support — Obsidian's native data-table feature that aggregates
markdown files by frontmatter.

The owner uses multiple vaults (personal, work). v1's
`[obsidian.vaults]` section (ADR-036) maps vault names to absolute
paths; the workstream-notes folder is `[obsidian].notes_folder` inside
the chosen vault.

## Decision

### Note path

For workstream `<session>` written to vault `<vault-name>` at folder
`<folder>`:

```
<obsidian.vaults[<vault-name>]>/<folder>/<session>.md
```

Example with vault `personal` and folder `00 - af`:

```
/Users/kemal/Vaults/personal/00 - af/kakkoyun--issue-42.md
```

### Frontmatter schema (`af_schema: 1`)

```yaml
---
af_schema: 1
af_session: kakkoyun--issue-42
af_repo: af
af_branch: kakkoyun/issue-42
af_base_branch: upstream/main
af_status: active # active | suspended | completed | abandoned
af_agents: # array, one entry per slot
  - slot: primary
    provider: pi
    status: running # running | stopped | crashed | suspended
  - slot: review
    provider: claude
    status: running
af_started_at: 2026-05-06T12:00:00Z
af_completed_at: null
af_pr_number: 0 # 0 = no PR
af_pr_url: ""
af_pr_state: "" # "" | "open" | "merged" | "closed"
tags: [af] # top-level tag for Obsidian Base filtering (taggedWith: af)
af_tags: [] # user-editable extension list (project, area, etc.)
---
```

The `af_*` prefix namespaces structured workstream fields so they
don't collide with user-added Obsidian fields (`aliases`, `cssclasses`,
etc.). Two tag-shaped fields exist deliberately:

- `tags` (top-level, value `[af]`): what the example Obsidian Base
  uses to identify workstream notes via `taggedWith: af`. Obsidian's
  built-in tag index also picks it up.
- `af_tags`: free-form user-editable list (e.g. `["work", "infra"]`).
  Not consumed by the example Base; users may write their own Base
  filtering on it.

### Body template

Initial body created by `af create`:

```markdown
# kakkoyun--issue-42

## Goal

_(describe what this workstream is for)_

## Context

_(starting point, base branch, related links)_

## Log

_(agent runs append here; user adds notes)_

- [pi] launched at 2026-05-06 12:00:00 UTC

## Outcome

_(filled in by `af done`)_
```

Configurable via `[obsidian].notes_template` — a path to a markdown
template; if set, that template is used instead of the default (with
the same frontmatter prepended).

### Lifecycle hooks

| Trigger         | Frontmatter change                                               | Body change                                        |
| --------------- | ---------------------------------------------------------------- | -------------------------------------------------- |
| `af create`     | Write full frontmatter                                           | Write template body                                |
| `af agent add`  | Append to `af_agents`                                            | Append `- [provider] launched at <ts>` to `## Log` |
| `af agent stop` | Update `af_agents[slot].status` (added)                          | Append `- [provider] stopped at <ts>` to `## Log`  |
| `af suspend`    | `af_status: suspended`                                           | Append `- session suspended at <ts>` to `## Log`   |
| `af resume`     | `af_status: active`                                              | Append `- session resumed at <ts>` to `## Log`     |
| `af done`       | `af_status: completed` (or `abandoned`); `af_completed_at: <ts>` | Append `- session ended at <ts>` to `## Log`       |
| `af pr`         | `af_pr_number`, `af_pr_url`, `af_pr_state`                       | Append `- PR opened: <url>` to `## Log`            |

All updates are best-effort: if the vault is unreachable (e.g. on a
remote machine without filesystem access to the local vault), the
operation logs a warning and continues. The workstream is the source
of truth; the note is a derived artefact.

### Frontmatter library

`gopkg.in/yaml.v3` for parse/emit. Frontmatter is the YAML block
between `---` markers at the top of the file; the rest is opaque
markdown body that `af` only appends to (never rewrites).

### `af note [session]`

Opens the workstream's markdown note in:

1. **Obsidian itself**, via the `obsidian://open?vault=<vault>&file=<path>` URI scheme. macOS `open`, Linux `xdg-open` invoked.
2. **Fallback**: `$EDITOR` if Obsidian URI handling fails (e.g. on a remote machine without Obsidian installed).

### Agent-side context appendable

Agents may want to drop a paragraph into the workstream note ("here's
what I did, here's what's left"). v1 exposes this via:

- A small helper command: `af note --append "<text>"` appends `<text>` under `## Log` with a timestamp.
- An agent-side wrapper: agents that support hook scripts can invoke `af note --append "..."` from within their own session.

The hook integration is **opt-in**: agents don't auto-call this. The
owner configures it manually in their pi/claude/codex hook config.

### Obsidian Bases aggregator

`af` ships an example `.base` file at `examples/obsidian/active-workstreams.base`:

```yaml
filters:
  and:
    - taggedWith: af # matches the top-level `tags: [af]` field of every workstream note
    - or:
        - field: af_status
          equals: active
        - field: af_status
          equals: suspended
views:
  - type: table
    name: "Active workstreams"
    columns:
      - af_session
      - af_repo
      - af_branch
      - af_status
      - af_agents
      - af_started_at
      - af_pr_number
```

The user copies this file into their vault's preferred location
manually. `af setup` doesn't auto-install it (Bases location is a vault
preference; auto-installing risks overwriting the user's setup).

The Base displays a live table of active and suspended workstreams,
sortable by start date or PR number, with click-through to each note.

### Multi-vault support

`[obsidian.vaults]` may declare multiple vaults; `[obsidian].notes_vault`
chooses one as the **default destination** for new workstream notes.
A repo can override with its own `[obsidian].notes_vault` (per ADR-036
layered config) — useful when a work repo's notes should land in the
work vault, not the personal one.

### Disable

If `[obsidian.vaults]` is empty and `[obsidian].notes_vault` is unset,
all Obsidian writes are skipped (with a one-time `slog.Info` per `af`
invocation). `af note` returns "no vault configured; run `af setup` for
a hint."

## Consequences

- Every workstream produces a queryable note.
- Bases aggregator gives an at-a-glance dashboard with no extra code in `af`.
- Agents can drop context into the note via a single helper command.
- Multi-vault support means personal-vs-work routing works without per-command flags.

## Alternatives Considered

- **Single hard-coded vault.** Rejected; the owner uses multiple vaults.
- **Dataview dashboards instead of Bases.** Rejected; Bases is Obsidian's first-party feature, no plugin needed.
- **Agent-side native integration (each agent writes its own note).** Rejected; one note per workstream is the right granularity, and agents shouldn't need to know about each other.
- **Embed the Base file as a string and write it to the vault on `af setup`.** Rejected; touching the vault from `af setup` is invasive. The owner copies it once.

## References

- v0 ADR-007 — superseded for v1.
- ADR-031 — v1 master.
- ADR-036 — `[obsidian.vaults]` config schema.
- ADR-037 — workstream lifecycle (status transitions trigger note updates).
- ADR-045 — `af setup` prints vault hint.
- ADR-048 — `af pr` updates `af_pr_*` fields.
