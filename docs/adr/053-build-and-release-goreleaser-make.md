---
adr: 053
title: "Build & Release — goreleaser + Make"
status: proposed
implementation: pending
date: 2026-05-06
last_modified: 2026-05-06
supersedes: []
superseded_by: null
related: ["031", "034", "050", "051"]
tags: ["go", "build", "release", "goreleaser"]
---

# ADR-053: Build & Release — goreleaser + Make

## Context

v1 builds, tests, and cross-compiles the `af` binary for the owner's
target platforms. Build-tool decisions:

- **Make replaces just** per user directive.
- `goreleaser` cross-compiles for `linux/{amd64,arm64}` +
  `darwin/{amd64,arm64}` by default.
- `go install github.com/kakkoyun/af@latest` works.
- **No release** in the v0 sense — single user, no GitHub Releases, no
  Homebrew tap. `goreleaser` runs in `--snapshot` mode for local
  cross-compile sanity-checks; tagged releases are not part of v1.

## Decision

### Makefile

Top-level `Makefile` exposes the standard recipes. Pinned tool
versions ensure reproducibility.

```makefile
GOLANGCI_LINT_VERSION = 2.3.0
GOFUMPT_VERSION       = 0.7.0
GORELEASER_VERSION    = 2.5.0

GO          ?= go
GOLANGCI    ?= go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v$(GOLANGCI_LINT_VERSION)
GOFUMPT     ?= go run mvdan.cc/gofumpt@v$(GOFUMPT_VERSION)
GORELEASER  ?= go run github.com/goreleaser/goreleaser/v2@v$(GORELEASER_VERSION)

.PHONY: all build test test-property lint fmt fmt-check check install \
        release-snapshot clean

all: check

build:
	$(GO) build -o bin/af ./cmd/af

test:
	$(GO) test -race -count=1 -shuffle=on ./...

test-property:
	$(GO) test -run TestProperty -count=10000 -timeout 120s ./...

lint:
	$(GOLANGCI) run

fmt:
	$(GOFUMPT) -w .
	$(GO) tool goimports -w -local github.com/kakkoyun/af .

fmt-check:
	@out=$$($(GOFUMPT) -l .); if [ -n "$$out" ]; then echo "gofumpt:" "$$out"; exit 1; fi
	@out=$$($(GO) tool goimports -l -local github.com/kakkoyun/af .); if [ -n "$$out" ]; then echo "goimports:" "$$out"; exit 1; fi

check: fmt-check lint test

install:
	$(GO) install ./cmd/af

release-snapshot:
	$(GORELEASER) release --snapshot --clean

clean:
	rm -rf bin/ dist/
```

`go run` for tool invocation avoids requiring a separate install step;
the version is pinned in this `Makefile`. CI calls `make check`.

### `.goreleaser.yml` skeleton

```yaml
version: 2

project_name: af

builds:
  - id: af
    main: ./cmd/af
    binary: af
    env:
      - CGO_ENABLED=0
    goos: [linux, darwin]
    goarch: [amd64, arm64]
    ldflags:
      - -s -w
      - -X github.com/kakkoyun/af/internal/version.Version={{.Version}}
      - -X github.com/kakkoyun/af/internal/version.Commit={{.Commit}}
      - -X github.com/kakkoyun/af/internal/version.Date={{.Date}}

archives:
  - id: af
    name_template: "af_{{.Version}}_{{.Os}}_{{.Arch}}"
    format: tar.gz

checksum:
  name_template: "af_{{.Version}}_checksums.txt"
  algorithm: sha256

snapshot:
  name_template: "{{ incpatch .Version }}-snapshot"

release:
  disable: true
```

`release.disable: true` means no GitHub Release is ever created —
matches the v1 single-user constraint. `make release-snapshot`
produces `dist/` artefacts locally for sanity check.

### `internal/version/`

```go
package version

var (
    Version = "dev"
    Commit  = "none"
    Date    = "unknown"
)

func String() string { /* "af 1.0.0 (abc1234, 2026-05-06)" */ }
```

`af version` prints `version.String()`. Build-time injection via
`-ldflags -X` in `goreleaser` and `make build` (when the user wants a
real version stamp on a local build).

### CI

Single workflow at `.github/workflows/check.yml`:

```yaml
name: check
on: [push, pull_request]
jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: stable }
      - run: make check
```

No release workflow. No matrix runs (single OS for CI is sufficient
since cross-compile is a `goreleaser` concern, exercised by `make
release-snapshot` if the owner wants to verify).

### Cross-compile verification

`make release-snapshot` produces:

```
dist/
├── af_<version>_linux_amd64/af
├── af_<version>_linux_arm64/af
├── af_<version>_darwin_amd64/af
├── af_<version>_darwin_arm64/af
├── af_<version>_linux_amd64.tar.gz
├── ...
└── af_<version>_checksums.txt
```

The owner runs this locally before declaring "v1.0.0" complete.

### Future: Homebrew tap

Not in scope for v1. If v1 escapes single-user scope, a follow-up ADR
adds a `brews:` block to `.goreleaser.yml`.

### Distribution paths

| Path | What |
|---|---|
| `go install github.com/kakkoyun/af@latest` | Most users. Uses the latest commit on `main`; no release tag needed. |
| `make install` | Local clone. Build from current source; `bin/af` ends up in `$GOBIN`. |
| `make release-snapshot` | Local cross-compile; no install. |

There is no GitHub Release path in v1. There is no `gh release
download`. There are no signed checksums.

## Consequences

- Build is one `make` invocation away.
- Cross-compile is verified locally, not in CI.
- Tooling versions are explicit in the Makefile; no drift.
- The release path stays small: when v1.0 is "complete," the owner
  builds locally, smoke-tests, and uses the binary. No tag, no
  changelog update, no announcement.

## Alternatives Considered

- **Drop goreleaser; raw `go build` for each target.** Rejected; goreleaser handles archive + checksums + version-injection in one config.
- **Use `mage` instead of Make.** Rejected; Make is universally available, mage requires installation.
- **Tag releases on GitHub.** Rejected per v1 scope; if it changes, ADR-053 is amended.
- **Cross-compile in CI** (multi-OS matrix). Rejected; flaky, slow, and unnecessary for a single-user tool.

## References

- [`goreleaser` documentation](https://goreleaser.com/)
- [Go release notes — toolchain pinning](https://go.dev/doc/toolchain)
- v0 ADR-021 — superseded for v1.
- ADR-031 — v1 master, no-release directive.
- ADR-034 — Go module layout.
- ADR-050 — `golangci-lint` version pin.
- ADR-051 — `make test` and `make test-property` recipes.
