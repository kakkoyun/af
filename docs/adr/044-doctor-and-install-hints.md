---
adr: 044
title: "af doctor + Install Hints (local & --remote)"
status: proposed
implementation: pending
date: 2026-05-06
last_modified: 2026-05-06
supersedes: []
superseded_by: null
related: ["031", "041", "042", "043", "045"]
tags: ["go", "doctor", "install"]
---

# ADR-044: `af doctor` — Install Hints, Never Auto-Install

## Context

v0's `af doctor --fix` auto-installed missing dependencies via the
platform package manager. That coupled `af` to package-manager state
and platform detection it didn't really need; the owner's machines are
already curated by dotfiles. v1 makes `doctor` strictly **read-only**:
it probes, it reports, it suggests install commands, it does **not**
install.

A separate command `af setup` (ADR-045) handles the user-scope
environment setup that **doesn't** need package-manager privileges.

`af doctor` must also work over SSH so the owner can verify
remote-machine readiness before launching a workstream there.

## Decision

### Local probe

`af doctor` checks the following tools on the local machine, grouped
by tier:

| Tier   | Tool                         | Required for                    |
| ------ | ---------------------------- | ------------------------------- |
| Must   | `git`                        | core git operations             |
| Must   | `tmux`                       | multiplexer (ADR-040)           |
| Must   | one of `pi`/`claude`/`codex` | at least one agent (ADR-043)    |
| Should | `gh`                         | PR detection, `af pr` (ADR-048) |
| Should | `fzf`                        | session picker in `af resume`   |
| Nice   | `slicer`                     | local sandbox (ADR-042)         |
| Nice   | `sbx`                        | local sandbox (ADR-042)         |
| Nice   | `delta`, `diff-so-fancy`     | nicer `af diff` rendering       |

Plus any binaries listed in `[doctor].extra_tools` config.

### Output

```
$ af doctor

Local environment:
  ✓ git           (/opt/homebrew/bin/git, version 2.43.0)
  ✓ tmux          (/opt/homebrew/bin/tmux, version 3.4)
  ✓ pi            (/opt/homebrew/bin/pi, version 0.73.0)
  ✗ claude        not in PATH
        → install: npm install -g @anthropic-ai/claude-code
  ✓ codex         (/opt/homebrew/bin/codex, version 1.2.0)
  ✓ gh            (/opt/homebrew/bin/gh, version 2.40.0)
  ⚠ slicer        not in PATH (optional; only needed for --sandbox slicer)
        → install: see https://slicervm.com/install
  ⚠ sbx           not in PATH (optional; only needed for --sandbox sbx)
        → install: see https://docs.docker.com/ai/sandboxes/

Status: 1 missing required tool.
```

Exit code: `0` if all `Must` tier tools present, `1` if any are
missing. `Should`/`Nice` tier missing tools warn but don't fail.

### `--verbose`

Shows full version output (not just `--version` first line) and the
result of `which`. Used for debugging unusual install paths.

### Remote probe (`--remote <host>`)

`af doctor --remote <host>`:

1. Connects via `ssh <host>` (using `[remote].ssh_options` per ADR-036).
2. Detects the remote OS via `cat /etc/os-release`.
3. Runs the same tool probe over SSH.
4. Prints install commands appropriate for the **remote's** package manager.

### Install hints by platform

| Platform        | Detection                                 | Install command pattern |
| --------------- | ----------------------------------------- | ----------------------- |
| macOS           | `uname -s == Darwin`                      | `brew install <pkg>`    |
| Arch / Manjaro  | `/etc/os-release ID == arch \| manjaro`   | `pacman -S <pkg>`       |
| Debian / Ubuntu | `/etc/os-release ID_LIKE contains debian` | `apt install <pkg>`     |
| Other Linux     | fallback                                  | "see upstream docs"     |
| Other           | fallback                                  | "see upstream docs"     |

For tools without distro packages (e.g. `pi`, `claude`, `slicer`),
print the upstream install URL or `npm install -g <pkg>` / `cargo
install <pkg>` as appropriate.

### What `af doctor` does NOT do

- **Does not run `sudo`.** Even on `--fix`. (There is no `--fix` in v1.)
- **Does not modify config files.** (That's `af setup`.)
- **Does not install anything.**
- **Does not detect tmux server liveness.** That's `af list`.

### Concurrency

`af doctor` is read-only: no flock, no state.toml writes.

## Consequences

- The doctor is a pure diagnostic tool. The user can copy-paste install commands.
- Remote doctor is the only "fix the remote" mechanism in v1. The owner is comfortable installing manually.
- The platform-package-manager integration shrinks from a code-driven module (v0) to a pair of detect-and-print functions.

## Alternatives Considered

- **Keep `--fix`** from v0. Rejected; couples `af` to package-manager state, conflicts with the dotfiles-as-companion model.
- **Drop the doctor command entirely.** Rejected; the install hints are a real productivity win when working from a fresh machine.
- **Doctor over SSH installs remotely with `--fix`.** Rejected; same reasoning as above, doubly so on a remote.

## References

- v0 ADR-009 (Provisioning), v0 ADR-010 (Platform-Aware Dependencies) — superseded for v1.
- ADR-031 — v1 master.
- ADR-036 — `[doctor].extra_tools` config.
- ADR-041 — SSH remote (doctor uses the same SSH machinery).
- ADR-042 — sandbox provider binaries probed.
- ADR-043 — agent binaries probed.
- ADR-045 — `af setup` complements doctor with user-scope writes.
