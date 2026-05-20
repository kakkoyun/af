// Package setup performs the user-scope environment writes documented in
// ADR-045: state directory creation, config init, global gitignore
// update, shell completion install, and the Obsidian vault hint.
//
// The package is intentionally seamful: every external interaction goes
// through an injected interface so tests can substitute fakes.
package setup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/kakkoyun/af/internal/config"
)

const (
	stateDirPerm    = 0o750
	secretsDirPerm  = 0o700
	configDirPerm   = 0o750
	completionsPerm = 0o644
	gitignorePerm   = 0o644
)

// ErrUnsupportedShell reports a shell that has no completion install rule.
var ErrUnsupportedShell = errors.New("unsupported shell")

// Options configures a setup run. Zero values are sensible defaults.
type Options struct {
	Git              GitConfigurer
	GenerateBash     func(io.Writer) error
	GenerateZsh      func(io.Writer) error
	GenerateFish     func(io.Writer) error
	GenerateAnyShell func(shell string) ([]byte, error)
	Shell            string
	HomeDir          string
	UserConfigPath   string
	ShellDetectEnv   string
	StateDirRoot     string
	ConfigDirRoot    string
	GitignorePath    string
	BashCompletion   string
	ZshCompletion    string
	FishCompletion   string
	Force            bool
	SkipCompletions  bool
	SkipGitignore    bool
}

// GitConfigurer reads and writes user-scope git configuration.
type GitConfigurer interface {
	GetGlobal(ctx context.Context, key string) (string, error)
	SetGlobal(ctx context.Context, key, value string) error
}

// Result summarises what setup did. Fields are appended to as steps run.
type Result struct {
	StateDir       string
	ConfigPath     string
	GitignorePath  string
	Shell          string
	CompletionPath string
	ConfigCreated  bool
	GitignoreAdded bool
	ObsidianHint   bool
}

// Run performs the setup steps in order. The first failure aborts;
// earlier successful steps are recorded on the returned Result.
func Run(ctx context.Context, w io.Writer, opts Options) (Result, error) {
	defaults := withDefaults(opts)

	result := Result{StateDir: filepath.Join(defaults.StateDirRoot, "v1")}
	err := createStateDirs(result.StateDir)
	if err != nil {
		return result, err
	}

	configResult, err := ensureUserConfig(defaults)
	if err != nil {
		return result, err
	}
	result.ConfigPath = configResult.path
	result.ConfigCreated = configResult.created

	if !defaults.SkipGitignore {
		gi, giErr := updateGitignore(ctx, defaults)
		if giErr != nil {
			return result, giErr
		}
		result.GitignorePath = gi.path
		result.GitignoreAdded = gi.added
	}

	if !defaults.SkipCompletions {
		comp, compErr := installCompletions(defaults)
		if compErr != nil {
			return result, compErr
		}
		result.Shell = comp.shell
		result.CompletionPath = comp.path
	}

	result.ObsidianHint = needsObsidianHint(ctx, defaults)

	printSummary(w, result, defaults)
	return result, nil
}

func withDefaults(opts Options) Options {
	if opts.HomeDir == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			opts.HomeDir = home
		}
	}
	if opts.StateDirRoot == "" {
		opts.StateDirRoot = filepath.Join(opts.HomeDir, ".local", "share", "af")
	}
	if opts.ConfigDirRoot == "" {
		opts.ConfigDirRoot = filepath.Join(opts.HomeDir, ".config", "af")
	}
	if opts.UserConfigPath == "" {
		opts.UserConfigPath = filepath.Join(opts.ConfigDirRoot, "config.toml")
	}
	if opts.GitignorePath == "" {
		opts.GitignorePath = filepath.Join(opts.HomeDir, ".config", "git", "ignore")
	}
	if opts.BashCompletion == "" {
		opts.BashCompletion = filepath.Join(opts.HomeDir, ".local", "share", "bash-completion", "completions", "af")
	}
	if opts.ZshCompletion == "" {
		opts.ZshCompletion = filepath.Join(opts.HomeDir, ".config", "zsh", "completions", "_af")
	}
	if opts.FishCompletion == "" {
		opts.FishCompletion = filepath.Join(opts.HomeDir, ".config", "fish", "completions", "af.fish")
	}
	return opts
}

