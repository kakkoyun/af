---
adr: 068
title: "Operational UX Contract"
status: proposed
implementation: complete
date: 2026-05-21
last_modified: 2026-05-22
supersedes: []
superseded_by: null
related: ["034", "035", "037", "049", "050", "054", "055"]
tags: ["go", "ux", "json", "exit-codes", "tty", "concurrency", "completion"]
---

# ADR-068: Operational UX Contract

## Context

`af` v1's command surface (ADRs 035, 044–059, 063–067) has grown
piecewise. Several cross-cutting UX concerns were touched in passing
by individual command ADRs but never centralised:

1. **JSON output.** ADR-054 (`af status --json`) and ADR-055
   (`af info --json`) each specify their payload shape, but there is
   no rule that says every `--json` everywhere follows the same
   envelope, carries a version, or behaves consistently on error.
2. **Exit codes.** ADRs scatter phrases like "exits non-zero on
   conflict" and "returns an error" without pinning down a canonical
   table. Scripting users have no way to distinguish "session not
   found" from "git rebase failed" from "external tool missing".
3. **stdout / stderr / TTY / color.** ADR-034 mandates `slog` to
   stderr but never resolves which stream carries `gh` prompts, `fzf`
   pickers, or progress text. Color handling is implicit.
4. **Concurrency across `af` invocations.** ADR-037 has atomic
   `state.toml` writes via per-file flock, but two `af` invocations
   touching the same session are otherwise unspecified.
5. **Tab completion.** ADR-035 mentions `RegisterFlagCompletionFunc`
   for `--from` but the rest of the completion surface is implicit.

These are individually small but compound into a discoverable,
script-friendly CLI when treated as one contract. This ADR collects
them.

## Decision

`af` v1 commits to the operational contracts below. They are stable
across the v1 series; changes require new ADRs that supersede
specific sections of this one.

### 1. JSON output

Every command that accepts `--json` emits a single JSON document on
**stdout** and nothing else. The document has this top-level shape:

```json
{
  "schema": 1,
  "data": { ...command-specific... }
}
```

- `schema` is a positive integer. Each command owns its schema
  number; numbers are independent across commands. The schema bumps
  **only** on breaking changes (field removal, type change,
  semantic change). Additions (new fields, new event types) are
  non-breaking and do not bump the schema.
- `data` is the command's payload. Its shape is documented in that
  command's ADR.
- Field order inside `data` is stable across releases. Go
  `encoding/json` plus explicit struct definitions in
  `internal/jsonio/` guarantee this.

On error during a `--json` invocation, stdout stays empty and a
JSON error document is written to **stderr**:

```json
{
  "schema": 1,
  "error": {
    "code": "EX_NOINPUT",
    "message": "session 'foo' not found",
    "hint": "run 'af list' to see active workstreams"
  }
}
```

`code` is the symbolic name of the exit code (see §2). `hint` is
optional. The command exits with the corresponding non-zero exit
code.

Per-command `--json` schemas (and any breaking-change history) are
recorded in their owning ADRs (ADR-054 for status, ADR-055 for info,
…). New commands that ship `--json` reference §1 of this ADR and
declare their schema number.

### 2. Exit codes

`af` follows BSD sysexits conventions (`<sysexits.h>`) where they
apply, plus the three universal codes (`0`, `1`, `2`, `130`).

| Code  | Symbol            | Meaning                                              |
| ----- | ----------------- | ---------------------------------------------------- |
| `0`   | `EX_OK`           | Success.                                             |
| `1`   | `EX_GENERAL`      | Generic unclassified error.                          |
| `2`   | `EX_USAGE_COBRA`  | Cobra-surfaced usage error (unknown flag/command).   |
| `64`  | `EX_USAGE`        | Argument or flag validation failure.                 |
| `65`  | `EX_DATAERR`      | Bad `state.toml` / `config.toml` / `ledger.jsonl`.   |
| `66`  | `EX_NOINPUT`      | Session, branch, or file not found.                  |
| `69`  | `EX_UNAVAILABLE`  | Required external tool missing (`gh`, `slicer`, …). |
| `70`  | `EX_SOFTWARE`     | Internal invariant violated (a bug).                 |
| `75`  | `EX_TEMPFAIL`     | Retryable failure (network, lock timeout).           |
| `77`  | `EX_NOPERM`       | Permission denied (keyring, filesystem, SSH).        |
| `130` | `EX_INTERRUPTED`  | `SIGINT` received during the command.                |

Each command's per-failure mapping is documented in its owning ADR.
For example:

- `af pr --ai` empty-diff → `EX_DATAERR` (65).
- `af resume <unknown>` → `EX_NOINPUT` (66).
- `af doctor` finds missing tools → `EX_OK` (0); doctor is a
  reporter, not a gate.
- `af sync` conflict → `EX_GENERAL` (1), with conflict files left in
  place.

New error classes are added by amending this table in a new ADR; no
ad-hoc numbers in the 64–78 range.

### 3. stdout / stderr / TTY / color

| Stream | Carries                                                                |
| ------ | ---------------------------------------------------------------------- |
| stdout | Command **data**: tabular text, JSON, or empty.                        |
| stderr | All diagnostics (`slog`), progress messages, interactive prompts,      |
|        | `fzf` pickers (ADR-070), confirmation prompts, error messages.         |

This makes every command pipeable:

```bash
af list --json > /tmp/x.json     # JSON to file, prompts to terminal
af status --json | jq '.data'    # JSON parsed without stderr noise
```

Color is enabled when **all** the following hold:

1. The target stream is a TTY (checked per-stream — stdout for
   tabular output, stderr for diagnostics).
