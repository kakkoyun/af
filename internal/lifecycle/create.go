package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kakkoyun/af/internal/agent"
	"github.com/kakkoyun/af/internal/git"
	"github.com/kakkoyun/af/internal/mux"
	"github.com/kakkoyun/af/internal/obsidian"
	"github.com/kakkoyun/af/internal/session"
	"github.com/kakkoyun/af/internal/version"
	"github.com/kakkoyun/af/internal/workstream"
)

const (
	stateDirPerm     = 0o750
	executionLocal   = "local"
	primaryAgentSlot = "primary"
)

// ErrCreate aborts when create cannot satisfy a precondition.
var ErrCreate = errors.New("create workstream failed")

// CreateOptions configures a local workstream creation.
type CreateOptions struct { //nolint:govet // Field grouping by semantic domain beats pointer-size packing.
	// Clock + IO.
	Now time.Time
	// Required identity inputs.
	Name         string // optional; auto-generated when empty
	FromBranch   string // base branch (e.g. upstream/main)
	GitRoot      string // absolute path to the source git repo
	RepoSlug     string // logical repo identifier (e.g. github.com/kakkoyun/af)
	WorktreeRoot string // expanded ~/Workspace/.worktrees
	StateDir     string // ~/.local/share/af/v1/sessions
	ArchiveDir   string // ~/.local/share/af/v1/archive; ADR-069 §3 collision check
	NotesDir     string // optional; if non-empty, write an Obsidian note
	// Identity rules.
	BranchPrefix string
	// Behaviour switches.
	AgentName        string // resolved agent provider name (pi/claude/codex)
	PrefixOnForkOnly bool
	HasUpstream      bool
	Bare             bool // skip agent launch
	// Control holds effective resolved ADR-061 control settings captured from
	// ResolveControl. Zero value means all defaults.
	Control ControlContext
	// SandboxGroup and SandboxResources capture the resolved slicer group and
	// resource profile (ADR-062). Written to state.toml at create time so
	// `af resume --respawn` can reproduce the same VM shape.
	SandboxGroup     string
	SandboxResources SandboxResourceProfile
}

// SandboxResourceProfile records the effective slicer VM resource shape
// resolved at create time per ADR-062.
//
//nolint:govet // field order prioritises readability over pointer-size packing
type SandboxResourceProfile struct {
	VCPU         int
	RAMGB        int
	GPUCount     int
	ProfileName  string
	StorageSize  string
	Image        string
	Hypervisor   string
	ManagedGroup string
}

// CreateResult records the artefacts produced by a successful Create.
type CreateResult struct {
	SessionID    string
	SessionName  string
	Branch       string
	WorktreePath string
	StatePath    string
	LedgerPath   string
	NotePath     string
	TmuxSession  string
}

// CreateDeps wires the orchestrator to its external collaborators.
type CreateDeps struct {
	Git     git.Runner
	Mux     mux.Multiplexer
	Agent   agent.Agent
	Notes   obsidian.Store // optional; nil disables note creation
	NowFunc func() time.Time
}

