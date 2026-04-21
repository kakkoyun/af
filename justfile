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

# ── Release ────────────────────────────────────────────────────────────────────

# Dry-run the release workflow against a throwaway tag to verify all 6 matrix
# build targets before pushing a real version tag. See ADR-021.
release-dry-run:
    gh workflow run release.yml -f tag=v0.0.0-dry
    @echo "Monitor:  gh run list --workflow=release.yml --limit 1"
    @echo "Cleanup:  gh release delete v0.0.0-dry --yes 2>/dev/null; git push origin :refs/tags/v0.0.0-dry 2>/dev/null || true"

# ── Docs ───────────────────────────────────────────────────────────────────────

# Generate command reference pages in book/src/commands/ from --help output.
# Requires the binary to be built. See ADR-020 and Lane C1.
book-gen: build
    @echo "book-gen: stub — full implementation in Lane C1"
    @echo "Will generate book/src/commands/*.md from 'af <cmd> --help' output"

# Build the mdBook user guide (requires mdbook: cargo install mdbook)
book-build:
    mdbook build book

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
