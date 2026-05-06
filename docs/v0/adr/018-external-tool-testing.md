# ADR-018: External Tool Dependency Testing (CommandRunner Trait + Feature Gates)

**Status:** Accepted
**Date:** 2026-04-21

## Context

Several `af` providers shell out to external CLI tools that may not be installed on
every developer machine or CI runner:

- `provider/workspaces.rs` calls the `workspaces` CLI (DD Workspaces)
- `provider/slicer.rs` calls `slicer` (Firecracker MicroVM manager)
- `provider/exedev.rs` executes SSH commands via `ssh` and `scp`
- `provider/docker.rs` calls `sbx` (Docker AI Sandboxes)

The current `WorkspacesProvider` is a stub (`anyhow::bail!("not yet implemented")`).
When implementing real providers, tests that require the external binary would fail on
machines without it, making TDD impractical and CI fragile.

Two patterns could address this:

1. **Inject a `CommandRunner` trait** — abstract `std::process::Command` behind a trait so
   tests inject a fake that returns canned output without spawning a real process.
2. **Feature-gate integration tests** — tests that exercise the real binary run only when
   the corresponding Cargo feature is enabled and the binary is present.

## Decision

### CommandRunner trait

Define a `CommandRunner` trait in `src/util/command.rs` (or `src/util/mod.rs`):

```rust
/// Abstracts process spawning so providers can be tested without real binaries.
pub trait CommandRunner: Send + Sync {
    fn run(&self, cmd: &str, args: &[&str]) -> anyhow::Result<std::process::Output>;
}

/// Production implementation — delegates to std::process::Command.
pub struct SystemCommandRunner;

impl CommandRunner for SystemCommandRunner {
    fn run(&self, cmd: &str, args: &[&str]) -> anyhow::Result<std::process::Output> {
        std::process::Command::new(cmd)
            .args(args)
            .output()
            .map_err(|e| anyhow::anyhow!("failed to run {cmd}: {e}"))
    }
}
```

Each provider that shells out holds a `Box<dyn CommandRunner>`. The constructor accepts
an optional runner; production code passes `SystemCommandRunner`, tests pass a fake.

### FakeCommandRunner for tests

```rust
#[cfg(test)]
pub struct FakeCommandRunner {
    pub responses: std::collections::HashMap<String, std::process::Output>,
}
```

The fake matches on the command name (first token) and returns the pre-configured
`Output`. Unregistered commands return a generic `Ok(exit 0, stdout: "")`. Tests
construct fakes with `FakeCommandRunner::new().with("workspaces", exit(0), "list output")`.

### Binary names

| Provider | Binary | Feature flag |
|---|---|---|
| WorkspacesProvider | `workspaces` | `workspaces` |
| SlicerProvider (remote) | `slicer` | `slicer-remote` |
| DockerProvider | `sbx` | (always present, sbx is bundled) |
| ExedevProvider | `ssh`, `scp` | (always present, standard POSIX) |

### Feature-gated integration tests

Integration tests that require a real binary live in `tests/<provider>_integration.rs`
behind `#[cfg(feature = "<flag>")]`. CI default job runs without those features;
a separate nightly job runs `cargo test --features workspaces,slicer-remote` on a
machine with the real binaries installed.

```rust
// tests/workspaces_integration.rs
#[cfg(feature = "workspaces")]
mod integration {
    #[test]
    fn workspaces_list_returns_json() {
        // Requires real `workspaces` binary in PATH
    }
}
```

### Placement

- `CommandRunner` trait + `SystemCommandRunner` → `src/util/command.rs`
- `FakeCommandRunner` → same file, `#[cfg(test)]` block
- Each provider's unit tests use `FakeCommandRunner` inline in the `#[cfg(test)]` module

## Consequences

- Provider unit tests pass on any machine without any external binary installed.
- Integration tests are opt-in via feature flags; default `cargo test` always passes.
- Adding a new shelling-out provider follows a documented pattern: implement
  `CommandRunner`, accept it in the constructor, write a fake for tests.
- The `CommandRunner` trait adds one layer of indirection per external call. This is
  acceptable: the layer is thin, the benefit (testability) is concrete.
- `FakeCommandRunner` is test-only. It is never compiled into release builds.
