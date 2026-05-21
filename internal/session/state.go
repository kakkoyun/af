package session

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"golang.org/x/sys/unix"
)

const (
	currentStateSchemaVersion = 1
	directoryPerm             = 0o750
	filePerm                  = 0o600
	eventBaseFields           = 2
)

var (
	// ErrSchemaTooNew reports a state.toml schema newer than this binary supports.
	ErrSchemaTooNew = errors.New("state schema too new")
	// ErrNoCurrentWorkstream reports that discovery found no current workstream.
	ErrNoCurrentWorkstream   = errors.New("no current workstream")
	errEmptyLedger           = errors.New("empty ledger")
	errEventTimestampMissing = errors.New("ledger event timestamp is required")
	errEventTypeMissing      = errors.New("ledger event type is required")
)

// State is the v1 state.toml schema for one workstream.
type State struct {
	Session       Info           `toml:"session"`
	Stack         StackState     `toml:"stack"`
	Versions      VersionsState  `toml:"versions"`
	Execution     ExecutionState `toml:"execution"`
	Worktree      WorktreeState  `toml:"worktree"`
	PR            PRState        `toml:"pr"`
	Agents        []AgentState   `toml:"agents"`
	SchemaVersion int            `toml:"schema_version"`
}

// Info stores workstream identity and lifecycle status.
type Info struct {
	CreatedAt    time.Time  `toml:"created_at"`
	SuspendedAt  *time.Time `toml:"suspended_at,omitempty"`
	Name         string     `toml:"name"`
	ID           string     `toml:"id"`
	Status       string     `toml:"status"`
	ApprovalMode string     `toml:"approval_mode,omitempty"`
	MaxAgents    int        `toml:"max_agents,omitempty"`
}

// WorktreeState stores the primary git worktree metadata.
type WorktreeState struct {
	Path       string `toml:"path"`
	Branch     string `toml:"branch"`
	BaseBranch string `toml:"base_branch"`
	GitRoot    string `toml:"git_root"`
	RepoSlug   string `toml:"repo_slug"`
}

// ExecutionState stores where and how the workstream is running.
type ExecutionState struct {
	Mode            string `toml:"mode"`
	Multiplexer     string `toml:"multiplexer"`
	TmuxSession     string `toml:"tmux_session"`
	SSHHost         string `toml:"ssh_host"`
	RemotePath      string `toml:"remote_path"`
	SandboxProvider string `toml:"sandbox_provider"`
	SandboxID       string `toml:"sandbox_id"`
	RemoteControl   string `toml:"remote_control,omitempty"`
}

// AgentState stores one agent slot in a workstream.
type AgentState struct {
	CreatedAt     time.Time  `toml:"created_at"`
	LastResumedAt *time.Time `toml:"last_resumed_at,omitempty"`
	Slot          string     `toml:"slot"`
	Provider      string     `toml:"provider"`
	Pane          string     `toml:"pane"`
	Status        string     `toml:"status"`
	SubWorktree   string     `toml:"sub_worktree"`
	SubBranch     string     `toml:"sub_branch"`
	SessionIDs    []string   `toml:"session_ids"`
}

// PRState stores pull-request metadata associated with a workstream.
type PRState struct {
	URL    string `toml:"url"`
	State  string `toml:"state"`
	Number int    `toml:"number"`
}

// StackState stores stack parent metadata for a workstream.
type StackState struct {
	LinkedAt      *time.Time `toml:"linked_at,omitempty"`
	ParentSession string     `toml:"parent_session"`
	ParentBranch  string     `toml:"parent_branch"`
}

// VersionsState records tool versions captured when the workstream is created.
type VersionsState struct {
	AgentVersions map[string]string `toml:"agent_versions"`
	AF            string            `toml:"af"`
}

// Event is one append-only ledger.jsonl record.
type Event struct {
	Timestamp time.Time
	Fields    map[string]any
	Type      string
}

// LockMode selects shared or exclusive flock semantics.
type LockMode int

const (
	// LockShared acquires a shared read lock.
	LockShared LockMode = iota
	// LockExclusive acquires an exclusive write lock.
	LockExclusive
)

// Lock is an acquired flock-backed lock file.
type Lock struct {
	file *os.File
}

// DiscoverOptions controls current-workstream discovery.
type DiscoverOptions struct {
	SessionName string
	SessionsDir string
	Cwd         string
	TmuxSession string
}

