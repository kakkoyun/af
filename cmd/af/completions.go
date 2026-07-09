package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/kakkoyun/af/internal/session"
)

var (
	// errUnsupportedShell reports that the requested (or auto-detected)
	// shell is not one this command knows how to generate a script for.
	errUnsupportedShell = errors.New("unsupported completion shell")
	// errCannotDetectShell reports that --install was passed without a
	// positional shell argument and $SHELL's basename did not match a
	// supported shell (or $SHELL was empty). It is a sibling of
	// errUnsupportedShell in exit_codes.go's isDomainUsageError.
	errCannotDetectShell = errors.New("cannot detect shell from $SHELL; pass one explicitly: af completions zsh --install")
	// errDryRunRequiresInstall reports that --dry-run was passed without
	// --install; dry-run only makes sense as a preview of an install.
	errDryRunRequiresInstall = errors.New("--dry-run only valid together with --install")
)

const (
	shellBash       = "bash"
	shellZsh        = "zsh"
	shellFish       = "fish"
	shellPowerShell = "powershell"

	// completionInstallDirPerm is the mode for any parent directory
	// this command creates on the way to an install destination.
	completionInstallDirPerm = 0o750
	// completionInstallFilePerm is the mode for an installed completion
	// script: world-readable like any other dotfile, not sensitive.
	completionInstallFilePerm = 0o644
)

// supportedCompletionShells lists every shell this command can emit a
// completion script for (stdout mode).
func supportedCompletionShells() []string {
	return []string{shellBash, shellZsh, shellFish, shellPowerShell}
}

// installableCompletionShells lists the shells --install knows a
// standard user-local destination path for. PowerShell is excluded: it
// has no such per-user convention this command targets.
func installableCompletionShells() []string {
	return []string{shellBash, shellZsh, shellFish}
}

// completionsOptions holds the flags for `af completions`.
type completionsOptions struct {
	install bool
	dryRun  bool
}

func newCompletionsCmd() *cobra.Command {
	opts := &completionsOptions{}
	cmd := &cobra.Command{
		Use:   "completions [bash|zsh|fish|powershell]",
		Short: "Emit or install a shell completion script for af",
		Long: "completions writes a shell-specific completion script to stdout. Pipe it into " +
			"your shell's completion loader (e.g. `af completions zsh > \"${fpath[1]}/_af\"`).\n\n" +
			"Pass --install to write the script to the shell's standard user-local completions " +
			"path instead of stdout. Installing is idempotent: re-running with unchanged content " +
			"writes nothing and reports the destination is already up to date.\n\n" +
			"There is no separate --shell flag: the existing positional argument doubles as the " +
			"shell override for --install (e.g. `af completions zsh --install`). Omit the " +
			"positional with --install to auto-detect the shell from the basename of $SHELL " +
			"(bash, zsh, or fish only); an empty or unrecognized $SHELL is an error asking for " +
			"an explicit shell argument.\n\n" +
			"--dry-run (only valid together with --install) reports what would be written, and " +
			"prints the same activation hint, without writing anything.",
		Args:      completionsArgs,
		ValidArgs: supportedCompletionShells(),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCompletions(cmd, args, opts)
		},
	}
	cmd.Flags().BoolVar(&opts.install, "install", false, "install the completion script to the shell's user-local completions path instead of printing it to stdout")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "with --install, report what would be installed without writing anything")
	return cmd
}

// completionsArgs enforces exactly one positional shell argument unless
// --install is set, in which case zero or one is accepted (zero
// triggers $SHELL auto-detection). This keeps the plain,
// non-installing `af completions <shell>` path's argument-count error
// byte-for-byte the same cobra-level error it always was.
func completionsArgs(cmd *cobra.Command, args []string) error {
	install, err := cmd.Flags().GetBool("install")
	if err != nil {
		return fmt.Errorf("completions: %w", err)
	}
	if install {
		return cobra.MaximumNArgs(1)(cmd, args)
	}
	return cobra.ExactArgs(1)(cmd, args)
}

func runCompletions(cmd *cobra.Command, args []string, opts *completionsOptions) error {
	if opts.dryRun && !opts.install {
		return fmt.Errorf("completions: %w", errDryRunRequiresInstall)
	}

	shell := ""
	if len(args) == 1 {
		shell = args[0]
	}
	if shell == "" {
		detected, err := detectShellFromEnv(os.Getenv("SHELL"))
		if err != nil {
			return fmt.Errorf("completions: %w", err)
		}
		shell = detected
	}

	script, err := generateCompletionScript(cmd, shell)
	if err != nil {
		return err
	}

	if !opts.install {
		_, err = cmd.OutOrStdout().Write(script)
		if err != nil {
			return fmt.Errorf("completions write: %w", err)
		}
		return nil
	}

	return installCompletions(cmd, shell, script, opts.dryRun)
}

