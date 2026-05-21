# af v1 — Gap Analysis (working document)

> Scratchpad for the SPEC-vs-ADR reconciliation pass. **Not** a long-lived
> doc; once the missing ADRs land and `SPEC.md` is rewritten, this file
> is deleted (in the same commit as the new SPEC, so the diff still
> tells the story).

---

## 1. SPEC drift since ADR-059

`docs/SPEC.md` was last meaningfully reshaped around ADR-059. Eight
ADRs landed after it (060–067) and none of them edited `SPEC.md`. Each
of these is a concrete gap.

| ADR | Topic | What SPEC currently says | What ADR now says | Where SPEC is wrong |
| --- | --- | --- | --- | --- |
| 060 | Slicer-only sandbox | §10 lists two providers (`slicer`, `sbx`); §5.2 `sandbox_provider` enum still includes `"sbx"`; §6.2 `[sandbox]` mentions `sbx.*` | `slicer` is the only runtime provider; `"sbx"` in old state must fail-closed with a migration hint | §10, §5.2, §6.2, §15 |
| 061 | Repo-scoped `[control]` | §6.1 says repo config has no `[obsidian.vaults]` — nothing else | Repo config grows a `[control]` section governing default remote-control provider and other per-repo launch defaults | §6.1, §6.2 |
| 062 | `[sandbox.slicer.resources]` | §6.2 has no profile schema | `[sandbox.slicer.resources]` (vcpu/ram/storage/gpu/image/hypervisor); managed group name `af-<repo-slug>-<profile>`; state.toml captures effective profile | §6.2, §5.2 |
| 063 | `af control up/down/status` via Tailscale + superterm | §3 has no command group; §15 doesn't mention Tailscale | New top-level command group `af control`; bound to host, not workstream | §3 (new subsection), §6.2, §15 |
| 064 | Opinionated diff rendering | §3.8 says `af diff` is a pure proxy reading `[diff].cmd` | Default dispatch is `hunk` → `git` (terminal) or `diffity` (`--web`); `[diff].cmd` is now an explicit escape hatch | §3.8, §6.2 |
| 065 | `slicer wt` worktree transport | Nothing | Host worktree is leased to VM via `slicer wt push/pull`; no host mount | §10 (rewritten), §15 |
| 066 | VM agent session export | Nothing | New `af session-data pull/list` command group | §3 (new subsection), §2 lifecycle table |
| 067 | Automatic session sync + state | Nothing | `pull` renamed to `sync`; sync state added to `state.toml`; auto-runs on `af suspend`/`af done` for slicer VMs | §3, §2, §5.2 state schema |

Plus stale boilerplate:

- §16 References says "ADRs 031–059". Should be 031–067 (+ whatever 068+
  we add in this pass).
- §1 Overview talks about "tmux session per workstream, one pane per
  agent" but does not mention that **host-level** `af control` exposes
  the whole tmux server (not per-workstream).
- §3.10 Meta is the last subsection — needs at least `af control` and
  `af session-data` slotted in before it.

## 2. Things the ADRs decided but the SPEC never absorbed

Even before ADR-060, some details were never lifted from per-command
ADRs into the SPEC, so the SPEC reads as more vague than the contract
actually is.

- **`af status --json` schema** (ADR-054). SPEC §3.3 just says
  "`--json` emits JSON". The actual contract is "machine-stable, sorted
  by session name, fixed key order, schema-versioned." Belongs in §3 or
  a new §17 (Output contracts).
- **`af info --json` schema** (ADR-055) — same.
- **Three-strategy merge detection** (ADR-056) is the reusable engine
  behind both `af clean` and `af sync` (ADR-059 §sync). SPEC mentions
  it for clean but never names the shared engine.
- **`agent.BodyCmd` interface** (ADR-057). SPEC §7 lists the method but
  doesn't say that ADR-057's prompt template is hard-coded — users may
  expect it to be configurable.
- **Stack chain rendering** (ADR-055 §--json) — `stack.chain` walk-order
  rule never makes it to SPEC.

## 3. Schema gaps (state.toml + config.toml)

`state.toml` schema in ADR-037 is the canonical, and SPEC §5.2 mirrors
it. Several later ADRs amend the schema without re-emitting it:

| ADR | Field amendment to state.toml | Status |
| --- | --- | --- |
| 059 | `[stack]` block (`parent_session`, `parent_branch`, `linked_at`) | ✅ in SPEC |
| 061 | `[control]` block — what gets captured from repo config? | Hinted in ADR-061 §State capture, not formalised |
| 062 | Effective resource profile (`profile_name`, `vcpu`, `ram_gb`, `group`, …) | ADR-062 §Resolution step 8 says "record" but no field name |
| 065 | Worktree lease state (lessee VM, last push, last pull) | Implicit |
| 067 | `[session_sync]` block (per-agent cursors, hashes, last sync ts) | ADR-067 §State schema — explicit, but never folded into SPEC §5.2 |