// Create executes the local-workstream create pipeline per ADR-038 +
// ADR-039. Steps are idempotent where possible: state.toml writes use
// the package's atomic helper, ledger appends are append-only, and the
// `.af/state.toml` discovery symlink is reconciled.
func Create(ctx context.Context, deps CreateDeps, opts CreateOptions) (CreateResult, error) {
	err := validateCreateInputs(deps, opts)
	if err != nil {
		return CreateResult{}, err
	}

	resolved := resolveCreateNames(opts)

	// The collision check and session-dir creation must be atomic
	// against concurrent creates, so both run under the state-root
	// lock (ADR-069 strict collision semantics).
	var (
		plan       git.WorktreePlan
		statePath  string
		ledgerPath string
	)
	err = withStateRootLock(opts.StateDir, func() error {
		lockedErr := checkNameCollision(opts.StateDir, opts.ArchiveDir, resolved.name)
		if lockedErr != nil {
			return lockedErr
		}

		plan, lockedErr = git.PlanPrimaryWorktree(git.WorktreeOptions{
			Root:   resolved.worktreeRoot,
			Repo:   resolved.repoSlug,
			Branch: resolved.branch,
		})
		if lockedErr != nil {
			return fmt.Errorf("plan worktree: %w", lockedErr)
		}

		lockedErr = ensureGitWorktree(ctx, deps.Git, opts.GitRoot, plan, opts.FromBranch)
		if lockedErr != nil {
			return lockedErr
		}

		statePath, ledgerPath, lockedErr = writeInitialState(opts, resolved, plan)
		return lockedErr
	})
	if err != nil {
		return CreateResult{}, err
	}

	err = git.EnsureStateSymlink(plan.Path, statePath)
	if err != nil {
		return CreateResult{}, fmt.Errorf("symlink .af/state.toml: %w", err)
	}

	notePath, err := writeWorkstreamNote(ctx, deps.Notes, resolved, plan, opts)
	if err != nil {
		return CreateResult{}, err
	}

	tmuxName, err := launchTmuxAndAgent(ctx, deps, resolved, plan, opts)
	if err != nil {
		return CreateResult{}, err
	}

	return CreateResult{
		SessionID:    resolved.sessionID,
		SessionName:  resolved.name,
		Branch:       resolved.branch,
		WorktreePath: plan.Path,
		StatePath:    statePath,
		LedgerPath:   ledgerPath,
		NotePath:     notePath,
		TmuxSession:  tmuxName,
	}, nil
}

type resolvedNames struct { //nolint:govet // Field grouping prioritises readability over packing.
	name         string
	branch       string
	sessionID    string
	worktreeRoot string
	repoSlug     string
	tmuxSession  string
	now          time.Time
}

func resolveCreateNames(opts CreateOptions) resolvedNames {
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	name := opts.Name
	if name == "" {
		name = workstream.AutoSessionName(opts.RepoSlug, now)
	}
	branch := workstream.BranchName(workstream.BranchOptions{
		Name:              name,
		Prefix:            opts.BranchPrefix,
		PrefixOnForkOnly:  opts.PrefixOnForkOnly,
		HasUpstreamRemote: opts.HasUpstream,
	})
	id := workstream.SessionID(opts.RepoSlug, branch, primaryAgentSlot, now).String()
	tmux := "af-" + workstream.Sanitize(name)
	return resolvedNames{
		name:         name,
		branch:       branch,
		sessionID:    id,
		worktreeRoot: opts.WorktreeRoot,
		repoSlug:     opts.RepoSlug,
		tmuxSession:  tmux,
		now:          now,
	}
}

func validateCreateInputs(deps CreateDeps, opts CreateOptions) error {
	err := validateCreateOpts(opts)
	if err != nil {
		return err
	}
	return validateCreateDeps(deps, opts)
}

func validateCreateOpts(opts CreateOptions) error {
	switch {
	case opts.GitRoot == "":
		return fmt.Errorf("%w: empty git root", ErrCreate)
	case opts.RepoSlug == "":
		return fmt.Errorf("%w: empty repo slug", ErrCreate)
	case opts.WorktreeRoot == "":
		return fmt.Errorf("%w: empty worktree root", ErrCreate)
	case opts.StateDir == "":
		return fmt.Errorf("%w: empty state dir", ErrCreate)
	case opts.FromBranch == "":
		return fmt.Errorf("%w: empty from-branch", ErrCreate)
	}
	err := workstream.ValidateSessionName(opts.Name)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCreate, err)
	}
	return nil
}

func validateCreateDeps(deps CreateDeps, opts CreateOptions) error {
	if deps.Git == nil {
		return fmt.Errorf("%w: nil git runner", ErrCreate)
	}
	if !opts.Bare && deps.Mux == nil {
		return fmt.Errorf("%w: nil mux for non-bare create", ErrCreate)
	}
	if !opts.Bare && deps.Agent == nil {
		return fmt.Errorf("%w: nil agent for non-bare create", ErrCreate)
	}
	return nil
}

