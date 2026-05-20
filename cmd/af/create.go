package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kakkoyun/af/internal/agent"
	"github.com/kakkoyun/af/internal/config"
	"github.com/kakkoyun/af/internal/git"
	"github.com/kakkoyun/af/internal/lifecycle"
	"github.com/kakkoyun/af/internal/mux"
	"github.com/kakkoyun/af/internal/obsidian"
)

type createOptions struct {
	root      *rootOptions
	from      string
	agentName string
	remote    string
	sandbox   string
	current   bool
	bare      bool
	yolo      bool
}

// createContext bundles the seams used by `af create` so tests can
// substitute fakes without rewiring the cobra command tree.
type createContext struct {
	git              git.Runner
	mux              mux.Multiplexer
	notes            obsidian.Store
	getwd            func() (string, error)
	stateDirOverride string
}

//nolint:gochecknoglobals // Test seam for the create subcommand.
var (
	newCreateContextOverride func(*rootOptions) *createContext
	errEmptyGitTopLevel      = errors.New("git rev-parse --show-toplevel returned empty")
)

const remoteURLHostAndPath = 2

func newCreateCmd(opts *rootOptions) *cobra.Command {
	cOpts := &createOptions{root: opts}
	cmd := &cobra.Command{
		Use:   "create [name]",
		Short: "Create a local workstream: branch, worktree, state, tmux, primary agent",
		Long:  "create scaffolds a local workstream per ADR-038/ADR-039: a git worktree on a new branch, the durable state.toml + ledger.jsonl, an optional Obsidian note, a tmux session at the worktree path, and the primary agent launch.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			return runCreate(cmd.Context(), cmd, cOpts, name)
		},
	}
	cmd.Flags().StringVar(&cOpts.from, "from", "", "base branch to fork the new workstream from")
	cmd.Flags().BoolVar(&cOpts.current, "current", false, "fork from the current HEAD")
	cmd.Flags().StringVar(&cOpts.agentName, "agent", "", "primary agent (pi, claude, codex); defaults to [general].default_agent")
	cmd.Flags().BoolVar(&cOpts.bare, "bare", false, "skip tmux + agent launch (create state + worktree only)")
	cmd.Flags().BoolVar(&cOpts.yolo, "yolo", false, "launch the primary agent with permissive approval mode")
	cmd.Flags().StringVar(&cOpts.remote, "remote", "", "ssh host to create the workstream on (ADR-041)")
	cmd.Flags().StringVar(&cOpts.sandbox, "sandbox", "", "sandbox provider: slicer or sbx (ADR-042)")
	return cmd
}

func runCreate(ctx context.Context, cmd *cobra.Command, opts *createOptions, name string) error {
	cfg, err := loadCreateConfig(ctx, opts.root)
	if err != nil {
		return err
	}

	cc := defaultCreateContext(opts.root)
	if newCreateContextOverride != nil {
		cc = newCreateContextOverride(opts.root)
	}

	gitRoot, repoSlug, hasUpstream, err := resolveRepoContext(ctx, cc)
	if err != nil {
		return err
	}

	fromBranch, err := resolveFromBranch(ctx, cc, gitRoot, opts)
	if err != nil {
		return err
	}

	agentName := resolveAgentName(opts, cfg)
	primary, err := resolvePrimaryAgent(agentName)
	if err != nil {
		return err
	}

	err = preCreateRemoteSandbox(ctx, cmd, opts, cfg, repoSlug, agentName, fromBranch)
	if err != nil {
		return err
	}

	result, err := lifecycle.Create(ctx, lifecycle.CreateDeps{
		Git:   cc.git,
		Mux:   cc.mux,
		Agent: primary,
		Notes: cc.notes,
	}, lifecycle.CreateOptions{
		Name:             name,
		FromBranch:       fromBranch,
		GitRoot:          gitRoot,
		RepoSlug:         repoSlug,
		WorktreeRoot:     cfg.General.WorktreeRoot,
		StateDir:         resolveStateDir(cc),
		NotesDir:         resolveNotesDir(cfg),
		BranchPrefix:     cfg.Branch.Prefix,
		PrefixOnForkOnly: cfg.Branch.PrefixOnForkOnly,
		HasUpstream:      hasUpstream,
		Bare:             opts.bare,
		AgentName:        agentName,
	})
	if err != nil {
		return fmt.Errorf("create: %w", err)
	}

	return printCreateResult(cmd, result)
}

func resolveAgentName(opts *createOptions, cfg config.Config) string {
	if opts.agentName != "" {
		return opts.agentName
	}
	return cfg.General.DefaultAgent
}

func preCreateRemoteSandbox(ctx context.Context, cmd *cobra.Command, opts *createOptions, cfg config.Config, repoSlug, agentName, fromBranch string) error {
	if opts.remote != "" {
		_, err := lifecycle.PrepareRemoteWorkstream(ctx, lifecycle.RemoteContext{
			Host:       opts.remote,
			SSHOptions: cfg.Remote.SSHOptions,
		}, repoSlug, agentName, fromBranch)
		if err != nil {
			return fmt.Errorf("create --remote: %w", err)
		}
	}
	if opts.sandbox != "" {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "sandbox provider %s requested; sandbox launch is performed at agent start (ADR-042)\n", opts.sandbox) //nolint:errcheck // Diagnostic only; failure surfaces in next step.
	}
	return nil
}

func defaultCreateContext(_ *rootOptions) *createContext {
	return &createContext{
		git:   git.NewExecRunner(),
		mux:   mux.NewTmux(),
		notes: nil,
		getwd: os.Getwd,
	}
}

