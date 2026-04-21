# ADR-016: Secret Storage for `af auth`

**Status:** Accepted
**Date:** 2026-04-21

## Context

Agent providers require API keys (e.g., `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`,
`GEMINI_API_KEY`). Currently, users export these in their shell profile or set
them in project `.env` files. This has two problems:

1. **Discovery friction.** `af create` does not check for required keys or explain
   which are missing. The agent fails silently or with a confusing error.
2. **Portability.** On remote sessions (exe.dev), keys must be re-exported in the
   remote environment. There is no injection pathway today.

`af auth` will provide `setup`, `reroll`, `status`, and `clear` subcommands that
store and retrieve secrets from the OS keyring, then inject them into agent launch
environments.

## Decision

### Library: `keyring` crate (v3.x)

Use the [`keyring`](https://crates.io/crates/keyring) crate. Rationale:

- Cross-platform: macOS Security framework, Linux `org.freedesktop.secrets` (D-Bus),
  Windows Credential Manager.
- Actively maintained (v3 released 2024, v2 LTS maintained).
- Well-audited public API; no unsafe in the crate itself.
- The `mock` feature provides an in-memory backend for tests (no daemon required in CI).

### Supported backends

| Platform | Backend | Notes |
|---|---|---|
| macOS | macOS Keychain (Security framework) | Available on all Mac hardware |
| Arch Linux | `org.freedesktop.secrets` D-Bus | Requires `kwallet` or `gnome-keyring` |

No encrypted-file fallback. If the Secret Service daemon is unavailable on Linux,
`af auth setup` exits with an actionable error:

```
error: no secret service available — install and unlock kwallet or gnome-keyring
       On Arch: pacman -S kwallet kwallet-pam
                or: pacman -S gnome-keyring libsecret
```

This keeps the security model clean: the daemon manages key encryption and locking.
A file-based fallback would require its own key-management strategy (where does the
file encryption key live?) and is out of scope.

### `af auth` subcommand design

```
af auth setup --provider <name>     # Prompt for API key, store in keyring
af auth reroll --provider <name>    # Overwrite existing entry (re-prompt)
af auth status                      # List which providers have stored keys (never prints values)
af auth clear --provider <name>     # Delete the keyring entry
```

Provider names match the `--agent` flag values: `claude`, `pi`, `codex`, `gemini`,
`amp`, `copilot`. The keyring service name is `af` and the account is
`af/<provider-name>` (e.g., `af/claude`).

### Injection into agent launch env

`src/auth/mod.rs` exports `inject(env: &mut HashMap<String, String>, provider: &str)`.
Each agent provider's `launch()` implementation calls `auth::inject` before building
its `Command`. If no key is stored, inject is a no-op (existing shell env takes
precedence, preserving backwards compatibility).

### Cargo feature gate

The `keyring` crate is an optional dep gated behind the `keyring` Cargo feature
(already scaffolded in Lane D). Default-on. Users who build without keyring support
(e.g., minimal CI builds) get a compile-time stub that always returns `NotFound`.

### Testing

- Unit tests use `keyring`'s in-memory backend (`keyring::set_default_credential_builder`
  with `MockCredentialBuilder`). No daemon required.
- Integration test (`#[cfg(feature = "keyring")]`) does a real round-trip on the
  developer machine.
- `auth::inject` is a pure function (no global state) and is tested independently.

## Consequences

- `af auth` adds a discoverable, consistent secret-management UX.
- Remote session injection is now possible: `inject` is called during `af create --remote`,
  and the key is forwarded over the SSH session environment (using `SetEnv` or
  `-o SendEnv` in the SSH command arguments).
- Users must have kwallet or gnome-keyring running on Arch Linux. This is standard
  for any desktop session; headless servers without a DE need manual configuration.
- No Windows support is planned; this is not a target platform.
- `keyring` v3 has no known CVEs at time of writing (verified via cargo-audit).
