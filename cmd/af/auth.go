package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/kakkoyun/af/internal/config"
	"github.com/kakkoyun/af/internal/secret"
)

type authContext struct {
	root        *rootOptions
	keyringMake func(service string) secret.Keyring
	readSecret  func(io.Writer, io.Reader, string) (string, error)
}

// newAuthContextOverride lets tests substitute the authContext (and
// therefore the keyring and secret reader) without rewiring the cobra
// command tree.
//
//nolint:gochecknoglobals // Test seam for the auth subcommand tree.
var newAuthContextOverride func(*rootOptions) *authContext

var errEmptyAuthValue = errors.New("empty value")

const redactionPreviewLen = 4

func newAuthCmd(opts *rootOptions) *cobra.Command {
	var ac *authContext
	if newAuthContextOverride != nil {
		ac = newAuthContextOverride(opts)
	} else {
		ac = &authContext{
			root:        opts,
			keyringMake: func(service string) secret.Keyring { return secret.NewSystemKeyring(service) },
			readSecret:  readSecretFromStdin,
		}
	}

	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage credentials stored in the OS keyring",
		Long:  "auth manages credentials stored under the [secret].keyring_service (default \"af\") via the OS keyring. Values are read interactively from a TTY; non-TTY input is accepted from stdin.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprint(cmd.OutOrStdout(), cmd.UsageString())
			if err != nil {
				return fmt.Errorf("show auth help: %w", err)
			}
			return nil
		},
	}
	cmd.AddCommand(newAuthSetCmd(ac))
	cmd.AddCommand(newAuthGetCmd(ac))
	cmd.AddCommand(newAuthStatusCmd(ac))
	cmd.AddCommand(newAuthClearCmd(ac))
	cmd.AddCommand(newAuthListCmd(ac))
	return cmd
}

func newAuthSetCmd(ac *authContext) *cobra.Command {
	return &cobra.Command{
		Use:   "set <key>",
		Short: "Store a credential under <key>",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthSet(cmd.Context(), cmd, ac, args[0])
		},
	}
}

func newAuthGetCmd(ac *authContext) *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Print the credential stored under <key>",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthGet(cmd.Context(), cmd, ac, args[0])
		},
	}
}

func newAuthStatusCmd(ac *authContext) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show availability of known credentials",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAuthStatus(cmd.Context(), cmd, ac)
		},
	}
}

func newAuthClearCmd(ac *authContext) *cobra.Command {
	return &cobra.Command{
		Use:   "clear <key>",
		Short: "Remove the credential stored under <key>",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthClear(cmd.Context(), cmd, ac, args[0])
		},
	}
}

func newAuthListCmd(ac *authContext) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the names of all af-stored credentials",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAuthList(cmd.Context(), cmd, ac)
		},
	}
}

// curatedKeys returns the credential names af specifically tracks for
// `auth status`. The keyring may hold additional entries; status reports
// any extras under "Other keyring entries:".
func curatedKeys() []string {
	return []string{"anthropic_api_key", "openai_api_key", "github_token"}
}

func runAuthSet(ctx context.Context, cmd *cobra.Command, ac *authContext, key string) error {
	ring, err := authKeyring(ctx, ac)
	if err != nil {
		return err
	}

	value, err := ac.readSecret(cmd.ErrOrStderr(), cmd.InOrStdin(), key)
	if err != nil {
		return fmt.Errorf("auth set %s: %w", key, err)
	}
	if value == "" {
		return fmt.Errorf("auth set %s: %w", key, errEmptyAuthValue)
	}

	err = ring.Set(ctx, key, value)
	if err != nil {
		return fmt.Errorf("auth set %s: %w", key, err)
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "stored %s in keyring\n", key)
	if err != nil {
		return fmt.Errorf("auth set %s: write confirmation: %w", key, err)
	}
	return nil
}

func runAuthGet(ctx context.Context, cmd *cobra.Command, ac *authContext, key string) error {
	ring, err := authKeyring(ctx, ac)
	if err != nil {
		return err
	}

	value, err := ring.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("auth get %s: %w", key, err)
	}

	out := cmd.OutOrStdout()
	if isTerminalWriter(out) {
		_, err = fmt.Fprintln(out, value)
	} else {
		_, err = fmt.Fprintln(out, redactedDisplay(value))
	}
	if err != nil {
		return fmt.Errorf("auth get %s: write value: %w", key, err)
	}
	return nil
}

func runAuthStatus(ctx context.Context, cmd *cobra.Command, ac *authContext) error {
	ring, err := authKeyring(ctx, ac)
	if err != nil {
		return err
	}
	stored, err := ring.List(ctx)
	if err != nil {
		return fmt.Errorf("auth status: %w", err)
	}
	return writeStatusReport(cmd.OutOrStdout(), stored)
}

