---
adr: 045
title: "af setup — Environment Companion to Doctor"
status: proposed
implementation: in-progress
date: 2026-05-06
last_modified: 2026-05-21
supersedes: []
superseded_by: null
related: ["031", "035", "036", "038", "044", "047"]
tags: ["go", "setup", "command"]
---

# ADR-045: `af setup` — Environment Companion to `af doctor`

## Context

`af doctor` (ADR-044) is read-only: probe and report. The owner asked
for a separate command that **does the basic environment setup** —
things that `af` itself can do without `sudo` and without invoking a
package manager:

- Add `.af/` to the user's global gitignore so per-repo `.af/state.toml`
  symlinks don't leak into commits.
- Detect the user's shell and install completions.
- Initialise `~/.config/af/config.toml` if it doesn't exist.
- Make sure the state directory `~/.local/share/af/v1/` exists.
- Optionally print a hint about Obsidian vault config if it isn't set.

The boundary is sharp: **`af setup` writes user-scope files only**. It
does not run `sudo`, does not install packages, does not touch any
machine-level config. Anything requiring elevated privileges or
machine state belongs to dotfiles or to manual setup.

## Decision

### Command

```
af setup [--force] [--shell SHELL] [--skip-completions] [--skip-gitignore]
```

| Flag                 | Behaviour                                                                                |
| -------------------- | ---------------------------------------------------------------------------------------- |
| `--force`            | Overwrite existing `~/.config/af/config.toml`. By default, existing config is preserved. |
| `--shell`            | Override shell detection (one of `bash`, `zsh`, `fish`, `powershell`).                   |
| `--skip-completions` | Don't install shell completions.                                                         |
| `--skip-gitignore`   | Don't modify `~/.config/git/ignore`.                                                     |

### Steps (idempotent)

1. **Create state directory tree**:
   - `mkdir -p ~/.local/share/af/v1/sessions`
   - `mkdir -p ~/.local/share/af/v1/archive`
   - `mkdir -p ~/.local/share/af/v1/secrets` (mode 0700; for ADR-049)

2. **Initialise user config** if `~/.config/af/config.toml` doesn't exist (or `--force`):
   - `mkdir -p ~/.config/af`
   - Write the default config (per ADR-036) with header comment explaining each section.

3. **Add `.af/` to global gitignore** unless `--skip-gitignore`:
   - Read `git config --global --get core.excludesfile`. If unset, default to `~/.config/git/ignore`.
   - Ensure that file exists.
   - Append `.af/` if not already present (don't append duplicates; use `grep -q '^\.af/$'`-equivalent).
   - If `core.excludesfile` was unset, set it: `git config --global core.excludesfile ~/.config/git/ignore`.

4. **Detect shell** unless `--shell` is passed:
   - Look at `$SHELL`. Fall back to `getent passwd $USER`.
   - If unsupported (e.g. `tcsh`), warn and skip completions.

5. **Install completions** unless `--skip-completions`:
   - Generate via `cobra` (ADR-035): `af completions <shell>`.
   - Write to the shell's user-scope location:

   | Shell      | Path                                                                              |
   | ---------- | --------------------------------------------------------------------------------- |
   | bash       | `~/.local/share/bash-completion/completions/af`                                   |
   | zsh        | `~/.config/zsh/completions/_af` (and prompt user to add `fpath` entry if missing) |
   | fish       | `~/.config/fish/completions/af.fish`                                              |
   | powershell | `$PROFILE` directory; print install snippet                                       |

   Idempotent: overwriting is fine, file content is regenerated each
   run.

6. **Print Obsidian vault hint** if `[obsidian.vaults]` is empty:

   ```
   Tip: configure your Obsidian vault paths in ~/.config/af/config.toml
        under [obsidian.vaults] to enable `af note` and the workstream
        markdown integration. Example:
            [obsidian.vaults]
            personal = "/Users/you/Vaults/personal"
   ```

7. **Print summary**:
   ```
   af setup complete:
     ✓ state dir:     ~/.local/share/af/v1/
     ✓ user config:   ~/.config/af/config.toml (created)
     ✓ git ignore:    ~/.config/git/ignore (entry added)
     ✓ completions:   zsh installed at ~/.config/zsh/completions/_af
     ! obsidian:      [obsidian.vaults] empty — see hint above
   ```

### What `af setup` does NOT do

- **No `sudo`.** Ever.
- **No package installs.** That's the user's job, guided by `af doctor`.
- **No remote setup.** `af doctor --remote <host>` covers remote probes; remote setup is the user's responsibility.
- **No tmux config edits.** `af` doesn't own the user's `~/.tmux.conf`.
- **No agent CLI configuration.** Agents have their own `--init` / first-run flows.

### Idempotency

Running `af setup` twice is safe. Subsequent runs:

- Do not overwrite existing config (without `--force`).
- Do not duplicate gitignore lines.
- Re-emit the completion script (generated content; matching is fine).
- Re-create state-dir paths (a no-op if they exist).

### Cobra registration

`af setup` is implemented in `cmd/af/setup.go` per the ADR-035 idiom.
It depends on `internal/config` (default config), `internal/shell`
(detection + completion paths — small new package), and the cobra
generator from `github.com/spf13/cobra`.

## Consequences

- New users (or fresh machines) run `af setup` once before `af create`.
- The `~/.config/git/ignore` mechanism eliminates the need to copy a `.gitignore` snippet into every repo.
- Completions work without manually copying scripts around.
- The boundary with `doctor` is unambiguous: setup writes, doctor doesn't.

## Alternatives Considered

- **Auto-invoke `af setup` on first `af create`.** Rejected; surprising side-effects on first run.
- **Embed completions in `init()` and skip the install step.** Rejected; users still need them in their shell rc.
- **Combine `setup` and `doctor`** into one command with subcommands. Rejected; the read-only / writes split is conceptually clear and easy to remember.
- **Have `setup` install the binary itself** (`go install`). Rejected; circular and out of scope. The user installs the binary externally.

## References

- ADR-031 — v1 master, `af setup` is a new v1 command.
- ADR-035 — cobra completion generator.
- ADR-036 — default config to write.
- ADR-038 — `.af/` directory needs gitignore.
- ADR-044 — boundary with doctor.
- ADR-047 — Obsidian config hint.