func ensureGitWorktree(ctx context.Context, runner git.Runner, gitRoot string, plan git.WorktreePlan, fromBranch string) error {
	err := os.MkdirAll(filepath.Dir(plan.Path), stateDirPerm)
	if err != nil {
		return fmt.Errorf("create worktree parent: %w", err)
	}
	_, err = runner.Run(ctx, gitRoot, "worktree", "add", "-b", plan.Branch, plan.Path, fromBranch)
	if err != nil {
		return fmt.Errorf("git worktree add: %w", err)
	}
	return nil
}

// withStateRootLock runs fn while holding the exclusive flock at
// <stateDir>/.af.lock, serializing create's collision-check +
// session-dir creation against concurrent af processes.
func withStateRootLock(stateDir string, fn func() error) error {
	lock, err := session.LockFile(filepath.Join(stateDir, session.LockFileName), session.LockExclusive)
	if err != nil {
		return fmt.Errorf("lock state root %s: %w", stateDir, err)
	}
	defer func() { _ = lock.Unlock() }() //nolint:errcheck // Best-effort unlock on return.
	return fn()
}

// ErrNameCollision reports that the requested session name is already
// in use by an active, suspended, or archived workstream. ADR-069 §3
// requires strict collision across all three.
var ErrNameCollision = errors.New("session name already in use (active, suspended, or archived)")

// checkNameCollision returns ErrNameCollision when name already exists
// in stateDir (active/suspended) or archiveDir (archived). Empty
// archiveDir disables the archive check.
func checkNameCollision(stateDir, archiveDir, name string) error {
	if name == "" {
		return nil
	}
	for _, root := range []string{stateDir, archiveDir} {
		if root == "" {
			continue
		}
		candidate, err := containedJoin(root, name)
		if err != nil {
			return err
		}
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return fmt.Errorf("%w: %q already exists at %s", ErrNameCollision, name, candidate)
		}
	}
	return nil
}

// containedJoin joins name onto root and rejects results that escape
// root. It backstops workstream.ValidateSessionName for any caller that
// reaches path construction without opts validation.
func containedJoin(root, name string) (string, error) {
	candidate := filepath.Join(root, filepath.FromSlash(name))
	rel, err := filepath.Rel(root, candidate)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %q escapes %s", workstream.ErrInvalidSessionName, name, root)
	}
	return candidate, nil
}

func writeInitialState(opts CreateOptions, resolved resolvedNames, plan git.WorktreePlan) (string, string, error) {
	sessionDir, err := containedJoin(opts.StateDir, resolved.name)
	if err != nil {
		return "", "", err
	}
	err = os.MkdirAll(sessionDir, stateDirPerm)
	if err != nil {
		return "", "", fmt.Errorf("create session dir: %w", err)
	}
	statePath := filepath.Join(sessionDir, "state.toml")
	ledgerPath := filepath.Join(sessionDir, "ledger.jsonl")

	state := buildInitialState(opts, resolved, plan)
	err = session.WriteState(statePath, state)
	if err != nil {
		return "", "", fmt.Errorf("write state.toml: %w", err)
	}

	event := session.Event{
		Timestamp: resolved.now,
		Type:      "created",
		Fields: map[string]any{
			"session_id": resolved.sessionID,
			"branch":     plan.Branch,
			"agent":      opts.AgentName,
		},
	}
	err = session.AppendEvent(ledgerPath, event)
	if err != nil {
		return "", "", fmt.Errorf("append create event: %w", err)
	}
	return statePath, ledgerPath, nil
}

