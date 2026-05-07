---
adr: 050
title: "Code Quality — golangci-lint Pedantic"
status: proposed
implementation: pending
date: 2026-05-06
last_modified: 2026-05-06
supersedes: []
superseded_by: null
related: ["031", "034", "051", "053"]
tags: ["go", "lint", "quality"]
---

# ADR-050: Code Quality — golangci-lint Pedantic

## Context

The owner wants pedantic linting on par with v0's clippy + restriction
+ nursery configuration. Go's `golangci-lint` aggregates ~50 individual
linters; "pedantic" here means **enable all** with explicit
allow-listed exceptions for the rare cases that don't apply, rather
than picking a small subset.

The mandatory baseline (must always be on): `errcheck`, `staticcheck`,
`unparam`, `revive`, `gocritic`, `gosec`, `nolintlint`. Beyond those,
v1 enables every linter that fires non-trivially in the codebase, and
documents each disabled linter with a justification.

Pair this with `gofumpt` (stricter `gofmt`) and `goimports` for
formatting. CI checks `-l` (zero output) for both.

## Decision

### `.golangci.yml` skeleton

```yaml
version: "2"

run:
  timeout: 5m
  tests: true

linters:
  enable-all: true
  disable:
    # Justified disables — each has a reason in the body comment.
    - exhaustruct        # too noisy: Go literals don't always init every field
    - depguard           # we use a per-import allowlist instead (see ADR-031)
    - varnamelen         # often disagrees with idiomatic Go (i, j, _, ok)
    - lll                # gofumpt enforces line wrapping where it matters
    - wsl                # whitespace linter conflicts with gofumpt
    - wsl_v5             # ditto
    - tagliatelle        # struct tag naming policed via revive
    - nlreturn           # too pedantic for our taste
    - testpackage        # we mix _test and same-package tests deliberately
    - paralleltest       # paralleltest is good practice but not enforced

linters-settings:
  gocritic:
    enabled-tags: [diagnostic, style, performance, opinionated, experimental]
    disabled-checks:
      - hugeParam        # 80-byte threshold is too tight for our argv slices

  govet:
    enable-all: true

  errcheck:
    check-type-assertions: true
    check-blank: true

  revive:
    rules:
      - name: var-naming
      - name: exported
      - name: unused-parameter
      - name: package-comments
      - name: unused-receiver

  staticcheck:
    checks: ["all"]

  goconst:
    min-len: 3
    min-occurrences: 3

issues:
  max-issues-per-linter: 0
  max-same-issues: 0
  exclude-use-default: false
  exclude-rules:
    # Tests can be looser on a few rules.
    - path: _test\.go
      linters:
        - dupl
        - funlen
        - gocyclo
        - gocognit
        - goconst
        - errcheck
```

The exact allow-list will evolve at impl time as new linters fire on
real code. The principle: **disable explicitly, never silently**.

### Format

```bash
gofumpt -l .                  # CI fails if any output
goimports -l -local github.com/kakkoyun/af .   # CI fails if any output
```

`gofumpt -l` is the strict-`gofmt` equivalent. `goimports -local`
sorts the local module's imports last in the third group, per Go
convention.

### Inline exceptions

`//nolint:<linter>[,<linter>] // reason` is required for any
suppression. Without the reason comment, `nolintlint` flags it.

### Mandatory checks

- `errcheck`: every error returned must be checked or explicitly
  discarded with `_ =`.
- `staticcheck`: SA-codes are bugs, not style — fix, don't suppress.
- `unparam`: unused parameters indicate stale APIs.
- `revive` rules above: doc comments, package comments, unused
  receivers.
- `gocritic`: idiom + performance + opinionated style.
- `gosec`: security audit (no hard-coded credentials, no `MD5/SHA1`
  for security, no command injection patterns).
- `nolintlint`: no unjustified suppressions.

### Pre-commit and CI

`make lint` runs `golangci-lint run`. CI runs `make check` which is
`make fmt-check && make lint && make test`. PRs that fail any step
don't merge (single-user but the discipline still helps).

A pre-commit hook can run `make fmt && make lint` automatically if the
user wants; v1 doesn't ship one (the user installs whatever git hooks
they prefer in their dotfiles).

### Lint baselines

When the codebase first builds, `golangci-lint run` will fire on legit
issues. v1 fixes them all. **No `golangci-lint cache` baselines
allowed** — the bar is zero warnings on every PR.

### Versioning

`golangci-lint` is pinned to a specific version in `Makefile`:

```make
GOLANGCI_LINT_VERSION = 2.3.0
```

So linter bumps are explicit, not surprise-on-`go install`.

## Consequences

- Lint output is the gate for "is this PR mergeable?"
- New linters added to `golangci-lint` upstream may fire on existing
  code; bumping the version is a deliberate decision.
- Allow-list comments are documented; no folklore "we just don't
  enable that one."

## Alternatives Considered

- **Curated short list** (errcheck, staticcheck, govet only). Rejected; pedantic is the user's stated preference.
- **`enable-all` with no disables**. Rejected; some linters genuinely conflict with idiomatic Go.
- **Per-package config**. Rejected; one `.golangci.yml` for the whole module is simpler.
- **Tier-based linting** (must/should/nice). Rejected; lint output is binary (clean or not), not graded.

## References

- [`golangci-lint` documentation](https://golangci-lint.run/)
- v0 `Cargo.toml` clippy config — superseded by this ADR for v1 (different language).
- ADR-031 — v1 master.
- ADR-034 — Go module idiom (errors, slog, context — many of these reinforce lint rules).
- ADR-051 — testing strategy (test files have looser exclusions).
- ADR-053 — build (Makefile pins `golangci-lint` version).
