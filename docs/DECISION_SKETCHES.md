# af v1 — Decision Sketches for ADR Batch 068–072

> One-paragraph proposed decision for each of the five new ADRs.
> **Redline this file.** Once you accept the sketches, I expand each
> into a full ADR following the ADR-032 conventions, then rewrite
> `SPEC.md` to absorb everything.

The file is throwaway (deleted with `GAP_ANALYSIS.md` in the final
commit).

---

## ADR-068 — Operational UX Contract

**Bundles:** JSON output, exit codes, stdout/stderr/TTY/color, concurrency,
tab completion.

Five sub-decisions, all small, all interrelated enough to live together.

### 1. JSON output

- Every command that supports `--json` emits a single JSON document
  on **stdout**, nothing else on stdout.
- All `--json` payloads carry two top-level fields: `"schema":
  <integer>` and `"data": <command-specific>`. The `schema` is a
  positive integer that increments only on a **breaking** change
  (field removal or type change). Additive changes are non-breaking.
- Field order in the payload is stable across releases (Go
  `encoding/json` plus struct definitions).
- Errors during `--json` invocations write a JSON error doc on stderr
  (`{"schema":1,"error":{"code":"...","message":"..."}}`) and exit
  non-zero. Stdout stays empty.
- The schema number lives in `internal/jsonio/` constants; each
  command's `--json` schema bumps independently. The schema is
  documented in the SPEC's new §15 "Operational contracts".

### 2. Exit codes

Standard sysexits-style table, copied to the SPEC:

| Code | Meaning                                  |
| ---- | ---------------------------------------- |
| 0    | Success                                  |
| 1    | Generic error (unclassified)             |
| 2    | Usage error (cobra surfaces this)        |
| 64   | `EX_USAGE` — argument validation         |
| 65   | `EX_DATAERR` — bad state.toml/config     |
| 66   | `EX_NOINPUT` — no such session/file      |
| 69   | `EX_UNAVAILABLE` — external tool missing |
| 70   | `EX_SOFTWARE` — internal invariant fail  |
| 75   | `EX_TEMPFAIL` — retryable (network)      |
| 77   | `EX_NOPERM` — permission denied          |
| 130  | SIGINT received during command           |

Commands with named failure modes (e.g. `af pr --ai` empty-diff vs
empty-body) pick the closest fit; the SPEC table notes the exact
mapping. New error classes need an ADR amendment.

### 3. stdout / stderr / TTY / color

- **Stdout** carries the command's data: tabular text (TTY), JSON
  (with `--json`), or empty.
- **Stderr** carries every diagnostic (`slog`), progress message,
  interactive prompt, fzf picker, error text, and confirmation prompt.
- Color is enabled when stdout is a TTY, `TERM != dumb`, and
  `NO_COLOR` is unset. `FORCE_COLOR` forces on.
