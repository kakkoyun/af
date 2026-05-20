package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kakkoyun/af/internal/agent"
	"github.com/kakkoyun/af/internal/config"
	"github.com/kakkoyun/af/internal/git"
	"github.com/kakkoyun/af/internal/proxy"
	"github.com/kakkoyun/af/internal/session"
)

var (
	errProxyNoState        = errors.New("workstream state not found")
	errEditorNotConfigured = errors.New("no editor configured (see [editor].terminal/[editor].visual)")
	errPRAIWebIncompatible = errors.New("pr: --ai is incompatible with --web")
	errPRAIEmptyDiff       = errors.New("pr: --ai requires a non-empty diff between base and head")
	errPRAIAgentNoBody     = errors.New("pr: agent does not support non-interactive body generation")
	errPRAIEmptyBody       = errors.New("pr: agent returned an empty body")
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
	var base string
	cmd := &cobra.Command{
		Use:   "diff [session]",
		Short: "Run the configured diff proxy command for a workstream",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			return runDiff(cmd, name, base)
		},
	}
	cmd.Flags().StringVar(&base, "base", "", "override the base ref")
	return cmd
}

func newPRCmd(_ *rootOptions) *cobra.Command {
	var (
		title string
		body  string
		draft bool
		web   bool
		ai    bool
		model string
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
				title: title,
				body:  body,
				draft: draft,
				web:   web,
				ai:    ai,
				model: model,
			})
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "PR title")
	cmd.Flags().StringVar(&body, "body", "", "PR body (overrides --ai if both are set)")
	cmd.Flags().BoolVar(&draft, "draft", false, "open the PR as a draft")
	cmd.Flags().BoolVar(&web, "web", false, "open the PR creation page in the browser")
	cmd.Flags().BoolVar(&ai, "ai", false, "ask the primary agent to author the PR body (ADR-057)")
	cmd.Flags().StringVar(&model, "ai-model", "", "override the agent model used by --ai")
	return cmd
}

type prOptions struct {
	title string
	body  string
	model string
	draft bool
	web   bool
	ai    bool
}

func runEditor(cmd *cobra.Command, name string, terminal, visual bool) error {
	state, cfg, err := loadProxyState(cmd.Context(), name)
	if err != nil {
		return err
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
	cmdExec := exec.CommandContext(cmd.Context(), target, state.Worktree.Path) //nolint:gosec // Editor name from config; workstream path from state.
	cmdExec.Stdout = cmd.OutOrStdout()
	cmdExec.Stderr = cmd.ErrOrStderr()
	cmdExec.Stdin = cmd.InOrStdin()
	err = cmdExec.Run()
	if err != nil {
		return fmt.Errorf("editor: %w", err)
	}
	return nil
}

func runDiff(cmd *cobra.Command, name, baseOverride string) error {
	state, cfg, err := loadProxyState(cmd.Context(), name)
	if err != nil {
		return err
	}
	tokens := proxy.Tokens{
		"base":     baseOrDefault(state, baseOverride),
		"head":     state.Worktree.Branch,
		"worktree": state.Worktree.Path,
	}
	command, err := buildProxyInvocation(cfg.Diff.Command, tokens, state.Worktree.Path)
	if err != nil {
		return fmt.Errorf("diff: %w", err)
	}
	out, err := proxy.ExecRunner{}.Run(cmd.Context(), command)
	_, _ = cmd.OutOrStdout().Write(out) //nolint:errcheck // Pass-through to user terminal.
	if err != nil {
		return fmt.Errorf("diff: %w", err)
	}
	return nil
}

func runPR(cmd *cobra.Command, name string, opts prOptions) error {
	if opts.ai && opts.web {
		return fmt.Errorf("%w", errPRAIWebIncompatible)
	}
	state, cfg, err := loadProxyState(cmd.Context(), name)
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
	diff, err := computeWorktreeDiff(ctx, git.NewExecRunner(), st.Worktree.Path, st.Worktree.BaseBranch, st.Worktree.Branch)
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
	return runAgentBody(ctx, bodyArgs, st.Worktree.Path, buildBodyPrompt(diff))
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
func buildBodyPrompt(diff string) string {
	return bodyPromptHeader + diff + bodyPromptFooter
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

func loadProxyState(ctx context.Context, name string) (session.State, config.Config, error) {
	statePath, err := resolveLifecycleStatePath(name)
	if err != nil {
		return session.State{}, config.Config{}, err
	}
	state, err := session.ReadState(statePath)
	if err != nil {
		return session.State{}, config.Config{}, fmt.Errorf("proxy: %w: %w", errProxyNoState, err)
	}
	cfg, err := config.LoadWithOptions(ctx, config.LoadOptions{RepoDir: state.Worktree.Path})
	if err != nil {
		return state, config.Config{}, fmt.Errorf("proxy: load config: %w", err)
	}
	return state, cfg, nil
}

func baseOrDefault(state session.State, override string) string {
	if override != "" {
		return override
	}
	if state.Worktree.BaseBranch != "" {
		return state.Worktree.BaseBranch
	}
	return "HEAD"
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
