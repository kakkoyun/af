---
adr: 074
title: "af doctor --all — host self-smoke with actionable report"
status: proposed
implementation: complete
date: 2026-07-07
last_modified: 2026-07-07
supersedes: []
superseded_by: null
related: ["044", "051", "068"]
tags: ["go", "doctor", "smoke", "report", "v1"]
---

# ADR-074: `af doctor --all` — host self-smoke with actionable report

## Context

`af doctor` (ADR-044) probes for tools and prints install hints, but it
cannot answer "does af actually *work* on this machine?". That question
is answered today by `docs/PRE_RELEASE_SMOKE.md`, an owner-run manual
checklist. The owner wants a one-command version whose output is
actionable by an AI assistant: run it, paste the report, and get
diagnosis or an automatically filed issue — streamlining error
reporting, bug fixing, and development.

## Decision

Extend `af doctor` with a self-smoke mode:

```
af doctor --all [--report] [--report-dir DIR] [--issue]
```

- `--all` runs a suite of real `af` invocations (the binary re-executes
  itself via `os.Executable`) inside an **isolated environment**: a
  temporary root containing a scratch `HOME` (so config, state,
  sessions, and archive live under it), a scratch git repository, and an
  Obsidian vault directory. Nothing outside the temp root is touched,
  and the root is removed afterwards — the machine is left clean.
- Steps cover the local command surface (version, help, setup, config,
  create incl. traversal rejection, list/status/info, note,
  stack/sync validation, suspend/resume, done + archive, clean
  --dry-run, retro) and record for each: exact argv, environment
  deltas, exit code, duration, captured stdout/stderr, the expectation
  checked, and a PASS/FAIL/SKIP verdict. Steps whose external tool is
  missing (e.g. git) SKIP with the reason rather than failing.
- `--report` writes two artifacts to `--report-dir` (default: the
  current directory): a Markdown report designed to be pasted to an AI
  assistant (environment block, failure-first ordering, fenced
  command/output/repro sections) and a JSON sidecar with the same data
  for tooling.
- `--issue` files the failure section as a GitHub issue on
  `kakkoyun/af` via the `gh` CLI when at least one step fails.
  Environment problems (missing tools → SKIPs) are never filed — they
  are actionable locally and stay on the terminal. Without `gh`, the
  flag degrades to a terminal notice.
- Exit code: 0 when no step fails (skips allowed), 1 otherwise, per the
  ADR-068 contract.

## Consequences

- The owner smoke checklist shrinks further: stages already annotated
  as CI-automated gain an on-host equivalent, and regressions on the
  owner's real machine produce a paste-ready diagnostic instead of a
  prose bug report.
- The suite runs the same binary it ships in, so a broken build
  reports itself.
- Real-integration stages (slicer VMs, real GitHub PRs, remote hosts)
  stay in `PRE_RELEASE_SMOKE.md` stages 11–13; the self-smoke is
  local-only by design and does not touch the network.

## Alternatives considered

- **Shipping the testscript suite**: the golden scripts run against
  fake externals by design; the point here is the *real* host tools.
- **A shell script in `scripts/`**: loses the isolation guarantees,
  Windows-hostile, and cannot reuse the binary's own probes.

## References

- ADR-044 (doctor), ADR-051 (testing strategy), ADR-068 (operational
  contract), `docs/PRE_RELEASE_SMOKE.md`.
