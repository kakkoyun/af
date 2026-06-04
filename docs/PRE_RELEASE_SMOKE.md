# Pre-release smoke test — v1.0.0

Run this before approving `v1.0.0`. The required path is isolated: it
uses a temporary `$HOME`, a temporary git repo, and a locally-built `af`
binary. It should not touch your real af state.

> **Release gate:** do not cut the release until the owner reports this
> smoke test result.

## 0. Build the candidate binary

Run from the repository root on `main`:

```bash
set -euo pipefail

make check
goreleaser check
goreleaser release --snapshot --clean

go build -o /tmp/af-smoke-bin ./cmd/af
/tmp/af-smoke-bin version
```

## 1. Create an isolated smoke environment

```bash
set -euo pipefail

SMOKE_ROOT="$(mktemp -d)"
export HOME="$SMOKE_ROOT/home"
export PATH="$(dirname /tmp/af-smoke-bin):$PATH"
AF=/tmp/af-smoke-bin

mkdir -p "$HOME" "$SMOKE_ROOT/repo"
cd "$SMOKE_ROOT/repo"

git init -b main
git config user.email smoke@example.invalid
git config user.name 'AF Smoke Test'
git remote add origin https://github.com/kakkoyun/af-smoke.git
printf 'smoke base\n' > README.md
git add README.md
git commit -m 'initial smoke commit'

$AF setup --skip-completions --skip-gitignore
$AF config show | grep -E '^schema_version = 1|worktree_root'
$AF doctor
```

## 2. Exercise local lifecycle and state discovery

```bash
set -euo pipefail

$AF create smoke-one --from main --bare
$AF list | grep smoke-one
$AF status | grep smoke-one
$AF status --json | jq -e '.schema == 1 and (.data | type == "array")'
$AF info smoke-one | grep 'Session:   smoke-one'
$AF info --json smoke-one | jq -e '.schema == 1 and .data.session.Name == "smoke-one"'

$AF note smoke-one --append 'manual smoke note'
$AF suspend smoke-one
$AF resume smoke-one --bare

# ADR-070: cwd discovery through the worktree .af/state.toml symlink.
SMOKE_WT="$HOME/Workspace/.worktrees/github.com/af-smoke/smoke-one"
cd "$SMOKE_WT"
$AF info | grep 'Session:   smoke-one'
AF_SESSION=smoke-one $AF info | grep 'Session:   smoke-one'
```

## 3. Exercise session selection, completions, and exit codes

```bash
set -euo pipefail

cd "$SMOKE_ROOT/repo"
$AF create smoke-two --from main --bare

# Root --session should override the positional arg and warn on stderr.
$AF --session smoke-two info smoke-one 2>"$SMOKE_ROOT/session-warning.txt" | grep 'Session:   smoke-two'
grep 'overrides positional session' "$SMOKE_ROOT/session-warning.txt"

# Completion sources should include workstream names.
$AF __complete --session '' 2>/dev/null | grep -E 'smoke-one|smoke-two'
$AF __complete status --filter '' 2>/dev/null | grep -E 'active|suspended|completed|abandoned'

# EX_DATAERR path: --refresh without a PR should exit 65.
set +e
$AF pr --refresh smoke-one >/tmp/af-smoke-pr-refresh.out 2>/tmp/af-smoke-pr-refresh.err
code=$?
set -e
test "$code" -eq 65
```

## 4. Exercise stack metadata and cleanup

```bash
set -euo pipefail

cd "$SMOKE_ROOT/repo"
$AF stack smoke-two --parent smoke-one
grep 'parent_session = "smoke-one"' "$HOME/.local/share/af/v1/sessions/smoke-two/state.toml"
$AF unstack smoke-two
grep 'parent_session = ""' "$HOME/.local/share/af/v1/sessions/smoke-two/state.toml"

$AF done smoke-two --force
$AF done smoke-one --force

test -d "$HOME/.local/share/af/v1/archive/smoke-one"
test -d "$HOME/.local/share/af/v1/archive/smoke-two"
```

## 5. Optional real-integration checks

Run only when the relevant external tool/service is available.

### GitHub PR-backed review path

From a real af workstream that has an open PR:

```bash
af pr --refresh <session>
af review <session> --stdout | head -40
af review <session>
ls -1 <worktree>/.af/reviews/
```

Expected: `af pr --refresh` reports the current PR state; `af review
--stdout` prints a markdown report; `af review` writes a report under
`.af/reviews/` and never posts to GitHub.

### Slicer-backed path

Only if you want to validate the heavy slicer path before release:

```bash
af create smoke-slicer --from main --sandbox slicer
af status | grep smoke-slicer
af info smoke-slicer | grep 'Slicer worktree'
af pull smoke-slicer
af done smoke-slicer --force
```

Expected: create pushes via `slicer wt push`; status/info show VM lease
metadata; pull releases the lease.

## 6. Report back

Please report:

- required smoke: pass/fail;
- optional GitHub review path: pass/fail/skipped;
- optional slicer path: pass/fail/skipped;
- any command output that surprised you.

The release must wait for this report.