// ReadState loads and validates a state.toml file.
func ReadState(path string) (State, error) {
	var state State
	_, err := toml.DecodeFile(path, &state)
	if err != nil {
		return State{}, fmt.Errorf("read state %s: %w", path, err)
	}
	if state.SchemaVersion > currentStateSchemaVersion {
		return State{}, fmt.Errorf("read state %s: schema_version %d is newer than supported %d: %w", path, state.SchemaVersion, currentStateSchemaVersion, ErrSchemaTooNew)
	}

	return state, nil
}

// WriteState atomically writes state to path.
func WriteState(path string, state State) error {
	if state.SchemaVersion == 0 {
		state.SchemaVersion = currentStateSchemaVersion
	}
	err := os.MkdirAll(filepath.Dir(path), directoryPerm)
	if err != nil {
		return fmt.Errorf("create state directory %s: %w", filepath.Dir(path), err)
	}

	tmpPath := path + ".tmp"
	file, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, filePerm) //nolint:gosec // State paths are controlled by af's state store and tests.
	if err != nil {
		return fmt.Errorf("create temporary state %s: %w", tmpPath, err)
	}
	encodeErr := toml.NewEncoder(file).Encode(state)
	if encodeErr != nil {
		return closeAfterError(file, fmt.Errorf("encode state %s: %w", tmpPath, encodeErr))
	}
	err = file.Sync()
	if err != nil {
		return closeAfterError(file, fmt.Errorf("sync state %s: %w", tmpPath, err))
	}
	err = closeNamedFile(file, "state", tmpPath)
	if err != nil {
		return err
	}
	err = os.Rename(tmpPath, path)
	if err != nil {
		return fmt.Errorf("replace state %s: %w", path, err)
	}
	err = syncDir(filepath.Dir(path))
	if err != nil {
		return err
	}

	return nil
}

// AppendEvent appends one newline-terminated event to ledgerPath.
func AppendEvent(ledgerPath string, event Event) error {
	err := os.MkdirAll(filepath.Dir(ledgerPath), directoryPerm)
	if err != nil {
		return fmt.Errorf("create ledger directory %s: %w", filepath.Dir(ledgerPath), err)
	}
	line, err := marshalEvent(event)
	if err != nil {
		return err
	}

	file, err := os.OpenFile(ledgerPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY|unix.O_SYNC, filePerm) //nolint:gosec // Ledger paths are controlled by af's state store and tests.
	if err != nil {
		return fmt.Errorf("open ledger %s: %w", ledgerPath, err)
	}
	_, err = file.Write(append(line, '\n'))
	if err != nil {
		return closeAfterError(file, fmt.Errorf("append ledger %s: %w", ledgerPath, err))
	}

	return closeNamedFile(file, "ledger", ledgerPath)
}

// LastTouchedAt returns the timestamp from the last ledger record.
func LastTouchedAt(ledgerPath string) (time.Time, error) {
	file, err := os.Open(ledgerPath) //nolint:gosec // Ledger paths are controlled by af's state store and tests.
	if err != nil {
		return time.Time{}, fmt.Errorf("open ledger %s: %w", ledgerPath, err)
	}

	var last string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			last = line
		}
	}
	err = scanner.Err()
	if err != nil {
		return time.Time{}, closeAfterError(file, fmt.Errorf("read ledger %s: %w", ledgerPath, err))
	}
	if last == "" {
		return time.Time{}, closeAfterError(file, fmt.Errorf("read ledger %s: %w", ledgerPath, errEmptyLedger))
	}

	var record struct {
		Timestamp time.Time `json:"ts"`
	}
	err = json.Unmarshal([]byte(last), &record)
	if err != nil {
		return time.Time{}, closeAfterError(file, fmt.Errorf("decode last ledger record %s: %w", ledgerPath, err))
	}
	err = closeNamedFile(file, "ledger", ledgerPath)
	if err != nil {
		return time.Time{}, err
	}

	return record.Timestamp, nil
}

