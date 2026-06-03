package config

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

const (
	currentSchemaVersion = 1
	defaultMaxSessions   = 10
	defaultMaxParallel   = 8
	defaultRetentionDays = 90
	defaultPRRefreshTTL  = 10 * time.Minute
)

const (
	sectionGeneral  = "general"
	sectionBranch   = "branch"
	sectionEditor   = "editor"
	sectionDiff     = "diff"
	sectionPR       = "pr"
	sectionRemote   = "remote"
	sectionSandbox  = "sandbox"
	sectionObsidian = "obsidian"
	sectionDoctor   = "doctor"
	sectionSecret   = "secret"
	sectionStatus   = "status"
	sectionLife     = "lifecycle"
	sectionControl  = "control"
	sectionReview   = "review"
)

const (
	fieldSchemaVersion = "schema_version"
	fieldCmd           = "cmd"
	fieldShell         = "shell"
	fieldVaults        = "vaults"
)

var (
	errUnsupportedSchema   = errors.New("unsupported config schema")
	errInvalidCommand      = errors.New("invalid proxy command")
	errTypeMismatch        = errors.New("config type mismatch")
	errInvalidControlField = errors.New("invalid control field")
)

// Config is the fully merged af configuration.
type Config struct { //nolint:govet // Field grouping by semantic domain beats pointer-size packing here.
	Obsidian      ObsidianConfig
	Sandbox       SandboxConfig
	Editor        EditorConfig
	General       GeneralConfig
	Secret        SecretConfig
	Remote        RemoteConfig
	Doctor        DoctorConfig
	Branch        BranchConfig
	PR            PRConfig
	Review        ReviewConfig
	Diff          DiffConfig
	Lifecycle     LifecycleConfig
	Control       ControlConfig
	SchemaVersion int
	Status        StatusConfig
}

// ControlConfig carries per-repo (or per-user) workstream launch defaults per
// ADR-061. CLI flags always win; repo [control] overrides user [control];
// subsystem defaults are the fallback.
type ControlConfig struct {
	// Agent is the preferred agent provider ("pi", "claude", "codex", or "").
	Agent string
	// ApprovalMode is the agent permission mode ("" | "auto" | "yolo").
	ApprovalMode string
	// Sandbox is the sandbox provider ("" | "slicer").
	Sandbox string
	// Remote is the SSH host string (opaque; must have no shell metacharacters).
	Remote string
	// RemoteControl is the remote-control helper ("" | "off" | "superterm").
	RemoteControl string
	// MaxAgents caps the number of agents for this repo (0 = no repo-level cap).
	MaxAgents int
}

// GeneralConfig contains process-wide defaults.
type GeneralConfig struct {
	DefaultAgent string
	Multiplexer  string
	WorktreeRoot string
	MaxSessions  int
}

// BranchConfig contains branch naming defaults.
type BranchConfig struct {
	Prefix           string
	PrefixOnForkOnly bool
}

// EditorConfig contains editor proxy defaults.
type EditorConfig struct {
	Terminal string
	Visual   string
}

// ProxyCommandConfig stores either argv-mode or shell-mode proxy command data.
type ProxyCommandConfig struct {
	Script string
	Argv   []string
	Shell  bool
}

// DiffConfig contains the configured diff proxy command.
type DiffConfig struct {
	Command ProxyCommandConfig
}

// PRConfig contains the configured pull-request proxy command.
type PRConfig struct {
	FlagTemplate map[string][]string
	Template     string
	AIModel      string
	Command      ProxyCommandConfig
	// RefreshTTL bounds how stale the cached PR state may be before
	// af status / af info trigger a gh pr view refresh (ADR-071).
	// Default 10m. Set to 0 to force always-refresh on read.
	RefreshTTL time.Duration
}

// ReviewConfig holds the ADR-073 [review] section.
type ReviewConfig struct {
	// Agent slot used for the review. Empty means: use the
	// workstream's primary agent, or "claude" when no session is loaded.
	Agent string
	// Model override forwarded to BodyCmd. Empty means agent default.
	Model string
	// SystemPromptAppend is appended to the af-owned immutable system
	// prompt. The af prefix always runs first; this content cannot
	// replace it, only extend it. Repo-level overrides user-level.
	SystemPromptAppend string
	// SystemPromptAppendFile is a repo-relative path to a markdown file
	// whose contents are appended after SystemPromptAppend. When unset,
	// af looks for ".af/review-system-prompt.md" at the repo root.
	SystemPromptAppendFile string
	// SuggestedSkills are advisory slash-command names. The agent reads
	// .claude/commands/ to discover real skill definitions; this list
	// is purely advisory.
	SuggestedSkills []string
}

// RemoteConfig contains SSH remote defaults.
type RemoteConfig struct {
	DefaultHost string
	SSHOptions  []string
}

// SandboxConfig contains sandbox provider defaults.
// Only "slicer" is a valid provider per ADR-060.
type SandboxConfig struct {
	DefaultProvider string
	Slicer          SlicerConfig
}

// SlicerResourcesConfig holds per-repo VM resource requests per ADR-062.
// All fields are optional; zero values mean "use slicer/group default".
//
//nolint:govet // field order prioritises readability over pointer-size packing
type SlicerResourcesConfig struct {
	VCPU        int    // 0 = group default
	RAMGB       int    // 0 = group default
	GPUCount    int    // 0 = no GPU request
	Name        string // optional profile name; empty = derived from repo slug
	StorageSize string // e.g. "25G"; empty = group default
	Image       string // optional slicer image override
	Hypervisor  string // empty = slicer default; "firecracker" or "qemu"
}

