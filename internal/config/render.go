package config

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Render returns the effective configuration as canonical TOML.
//
// Output is stable and deterministic for the same input; map keys are
// emitted in sorted order. ProxyCommandConfig polymorphism is rendered
// as either `cmd = [...]` (argv mode) or `cmd = "..."` (shell mode) per
// the schema in ADR-036.
func Render(cfg Config) string {
	var b strings.Builder

	renderHeader(&b, cfg.SchemaVersion)
	renderGeneral(&b, cfg.General)
	renderBranch(&b, cfg.Branch)
	renderEditor(&b, cfg.Editor)
	renderDiff(&b, cfg.Diff)
	renderPR(&b, cfg.PR)
	renderRemote(&b, cfg.Remote)
	renderSandbox(&b, cfg.Sandbox)
	renderObsidian(&b, cfg.Obsidian)
	renderDoctor(&b, cfg.Doctor)
	renderSecret(&b, cfg.Secret)
	renderStatus(&b, cfg.Status)
	renderLifecycle(&b, cfg.Lifecycle)

	return b.String()
}

func renderHeader(b *strings.Builder, schemaVersion int) {
	fmt.Fprintf(b, "schema_version = %d\n\n", schemaVersion)
}

func renderGeneral(b *strings.Builder, g GeneralConfig) {
	b.WriteString("[general]\n")
	renderString(b, "default_agent", g.DefaultAgent)
	renderString(b, "multiplexer", g.Multiplexer)
	renderInt(b, "max_sessions", g.MaxSessions)
	renderString(b, "worktree_root", g.WorktreeRoot)
	b.WriteString("\n")
}

func renderBranch(b *strings.Builder, br BranchConfig) {
	b.WriteString("[branch]\n")
	renderString(b, "prefix", br.Prefix)
	renderBool(b, "prefix_on_fork_only", br.PrefixOnForkOnly)
	b.WriteString("\n")
}

func renderEditor(b *strings.Builder, e EditorConfig) {
	b.WriteString("[editor]\n")
	renderString(b, "terminal", e.Terminal)
	renderString(b, "visual", e.Visual)
	b.WriteString("\n")
}

func renderDiff(b *strings.Builder, d DiffConfig) {
	b.WriteString("[diff]\n")
	renderProxyCommand(b, d.Command)
	b.WriteString("\n")
}

func renderPR(b *strings.Builder, p PRConfig) {
	b.WriteString("[pr]\n")
	renderProxyCommand(b, p.Command)
	renderString(b, "template", p.Template)
	renderString(b, "ai_model", p.AIModel)
	b.WriteString("\n")

	b.WriteString("[pr.flag_template]\n")
	keys := make([]string, 0, len(p.FlagTemplate))
	for k := range p.FlagTemplate {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		renderStrings(b, k, p.FlagTemplate[k])
	}
	b.WriteString("\n")
}

func renderRemote(b *strings.Builder, r RemoteConfig) {
	b.WriteString("[remote]\n")
	renderString(b, "default_host", r.DefaultHost)
	renderStrings(b, "ssh_options", r.SSHOptions)
	b.WriteString("\n")
}

func renderSandbox(b *strings.Builder, s SandboxConfig) {
	b.WriteString("[sandbox]\n")
	renderString(b, "default_provider", s.DefaultProvider)
	b.WriteString("\n")

	b.WriteString("[sandbox.slicer]\n")
	renderString(b, "group", s.Slicer.Group)
	b.WriteString("\n")
}

func renderObsidian(b *strings.Builder, o ObsidianConfig) {
	b.WriteString("[obsidian]\n")
	renderString(b, "notes_vault", o.NotesVault)
	renderString(b, "notes_folder", o.NotesFolder)
	renderString(b, "notes_template", o.NotesTemplate)
	b.WriteString("\n")

	b.WriteString("[obsidian.vaults]\n")
	keys := make([]string, 0, len(o.Vaults))
	for k := range o.Vaults {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		renderString(b, k, o.Vaults[k])
	}
	b.WriteString("\n")
}

func renderDoctor(b *strings.Builder, d DoctorConfig) {
	b.WriteString("[doctor]\n")
	renderStrings(b, "extra_tools", d.ExtraTools)
	b.WriteString("\n")
}

func renderSecret(b *strings.Builder, s SecretConfig) {
	b.WriteString("[secret]\n")
	renderString(b, "keyring_service", s.KeyringService)
	renderStrings(b, "redact_keys", s.RedactKeys)
	b.WriteString("\n")
}

func renderStatus(b *strings.Builder, s StatusConfig) {
	b.WriteString("[status]\n")
	renderInt(b, "max_parallel", s.MaxParallel)
	b.WriteString("\n")
}

func renderLifecycle(b *strings.Builder, l LifecycleConfig) {
	b.WriteString("[lifecycle]\n")
	renderInt(b, "retention_days", l.RetentionDays)
	renderBool(b, "auto_archive", l.AutoArchive)
	b.WriteString("\n")
}

func renderProxyCommand(b *strings.Builder, cmd ProxyCommandConfig) {
	renderBool(b, "shell", cmd.Shell)
	if cmd.Shell {
		renderString(b, "cmd", cmd.Script)
		return
	}
	renderStrings(b, "cmd", cmd.Argv)
}

func renderString(b *strings.Builder, key, value string) {
	fmt.Fprintf(b, "%s = %s\n", key, strconv.Quote(value))
}

func renderInt(b *strings.Builder, key string, value int) {
	fmt.Fprintf(b, "%s = %d\n", key, value)
}

func renderBool(b *strings.Builder, key string, value bool) {
	fmt.Fprintf(b, "%s = %t\n", key, value)
}

func renderStrings(b *strings.Builder, key string, values []string) {
	b.WriteString(key)
	b.WriteString(" = [")
	for i, v := range values {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(strconv.Quote(v))
	}
	b.WriteString("]\n")
}
