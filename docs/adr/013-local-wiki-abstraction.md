# ADR-013: Local Wiki Abstraction (Obsidian as Knowledge Base)

**Status:** Accepted
**Date:** 2026-04-10

## Context

Agentic workflows generate valuable context: decisions, plans, notes, memories, state, and
guidance. This information lives across multiple places — agent session logs, git commit
messages, PR descriptions, developer notes — with no unified access layer.

Obsidian is the owner's weapon of choice for knowledge management. It stores everything as
plain markdown files in a vault (filesystem directory). This makes it agent-friendly:
any tool that can read/write files can interact with Obsidian notes.

The current Obsidian integration (ADR-007) is session-scoped: one note per workstream with
YAML frontmatter for metadata. But the value goes beyond session tracking:

1. **Capturing** — agent observations, research findings, architectural decisions
2. **Notes** — working notes during development (scratchpad for the human + agent)
3. **Memories** — persistent knowledge that survives session boundaries
4. **State** — current project status, blockers, priorities
5. **Guidance** — instructions and preferences for agent behaviour

## Decision

### Obsidian as the local wiki layer

`af` treats Obsidian as a **local markdown wiki** — an abstraction over a directory of
markdown files with YAML frontmatter. The abstraction is not Obsidian-specific; any
markdown-file-based system works (Dendron, Foam, plain files).

### Note categories

| Category | Path in vault | Created by | Used by |
|---|---|---|---|
| **Workstream notes** | `workstreams/<session>.md` | `af create` | `af note`, `af done` |
| **Decisions** | `decisions/<repo>/<topic>.md` | Manual or agent | Agent context loading |
| **Memories** | `memories/<repo>.md` | Agent (append-only) | Agent session startup |
| **State** | `state/<repo>.md` | `af create`, `af done` | Agent context loading |
| **Guidance** | `guidance/<topic>.md` | Manual | Agent system prompt injection |

### Interface

The `obsidian` module provides generic markdown file operations:

```rust
pub fn note_path(config: &ObsidianConfig, category: &str, name: &str) -> Result<PathBuf>;
pub fn create_note(path: &Path, meta: &NoteMeta) -> Result<()>;
pub fn update_status(path: &Path, status: &str, completed_at: Option<DateTime>) -> Result<()>;
pub fn open_note(path: &Path) -> Result<()>;
pub fn read_note(path: &Path) -> Result<String>;
pub fn append_to_note(path: &Path, content: &str) -> Result<()>;
```

### Configuration

```toml
[obsidian]
enabled = true
vault = "~/Vaults/work"          # path to vault root
folder = "workstreams"           # default subfolder for session notes
```

The vault path is the only required setting. Categories map to subfolders within it.

## Consequences

- Obsidian becomes the durable memory layer across sessions and agents.
- Any markdown-based wiki works — the abstraction is filesystem operations on `.md` files.
- Agents can read guidance notes at session start for project-specific instructions.
- The append-only memories pattern avoids conflict between human and agent edits.
- Future: Dataview queries can aggregate workstream metadata across the vault.
