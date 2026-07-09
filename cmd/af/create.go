package main

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	"github.com/kakkoyun/af/internal/sandbox"
	"github.com/kakkoyun/af/internal/secret"
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
	noAttach  bool
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
	newCreateContextOverride  func(*rootOptions) *createContext
	errEmptyGitTopLevel       = errors.New("git rev-parse --show-toplevel returned empty")
	errSandboxFlagUnsupported = errors.New("--sandbox only accepts \"slicer\" (ADR-060)")
)

const remoteURLHostAndPath = 2

// obsidianDisabledWarning is printed to stderr, at most once per
// invocation, whenever `af create` skips the Obsidian note step
// because [obsidian] notes_vault is unset (issue #17 Option 2). The
// skip itself is not an error, so the warning never changes the exit
// code — it only makes the silent skip visible.
const obsidianDisabledWarning = "note: Obsidian integration is disabled (notes_vault is empty — set [obsidian] notes_vault in ~/.config/af/config.toml)"

func newCreateCmd(opts *rootOptions) *cobra.Command {
	cOpts := &createOptions{root: opts}
	cmd := &cobra.Command{
		Use:   "create [name]",
		Short: "Create a local workstream: branch, worktree, state, tmux, primary agent",
		Long: "create scaffolds a local workstream: a git worktree on a new branch, the durable state.toml + " +
			"ledger.jsonl, an optional Obsidian note, a tmux session at the worktree path, and the primary agent " +
			"launch. When run interactively it then attaches to that tmux session by default; pass --no-attach " +
			"(or --bare) to skip the attach and print the next-steps hint instead.",
		Example: "  af create fix-auth\n" +
			"  af create fix-auth --agent claude\n" +
			"  af create fix-auth --no-attach\n" +
			"  af create fix-auth --sandbox slicer",
		Args: cobra.MaximumNArgs(1),
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
	cmd.Flags().BoolVar(&cOpts.bare, "bare", false, "skip tmux + agent launch (create state + worktree only); implies --no-attach")
	cmd.Flags().BoolVar(&cOpts.yolo, "yolo", false, "launch the primary agent with permissive approval mode")
	cmd.Flags().StringVar(&cOpts.remote, "remote", "", "ssh host to create the workstream on (ADR-041)")
	cmd.Flags().StringVar(&cOpts.sandbox, "sandbox", "", "sandbox provider: slicer (ADR-060)")
	cmd.Flags().BoolVar(&cOpts.noAttach, "no-attach", false, "never attach after create; print the next-steps hint instead")
	return cmd
}

func runCreate(ctx context.Context, cmd *cobra.Command, opts *createOptions, name string) error { //nolint:funlen // Pipeline orchestration; further splitting hurts readability.
	cfg, err := loadCreateConfig(ctx, opts.root)
	if err != nil {
		return err
	}
	warnIfObsidianVaultUnset(cmd, cfg)

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

	err = preCreateRemote(ctx, opts, cfg, repoSlug, agentName, fromBranch)
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
		ArchiveDir:       resolveArchiveDir(),
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

	err = launchSandbox(ctx, opts, cfg, result, primary)
	if err != nil {
		return err
	}

	err = printCreateResult(cmd, result)
	if err != nil {
		return err
	}

	return finishCreateOutput(ctx, cmd, cc, opts, result)
}

// finishCreateOutput implements issue #21: attach to the just-created
// tmux session by default, reusing the exact mux.Multiplexer.Attach
// mechanism `af resume` uses (attachResumeSession in
// cmd/af/suspend_resume.go), or print the next-steps footer when
// attaching would be wrong (bare, --no-attach, non-interactive, or no
// tmux session to attach to at all).
func finishCreateOutput(ctx context.Context, cmd *cobra.Command, cc *createContext, opts *createOptions, result lifecycle.CreateResult) error {
	if shouldAttachAfterCreate(cmd, opts, result) {
		err := cc.mux.Attach(ctx, result.TmuxSession)
		if err != nil {
			return fmt.Errorf("create: attach: %w", err)
		}
		return nil
	}
	return printCreateFooter(cmd.OutOrStdout(), result.SessionName, result.TmuxSession)
}

// isInteractiveCreateFunc detects whether the current invocation has a
// real terminal to attach to. It reuses the exact TTY-detection approach
// the ADR-070 fzf session picker already uses (isTerminalReader /
// isTerminalWriter in session_resolve.go), just checked against
// stdin/stdout instead of stdin/stderr, since attaching (unlike the
// picker) takes over stdout, not just stderr. It is a package-level var,
// like sessionPickerFunc/fzfCommandFunc/newResumeMux, so tests can force
// the interactive branch without a real pty.
//
//nolint:gochecknoglobals // Test seam for `af create`'s attach-vs-footer decision (issue #21).
var isInteractiveCreateFunc = func(cmd *cobra.Command) bool {
	return isTerminalReader(cmd.InOrStdin()) && isTerminalWriter(cmd.OutOrStdout())
}

// shouldAttachAfterCreate reports whether create should attach rather
// than print the footer. --bare and --no-attach both opt out
// unconditionally; otherwise create only attaches when there is a tmux
// session to attach to and the invocation is interactive.
func shouldAttachAfterCreate(cmd *cobra.Command, opts *createOptions, result lifecycle.CreateResult) bool {
	if opts.bare || opts.noAttach {
		return false
	}
	if result.TmuxSession == "" {
		return false
	}
	return isInteractiveCreateFunc(cmd)
}

// printCreateFooter prints the issue #25 Part 4.1 next-steps footer used
// whenever create does not attach. When there is no tmux session (a
// --bare/--no-attach create with nothing to attach to), the parenthetical
// tmux alternative is omitted rather than printing an empty target.
func printCreateFooter(w io.Writer, name, tmuxSession string) error {
	attachLine := "  → to attach:   af resume " + name
	if tmuxSession != "" {
		attachLine += "     (or: tmux attach -t " + tmuxSession + ")"
	}
	lines := []string{
		attachLine,
		"  → to check in: af status",
		"  → to finish:   af done " + name,
	}
	for _, line := range lines {
		_, err := fmt.Fprintln(w, line)
		if err != nil {
			return fmt.Errorf("write create footer: %w", err)
		}
	}
	return nil
}

// launchSandbox invokes lifecycle.LaunchSandboxWorkstream after the
// worktree and state files have been created by lifecycle.Create.
// Returns nil immediately when --sandbox is unset or --bare is active.
// The envelope file is placed in the same directory as state.toml so
// the slicer mount can source it, and is deleted by the deferred
// lifecycle.LaunchSandboxWorkstream cleanup.
func launchSandbox(ctx context.Context, opts *createOptions, cfg config.Config, result lifecycle.CreateResult, primary agent.Agent) error {
	if opts.sandbox == "" || opts.bare {
		return nil
	}
	resources := sandbox.SlicerResources{
		Name:        cfg.Sandbox.Slicer.Resources.Name,
		VCPU:        cfg.Sandbox.Slicer.Resources.VCPU,
		RAMGB:       cfg.Sandbox.Slicer.Resources.RAMGB,
		StorageSize: cfg.Sandbox.Slicer.Resources.StorageSize,
		GPUCount:    cfg.Sandbox.Slicer.Resources.GPUCount,
		Image:       cfg.Sandbox.Slicer.Resources.Image,
		Hypervisor:  cfg.Sandbox.Slicer.Resources.Hypervisor,
	}
	prober := sandbox.ExecGroupProber{Runner: sandbox.ExecRunner{}}
	group, _, err := sandbox.ResolveLaunchGroup(ctx, prober, result.SessionName, cfg.Sandbox.Slicer.Group, resources)
	if err != nil {
		return fmt.Errorf("create --sandbox resolve group: %w", err)
	}
	provider := sandbox.NewSlicerProvider(sandbox.SlicerOptions{
		Group:     group,
		Resources: resources,
	}, sandbox.ExecRunner{})
	agentArgv := primary.LaunchCmd(agent.LaunchOpts{
		Cwd:       result.WorktreePath,
		SessionID: result.SessionID,
	})
	envelopePath := filepath.Join(filepath.Dir(result.StatePath), result.SessionName+"-sandbox.env")
	_, err = lifecycle.LaunchSandboxWorkstream(ctx, lifecycle.SandboxContext{
		Provider: provider,
		Envelope: secret.Envelope{Path: envelopePath},
	}, sandbox.LaunchOpts{
		Workstream: result.SessionName,
		Worktree:   result.WorktreePath,
		AgentArgv:  agentArgv,
	})
	if err != nil {
		return fmt.Errorf("create --sandbox launch: %w", err)
	}
	return nil
}

func resolveAgentName(opts *createOptions, cfg config.Config) string {
	if opts.agentName != "" {
		return opts.agentName
	}
	return cfg.General.DefaultAgent
}

func preCreateRemote(ctx context.Context, opts *createOptions, cfg config.Config, repoSlug, agentName, fromBranch string) error {
	if opts.remote != "" {
		_, err := lifecycle.PrepareRemoteWorkstream(ctx, lifecycle.RemoteContext{
			Host:       opts.remote,
			SSHOptions: cfg.Remote.SSHOptions,
		}, repoSlug, agentName, fromBranch)
		if err != nil {
			return fmt.Errorf("create --remote: %w", err)
		}
	}
	if opts.sandbox != "" && opts.sandbox != "slicer" {
		return fmt.Errorf("%w: got %q", errSandboxFlagUnsupported, opts.sandbox)
	}
	return nil
}

func defaultCreateContext(_ *rootOptions) *createContext {
	return &createContext{
		git:   git.NewExecRunner(),
		mux:   mux.NewTmux(),
		notes: obsidian.NewDirStore(),
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

func resolvePrimaryAgent(agentName string) (agent.Agent, error) {
	registry := agent.DefaultRegistry()
	resolved, err := registry.Resolve(agentName)
	if err != nil {
		return nil, fmt.Errorf("resolve agent %q: %w", agentName, err)
	}
	return resolved, nil
}

// warnIfObsidianVaultUnset prints obsidianDisabledWarning to stderr
// exactly once when cfg.Obsidian.NotesVault is empty — the same
// condition under which resolveNotesDir skips the Obsidian note step.
// It never returns an error and never affects the command's exit code;
// it exists solely so the skip in issue #17 is no longer silent.
func warnIfObsidianVaultUnset(cmd *cobra.Command, cfg config.Config) {
	if cfg.Obsidian.NotesVault != "" {
		return
	}
	_, _ = fmt.Fprintln(cmd.ErrOrStderr(), obsidianDisabledWarning) //nolint:errcheck // Best-effort diagnostic; a stderr write failure must not fail `af create`.
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
		createTmuxLine(res),
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

// createTmuxLine renders the create summary's tmux line. Issue #24
// Option C: point users at the workstream name they should actually pass
// to af commands (session name), not the tmux session name, since
// passing the tmux name to `af resume` is exactly the confusion issue
// #24 is about. A --bare create has no tmux session at all, so the
// attach hint is omitted rather than pointing at an empty target.
func createTmuxLine(res lifecycle.CreateResult) string {
	if res.TmuxSession == "" {
		return "  tmux:      "
	}
	return fmt.Sprintf("  tmux:      %s   (attach: af resume %s)", res.TmuxSession, res.SessionName)
}

// resolveArchiveDir returns the canonical archive directory used to
// detect ADR-069 §3 name collisions with archived workstreams. A
// missing $HOME silently returns "" so collision checking is skipped
// rather than failing the create call.
func resolveArchiveDir() string {
	dir, err := defaultArchiveDir()
	if err != nil {
		return ""
	}
	return dir
}
