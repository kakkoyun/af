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
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	err := run(ctx, os.Args, os.Stdout, os.Stderr)
	cancel()
	if err != nil {
		_, writeErr := fmt.Fprintln(os.Stderr, err)
		if writeErr != nil {
			os.Exit(1)
		}
		os.Exit(1)
	}
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
