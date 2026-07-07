# Obsidian examples (ADR-047)

`af create` writes one note per workstream into the configured vault via
the disk-backed note store. Configure the vault in
`~/.config/af/config.toml`:

```toml
[obsidian]
notes_vault  = "personal"       # key from [obsidian.vaults]
notes_folder = "00 - af"        # subfolder inside the vault

[obsidian.vaults]
personal = "/Users/owner/Vaults/personal"
```

With that config, `af create fix-parser` produces
`/Users/owner/Vaults/personal/00 - af/fix-parser.md`:

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
entirely.
