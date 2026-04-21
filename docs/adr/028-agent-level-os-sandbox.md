# ADR-028: Agent-Level OS Sandbox

**Status:** Accepted
**Date:** 2026-04-21

## Context

Directive D6: agent-local sandbox modes are orthogonal to af's VM/container
isolation layer. Verified on the developer's machine:

- **Codex:** `codex sandbox {macos|linux|windows}` (Seatbelt / bubblewrap or
  Landlock / restricted token) and `-s/--sandbox <MODE>` policy flag.
- **Claude:** `--dangerously-skip-permissions` help text states *"Recommended
  only for sandboxes with no internet access"* — implying Claude defers to
  the caller's OS sandbox and does not provide one.
- **Amp, Gemini, Copilot, Pi:** no CLI sandbox flag discovered.

End goal D7's "isolation for overnight" pillar requires this. `af create
--yolo` on a local bare host today gives the agent unfettered write access.
Even without af's VM sandbox, the agent's own OS sandbox provides meaningful
defense.

## Decision

Add a new CLI flag:

```
af create --agent-sandbox <none|os>
```

- **Default:** `os` when the agent supports it, `none` otherwise.
- **Per-agent mapping:**

  | Agent | Mapping |
  |---|---|
  | codex | append `-s workspace-write` (or the codex default-safe policy — the exact mapping is reserved to the per-agent module and is subject to testing) |
  | claude | **no-op** — claude defers sandboxing to the caller; af does not need to pass a flag. Documented in `book/src/agents/claude.md` |
  | amp, gemini, copilot, pi | no-op; `--agent-sandbox=os` silently degrades to `none` with an info-level tracing log |

- **Orthogonal to the VM-sandbox layer.** Composes as:

  ```
  af create --sandbox --agent-sandbox=os --yolo   # belt + suspenders
  af create --agent-sandbox=os --yolo             # local, protected
  af create --yolo                                # warn (G16 guard)
  ```

## Alternatives considered

- **Implicit "always on when available."** Rejected: no opt-out for power
  users whose agent sandbox conflicts with a legitimate tool (for example,
  codex's Seatbelt blocking project-local binaries). `--agent-sandbox=none`
  is the documented escape.
- **Map to the agent's full native flag surface** (codex's `read-only` vs
  `workspace-write` vs `danger-full-access`). Rejected for 0.1.0: the
  tri-state complexity buys little; `os` vs `none` covers the common case.
  0.2.0 can add `--agent-sandbox-policy` if users ask.

## Consequences

- Lane L-AGENT-SANDBOX is small (<50 LOC plus per-agent tests).
- The G16 guard ("warn when `--yolo` has no sandbox") becomes actionable:
  "run with `--agent-sandbox=os` or `--sandbox`."
- Users on agents without OS sandbox support lose no functionality — they
  silently get `none`. `af doctor` can surface "your agent does not support
  OS sandbox" as an info message.
- Overnight-yolo has a documented safe-path configuration in the book.
