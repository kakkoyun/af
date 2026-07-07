package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/kakkoyun/af/internal/agent"
	"github.com/kakkoyun/af/internal/config"
	"github.com/kakkoyun/af/internal/diff"
	"github.com/kakkoyun/af/internal/git"
	"github.com/kakkoyun/af/internal/pr"
	"github.com/kakkoyun/af/internal/proxy"
	"github.com/kakkoyun/af/internal/sandbox"
	"github.com/kakkoyun/af/internal/session"
)

var (
	errProxyNoState         = errors.New("workstream state not found")
	errEditorNotConfigured  = errors.New("no editor configured (see [editor].terminal/[editor].visual)")
	errPRAIWebIncompatible  = errors.New("pr: --ai is incompatible with --web")
	errPRAIEmptyDiff        = errors.New("pr: --ai requires a non-empty diff between base and head")
	errPRAIAgentNoBody      = errors.New("pr: agent does not support non-interactive body generation")
	errPRAIEmptyBody        = errors.New("pr: agent returned an empty body")
	errPRWorktreeLeasedToVM = errors.New("pr: host branch may not contain VM commits")
)

const defaultBodyAgentName = "pi"

const (
	bodyPromptHeader = "You are drafting a pull request body for the change below.\n\n# Diff\n"
	bodyPromptFooter = "\n\nWrite a PR body in markdown with these sections: Summary, Why, What changed, Test plan.\nBe concise. Do not include a title \u2014 only the body. Do not wrap in code fences.\n"
)

// prAIBodyFn is the function signature for the --ai body generation path.
// Tests replace prAIBodyFunc with a stub to avoid spawning a real agent.
type prAIBodyFn func(ctx context.Context, st session.State, model string) (string, error)

var prAIBodyFunc prAIBodyFn = defaultPRAIBody //nolint:gochecknoglobals // Test seam: replaced by tests to avoid spawning a real agent; same pattern as newAuthContextOverride.

// editorCommandFn builds the *exec.Cmd that runEditor will Run() to open the
// configured editor in the workstream worktree. Tests replace
// editorCommandFunc with a stub that returns a fast no-op command so the
// editor warning path (ADR-065 lease) can be asserted without spawning a
// real editor.
type editorCommandFn func(ctx context.Context, target, worktreePath string) *exec.Cmd

var editorCommandFunc editorCommandFn = defaultEditorCommand //nolint:gochecknoglobals // Test seam: same pattern as prAIBodyFunc above.

func defaultEditorCommand(ctx context.Context, target, worktreePath string) *exec.Cmd {
	return exec.CommandContext(ctx, target, worktreePath)
}

func newEditorCmd(_ *rootOptions) *cobra.Command {
	var (
		terminal bool
		visual   bool
	)
	cmd := &cobra.Command{
		Use:   "editor [session]",
		Short: "Open the configured editor (terminal or visual) in the workstream worktree",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			return runEditor(cmd, name, terminal, visual)
		},
	}
	cmd.Flags().BoolVarP(&terminal, "terminal", "t", false, "open the terminal editor (config [editor].terminal)")
	cmd.Flags().BoolVar(&visual, "visual", false, "open the visual editor (config [editor].visual)")
	return cmd
}

func newDiffCmd(_ *rootOptions) *cobra.Command {
	var (
		base        string
		web         bool
		interactive bool
	)
	cmd := &cobra.Command{
		Use:   "diff [session]",
		Short: "Render the workstream diff (hunk if installed, else git diff; --web opens diffity)",
		Long:  "diff resolves the base ref and dispatches to hunk (if installed) for terminal rendering, or plain git diff as a fallback. --web opens the diff range in diffity. Non-interactive stdout uses git diff --stat.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			return runDiff(cmd, name, base, web, interactive)
		},
	}
	cmd.Flags().StringVar(&base, "base", "", "override the base ref (default: stack parent > base_branch > HEAD)")
	cmd.Flags().BoolVar(&web, "web", false, "open the diff in a browser via diffity (ADR-064)")
	cmd.Flags().BoolVar(&interactive, "interactive", false, "force interactive mode even when stdout is not a TTY")
	return cmd
}