`config.toml` schema in ADR-036 has the same problem:

| ADR | Schema addition to config.toml | Status |
| --- | --- | --- |
| 057 | `[pr].ai_model` | In SPEC §6.2 already |
| 061 | `[control]` (repo-only) | Not in SPEC §6.2 |
| 062 | `[sandbox.slicer.resources]`, `[sandbox.slicer].group` | Not in SPEC §6.2 |
| 064 | Whether `[diff]` is the legacy escape hatch and what the new opinionated knobs are (e.g. `[diff].mode = "opinionated" \| "custom"`) | Not decided in ADR-064 itself; SPEC §6.2 stale |

## 4. Cross-cutting concerns with no ADR

These are areas where multiple ADRs touch the same surface but no ADR
centralises the contract. Many are small. Some are worth a real ADR;
others can become a single "operational contracts" ADR.

### A. JSON output contract
`af status`, `af info` (and arguably `af list`, `af agent list`, `af retro`)
all want `--json`. ADR-054 says "stable, schema-versioned". ADR-055
says "field order stable, adding fields non-breaking". There's no
top-level rule. Should every `--json` carry `"schema": <int>`? When
does the schema bump?
**Need ADR.**

### B. Exit-code contract
Scattered across ADRs ("returns non-zero", "exits 1 on conflict") with
no canonical table. `af pr --ai` distinguishes empty-diff from
empty-body errors but doesn't pin them to codes. `af clean --dry-run`
vs failures — same.
**Need ADR (small).**

### C. `AF_*` environment variable contract
Used throughout the test harness (`AF_TEST_FAKEBIN`, `AF_TEST_MUX=fake`,
`AF_LOG_LEVEL`) and likely in production (`AF_CONFIG`, etc.) but
never centrally listed. ADR-034 hints at `AF_LOG_LEVEL`; ADR-051 hints
at `AF_TEST_*`.
**Need ADR (single registry).**

### D. TTY / stdout-stderr contract
ADR-034 says "stderr for diagnostics, stdout for data". Some commands
violate that without realising (e.g. fzf prompts on stdout). No ADR
formalises:
- When is stdout machine-readable vs human-readable?
- When does color get suppressed (`NO_COLOR`, non-TTY)?
- What does `--quiet` mean (does it exist)?
**Could be folded into a single "Operational UX" ADR.**

### E. Interactive selection when `[session]` is omitted
Many commands accept `[session]` optionally. The convention today is
"infer from cwd via `.af/state.toml` symlink". But when neither cwd
inference nor an arg is available, what should `af` do?
- Error?
- `fzf`-style picker (ADR-044 probes fzf, but never specifies its use)?
- Just print "no session active"?
**Need ADR — small.**

### F. Telemetry / privacy promise
ADR-031 implies "no network calls". Worth making explicit in one
sentence:
- `af` makes zero outbound network calls beyond user-invoked
  `git`/`gh`/`ssh`/`slicer`/`sbx`/agent processes.
- No metrics, no crash reports, no version-check pings.
**Need ADR — single paragraph.**

### G. Concurrency across `af` invocations
ADR-037 has per-file flock for state.toml writes. But:
- Two `af` invocations on the same session?
- `af list` running while `af create` is writing?
- `af sync` (rebase) running while `af pr` is pushing?
**Need ADR — small contract.**

### H. PR / branch state machine refresh cadence
`state.toml.[pr].state` is `""` until something flips it to
`open|merged|closed`. ADR-054 reads `gh pr view` to render the column
but does it write back? ADR-056 needs `merged` to reap. There's no
ADR for "who updates `pr.state`, when".
**Need ADR — small.**

### I. Multi-machine state model
ADR-063 enables remote attach via Tailscale, but `state.toml` lives on
the host. Two machines holding workstreams under the same `repo_slug`
will collide in Obsidian frontmatter (`af_session` is the bare name).
- Is `af` strictly single-machine for canonical state?
- Or is multi-machine supported under the Tailscale model with
  read-only state for the attaching machine?
**Need ADR — clarity even if the answer is "single-machine, period".**

### J. Tab completion surface
ADR-035 mentions `completeBranches` for `--from`. What else completes?
- Session names: `af resume <TAB>` → list active sessions?
- Slot names: `af agent stop <TAB>` → list slots in the inferred session?
- Agent providers: `--agent <TAB>` → `pi claude codex`?
- Sandbox providers: `--sandbox <TAB>` → `slicer` only post-ADR-060.
- Hosts: `--remote <TAB>` → `~/.ssh/config` aliases?
**Need ADR — small completion contract.**

