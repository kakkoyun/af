# Pre-release smoke test — v1.0.0

Run this before approving `v1.0.0`. The required path uses a temporary
`$HOME`, a temporary git repository, and the release-candidate `af`
installed on your normal `PATH`. It should not touch your real af state.

> **Release gate:** do not tag `v1.0.0` and do not run
> `goreleaser release --clean` until the owner reports every required
> stage as pass or explicitly waives a discrepancy.

## How to run and report

Run **one stage at a time**. After each stage, stop and report one of:

- `Stage N PASS` plus any output that surprised you; or
- `Stage N FAIL` plus the exact failing command, exit code, stdout, and
  stderr; or
- `Stage N DISCREPANCY` if the command passed but the behaviour differs
  from your expectation.

If a required stage fails or feels wrong, stop. The maintainer will either
fix the implementation/docs or create/amend an ADR if the expected
behaviour needs a design decision.

Required stages: **0–10**. Optional real-integration stages: **11–13**.

## Stage 0 — Build, verify, and install the candidate

Run from the repository root on `main`.

```bash
set -euo pipefail

git branch --show-current | grep '^main$'
git status --short

make check
goreleaser check --config .goreleaser.yaml
goreleaser release --snapshot --clean --config .goreleaser.yaml

# Direct system/user install. This overwrites the `af` binary in Go's install bin.
make install

GO_BIN="$(go env GOBIN)"
if [ -z "$GO_BIN" ]; then
  GO_BIN="$(go env GOPATH)/bin"
fi
export PATH="$GO_BIN:$PATH"
hash -r

AF="$GO_BIN/af"
test -x "$AF"
"$AF" version

AF_SMOKE_ENV="${AF_SMOKE_ENV:-/tmp/af-v1-smoke.env}"
cat > "$AF_SMOKE_ENV" <<SMOKE_ENV
export AF="$AF"
export AF_SMOKE_ENV="$AF_SMOKE_ENV"
export AF_INSTALL_BIN="$GO_BIN"
SMOKE_ENV

printf 'Smoke env saved to %s\n' "$AF_SMOKE_ENV"
```

Expected:

- `make check`, `goreleaser check`, and snapshot build all pass.
- `af version` prints the candidate version/build metadata.
- `git status --short` is empty. If it is not empty, report it.

Optional hard install into a system directory, if you specifically want
`/opt/homebrew/bin/af` or `/usr/local/bin/af` to be the tested binary:

```bash
# Pick the directory that is actually on your PATH.
sudo install -m 0755 "$AF" /opt/homebrew/bin/af
hash -r
af version
```

## Stage 1 — Create the isolated smoke environment

```bash
set -euo pipefail
source "${AF_SMOKE_ENV:-/tmp/af-v1-smoke.env}"

AF_SMOKE_ROOT="$(mktemp -d)"
AF_SMOKE_BIN="$AF_SMOKE_ROOT/bin"
AF_SMOKE_REPO="$AF_SMOKE_ROOT/repo"
export HOME="$AF_SMOKE_ROOT/home"
export PATH="$AF_SMOKE_BIN:$AF_INSTALL_BIN:$(dirname "$AF"):$PATH"

mkdir -p "$AF_SMOKE_BIN" "$HOME" "$AF_SMOKE_REPO"
cd "$AF_SMOKE_REPO"

git init -b main
git config user.email smoke@example.invalid
git config user.name 'AF Smoke Test'
git remote add origin https://github.com/kakkoyun/af-smoke.git
printf 'smoke base\n' > README.md
git add README.md
git commit -m 'initial smoke commit'

cat >> "$AF_SMOKE_ENV" <<SMOKE_ENV
export AF_SMOKE_ROOT="$AF_SMOKE_ROOT"
export AF_SMOKE_BIN="$AF_SMOKE_BIN"
export AF_SMOKE_REPO="$AF_SMOKE_REPO"
export HOME="$HOME"
export PATH="$AF_SMOKE_BIN:$AF_INSTALL_BIN:$(dirname "$AF"):\$PATH"
SMOKE_ENV

printf 'Smoke root: %s\n' "$AF_SMOKE_ROOT"
printf 'Smoke repo: %s\n' "$AF_SMOKE_REPO"
printf 'Smoke HOME: %s\n' "$HOME"
```

