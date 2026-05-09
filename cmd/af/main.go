// Package main contains the af command entry point.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

var (
	errScaffoldOnly = errors.New("af v1 command tree is not implemented")
	errNilOutput    = errors.New("output writer is nil")
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := run(ctx, os.Args, os.Stdout, os.Stderr); err != nil {
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("run af scaffold: %w", err)
	}

	if stdout == nil || stderr == nil {
		return errNilOutput
	}

	program := "af"
	if len(args) > 0 && args[0] != "" {
		program = filepath.Base(args[0])
	}

	if _, err := fmt.Fprintf(stderr, "%s: Go scaffold is ready; command tree lands in TODO.md I0.2\n", program); err != nil {
		return fmt.Errorf("write scaffold diagnostic: %w", err)
	}

	return errScaffoldOnly
}