### K. Workstream-name uniqueness across repos
Two repos may both produce `kakkoyun--issue-42`. Sessions are filed
flat under `~/.local/share/af/v1/sessions/<name>/`. Collision policy
isn't stated.
**Need ADR — small (likely "names are unique across all repos; create
fails if it collides").**

## 5. Things that look like ADRs but really belong in the SPEC

A handful of ADRs (060, 064, 067 in particular) make decisions that
should have included a `SPEC.md` update in the same commit. The
working agreement (`AGENTS.md`, §"Doc update rule") already requires
this; we just missed it. The fix is the SPEC rewrite, not a new ADR.

## 6. What we do NOT need an ADR for

- A combined config schema reference document. The ADRs are the
  authority. The SPEC is the reader-friendly summary.
- A "v1 release plan". Already covered by ADR-053 plus PROGRESS.md.
- A new "command catalogue" doc. ADR-035 is already that.

## 7. Proposed structure for the new ADRs

A consolidated set of ten short ADRs (068–077), each ~1–2 pages,
ordered so they form a coherent batch:

| # | Title | Bytes | Touches SPEC § |
| - | --- | --- | --- |
| 068 | JSON output contract | ~1 page | new §17 |
| 069 | Exit-code contract | ~1 page | new §17 |
| 070 | `AF_*` environment variable registry | ~1 page | new §18 |
| 071 | TTY / stdout-stderr / `NO_COLOR` contract | ~1 page | new §17 |
| 072 | Interactive selection policy when `[session]` is omitted | ~1 page | §3 prelude |
| 073 | Network/telemetry/privacy promise | ½ page | new §15.1 |
| 074 | Concurrency model for concurrent `af` invocations | 1 page | §5 prelude |
| 075 | PR/branch state refresh cadence and write contract | 1 page | §5.2 |
| 076 | Multi-machine state model — single-machine canonical | 1 page | §1, §11 |
| 077 | Tab completion surface | ½ page | §3 footer |
| 078 | Workstream-name uniqueness and collision policy | ½ page | §4.1 |
| 079 | State schema amendments roll-up (061, 062, 065, 067 in one canonical place; supersedes the schema sections of ADR-037 by amendment) | 1 page | §5.2 (rewritten) |

The number can shrink — many of these are one-paragraph decisions; we
can fold tightly related ones (A+B+D+G into one "Operational UX
contract" ADR; F+I+K into one "Boundary contracts" ADR) so we land
~6 ADRs instead of ~12. Up to you.

## 8. Proposed SPEC rewrite outline

Rough table of contents for the rewritten `docs/SPEC.md`:

```
1.  Overview (workstream concept, single-user, single-machine canonical)
2.  Workstream lifecycle (states + slicer-VM session-sync side effects)
3.  Command surface
    3.1  Creation, teardown, listing
    3.2  Multi-agent management
    3.3  Inspection (list, status, info)
    3.4  Reaping (clean)
    3.5  Stacking (stack/unstack/sync)
    3.6  Environment & utilities (setup, doctor, config, completions)
    3.7  Notes & retro
    3.8  Proxy commands (editor)
    3.9  Diff rendering (af diff — hunk/diffity/git, --web)
    3.10 PR creation (af pr / --ai)
    3.11 Secrets (auth)
    3.12 Remote control (af control up/down/status)
    3.13 Slicer worktree transport (af-side wrappers around slicer wt)
    3.14 VM session-data sync (af session-data sync/list)
    3.15 Meta (version, --help, --version)
4.  Workstream identifiers (names, session IDs, worktree paths, uniqueness)
5.  State files
    5.1 Layout
    5.2 state.toml schema (now including control, resources, session_sync)
    5.3 ledger.jsonl events
    5.4 Atomicity & concurrency model
6.  Configuration (sections, layered merge, repo-only sections)
7.  Agent providers
8.  Multiplexer
9.  Remote (SSH)
10. Sandbox (slicer only; resources; slicer wt; session-data sync)
11. Secrets
12. Obsidian integration
13. Doctor + setup
14. Build & distribution
15. Operational contracts
    15.1 Network / telemetry promise
    15.2 stdout/stderr/TTY/color
    15.3 Exit codes
    15.4 JSON output schema
    15.5 AF_* environment variables
    15.6 Tab completion
16. Out of scope for v1
17. References
```

This is one possible structure. Feedback welcome on grouping/granularity.