Expected:

- A new temp repo exists on branch `main` with one commit.
- The printed `$HOME` is under the temp smoke root, not your real home.

## Stage 2 — Setup, config, doctor, and completions

```bash
set -euo pipefail
source "${AF_SMOKE_ENV:-/tmp/af-v1-smoke.env}"
cd "$AF_SMOKE_REPO"

"$AF" setup --skip-completions --skip-gitignore
mkdir -p "$HOME/Vaults/personal"
TRUE_BIN="$(command -v true)"

cat > "$HOME/.config/af/config.toml" <<SMOKE_CONFIG
schema_version = 1

[general]
default_agent = "pi"
worktree_root = "$HOME/Workspace/.worktrees"

[editor]
terminal = "$TRUE_BIN"
visual = "$TRUE_BIN"

[pr]
shell = false
cmd = ["echo", "smoke-pr", "{base}", "{head}"]
ai_model = ""

[obsidian]
notes_vault = "personal"
notes_folder = "00 - af"

[obsidian.vaults]
personal = "$HOME/Vaults/personal"

[secret]
keyring_service = "af-smoke-v1"
SMOKE_CONFIG

ALT_CONFIG="$AF_SMOKE_ROOT/alt-config.toml"
"$AF" --config "$ALT_CONFIG" config init
test -s "$ALT_CONFIG"
set +e
"$AF" --config "$ALT_CONFIG" config init >"$AF_SMOKE_ROOT/config-init-overwrite.out" 2>"$AF_SMOKE_ROOT/config-init-overwrite.err"
config_init_code=$?
set -e
test "$config_init_code" -ne 0

"$AF" config show | tee "$AF_SMOKE_ROOT/config-show.txt"
grep '^schema_version = 1' "$AF_SMOKE_ROOT/config-show.txt"
grep 'worktree_root = ' "$AF_SMOKE_ROOT/config-show.txt"
grep 'personal = ' "$AF_SMOKE_ROOT/config-show.txt"

"$AF" doctor | tee "$AF_SMOKE_ROOT/doctor.txt"
grep -E '✓ tmux +\(.+version ' "$AF_SMOKE_ROOT/doctor.txt"
grep -E '✓ pi +\(.+version ' "$AF_SMOKE_ROOT/doctor.txt"
if grep -q '✓ slicer' "$AF_SMOKE_ROOT/doctor.txt"; then
  grep -E '✓ slicer +\(.+version ' "$AF_SMOKE_ROOT/doctor.txt"
fi
grep '✓ obsidian:personal' "$AF_SMOKE_ROOT/doctor.txt"

for shell in bash zsh fish powershell; do
  "$AF" completions "$shell" > "$AF_SMOKE_ROOT/completions-$shell.txt"
  test -s "$AF_SMOKE_ROOT/completions-$shell.txt"
done
```

Expected:

- `config init` writes an alternate config and refuses to overwrite it.
- `config show` includes schema version, temp worktree root, and the temp
  Obsidian vault path.
- `doctor` reports versions for `tmux` and `pi`; reports a `slicer`
  version when slicer is installed; and reports `✓ obsidian:personal`.
- Completion files are non-empty.

## Stage 3 — Command surface and help for every command

```bash
set -euo pipefail
source "${AF_SMOKE_ENV:-/tmp/af-v1-smoke.env}"
cd "$AF_SMOKE_REPO"

"$AF" --help | grep 'Available Commands'
"$AF" help doctor | grep 'Probe'
"$AF" version

for cmd in \
  agent auth clean completions config control create diff doctor done \
  editor info list note pr pull resume retro review session-branch \
  session-data setup stack status suspend sync unstack version; do
  "$AF" "$cmd" --help > "$AF_SMOKE_ROOT/help-$cmd.txt"
  test -s "$AF_SMOKE_ROOT/help-$cmd.txt"
done

for subcmd in \
  'agent list' 'agent add' 'agent stop' \
  'auth set' 'auth get' 'auth status' 'auth clear' 'auth list' \
  'config init' 'config show' \
  'control up' 'control down' 'control status' \
  'session-data sync' 'session-data list'; do
  # shellcheck disable=SC2086 # intentional command-word splitting for subcommand pairs
  "$AF" $subcmd --help > "$AF_SMOKE_ROOT/help-${subcmd// /-}.txt"
  test -s "$AF_SMOKE_ROOT/help-${subcmd// /-}.txt"
done
```

