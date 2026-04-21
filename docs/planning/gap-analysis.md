# Phase III Gap Analysis — Pattern-Hardening Sprint

**Status:** Draft — awaiting user approval before spawning Phase III lanes
**Date:** 2026-04-21
**Author:** planning session (Opus 4.7)

This document captures the research findings that surfaced after Lane D landed and
Phase II ADRs (015–021) were accepted. It exists to:

1. Reconcile the five accepted Phase II ADRs against the **actual** CLI surface of
   slicer, sbx, workspaces, and cmux.
2. Identify implementation drift in the existing `provider/slicer.rs` and
   `provider/docker.rs` that Phase III must fix, not paper over.
3. Propose concrete revisions: which ADRs to supersede, which to extend, which new
   ADRs to write, and how Phase III lane scope changes.

Planning docs under `docs/planning/` are **transient** — they are deleted or archived
once the work they describe has landed. Durable decisions live in `docs/adr/`.

---

## 0. User directives (2026-04-21, after initial review)

Three constraints from the user that narrow the revision scope:

**D1. Secrets are not a cross-cutting concern requiring af-level orchestration.**
Each provider owns its own secret store. `af auth` (ADR-016) manages the host
keyring for the local-agent-no-sandbox case only. No sync layer. No `af auth sync`.
When a user runs a sandboxed session, `sbx secret` / `slicer secret` /
`workspaces secrets` are the source of truth — af points the user at them,
doesn't manage them.

**D2. "Remote" = "SSH-able host." exe.dev is not special.**
Drop exe.dev-specific logic where possible. The generic pattern:
- `workspaces` and `exe.dev` handle their own VM lifecycle (create/delete)
- Post-create, both expose an SSH-reachable host
- af uses SSH as the common protocol (probe, session attach, file sync)
- Lane B4 (exe.dev SSH drop detection) disappears — it's Lane B3 (generic SSH probe)

**D3. tmux and cmux are interchangeable multiplexers.**
Selected via `[general] multiplexer = "tmux"|"cmux"` config. cmux is first-class,
not deferred. Research cmux's "good modes" (it advertises tmux-compat commands,
has `cmux ssh` for remote workspaces, claude-teams/omc/omo/omx for agent
launchers, RPC interface) and leverage what's useful. The `Multiplexer` trait
in `src/mux/mod.rs` should accommodate both without exposing workspace/surface
primitives that tmux lacks.

---

## 0.2 Second directive batch (2026-04-21, after critic synthesis)

Four more directives that expand the end-goal framing and surface gaps not in the
original §2 matrix:

