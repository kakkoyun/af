# af — development task runner
# Install just: cargo install just
# Usage: just <recipe>

set shell := ["bash", "-euo", "pipefail", "-c"]

# Default: show available recipes
default:
    @just --list

# ── Quality ────────────────────────────────────────────────────────────────────

# Run all checks (what CI runs)
check: fmt-check lint test deny doc-check

# Format code
fmt:
    cargo fmt --all

# Check formatting (CI mode)
fmt-check:
    cargo fmt --all -- --check

# Run clippy with all the pedantic lints
lint:
    cargo clippy --all-targets --all-features -- -D warnings

# Run tests
test:
    cargo test --all-features

# Run tests with nextest (if installed)
test-next:
    cargo nextest run --all-features

# Run cargo-deny checks (licenses, advisories, bans)
deny:
    cargo deny check

# Audit dependencies for security vulnerabilities
audit:
    cargo audit

# Find unused dependencies
machete:
    cargo machete

# ── Build ──────────────────────────────────────────────────────────────────────

# Build in debug mode
build:
    cargo build

# Build in release mode
build-release:
    cargo build --release

# Build for all supported targets (requires cross)
build-cross target:
    cross build --release --target {{ target }}

# ── Run ────────────────────────────────────────────────────────────────────────

# Run the CLI
run *args:
    cargo run -- {{ args }}

# Run with release optimizations
run-release *args:
    cargo run --release -- {{ args }}

# ── Maintenance ────────────────────────────────────────────────────────────────

# Install all development tools
install-tools:
    cargo install cargo-deny cargo-audit cargo-machete cargo-nextest cargo-release just

# Update dependencies
update:
    cargo update

# Clean build artifacts
clean:
    cargo clean

# Build docs (warnings are errors)
doc-check:
    RUSTDOCFLAGS="-D warnings" cargo doc --no-deps --all-features

# Generate and open docs
doc:
    cargo doc --no-deps --open

# ── Git Hooks ──────────────────────────────────────────────────────────────────

# Install git pre-commit hook
install-hooks:
    @echo '#!/usr/bin/env bash' > .git/hooks/pre-commit
    @echo 'set -euo pipefail' >> .git/hooks/pre-commit
    @echo '' >> .git/hooks/pre-commit
    @echo '# Format check' >> .git/hooks/pre-commit
    @echo 'cargo fmt --all -- --check || {' >> .git/hooks/pre-commit
    @echo '    echo "❌ cargo fmt failed. Run: just fmt"' >> .git/hooks/pre-commit
    @echo '    exit 1' >> .git/hooks/pre-commit
    @echo '}' >> .git/hooks/pre-commit
    @echo '' >> .git/hooks/pre-commit
    @echo '# Clippy' >> .git/hooks/pre-commit
    @echo 'cargo clippy --all-targets --all-features -- -D warnings || {' >> .git/hooks/pre-commit
    @echo '    echo "❌ clippy failed. Fix warnings above."' >> .git/hooks/pre-commit
    @echo '    exit 1' >> .git/hooks/pre-commit
    @echo '}' >> .git/hooks/pre-commit
    @chmod +x .git/hooks/pre-commit
    @echo "✅ Pre-commit hook installed"