// detectShellFromEnv maps a $SHELL value to a supported shell name by
// its path basename. An empty value or an unrecognized basename
// returns errCannotDetectShell; ps-based process-tree detection is
// deliberately out of scope (see issue #22).
func detectShellFromEnv(shellEnv string) (string, error) {
	base := filepath.Base(shellEnv)
	for _, candidate := range []string{shellBash, shellZsh, shellFish} {
		if base == candidate {
			return candidate, nil
		}
	}
	return "", errCannotDetectShell
}

// generateCompletionScript renders shell's completion script into an
// in-memory buffer using the same cobra generator calls the original
// stdout-only path used, so install and stdout modes always agree
// byte-for-byte.
func generateCompletionScript(cmd *cobra.Command, shell string) ([]byte, error) {
	root := cmd.Root()
	var buf bytes.Buffer

	var err error
	switch shell {
	case shellBash:
		err = root.GenBashCompletion(&buf)
	case shellZsh:
		err = root.GenZshCompletion(&buf)
	case shellFish:
		err = root.GenFishCompletion(&buf, true)
	case shellPowerShell:
		err = root.GenPowerShellCompletionWithDesc(&buf)
	default:
		return nil, fmt.Errorf("completions: %w %q; want one of %v", errUnsupportedShell, shell, supportedCompletionShells())
	}
	if err != nil {
		return nil, fmt.Errorf("completions %s: %w", shell, err)
	}

	return buf.Bytes(), nil
}

// completionInstallPath returns the standard user-local destination
// path for shell's completion script, rooted at home.
func completionInstallPath(home, shell string) (string, error) {
	switch shell {
	case shellZsh:
		return filepath.Join(home, ".zfunc", "_af"), nil
	case shellBash:
		return filepath.Join(home, ".bash_completion.d", "af"), nil
	case shellFish:
		return filepath.Join(home, ".config", "fish", "completions", "af.fish"), nil
	default:
		return "", fmt.Errorf("completions: %w %q; want one of %v", errUnsupportedShell, shell, installableCompletionShells())
	}
}

// installCompletions writes script to shell's install destination
// (see completionInstallPath), skipping the write entirely when the
// destination already holds byte-identical content, and always prints
// the shell's activation hint afterward. In dry-run mode nothing is
// written; the message instead describes what would happen.
func installCompletions(cmd *cobra.Command, shell string, script []byte, dryRun bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("completions install: resolve home: %w", err)
	}
	path, err := completionInstallPath(home, shell)
	if err != nil {
		return err
	}

	if dryRun {
		err = printLine(cmd, "would install %s completions to %s", shell, path)
		if err != nil {
			return err
		}
		return printActivationHint(cmd, shell)
	}

	written, err := writeCompletionScript(path, script)
	if err != nil {
		return fmt.Errorf("completions install: %w", err)
	}
	if written {
		err = printLine(cmd, "installed %s completions: %s", shell, path)
	} else {
		err = printLine(cmd, "completions for %s already up to date: %s", shell, path)
	}
	if err != nil {
		return err
	}
	return printActivationHint(cmd, shell)
}

// writeCompletionScript writes script to path, creating path's parent
// directory as needed, unless path already holds byte-identical
// content — in which case it does nothing. The returned bool reports
// whether a write actually happened, so the caller can pick between
// an "installed" and an "already up to date" message.
func writeCompletionScript(path string, script []byte) (bool, error) {
	existing, readErr := os.ReadFile(path) //nolint:gosec // path is built from os.UserHomeDir plus a fixed shell-specific suffix, not user input.
	if readErr == nil && bytes.Equal(existing, script) {
		return false, nil
	}

	err := os.MkdirAll(filepath.Dir(path), completionInstallDirPerm)
	if err != nil {
		return false, fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	err = session.WriteFileAtomic(path, script, completionInstallFilePerm)
	if err != nil {
		return false, fmt.Errorf("write %s: %w", path, err)
	}
	return true, nil
}

// printActivationHint prints the shell-specific rc-file hint that
// tells the user how to activate the installed completions. af never
// edits rc files itself.
func printActivationHint(cmd *cobra.Command, shell string) error {
	switch shell {
	case shellZsh:
		return printLines(cmd,
			"ensure `fpath=(~/.zfunc $fpath)` is set in ~/.zshrc before compinit runs",
			"ensure `autoload -Uz compinit && compinit` runs in ~/.zshrc",
		)
	case shellBash:
		return printLine(cmd, "source `~/.bash_completion.d/af` from ~/.bashrc")
	case shellFish:
		return printLine(cmd, "no action needed: fish loads completions from ~/.config/fish/completions automatically")
	default:
		return nil
	}
}

// printLine writes a single formatted, newline-terminated line to
// cmd's stdout.
func printLine(cmd *cobra.Command, format string, args ...any) error {
	_, err := fmt.Fprintf(cmd.OutOrStdout(), format+"\n", args...)
	if err != nil {
		return fmt.Errorf("completions write: %w", err)
	}
	return nil
}

// printLines writes each of lines as its own newline-terminated line
// to cmd's stdout.
func printLines(cmd *cobra.Command, lines ...string) error {
	for _, line := range lines {
		err := printLine(cmd, "%s", line)
		if err != nil {
			return err
		}
	}
	return nil
}