- `--quiet` / `-q` is **not** a global flag in v1; per-command
  exceptions (`af doctor --verbose` etc.) stay as they are. (Defer
  unless the implementor says it's needed.)

### 4. Concurrency model

- Each session directory carries a file lock at
  `~/.local/share/af/v1/sessions/<name>/.af.lock`. Operations that
  mutate `state.toml` or `ledger.jsonl` acquire it (flock, exclusive,
  blocking with timeout); read-only operations (`af list`, `af status`,
  `af info`) do not.
- Read-only ops accept tearing — they read whatever the writer has
  flushed.
- Cross-session locking is not introduced (`af list` can race `af
  create` cheaply because each writes its own session dir).
- The 60-minute secrets sweep (ADR-049) is the only cross-session
  sweeper; nothing else mutates other sessions.

### 5. Tab completion

- Cobra completion is **always** registered for these surfaces:
  - `<session>` arg / `--session NAME` → active + suspended workstream
    names (in the cwd-inferred repo first, then alphabetical).
  - `--from BRANCH` → local + remote branches (already in ADR-035).
  - `--agent NAME` / `agent add --agent` → `pi`, `claude`, `codex`.
  - `--sandbox PROVIDER` → `slicer` only (post-ADR-060).
  - `--remote HOST` → `~/.ssh/config` host aliases.
  - `<slot>` arg on `af agent stop` → slot names from the inferred
    session.
  - `--shell SHELL` on `af setup` and `af completions` → `bash zsh
    fish powershell`.
- Completions never run agent CLIs; they read files only.

**Touches SPEC:** new §15 (Operational contracts), §3 prelude (exit codes / fzf).

---

## ADR-069 — Boundary & Privacy

**Bundles:** Telemetry promise, multi-machine model, workstream-name
collision policy.

### 1. Telemetry / privacy promise

`af` makes **zero** outbound network calls in its own code path. The
only network traffic is from sub-processes the user explicitly
invoked: `git`, `gh`, `ssh`, `slicer`, `sbx`, `tailscale`, agent CLIs.
No version check, no crash report, no usage metric, no update ping.

This is stated as a hard contract; introducing any outbound call
requires an ADR amendment that explicitly cites it.

### 2. Multi-machine state model

**Decision:** Canonical workstream state (`state.toml`, `ledger.jsonl`,
secrets, Obsidian frontmatter) lives on **one machine per session — the
host**.

`af control up --remote HOST` exposes the host's tmux server through
Tailscale Serve + superterm for **read-attached browsing only**. The
attaching machine never writes to the host's `state.toml`.

Concretely:
- `af create`, `af suspend`, `af resume`, `af done`, `af note`, `af pr`,
  etc. **always** run on the host that owns the session directory.
- Attaching from a second machine means opening a browser (superterm
  via Tailscale) — agents are interacted with through that pane, not
  through a second `af` install.
- If the user wants to drive multiple physical machines, each holds its
  own canonical `~/.local/share/af/v1/sessions/`. Names are not
  synchronised across machines.

This rules out e.g. running `af list` on a laptop and seeing sessions
on a desktop. The escape hatch is `ssh desktop af list`.

### 3. Workstream-name collisions across repos

**Decision:** Workstream names are globally unique within a single
host's `~/.local/share/af/v1/sessions/`. Two repos cannot both host
`kakkoyun--issue-42`.

`af create <name>` fails closed with:

```
workstream "kakkoyun--issue-42" already exists for repo <repo-slug>.
Use a different name or 'af done <name>' first.
```

If the same name truly is wanted (and the existing one is from a
different repo), the user picks a new name or supplies `--name
<other>`. No automatic suffixing — explicit > implicit.

The branch the workstream lives on stays per-repo (no collision
there); the constraint is on the directory name only.

**Touches SPEC:** §1 (single-machine clarification), §4.1 (collision
rule), new §15.0 (privacy promise).

---

## ADR-070 — Session Selection & Inference

**Bundles:** `[session]` arg resolution, `--session NAME` flag,
interactive picker behaviour, the cwd discovery symlink.

### Resolution order (every command that accepts `[session]`)

1. Explicit positional arg `[session]` — used verbatim.
2. Explicit `--session NAME` flag — overrides arg if both are
   somehow passed (`--session` wins, with a warning).
3. `AF_SESSION` env var — used if both 1 and 2 are empty. Mainly
   for tmux pane-level `setenv`.
4. cwd inference: walk up to find `.af/state.toml`; if present, use
   the session it points to. ADR-038's existing behaviour.
5. If none of 1–4 resolved a session **and** stdin is a TTY **and**
   `fzf` is on `PATH` **and** at least one workstream exists, run a
   fzf picker on stderr. Selection becomes the session. The user can
   abort with Ctrl-C → exit 130.
6. Otherwise, error:
   ```
   no session specified and none could be inferred from the current
   directory. Pass [session], set --session NAME, or run inside a
   workstream worktree.
   ```

Read-only commands (`af list`, `af status`) skip the inference
entirely; they always show all sessions. `af status --filter` is the
narrow knob.

`af session-branch` is unchanged — it never takes `[session]`.

**Touches SPEC:** new §3 prelude; §4.4 (new) for inference order.

---

## ADR-071 — PR State Lifecycle

**Bundles:** When does `state.toml.[pr].state` change? Who writes it?

### Decision

`state.toml.[pr].state` is a **cache** of the upstream PR state, never
the source of truth. Writes happen at three moments:

1. **`af pr` (on success)** writes `number`, `url`, and `state =
   "open"` (or `"draft"` for `--draft`).
2. **`af status` / `af info` / `af clean`**, on any invocation,
   refresh the cache for every workstream whose `pr.number != 0` by
   running `gh pr view --json state` (with `[status].max_parallel`
   cap, 5s timeout per fetch). Successful refreshes write back to
   the session's `state.toml`. Failures leave the existing value and
   render the column as `?`.
3. **`af done`** before tearing down, runs the same refresh once,
   then makes the lifecycle decision (`completed` if `merged`,
   `abandoned` if forced on `open|closed`).

There is **no daemon**. There is no on-create polling. Refresh only
happens during user-initiated reads of the dashboard / detail / reap
commands.

The ledger records `pr_state_changed` events on every cache flip
(open → merged, merged → closed, etc.) so `af retro` can see the
timing. Direct merge detection (ADR-056) is independent of this
cache; the cache is for UX, the three-strategy merge detection is for
correctness in `af clean` and `af sync`.

**Touches SPEC:** §5.2 (PR block notes), §3.3/3.4 (cache behaviour
called out).

---

## ADR-072 — state.toml Schema Amendments Roll-up

**Bundles:** Collects schema additions from ADRs 061, 062, 065, 067
into one place and amends ADR-037 by reference. Does NOT supersede
ADR-037; just consolidates the schema as of today.

### New blocks added since ADR-037

```toml
[control]                            # ADR-061
provider           = "superterm"     # captured from repo [control]; "" if disabled
port               = 0               # set by `af control up`; 0 when down
bound_at           = ""              # ISO timestamp when last bound

[sandbox.slicer.resources]           # ADR-062 §Resolution step 8
profile_name       = ""              # "" = default-group; else managed-group profile name
group              = ""              # resolved Slicer group ("af-<slug>-<profile>" or explicit)
vcpu               = 0
ram_gb             = 0
storage_size       = ""
gpu_count          = 0
image              = ""
hypervisor         = ""

[sandbox.slicer.lease]               # ADR-065
holder_vm          = ""              # "" if not currently leased to a VM
last_push_at       = ""              # ISO; populated by `slicer wt push`
last_pull_at       = ""              # ISO; populated by `slicer wt pull`

[[session_sync]]                     # ADR-067 — one entry per harvested agent/harness
agent              = "claude"        # claude | codex | pi | harness
source_root        = "/home/agent/.claude/sessions"
last_synced_at     = ""
last_hash          = ""              # sha256 of last imported tail; for prefix-append guard
last_offset        = 0               # byte offset for resumable JSONL appends
```

All four blocks are **omitted entirely** when unused. ADR-037's
"schema_version = 1" stays at 1 because additions are non-breaking.

ADR-037 gets an inline note linking forward to this ADR for the
schema delta.

**Touches SPEC:** §5.2 fully rewritten with the new blocks. Existing
fields stay byte-for-byte the same.

---

## Summary table

| ADR | Title                                       | Pages |
| --- | ------------------------------------------- | ----- |
| 068 | Operational UX contract                     | 2     |
| 069 | Boundary & privacy                          | 1     |
| 070 | Session selection & inference               | 1     |
| 071 | PR state lifecycle                          | 1     |
| 072 | state.toml schema amendments roll-up        | 1     |

After they're written, the SPEC gets a single comprehensive rewrite
commit that absorbs:
- ADRs 060–067 (eight previously-unabsorbed)
- ADRs 068–072 (this batch)
- A new §15 "Operational contracts" section
- Deletion of `GAP_ANALYSIS.md` and `DECISION_SKETCHES.md`

The implementor session is then pinged with a summary of the new
contracts; their work continues against whatever's in `main` until we
merge.
