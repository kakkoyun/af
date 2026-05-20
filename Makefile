# shellcheck disable=SC2283,SC2276,SC2157 # Make syntax, not shell
GOLANGCI_LINT_VERSION = 2.3.0
GOFUMPT_VERSION       = 0.7.0
GOIMPORTS_VERSION     = 0.38.0
GORELEASER_VERSION    = 2.5.0

GO         ?= go
GOLANGCI   ?= $(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v$(GOLANGCI_LINT_VERSION)
GOFUMPT    ?= $(GO) run mvdan.cc/gofumpt@v$(GOFUMPT_VERSION)
GOIMPORTS  ?= $(GO) run golang.org/x/tools/cmd/goimports@v$(GOIMPORTS_VERSION)
GORELEASER ?= $(GO) run github.com/goreleaser/goreleaser/v2@v$(GORELEASER_VERSION)

.PHONY: all build test test-property lint fmt fmt-check check install \
	release-snapshot snapshot clean

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
	$(GOIMPORTS) -w -local github.com/kakkoyun/af .

fmt-check:
	@out="$$($(GOFUMPT) -l .)"; if [ -n "$$out" ]; then printf '%s\n%s\n' "gofumpt:" "$$out"; exit 1; fi
	@out="$$($(GOIMPORTS) -l -local github.com/kakkoyun/af .)"; if [ -n "$$out" ]; then printf '%s\n%s\n' "goimports:" "$$out"; exit 1; fi

check: fmt-check lint test

install:
	$(GO) install ./cmd/af

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