func writeStatusReport(out io.Writer, stored []string) error {
	storedSet := storedAsSet(stored)

	_, err := fmt.Fprintln(out, "Curated credentials:")
	if err != nil {
		return fmt.Errorf("auth status: write: %w", err)
	}
	for _, key := range curatedKeys() {
		marker, state := curatedMarker(storedSet, key)
		_, err = fmt.Fprintf(out, "  %s %-20s %s\n", marker, key, state)
		if err != nil {
			return fmt.Errorf("auth status: write: %w", err)
		}
	}

	extras := otherKeys(stored)
	if len(extras) == 0 {
		return nil
	}
	_, err = fmt.Fprintln(out, "\nOther keyring entries:")
	if err != nil {
		return fmt.Errorf("auth status: write: %w", err)
	}
	for _, key := range extras {
		_, err = fmt.Fprintf(out, "  • %s\n", key)
		if err != nil {
			return fmt.Errorf("auth status: write: %w", err)
		}
	}
	return nil
}

func storedAsSet(stored []string) map[string]struct{} {
	out := make(map[string]struct{}, len(stored))
	for _, key := range stored {
		out[key] = struct{}{}
	}
	return out
}

func curatedMarker(stored map[string]struct{}, key string) (string, string) {
	if _, ok := stored[key]; ok {
		return "✓", "available"
	}
	return "✗", "absent"
}

func runAuthClear(ctx context.Context, cmd *cobra.Command, ac *authContext, key string) error {
	ring, err := authKeyring(ctx, ac)
	if err != nil {
		return err
	}

	err = ring.Delete(ctx, key)
	if err != nil {
		return fmt.Errorf("auth clear %s: %w", key, err)
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "cleared %s from keyring\n", key)
	if err != nil {
		return fmt.Errorf("auth clear %s: write confirmation: %w", key, err)
	}
	return nil
}

func runAuthList(ctx context.Context, cmd *cobra.Command, ac *authContext) error {
	ring, err := authKeyring(ctx, ac)
	if err != nil {
		return err
	}
	keys, err := ring.List(ctx)
	if err != nil {
		return fmt.Errorf("auth list: %w", err)
	}
	out := cmd.OutOrStdout()
	for _, key := range keys {
		_, err = fmt.Fprintln(out, key)
		if err != nil {
			return fmt.Errorf("auth list: write: %w", err)
		}
	}
	return nil
}

func authKeyring(ctx context.Context, ac *authContext) (secret.Keyring, error) { //nolint:ireturn // Keyring interface decouples cmd from real-vs-fake backends.
	cfg, err := loadAuthConfig(ctx, ac.root)
	if err != nil {
		return nil, err
	}
	service := cfg.Secret.KeyringService
	if service == "" {
		service = "af"
	}
	return ac.keyringMake(service), nil
}

func loadAuthConfig(ctx context.Context, opts *rootOptions) (config.Config, error) {
	repoDir, err := os.Getwd()
	if err != nil {
		repoDir = ""
	}
	cfg, err := config.LoadWithOptions(ctx, config.LoadOptions{
		UserConfigPath: opts.configPath,
		RepoDir:        repoDir,
	})
	if err != nil {
		return config.Config{}, fmt.Errorf("auth: load config: %w", err)
	}
	return cfg, nil
}

// readSecretFromStdin prompts for a secret on the TTY (echo off) or
// reads a single line from stdin when stdin is not a terminal.
func readSecretFromStdin(prompt io.Writer, in io.Reader, key string) (string, error) {
	stdin, ok := in.(*os.File)
	if ok && term.IsTerminal(int(stdin.Fd())) {
		_, err := fmt.Fprintf(prompt, "value for %s: ", key)
		if err != nil {
			return "", fmt.Errorf("write prompt: %w", err)
		}
		bytes, err := term.ReadPassword(int(stdin.Fd()))
		_, _ = fmt.Fprintln(prompt) //nolint:errcheck // Newline after echo-off prompt.
		if err != nil {
			return "", fmt.Errorf("read password: %w", err)
		}
		return strings.TrimSpace(string(bytes)), nil
	}

	reader := bufio.NewReader(in)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read stdin: %w", err)
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func isTerminalWriter(w io.Writer) bool {
	file, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(file.Fd()))
}

func redactedDisplay(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= redactionPreviewLen {
		return "[REDACTED]"
	}
	return fmt.Sprintf("[REDACTED:%s...]", value[:redactionPreviewLen])
}

func otherKeys(stored []string) []string {
	curated := make(map[string]struct{})
	for _, k := range curatedKeys() {
		curated[k] = struct{}{}
	}
	out := make([]string, 0)
	for _, k := range stored {
		if _, ok := curated[k]; !ok {
			out = append(out, k)
		}
	}
	return out
}