**D4. Agent-native `--worktree` flags: keep af's abstraction.**
Verified on this machine: only `claude -w/--worktree [name]` exposes worktree
management as a CLI flag. Codex, Amp, Gemini, Copilot, and Pi do not.
**Decision:** af keeps ownership of `src/git/worktree.rs`. Delegating would cripple
5/6 agents. When Claude is launched, af's worktree is passed as the working
directory; `claude --worktree` is **suppressed** (we've already created it). No
feature flag, no trait change. Document this explicitly in the book's Claude page.

**D5. Permission modes are a first-class abstraction — articulate, don't rebuild.**
ADR-012 already defines tri-state `ApprovalMode::{Default, Auto, Yolo}`. Wired in
claude/codex/copilot; partial in amp/gemini/pi. Two research findings:

- **Claude's native modes are richer:** `default, acceptEdits, auto, plan,
  bypassPermissions, dontAsk` (six states). Our tri-state maps to three of six.
  `plan` mode (read-only, preview changes) has real user value for overnight runs
  where you want morning review without risk.
- **Codex's native modes are different again:** `--ask-for-approval
  <on-request|on-failure|never>` + `--full-auto` + `--dangerously-bypass-approvals-and-sandbox`.
  Tri-state already covers the common cases.

**Decision:** Keep tri-state for 0.1.0. Add `Plan` as an optional fourth state in
0.2.0 if users ask for it. Document the per-agent mapping table (currently only
living in ADR-012) as a book page under `concepts/approval-modes.md`. This
surfaces af's opinion: "three levels, agents degrade gracefully."

**D6. Agent-local sandbox modes are orthogonal to af's VM sandboxes.**
Codex exposes `codex sandbox {macos|linux|windows}` — process-level confinement via
Apple's Seatbelt, Linux bubblewrap/landlock, or Windows restricted tokens. Also
`-s/--sandbox <SANDBOX_MODE>` for the integrated policy. Claude's
`--dangerously-skip-permissions` help text reads: *"Recommended only for sandboxes
with no internet access"* — implying Claude defers network-isolation to the
caller's environment.

This is **a second, finer-grained isolation layer** that sits **below** af's
remote+sandbox composition:

```
  ┌─ af VM/container sandbox (slicer microVM, sbx Docker) ← isolates the whole host
  │    └─ af worktree inside VM
  │         └─ agent process
  │              └─ agent-native OS sandbox (Seatbelt / bubblewrap / Landlock) ← isolates the agent's spawned commands
```

**Decision:** af's `--sandbox` flag stays VM/container-scoped. Add a **new
orthogonal flag** `--agent-sandbox=<none|os>` (default: `os` when the agent
supports it, `none` otherwise) that maps to the agent's native OS sandbox mode
for any run — local, remote, or inside a VM sandbox. On codex this sets
`-s workspace-write` (or similar safe default); on claude this is a no-op (handled
internally). This gives defense-in-depth for free on the local-agent case which
is otherwise the most exposed mode. See new ADR-028 in §8.3 revised slate.

**D7. End goal articulation: af = single entrypoint for agentic workflow.**
Explicit north star (overrides any scope creep):

> af is a single entrypoint that streamlines coding **and writing** workflows
> using AI agents — managing agents, worktrees, notes/memory/wiki, and isolation
> suitable for overnight unattended runs.

Four pillars, four abstractions:

| Pillar | Primary abstraction | Status |
|---|---|---|
| **Agents** | `AgentProvider` trait (ADR-001), 6 providers | Done |
| **Worktrees** | `src/git/worktree.rs` + session branch naming | Done; D4 confirms keep |
| **Notes / memory / wiki** | `src/obsidian/` + ADR-007 + ADR-013 (local wiki) | Done; may need polish in book |
| **Isolation for overnight** | **Composition matrix:** yolo × agent-sandbox × VM-sandbox × remote | **Partially done; needs D6 finish** |

What "overnight" actually requires (derived from D7):

1. **Unattended approval:** `ApprovalMode::Yolo` + `--agent-sandbox=os` (D6) as
   the safe default when yolo is selected on a local box.
2. **Isolation strong enough to leave running:** yolo-on-local-no-sandbox should
   warn loudly; yolo-with-agent-sandbox should be the smooth path; yolo-with-VM
   is the paranoid path.
3. **Liveness + resume:** ADR-017 probe + reconnect.
4. **Notes + ledger** so morning review shows what happened (already in place via
   `af stats`, `af export`, Obsidian frontmatter).
5. **Notifications** on session-complete / agent-stop (already wired per
   `Backlog` item "Remote control: superterm notification integration" — marked
   done).

**Implication for Phase III:** `af create --yolo` on a local host without a
sandbox should emit a security warning unless `--agent-sandbox=os` is active or
the user passes an opt-out. This is policy, not code — a two-line guard plus a
book section on "Recommended overnight configuration."

---

## 1. CLI Surface Reference (verified 2026-04-21 on this machine)

### slicer (installed)

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

**Implication:** "remote slicer" is **just "pass `--url`"** — no SSH install step, no
separate provisioning pipeline. The user's shell already wraps this:
`slicer --remote=<host> <cmd>` → `command slicer <cmd> --url <resolved-url> --token-file ...`.

### sbx (installed)

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

**Agents supported by `sbx run`:** `claude, codex, copilot, docker-agent, droid,
gemini, kiro, opencode, shell`.

**Critical quote from `sbx secret --help`:**

> When a sandbox starts, the proxy uses stored secrets to authenticate API requests
> on behalf of the agent. **The secret is never exposed directly to the agent.**

### workspaces (DD, installed)

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

### cmux (installed)

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

## 2. Gap Matrix

Legend for severity:
**H** — blocks Phase III correctness;
**M** — shipped design is incomplete or misaligned with target tools;
**L** — nice-to-have extension.

| # | Area | Gap | Sev | Proposed Fix |
|---|------|-----|-----|--------------|
| G1 | ADR-014 `--sandbox --remote` | Says "sandbox provider creates VM on `host` via SSH" — **stale**. Slicer daemon mode (`--url`) makes this a one-arg change, no SSH or provisioning pipeline. | H | New ADR-024: Remote Sandbox via Daemon URL. Supersedes ADR-014 §"Composition model" for slicer. Lane B1 simplifies to "plumb `--url` through `SandboxConfig`." |
| G2 | ADR-016 Secret Storage | Only covers keyring → env-var injection. Misses `sbx secret` / `slicer secret` / `workspaces secrets` native stores. Injecting `ANTHROPIC_API_KEY` as env var **inside an sbx sandbox is redundant** — the proxy authenticates on the agent's behalf. | H | **Per D1**: no af-level sync. New ADR-025: **Secret Boundaries** (not "Strategies"). Narrow scope: (a) ADR-016 keyring applies ONLY to local-agent-no-sandbox; (b) sandboxed sessions defer to the provider-native store — `af doctor` / `af auth status` tells the user when a provider-native secret is missing, with the exact CLI command to set it; (c) no code-level sync logic. |
| G3 | `provider/slicer.rs` API choice | Uses `slicer vm add` (low-level) for `create()` + `slicer claude/codex/...` for agent launch — **two different slicer abstractions mixed**. Unclear whether `vm add` + `claude` produces the same shape VM as `slicer workspace .`, and `provision()` is a no-op. | M | New ADR-023: Sandbox Agent-Layer Conflict Resolution. Decide: (a) keep split (vm-add for lifecycle, agent-subcommand for launch) and document why, OR (b) switch to `slicer workspace .` + `slicer agent-cmd` composition. Affects provider/slicer.rs. |
| G4 | `provider/docker.rs::create()` | Calls `sbx create claude . --name <name>` — literal `"."` as path, ignores the passed-in session workspace. Also hardcodes `claude` regardless of agent. **This is a bug**, not just a gap. | H | Fix in Lane B1.5 (scope extension): pass `workdir` as positional arg, resolve agent from session config. Should be one commit with a regression test. |
| G5 | `provider/docker.rs` agent map | `KNOWN_SBX_AGENTS = ["claude", "codex"]` — misses `copilot, docker-agent, droid, gemini, kiro, opencode, shell`. Unknown agents silently coerced to `claude`. | M | Extend list; add pass-through for agents sbx supports but af does not (e.g., `droid`). Folds into G4 fix. |
| G6 | `provider/docker.rs` double-create | `SandboxProvider::create()` runs `sbx create`, then `agent_sandbox_cmd()` builds `sbx run`. `sbx run` also creates if absent — the `sbx create` call is redundant (and buggy per G4). | M | Drop `sbx create` from provider `create()`; make it idempotent "name only" and let `sbx run` (launched via mux) do the real work. |
| G7 | cmux as multiplexer | Not covered in any ADR. cmux explicitly advertises a "tmux compatibility commands" section (capture-pane, resize-pane, pipe-pane, swap-pane, break-pane, join-pane, next-window, etc.) + native `send` / `send-key` / `new-workspace` / `cmux ssh <dest>` / RPC interface. It is designed as a drop-in. | **H** per D3 | **Mandatory** new ADR-022: cmux Multiplexer Provider. Lane M1 promoted from optional to required for 0.1.0. Multiplexer factory auto-selects from `CMUX_WORKSPACE_ID` / `TMUX=` env or explicit config `[general] multiplexer = "cmux"\|"tmux"`. cmux's agent-opinionated subcommands (`claude-teams`, `omc`, `omo`, `omx`) are ignored by af's own agent abstraction — listed in ADR-022 §"Non-goals". |
| G8 | ADR-019 remote editors | VS Code / Cursor URL schemes specified. Misses `workspaces connect <name> --editor <e>` which is the DD-native flow for Workspaces sessions — simpler and respects DD's internal SSH config. | M | Extend ADR-019 with a `workspaces`-specific branch: when session has `remote.provider = "workspaces"`, use `workspaces connect` instead of URL scheme. Add as Lane B5 sub-spec. |
| G9 | Lane A1 scope assumption | Plan assumed building the DD Workspaces provider "from scratch." In reality, `workspaces` CLI already has `create`, `delete`, `list`, `restart`, `ssh-config`, `connect`, `secrets sync`, `dotfiles`. Lane A1 reduces to "wrap the CLI" — smaller than expected. | L | Update Lane A1 spec to enumerate exact wrapping points; call out that provisioning is a no-op (workspaces handles it). Document that DD policies (region, instance-type) are exposed through af config. |
| G10 | ADR-018 CommandRunner coverage | Defines the trait but the new agent/sandbox subcommand surfaces (`sbx run`, `slicer workspace`, `workspaces connect`) need reference snippets in the ADR or a companion `docs/reference/external-tools.md` so subagents know what args to stub in fakes. | L | Add `docs/reference/external-tools.md` with verified command signatures (a snapshot of §1 of this doc, durable). Not a new ADR — a reference. |
| G11 | ADR-017 probe assumption | `is_alive(host)` probes SSH directly. Fine for exe.dev. For DD Workspaces, a suspended VM is SSH-unreachable but is not an orphan. | L | **Per D2**: SSH probe stays universal. Suspended workspaces are handled in Lane A2 (orphan detection): before marking a remote session orphaned, check `workspaces list` for presence; if present but SSH-dead, mark "suspended" not "orphan". No new ADR; Lane A2 spec gains a paragraph. Drop proposed ADR-026. |
| G12 | Plan worktree naming | `docs/CONVENTIONS.md` lists A1/B1/... worktree names. No entry for M1 (cmux) if G7 is adopted. | L | Append M1 row; regenerate table. |
| G13 | Agent-native `--worktree` vs af's abstraction (D4) | Only `claude -w` exposes worktree management. 5/6 agents have no equivalent. Delegating would break the feature for amp/gemini/codex/copilot/pi. | L | **Decision:** keep af's `src/git/worktree.rs` abstraction. Suppress `claude --worktree` when launching claude (af's worktree wins). Document explicitly in `book/src/agents/claude.md`. No code change; policy + doc only. |
| G14 | Permission modes not surfaced in docs (D5) | ADR-012 tri-state (`Default/Auto/Yolo`) is wired in 3/6 agents but only documented inside the ADR. Users discover it through `--help`. Claude's native surface has 6 modes (`plan` mode is notably useful for overnight preview); codex's is different again. | M | No ADR change for 0.1.0. Add `book/src/concepts/approval-modes.md` (owned by Lane L-BOOK) with the per-agent mapping table lifted from ADR-012. Flag `Plan` as a 0.2.0 candidate. |
| G15 | Agent-native OS sandbox modes unused (D6) | `codex sandbox {macos\|linux\|windows}` + `-s/--sandbox` exposes Seatbelt / bubblewrap / Landlock / Win-restricted-token. Claude's `--dangerously-skip-permissions` help text assumes callers provide OS sandboxing. af currently ignores this entire layer. Overnight-yolo on a bare host is riskier than it needs to be. | **H** per D7 | New **ADR-028: Agent-Level OS Sandbox** — adds `--agent-sandbox=<none\|os>` flag (default `os` when supported). Per-agent mapping: codex → `-s workspace-write`; claude → no-op; others → no-op. Independent of af's VM/container sandbox layer. Enables D7 "safe overnight" pillar with two lines of code per supported agent. |
| G16 | `--yolo` on local-no-sandbox lacks safety rail (D7) | Today's `af create --yolo task` on a bare host gives the agent full write access to `$HOME` and the network. No warning, no guard, no opt-out. "Overnight isolation" pillar is broken here. | M | Policy guard in `src/cmd/create.rs`: when `ApprovalMode::Yolo` and no VM-sandbox and no agent-sandbox, print a warning with remediation (`--agent-sandbox=os` or `--sandbox`) and require `--i-know-its-risky` to proceed. Two-line change; owned by the lead in Phase IV (touches `cli.rs` + `create.rs`). |

---

## 3. Proposed ADR Slate (after Phase II.5)

| ADR | Title | Kind | Drives | Size |
|-----|-------|------|--------|------|
| 022 | cmux Multiplexer Provider | Design | G7 | M |
| 023 | Sandbox Agent-Layer Conflict Resolution | Design | G3 | S |
| 024 | Remote Sandbox via Daemon URL | Design | G1 | S (supersedes part of ADR-014) |
| 025 | Secret Injection Strategies | Design | G2 | M (extends ADR-016) |

Optional / reference-only additions:

- `docs/reference/external-tools.md` — not an ADR, a verified CLI-surface snapshot for G10.
- Amendments to ADR-017 and ADR-019: per the constitution, ADRs are immutable. Two options:
  (a) write a tiny new ADR (e.g., ADR-026: Provider-Specific Probe Strategy) that supersedes
  only the affected section of 017, or (b) treat the extension as part of ADR-025 /
  ADR-024. **Recommendation: (a)** — keep ADR boundaries clean.

Worst case this adds 5 ADRs (022–026); best case 4 (roll G11 into ADR-025).

---

## 4. Revised Phase III Lane Plan

Phase II.5 sits between the current Phase II (ADRs 015–021 accepted) and Phase III
(implementation). It is a design round, not a code round.

```
Phase II.5 — ADR Revision Round (parallel, no code changes)
  Lane D2 : docs/reference/external-tools.md + ADR-022 + ADR-023 + ADR-024 + ADR-025 [+ 026]
            (lead authors all five; no subagent split since these touch shared ADR
            numbering space, conflicts are cheap to serialize)

Phase III — Implementation (parallel, file-disjoint) — REVISED
  Lane A1  : DD Workspaces provider (wrap CLI; no SSH/provision logic — G9)
  Lane A2  : Orphan detection in `af list` (provider-specific probes — G11)
  Lane B1  : slicer --sandbox --remote via --url (G1 simplified)
  Lane B1.5: provider/docker.rs bug fixes + sbx agent list expansion (G4, G5, G6)
  Lane B2  : `af auth` setup/reroll/status/clear + keyring inject (original scope)
  Lane B2.5: Secret sync: keyring → sbx secret / slicer secret / workspaces secrets (G2)
  Lane B3  : Remote session resume (ADR-017; reuse ADR-026 probe if split)
  Lane B4  : exe.dev SSH liveness (folds into B3 as before)
  Lane B5  : `af editor` remote (URL schemes + workspaces-connect branch per G8)
  Lane C1  : mdBook user guide (original scope; autogen picks up new commands)
  Lane M1  : cmux multiplexer provider (NEW; parallel, file-disjoint under src/mux/)
  Lane S1  : provider/slicer.rs API choice per ADR-023 (G3)
            (may be no-op if ADR-023 ratifies current split)

Phase IV — Integration (lead-only, unchanged)
Phase V  — Release Gate (user-triggered, unchanged)
```

**New lanes:** B1.5, B2.5, M1, S1. Original count 8 → revised count 12.

**Parallelism map:** Phase III can still run 8-wide; the new lanes are file-disjoint
from each other (sbx, keyring/sbx/slicer/workspaces secrets, cmux, slicer API choice).

---

## 5. Code Drift To Repair (independent of ADR decisions)

Even without any new ADR, the following are code bugs that should land as fixes in
Phase III:

### 5.1 `provider/docker.rs::create()` — G4

```rust
// CURRENT (wrong)
.args(["create", "claude", ".", "--name", name])

// FIX
.args(["create", agent, workdir_str, "--name", name])
// where `agent` comes from session config and `workdir_str` is the session workspace
```

Red test: mock CommandRunner, assert `sbx create` is called with the session's
actual workdir, not `.`.

### 5.2 `KNOWN_SBX_AGENTS` — G5

Expand from `["claude", "codex"]` to the full set sbx supports, and add a pass-through
so agents not yet known to af don't get silently coerced.

### 5.3 Double-create — G6

Either:
- Remove `sbx create` from `SandboxProvider::create()` (let `sbx run` in
  `agent_sandbox_cmd` handle creation on first call), OR
- Keep `sbx create` but change `agent_sandbox_cmd` to use `sbx exec` + `sbx run <name>`
  (attach to existing).

The first option is simpler and matches sbx's own docs ("Use `sbx run SANDBOX` to
attach to the agent after creation").

### 5.4 `provider/slicer.rs` — depends on ADR-023

If ADR-023 ratifies the split ("vm add for lifecycle, claude/codex for launch"), no
code change. If it chooses `slicer workspace` as the entry point, `create()`
changes significantly.

---

## 6. Open Questions for User

The gap analysis is decision-ready. The user needs to pick on six items before we
spawn Phase II.5:

1. **cmux as Phase III lane or 0.2.0 backlog?** Adding Lane M1 extends the sprint
   by one lane. Alternative: stub ADR-022 as "deferred" and keep cmux out of 0.1.0.
2. **ADR split for ADR-017 extension (G11)?** (a) New ADR-026 superseding part of
   017 (clean), or (b) fold into ADR-025 (messy-ish). Recommendation: (a).
3. **Slicer API choice (ADR-023):** keep current split, or switch to
   `slicer workspace`? Current split is shipped and mostly working; switching is
   cleaner but touches `provider/slicer.rs` broadly.
4. **Secret strategy defaults (ADR-025):** when a session runs Claude in an sbx
   sandbox and the user has an ANTHROPIC key in keyring, do we (a) sync to sbx
   secret store automatically, (b) prompt on first create, or (c) require explicit
   `af auth sync`? Recommendation: (a) with a `--no-secret-sync` opt-out.
5. **Provider-specific probes (ADR-017 / ADR-026):** does `workspaces list` block
   on daemon responsiveness? If yes, probe can timeout just like SSH. If it's
   cached, probe is free but possibly stale. Need to test before writing the ADR.
6. **Code drift timing (Lane B1.5):** land independently as `fix(docker):` commits
   **before** Phase II.5 ADRs settle? These are bugs, not design choices — fixing
   them immediately unblocks B1.5 from the dependency chain.

---

## 7. Proposed Order of Operations

If all six questions resolve the recommended way:

1. **Now (Lane B1.5, independent):** Fix `provider/docker.rs` bugs (G4, G5, G6) as
   standalone `fix(docker):` commits. Red test for each. ~1 hour, 3 commits.
2. **Next (Phase II.5, lead):** Author ADR-022 through ADR-026 + `docs/reference/external-tools.md`. ~2 hours.
3. **Then (Phase III, parallel):** 8 subagents on disjoint lanes per the revised map.
4. **Finally (Phase IV, lead):** Integration, same as original plan.

Total addition to the sprint: ~3 hours lead work + 1 extra implementation lane (M1)
or zero if cmux is deferred.

---

## Appendix A — File-Ownership Update for Revised Lanes

```
Lane B1.5 owns:
  src/provider/docker.rs

Lane B2.5 owns:
  src/auth/sync.rs (new)
  src/auth/mod.rs  (extend)

Lane M1 owns:
  src/mux/cmux.rs (new)
  (Multiplexer trait in src/mux/mod.rs is shared; lead integrates)

Lane S1 owns:
  src/provider/slicer.rs

Do not touch (shared, unchanged from ADR-015):
  Cargo.toml, src/cli.rs, src/lib.rs, src/{provider,cmd,mux}/mod.rs,
  README.md, CHANGELOG.md, TODO.md, PROGRESS.md, docs/adr/README.md
```

---

## 8. Critic + Security + Architect Findings (synthesized 2026-04-21)

Three parallel Opus reviews were spawned against the accepted ADRs and this gap
analysis: **critic** (hard critique of plan + ADRs), **security-reviewer** (secret
handling + SSH), **architect** (composition + trait evolution). Key findings below;
each is tagged `[C]`, `[S]`, or `[A]` for provenance.

### 8.1 New Critical findings (not in §2 matrix, block Phase III)

| # | Source | Finding | Fix |
|---|---|---|---|
| N1 | [S] C1 | **ADR-016 §Consequences L91–93 ships API keys via `SetEnv`/`SendEnv` over SSH.** On multi-tenant exe.dev, the key lands in `/proc/<sshd-child>/environ` and `/proc/<agent-pid>/environ`, readable by any co-tenant process (LSPs, npm postinstall, language toolchains). `sshd` debug configs can log `SetEnv` names. | Forbid `SetEnv`/`SendEnv` for API keys. Deliver via **stdin pipe** to `af agent launch --read-env-from-stdin`, or write to `/run/user/$UID/af-<session>/.env` mode 0600 and unlink after first read. Amend ADR-016 via a new ADR ("Secret Delivery Transport") before Lane B2 opens. |
| N2 | [S] C2 | **ADR-017 L33 + L80–83 uses `StrictHostKeyChecking=no` on probe**, then `accept-new` on session. MITM attacker that hijacks the first connection (ARP/BGP) has the probe pass against a fake sshd, then the session `accept-new` pins the attacker's host key as TOFU. API keys then flow to the attacker forever. | Use `StrictHostKeyChecking=accept-new` on **both** probe and session, or pre-populate `known_hosts` from provider control-plane fingerprint. **Never use `no` on any code path that precedes key material transit.** One-line fix to the ADR snippet. |
| N3 | [C] 1.1 + [S] | **ADR-016 env-var injection is wrong for sbx sandboxes**, not just redundant. sbx's proxy model (`sbx secret --help`: "the secret is never exposed directly to the agent") means `auth::inject(env, "claude")` into an sbx sandbox produces a duplicate credential path. Same for slicer's native `slicer secret` store. | Narrow ADR-016 via new ADR (see §8.3 revised slate): injection is **host + exedev only**. Sandboxed sessions defer to provider-native stores (af merely points the user at `sbx secret set` / `slicer secret create` when `af doctor` detects the key is missing). **Matches user directive D1.** |

### 8.2 New High findings (land before 0.1.0 tag)

| # | Source | Finding | Fix |
|---|---|---|---|
| H-a | [S] H1 | `auth::inject(env: &mut HashMap<String,String>, …)` has no redaction wrapper. One `tracing::debug!("env: {:?}", env)`, one panic backtrace, one crash reporter leaks the key verbatim. | Use `secrecy::SecretString` + `zeroize` for all stored values; custom `Debug` prints `[REDACTED]`. Grep gate in CI against `{:?}` on types named `*Env*` or containing `Secret`. Panic hook that strips env from backtraces. |
| H-b | [S] H2 | **Linux Secret Service ACL undefined**: with `service="af"`, any process in the unlocked user session can enumerate all `af/*` entries via D-Bus `SearchItems` — no per-secret ACL, no unlock prompt. Malicious VS Code extension / npm postinstall can exfiltrate silently. | Use a **dedicated non-default collection** (`CreateCollection`) that auto-locks on idle. Prefer opaque labels (`af-<uuid>` via pointer file) over `af/claude`. Add `af auth doctor` detecting auto-unlocked default collection. |
| H-c | [S] H3 | No rotation / revocation protocol. `af auth clear --provider claude` mid-session does **not** terminate the running agent — its env still holds the key. Silent false reassurance. | Enumerate live sessions from ledger on `af auth clear`; warn and offer `--kill-sessions`. Document explicitly in the ADR: keyring operations affect only **new** launches. |
| H-d | [C] 2.1 | **ADR-018 `CommandRunner` trait is premature abstraction.** Requires every provider to hold `Box<dyn CommandRunner>`, threaded through constructors — ~24 call sites × heap-allocated trait object. Rust idiom for this is a generic with `#[cfg(test)]` default, or simpler: `process::Command` directly + `assert_cmd` integration tests + per-provider feature gates. The feature gate alone solves CI fragility (which is the ADR's actual Context). The trait solves a different problem (unit-test determinism) at real indirection cost. | Scope ADR-018 via new ADR ("Testing Strategy Refinement") to "feature gates + `assert_cmd`" and drop the trait. If a provider later needs branch-coverage on shell failure paths, add the trait locally to that one provider. **Deletes ~200 LOC and one coordination axis from every lane spec.** Saves Lane A1 time. |
| H-e | [C] 1.2 + [A] §4 | **`RemoteProvider` trait mixes lifecycle with SSH-target emission**, and `setup()` only applies to exedev (workspaces CLI owns its own bootstrap). Keeping `setup()` on the trait forces workspaces provider to stub it. | Per [A]: new **ADR-027 "Remote = SSH Target"** — narrow trait to `create/list/teardown/detect/ssh_target(name) -> Result<SshTarget>` + `is_alive`. Move exedev's bootstrap to `ExedevProvider::bootstrap` (concrete method, not trait). Folds G11 in — supersedes ADR-017 prose for provider identity conflation. **Replaces the proposed ADR-026** (drop ADR-026 entirely per D2). |

### 8.3 Revised ADR slate (post-review)

Supersedes §3 above. Net 4 new ADRs + 1 addendum, not 5.

| ADR | Title | Kind | Drives | Size | Change from §3 |
|-----|-------|------|--------|------|---------------|
| 022 | cmux Multiplexer Provider | Design | G7, D3 | M | **Option (1)** (first-class impl, no trait change). Capability negotiation inside `CmuxMultiplexer`. See §8.5 for critic disagreement. |
| 023 | Sandbox Agent-Layer Conflict Resolution | Design | G3 | S (~2 paragraphs) | **Option (A)** — ratify current split. Shipped and tested; no code change. Plain `docs:` commit. |
| 024 | Remote Sandbox via Daemon URL | Design | G1 | S | Unchanged from §3. Supersedes ADR-014 §37–41 for slicer. |
| 025 | **Secret Boundaries** (renamed from "Injection Strategies") | Design | G2, D1, N3 | M (narrows ADR-016) | **No sync layer.** Injection = host + exedev only. Includes N1 (stdin/tmpfs transport) + H-a (redaction) + H-b (dedicated collection) + H-c (rotation protocol). |
| 027 | Remote = SSH Target | Design | G11, D2, H-e | S | **NEW per [A].** Narrows `RemoteProvider` trait. Drops `setup()` from trait. Adds `ssh_target()` + provider-specific `is_alive`. Supersedes ADR-004 §30–44 and ADR-017 §"probe" prose. |
| — | ADR-026 | — | — | — | **DROPPED.** Folds into 027. |
| 028 | Agent-Level OS Sandbox | Design | G15, D6, D7 | S | **NEW per D6.** Adds `--agent-sandbox=<none\|os>` flag. Per-agent mapping: codex → `-s workspace-write`, claude → no-op (internal), others → no-op. Orthogonal to VM-sandbox layer. Enables safe overnight-yolo on bare host. |
| **Addenda** | | | | | |
| A-a | `docs/reference/external-tools.md` | Reference | G10 | S | Unchanged from §3 (not an ADR). |
| A-b | Amend ADR-018 to "feature gates + assert_cmd only" | Design | H-d | S | **NEW.** Drops `CommandRunner` trait per critic. Written as ADR-018-addendum or superseding ADR. |
| A-c | `book/src/concepts/approval-modes.md` | Reference | G14, D5 | S | **NEW.** Lifts ADR-012's per-agent mapping table into a user-facing book page. Owned by Lane L-BOOK. |
| A-d | Overnight-yolo guard in `src/cmd/create.rs` | Code | G16, D7 | S | **NEW.** Policy guard: warn when `--yolo` runs without VM sandbox AND without `--agent-sandbox=os`. Touches `cli.rs` + `create.rs`; lead-owned in Phase IV. |

### 8.4 Revised lane plan (architect's consolidation, 12 → 7)

The [A] agent proposes collapsing the revised lane count. Each lane is
single-sentence-scope and file-disjoint:

| Lane | Folds in | Why |
|---|---|---|
| **L-FIX** | B1.5 (docker bugs) | Three `fix(docker):` commits **before** Phase II.5 — plain bugs, no ADR dep. See §5. |
| **L-REMOTE** | A1 + A2 + B3 + B4 | "Remote = SSH target" per ADR-027. One lane wraps `workspaces create/list/ssh-config`, adds `ssh_target`, implements liveness per provider. Kills four artificial splits (B3 and B4 were already folded). |
| **L-SBX-DAEMON** | B1 | Slicer `--url` + `--token` plumbing. One TOML field + a test. |
| **L-AUTH** | B2 | ADR-016 as narrowed by ADR-025. Host + exedev only. Drops B2.5. |
| **L-EDITOR** | B5 | URL schemes + `workspaces connect` branch (G8). |
| **L-MUX-CMUX** | M1 | cmux as second `Multiplexer` impl. Per directive D3, promoted mandatory — see §8.5 conflict. |
| **L-AGENT-SANDBOX** | ADR-028 + G15 | Per-agent mapping of `--agent-sandbox=os`. Touches `src/agent/codex.rs` (set `-s workspace-write`) and `src/agent/claude.rs` (documented no-op). File-disjoint from other lanes. Small (<50 LOC + tests). |
| **L-BOOK** | C1 + A-c (approval-modes page) | Unchanged from ADR-020, with an added `concepts/approval-modes.md` page for G14. |

**Dropped:** Lane S1 (slicer API rework) — ADR-023 ratifies current split in 2
paragraphs; no code change. Lane B2.5 (secret sync) — user directive forbids.

**Kept in §4 list for now:** cross-reference with §4 shows §4 is pre-consolidation.
If user accepts the architect's merge, §4 becomes obsolete and lane naming switches
to L-* prefix.

### 8.5 Conflicts between directives and critic

| Topic | User directive | Critic recommendation | Resolution |
|---|---|---|---|
| cmux in 0.1.0 | **D3: mandatory, first-class.** Multiplexer trait accommodates both. | [C] 2.3: **defer to 0.2.0** — ADR-022 draft has 4 unresolved open decisions including `CMUX_SOCKET_PASSWORD` auth UX; forcing keyring/secret coupling into mux layer. Adds Lane M1 + trait extension risk. | **User wins** (directive is authoritative). Critic's reasoning captured here for future-session visibility. ADR-022 must close the 4 open decisions before Lane M1 opens; if any resists resolution under TDD pressure, escalate back to user before blocking on it. |
| File-ownership manifest location | ADR-015 §Decision embeds the full list. | [C] 2.4: manifest is "aspirational for humans, performative for ADRs" — bureaucracy trap (changing the list requires a new ADR). Move list to `docs/CONVENTIONS.md` (a living doc); keep ADR-015 as ~20 lines about the pattern. | **Accept critic.** `docs/CONVENTIONS.md` already exists (Lane D). Reduce ADR-015 to pattern-only in a future amendment ADR. Low priority — don't block on it. |
| `af/<provider>` keyring naming | ADR-016 §L63 | [C] 2.2: `af/` prefix is redundant when service is already `af`. | **Accept critic.** Tiny one-line amendment to ADR-016 via ADR-025. |
| `StrictHostKeyChecking=no` on probe | ADR-017 §Decision L33 | [C] §4-4 + [S] C2: use `accept-new` on both. | **Accept both.** This is a Critical (N2). Fold into ADR-027. |
| `CommandRunner` trait | ADR-018 §Decision | [C] 2.1: premature abstraction. Use feature gates + `assert_cmd` only. | **Accept critic.** See H-d. Addendum ADR before Lane L-REMOTE opens. |

### 8.6 Missing scope decisions (surfaced by critic, require user call)

1. **Windows.** No project-level statement. ADR-016 says no Windows; ADR-017 uses POSIX SSH. Add one sentence to `docs/SPEC.md` or a tiny ADR.
2. **Headless Linux / CI.** ADR-016 says Secret Service is "standard for desktop sessions; headless needs manual config." But remote exe.dev VMs *are* headless. What does `af auth` do on remote? Unaddressed.
3. **Multi-user keyring.** Two devs on one shared mac — `service="af"` collides per-OS-user. Probably fine (macOS keychains are per-user) but unstated.
4. **Golden help-snapshot brittleness.** ADR-020's `include_str!` hard-compare eats friction on every clap upgrade. Prefer `insta` snapshots (one-command approval) or `--ignored` gate.
5. **awk extraction drift.** ADR-021's CHANGELOG anchor `/^## \[${version}\]/` and stopper `/^## \[/` are asymmetric — `## [Unreleased]` or `## [TBD]` breaks extraction. Tighten stopper to `/^## \[[0-9]/`, or swap to `git-cliff` / `clog`.
6. **`xtask` vs shell script.** Sprint plan §15(2) rejected `xtask` "to minimize moving parts." Critic counters: Bash is un-testable; `xtask` gets clippy + unit tests + cross-platform. Worth reconsidering once `book-gen` grows past trivial.
7. **`keyring` v3 CVE status.** ADR-016 asserts CVE-free; verify via `cargo audit` CI output rather than ADR assertion.

### 8.7 Things confirmed fine (do not touch)

- ADR-014 three-layer model (agent × remote × sandbox) is sound. Only the slicer-specific composition prose is stale (G1).
- Gap analysis document itself — [C] called it "genuinely good." Keep the pre-sprint gap-analysis ritual.
- ADR-021 CHANGELOG-first principle — correct; only the awk *tool* is weak.
- ADR-019 URL scheme table — correctly researched, minimal. Keep.
- Lane D foundation work — "executed well, don't touch."
- Immutable ADR policy + ADR-first — working as intended.

---

## 9. Proposed Order of Operations (revised after §8)

Supersedes §7.

1. **Immediate (L-FIX, independent of any ADR):** Fix `provider/docker.rs` bugs (G4, G5, G6) as three standalone `fix(docker):` commits with regression tests. Red-first. ~1 hour.
2. **Phase II.5 (lead-only):** Author ADR-022, 023, 024, 025, 027 + ADR-018 addendum + `docs/reference/external-tools.md` + amendments to ADR-017 (accept-new fix) and ADR-016 (account naming). ~2 hours. Delete `docs/planning/adr-drafts.md` and rename sections to `docs/adr/NNN-*.md`.
3. **Scope call (user):** decide on §8.6 open items (Windows, headless, multi-user, snapshot tool, extraction tool, xtask, CVE verify). Most can resolve as "defer to 0.2.0" with a one-sentence ADR.
4. **Phase III (parallel, 7 lanes):** per §8.4. All lanes file-disjoint; lead integrates.
5. **Phase IV + V:** unchanged from original plan.

Total lead work: ~3.5 hours (up from §7's ~3 hours — the security findings demand a Secret-Transport sub-decision).

---

## Appendix B — Version Stamps (reproducibility)

Probed on 2026-04-21 on the developer's macOS machine:

- `slicer` — installed; `/opt/homebrew/bin/slicer`; supports `--url` daemon mode
- `sbx` — installed; `/opt/homebrew/bin/sbx`; 9 agents including `shell`
- `workspaces` — installed; `/opt/homebrew/bin/workspaces`; DD-internal
- `cmux` — installed; `/Applications/cmux.app/Contents/Resources/bin/cmux`

Verified exact subcommand surfaces quoted in §1 are from these versions.
