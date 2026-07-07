# shellcheck disable=SC2283,SC2276,SC2157 # Make syntax, not shell
GOLANGCI_LINT_VERSION = 2.3.0
GOFUMPT_VERSION       = 0.7.0
GOIMPORTS_VERSION     = 0.38.0
GORELEASER_VERSION    = 2.8.2

VERSION_PKG   = github.com/kakkoyun/af/internal/version
BUILD_VERSION ?= dev
GIT_COMMIT    ?= $(shell git rev-parse --short=12 HEAD 2>/dev/null || echo none)
BUILD_DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LD_FLAGS      ?= -X $(VERSION_PKG).Version=$(BUILD_VERSION) -X $(VERSION_PKG).Commit=$(GIT_COMMIT) -X $(VERSION_PKG).Date=$(BUILD_DATE)

GO         ?= go
# golangci-lint refuses to run when built with a Go older than the
# repo's target; force the repo toolchain for the build.
GOLANGCI   ?= GOTOOLCHAIN=$(shell $(GO) env GOVERSION) $(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v$(GOLANGCI_LINT_VERSION)
GOFUMPT    ?= $(GO) run mvdan.cc/gofumpt@v$(GOFUMPT_VERSION)
GOIMPORTS  ?= $(GO) run golang.org/x/tools/cmd/goimports@v$(GOIMPORTS_VERSION)
GORELEASER ?= $(GO) run github.com/goreleaser/goreleaser/v2@v$(GORELEASER_VERSION)

.PHONY: all build release-build release/build test test-property test-integration lint fmt fmt-check \
	check install warn-dirty release-snapshot snapshot clean

all: check

build:
	$(GO) build -ldflags '$(LD_FLAGS)' -o bin/af ./cmd/af

release-build: build

release/build: build

test:
	$(GO) test -race -count=1 -shuffle=on ./...

test-property: ## Scoped to packages that actually define TestProperty* (CI gate speed, issue #4)
	$(GO) test -run TestProperty -count=10000 -timeout 120s $$(grep -rl 'func TestProperty' --include='*_test.go' internal/ cmd/ 2>/dev/null | xargs -n1 dirname | sort -u | sed 's|^|./|' | sed 's|$$|/...|')

test-integration: ## Real keychain + tmux integration tests (macOS CI / owner machine)
	AF_INTEGRATION_KEYRING=1 $(GO) test -race -count=1 -tags integration -run TestIntegration ./internal/secret/ ./internal/mux/

lint:
	$(GOLANGCI) run

fmt:
	$(GOFUMPT) -w .
	$(GOIMPORTS) -w -local github.com/kakkoyun/af .

fmt-check:
	@out="$$($(GOFUMPT) -l .)"; if [ -n "$$out" ]; then printf '%s\n%s\n' "gofumpt:" "$$out"; exit 1; fi
	@out="$$($(GOIMPORTS) -l -local github.com/kakkoyun/af .)"; if [ -n "$$out" ]; then printf '%s\n%s\n' "goimports:" "$$out"; exit 1; fi

check: fmt-check lint test

install: warn-dirty release-build
	$(GO) install -ldflags '$(LD_FLAGS)' ./cmd/af

warn-dirty:
	@if git rev-parse --is-inside-work-tree >/dev/null 2>&1 && [ -n "$$(git status --porcelain)" ]; then \
		printf '%s\n' 'warning: working tree has uncommitted changes; installed af will report dirty: true' >&2; \
		printf '%s\n' 'warning: commit or stash changes before release approval if this was not intentional' >&2; \
	fi

release-snapshot:
	$(GORELEASER) release --snapshot --clean --config .goreleaser.yaml

snapshot: release-snapshot

.PHONY: snapshot-all
snapshot-all: ## Build snapshots for ALL configured targets via system goreleaser
	@command -v goreleaser >/dev/null 2>&1 || { echo "goreleaser not installed; run: brew install goreleaser"; exit 1; }
	goreleaser build --snapshot --clean --config .goreleaser.yaml

.PHONY: release-check
release-check: ## Validate .goreleaser.yaml
	@command -v goreleaser >/dev/null 2>&1 || { echo "goreleaser not installed; run: brew install goreleaser"; exit 1; }
	goreleaser check --config .goreleaser.yaml

clean:
	rm -rf bin/ dist/
