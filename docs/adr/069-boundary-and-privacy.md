---
adr: 069
title: "Boundary & Privacy — Telemetry, Multi-Machine, Name Collisions"
status: accepted
implementation: complete
date: 2026-05-21
last_modified: 2026-07-03
supersedes: []
superseded_by: null
related: ["031", "037", "038", "040", "041", "047", "063"]
tags: ["go", "privacy", "telemetry", "multi-machine", "naming", "boundary"]
---

# ADR-069: Boundary & Privacy — Telemetry, Multi-Machine, Name Collisions

## Context

Three boundary concerns are implicit across v1 ADRs but never made
explicit:

1. **Outbound network calls.** ADR-031's scope cut implies "no
   release machinery, no fleet calls", but no ADR forbids
   version-check pings or telemetry. As `af` matures, the temptation
   to phone home grows; the contract is worth pinning now.
2. **Multi-machine model.** ADR-041 (SSH remote) and ADR-063
   (Tailscale + superterm) enable remote *execution* and remote
   *attach*, but neither says where canonical workstream state
   lives. Two laptops cloning the same repo could each create
   `state.toml`s for the same conceptual workstream.
3. **Workstream name collisions.** ADR-038 sanitises names for tmux
   but does not address "two repos both want
   `kakkoyun--issue-42`". Session directories under
   `~/.local/share/af/v1/sessions/` are flat; collisions are silent
   today.

