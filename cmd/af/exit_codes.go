package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime/debug"
	"strings"

	"github.com/kakkoyun/af/internal/session"
	"github.com/kakkoyun/af/internal/workstream"
)

// Exit codes follow BSD sysexits conventions plus the three universal
// codes (0, 1, 2, 130), per ADR-068 §2 / SPEC.md §15.3. The full
// symbol/meaning table lives in those documents; this file is the
// implementation of that frozen contract.
const (
	exitOK = 0
	// exitGeneral is EX_GENERAL: a generic, unclassified error.
	exitGeneral = 1
	// exitUsageCobra is EX_USAGE_COBRA: a cobra-surfaced usage error
	// (unknown command/flag, wrong arg count). Distinct from exitUsage,
	// which is af's own domain-level flag/argument validation.
	exitUsageCobra = 2
	// exitUsage is EX_USAGE: af's own argument or flag validation failure.
	exitUsage = 64
	// exitDataErr is EX_DATAERR: bad state.toml/config.toml/ledger.jsonl,
	// or a domain precondition backed by persisted data (e.g. no PR).
	exitDataErr = 65
	// exitNoInput is EX_NOINPUT: session, branch, or file not found.
	exitNoInput = 66
	// exitUnavailable is EX_UNAVAILABLE: a required external tool
	// (gh, slicer, ...) is missing.
	exitUnavailable = 69
	// exitSoftware is EX_SOFTWARE: an internal invariant was violated
	// (a bug). main's panic recovery reports this.
	exitSoftware = 70
	// exitTempFail is EX_TEMPFAIL: a retryable failure — network, or
	// (per ADR-068 §4) a lock-acquisition timeout.
	exitTempFail = 75
	// exitNoPerm is EX_NOPERM: permission denied (keyring, filesystem, SSH).
	exitNoPerm = 77
	// exitInterrupted is EX_INTERRUPTED: SIGINT received during the command.
	exitInterrupted = 130
)

// exitCodeForError maps a command error to its ADR-068 §2 exit code.
// The sentinel-keyed rows (exitCodeForSentinel) are checked first,
// most-specific first; the pattern-based rows (exitCodeForPattern) are
// the broadest matches and run last. Split into two functions to keep
// each below the cyclomatic-complexity budget.
func exitCodeForError(err error) int {
	if err == nil {
		return exitOK
	}
	if code, ok := exitCodeForSentinel(err); ok {
		return code
	}
	return exitCodeForPattern(err)
}

// exitCodeForSentinel maps errors identifiable via errors.Is against a
// known sentinel (including af's own domain usage errors). ok is false
// when no sentinel matched, so the caller falls back to
// exitCodeForPattern.
//
// os.ErrPermission covers filesystem permission denials (state dir,
// lock file, ledger). The OS keyring backend (zalando/go-keyring, see
// internal/secret/system_keyring.go) does not expose a distinguishable
// "access denied" error across platforms — macOS shells out to
// /usr/bin/security and only special-cases the "could not be found"
// message, and the Linux secret-service/dbus path only special-cases
// not-found too, so a keyring permission denial surfaces as an opaque
// wrapped error, not os.ErrPermission. Per the issue's own guidance,
// we do not invent detection by matching more than the one well-known
// "not found" message the library already recognizes, so keyring
// denials fall through to exitGeneral rather than exitNoPerm.
func exitCodeForSentinel(err error) (int, bool) {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, errSessionPickerInterrupted):
		return exitInterrupted, true
	case errors.Is(err, errSessionResolutionNoInput), errors.Is(err, errProxyNoState), errors.Is(err, errStackNoState):
		return exitNoInput, true
	case errors.Is(err, errPRRefreshNoPR), errors.Is(err, errPRAIEmptyDiff), errors.Is(err, errReviewEmptyDiff), errors.Is(err, errReviewEmptyBody):
		return exitDataErr, true
	case errors.Is(err, session.ErrLockBusy):
		return exitTempFail, true
	case errors.Is(err, exec.ErrNotFound):
		return exitUnavailable, true
	case isDomainUsageError(err):
		return exitUsage, true
	case errors.Is(err, os.ErrPermission):
		return exitNoPerm, true
	default:
		return 0, false
	}
}

// exitCodeForPattern maps errors only identifiable by their rendered
// text (cobra's own parse-time errors carry no sentinel), falling back
// to exitGeneral for anything unclassified.
func exitCodeForPattern(err error) int {
	if isCobraUsageError(err) {
		return exitUsageCobra
	}
	return exitGeneral
}

// isDomainUsageError reports af's own flag/argument validation
// sentinels, which map to EX_USAGE (64) — distinct from cobra's own
// parse-time errors (see isCobraUsageError), which map to
// EX_USAGE_COBRA (2) per ADR-068 §2.
func isDomainUsageError(err error) bool {
	return errors.Is(err, errNoteAppendRequired) ||
		errors.Is(err, errStackParentRequired) ||
		errors.Is(err, errUnsupportedShell) ||
		errors.Is(err, workstream.ErrInvalidSessionName)
}

// isCobraUsageError detects cobra's own parse-time usage errors.
// Cobra reports these as plain strings rather than sentinel errors,
// so this matches on the fixed substrings cobra itself uses for
// unknown-command, unknown-flag, and wrong-arg-count errors.
func isCobraUsageError(err error) bool {
	message := err.Error()
	// The arg-count match pins cobra's exact "accepts N arg(s)" wording:
	// a bare "accepts " would misclassify af's own domain errors (e.g.
	// "--sandbox only accepts ...") as parse-time usage.
	return strings.Contains(message, "unknown command") ||
		strings.Contains(message, "unknown flag") ||
		(strings.Contains(message, "accepts ") && strings.Contains(message, "arg(s)"))
}

// handlePanicOutput formats a recovered panic value and stack trace to
// w and returns the exit code main should use (EX_SOFTWARE). It is
// extracted from main's panic-recovery defer so the formatting and
// exit-code choice are unit-testable without forking a subprocess.
func handlePanicOutput(w io.Writer, recovered any) int {
	_, _ = fmt.Fprintf(w, "panic: %v\n%s", recovered, debug.Stack()) //nolint:errcheck // Best-effort diagnostic write right before process exit.
	return exitSoftware
}