// SlicerConfig contains slicer-specific sandbox defaults.
type SlicerConfig struct {
	Group     string
	Resources SlicerResourcesConfig
}

// ObsidianConfig contains note-writing defaults.
type ObsidianConfig struct {
	Vaults        map[string]string
	NotesVault    string
	NotesFolder   string
	NotesTemplate string
}

// DoctorConfig contains dependency-probe defaults.
type DoctorConfig struct {
	ExtraTools []string
}

// SecretConfig contains secret storage and redaction defaults.
type SecretConfig struct {
	KeyringService string
	RedactKeys     []string
}

// StatusConfig contains status command defaults.
type StatusConfig struct {
	MaxParallel int
}

// LifecycleConfig contains workstream lifecycle defaults.
type LifecycleConfig struct {
	RetentionDays int
	AutoArchive   bool
}

// LoadOptions selects optional config paths for LoadWithOptions.
type LoadOptions struct {
	Logger         *slog.Logger
	UserConfigPath string
	RepoDir        string
}

// Load reads the default user config and optional repo config for repoDir.
func Load(ctx context.Context, repoDir string) (Config, error) {
	return LoadWithOptions(ctx, LoadOptions{RepoDir: repoDir})
}

// LoadWithOptions reads, merges, validates, and normalizes af configuration.
func LoadWithOptions(ctx context.Context, opts LoadOptions) (Config, error) {
	err := ctx.Err()
	if err != nil {
		return Config{}, fmt.Errorf("load config: %w", err)
	}

	cfg := Defaults()
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	userPath, err := resolveUserConfigPath(opts.UserConfigPath)
	if err != nil {
		return Config{}, err
	}
	err = mergeFile(ctx, &cfg, userPath, true, logger)
	if err != nil {
		return Config{}, err
	}

	if opts.RepoDir != "" {
		repoPath := filepath.Join(opts.RepoDir, ".af", "config.toml")
		err = mergeFile(ctx, &cfg, repoPath, false, logger)
		if err != nil {
			return Config{}, err
		}
	}

	err = normalizePaths(&cfg)
	if err != nil {
		return Config{}, err
	}
	err = validateCommands(cfg)
	if err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// Defaults returns the compiled schema-version 1 defaults.
func Defaults() Config {
	return Config{
		SchemaVersion: currentSchemaVersion,
		General: GeneralConfig{
			DefaultAgent: "pi",
			Multiplexer:  "tmux",
			MaxSessions:  defaultMaxSessions,
			WorktreeRoot: "~/Workspace/.worktrees",
		},
		Branch: BranchConfig{
			PrefixOnForkOnly: true,
		},
		Editor: EditorConfig{
			Terminal: "$EDITOR",
		},
		Diff: DiffConfig{
			Command: ProxyCommandConfig{Argv: []string{"git", "diff", "{base}...HEAD"}},
		},
		PR: PRConfig{
			Command: ProxyCommandConfig{Argv: []string{"gh", "pr", "create", "--base", "{base}", "--head", "{head}"}},
			FlagTemplate: map[string][]string{
				"title": {"--title", "{title}"},
				"draft": {"--draft"},
				"web":   {"--web"},
				"body":  {"--body", "{body}"},
			},
			RefreshTTL: defaultPRRefreshTTL,
		},
		Review: ReviewConfig{
			SuggestedSkills: []string{"/review", "/go-review", "/simplify"},
		},
		Remote: RemoteConfig{
			SSHOptions: []string{"-o", "ServerAliveInterval=60"},
		},
		Obsidian: ObsidianConfig{
			NotesFolder: "00 - af",
			Vaults:      map[string]string{},
		},
		Doctor: DoctorConfig{
			ExtraTools: []string{},
		},
		Secret: SecretConfig{
			KeyringService: "af",
			RedactKeys:     []string{},
		},
		Status: StatusConfig{
			MaxParallel: defaultMaxParallel,
		},
		Lifecycle: LifecycleConfig{
			RetentionDays: defaultRetentionDays,
			AutoArchive:   true,
		},
	}
}

type configLayer struct {
	SchemaVersion *int
	General       generalLayer
	Branch        branchLayer
	Editor        editorLayer
	Diff          commandLayer
	PR            prLayer
	Review        reviewLayer
	Remote        remoteLayer
	Sandbox       sandboxLayer
	Obsidian      obsidianLayer
	Doctor        doctorLayer
	Secret        secretLayer
	Status        statusLayer
	Lifecycle     lifecycleLayer
	Control       controlLayer
}

type generalLayer struct {
	DefaultAgent *string
	Multiplexer  *string
	MaxSessions  *int
	WorktreeRoot *string
}

type branchLayer struct {
	Prefix           *string
	PrefixOnForkOnly *bool
}

type editorLayer struct {
	Terminal *string
	Visual   *string
}

type commandLayer struct {
	Shell  *bool
	Argv   *[]string
	Script *string
}

type prLayer struct {
	Command      commandLayer
	FlagTemplate map[string][]string
	Template     *string
	AIModel      *string
	RefreshTTL   *time.Duration
}

// reviewLayer is the ADR-073 [review] section layer.
type reviewLayer struct {
	Agent                  *string
	Model                  *string
	SystemPromptAppend     *string
	SystemPromptAppendFile *string
	SuggestedSkills        *[]string
}

type remoteLayer struct {
	DefaultHost *string
	SSHOptions  *[]string
}

type sandboxLayer struct {
	DefaultProvider     *string
	SlicerGroup         *string
	SlicerResourceName  *string
	SlicerResourceVCPU  *int
	SlicerResourceRAMGB *int
	SlicerStorageSize   *string
	SlicerGPUCount      *int
	SlicerImage         *string
	SlicerHypervisor    *string
}

type obsidianLayer struct {
	NotesVault    *string
	NotesFolder   *string
	NotesTemplate *string
	Vaults        map[string]string
}

type doctorLayer struct {
	ExtraTools *[]string
}

type secretLayer struct {
	KeyringService *string
	RedactKeys     *[]string
}

type statusLayer struct {
	MaxParallel *int
}

type lifecycleLayer struct {
	RetentionDays *int
	AutoArchive   *bool
}

type controlLayer struct {
	Agent         *string
	ApprovalMode  *string
	Sandbox       *string
	Remote        *string
	RemoteControl *string
	MaxAgents     *int
}

func mergeFile(ctx context.Context, cfg *Config, path string, allowGlobalOnly bool, logger *slog.Logger) error {
	err := ctx.Err()
	if err != nil {
		return fmt.Errorf("load config %s: %w", path, err)
	}

	layer, ok, err := loadLayer(path, allowGlobalOnly, logger)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	err = validateSchema(path, layer.SchemaVersion)
	if err != nil {
		return err
	}
	mergeLayer(cfg, layer)

	return nil
}

func resolveUserConfigPath(path string) (string, error) {
	if path != "" {
		return path, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config path: %w", err)
	}

	return filepath.Join(home, ".config", "af", "config.toml"), nil
}

func loadLayer(path string, allowGlobalOnly bool, logger *slog.Logger) (configLayer, bool, error) {
	raw, ok, err := decodeRawFile(path)
	if err != nil || !ok {
		return configLayer{}, ok, err
	}

	layer, err := parseLayer(raw, path, allowGlobalOnly, logger)
	if err != nil {
		return configLayer{}, true, err
	}

	return layer, true, nil
}

func decodeRawFile(path string) (map[string]any, bool, error) {
	var raw map[string]any
	_, err := toml.DecodeFile(path, &raw)
	if err == nil {
		return raw, true, nil
	}
	if os.IsNotExist(err) {
		return nil, false, nil
	}

	return nil, false, fmt.Errorf("parse config %s: %w", path, err)
}

func parseLayer(raw map[string]any, path string, allowGlobalOnly bool, logger *slog.Logger) (configLayer, error) {
	var layer configLayer
	value, ok, err := optionalInt(raw, fieldSchemaVersion, path)
	if err != nil {
		return configLayer{}, err
	}
	if ok {
		layer.SchemaVersion = &value
	}

	err = parseNamedSections(&layer, raw, path, allowGlobalOnly, logger)
	if err != nil {
		return configLayer{}, err
	}

	return layer, nil
}

func parseNamedSections(layer *configLayer, raw map[string]any, path string, allowGlobalOnly bool, logger *slog.Logger) error {
	parsers := map[string]func(map[string]any, string, bool, *slog.Logger, *configLayer) error{
		sectionGeneral:  parseGeneralSection,
		sectionBranch:   parseBranchSection,
		sectionEditor:   parseEditorSection,
		sectionDiff:     parseDiffSection,
		sectionPR:       parsePRSection,
		sectionReview:   parseReviewSection,
		sectionRemote:   parseRemoteSection,
		sectionSandbox:  parseSandboxSection,
		sectionObsidian: parseObsidianSection,
		sectionDoctor:   parseDoctorSection,
		sectionSecret:   parseSecretSection,
		sectionStatus:   parseStatusSection,
		sectionLife:     parseLifecycleSection,
		sectionControl:  parseControlSection,
	}

	for name, parser := range parsers {
		table, ok, err := optionalTable(raw, name, path)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		err = parser(table, path, allowGlobalOnly, logger, layer)
		if err != nil {
			return err
		}
	}

	return nil
}

func parseGeneralSection(table map[string]any, path string, _ bool, _ *slog.Logger, layer *configLayer) error {
	return assignGeneral(&layer.General, table, path)
}

func assignGeneral(layer *generalLayer, table map[string]any, path string) error {
	var err error
	layer.DefaultAgent, err = stringPointer(table, "default_agent", path)
	if err != nil {
		return err
	}
	layer.Multiplexer, err = stringPointer(table, "multiplexer", path)
	if err != nil {
		return err
	}
	layer.MaxSessions, err = intPointer(table, "max_sessions", path)
	if err != nil {
		return err
	}
	layer.WorktreeRoot, err = stringPointer(table, "worktree_root", path)
	if err != nil {
		return err
	}

	return nil
}

func parseBranchSection(table map[string]any, path string, _ bool, _ *slog.Logger, layer *configLayer) error {
	var err error
	layer.Branch.Prefix, err = stringPointer(table, "prefix", path)
	if err != nil {
		return err
	}
	layer.Branch.PrefixOnForkOnly, err = boolPointer(table, "prefix_on_fork_only", path)
	if err != nil {
		return err
	}

	return nil
}

func parseEditorSection(table map[string]any, path string, _ bool, _ *slog.Logger, layer *configLayer) error {
	var err error
	layer.Editor.Terminal, err = stringPointer(table, "terminal", path)
	if err != nil {
		return err
	}
	layer.Editor.Visual, err = stringPointer(table, "visual", path)
	if err != nil {
		return err
	}

	return nil
}

func parseDiffSection(table map[string]any, path string, _ bool, _ *slog.Logger, layer *configLayer) error {
	command, err := parseCommandLayer(table, path, sectionDiff)
	if err != nil {
		return err
	}
	layer.Diff = command

	return nil
}

func parsePRSection(table map[string]any, path string, _ bool, _ *slog.Logger, layer *configLayer) error {
	command, err := parseCommandLayer(table, path, sectionPR)
	if err != nil {
		return err
	}
	layer.PR.Command = command
	layer.PR.Template, err = stringPointer(table, "template", path)
	if err != nil {
		return err
	}
	layer.PR.AIModel, err = stringPointer(table, "ai_model", path)
	if err != nil {
		return err
	}
	layer.PR.FlagTemplate, err = stringSliceMap(table, "flag_template", path)
	if err != nil {
		return err
	}
	layer.PR.RefreshTTL, err = durationPointer(table, "refresh_ttl", path)
	if err != nil {
		return err
	}

	return nil
}

func parseReviewSection(table map[string]any, path string, _ bool, _ *slog.Logger, layer *configLayer) error {
	var err error
	layer.Review.Agent, err = stringPointer(table, "agent", path)
	if err != nil {
		return err
	}
	layer.Review.Model, err = stringPointer(table, "model", path)
	if err != nil {
		return err
	}
	layer.Review.SystemPromptAppend, err = stringPointer(table, "system_prompt_append", path)
	if err != nil {
		return err
	}
	layer.Review.SystemPromptAppendFile, err = stringPointer(table, "system_prompt_append_file", path)
	if err != nil {
		return err
	}
	layer.Review.SuggestedSkills, err = stringSlicePointer(table, "suggested_skills", path)
	if err != nil {
		return err
	}
	return nil
}

func parseRemoteSection(table map[string]any, path string, _ bool, _ *slog.Logger, layer *configLayer) error {
	var err error
	layer.Remote.DefaultHost, err = stringPointer(table, "default_host", path)
	if err != nil {
		return err
	}
	layer.Remote.SSHOptions, err = stringSlicePointer(table, "ssh_options", path)
	if err != nil {
		return err
	}

	return nil
}

// errSandboxProviderUnsupported is returned when a config file specifies
// a sandbox default_provider that is not "" or "slicer" (ADR-060).
var errSandboxProviderUnsupported = errors.New("sandbox.default_provider must be empty or \"slicer\"")

var (
	// errSlicerResourceConflict is returned when both group and resource fields are set.
	errSlicerResourceConflict = errors.New("sandbox.slicer: cannot set both group and resource fields")
	// errSlicerHypervisor is returned for unsupported hypervisor values.
	errSlicerHypervisor = errors.New("sandbox.slicer.resources.hypervisor must be empty, \"firecracker\", or \"qemu\"")
	// errSlicerStorageSize is returned for malformed storage_size values.
	errSlicerStorageSize = errors.New("sandbox.slicer.resources.storage_size must match \"<digits>[KMGT]\"")
	// errSlicerNegative is returned for negative integer resource fields.
	errSlicerNegative = errors.New("sandbox.slicer.resources: vcpu, ram_gb, and gpu_count must be >= 0")
)

func parseSandboxSection(table map[string]any, path string, _ bool, _ *slog.Logger, layer *configLayer) error {
	var err error
	layer.Sandbox.DefaultProvider, err = stringPointer(table, "default_provider", path)
	if err != nil {
		return err
	}
	if p := layer.Sandbox.DefaultProvider; p != nil {
		switch *p {
		case "", "slicer":
			// valid per ADR-060
		default:
			return fmt.Errorf("%w: got %q at %s", errSandboxProviderUnsupported, *p, path)
		}
	}
	return parseSlicerSection(table, path, layer)
}

func parseSlicerSection(table map[string]any, path string, layer *configLayer) error {
	slicer, ok, err := optionalTable(table, "slicer", path)
	if err != nil || !ok {
		return err
	}
	layer.Sandbox.SlicerGroup, err = stringPointer(slicer, "group", path)
	if err != nil {
		return err
	}
	err = parseSlicerResourcesSection(slicer, path, layer)
	if err != nil {
		return err
	}
	return validateSlicerLayer(layer.Sandbox, path)
}

func parseSlicerResourcesSection(slicer map[string]any, path string, layer *configLayer) error {
	res, ok, err := optionalTable(slicer, "resources", path)
	if err != nil || !ok {
		return err
	}
	layer.Sandbox.SlicerResourceName, err = stringPointer(res, "name", path)
	if err != nil {
		return err
	}
	layer.Sandbox.SlicerResourceVCPU, err = intPointer(res, "vcpu", path)
	if err != nil {
		return err
	}
	layer.Sandbox.SlicerResourceRAMGB, err = intPointer(res, "ram_gb", path)
	if err != nil {
		return err
	}
	layer.Sandbox.SlicerStorageSize, err = stringPointer(res, "storage_size", path)
	if err != nil {
		return err
	}
	layer.Sandbox.SlicerGPUCount, err = intPointer(res, "gpu_count", path)
	if err != nil {
		return err
	}
	layer.Sandbox.SlicerImage, err = stringPointer(res, "image", path)
	if err != nil {
		return err
	}
	layer.Sandbox.SlicerHypervisor, err = stringPointer(res, "hypervisor", path)
	if err != nil {
		return err
	}
	return nil
}

// storageSizeRe matches slicer-style size strings: digits followed by
// an optional unit suffix (K, M, G, T).
var storageSizeRe = regexp.MustCompile(`^\d+[KMGT]?$`)

func validateSlicerLayer(layer sandboxLayer, path string) error {
	err := validateSlicerIntegers(layer, path)
	if err != nil {
		return err
	}
	err = validateSlicerStorageAndHypervisor(layer, path)
	if err != nil {
		return err
	}
	if layer.SlicerGroup != nil && *layer.SlicerGroup != "" && slicerLayerHasResources(layer) {
		return fmt.Errorf("config %s: %w", path, errSlicerResourceConflict)
	}
	return nil
}

func validateSlicerIntegers(layer sandboxLayer, path string) error {
	for _, v := range []*int{layer.SlicerResourceVCPU, layer.SlicerResourceRAMGB, layer.SlicerGPUCount} {
		if v != nil && *v < 0 {
			return fmt.Errorf("config %s: %w", path, errSlicerNegative)
		}
	}
	return nil
}

func validateSlicerStorageAndHypervisor(layer sandboxLayer, path string) error {
	if layer.SlicerStorageSize != nil && *layer.SlicerStorageSize != "" {
		if !storageSizeRe.MatchString(*layer.SlicerStorageSize) {
			return fmt.Errorf("config %s: %w: got %q", path, errSlicerStorageSize, *layer.SlicerStorageSize)
		}
	}
	if layer.SlicerHypervisor != nil && *layer.SlicerHypervisor != "" {
		switch *layer.SlicerHypervisor {
		case "firecracker", "qemu":
		default:
			return fmt.Errorf("config %s: %w: got %q", path, errSlicerHypervisor, *layer.SlicerHypervisor)
		}
	}
	return nil
}

func slicerLayerHasResources(layer sandboxLayer) bool {
	return layer.SlicerResourceVCPU != nil ||
		layer.SlicerResourceRAMGB != nil ||
		layer.SlicerStorageSize != nil ||
		layer.SlicerGPUCount != nil ||
		layer.SlicerImage != nil ||
		layer.SlicerHypervisor != nil
}

func parseObsidianSection(table map[string]any, path string, allowGlobalOnly bool, logger *slog.Logger, layer *configLayer) error {
	var err error
	layer.Obsidian.NotesVault, err = stringPointer(table, "notes_vault", path)
	if err != nil {
		return err
	}
	layer.Obsidian.NotesFolder, err = stringPointer(table, "notes_folder", path)
	if err != nil {
		return err
	}
	layer.Obsidian.NotesTemplate, err = stringPointer(table, "notes_template", path)
	if err != nil {
		return err
	}
	layer.Obsidian.Vaults, err = globalOnlyStringMap(table, fieldVaults, path, allowGlobalOnly, logger)
	if err != nil {
		return err
	}

	return nil
}

func parseDoctorSection(table map[string]any, path string, _ bool, _ *slog.Logger, layer *configLayer) error {
	tools, err := stringSlicePointer(table, "extra_tools", path)
	if err != nil {
		return err
	}
	layer.Doctor.ExtraTools = tools

	return nil
}

func parseSecretSection(table map[string]any, path string, allowGlobalOnly bool, logger *slog.Logger, layer *configLayer) error {
	if !allowGlobalOnly {
		logger.WarnContext(context.Background(), "ignoring repo-only global config section", "path", path, "section", sectionSecret)
		return nil
	}

	var err error
	layer.Secret.KeyringService, err = stringPointer(table, "keyring_service", path)
	if err != nil {
		return err
	}
	layer.Secret.RedactKeys, err = stringSlicePointer(table, "redact_keys", path)
	if err != nil {
		return err
	}

	return nil
}

func parseStatusSection(table map[string]any, path string, _ bool, _ *slog.Logger, layer *configLayer) error {
	maxParallel, err := intPointer(table, "max_parallel", path)
	if err != nil {
		return err
	}
	layer.Status.MaxParallel = maxParallel

	return nil
}

func parseLifecycleSection(table map[string]any, path string, _ bool, _ *slog.Logger, layer *configLayer) error {
	var err error
	layer.Lifecycle.RetentionDays, err = intPointer(table, "retention_days", path)
	if err != nil {
		return err
	}
	layer.Lifecycle.AutoArchive, err = boolPointer(table, "auto_archive", path)
	if err != nil {
		return err
	}

	return nil
}

func parseControlSection(table map[string]any, path string, _ bool, _ *slog.Logger, layer *configLayer) error {
	var err error
	layer.Control.Agent, err = stringPointer(table, "agent", path)
	if err != nil {
		return err
	}
	layer.Control.ApprovalMode, err = stringPointer(table, "approval_mode", path)
	if err != nil {
		return err
	}
	layer.Control.Sandbox, err = stringPointer(table, "sandbox", path)
	if err != nil {
		return err
	}
	layer.Control.Remote, err = stringPointer(table, "remote", path)
	if err != nil {
		return err
	}
	layer.Control.RemoteControl, err = stringPointer(table, "remote_control", path)
	if err != nil {
		return err
	}
	layer.Control.MaxAgents, err = intPointer(table, "max_agents", path)
	if err != nil {
		return err
	}
	return validateControlLayer(layer.Control, path)
}

// shellMetaChars lists characters forbidden in control.remote per ADR-061.
const shellMetaChars = ";|&`$<>"

func validateControlLayer(layer controlLayer, path string) error {
	for _, check := range []func() error{
		func() error { return validateOptionalString(layer.Sandbox, path, validateControlSandbox) },
		func() error { return validateOptionalString(layer.RemoteControl, path, validateControlRemoteControl) },
		func() error { return validateOptionalString(layer.ApprovalMode, path, validateControlApprovalMode) },
		func() error { return validateOptionalString(layer.Remote, path, validateControlRemote) },
		func() error {
			if layer.MaxAgents != nil && *layer.MaxAgents < 0 {
				return fmt.Errorf("config %s: control.max_agents must be >= 0: %w", path, errInvalidControlField)
			}
			return nil
		},
	} {
		err := check()
		if err != nil {
			return err
		}
	}
	return nil
}

func validateOptionalString(ptr *string, path string, fn func(string, string) error) error {
	if ptr == nil {
		return nil
	}
	return fn(*ptr, path)
}

func validateControlSandbox(v, path string) error {
	switch v {
	case "", "slicer":
		return nil
	}
	return fmt.Errorf("config %s: control.sandbox %q is not one of [\"\", \"slicer\"]: %w", path, v, errInvalidControlField)
}

func validateControlRemoteControl(v, path string) error {
	switch v {
	case "", "off", "superterm":
		return nil
	}
	return fmt.Errorf("config %s: control.remote_control %q is not one of [\"\", \"off\", \"superterm\"]: %w", path, v, errInvalidControlField)
}

func validateControlApprovalMode(v, path string) error {
	switch v {
	case "", "auto", "yolo":
		return nil
	}
	return fmt.Errorf("config %s: control.approval_mode %q is not one of [\"\", \"auto\", \"yolo\"]: %w", path, v, errInvalidControlField)
}

func validateControlRemote(v, path string) error {
	if strings.ContainsAny(v, shellMetaChars) {
		return fmt.Errorf("config %s: control.remote %q contains shell metacharacter: %w", path, v, errInvalidControlField)
	}
	return nil
}

func mergeControl(cfg *ControlConfig, layer controlLayer) {
	assignString(&cfg.Agent, layer.Agent)
	assignString(&cfg.ApprovalMode, layer.ApprovalMode)
	assignString(&cfg.Sandbox, layer.Sandbox)
	assignString(&cfg.Remote, layer.Remote)
	assignString(&cfg.RemoteControl, layer.RemoteControl)
	assignInt(&cfg.MaxAgents, layer.MaxAgents)
}

func parseCommandLayer(table map[string]any, path, section string) (commandLayer, error) {
	var layer commandLayer
	var err error
	layer.Shell, err = boolPointer(table, fieldShell, path)
	if err != nil {
		return commandLayer{}, err
	}
	if raw, ok := table[fieldCmd]; ok {
		err = assignCommandValue(&layer, raw, path, section)
		if err != nil {
			return commandLayer{}, err
		}
	}

	return layer, nil
}

func assignCommandValue(layer *commandLayer, raw any, path, section string) error {
	if script, ok := raw.(string); ok {
		layer.Script = &script
		return nil
	}
	argv, ok, err := stringsFromAnySlice(raw, path, section+"."+fieldCmd)
	if err != nil {
		return err
	}
	if ok {
		layer.Argv = &argv
		return nil
	}

	return typeError(path, section+"."+fieldCmd, raw, "string or []string")
}

func globalOnlyStringMap(table map[string]any, key, path string, allowGlobalOnly bool, logger *slog.Logger) (map[string]string, error) {
	if _, ok := table[key]; !ok {
		return nil, nil //nolint:nilnil // Nil map plus nil error means the optional global-only table is absent.
	}
	if !allowGlobalOnly {
		logger.WarnContext(context.Background(), "ignoring repo-only global config section", "path", path, "section", sectionObsidian+"."+key)
		return nil, nil //nolint:nilnil // Nil map plus nil error means the global-only repo table was ignored.
	}

	return stringMap(table, key, path)
}

func validateSchema(path string, version *int) error {
	if version == nil {
		return nil
	}
	if *version > currentSchemaVersion {
		return fmt.Errorf("config %s: %s %d is newer than supported %d: %w", path, fieldSchemaVersion, *version, currentSchemaVersion, errUnsupportedSchema)
	}

	return nil
}

func mergeLayer(cfg *Config, layer configLayer) {
	if layer.SchemaVersion != nil {
		cfg.SchemaVersion = *layer.SchemaVersion
	}
	mergeGeneral(&cfg.General, layer.General)
	mergeBranch(&cfg.Branch, layer.Branch)
	mergeEditor(&cfg.Editor, layer.Editor)
	mergeCommand(&cfg.Diff.Command, layer.Diff)
	mergePR(&cfg.PR, layer.PR)
	mergeReview(&cfg.Review, layer.Review)
	mergeRemote(&cfg.Remote, layer.Remote)
	mergeSandbox(&cfg.Sandbox, layer.Sandbox)
	mergeObsidian(&cfg.Obsidian, layer.Obsidian)
	mergeDoctor(&cfg.Doctor, layer.Doctor)
	mergeSecret(&cfg.Secret, layer.Secret)
	mergeStatus(&cfg.Status, layer.Status)
	mergeLifecycle(&cfg.Lifecycle, layer.Lifecycle)
	mergeControl(&cfg.Control, layer.Control)
}

func mergeGeneral(cfg *GeneralConfig, layer generalLayer) {
	assignString(&cfg.DefaultAgent, layer.DefaultAgent)
	assignString(&cfg.Multiplexer, layer.Multiplexer)
	assignInt(&cfg.MaxSessions, layer.MaxSessions)
	assignString(&cfg.WorktreeRoot, layer.WorktreeRoot)
}

func mergeBranch(cfg *BranchConfig, layer branchLayer) {
	assignString(&cfg.Prefix, layer.Prefix)
	assignBool(&cfg.PrefixOnForkOnly, layer.PrefixOnForkOnly)
}

func mergeEditor(cfg *EditorConfig, layer editorLayer) {
	assignString(&cfg.Terminal, layer.Terminal)
	assignString(&cfg.Visual, layer.Visual)
}

func mergeCommand(cfg *ProxyCommandConfig, layer commandLayer) {
	assignBool(&cfg.Shell, layer.Shell)
	if layer.Argv != nil {
		cfg.Argv = append([]string(nil), (*layer.Argv)...)
		cfg.Script = ""
	}
	if layer.Script != nil {
		cfg.Script = *layer.Script
		cfg.Argv = nil
	}
}

func mergePR(cfg *PRConfig, layer prLayer) {
	mergeCommand(&cfg.Command, layer.Command)
	if layer.FlagTemplate != nil {
		cfg.FlagTemplate = cloneStringSliceMap(layer.FlagTemplate)
	}
	assignString(&cfg.Template, layer.Template)
	assignString(&cfg.AIModel, layer.AIModel)
	if layer.RefreshTTL != nil {
		cfg.RefreshTTL = *layer.RefreshTTL
	}
}

func mergeReview(cfg *ReviewConfig, layer reviewLayer) {
	assignString(&cfg.Agent, layer.Agent)
	assignString(&cfg.Model, layer.Model)
	assignString(&cfg.SystemPromptAppend, layer.SystemPromptAppend)
	assignString(&cfg.SystemPromptAppendFile, layer.SystemPromptAppendFile)
	assignStringSlice(&cfg.SuggestedSkills, layer.SuggestedSkills)
}

func mergeRemote(cfg *RemoteConfig, layer remoteLayer) {
	assignString(&cfg.DefaultHost, layer.DefaultHost)
	assignStringSlice(&cfg.SSHOptions, layer.SSHOptions)
}

func mergeSandbox(cfg *SandboxConfig, layer sandboxLayer) {
	assignString(&cfg.DefaultProvider, layer.DefaultProvider)
	assignString(&cfg.Slicer.Group, layer.SlicerGroup)
	assignString(&cfg.Slicer.Resources.Name, layer.SlicerResourceName)
	assignInt(&cfg.Slicer.Resources.VCPU, layer.SlicerResourceVCPU)
	assignInt(&cfg.Slicer.Resources.RAMGB, layer.SlicerResourceRAMGB)
	assignString(&cfg.Slicer.Resources.StorageSize, layer.SlicerStorageSize)
	assignInt(&cfg.Slicer.Resources.GPUCount, layer.SlicerGPUCount)
	assignString(&cfg.Slicer.Resources.Image, layer.SlicerImage)
	assignString(&cfg.Slicer.Resources.Hypervisor, layer.SlicerHypervisor)
}

func mergeObsidian(cfg *ObsidianConfig, layer obsidianLayer) {
	assignString(&cfg.NotesVault, layer.NotesVault)
	assignString(&cfg.NotesFolder, layer.NotesFolder)
	assignString(&cfg.NotesTemplate, layer.NotesTemplate)
	if layer.Vaults != nil {
		cfg.Vaults = cloneStringMap(layer.Vaults)
	}
}

func mergeDoctor(cfg *DoctorConfig, layer doctorLayer) {
	assignStringSlice(&cfg.ExtraTools, layer.ExtraTools)
}

func mergeSecret(cfg *SecretConfig, layer secretLayer) {
	assignString(&cfg.KeyringService, layer.KeyringService)
	assignStringSlice(&cfg.RedactKeys, layer.RedactKeys)
}

func mergeStatus(cfg *StatusConfig, layer statusLayer) {
	assignInt(&cfg.MaxParallel, layer.MaxParallel)
}

func mergeLifecycle(cfg *LifecycleConfig, layer lifecycleLayer) {
	assignInt(&cfg.RetentionDays, layer.RetentionDays)
	assignBool(&cfg.AutoArchive, layer.AutoArchive)
}

func normalizePaths(cfg *Config) error {
	var err error
	cfg.General.WorktreeRoot, err = expandHome(cfg.General.WorktreeRoot)
	if err != nil {
		return fmt.Errorf("expand general.worktree_root: %w", err)
	}
	cfg.PR.Template, err = expandHome(cfg.PR.Template)
	if err != nil {
		return fmt.Errorf("expand pr.template: %w", err)
	}
	cfg.Obsidian.NotesTemplate, err = expandHome(cfg.Obsidian.NotesTemplate)
	if err != nil {
		return fmt.Errorf("expand obsidian.notes_template: %w", err)
	}
	err = expandVaults(cfg.Obsidian.Vaults)
	if err != nil {
		return err
	}

	return nil
}

func expandVaults(vaults map[string]string) error {
	for name, path := range vaults {
		expanded, err := expandHome(path)
		if err != nil {
			return fmt.Errorf("expand obsidian.vaults.%s: %w", name, err)
		}
		vaults[name] = expanded
	}

	return nil
}

func expandHome(path string) (string, error) {
	if path == "" || path == "~" {
		return expandBareHome(path)
	}
	if !strings.HasPrefix(path, "~/") {
		return path, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}

	return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
}

func expandBareHome(path string) (string, error) {
	if path != "~" {
		return path, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}

	return home, nil
}

func validateCommands(cfg Config) error {
	err := validateCommand(sectionDiff, cfg.Diff.Command)
	if err != nil {
		return err
	}
	err = validateCommand(sectionPR, cfg.PR.Command)
	if err != nil {
		return err
	}

	return nil
}

func validateCommand(section string, command ProxyCommandConfig) error {
	if command.Shell {
		if command.Script == "" {
			return fmt.Errorf("%s.cmd: shell mode requires a string command: %w", section, errInvalidCommand)
		}
		return nil
	}
	if len(command.Argv) == 0 {
		return fmt.Errorf("%s.cmd: argv mode requires a non-empty string array: %w", section, errInvalidCommand)
	}

	return nil
}

func optionalTable(raw map[string]any, key, path string) (map[string]any, bool, error) {
	value, ok := raw[key]
	if !ok {
		return nil, false, nil
	}
	table, ok := value.(map[string]any)
	if !ok {
		return nil, false, typeError(path, key, value, "table")
	}

	return table, true, nil
}

func optionalInt(raw map[string]any, key, path string) (int, bool, error) {
	value, ok := raw[key]
	if !ok {
		return 0, false, nil
	}
	result, ok := value.(int64)
	if !ok {
		return 0, false, typeError(path, key, value, "integer")
	}

	return int(result), true, nil
}

func stringPointer(raw map[string]any, key, path string) (*string, error) {
	value, ok := raw[key]
	if !ok {
		return nil, nil //nolint:nilnil // Nil pointer plus nil error means the optional TOML key is absent.
	}
	result, ok := value.(string)
	if !ok {
		return nil, typeError(path, key, value, "string")
	}

	return &result, nil
}

func boolPointer(raw map[string]any, key, path string) (*bool, error) {
	value, ok := raw[key]
	if !ok {
		return nil, nil //nolint:nilnil // Nil pointer plus nil error means the optional TOML key is absent.
	}
	result, ok := value.(bool)
	if !ok {
		return nil, typeError(path, key, value, "bool")
	}

	return &result, nil
}

func intPointer(raw map[string]any, key, path string) (*int, error) {
	value, ok := raw[key]
	if !ok {
		return nil, nil //nolint:nilnil // Nil pointer plus nil error means the optional TOML key is absent.
	}
	result, ok := value.(int64)
	if !ok {
		return nil, typeError(path, key, value, "integer")
	}
	integer := int(result)

	return &integer, nil
}

// durationPointer parses a TOML string ("10m", "1h30m") via time.ParseDuration.
// Returns (nil, nil) when the key is absent; an error for non-string or
// unparseable values.
func durationPointer(raw map[string]any, key, path string) (*time.Duration, error) {
	value, ok := raw[key]
	if !ok {
		return nil, nil //nolint:nilnil // Nil pointer plus nil error means the optional TOML key is absent.
	}
	s, ok := value.(string)
	if !ok {
		return nil, typeError(path, key, value, "duration string")
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return nil, fmt.Errorf("%s: %s.%s: invalid duration %q: %w", path, sectionPR, key, s, err)
	}
	return &d, nil
}

func stringSlicePointer(raw map[string]any, key, path string) (*[]string, error) {
	value, ok := raw[key]
	if !ok {
		return nil, nil //nolint:nilnil // Nil pointer plus nil error means the optional TOML key is absent.
	}
	result, _, err := stringsFromAnySlice(value, path, key)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func stringSliceMap(raw map[string]any, key, path string) (map[string][]string, error) {
	value, ok := raw[key]
	if !ok {
		return nil, nil //nolint:nilnil // Nil map plus nil error means the optional TOML key is absent.
	}
	table, ok := value.(map[string]any)
	if !ok {
		return nil, typeError(path, key, value, "table")
	}

	return parseStringSliceMap(table, path, key)
}

func stringMap(raw map[string]any, key, path string) (map[string]string, error) {
	value, ok := raw[key]
	if !ok {
		return nil, nil //nolint:nilnil // Nil map plus nil error means the optional TOML key is absent.
	}
	table, ok := value.(map[string]any)
	if !ok {
		return nil, typeError(path, key, value, "table")
	}

	return parseStringMap(table, path, key)
}

func parseStringSliceMap(raw map[string]any, path, prefix string) (map[string][]string, error) {
	result := make(map[string][]string, len(raw))
	for key, value := range raw {
		field := prefix + "." + key
		slice, _, err := stringsFromAnySlice(value, path, field)
		if err != nil {
			return nil, err
		}
		result[key] = slice
	}

	return result, nil
}

func parseStringMap(raw map[string]any, path, prefix string) (map[string]string, error) {
	result := make(map[string]string, len(raw))
	for key, value := range raw {
		text, ok := value.(string)
		if !ok {
			return nil, typeError(path, prefix+"."+key, value, "string")
		}
		result[key] = text
	}

	return result, nil
}

func stringsFromAnySlice(raw any, path, field string) ([]string, bool, error) {
	slice, ok := raw.([]any)
	if !ok {
		return nil, false, typeError(path, field, raw, "[]string")
	}

	result := make([]string, 0, len(slice))
	for index, value := range slice {
		text, ok := value.(string)
		if !ok {
			return nil, true, typeError(path, fmt.Sprintf("%s[%d]", field, index), value, "string")
		}
		result = append(result, text)
	}

	return result, true, nil
}

func assignString(target, value *string) {
	if value != nil {
		*target = *value
	}
}

func assignBool(target, value *bool) {
	if value != nil {
		*target = *value
	}
}

func assignInt(target, value *int) {
	if value != nil {
		*target = *value
	}
}

func assignStringSlice(target, value *[]string) {
	if value != nil {
		*target = append([]string(nil), (*value)...)
	}
}

func cloneStringMap(values map[string]string) map[string]string {
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}

	return clone
}

func cloneStringSliceMap(values map[string][]string) map[string][]string {
	clone := make(map[string][]string, len(values))
	for key, value := range values {
		clone[key] = append([]string(nil), value...)
	}

	return clone
}

func typeError(path, key string, value any, want string) error {
	return fmt.Errorf("config %s: %s has type %T, want %s: %w", path, key, value, want, errTypeMismatch)
}