Each is small individually; they share a boundary theme ("what does
`af` and does it not do, across the network and across machines"),
so this ADR groups them.

## Decision

### 1. Telemetry / privacy promise

`af` makes **zero** outbound network calls from its own code. The
only network traffic during an `af` invocation comes from
sub-processes the user explicitly invoked:

- `git` (fetch / push / clone / ls-remote — when the user runs
  `af create`, `af sync`, `af pr`).
- `gh` (PR query / create — when the user runs `af pr`, `af status`,
  `af clean`, `af info`, `af done`; rate-limited by ADR-071's TTL
  cache).
- `ssh` (when the user passes `--remote HOST`).
- `slicer` / `tailscale` / agent CLIs (`pi`, `claude`, `codex`) —
  when the user explicitly invokes them.

Specifically, `af` does **not**:

- Check for newer `af` versions on launch.
- Emit crash reports, usage counters, or analytics events.
- Phone home for license or entitlement checks.
- Send anonymous telemetry of any kind.

This contract is enforced at code-review time: any new outbound HTTP
call from `af`'s own code is a `gosec` violation **and** a review
blocker; it requires an explicit amending ADR.

### 2. Multi-machine state model — single-machine canonical

Canonical workstream state — `state.toml`, `ledger.jsonl`, secrets
envelope, Obsidian frontmatter — lives on **one machine per
workstream: the host that ran `af create`**. There is no
synchronisation across machines.

Operations that **mutate** workstream state always run on the host:

- `af create`, `af suspend`, `af resume`, `af done`, `af clean`.
- `af note --append`, `af agent {add,stop}`, `af stack`, `af sync`,
  `af pr`, `af session-data sync`.
- All ledger writes.

`af control up --remote HOST` (ADR-063) exposes the host's tmux
server via Tailscale Serve + superterm for **read-attached**
browsing. The attaching machine renders tmux panes in a browser;
agents are interacted with through that surface, not through a
second `af` install. The attaching machine's `af` (if any) operates
on its own local sessions.

The escape hatch when you actually want to see the host's
workstreams from another box:

```bash
ssh creator-host af list
ssh creator-host af status --json | jq
```

This is a deliberate boundary. Multi-machine canonical state would
require a sync layer (rsync over Tailscale, S3, or a daemon),
cross-host locking, conflict resolution, and atomic
operation-across-hosts semantics — all of which are vastly out of
proportion for a single-user tool. If the use case materialises, a
future ADR designs it from first principles.

Implications:

- Each machine maintains its own independent
  `~/.local/share/af/v1/sessions/` tree. Workstream names are not
  shared across machines.
- Obsidian frontmatter `af_session` values are not guaranteed unique
  across machines (only within a single host's sessions directory).
  If the user maintains a multi-machine Obsidian vault, `af_repo` +
  `af_branch` are the unique-enough composite key.
- ADR-063's `af control status --remote HOST` queries the remote
  host's superterm; it does not consult any local `state.toml`.

### 3. Workstream name collisions

Workstream names are **globally unique** within a single host's
`~/.local/share/af/v1/`, including the `archive/` subdirectory. That
is, the same name cannot appear simultaneously in `sessions/` and
`archive/`, and it cannot appear in `sessions/` for two different
repos.

`af create <name>` checks both `sessions/<name>/` and
`archive/<name>/` before creating. On collision:

```text
workstream "kakkoyun--issue-42" already exists for repo kakkoyun/af
(status: active, last touched 2 hours ago).

Pick a different name, or run 'af done kakkoyun--issue-42' first
to free the name.
```

If the existing workstream is archived, the message reads
`(status: archived, completed 2026-05-10)` and the suggested
remediation is `pick a different name, or move the archive aside
manually.` No automatic suffixing (`-2`, `-3`) and no
auto-renaming — explicit wins.

The collision rule applies to the **directory name**, which is also
the tmux session name (ADR-040), the Obsidian frontmatter
`af_session` value (ADR-047), and the foreign key in the per-repo
discovery symlink (ADR-038). The underlying *branch* stays
per-repo and is unaffected.

Exit code: `EX_DATAERR` (65), per ADR-068 §2.

## Consequences

- The privacy promise is a marketing-grade claim the owner can make
  confidently. Any drift becomes a review blocker.
- The single-machine model keeps the data plane simple; no daemon,
  no sync, no conflict resolution.
- Multi-host usage is supported via `ssh host af ...` (the escape
  hatch) and `af control up` (the attach path), both of which fit
  inside the contract.
- Name collisions fail loudly at `af create` rather than corrupting
  state silently.
- A future need for cross-machine fleets is not foreclosed; this
  ADR simply punts the decision.

## Alternatives Considered

- **Multi-machine read-only fanout.** A new `[peers]` config section
  letting `af list --host all` aggregate from configured Tailscale
  peers. Rejected for v1: introduces failure modes (peer
  unreachable), schema changes (`host:` qualifier), and a network
  surface that violates §1 of this ADR. Revisit if the user's
  workflow genuinely needs it.
- **Full state-sync across machines.** Rejected: complexity orders of
  magnitude beyond v1's scope; no compelling single-user use case.
- **Workstream name auto-suffixing on collision** (`-2`, `-3`).
  Rejected: surprises the user, drifts the tmux session name from
  the workstream name. The strict-fail model matches the
  "explicit > implicit" pattern in ADR-038 (branch prefixes, fork
  source flags).
- **Repo-prefixed workstream names by default**
  (`<repo-slug>--<name>` everywhere). Rejected: makes tmux session
  names noisy; muscle-memory cost; double-prefix edge cases for
  names that already contain slashes.
- **Allow archived-name reuse.** Rejected for now: `af retro`
  reaches into the archive and benefits from a stable name → notes
  mapping. If the user genuinely wants to reuse an archived name,
  the manual step is `mv archive/foo archive/foo-old` and try again.

## References

- ADR-031 — v1 master, scope reduction, single-user posture.
- ADR-037 — state.toml + ledger layout (per-host).
- ADR-038 — naming, sanitisation, per-repo discovery symlink.
- ADR-040 — tmux session name == workstream name.
- ADR-041 — SSH remote model; `ssh host af ...` is the escape hatch.
- ADR-047 — Obsidian frontmatter `af_session` key.
- ADR-050 — `gosec` enforces no-outbound-call at lint time.
- ADR-063 — `af control` via Tailscale + superterm; the read-attach
  surface that prompted §2 of this ADR.
- ADR-068 — exit-code mapping for collision errors.
- `NO_COLOR` / privacy posture in operational contracts (ADR-068 §3).