Expected:

- Every top-level command and every nested command listed above has
  non-empty help output.

## Stage 4 — Auth command full circle

This stage stores a dummy value in the real OS keyring service
`af-smoke-v1`, then clears it. Do not use a real secret.

```bash
set -euo pipefail
source "${AF_SMOKE_ENV:-/tmp/af-v1-smoke.env}"
cd "$AF_SMOKE_REPO"

SMOKE_KEY="smoke_token_v1"
printf 'not-a-real-secret-smoke-value\n' | "$AF" auth set "$SMOKE_KEY"
"$AF" auth list | tee "$AF_SMOKE_ROOT/auth-list.txt"
grep "$SMOKE_KEY" "$AF_SMOKE_ROOT/auth-list.txt"

"$AF" auth status | tee "$AF_SMOKE_ROOT/auth-status.txt"
grep "$SMOKE_KEY" "$AF_SMOKE_ROOT/auth-status.txt"

"$AF" auth get "$SMOKE_KEY" | tee "$AF_SMOKE_ROOT/auth-get.txt"
grep '\[REDACTED:' "$AF_SMOKE_ROOT/auth-get.txt"

"$AF" auth clear "$SMOKE_KEY"
set +e
"$AF" auth get "$SMOKE_KEY" >"$AF_SMOKE_ROOT/auth-get-after-clear.out" 2>"$AF_SMOKE_ROOT/auth-get-after-clear.err"
code=$?
set -e
test "$code" -ne 0
```

Expected:

- The dummy key appears in `auth list` and `auth status`.
- Non-TTY `auth get` is redacted.
- `auth clear` removes the key; a later `auth get` fails.

## Stage 5 — Local workstream lifecycle and proxy commands

```bash
set -euo pipefail
source "${AF_SMOKE_ENV:-/tmp/af-v1-smoke.env}"
cd "$AF_SMOKE_REPO"

"$AF" create smoke-one --from main --bare
"$AF" list | tee "$AF_SMOKE_ROOT/list-1.txt"
grep smoke-one "$AF_SMOKE_ROOT/list-1.txt"

"$AF" status | tee "$AF_SMOKE_ROOT/status-1.txt"
grep smoke-one "$AF_SMOKE_ROOT/status-1.txt"
"$AF" status --json | jq -e '.schema == 1 and (.data | type == "array")'
"$AF" status --filter active | grep smoke-one

"$AF" info smoke-one | tee "$AF_SMOKE_ROOT/info-smoke-one.txt"
grep 'Session:   smoke-one' "$AF_SMOKE_ROOT/info-smoke-one.txt"
"$AF" info --json smoke-one | jq -e '.schema == 1 and .data.session.Name == "smoke-one"'

"$AF" note smoke-one --append 'manual smoke note'
"$AF" editor smoke-one

SMOKE_WT="$HOME/Workspace/.worktrees/github.com/af-smoke/smoke-one"
cat >> "$AF_SMOKE_ENV" <<SMOKE_ENV
export SMOKE_WT="$SMOKE_WT"
SMOKE_ENV

cd "$SMOKE_WT"
printf 'smoke worktree change\n' >> README.md
git add README.md
git commit -m 'smoke worktree change'

cd "$AF_SMOKE_REPO"
"$AF" diff smoke-one | tee "$AF_SMOKE_ROOT/diff-smoke-one.txt"
grep 'README.md' "$AF_SMOKE_ROOT/diff-smoke-one.txt"

"$AF" pr smoke-one --title 'Smoke PR' --body 'Smoke body' | tee "$AF_SMOKE_ROOT/pr-smoke-one.txt"
grep 'smoke-pr' "$AF_SMOKE_ROOT/pr-smoke-one.txt"

set +e
"$AF" pr --refresh smoke-one >"$AF_SMOKE_ROOT/pr-refresh.out" 2>"$AF_SMOKE_ROOT/pr-refresh.err"
code=$?
set -e
test "$code" -eq 65
```

