# Obsidian examples (ADR-047, issue #34)

`af create` writes one note per workstream into the configured vault via
the disk-backed note store. Configure the vault in
`~/.config/af/config.toml`:

```toml
[obsidian]
notes_vault          = "personal"          # key from [obsidian.vaults]
notes_folder         = "00 - workstreams"  # subfolder inside the vault
notes_subfolder_mode = "repo"              # "repo" (default) nests notes per-repo; "flat" is the pre-issue-#34 layout

[obsidian.vaults]
personal = "/Users/owner/Vaults/personal"
```

With that config and repo `github.com/owner/repo`, `af create fix-parser`
produces `/Users/owner/Vaults/personal/00 - workstreams/repo/fix-parser.md` —
notes nest under a subfolder named after the last path element of the
repo slug (`repo`), so every project's notes stay separate and a
workstream name can never create stray directories of its own (a
session named `team/x` becomes the file `team-x.md`, not a `team/`
subdirectory). Set `notes_subfolder_mode = "flat"` to go back to the
pre-issue-#34 layout, where every note lands directly under
`notes_folder` regardless of repo:
`/Users/owner/Vaults/personal/00 - workstreams/fix-parser.md`.

The note's frontmatter contract is unaffected by the layout change:

```markdown
---
af_started_at: 2026-07-03T12:00:00Z
af_completed_at: null
af_session: fix-parser
af_repo: github.com/owner/repo
af_branch: fix-parser
af_base_branch: main
af_status: active
af_pr_url: ""
af_pr_state: ""
af_agents:
    - slot: primary
      provider: pi
      status: running
tags: []
af_tags: []
af_schema: 1
af_pr_number: 0
---
# fix-parser

- Branch: `fix-parser`
- Worktree: `~/Workspace/.worktrees/github.com/owner/repo/fix-parser`
- Primary agent: `pi`

## Notes

```

The frontmatter keys (`af_*`, `af_schema: 1`) are the ADR-047 contract;
Obsidian Bases can filter on them (e.g. `af_status`, `af_tags`,
`af_pr_state`). The body below the frontmatter is owner-editable — af
only ever rewrites the frontmatter block.

When `notes_vault` is empty (the default), note creation is skipped
entirely, and `af create` prints a one-line warning to stderr so the
skip is never silent (issue #17):

```
note: Obsidian integration is disabled (notes_vault is empty — set [obsidian] notes_vault in ~/.config/af/config.toml)
```

The `af config init` template itself no longer ships a fixed
`/Users/owner` placeholder path under `[obsidian.vaults]` — the
commented-out example paths are generated from the real `$HOME` of
whoever runs `af config init`.