func createStateDirs(stateDir string) error {
	for _, sub := range []string{"sessions", "archive"} {
		err := os.MkdirAll(filepath.Join(stateDir, sub), stateDirPerm)
		if err != nil {
			return fmt.Errorf("create state subdir %s: %w", sub, err)
		}
	}
	err := os.MkdirAll(filepath.Join(stateDir, "secrets"), secretsDirPerm)
	if err != nil {
		return fmt.Errorf("create secrets dir: %w", err)
	}
	return nil
}

type configEnsureResult struct {
	path    string
	created bool
}

func ensureUserConfig(opts Options) (configEnsureResult, error) {
	res := configEnsureResult{path: opts.UserConfigPath}

	_, err := os.Stat(opts.UserConfigPath)
	switch {
	case err == nil && !opts.Force:
		return res, nil
	case err == nil && opts.Force:
		rmErr := os.Remove(opts.UserConfigPath)
		if rmErr != nil {
			return res, fmt.Errorf("remove existing config for --force: %w", rmErr)
		}
	case !errors.Is(err, os.ErrNotExist):
		return res, fmt.Errorf("stat user config: %w", err)
	}

	err = config.WriteUserConfig(opts.UserConfigPath)
	if err != nil {
		return res, fmt.Errorf("write user config: %w", err)
	}
	res.created = true
	return res, nil
}

type gitignoreResult struct {
	path  string
	added bool
}

func updateGitignore(ctx context.Context, opts Options) (gitignoreResult, error) {
	res := gitignoreResult{path: opts.GitignorePath}

	if opts.Git != nil {
		excludesFile, err := opts.Git.GetGlobal(ctx, "core.excludesfile")
		if err == nil && excludesFile != "" {
			res.path = expandTilde(excludesFile, opts.HomeDir)
		} else if err == nil {
			err = opts.Git.SetGlobal(ctx, "core.excludesfile", opts.GitignorePath)
			if err != nil {
				return res, fmt.Errorf("set git core.excludesfile: %w", err)
			}
		}
	}

	err := os.MkdirAll(filepath.Dir(res.path), configDirPerm)
	if err != nil {
		return res, fmt.Errorf("create gitignore parent: %w", err)
	}

	added, err := appendIgnoreEntry(res.path, ".af/")
	if err != nil {
		return res, err
	}
	res.added = added
	return res, nil
}

func appendIgnoreEntry(path, entry string) (bool, error) {
	existing, err := os.ReadFile(path) //nolint:gosec // Path resolved from $HOME or git config above.
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("read gitignore %s: %w", path, err)
	}
	if hasIgnoreEntry(string(existing), entry) {
		return false, nil
	}

	body := string(existing)
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	body += entry + "\n"

	err = os.WriteFile(path, []byte(body), gitignorePerm)
	if err != nil {
		return false, fmt.Errorf("write gitignore %s: %w", path, err)
	}
	return true, nil
}

func hasIgnoreEntry(body, entry string) bool {
	for _, line := range strings.Split(body, "\n") {
		if strings.TrimSpace(line) == entry {
			return true
		}
	}
	return false
}

type completionResult struct {
	shell string
	path  string
}

