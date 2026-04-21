# ADR-025: Secret Boundaries

**Status:** Accepted
**Date:** 2026-04-21
**Extends:** ADR-016

## Context

ADR-016 defined keyring storage and env-var injection as the secret mechanism
for agent API keys. Subsequent research surfaced five problems:

1. **Env-var injection is wrong for sandboxed agents.** `sbx secret` uses a
   proxy that never exposes the secret to the agent (`sbx secret --help`:
   "The secret is never exposed directly to the agent."). Slicer has its own
   native `slicer secret` store. Workspaces has `workspaces secrets`. Injecting
   into a sandboxed session env is either redundant or outright wrong.
2. **SSH `SetEnv`/`SendEnv` is a data-leak vector.** On multi-tenant exe.dev,
   an API key forwarded this way lands in `/proc/<sshd-child>/environ` readable
   by any co-tenant. `sshd` debug configs can log `SetEnv` names.
3. **Plain `HashMap<String, String>` has no scrubbing guarantee.** One
   `tracing::debug!("env: {:?}", env)` or panic backtrace leaks the key.
4. **Linux Secret Service default collection is enumerable.** Any process in
   the unlocked user session can `SearchItems` and read all `af/*` entries.
5. **No rotation protocol.** `af auth clear` does not signal running agents;
   the clear is a false reassurance.

Per user directive D1, af does not sync secrets across provider-native stores.

## Decision

**Boundary rule (the decision, one sentence):** af's keyring stores secrets
for **host agents and exedev SSH sessions only**; every other path (sbx,
slicer, workspaces) defers to the provider-native secret store, with af merely
pointing the user at the correct CLI command.

### Concrete rules

1. **Keyring scope** (amends ADR-016 §Decision §"`af auth` subcommand design"):
   - Service name stays `af`. Account is `<provider>` (not `af/<provider>` —
     drops the redundant prefix per review finding [C 2.2]).
   - Linux: use a **dedicated, non-default collection** via D-Bus
     `CreateCollection`. Auto-lock on idle. Use opaque labels
     (`af-<uuid>` → lookup table in the collection attributes, not the label).
   - macOS: use the user's default Keychain (hardware-backed on Apple Silicon;
     the `security` CLI exposes entries but user-root is the threat-model
     boundary).

2. **Delivery transport** (supersedes ADR-016 §Consequences L91–93):
   - **Host:** inject via `auth::inject(env, provider)` — env-var. Wrap values
     in `secrecy::SecretString`; `Debug` prints `[REDACTED]`.
   - **exedev (SSH):** **forbid `SetEnv`/`SendEnv`.** Write to
     `/run/user/$UID/af-<session>/.env` mode `0600` on the remote, and have the
     agent read-once-then-unlink. Fallback: pipe on stdin to
     `af agent launch --read-env-from-stdin`. Both mechanisms leave no
     `/proc/*/environ` trace visible to sibling processes.
   - **sbx, slicer, workspaces sandboxes:** **do not inject.** `af doctor`
     checks the native store for the expected key; on miss, prints the exact
     CLI the user should run (`sbx secret set …`, `slicer secret create …`,
     `workspaces secrets set …`). af never touches those stores.

3. **Rotation and revocation protocol:**
   - `af auth clear --provider <name>` lists live sessions using that provider
     from the ledger, warns, and offers `--kill-sessions` to terminate them.
   - `af auth reroll --provider <name>` behaves the same: `--kill-sessions`
     kills stale launches; otherwise the change applies to **new launches
     only**.
   - Help text documents this explicitly: *"keyring changes affect new
     launches; running agents hold the key in memory until terminated."*

4. **Redaction enforcement:**
   - All secret values are `secrecy::SecretString` from retrieval through to
     `execve`.
   - CI grep gate denies `{:?}` formatting on types named `*Env*` or containing
     `Secret` as a field.
   - A panic hook strips env from backtraces.

## Threat model

- **In scope:** casual shell-history exposure, accidental commit of `.env`
  files, basic malware in the user session, co-tenant on multi-tenant remote.
- **Out of scope:** root-level compromise, kernel-level attacks, physical
  access, adversarial sshd at provider level (that is the provider's threat
  model).

## Alternatives considered

- **af-level sync across provider stores** (the original ADR-025 draft).
  Rejected per D1 and the security findings: it expands blast radius, creates
  a rotation hazard, diverges the audit trail across N stores, and
  `workspaces secrets set KEY VAL` as argv leaks via `/proc/<pid>/cmdline`.
- **Encrypted-file fallback when Secret Service is down.** Rejected (ADR-016
  already): would require its own key-management strategy, out of scope.
- **User-provided encryption keys (à la `age` / `sops`).** Rejected: increases
  setup friction against marginal security benefit. macOS Keychain plus a
  dedicated Linux collection is the threat-model-appropriate baseline.

## Consequences

- `af auth setup/reroll/status/clear` scope is **host + exedev only**.
  Documented in help text and the book.
- Sandboxed-agent workflows require one-time secret setup in the
  provider-native store. `af doctor` guides the user. This is not an af
  regression — it matches how each sandbox tool is designed to work.
- Secret values never appear in `{:?}` output, panic backtraces, or SSH
  `SetEnv` headers.
- `af auth clear` is honest about what it does and does not terminate.
- **Implementation cost:** `secrecy` and `zeroize` added as deps (behind the
  `keyring` feature). Tmpfs delivery adds ~30 LOC to exedev bootstrap. The
  dedicated Linux collection adds ~40 LOC to `auth::init` (one-time
  `CreateCollection` if absent).