Expected:

- `create`, `list`, `status`, `info`, `note`, `editor`, `diff`, and
  safe `pr` proxy all complete.
- JSON output uses `{schema: 1, data: ...}` envelopes.
- `pr --refresh` without a PR exits `65`.

## Stage 6 — Session discovery, selection, completions, suspend/resume

```bash
set -euo pipefail
source "${AF_SMOKE_ENV:-/tmp/af-v1-smoke.env}"
cd "$AF_SMOKE_REPO"

"$AF" create smoke-two --from main --bare

"$AF" --session smoke-two info smoke-one 2>"$AF_SMOKE_ROOT/session-warning.txt" | tee "$AF_SMOKE_ROOT/session-override.txt"
grep 'Session:   smoke-two' "$AF_SMOKE_ROOT/session-override.txt"
grep 'overrides positional session' "$AF_SMOKE_ROOT/session-warning.txt"

"$AF" __complete --session '' 2>/dev/null | tee "$AF_SMOKE_ROOT/complete-session.txt"
grep smoke-one "$AF_SMOKE_ROOT/complete-session.txt"
grep smoke-two "$AF_SMOKE_ROOT/complete-session.txt"

"$AF" __complete status --filter '' 2>/dev/null | tee "$AF_SMOKE_ROOT/complete-status-filter.txt"
grep -E 'active|suspended|completed|abandoned' "$AF_SMOKE_ROOT/complete-status-filter.txt"

cd "$SMOKE_WT"
"$AF" info | grep 'Session:   smoke-one'
AF_SESSION=smoke-one "$AF" info | grep 'Session:   smoke-one'

cd "$AF_SMOKE_REPO"
"$AF" suspend smoke-one
"$AF" status --filter suspended | grep smoke-one
"$AF" resume smoke-one --bare
"$AF" status --filter active | grep smoke-one
```

Expected:

- Root `--session` wins over positional session and warns on stderr.
- Shell completion sources include workstream names and status filters.
- Cwd discovery and `AF_SESSION` selection work from inside the worktree.
- `suspend` and `resume --bare` round-trip `smoke-one`.

## Stage 7 — Agent slots, stack metadata, and sync

```bash
set -euo pipefail
source "${AF_SMOKE_ENV:-/tmp/af-v1-smoke.env}"
cd "$AF_SMOKE_REPO"

"$AF" agent --session smoke-one list | tee "$AF_SMOKE_ROOT/agent-list-1.txt"
grep primary "$AF_SMOKE_ROOT/agent-list-1.txt"

"$AF" agent --session smoke-one add --slot reviewer --agent codex
"$AF" agent --session smoke-one list | tee "$AF_SMOKE_ROOT/agent-list-2.txt"
grep reviewer "$AF_SMOKE_ROOT/agent-list-2.txt"

"$AF" agent --session smoke-one stop reviewer --remove-worktree
"$AF" agent --session smoke-one list | tee "$AF_SMOKE_ROOT/agent-list-3.txt"
grep stopped "$AF_SMOKE_ROOT/agent-list-3.txt"

"$AF" stack smoke-two --parent smoke-one
grep 'parent_session = "smoke-one"' "$HOME/.local/share/af/v1/sessions/smoke-two/state.toml"
"$AF" sync smoke-two
"$AF" unstack smoke-two
grep 'parent_session = ""' "$HOME/.local/share/af/v1/sessions/smoke-two/state.toml"
```

Expected:

- Agent slot add/list/stop works without launching a real agent because
  the workstreams were created with `--bare`.
