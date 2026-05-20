---
adr: 049
title: "Secret Management (keyring + ephemeral envelope)"
status: proposed
implementation: in-progress
date: 2026-05-06
last_modified: 2026-05-20
supersedes: []
superseded_by: null
related: ["031", "036", "041", "042", "045"]
tags: ["go", "secrets", "keyring", "security"]
---

# ADR-049: Secret Management (keyring + ephemeral envelope)

## Context

Sandboxed and remote workstreams need API tokens (Anthropic key,
OpenAI key, GitHub token, etc.) injected into the agent's environment
without leaving them on disk in plaintext or leaking via env vars
visible to every process on the remote machine.

v0's secret management grew across multiple ADRs (016, 025) with a
multi-tier lookup chain, custom redaction, and Linux dbus tweaks. v1
collapses this to: **keyring for storage + an ephemeral envelope file
for transport, no SSH `SetEnv`/`SendEnv`**.

"Ephemeral" here means: written, sourced once, then deleted before
the agent's first shell prompt. The file lives on whichever filesystem
is available — tmpfs on Linux when `/run/user/$UID/` exists; otherwise
the user-data dir on disk (see §"Storage location" below). The threat
model relies on the **delete-after-source** invariant, not on the
backing filesystem being non-persistent.

## Decision

### Storage: `zalando/go-keyring`

Single runtime dep. Wraps the OS keyring:

| Platform | Backend                                        |
| -------- | ---------------------------------------------- |
| macOS    | Keychain                                       |
| Linux    | Secret Service via dbus (libsecret-compatible) |

Service name: `af` (configurable via `[secret].keyring_service` —
defaults to `"af"`, no `af/` prefix on accounts per the simplification
mandate).

Account names are the credential keys: `anthropic_api_key`,
`openai_api_key`, `github_token`, etc. v1 ships a curated list; the
user can store arbitrary keys.

### `af auth` subcommands

```
af auth set <key>            # prompts for value via terminal echo-off
af auth get <key>            # prints to stdout (TTY only; redacted otherwise)
af auth status               # lists known keys with availability + source
af auth clear <key>          # removes from keyring
af auth list                 # lists all af-stored keys (names only, no values)
```

### Storage location

The envelope path is selected at runtime:

| Platform                                  | Path                                             | Filesystem         |
| ----------------------------------------- | ------------------------------------------------ | ------------------ |
| Linux with `/run/user/$UID/` writable     | `/run/user/$UID/af-<session>/.env`               | tmpfs (RAM-backed) |
| macOS, or Linux without `/run/user/$UID/` | `~/.local/share/af/v1/secrets/af-<session>/.env` | persistent disk    |

On the persistent-disk fallback, the directory tree is created by `af
setup` with mode `0700`. The envelope file itself is `0600`.

### Transport: write → mount/copy → source → delete

When `af` launches an agent in a sandbox or via SSH, secrets reach the
agent via the envelope:

1. **Write**: `af` writes the envelope at the path above with mode `0600`. Lines like `ANTHROPIC_API_KEY=sk-...`.
2. **Mount/copy**:
   - **slicer**: VM mount the envelope's directory into the VM at the same path via VirtioFS.
   - **sbx**: container `--env-file` flag pointing at the host envelope, OR `cp` the envelope into the container before agent launch.
   - **remote (no sandbox)**: `scp` the envelope to `/run/user/$UID/af-<session>/.env` (or `~/.local/share/af/v1/secrets/af-<session>/.env`, matching the remote's available filesystems) with `chmod 600`.
3. **Source**: agent's launch wrapper sources the envelope (`. /path/to/.env`) once, then exec's the agent.
4. **Delete**: **immediately after source-and-exec**, the launch wrapper deletes the envelope (`rm -f /path/to/.env`). The agent process keeps the env vars in its own `environ`; nothing on disk persists past launch.

### Cleanup invariants

- Step 4 is **not optional**. The launch wrapper is structured as a
  single shell snippet (`. /path/to/.env && rm -f /path/to/.env && exec
<agent-cmd>`) so the delete cannot be skipped without also failing
  the launch.
- If `af` crashes between step 1 and step 4 — e.g. the user `^C`s
  during `af resume` — a stray envelope can remain on disk. To
  mitigate this, **every `af` invocation that touches the secrets
  directory performs an inline sweep first**: it `glob`s
  `~/.local/share/af/v1/secrets/af-*`, `stat`s each, and `unlink`s
  any whose mtime is older than 60 minutes. The sweep is
  fail-soft (a `slog.Warn` on per-file errors; the original command
  proceeds). This is _lazy_ by design: af is a CLI, not a daemon, so
  there's no cron/launchd/systemd-user wiring — cleanup runs the next
  time a human triggers `af create`/`af resume`/`af agent add`/etc.
  (On Linux tmpfs the contents also disappear on reboot, which is
  the strict-mode user's preferred guarantee.)
- The remote tmpfs path is cleaned up by `af done` and `af suspend`
  via an explicit `ssh <host> rm -rf /run/user/$UID/af-<session>` step.
  Failures are logged but do not block teardown.

### Why never `SSH SetEnv`/`SendEnv`

- Env vars set via `SetEnv` end up in `/proc/<pid>/environ` for **every** process spawned in the SSH session — visible to anyone with the right read access.
- `SendEnv` requires the remote's `sshd_config` to allow specific names; a portable secret-injection mechanism shouldn't depend on remote config.
- Many shell histories capture command lines; passing `--api-key sk-...` arguments leaks via shell history.

The ephemeral envelope is sourced once and the file deleted; the
secret exists in the agent's process env (where it's needed) but
nowhere else durable. On Linux with `/run/user/$UID/` the file lives
on tmpfs, so even if delete-after-source were skipped the contents
disappear on reboot; on the persistent-disk fallback the
source-and-delete invariant plus the lazy 60-minute sweep are the
guarantee.

### Redaction in logs

`slog` handler at `internal/secret/redact.go` wraps the default
handler. On every log record, it walks the attribute tree and replaces
values for known sensitive keys (`api_key`, `token`, `password`,
`bearer`, `secret`, `auth`) with `<redacted>`.

The redaction list is hard-coded; `[secret].redact_keys` config
extends it.

### Threat model

**In scope** (the design defends against these):

- **Local file leakage** of long-lived secrets: `state.toml` and
  `ledger.jsonl` never contain secrets at any point. The keyring is
  the only durable storage.
- **Process-env leakage** to unrelated processes on the remote: only
  the agent process gets the secret env, not every shell command run
  on the remote (which is what `SSH SendEnv` would cause).
- **Log leakage**: redaction handler scrubs known sensitive keys.
- **Persistent-on-disk envelopes**: the source-and-delete launch
  invariant (§"Cleanup invariants") ensures envelopes don't outlive
  agent launch under normal flow.

**Out of scope** (acknowledged limitations):

- **Memory dumps** from the running agent process.
- **Compromised agent binaries**.
- **Compromised user account** on the local machine (the keyring is
  only as secure as the user's login session).
- **Crash-window envelope leakage**: an envelope can persist on disk
  if `af` is killed (`kill -9` from another process) between the
  write and source-and-delete. The lazy 60-minute sweep caps the
  exposure window. If this is unacceptable, the user can run
  exclusively on Linux with `/run/user/$UID/` (tmpfs guarantees
  reboot clears state).
- **Filesystem snapshots / Time Machine**: macOS Time Machine may
  snapshot `~/.local/share/af/v1/secrets/` between write and delete.
  Adding the directory to Time Machine's exclusion list is a manual
  step the user takes; `af setup` does not configure this.

### Local-only workstreams

Local workstreams (no `--remote`, no `--sandbox`) inherit the user's
ambient env. No envelope is created; the agent inherits whatever
`ANTHROPIC_API_KEY` etc. are already set in the parent shell. The
keyring is consulted only when the env var is missing.

### `af setup` and the secrets directory

`af setup` (per ADR-045) creates `~/.local/share/af/v1/secrets/` with
mode `0700`. This serves two purposes:

1. **Fallback envelope location** when `/run/user/$UID/` is unavailable
   (macOS, or Linux without systemd-style user runtime dirs).
2. **Stale-envelope cleanup target** for the lazy sweep described
   under §"Cleanup invariants."

## Consequences

- One runtime dep (`zalando/go-keyring`).
- No agent ever sees a secret on its command line or in
  `/proc/.../environ` of unrelated processes.
- `af auth` is the single user-facing surface for secret storage.
- Linux Secret Service is required on Linux for non-fallback storage;
  if absent, `af auth` errors with a hint to install `gnome-keyring` or `kwallet`.

## Alternatives Considered

- **Pass secrets via SSH `SendEnv`.** Rejected per security analysis above.
- **Store secrets in TOML config.** Rejected; config files end up in dotfiles repos accidentally.
- **`99designs/keyring`.** Equivalent functionality, larger dep tree, more backends than we need; rejected for being over-spec.
- **Roll our own AES-encrypted file at `~/.local/share/af/v1/secrets/keyring.bin`.** Rejected; OS keyring is the right primitive.
- **No secret management at all; require user to source `.env` manually.** Rejected; that's what the owner wants `af` to do _for_ them.

## References

- v0 ADR-016 (Secret Storage), v0 ADR-025 (Secret Boundaries) — superseded for v1.
- ADR-031 — v1 master, dep set.
- ADR-036 — `[secret].keyring_service` config.
- ADR-041 — SSH remote (no SetEnv/SendEnv).
- ADR-042 — sandbox transport.
- ADR-045 — `af setup` creates secrets directory.