func newPRCmd(_ *rootOptions) *cobra.Command {
	var (
		title   string
		body    string
		draft   bool
		web     bool
		ai      bool
		model   string
		refresh bool
	)
	cmd := &cobra.Command{
		Use:   "pr [session]",
		Short: "Run the configured PR-create proxy command",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			return runPR(cmd, name, prOptions{
				title:   title,
				body:    body,
				draft:   draft,
				web:     web,
				ai:      ai,
				model:   model,
				refresh: refresh,
			})
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "PR title")
	cmd.Flags().StringVar(&body, "body", "", "PR body (overrides --ai if both are set)")
	cmd.Flags().BoolVar(&draft, "draft", false, "open the PR as a draft")
	cmd.Flags().BoolVar(&web, "web", false, "open the PR creation page in the browser")
	cmd.Flags().BoolVar(&ai, "ai", false, "ask the primary agent to author the PR body (ADR-057)")
	cmd.Flags().StringVar(&model, "ai-model", "", "override the agent model used by --ai")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "force-refresh the cached PR state via gh pr view (ADR-071); no PR is opened")
	return cmd
}

type prOptions struct {
	title   string
	body    string
	model   string
	draft   bool
	web     bool
	ai      bool
	refresh bool
}

func runEditor(cmd *cobra.Command, name string, terminal, visual bool) error {
	state, cfg, err := loadProxyState(cmd, name)
	if err != nil {
		return err
	}
	if state.IsLeasedToVM() {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: host worktree may be stale; run `af pull %s` for latest VM state\n", state.Session.Name) //nolint:errcheck // Informational warning.
	}
	target := cfg.Editor.Terminal
	if visual {
		target = cfg.Editor.Visual
	}
	if !terminal && !visual {
		target = firstNonEmpty(cfg.Editor.Visual, cfg.Editor.Terminal)
	}
	if target == "" {
		return fmt.Errorf("editor: %w", errEditorNotConfigured)
	}
	if strings.HasPrefix(target, "$") {
		target = os.Getenv(strings.TrimPrefix(target, "$"))
	}
	cmdExec := editorCommandFunc(cmd.Context(), target, state.Worktree.Path)
	cmdExec.Stdout = cmd.OutOrStdout()
	cmdExec.Stderr = cmd.ErrOrStderr()
	cmdExec.Stdin = cmd.InOrStdin()
	err = cmdExec.Run()
	if err != nil {
		return fmt.Errorf("editor: %w", err)
	}
	return nil
}

func runDiff(cmd *cobra.Command, name, baseOverride string, web, forceInteractive bool) error {
	state, _, err := loadProxyState(cmd, name)
	if err != nil {
		return err
	}
	if state.IsLeasedToVM() {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: host worktree may be stale; run `af pull %s` for latest VM state\n", state.Session.Name) //nolint:errcheck // Informational warning.
	}

	// Resolve base: explicit flag > stack parent branch > base_branch > HEAD.
	base := baseOverride
	if base == "" {
		base = firstNonEmpty(state.Stack.ParentBranch, state.Worktree.BaseBranch, "HEAD")
	}
	head := state.Worktree.Branch
	if head == "" {
		head = "HEAD"
	}

	mode := diff.ModeAuto
	if web {
		mode = diff.ModeWeb
	}

	renderErr := diff.Render(cmd.Context(), diff.Deps{Exec: diff.ExecExecutor{}}, diff.Options{
		Worktree:    state.Worktree.Path,
		Base:        base,
		Head:        head,
		Mode:        mode,
		Stdout:      cmd.OutOrStdout(),
		Stderr:      cmd.ErrOrStderr(),
		Interactive: forceInteractive || isInteractiveStdout(cmd),
	})
	if renderErr != nil {
		return fmt.Errorf("diff: %w", renderErr)
	}
	return nil
}

