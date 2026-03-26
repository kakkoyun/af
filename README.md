# af

**af** — agentic-flow, automatic-flow, or as-fuck.

Workflow tooling for agentic/automatic programming.

## Installation

### From source

```bash
cargo install --locked --git https://github.com/kakkoyun/af
```

### From release binaries

Download from [GitHub Releases](https://github.com/kakkoyun/af/releases) — prebuilt for:

| Target | Description |
|---|---|
| `x86_64-unknown-linux-gnu` | Linux x86_64 (glibc) |
| `x86_64-unknown-linux-musl` | Linux x86_64 (static) |
| `aarch64-unknown-linux-gnu` | Linux ARM64 (glibc) |
| `aarch64-unknown-linux-musl` | Linux ARM64 (static) |
| `x86_64-apple-darwin` | macOS Intel |
| `aarch64-apple-darwin` | macOS Apple Silicon |

## Usage

```bash
af version    # Print version
af --help     # Show help
```

## Development

Requires: Rust 1.85+, [`just`](https://github.com/casey/just)

```bash
# Install dev tools
just install-tools

# Install git hooks
just install-hooks

# Run all checks (format, lint, test, deny)
just check

# Quick iteration
just fmt && just lint
just test
```

## License

[MIT](LICENSE)