- Stack metadata writes, `sync` runs, and `unstack` clears the parent.

## Stage 8 — Expected non-slicer and control-status behaviour

```bash
set -euo pipefail
source "${AF_SMOKE_ENV:-/tmp/af-v1-smoke.env}"
cd "$AF_SMOKE_REPO"

set +e
"$AF" pull smoke-one >"$AF_SMOKE_ROOT/pull-nonslicer.out" 2>"$AF_SMOKE_ROOT/pull-nonslicer.err"
pull_code=$?
"$AF" session-data list smoke-one >"$AF_SMOKE_ROOT/session-data-list-nonslicer.out" 2>"$AF_SMOKE_ROOT/session-data-list-nonslicer.err"
session_data_list_code=$?
"$AF" session-data sync smoke-one --dry-run >"$AF_SMOKE_ROOT/session-data-sync-nonslicer.out" 2>"$AF_SMOKE_ROOT/session-data-sync-nonslicer.err"
session_data_sync_code=$?
set -e
test "$pull_code" -ne 0
test "$session_data_list_code" -ne 0
test "$session_data_sync_code" -ne 0
grep -E 'slicer|lease|not slicer-backed' "$AF_SMOKE_ROOT/pull-nonslicer.err" "$AF_SMOKE_ROOT/session-data-list-nonslicer.err" "$AF_SMOKE_ROOT/session-data-sync-nonslicer.err"

"$AF" control status | tee "$AF_SMOKE_ROOT/control-status.txt"
grep -E 'remote control is not running|Remote control active' "$AF_SMOKE_ROOT/control-status.txt"
```

Expected:

- `pull`, `session-data list`, and `session-data sync --dry-run` fail
  clearly for a non-slicer workstream.
- `control status` is safe and reports either not running or an active
  endpoint.

## Stage 9 — Hermetic review and remote-control fakes

This stage shadows `gh`, `pi`, `superterm`, and `tailscale` only inside
`$AF_SMOKE_BIN`, which is already first on `PATH` for the smoke shell.
It exercises `review`, `control up`, and `control down` without touching
GitHub, an LLM, superterm, or Tailscale.

```bash
set -euo pipefail
source "${AF_SMOKE_ENV:-/tmp/af-v1-smoke.env}"
cd "$AF_SMOKE_REPO"

cat > "$AF_SMOKE_BIN/gh" <<'SH'
#!/bin/sh
if [ "$1" = "--version" ] || [ "$1" = "version" ]; then
  echo "gh version smoke"
  exit 0
fi
if [ "$1" = "pr" ] && [ "$2" = "view" ]; then
  case "$*" in
    *state,isDraft,mergedAt,closedAt*)
      echo '{"state":"OPEN","isDraft":false,"mergedAt":null,"closedAt":null}'
      ;;
    *)
      echo '{"number":123,"title":"Smoke PR","headRefName":"af/smoke-one","baseRefName":"main"}'
      ;;
  esac
  exit 0
fi
if [ "$1" = "pr" ] && [ "$2" = "diff" ]; then
  cat <<'DIFF'
diff --git a/README.md b/README.md
--- a/README.md
+++ b/README.md
@@ -1 +1,2 @@
 smoke base
+smoke review diff
DIFF
  exit 0
fi
echo "unexpected fake gh: $*" >&2
exit 1
SH
chmod +x "$AF_SMOKE_BIN/gh"

cat > "$AF_SMOKE_BIN/pi" <<'SH'
#!/bin/sh
if [ "$1" = "--version" ]; then
  echo "0.78.0-smoke"
  exit 0
fi
if [ "$1" = "--print" ]; then
  cat >/dev/null
  echo "Smoke review body"
  exit 0
fi
echo "fake pi invoked: $*" >&2
exit 0
SH
chmod +x "$AF_SMOKE_BIN/pi"

cat > "$AF_SMOKE_BIN/superterm" <<'SH'
#!/bin/sh
case "$1" in
  --version) echo "superterm smoke" ;;
  up) echo "superterm listening at http://localhost:7681" ;;
  status) echo "superterm active at http://localhost:7681" ;;
  down) echo "superterm stopped" ;;
  *) echo "unexpected fake superterm: $*" >&2; exit 1 ;;
esac
SH
chmod +x "$AF_SMOKE_BIN/superterm"

cat > "$AF_SMOKE_BIN/tailscale" <<'SH'
#!/bin/sh
if [ "$1" = "--version" ]; then
  echo "tailscale smoke"
  exit 0
fi
if [ "$1" = "serve" ] && [ "$2" = "--bg" ]; then
  echo "Available on the internet: https://af-smoke.ts.net/"
  exit 0
fi
if [ "$1" = "serve" ] && [ "$2" = "status" ]; then
  echo "https://af-smoke.ts.net/ -> http://localhost:7681"
  exit 0
fi
if [ "$1" = "serve" ] && [ "$2" = "off" ]; then
  echo "serve off"
  exit 0
fi
echo "unexpected fake tailscale: $*" >&2
exit 1
SH
chmod +x "$AF_SMOKE_BIN/tailscale"

"$AF" review smoke-one --stdout | tee "$AF_SMOKE_ROOT/review-stdout.md"
grep 'Smoke review body' "$AF_SMOKE_ROOT/review-stdout.md"

"$AF" review smoke-one | tee "$AF_SMOKE_ROOT/review-write.txt"
grep 'review: wrote' "$AF_SMOKE_ROOT/review-write.txt"
ls -1 "$SMOKE_WT/.af/reviews/" | grep 'pr123'

"$AF" control status | tee "$AF_SMOKE_ROOT/control-status-fake.txt"
grep 'Remote control active' "$AF_SMOKE_ROOT/control-status-fake.txt"
"$AF" control up --json | jq -e '.local_url == "http://localhost:7681" and .tailnet_url == "https://af-smoke.ts.net/"'
"$AF" control down | grep 'remote control stopped'
```