// isInteractiveStdout returns true when the command's stdout is an interactive
// terminal (i.e. a real TTY, not a pipe or test buffer).
func isInteractiveStdout(cmd *cobra.Command) bool {
	f, ok := cmd.OutOrStdout().(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

func runPR(cmd *cobra.Command, name string, opts prOptions) error {
	if opts.refresh {
		return runPRRefresh(cmd, name)
	}
	if opts.ai && opts.web {
		return fmt.Errorf("%w", errPRAIWebIncompatible)
	}
	// Check lease before loading full state; if held_by_vm the host branch
	// may not contain the VM's commits, making the PR misleading.
	stateEarly, stateEarlyErr := loadProxyStateOnly(cmd, name)
	if stateEarlyErr == nil && stateEarly.IsLeasedToVM() {
		return fmt.Errorf("%w (vm=%s); run `af pull %s` first", errPRWorktreeLeasedToVM, stateEarly.SlicerWT.VM, stateEarly.Session.Name)
	}
	state, cfg, err := loadProxyState(cmd, name)
	if err != nil {
		return err
	}
	body, err := maybeBuildAIBody(cmd.Context(), opts, state)
	if err != nil {
		return err
	}
	tokens := proxy.Tokens{
		"base":     state.Worktree.BaseBranch,
		"head":     state.Worktree.Branch,
		"worktree": state.Worktree.Path,
		"title":    opts.title,
		"body":     body,
	}
	command, err := buildProxyInvocation(cfg.PR.Command, tokens, state.Worktree.Path)
	if err != nil {
		return fmt.Errorf("pr: %w", err)
	}
	command.Args = append(command.Args, expandFlagTemplate(cfg.PR.FlagTemplate, opts, tokens)...)
	out, err := proxy.ExecRunner{}.Run(cmd.Context(), command)
	_, _ = cmd.OutOrStdout().Write(out) //nolint:errcheck // Pass-through to user terminal.
	if err != nil {
		return fmt.Errorf("pr: %w", err)
	}
	return nil
}

// maybeBuildAIBody returns the PR body: the user-supplied body if set, the
// AI-generated body if --ai is set and body is empty, or empty string otherwise.
func maybeBuildAIBody(ctx context.Context, opts prOptions, state session.State) (string, error) {
	if !opts.ai || opts.body != "" {
		return opts.body, nil
	}
	body, err := prAIBodyFunc(ctx, state, opts.model)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(body) == "" {
		return "", fmt.Errorf("%w", errPRAIEmptyBody)
	}
	return strings.TrimSpace(body), nil
}

// primaryAgentName returns the provider name for the "primary" agent slot,
// or empty string when none is recorded in state.
func primaryAgentName(agents []session.AgentState) string {
	for i := range agents {
		if agents[i].Slot == "primary" {
			return agents[i].Provider
		}
	}
	return ""
}

// defaultPRAIBody computes a diff, resolves the workstream's agent, invokes
// it in non-interactive print mode, and returns the trimmed stdout as the
// PR body per ADR-057.
func defaultPRAIBody(ctx context.Context, st session.State, model string) (string, error) {
	worktreeDiff, err := computeWorktreeDiff(ctx, git.NewExecRunner(), st.Worktree.Path, st.Worktree.BaseBranch, st.Worktree.Branch)
	if err != nil {
		return "", err
	}
	agentProvider, err := resolveBodyAgent(primaryAgentName(st.Agents))
	if err != nil {
		return "", err
	}
	bodyArgs, ok := agentProvider.BodyCmd(agent.BodyOpts{Cwd: st.Worktree.Path, Model: model})
	if !ok {
		return "", fmt.Errorf("%w", errPRAIAgentNoBody)
	}
	return runAgentBody(ctx, bodyArgs, st.Worktree.Path, buildBodyPrompt(worktreeDiff))
}

// computeWorktreeDiff returns the diff between base and head in dir.
// Returns errPRAIEmptyDiff when the diff is empty.
func computeWorktreeDiff(ctx context.Context, runner git.Runner, dir, base, head string) (string, error) {
	out, err := runner.Run(ctx, dir, "diff", base+"..."+head)
	if err != nil {
		return "", fmt.Errorf("pr --ai: compute diff: %w", err)
	}
	if strings.TrimSpace(string(out)) == "" {
		return "", fmt.Errorf("%w", errPRAIEmptyDiff)
	}
	return strings.TrimSpace(string(out)), nil
}

// resolveBodyAgent resolves the agent provider named agentName (defaults to
// defaultBodyAgentName when empty) from the default registry.
func resolveBodyAgent(agentName string) (agent.Agent, error) { //nolint:ireturn // Returns agent.Agent; registry hides concrete provider implementations.
	name := agentName
	if name == "" {
		name = defaultBodyAgentName
	}
	resolved, err := agent.DefaultRegistry().Resolve(name)
	if err != nil {
		return nil, fmt.Errorf("pr --ai: %w", err)
	}
	return resolved, nil
}

// runAgentBody invokes the agent argv in non-interactive mode, feeding prompt
// on stdin, and returns the raw stdout.
func runAgentBody(ctx context.Context, argv []string, cwd, prompt string) (string, error) {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // Agent binary resolved from trusted provider registry.
	cmd.Dir = cwd
	cmd.Stdin = strings.NewReader(prompt)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("pr --ai: agent run: %w", err)
	}
	return string(out), nil
}

// buildBodyPrompt returns the stdin prompt fed to the agent for PR body
// generation. In v1 the template is hard-coded (ADR-057).
func buildBodyPrompt(diffOutput string) string {
	return bodyPromptHeader + diffOutput + bodyPromptFooter
}

func expandFlagTemplate(template map[string][]string, opts prOptions, tokens proxy.Tokens) []string {
	out := make([]string, 0)
	if opts.title != "" {
		out = append(out, proxy.Expand(template["title"], tokens)...)
	}
	if opts.draft {
		out = append(out, proxy.Expand(template["draft"], tokens)...)
	}
	if opts.web {
		out = append(out, proxy.Expand(template["web"], tokens)...)
	}
	if tokens["body"] != "" {
		out = append(out, proxy.Expand(template["body"], tokens)...)
	}
	return out
}

func buildProxyInvocation(cfgCmd config.ProxyCommandConfig, tokens proxy.Tokens, dir string) (proxy.Command, error) {
	if cfgCmd.Shell {
		expanded := proxy.ExpandString(cfgCmd.Script, tokens)
		return proxy.BuildShellCommand(expanded, dir), nil
	}
	expanded := proxy.Expand(cfgCmd.Argv, tokens)
	command, err := proxy.BuildArgvCommand(expanded, dir)
	if err != nil {
		return proxy.Command{}, fmt.Errorf("build argv command: %w", err)
	}
	return command, nil
}

func loadProxyStateOnly(cmd *cobra.Command, name string) (session.State, error) {
	statePath, err := resolveLifecycleStatePathForCommand(cmd, name)
	if err != nil {
		return session.State{}, err
	}
	state, err := session.ReadState(statePath)
	if err != nil {
		return session.State{}, fmt.Errorf("proxy: %w: %w", errProxyNoState, err)
	}
	return state, nil
}

func loadProxyState(cmd *cobra.Command, name string) (session.State, config.Config, error) {
	statePath, err := resolveLifecycleStatePathForCommand(cmd, name)
	if err != nil {
		return session.State{}, config.Config{}, err
	}
	state, err := session.ReadState(statePath)
	if err != nil {
		return session.State{}, config.Config{}, fmt.Errorf("proxy: %w: %w", errProxyNoState, err)
	}
	cfg, err := config.LoadWithOptions(cmd.Context(), config.LoadOptions{RepoDir: state.Worktree.Path})
	if err != nil {
		return state, config.Config{}, fmt.Errorf("proxy: load config: %w", err)
	}
	return state, cfg, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// errPRRefreshNoPR is exposed for tests asserting EX_DATAERR-style exit
// when --refresh is invoked but no PR has been opened yet (ADR-071 §"PR
// open new" row).
var errPRRefreshNoPR = errors.New("pr: --refresh requires an open PR; create one with `af pr` first")

// runPRRefresh handles the ADR-071 --refresh path: force-refresh the
// cached PR state via gh pr view without opening anything. Writes back
// the updated state.toml and emits a pr_state_changed ledger event on
// a flip.
func runPRRefresh(cmd *cobra.Command, name string) error {
	statePath, err := resolveLifecycleStatePathForCommand(cmd, name)
	if err != nil {
		return fmt.Errorf("pr --refresh: %w", err)
	}
	var (
		state  session.State
		result pr.Result
	)
	err = withSessionLock(statePath, func() error {
		var lockedErr error
		state, lockedErr = session.ReadState(statePath)
		if lockedErr != nil {
			return fmt.Errorf("pr --refresh: read state: %w", lockedErr)
		}
		if state.PR.Number == 0 {
			return fmt.Errorf("%w", errPRRefreshNoPR)
		}
		cfg, lockedErr := loadConfigForRefresh(cmd.Context())
		if lockedErr != nil {
			return fmt.Errorf("pr --refresh: %w", lockedErr)
		}
		result, lockedErr = prRefreshFunc(cmd.Context(), &state.PR, pr.Options{
			Runner:   sandbox.ExecRunner{},
			RepoSlug: state.Worktree.RepoSlug,
			TTL:      cfg.PR.RefreshTTL,
			Force:    true,
			Now:      time.Now,
		})
		if lockedErr != nil {
			return fmt.Errorf("pr --refresh: %w", lockedErr)
		}
		lockedErr = session.WriteState(statePath, state)
		if lockedErr != nil {
			return fmt.Errorf("pr --refresh: write state: %w", lockedErr)
		}
		if result.Changed {
			lockedErr = emitPRStateChangedEvent(statePath, &state, result)
			if lockedErr != nil {
				return fmt.Errorf("pr --refresh: %w", lockedErr)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	writef(cmd.OutOrStdout(),
		"pr: %s: %s → %s (refreshed=%t skipped=%t)\n",
		state.Session.Name, result.Old, result.New, !result.Skipped, result.Skipped)
	return nil
}

// prRefreshFunc is the test seam wrapping pr.Refresh.
//
//nolint:gochecknoglobals // Test seam: replaced in tests; same pattern as prAIBodyFunc above.
var prRefreshFunc = pr.Refresh

// loadConfigForRefresh loads the layered config without requiring a
// workstream context — used by --refresh which only needs PR.RefreshTTL.
func loadConfigForRefresh(ctx context.Context) (config.Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return config.Config{}, fmt.Errorf("resolve home: %w", err)
	}
	loaded, err := config.LoadWithOptions(ctx, config.LoadOptions{
		UserConfigPath: home + "/.config/af/config.toml",
	})
	if err != nil {
		return config.Config{}, fmt.Errorf("load config: %w", err)
	}
	return loaded, nil
}

func emitPRStateChangedEvent(statePath string, state *session.State, result pr.Result) error {
	ledgerPath := pathDir(statePath) + "/ledger.jsonl"
	event := session.Event{
		Timestamp: time.Now().UTC(),
		Type:      "pr_state_changed",
		Fields: map[string]any{
			"session": state.Session.Name,
			"number":  state.PR.Number,
			"url":     state.PR.URL,
			"from":    result.Old,
			"to":      result.New,
		},
	}
	err := session.AppendEvent(ledgerPath, event)
	if err != nil {
		return fmt.Errorf("append pr_state_changed: %w", err)
	}
	return nil
}

// pathDir returns the directory portion of path without importing
// path/filepath at the top of this already-large file.
func pathDir(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[:i]
		}
	}
	return p
}