func installCompletions(opts Options) (completionResult, error) {
	shell := detectShell(opts)
	res := completionResult{shell: shell}

	path, generator, err := completionTarget(opts, shell)
	if err != nil {
		return res, err
	}
	res.path = path

	if generator == nil {
		return res, nil
	}

	err = os.MkdirAll(filepath.Dir(path), configDirPerm)
	if err != nil {
		return res, fmt.Errorf("create completion dir for %s: %w", shell, err)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, completionsPerm) //nolint:gosec // Path is derived from HomeDir.
	if err != nil {
		return res, fmt.Errorf("open completion file %s: %w", path, err)
	}
	defer func() {
		_ = file.Close() //nolint:errcheck // Best-effort close on the regenerated completion file.
	}()

	err = generator(file)
	if err != nil {
		return res, fmt.Errorf("generate %s completion: %w", shell, err)
	}
	return res, nil
}

func completionTarget(opts Options, shell string) (string, func(io.Writer) error, error) {
	switch shell {
	case "bash":
		return opts.BashCompletion, opts.GenerateBash, nil
	case "zsh":
		return opts.ZshCompletion, opts.GenerateZsh, nil
	case "fish":
		return opts.FishCompletion, opts.GenerateFish, nil
	case "powershell":
		return "", nil, nil
	default:
		return "", nil, fmt.Errorf("%w: %s", ErrUnsupportedShell, shell)
	}
}

func detectShell(opts Options) string {
	if opts.Shell != "" {
		return opts.Shell
	}
	value := opts.ShellDetectEnv
	if value == "" {
		value = os.Getenv("SHELL")
	}
	return filepath.Base(value)
}

func needsObsidianHint(ctx context.Context, opts Options) bool {
	cfg, err := config.LoadWithOptions(ctx, config.LoadOptions{UserConfigPath: opts.UserConfigPath})
	if err != nil {
		return false
	}
	return len(cfg.Obsidian.Vaults) == 0
}

func printSummary(w io.Writer, res Result, opts Options) {
	for _, line := range summaryLines(res, opts) {
		_, _ = fmt.Fprintln(w, line) //nolint:errcheck // Diagnostic output; Fprintln cannot fail on a bytes.Buffer or *os.File in normal use.
	}
}

func summaryLines(res Result, opts Options) []string {
	lines := []string{
		"af setup complete:",
		"  ✓ state dir:     " + res.StateDir,
		"  ✓ user config:   " + configSummaryNote(res),
	}
	if !opts.SkipGitignore {
		lines = append(lines, fmt.Sprintf("  ✓ git ignore:    %s (%s)", res.GitignorePath, gitignoreSummaryNote(res)))
	}
	if !opts.SkipCompletions {
		lines = append(lines, completionsSummaryLine(res))
	}
	if res.ObsidianHint {
		lines = append(lines, obsidianHintLines()...)
	}
	return lines
}

func configSummaryNote(res Result) string {
	if res.ConfigCreated {
		return res.ConfigPath + " (created)"
	}
	return res.ConfigPath + " (already exists)"
}

func gitignoreSummaryNote(res Result) string {
	if res.GitignoreAdded {
		return "entry added"
	}
	return "no change"
}

func completionsSummaryLine(res Result) string {
	switch {
	case res.CompletionPath != "":
		return fmt.Sprintf("  ✓ completions:   %s installed at %s", res.Shell, res.CompletionPath)
	case res.Shell == "powershell":
		return "  ! completions:   powershell — run `af completions powershell` and source the output from $PROFILE"
	default:
		return fmt.Sprintf("  ! completions:   shell %q not supported; rerun with --shell bash|zsh|fish", res.Shell)
	}
}

func obsidianHintLines() []string {
	return []string{
		"  ! obsidian:      [obsidian.vaults] empty — see hint below",
		"",
		"Tip: configure your Obsidian vault paths in ~/.config/af/config.toml under [obsidian.vaults]",
		"     to enable `af note` and the workstream markdown integration. Example:",
		"         [obsidian.vaults]",
		`         personal = "/Users/you/Vaults/personal"`,
	}
}

func expandTilde(path, home string) string {
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "~/") || path == "~" {
		return filepath.Join(home, strings.TrimPrefix(path, "~"))
	}
	return path
}
