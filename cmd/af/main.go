// Package main contains the af command entry point.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

var errNilOutput = errors.New("output writer is nil")

func main() {
	// An internal invariant violation (a bug) should never crash with a
	// bare Go panic trace and the runtime's default exit status (2,
	// which ADR-068 §2 reserves for cobra usage errors). Catch it,
	// print it, and exit EX_SOFTWARE (70) deliberately instead.
	defer func() {
		r := recover()
		if r != nil {
			os.Exit(handlePanicOutput(os.Stderr, r))
		}
	}()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	err := run(ctx, os.Args, os.Stdout, os.Stderr)
	cancel()
	exitOnError(err)
}

// exitOnError prints a non-nil run() error to stderr and exits with
// its mapped exit code. It is a separate function (rather than inline
// in main) so its os.Exit calls don't trip the exitAfterDefer check
// against main's panic-recovery defer above: neither path here is a
// panic, so skipping that defer is intentional and safe.
func exitOnError(err error) {
	if err == nil {
		return
	}
	_, writeErr := fmt.Fprintln(os.Stderr, err)
	if writeErr != nil {
		os.Exit(1)
	}
	os.Exit(exitCodeForError(err))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if stdout == nil || stderr == nil {
		return errNilOutput
	}

	cmd := newRootCmd() //nolint:contextcheck // cobra threads context via ExecuteContext, not constructor parameters.
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(commandArgs(args))

	err := cmd.ExecuteContext(ctx)
	if err != nil {
		return fmt.Errorf("execute af: %w", err)
	}

	return nil
}

func commandArgs(args []string) []string {
	if len(args) == 0 {
		return nil
	}
	return args[1:]
}