Expected:

- `review --stdout` prints the fake review body.
- `review` writes a report under `.af/reviews/` and does not post.
- `control status`, `control up --json`, and `control down` complete
  through fakes.

## Stage 10 — Session-branch, cleanup, clean, and retro

```bash
set -euo pipefail
source "${AF_SMOKE_ENV:-/tmp/af-v1-smoke.env}"
cd "$AF_SMOKE_REPO"

SESSION_BRANCH_OUTPUT="$("$AF" session-branch)"
printf '%s\n' "$SESSION_BRANCH_OUTPUT" | tee "$AF_SMOKE_ROOT/session-branch.txt"
SESSION_BRANCH_NAME="$(printf '%s\n' "$SESSION_BRANCH_OUTPUT" | awk '{print $3}')"
test -n "$SESSION_BRANCH_NAME"
"$AF" status --all | grep "$SESSION_BRANCH_NAME"
git checkout main

# Do not run `af done` for session-branch: it points at the checkout root.
# The whole smoke repo is temporary, so remove only its state directory.
rm -rf "$HOME/.local/share/af/v1/sessions/$SESSION_BRANCH_NAME"

"$AF" done smoke-two --force
"$AF" done smoke-one --force

test -d "$HOME/.local/share/af/v1/archive/smoke-one"
test -d "$HOME/.local/share/af/v1/archive/smoke-two"

"$AF" clean --dry-run --include-abandoned | tee "$AF_SMOKE_ROOT/clean-dry-run.txt"

# Seed a deterministic archived note so `retro` is covered even if the
# lifecycle did not move an Obsidian note into the archive directory.
cat > "$HOME/.local/share/af/v1/archive/smoke-one/note.md" <<'NOTE'
---
af_schema: 1
af_started_at: 2026-01-01T00:00:00Z
af_session: smoke-one
af_repo: github.com/af-smoke
af_branch: smoke-one
af_base_branch: main
af_status: completed
af_agents:
  - slot: primary
    provider: pi
    status: stopped
tags:
  - af-smoke
af_tags:
  - smoke
---
smoke-retro note body
NOTE

"$AF" retro --search smoke-retro --limit 5 | tee "$AF_SMOKE_ROOT/retro.txt"
grep 'smoke-one' "$AF_SMOKE_ROOT/retro.txt"

"$AF" list | tee "$AF_SMOKE_ROOT/list-final.txt"
"$AF" status --all | tee "$AF_SMOKE_ROOT/status-final.txt"
```

