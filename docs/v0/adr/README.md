# Architecture Decision Records

ADRs follow the [Michael Nygard format](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions).
Each decision is numbered and immutable once accepted. Superseded or amended decisions link to their replacement.

| ADR | Title | Status |
|---|---|---|
| [001](001-agent-provider.md) | Agent Provider Abstraction | Accepted |
| [002](002-multiplexer-abstraction.md) | Terminal Multiplexer Abstraction | Accepted |
| [003](003-configuration-system.md) | Layered Configuration System | Accepted |
| [004](004-remote-provider.md) | Remote Execution Provider | Accepted |
| [005](005-sandbox-provider.md) | Sandbox Provider | Accepted |
| [006](006-session-metadata.md) | Session Metadata & Persistence | Accepted |
| [007](007-obsidian-integration.md) | Workstream Documentation (Obsidian) | Accepted |
| [008](008-phased-delivery.md) | Phased Delivery Strategy | Accepted |
| [009](009-provisioning.md) | Provisioning System | Accepted |
| [010](010-platform-deps.md) | Platform-Aware Dependency Management | Accepted |
| [011](011-workstream-lifecycle.md) | Workstream Lifecycle & Session Ledger | Accepted |
| [012](012-approval-modes.md) | Tri-State Approval Mode | Accepted |
| [013](013-local-wiki-abstraction.md) | Local Wiki Abstraction | Accepted |
| [014](014-three-layer-composition.md) | Three-Layer Composition Model | Accepted |
| [015](015-subagent-coordination.md) | Subagent Coordination Patterns | Accepted |
| [016](016-secret-storage.md) | Secret Storage for `af auth` | Accepted |
| [017](017-remote-resume.md) | Remote Session Resume & Reconnect Strategy | Accepted |
| [018](018-external-tool-testing.md) | External Tool Dependency Testing | Accepted |
| [019](019-remote-editor-urls.md) | Remote Editor URL Scheme Strategy | Accepted |
| [020](020-mdbook-structure.md) | mdBook User Guide Structure + Machine Index | Accepted |
| [021](021-release-discipline.md) | Release Discipline & CHANGELOG-Driven Notes | Accepted |
| [022](022-cmux-multiplexer.md) | cmux Multiplexer Provider | Accepted |
| [023](023-sandbox-agent-layer-conflict.md) | Sandbox Agent-Layer Conflict Resolution | Accepted |
| [024](024-remote-sandbox-daemon-url.md) | Remote Sandbox via Daemon URL (supersedes ADR-014 §"Composition model" for slicer) | Accepted |
| [025](025-secret-boundaries.md) | Secret Boundaries (extends ADR-016) | Accepted |
| [027](027-remote-ssh-target.md) | Remote = SSH Target (supersedes parts of ADR-004, ADR-017) | Accepted |
| [028](028-agent-level-os-sandbox.md) | Agent-Level OS Sandbox | Accepted |
| [029](029-external-tool-testing-addendum.md) | External Tool Testing — `CommandRunner` Dropped (addendum to ADR-018) | Accepted |
| [030](030-skill-bundle-installer.md) | `af` Skill Bundle — URL-Driven Claude Code Skill Installer | Accepted |

**ADR-026** was drafted (provider-specific liveness) but folded into ADR-027 during the Phase II.5 revision round; it never landed as an independent ADR.