2. `NO_COLOR` is unset.
3. `TERM` is set and not equal to `dumb`.

`FORCE_COLOR` (any non-empty value) overrides #1 and #3. `NO_COLOR`
always wins (`NO_COLOR` is the dominant signal per the
<https://no-color.org/> convention).

`af` does **not** ship a global `--quiet` / `-q` flag in v1.
Per-command `--verbose` flags (e.g. `af doctor --verbose`) remain as
their owning ADRs specify them. If the implementor finds a concrete
need for global suppression, a future ADR adds it.

### 4. Concurrency across `af` invocations

Each session directory carries a file lock:

```
~/.local/share/af/v1/sessions/<name>/.af.lock
```

- **Mutating operations** acquire it (`flock` exclusive,
  blocking with a 30-second timeout). Examples: `af create`,
  `af suspend`, `af resume`, `af done`, `af note --append`,
  `af agent add`, `af agent stop`, `af sync`, `af pr`, `af stack`,
  `af session-data sync` (ADR-067), refresh writes from
  ADR-071.
- **Read-only operations** do not acquire it. `af list`, `af status`
  (read path), `af info` read whatever the most recent flushed write
  provided. Atomic `state.toml` writes (ADR-037) guarantee they see
  a consistent file, even mid-write.
- Lock-timeout failures return `EX_TEMPFAIL` (75) with a hint:
  `another af invocation is holding the lock on <session>; retry in
  a moment.`

The lock file itself is **outside** the archive: when `af done`
archives a session, the lock is released before the move, then
deleted along with the original session directory.

**No cross-session lock.** Two `af` invocations on different
sessions are independent. The ADR-049 60-minute secrets sweep is the
only cross-session sweeper.

**No daemon, no IPC.** Every `af` invocation is a fresh process that
runs to completion.

### 5. Tab completion

Cobra completion is generated for `bash`, `zsh`, `fish`, and
`powershell` via `af completions <shell>` (per ADR-045). The
following completion sources are mandatory:

| Completion source | Surfaces                                                                       |
| ----------------- | ------------------------------------------------------------------------------ |
| Workstream names  | `<session>` positional arg and `--session NAME` on every command that accepts it (active + suspended, current repo first). |
| Branch names      | `--from BRANCH`, `--base REF` (local + remote refs).                           |
| Agent providers   | `--agent NAME`, `agent add --agent NAME` → `pi`, `claude`, `codex`.            |
| Sandbox providers | `--sandbox PROVIDER` → `slicer` only (post-ADR-060).                           |
| SSH hosts         | `--remote HOST` → aliases from `~/.ssh/config`.                                |
| Slot names        | `<slot>` arg on `af agent stop`, scoped to the inferred session.               |
| Shell names       | `--shell SHELL` on `af setup` and `af completions` → `bash zsh fish powershell`. |
| Lifecycle states  | `--filter STATE` on `af status` → `active suspended completed abandoned`.      |

Completion functions:

- **Never** invoke agent CLIs, never invoke `gh`, never invoke `ssh`
  except to parse `~/.ssh/config` as a static file. Network calls
  inside completion would block the shell.
- Are tested via `cmd/af/testdata/script/completion-*.txt`
  scenarios (per ADR-051).

## Consequences

- Scripts can rely on a single JSON envelope across the binary; new
  `--json`-bearing commands plug in trivially.
- Exit codes form a documented vocabulary; users can write
  `if af pr --ai; then ...; elif test $? -eq 65; then ...`-style
  control flow.
- Stdout/stderr discipline keeps pipelines clean: `af status --json
  | jq` always works.
- Per-session locking eliminates the "two terminals touched the same
  session" class of corruption without introducing a daemon.
- Tab completion is consistent across commands; muscle memory works.
- Five small contracts fit in one ADR rather than five. The
  trade-off is that an amendment to (say) the exit-code table is a
  full new ADR, not a `nit` edit.

## Alternatives Considered

- **One ADR per contract** (068 JSON, 069 exits, 070 TTY, 071
  concurrency, 072 completion). Rejected: the contracts compose, and
  five ¼-page ADRs are harder to maintain than one 2-page ADR.
- **Adopt full BSD sysexits.** Rejected: many sysexits codes
  (`EX_CONFIG`, `EX_OSFILE`, `EX_PROTOCOL`) overlap with the ones we
  picked, and adding them later costs nothing.
- **Global `--quiet`.** Rejected for v1: each command already has
  the right amount of output by default; adding a knob now invites
  feature creep.
- **Pid-file lock at `~/.local/share/af/v1/af.lock` (global lock).**
  Rejected: serialises unrelated sessions for no benefit.
- **Color via `aurora` / `lipgloss` third-party libs.** Out of
  scope; v1 keeps to stdlib + `golang.org/x/term`.

## References

- ADR-034 — Go idiom; reinforces stderr-for-diagnostics.
- ADR-035 — cobra; provides the completion mechanism.
- ADR-037 — state.toml atomic writes (per-file flock).
- ADR-049 — secrets sweep (the one cross-session sweeper).
- ADR-050 — golangci-lint pedantic; `gosec` flags `flock` misuse.
- ADR-054 / ADR-055 — first commands to implement §1 (JSON envelope).
- ADR-070 — session inference; fzf picker writes to stderr per §3.
- ADR-071 — PR-state refresh writes through §4 lock.
- ADR-072 — state.toml schema additions; no concurrency impact.
- BSD sysexits: <https://man.openbsd.org/sysexits.3>
- `NO_COLOR` convention: <https://no-color.org/>