func buildInitialState(opts CreateOptions, resolved resolvedNames, plan git.WorktreePlan) session.State {
	return session.State{
		SchemaVersion: 1,
		Session: session.Info{
			ID:           resolved.sessionID,
			Name:         resolved.name,
			Status:       string(Active),
			CreatedAt:    resolved.now,
			ApprovalMode: approvalModeToString(opts.Control.ApprovalMode),
			MaxAgents:    opts.Control.MaxAgents,
		},
		Worktree: session.WorktreeState{
			Path:       plan.Path,
			Branch:     plan.Branch,
			BaseBranch: opts.FromBranch,
			GitRoot:    opts.GitRoot,
			RepoSlug:   resolved.repoSlug,
		},
		Execution: session.ExecutionState{
			Mode:          executionLocal,
			Multiplexer:   "tmux",
			TmuxSession:   resolved.tmuxSession,
			RemoteControl: opts.Control.RemoteControl,
			// ADR-062: capture resolved sandbox resource profile so resume --respawn
			// reproduces the same VM shape even if the repo config changes later.
			SandboxResourceProfile:     opts.SandboxResources.ProfileName,
			SandboxResourceVCPU:        opts.SandboxResources.VCPU,
			SandboxResourceRAMGB:       opts.SandboxResources.RAMGB,
			SandboxResourceStorageSize: opts.SandboxResources.StorageSize,
			SandboxResourceGPUCount:    opts.SandboxResources.GPUCount,
			SandboxResourceImage:       opts.SandboxResources.Image,
			SandboxResourceHypervisor:  opts.SandboxResources.Hypervisor,
			SandboxManagedGroup:        opts.SandboxGroup,
		},
		Versions: session.VersionsState{
			AF:            version.Version,
			AgentVersions: map[string]string{},
		},
		Agents: []session.AgentState{{
			Slot:       primaryAgentSlot,
			Provider:   opts.AgentName,
			Status:     string(Active),
			CreatedAt:  resolved.now,
			SessionIDs: []string{},
		}},
	}
}

func writeWorkstreamNote(ctx context.Context, store obsidian.Store, resolved resolvedNames, plan git.WorktreePlan, opts CreateOptions) (string, error) {
	if store == nil || opts.NotesDir == "" {
		return "", nil
	}
	notePath := filepath.Join(opts.NotesDir, resolved.name+".md")
	note := obsidian.Note{
		Frontmatter: obsidian.Frontmatter{
			Schema:     1,
			Session:    resolved.name,
			Repo:       resolved.repoSlug,
			Status:     string(Active),
			Branch:     plan.Branch,
			BaseBranch: opts.FromBranch,
			StartedAt:  resolved.now,
			Agents: []obsidian.Agent{{
				Slot:     primaryAgentSlot,
				Provider: opts.AgentName,
				Status:   string(Active),
			}},
		},
		Body: workstreamNoteBody(resolved, plan, opts.AgentName),
	}
	err := store.Write(ctx, notePath, note)
	if err != nil {
		return "", fmt.Errorf("write obsidian note: %w", err)
	}
	return notePath, nil
}

func workstreamNoteBody(resolved resolvedNames, plan git.WorktreePlan, agentName string) string {
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(resolved.name)
	b.WriteString("\n\n")
	b.WriteString("- Branch: `")
	b.WriteString(plan.Branch)
	b.WriteString("`\n")
	b.WriteString("- Worktree: `")
	b.WriteString(plan.Path)
	b.WriteString("`\n")
	if agentName != "" {
		b.WriteString("- Primary agent: `")
		b.WriteString(agentName)
		b.WriteString("`\n")
	}
	b.WriteString("\n## Notes\n\n")
	return b.String()
}

func launchTmuxAndAgent(ctx context.Context, deps CreateDeps, resolved resolvedNames, plan git.WorktreePlan, opts CreateOptions) (string, error) {
	if opts.Bare || deps.Mux == nil {
		return "", nil
	}
	err := deps.Mux.CreateSession(ctx, resolved.tmuxSession, plan.Path)
	if err != nil {
		return "", fmt.Errorf("tmux create-session: %w", err)
	}
	err = deps.Mux.SetEnv(ctx, resolved.tmuxSession, "AF_SESSION", resolved.name)
	if err != nil {
		return "", fmt.Errorf("tmux set AF_SESSION: %w", err)
	}

	if deps.Agent == nil {
		return resolved.tmuxSession, nil
	}
	launchArgv := deps.Agent.LaunchCmd(agent.LaunchOpts{
		Cwd:       plan.Path,
		SessionID: resolved.sessionID,
	})
	if len(launchArgv) == 0 {
		return resolved.tmuxSession, nil
	}
	err = deps.Mux.SendKeys(ctx, resolved.tmuxSession, "", strings.Join(launchArgv, " ")+"\n")
	if err != nil {
		return "", fmt.Errorf("tmux send-keys agent launch: %w", err)
	}
	return resolved.tmuxSession, nil
}