// RepoSlugFromRemote extracts owner/name for GitHub remotes.
func RepoSlugFromRemote(remote string) string {
	candidate := remote
	if strings.HasPrefix(remote, "git@github.com:") {
		candidate = "ssh://git@github.com/" + strings.TrimPrefix(remote, "git@github.com:")
	}

	parsed, err := url.Parse(candidate)
	if err != nil || parsed.Hostname() != "github.com" {
		return ""
	}
	parts := strings.Split(strings.Trim(strings.TrimSuffix(parsed.Path, ".git"), "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}

	return parts[0] + "/" + parts[1]
}

// DiscoverStatePath resolves the state.toml path for the current workstream.
func DiscoverStatePath(opts DiscoverOptions) (string, error) {
	if opts.SessionName != "" {
		return filepath.Join(opts.SessionsDir, opts.SessionName, "state.toml"), nil
	}
	if opts.Cwd != "" {
		path, err := discoverFromCwd(opts.Cwd)
		if err == nil {
			return path, nil
		}
		if !errors.Is(err, ErrNoCurrentWorkstream) {
			return "", err
		}
	}
	if opts.TmuxSession != "" {
		path := filepath.Join(opts.SessionsDir, opts.TmuxSession, "state.toml")
		_, err := os.Stat(path)
		if err == nil {
			return path, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("stat tmux session state %s: %w", path, err)
		}
	}

	return "", ErrNoCurrentWorkstream
}

// LockFile acquires a flock-backed lock file.
func LockFile(path string, mode LockMode) (*Lock, error) {
	err := os.MkdirAll(filepath.Dir(path), directoryPerm)
	if err != nil {
		return nil, fmt.Errorf("create lock directory %s: %w", filepath.Dir(path), err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, filePerm) //nolint:gosec // Lock paths are controlled by af's state store and tests.
	if err != nil {
		return nil, fmt.Errorf("open lock %s: %w", path, err)
	}

	operation := unix.LOCK_SH
	if mode == LockExclusive {
		operation = unix.LOCK_EX
	}
	err = unix.Flock(int(file.Fd()), operation)
	if err != nil {
		return nil, closeAfterError(file, fmt.Errorf("lock %s: %w", path, err))
	}

	return &Lock{file: file}, nil
}

// Unlock releases the lock file.
func (lock *Lock) Unlock() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	err := unix.Flock(int(lock.file.Fd()), unix.LOCK_UN)
	if err != nil {
		return closeAfterError(lock.file, fmt.Errorf("unlock %s: %w", lock.file.Name(), err))
	}
	err = closeNamedFile(lock.file, "lock", lock.file.Name())
	if err != nil {
		return err
	}
	lock.file = nil

	return nil
}

func marshalEvent(event Event) ([]byte, error) {
	if event.Timestamp.IsZero() {
		return nil, errEventTimestampMissing
	}
	if event.Type == "" {
		return nil, errEventTypeMissing
	}
	record := make(map[string]any, len(event.Fields)+eventBaseFields)
	for key, value := range event.Fields {
		record[key] = value
	}
	record["ts"] = event.Timestamp.UTC().Format(time.RFC3339Nano)
	record["event"] = event.Type

	line, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("encode ledger event: %w", err)
	}

	return line, nil
}

func discoverFromCwd(cwd string) (string, error) {
	current, err := filepath.Abs(cwd)
	if err != nil {
		return "", fmt.Errorf("resolve cwd %s: %w", cwd, err)
	}
	for {
		candidate := filepath.Join(current, ".af", "state.toml")
		target, ok, err := existingSymlinkTarget(candidate)
		if err != nil {
			return "", err
		}
		if ok {
			return target, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	return "", ErrNoCurrentWorkstream
}

func existingSymlinkTarget(path string) (string, bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("stat discovery symlink %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return "", false, nil
	}
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", false, fmt.Errorf("resolve discovery symlink %s: %w", path, err)
	}

	return target, true, nil
}

func closeAfterError(file *os.File, original error) error {
	closeErr := file.Close()
	if closeErr != nil {
		return errors.Join(original, fmt.Errorf("close %s: %w", file.Name(), closeErr))
	}

	return original
}

func closeNamedFile(file *os.File, kind, path string) error {
	err := file.Close()
	if err != nil {
		return fmt.Errorf("close %s %s: %w", kind, path, err)
	}

	return nil
}

func syncDir(path string) error {
	dir, err := os.Open(path) //nolint:gosec // Directory paths are controlled by af's state store and tests.
	if err != nil {
		return fmt.Errorf("open state directory %s: %w", path, err)
	}
	err = dir.Sync()
	if err != nil {
		return closeAfterError(dir, fmt.Errorf("sync state directory %s: %w", path, err))
	}

	return closeNamedFile(dir, "state directory", path)
}
