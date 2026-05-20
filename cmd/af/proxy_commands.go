package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kakkoyun/af/internal/config"
	"github.com/kakkoyun/af/internal/proxy"
	"github.com/kakkoyun/af/internal/session"
)

var (
	errProxyNoState        = errors.New("workstream state not found")
	errEditorNotConfigured = errors.New("no editor configured (see [editor].terminal/[editor].visual)")
)

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
	state, cfg, err := loadProxyState(cmd.Context(), name)
	if err != nil {
		return err
	}
	body := opts.body
	if body == "" && opts.ai {
		body = fmt.Sprintf("[AI body for %s generated via model=%s]", state.Session.Name, opts.model)
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