Expected:

- `session-branch` creates a state entry and branch in the disposable
  repo; the stage removes only that state entry.
- `done --force` archives both workstreams.
- `clean --dry-run` completes.
- `retro` finds the deterministic archived note.
- Final list/status should not show active `smoke-one` or `smoke-two`.

## Stage 11 — Optional real GitHub PR/review path

Run only from a real af workstream that has an open PR and where `gh auth
status` is healthy.

```bash
set -euo pipefail

SESSION="<real-session>"
WORKTREE="<real-worktree-path>"

af pr --refresh "$SESSION"
af review "$SESSION" --stdout | head -40
af review "$SESSION"
ls -1 "$WORKTREE/.af/reviews/"
```

Expected:

- `af pr --refresh` reports the current PR state.
- `af review --stdout` prints a markdown report.
- `af review` writes a report under `.af/reviews/` and never posts to
  GitHub.

Report this stage as `PASS`, `FAIL`, or `SKIPPED`.

## Stage 12 — Optional real slicer-backed path

Run only if you want to validate the heavy slicer path before release.
This may launch a VM.

```bash
set -euo pipefail
source "${AF_SMOKE_ENV:-/tmp/af-v1-smoke.env}"
cd "$AF_SMOKE_REPO"

af create smoke-slicer --from main --sandbox slicer
af status | grep smoke-slicer
af info smoke-slicer | grep 'Slicer worktree'
af session-data list smoke-slicer --agent pi || true
af session-data sync smoke-slicer --dry-run || true
af pull smoke-slicer
af done smoke-slicer --force
```

Expected:

- `create` pushes via `slicer wt push` and records VM lease metadata.
- `status`/`info` show VM lease state.
- `pull` releases the lease.
- `done --force` archives the workstream.

Report this stage as `PASS`, `FAIL`, or `SKIPPED`.

## Stage 13 — Optional real remote doctor/control path

Run only if you have an SSH host configured for af remote checks.

```bash
set -euo pipefail

HOST="<ssh-host>"
af doctor --remote "$HOST"
af control --remote "$HOST" status
```

Expected:

- `doctor --remote` probes the remote tool surface over SSH.
- `control --remote status` is safe and reports active/not-running.

Report this stage as `PASS`, `FAIL`, or `SKIPPED`.

## Command coverage matrix

| Command | Required stage | Coverage type |
| --- | ---: | --- |
| `help` | 3 | direct |
| `version` | 0, 3 | direct |
| `setup` | 2 | direct |
| `config init/show` | 2, 3 | direct + help |
| `doctor` | 2 | direct |
| `completions` | 2 | direct for bash/zsh/fish/powershell |
| `auth set/get/status/list/clear` | 4 | direct full circle |
| `create/list/status/info/note/editor/diff/pr` | 5 | direct local workstream |
| `suspend/resume` | 6 | direct round trip |
| `agent list/add/stop` | 7 | direct full circle |
| `stack/sync/unstack` | 7 | direct full circle |
| `pull` | 8, 12 | expected non-slicer failure; optional slicer success |
| `session-data list/sync` | 8, 12 | expected non-slicer failure for both; optional slicer success |
| `control status/up/down` | 8, 9, 13 | safe status + fake full circle; optional remote |
| `review` | 9, 11 | fake full circle; optional real PR |
| `session-branch` | 10 | direct in disposable repo |
| `done/clean/retro` | 10 | direct cleanup + deterministic retro note |

## Final report template

Please report:

- Required stages 0–10: pass/fail/discrepancy for each stage.
- Optional GitHub review path: pass/fail/skipped.
- Optional slicer path: pass/fail/skipped.
- Optional remote path: pass/fail/skipped.
- Any command output that surprised you, even if the stage passed.

The release must wait for this report.