func loadCreateConfig(ctx context.Context, opts *rootOptions) (config.Config, error) {
	repoDir, err := os.Getwd()
	if err != nil {
		repoDir = ""
	}
	cfg, err := config.LoadWithOptions(ctx, config.LoadOptions{
		UserConfigPath: opts.configPath,
		RepoDir:        repoDir,
	})
	if err != nil {
		return config.Config{}, fmt.Errorf("create: load config: %w", err)
	}
	return cfg, nil
}

func resolveRepoContext(ctx context.Context, cc *createContext) (string, string, bool, error) {
	cwd, err := cc.getwd()
	if err != nil {
		return "", "", false, fmt.Errorf("create: getwd: %w", err)
	}
	root, err := gitTopLevel(ctx, cc.git, cwd)
	if err != nil {
		return "", "", false, err
	}
	remote, remoteErr := readRemoteURL(ctx, cc.git, root, "origin")
	if remoteErr != nil {
		remote = ""
	}
	slug := guessRepoSlug(root, remote)
	hasUpstream := remoteExists(ctx, cc.git, root, "upstream")
	return root, slug, hasUpstream, nil
}

func gitTopLevel(ctx context.Context, runner git.Runner, dir string) (string, error) {
	out, err := runner.Run(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("git rev-parse --show-toplevel: %w", err)
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return "", errEmptyGitTopLevel
	}
	return root, nil
}

func readRemoteURL(ctx context.Context, runner git.Runner, dir, name string) (string, error) {
	out, err := runner.Run(ctx, dir, "config", "--get", "remote."+name+".url")
	if err != nil {
		return "", fmt.Errorf("git config remote.%s.url: %w", name, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func remoteExists(ctx context.Context, runner git.Runner, dir, name string) bool {
	_, err := runner.Run(ctx, dir, "config", "--get", "remote."+name+".url")
	return err == nil
}

func guessRepoSlug(root, remoteURL string) string {
	if remoteURL != "" {
		slug := parseRemoteSlug(remoteURL)
		if slug != "" {
			return slug
		}
	}
	return filepath.Base(root)
}

// parseRemoteSlug turns common git remote URLs into a "<host>/<owner>/<repo>" slug.
func parseRemoteSlug(remoteURL string) string {
	url := strings.TrimSpace(remoteURL)
	url = strings.TrimSuffix(url, ".git")
	if strings.HasPrefix(url, "git@") {
		rest := strings.TrimPrefix(url, "git@")
		host, path, ok := strings.Cut(rest, ":")
		if !ok {
			return ""
		}
		return host + "/" + path
	}
	for _, prefix := range []string{"https://", "http://", "ssh://"} {
		if !strings.HasPrefix(url, prefix) {
			continue
		}
		rest := strings.TrimPrefix(url, prefix)
		parts := strings.SplitN(rest, "/", remoteURLHostAndPath)
		if len(parts) != remoteURLHostAndPath {
			continue
		}
		host := strings.TrimPrefix(parts[0], "git@")
		return host + "/" + parts[1]
	}
	return ""
}

func resolveFromBranch(ctx context.Context, cc *createContext, gitRoot string, opts *createOptions) (string, error) {
	if opts.current {
		out, err := cc.git.Run(ctx, gitRoot, "rev-parse", "--abbrev-ref", "HEAD")
		if err != nil {
			return "", fmt.Errorf("git rev-parse HEAD: %w", err)
		}
		return strings.TrimSpace(string(out)), nil
	}
	if opts.from != "" {
		return opts.from, nil
	}
	for _, candidate := range []string{"upstream/main", "origin/main", "main"} {
		_, err := cc.git.Run(ctx, gitRoot, "rev-parse", "--verify", candidate)
		if err == nil {
			return candidate, nil
		}
	}
	return "HEAD", nil
}

//nolint:ireturn // Agent interface decouples cmd from concrete providers.
func resolvePrimaryAgent(agentName string) (agent.Agent, error) {
	registry := agent.DefaultRegistry()
	resolved, err := registry.Resolve(agentName)
	if err != nil {
		return nil, fmt.Errorf("resolve agent %q: %w", agentName, err)
	}
	return resolved, nil
}

func resolveNotesDir(cfg config.Config) string {
	if cfg.Obsidian.NotesVault == "" {
		return ""
	}
	vaultPath, ok := cfg.Obsidian.Vaults[cfg.Obsidian.NotesVault]
	if !ok || vaultPath == "" {
		return ""
	}
	if cfg.Obsidian.NotesFolder == "" {
		return vaultPath
	}
	return filepath.Join(vaultPath, cfg.Obsidian.NotesFolder)
}

func resolveStateDir(cc *createContext) string {
	if cc.stateDirOverride != "" {
		return cc.stateDirOverride
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "af", "v1", "sessions")
}

func printCreateResult(cmd *cobra.Command, res lifecycle.CreateResult) error {
	w := cmd.OutOrStdout()
	_, err := fmt.Fprintf(w, "created workstream %s\n", res.SessionName)
	if err != nil {
		return fmt.Errorf("write create summary: %w", err)
	}
	for _, line := range []string{
		"  branch:    " + res.Branch,
		"  worktree:  " + res.WorktreePath,
		"  state:     " + res.StatePath,
		"  ledger:    " + res.LedgerPath,
		"  tmux:      " + res.TmuxSession,
	} {
		_, err = fmt.Fprintln(w, line)
		if err != nil {
			return fmt.Errorf("write create line: %w", err)
		}
	}
	if res.NotePath != "" {
		_, err = fmt.Fprintln(w, "  note:      "+res.NotePath)
		if err != nil {
			return fmt.Errorf("write create note line: %w", err)
		}
	}
	return nil
}
