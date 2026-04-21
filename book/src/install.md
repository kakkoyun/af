# Installation

## From source (recommended while in alpha)

```bash
cargo install --locked --git https://github.com/kakkoyun/af
```

Requires Rust 1.85 or newer. Install Rust via [rustup](https://rustup.rs).

## From release binaries

Download pre-built binaries from
[GitHub Releases](https://github.com/kakkoyun/af/releases):

| Target | Description |
|---|---|
| `x86_64-unknown-linux-gnu` | Linux x86_64 (glibc) |
| `x86_64-unknown-linux-musl` | Linux x86_64 (static) |
| `aarch64-unknown-linux-gnu` | Linux ARM64 (glibc) |
| `aarch64-unknown-linux-musl` | Linux ARM64 (static) |
| `x86_64-apple-darwin` | macOS Intel |
| `aarch64-apple-darwin` | macOS Apple Silicon |

SHA256 checksums are published alongside each binary.

## Prerequisites

`af` requires a handful of external tools. Check what is missing:

```bash
af doctor
```

Install missing dependencies automatically:

```bash
af doctor --fix
```

Required tools:

- `git` — for worktree management
- `tmux` — default multiplexer (other muxers planned)
- At least one AI agent binary (`claude`, `pi`, `codex`, `gemini`, `amp`, or `copilot`)

Optional tools (unlocked by flags):

- `gh` — required for `--from-pr` and `af pr`
- `slicer` — required for `--sandbox` (Firecracker isolation)
- `fzf` — used by `af resume` for interactive session picking
- `delta` or `diffity` — used by `af diff`
