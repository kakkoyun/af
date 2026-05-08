---
adr: 049
title: "Secret Management (keyring + tmpfs envelope)"
status: proposed
implementation: pending
date: 2026-05-06
last_modified: 2026-05-06
supersedes: []
superseded_by: null
related: ["031", "036", "041", "042", "045"]
tags: ["go", "secrets", "keyring", "security"]
---

# ADR-049: Secret Management (keyring + tmpfs envelope)

## Context

Sandboxed and remote workstreams need API tokens (Anthropic key,
OpenAI key, GitHub token, etc.) injected into the agent's environment
without leaving them on disk in plaintext or leaking via env vars
visible to every process on the remote machine.

v0's secret management grew across multiple ADRs (016, 025) with a
multi-tier lookup chain, custom redaction, and Linux dbus tweaks. v1
collapses this to: **keyring for storage + tmpfs envelope file for
transport, no SSH `SetEnv`/`SendEnv`**.

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

### Transport to local sandbox / remote: tmpfs envelope file

When `af` launches an agent in a sandbox or via SSH, secrets reach the
agent via a **tmpfs envelope file**:

1. **Write**: `af` writes `/run/user/$UID/af-<session>/.env` with mode `0600`. Lines like `ANTHROPIC_API_KEY=sk-...`.
2. **Mount/copy**:
   - **slicer**: VM mount `/run/user/$UID/af-<session>/` into the VM at the same path via VirtioFS.
   - **sbx**: container `--env-file` flag pointing at the host envelope, OR `cp` the envelope into the container before agent launch.
   - **remote (no sandbox)**: `scp` the envelope to the same path on the remote, with `chmod 600`.
3. **Source**: agent's launch wrapper sources the envelope (`. /run/user/$UID/af-<session>/.env`) once, then exec's the agent.
4. **Cleanup**: after agent launch, `af` deletes the envelope. (For sbx, the in-container copy is removed via the post-launch cleanup step.)

### Why never `SSH SetEnv`/`SendEnv`

- Env vars set via `SetEnv` end up in `/proc/<pid>/environ` for **every** process spawned in the SSH session — visible to anyone with the right read access.
- `SendEnv` requires the remote's `sshd_config` to allow specific names; a portable secret-injection mechanism shouldn't depend on remote config.
- Many shell histories capture command lines; passing `--api-key sk-...` arguments leaks via shell history.

The tmpfs envelope is sourced once and the file deleted; the secret
exists in the agent's process env (where it's needed) but nowhere else
durable.

### Redaction in logs

`slog` handler at `internal/secret/redact.go` wraps the default
handler. On every log record, it walks the attribute tree and replaces
values for known sensitive keys (`api_key`, `token`, `password`,
`bearer`, `secret`, `auth`) with `<redacted>`.

The redaction list is hard-coded; `[secret].redact_keys` config
extends it.

### Threat model

In scope:

- Local file leakage: `state.toml` and `ledger.jsonl` never contain secrets.
- Process env leakage: only the agent process gets the secret env, not
  every shell command run on the remote.
- Log leakage: redaction handler scrubs known keys.

Out of scope:

- Memory dumps from the running agent process.
- Compromised agent binaries.
- Compromised user account on the local machine (the keyring is only
  as secure as the user's login session).

### Local-only workstreams

Local workstreams (no `--remote`, no `--sandbox`) inherit the user's
ambient env. No tmpfs envelope is created; the agent inherits whatever
`ANTHROPIC_API_KEY` etc. are already set in the parent shell. The
keyring is consulted only when the env var is missing.

### `af setup` and the secrets directory

`af setup` (per ADR-045) creates `~/.local/share/af/v1/secrets/` with
mode `0700`. This is the **fallback** location for envelope files
when `/run/user/$UID/` is not available (e.g. macOS, where `tmpfs` at
that path doesn't exist by default). On macOS, `af` uses
`~/.local/share/af/v1/secrets/af-<session>/.env` instead, with the
same `0600` cleanup discipline.

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
