package main

import (
	"strings"

	"github.com/spf13/cobra"
)

func completeSessionNames(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	stateDir, err := defaultSessionsDir()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	summaries, err := readAllStates(stateDir)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	out := make([]string, 0, len(summaries))
	for i := range summaries {
		name := summaries[i].state.Session.Name
		if strings.HasPrefix(name, toComplete) {
			out = append(out, name)
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

func completeLifecycleStates(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	states := []string{"active", "suspended", "completed", "abandoned"}
	out := make([]string, 0, len(states))
	for _, state := range states {
		if strings.HasPrefix(state, toComplete) {
			out = append(out, state)
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

func registerCompletionContracts(root *cobra.Command) {
	registerFlagCompletion(root, "session", completeSessionNames)
	registerSessionArgCompletions(root)
}

func registerFlagCompletion(cmd *cobra.Command, name string, fn func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective)) {
	err := cmd.RegisterFlagCompletionFunc(name, fn)
	if err != nil {
		return
	}
}

func registerSessionArgCompletions(cmd *cobra.Command) {
	for _, child := range cmd.Commands() {
		if strings.Contains(child.Use, "[session]") {
			child.ValidArgsFunction = completeSessionNames
		}
		registerSessionArgCompletions(child)
	}
}
