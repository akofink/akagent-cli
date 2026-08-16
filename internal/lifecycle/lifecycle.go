// Package lifecycle coordinates durable local task state with tmux observations.
package lifecycle

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/akofink/akagent-cli/internal/credential"
	"github.com/akofink/akagent-cli/internal/store"
)

const DefaultHeartbeatTimeout = 2 * time.Minute

const (
	ObservationFresh         = "fresh"
	ObservationStale         = "stale"
	ObservationMissing       = "missing"
	ObservationReplaced      = "replaced"
	ObservationContradictory = "contradictory"
	ObservationUnavailable   = "unavailable"
)

// TmuxProcess is the non-secret identity observed for one task pane.
// PID alone is deliberately insufficient because it can be reused.
type TmuxProcess struct {
	WindowID  string
	PaneID    string
	PID       int
	StartTime uint64
}

type TmuxObservation struct {
	Available bool
	Processes []TmuxProcess
}

// GitStatus contains observations from one Git worktree without command output.
type GitStatus struct {
	Exists    bool
	Root      string
	CommonDir string
	Branch    string
	Head      string
	Dirty     bool
	Untracked bool
}

// Git provides the small Git surface needed by task lifecycle operations.
type Git interface {
	RepositoryRoot(path string) (string, error)
	CommonDir(path string) (string, error)
	Head(path string) (string, error)
	Branch(path string) (string, error)
	Resolve(path, revision string) (string, error)
	IsAncestor(path, ancestor, descendant string) (bool, error)
	Status(path string) (GitStatus, error)
	AddWorktree(repository, path, branch, base string) error
	RemoveWorktree(repository, path string) error
}

type Tmux interface {
	Start(taskID, branch string) (TmuxProcess, error)
	Observe(taskID string) (TmuxObservation, error)
	Attach(taskID, windowID string) error
	Stop(taskID string) error
}

// ManagedTmux starts a task with an explicit launch configuration. The legacy
// Tmux.Start path remains available for tasks that intentionally launch a shell.
type ManagedTmux interface {
	StartManaged(taskID, branch string, launch store.LaunchConfig) (TmuxProcess, error)
}

// ExecutionTmux is the tool-neutral tmux surface for independently managed
// executions. The task and execution IDs are both verified in tmux metadata.
type ExecutionTmux interface {
	StartExecution(executionID, taskID, label, directory, command string, arguments []string) (TmuxProcess, error)
	ObserveExecution(executionID, taskID string) (TmuxObservation, error)
	AttachExecution(executionID, taskID, windowID string) error
	StopExecution(executionID, taskID string) error
	CaptureExecution(executionID, taskID string) (string, error)
}

// ExecutionStateTmux publishes the shared tmux status option for an execution.
// The empty state clears @agent_state rather than displaying an active value.
type ExecutionStateTmux interface {
	SetExecutionState(executionID, taskID, state string) error
}

type Manager struct {
	Store              *store.Store
	Tmux               Tmux
	Git                Git
	Credentials        func() (*credential.Manifest, error)
	Checker            *credential.Checker
	Now                func() time.Time
	HeartbeatTimeout   time.Duration
	GitFacts           func(store.Manifest) (store.GitFacts, error)
	TerminalHistory    func(string) (string, error)
	CleanupWorktree    func(store.Manifest, store.GitFacts) error
	CleanupCredentials func(store.Manifest) error
	ResolveAgent       func(string) (string, error)
	ExecAgent          func(string, []string, []string) error
	operationMu        sync.Mutex
}

type StartRequest struct {
	ID              string
	Title           string
	Repository      string
	Branch          string
	BaseRevision    string
	WorktreePath    string
	Requirements    []string
	Optional        []string
	Agent           string
	PromptReference string
	WorkingContext  string
}

// CreateRequest contains only durable task and Git resource inputs. It never
// selects or starts an execution.
type CreateRequest struct {
	ID           string
	Title        string
	Repository   string
	Branch       string
	BaseRevision string
	WorktreePath string
	Requirements []string
	Optional     []string
}

// ResourceRequest contains Git inputs and optional non-secret delivery
// metadata. Resource IDs are supplied by the caller so retries remain
// idempotent.
type ResourceRequest struct {
	ID           string
	Repository   string
	Branch       string
	BaseRevision string
	WorktreePath string
	Metadata     map[string]string
	ExternalURLs []string
}

// ResourceUpdateRequest changes only the mutable, provider-neutral metadata of
// an existing resource. Git ownership inputs remain immutable.
type ResourceUpdateRequest struct {
	Metadata     map[string]string
	ExternalURLs []string
}

// LaunchRequest preserves the compatibility task launch surface. New callers
// should create and launch independent Execution records instead.
type LaunchRequest struct {
	ExecutionID     string
	Label           string
	Target          string
	ResourceID      string
	PromptReference string
	WorkingContext  string
}

type StartResult struct {
	Manifest store.Manifest
	Created  bool
}

func New(state *store.Store) *Manager {
	manager := &Manager{
		Store: state, Tmux: commandTmux{}, Git: commandGit{}, Credentials: func() (*credential.Manifest, error) {
			return credential.Load(credential.ConfigPath())
		}, Checker: credential.NewChecker(), Now: time.Now, HeartbeatTimeout: DefaultHeartbeatTimeout,
		ResolveAgent:       exec.LookPath,
		TerminalHistory:    func(string) (string, error) { return "", nil },
		CleanupCredentials: func(store.Manifest) error { return nil },
		ExecAgent:          syscall.Exec,
	}
	manager.GitFacts = manager.inspectGitFacts
	manager.CleanupWorktree = manager.removeWorktree
	return manager
}

func (m *Manager) RegisterRepository(name, path, policy string, worktreeRoots ...string) (store.Repository, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return store.Repository{}, fmt.Errorf("resolve repository path")
	}
	root, err := m.Git.RepositoryRoot(abs)
	if err != nil {
		return store.Repository{}, fmt.Errorf("repository path is not a Git worktree")
	}
	root, err = filepath.Abs(root)
	if err != nil || root != abs {
		return store.Repository{}, fmt.Errorf("repository path must be the Git worktree root")
	}
	if policy == "" {
		policy = discoverPolicy(abs)
	}
	if policy != "worktree" && policy != "direct" {
		return store.Repository{}, fmt.Errorf("repository policy must be worktree or direct")
	}
	worktreeRoot, err := configuredWorktreeRoot(worktreeRoots)
	if err != nil {
		return store.Repository{}, err
	}
	if worktreeRoot != "" && policy != "worktree" {
		return store.Repository{}, &store.Error{Kind: store.KindUsage, Message: "A worktree root requires the worktree repository policy", Recovery: "Register the repository with `--policy worktree`"}
	}
	if policy == "worktree" && worktreeRoot == "" {
		worktreeRoot = derivedWorktreeRoot(abs, name)
	}
	repository := store.Repository{Name: name, Path: abs, Policy: policy, WorktreeRoot: worktreeRoot, Instructions: discoverInstructions(abs)}
	if worktreeRoot == derivedWorktreeRoot(abs, name) && (len(worktreeRoots) == 0 || worktreeRoots[0] == "") {
		if existing, existingErr := m.Store.ReadRepository(name); existingErr == nil && existing.Policy == "worktree" && existing.Path == abs && existing.WorktreeRoot == "" {
			repository.WorktreeRoot = ""
		}
	}
	if _, err := m.Store.RegisterRepository(repository); err != nil {
		return store.Repository{}, err
	}
	return repository, nil
}

// ListRepositories returns registrations in deterministic name order.
func (m *Manager) ListRepositories() ([]store.Repository, error) {
	names, err := m.Store.RepositoryNames()
	if err != nil {
		return nil, err
	}
	repositories := make([]store.Repository, 0, len(names))
	for _, name := range names {
		repository, err := m.Store.ReadRepository(name)
		if err != nil {
			return nil, err
		}
		repositories = append(repositories, repository)
	}
	return repositories, nil
}

// InspectRepository returns one durable repository registration.
func (m *Manager) InspectRepository(name string) (store.Repository, error) {
	return m.Store.ReadRepository(name)
}

// UpdateRepository changes only registration metadata. It never writes to the
// Git checkout or any task record.
func (m *Manager) UpdateRepository(name, path, policy string, worktreeRoots ...string) (store.Repository, error) {
	if _, err := m.Store.ReadRepository(name); err != nil {
		return store.Repository{}, err
	}
	if path == "" && policy == "" && len(worktreeRoots) == 0 {
		return store.Repository{}, fmt.Errorf("repository path, policy, or worktree root update is required")
	}
	configuredRoot, err := configuredWorktreeRoot(worktreeRoots)
	if err != nil {
		return store.Repository{}, err
	}
	if configuredRoot != "" && policy == "direct" {
		return store.Repository{}, &store.Error{Kind: store.KindUsage, Message: "A worktree root requires the worktree repository policy", Recovery: "Update the repository with `--policy worktree`"}
	}
	var resolvedPath string
	if path != "" {
		abs, err := filepath.Abs(path)
		if err != nil {
			return store.Repository{}, fmt.Errorf("resolve repository path")
		}
		if err := m.validateRepositoryPath(abs); err != nil {
			return store.Repository{}, err
		}
		resolvedPath = abs
	}
	return m.Store.UpdateRepository(name, func(repository *store.Repository) error {
		if configuredRoot != "" && repository.Policy == "direct" && policy != "worktree" {
			return &store.Error{Kind: store.KindUsage, Message: "A worktree root requires the worktree repository policy", Recovery: "Update the repository with `--policy worktree`"}
		}
		oldPath, oldPolicy, oldRoot := repository.Path, repository.Policy, repository.WorktreeRoot
		if resolvedPath != "" {
			repository.Path = resolvedPath
		}
		if policy != "" {
			if policy != "worktree" && policy != "direct" {
				return fmt.Errorf("repository policy must be worktree or direct")
			}
			repository.Policy = policy
		}
		if repository.Policy == "direct" {
			repository.WorktreeRoot = ""
		} else if configuredRoot != "" {
			repository.WorktreeRoot = configuredRoot
		} else if oldPolicy != "worktree" || oldRoot == "" || oldRoot == derivedWorktreeRoot(oldPath, repository.Name) {
			repository.WorktreeRoot = derivedWorktreeRoot(repository.Path, repository.Name)
		}
		if resolvedPath != "" || policy != "" {
			repository.Instructions = discoverInstructions(repository.Path)
		}
		return nil
	})
}

// UnregisterRepository removes only the registration record. The registered
// checkout, task files, and Git metadata remain untouched.
func (m *Manager) UnregisterRepository(name string) error {
	return m.Store.UnregisterRepository(name)
}

func (m *Manager) validateRepositoryPath(path string) error {
	root, err := m.Git.RepositoryRoot(path)
	if err != nil {
		return fmt.Errorf("repository path is not a Git worktree")
	}
	root, err = filepath.Abs(root)
	if err != nil || root != path {
		return fmt.Errorf("repository path must be the Git worktree root")
	}
	return nil
}

func discoverPolicy(path string) string {
	if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
		return "worktree"
	}
	return "direct"
}

func discoverInstructions(path string) []string {
	var reversed []string
	for current := path; ; current = filepath.Dir(current) {
		candidate := filepath.Join(current, "AGENTS.md")
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			reversed = append(reversed, candidate)
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	if len(reversed) == 0 {
		return nil
	}
	instructions := make([]string, len(reversed))
	for i := range reversed {
		instructions[i] = reversed[len(reversed)-1-i]
	}
	return instructions
}

// Create durably records task intent and, when repository inputs are supplied,
// the compatibility initial Git resource. It never starts a process.
func (m *Manager) Create(request CreateRequest) (StartResult, error) {
	if request.ID == "" || request.Title == "" {
		return StartResult{}, fmt.Errorf("task ID and title are required")
	}
	request.Requirements = unique(request.Requirements)
	request.Optional = unique(request.Optional)
	warnings, err := m.checkCredentials(request.Requirements, request.Optional)
	if err != nil {
		return StartResult{}, err
	}
	if request.Repository == "" {
		if request.Branch != "" || request.BaseRevision != "" || request.WorktreePath != "" {
			return StartResult{}, &store.Error{Kind: store.KindUsage, Message: "Git inputs require a repository resource", Recovery: "Create the task first, then use `akagent task resource create`"}
		}
		manifest := store.Manifest{Title: request.Title, Worker: "local", Lifecycle: "created", Condition: "none", HeartbeatAt: m.now(), Requirements: strings.Join(request.Requirements, ","), Warnings: strings.Join(warnings, "; ")}
		created, existing, err := m.Store.CreateManifest(request.ID, manifest)
		if err != nil {
			return StartResult{}, err
		}
		if !created {
			if existing.Title != request.Title || existing.Worker != "local" || existing.Requirements != manifest.Requirements {
				return StartResult{}, fmt.Errorf("task inputs conflict with the existing task")
			}
			return StartResult{Manifest: existing}, nil
		}
		if _, err := m.Store.AppendEvent(request.ID, store.Event{Operation: "create", Outcome: "intent"}); err != nil {
			return StartResult{}, err
		}
		return StartResult{Manifest: manifest, Created: true}, nil
	}
	repository, err := m.Store.ReadRepository(request.Repository)
	if err != nil {
		return StartResult{}, err
	}
	var result StartResult
	err = m.Store.WithRepositoryLock(repository.Name, func() error {
		existingEnvelope, readErr := m.Store.ReadManifest(request.ID)
		if readErr == nil {
			existing, decodeErr := existingEnvelope.DecodeManifest()
			if decodeErr != nil {
				return decodeErr
			}
			if err := m.validateExistingCreate(request, existing); err != nil {
				return err
			}
			if migrateLegacyCreate(&existing) {
				if err := m.Store.WriteManifest(request.ID, existing); err != nil {
					return err
				}
				if _, err := m.Store.AppendEvent(request.ID, store.Event{Operation: "migrate", Outcome: "created", Detail: "legacy launch state detached from task creation"}); err != nil {
					return err
				}
			}
			if existing.Lifecycle == "created" {
				if err := m.ensureWorktree(request.ID, repository, &existing); err != nil {
					return err
				}
			}
			if err := m.ensureLegacyResource(request.ID, &existing); err != nil {
				return err
			}
			result = StartResult{Manifest: existing}
			return nil
		}
		if !store.IsKind(readErr, store.KindNotFound) {
			return readErr
		}
		branch, base, worktree, err := m.startInputs(StartRequest{ID: request.ID, Branch: request.Branch, BaseRevision: request.BaseRevision, WorktreePath: request.WorktreePath}, repository)
		if err != nil {
			return err
		}
		manifest := store.Manifest{Title: request.Title, Worker: "local", Repository: request.Repository, Branch: branch, BaseRevision: base, WorktreePath: worktree, Lifecycle: "created", Condition: "none", HeartbeatAt: m.now(), Requirements: strings.Join(request.Requirements, ","), Warnings: strings.Join(warnings, "; ")}
		created, existing, err := m.Store.CreateManifest(request.ID, manifest)
		if err != nil {
			return err
		}
		if !created {
			if !sameCreate(existing, manifest) {
				return fmt.Errorf("task inputs conflict with the existing task")
			}
			result = StartResult{Manifest: existing}
			return nil
		}
		if _, err := m.Store.AppendEvent(request.ID, store.Event{Operation: "create", Outcome: "intent"}); err != nil {
			return err
		}
		if err := m.ensureWorktree(request.ID, repository, &manifest); err != nil {
			_, _ = m.Store.AppendEvent(request.ID, store.Event{Operation: "create", Outcome: "failed", Detail: "worktree"})
			return err
		}
		if err := m.ensureLegacyResource(request.ID, &manifest); err != nil {
			return err
		}
		result = StartResult{Manifest: manifest, Created: true}
		return nil
	})
	return result, err
}

// LaunchExecution preserves the direct shell compatibility shortcut for an
// existing task. Optional integrations use the generic execution methods.
func (m *Manager) LaunchExecution(id string, request LaunchRequest) (store.Manifest, error) {
	if id == "" {
		return store.Manifest{}, fmt.Errorf("task ID is required")
	}
	if request.Target != "shell" {
		return store.Manifest{}, &store.Error{Kind: store.KindUsage, Message: "core execution launch supports shell only", Recovery: "Use `akagent task execution launch` or select an optional integration"}
	}
	manifest, err := m.manifest(id)
	if err != nil {
		return store.Manifest{}, err
	}
	if request.ExecutionID != "" && manifest.ExecutionIDs != "" && manifest.ExecutionIDs != request.ExecutionID {
		return store.Manifest{}, &store.Error{Kind: store.KindConflict, Message: "execution ID conflicts with the existing task launch", Recovery: fmt.Sprintf("Inspect task %s and retry with its existing execution ID", id)}
	}
	if manifest.Lifecycle == "running" {
		if manifest.Launch != nil && manifest.Launch.Target == "shell" {
			return manifest, nil
		}
		return store.Manifest{}, fmt.Errorf("task already has a different running execution")
	}
	if manifest.Lifecycle == "stopped" || manifest.Lifecycle == "finished" {
		return store.Manifest{}, fmt.Errorf("task cannot launch after it is %s", manifest.Lifecycle)
	}
	if manifest.WorktreePath == "" {
		resourceID := request.ResourceID
		if resourceID == "" {
			ids := strings.Split(manifest.ResourceIDs, ",")
			if len(ids) == 1 && ids[0] != "" {
				resourceID = ids[0]
			}
		}
		if resourceID == "" {
			return store.Manifest{}, &store.Error{Kind: store.KindConflict, Message: "task launch requires a selected resource", Recovery: "Create a resource, then retry with `--resource <resource-id>`"}
		}
		resource, resourceErr := m.InspectResource(id, resourceID)
		if resourceErr != nil {
			return store.Manifest{}, resourceErr
		}
		manifest.Repository, manifest.Branch, manifest.BaseRevision, manifest.WorktreeBaseRevision, manifest.WorktreePath = resource.Repository, resource.Branch, resource.BaseRevision, resource.WorktreeBaseRevision, resource.WorktreePath
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	launch := &store.LaunchConfig{Target: "shell", Command: shell, WorkingDirectory: manifest.WorktreePath}
	if request.PromptReference != "" || request.WorkingContext != "" {
		return store.Manifest{}, fmt.Errorf("shell execution does not accept prompt or context")
	}
	if manifest.Launch != nil && !sameRequestedLaunch(manifest.Launch, launch) {
		return store.Manifest{}, fmt.Errorf("execution inputs conflict with the existing task")
	}
	if manifest.Launch == nil {
		manifest.Launch = launch
	}
	manifest.Lifecycle = "starting"
	manifest.Observation = ObservationMissing
	manifest.HeartbeatAt = m.now()
	if err := m.Store.WriteManifest(id, manifest); err != nil {
		return store.Manifest{}, err
	}
	if _, err := m.Store.AppendEvent(id, store.Event{Operation: "launch", Outcome: "intent", Detail: request.Target}); err != nil {
		return store.Manifest{}, err
	}
	if err := m.ensureStarted(id, &manifest); err != nil {
		return store.Manifest{}, err
	}
	if err := m.ensureLegacyExecutionWithID(id, request.ExecutionID, request.Label); err != nil {
		return store.Manifest{}, err
	}
	return manifest, nil
}

func (m *Manager) checkLaunchCredentials(manifest store.Manifest) error {
	if manifest.Requirements == "" {
		return nil
	}
	_, err := m.checkCredentials(splitRequirements(manifest.Requirements), nil)
	return err
}

func (m *Manager) Start(request StartRequest) (StartResult, error) {
	if request.ID == "" || request.Title == "" || request.Repository == "" {
		return StartResult{}, fmt.Errorf("task ID, title, and repository are required")
	}
	repository, err := m.Store.ReadRepository(request.Repository)
	if err != nil {
		return StartResult{}, err
	}
	request.Requirements = unique(request.Requirements)
	request.Optional = unique(request.Optional)
	launch, err := m.prepareLaunch(request)
	if err != nil {
		return StartResult{}, err
	}
	warnings, err := m.checkCredentials(request.Requirements, request.Optional)
	if err != nil {
		return StartResult{}, err
	}
	var result StartResult
	err = m.Store.WithRepositoryLock(repository.Name, func() error {
		existingEnvelope, readErr := m.Store.ReadManifest(request.ID)
		if readErr == nil {
			existing, decodeErr := existingEnvelope.DecodeManifest()
			if decodeErr != nil {
				return decodeErr
			}
			if err := m.validateExistingStart(request, repository, existing, launch); err != nil {
				return err
			}
			if existing.Lifecycle == "starting" {
				if err := m.ensureWorktree(request.ID, repository, &existing); err != nil {
					return err
				}
				if err := m.ensureStarted(request.ID, &existing); err != nil {
					return err
				}
			}
			result = StartResult{Manifest: existing}
			return nil
		}
		if !store.IsKind(readErr, store.KindNotFound) {
			return readErr
		}
		branch, base, worktree, err := m.startInputs(request, repository)
		if err != nil {
			return err
		}
		if launch != nil {
			launch.WorkingDirectory = worktree
		}
		manifest := store.Manifest{Title: request.Title, Worker: "local", Repository: request.Repository, Branch: branch, BaseRevision: base, WorktreePath: worktree, Lifecycle: "starting", Condition: "none", HeartbeatAt: m.now(), Requirements: strings.Join(request.Requirements, ","), Warnings: strings.Join(warnings, "; "), Launch: launch}
		created, existing, err := m.Store.CreateManifest(request.ID, manifest)
		if err != nil {
			return err
		}
		if !created {
			if !sameStart(existing, manifest) {
				return fmt.Errorf("task inputs conflict with the existing task")
			}
			result = StartResult{Manifest: existing}
			return nil
		}
		if _, err := m.Store.AppendEvent(request.ID, store.Event{Operation: "start", Outcome: "intent"}); err != nil {
			return err
		}
		if err := m.ensureWorktree(request.ID, repository, &manifest); err != nil {
			_, _ = m.Store.AppendEvent(request.ID, store.Event{Operation: "start", Outcome: "failed", Detail: "worktree"})
			return err
		}
		if err := m.ensureStarted(request.ID, &manifest); err != nil {
			return err
		}
		result = StartResult{Manifest: manifest, Created: true}
		return nil
	})
	if err == nil {
		if migrationErr := m.ensureLegacyExecution(request.ID); migrationErr != nil {
			err = migrationErr
		}
	}
	return result, err
}

func (m *Manager) prepareLaunch(request StartRequest) (*store.LaunchConfig, error) {
	if request.Agent == "" && request.PromptReference == "" && request.WorkingContext == "" {
		return nil, nil
	}
	if request.Agent == "" {
		return nil, fmt.Errorf("an agent target is required for managed launch")
	}
	if request.Agent != "pi" {
		return nil, fmt.Errorf("unsupported agent target %q", request.Agent)
	}
	resolve := m.ResolveAgent
	if resolve == nil {
		resolve = exec.LookPath
	}
	command, err := resolve(request.Agent)
	if err != nil || command == "" {
		return nil, fmt.Errorf("Pi agent command is unavailable")
	}
	prompt := request.PromptReference
	if prompt != "" {
		prompt, err = filepath.Abs(prompt)
		if err != nil {
			return nil, fmt.Errorf("resolve the prompt reference")
		}
		info, statErr := os.Lstat(prompt)
		if statErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("prompt reference must identify a regular local file")
		}
	}
	if strings.ContainsAny(request.WorkingContext, "\r\n") {
		return nil, fmt.Errorf("working context must be a single non-secret line")
	}
	return &store.LaunchConfig{Target: request.Agent, Command: command, PromptReference: prompt, WorkingContext: request.WorkingContext}, nil
}

func configuredWorktreeRoot(values []string) (string, error) {
	if len(values) > 1 {
		return "", &store.Error{Kind: store.KindUsage, Message: "Only one worktree root may be configured", Recovery: "Provide one `--worktree-root` value"}
	}
	if len(values) == 0 || values[0] == "" {
		return "", nil
	}
	if !filepath.IsAbs(values[0]) {
		return "", &store.Error{Kind: store.KindUsage, Message: "Repository worktree root must be absolute", Recovery: "Provide an absolute `--worktree-root` value"}
	}
	return filepath.Clean(values[0]), nil
}

func derivedWorktreeRoot(repositoryPath, repositoryName string) string {
	return filepath.Join(filepath.Dir(repositoryPath), ".akagent", "worktrees", repositoryName)
}

func repositoryWorktreeRoot(repository store.Repository) string {
	if repository.WorktreeRoot != "" {
		return filepath.Clean(repository.WorktreeRoot)
	}
	return derivedWorktreeRoot(repository.Path, repository.Name)
}

func (m *Manager) startInputs(request StartRequest, repository store.Repository) (string, string, string, error) {
	branch := request.Branch
	if branch == "" {
		if repository.Policy == "direct" {
			var err error
			branch, err = m.Git.Branch(repository.Path)
			if err != nil {
				return "", "", "", fmt.Errorf("inspect the repository branch")
			}
		} else {
			return "", "", "", &store.Error{
				Kind:     store.KindUsage,
				Message:  "worktree repository tasks require an explicit descriptive --branch",
				Recovery: "Retry with `--branch akofink/<issue-or-ticket>-<description>`",
			}
		}
	}
	if !validBranch(branch) {
		return "", "", "", fmt.Errorf("task branch is invalid")
	}
	base := request.BaseRevision
	if base == "" {
		var err error
		base, err = m.Git.Head(repository.Path)
		if err != nil {
			return "", "", "", fmt.Errorf("inspect the repository base revision")
		}
	} else {
		resolved, err := m.Git.Resolve(repository.Path, base)
		if err != nil {
			return "", "", "", fmt.Errorf("requested base revision is unavailable")
		}
		base = resolved
	}
	worktree := request.WorktreePath
	if repository.Policy == "direct" {
		if worktree == "" {
			worktree = repository.Path
		}
		abs, err := filepath.Abs(worktree)
		if err != nil || abs != repository.Path {
			return "", "", "", fmt.Errorf("direct repository policy requires its registered worktree path")
		}
		currentHead, err := m.Git.Head(repository.Path)
		if err != nil || currentHead != base {
			return "", "", "", fmt.Errorf("direct repository base does not match its current revision")
		}
	} else {
		root := repositoryWorktreeRoot(repository)
		if worktree == "" {
			worktree = filepath.Join(root, branchLabel(branch))
		}
		abs, err := filepath.Abs(worktree)
		if err != nil || !within(abs, root) || abs == repository.Path {
			return "", "", "", fmt.Errorf("task worktree must be under the repository worktree root")
		}
		worktree = abs
	}
	return branch, base, worktree, nil
}

func (m *Manager) validateExistingStart(request StartRequest, repository store.Repository, existing store.Manifest, launch *store.LaunchConfig) error {
	if existing.Title != request.Title || existing.Repository != request.Repository || existing.Worker != "local" {
		return fmt.Errorf("task inputs conflict with the existing task")
	}
	if strings.Join(unique(request.Requirements), ",") != existing.Requirements {
		return fmt.Errorf("task inputs conflict with the existing task")
	}
	if request.Branch != "" && request.Branch != existing.Branch {
		return fmt.Errorf("task inputs conflict with the existing task")
	}
	if request.WorktreePath != "" {
		abs, err := filepath.Abs(request.WorktreePath)
		if err != nil || abs != existing.WorktreePath {
			return fmt.Errorf("task inputs conflict with the existing task")
		}
	}
	if request.BaseRevision != "" {
		resolved, err := m.Git.Resolve(repository.Path, request.BaseRevision)
		if err != nil || resolved != existing.BaseRevision {
			return fmt.Errorf("task inputs conflict with the existing task")
		}
	}
	if !sameRequestedLaunch(existing.Launch, launch) {
		return fmt.Errorf("task inputs conflict with the existing task")
	}
	return nil
}

func (m *Manager) ensureWorktree(id string, repository store.Repository, manifest *store.Manifest) error {
	status, err := m.Git.Status(manifest.WorktreePath)
	if err == nil && status.Exists {
		if !m.worktreeMatches(repository, *manifest, status, true) {
			if repository.Policy == "direct" {
				return fmt.Errorf("registered direct worktree does not match task inputs")
			}
			return fmt.Errorf("task worktree does not match its immutable inputs")
		}
		if manifest.WorktreeBaseRevision == "" {
			manifest.WorktreeBaseRevision = status.Head
		}
		m.applyGitStatus(manifest, status)
		return m.Store.WriteManifest(id, *manifest)
	}
	if repository.Policy == "direct" {
		return fmt.Errorf("registered direct worktree does not match task inputs")
	}
	if _, err := os.Stat(manifest.WorktreePath); err == nil {
		return fmt.Errorf("task worktree path exists but is not the expected Git worktree")
	}
	if err := os.MkdirAll(filepath.Dir(manifest.WorktreePath), 0o755); err != nil {
		return fmt.Errorf("create the task worktree parent")
	}
	if err := m.Git.AddWorktree(repository.Path, manifest.WorktreePath, manifest.Branch, manifest.BaseRevision); err != nil {
		return fmt.Errorf("create task Git worktree")
	}
	status, err = m.Git.Status(manifest.WorktreePath)
	if err != nil || !status.Exists || !m.worktreeMatches(repository, *manifest, status, true) {
		return fmt.Errorf("created task worktree could not be validated")
	}
	manifest.WorktreeBaseRevision = status.Head
	m.applyGitStatus(manifest, status)
	return m.Store.WriteManifest(id, *manifest)
}

func (m *Manager) ensureStarted(id string, manifest *store.Manifest) error {
	if manifest.Lifecycle == "running" {
		observation, err := m.Tmux.Observe(id)
		if err != nil {
			return err
		}
		if observation.Available {
			applyObservation(manifest, observation, m.now(), m.heartbeatTimeout())
			return m.Store.WriteManifest(id, *manifest)
		}
		return nil
	}
	var process TmuxProcess
	var err error
	if manifest.Launch != nil && manifest.Launch.Target == "pi" {
		managed, ok := m.Tmux.(ManagedTmux)
		if !ok {
			return fmt.Errorf("tmux implementation does not support managed launch")
		}
		process, err = managed.StartManaged(id, manifest.Branch, *manifest.Launch)
	} else {
		process, err = m.Tmux.Start(id, manifest.Branch)
	}
	if err != nil {
		_, _ = m.Store.AppendEvent(id, store.Event{Operation: "start", Outcome: "failed"})
		return fmt.Errorf("start tmux task: %w", err)
	}
	manifest.Lifecycle = "running"
	manifest.TmuxWindow = process.WindowID
	manifest.ProcessPID = process.PID
	manifest.ProcessStartTime = process.StartTime
	manifest.ObservedPID = process.PID
	manifest.ObservedStartTime = process.StartTime
	manifest.ProcessPane = process.PaneID
	manifest.Observation = processState(process)
	manifest.ObservationAt = m.now()
	if err := m.Store.WriteManifest(id, *manifest); err != nil {
		return err
	}
	_, err = m.Store.AppendEvent(id, store.Event{Operation: "start", Outcome: "succeeded"})
	return err
}

func (m *Manager) Publish(id, condition, reason, activity string) (store.Manifest, error) {
	if !validCondition(condition) {
		return store.Manifest{}, fmt.Errorf("condition must be active, waiting, blocked, failed, or none")
	}
	var changed bool
	manifest, err := m.Store.UpdateManifest(id, func(manifest *store.Manifest) error {
		changed = manifest.Condition != condition || manifest.Reason != reason || manifest.Activity != activity
		manifest.Condition, manifest.Reason, manifest.Activity, manifest.HeartbeatAt = condition, reason, activity, m.now()
		return nil
	})
	if err != nil {
		return store.Manifest{}, err
	}
	if changed {
		_, err = m.Store.AppendEvent(id, store.Event{Operation: "publish", Outcome: condition})
	}
	if syncErr := m.updateExecutionFromManifest(id, manifest); err == nil && syncErr != nil {
		err = syncErr
	}
	return manifest, err
}

func (m *Manager) Finish(id, outcome, result string) (store.Manifest, error) {
	if outcome != "succeeded" && outcome != "failed" {
		return store.Manifest{}, fmt.Errorf("finish outcome must be succeeded or failed")
	}
	observation, err := m.Tmux.Observe(id)
	if err != nil {
		return store.Manifest{}, err
	}
	if !observation.Available {
		return store.Manifest{}, fmt.Errorf("task process observation is unavailable")
	}
	if len(observation.Processes) != 0 {
		return store.Manifest{}, fmt.Errorf("task process is still running")
	}
	var changed bool
	manifest, err := m.Store.UpdateManifest(id, func(manifest *store.Manifest) error {
		if manifest.Lifecycle == "finished" && manifest.Result == result && manifest.Condition == outcomeToCondition(outcome) {
			return nil
		}
		changed = true
		manifest.Lifecycle, manifest.Condition, manifest.Result = "finished", outcomeToCondition(outcome), result
		manifest.Observation, manifest.ObservationAt = ObservationMissing, m.now()
		manifest.ObservedPID, manifest.ObservedStartTime = 0, 0
		return nil
	})
	if err != nil {
		return store.Manifest{}, err
	}
	if m.refreshGit(&manifest) {
		if err := m.Store.WriteManifest(id, manifest); err != nil {
			return store.Manifest{}, err
		}
	}
	if err := m.syncResourceFacts(id, manifest); err != nil {
		return store.Manifest{}, err
	}
	if changed {
		_, err = m.Store.AppendEvent(id, store.Event{Operation: "finish", Outcome: outcome})
	}
	if syncErr := m.updateExecutionFromManifest(id, manifest); err == nil && syncErr != nil {
		err = syncErr
	}
	return manifest, err
}

func (m *Manager) Stop(id string) (store.Manifest, error) {
	manifest, err := m.manifest(id)
	if err != nil {
		return store.Manifest{}, err
	}
	if manifest.Lifecycle == "stopped" || manifest.Lifecycle == "finished" {
		if syncErr := m.updateExecutionFromManifest(id, manifest); syncErr != nil {
			return store.Manifest{}, syncErr
		}
		return manifest, nil
	}
	if manifest.Lifecycle == "created" {
		manifest.Lifecycle, manifest.Condition = "stopped", "none"
		manifest.Observation, manifest.ObservationAt = ObservationMissing, m.now()
		if err := m.Store.WriteManifest(id, manifest); err != nil {
			return store.Manifest{}, err
		}
		if _, err := m.Store.AppendEvent(id, store.Event{Operation: "stop", Outcome: "succeeded", Detail: "no execution launched"}); err != nil {
			return store.Manifest{}, err
		}
		if syncErr := m.updateExecutionFromManifest(id, manifest); syncErr != nil {
			return store.Manifest{}, syncErr
		}
		return manifest, nil
	}
	if err := m.Tmux.Stop(id); err != nil {
		return store.Manifest{}, err
	}
	var changed bool
	manifest, err = m.Store.UpdateManifest(id, func(manifest *store.Manifest) error {
		if manifest.Lifecycle == "stopped" || manifest.Lifecycle == "finished" {
			return nil
		}
		changed = true
		manifest.Lifecycle, manifest.Condition = "stopped", "none"
		manifest.Observation, manifest.ObservationAt = ObservationMissing, m.now()
		manifest.ObservedPID, manifest.ObservedStartTime = 0, 0
		return nil
	})
	if err != nil {
		return store.Manifest{}, err
	}
	if m.refreshGit(&manifest) {
		if err := m.Store.WriteManifest(id, manifest); err != nil {
			return store.Manifest{}, err
		}
	}
	if err := m.syncResourceFacts(id, manifest); err != nil {
		return store.Manifest{}, err
	}
	if changed {
		_, err = m.Store.AppendEvent(id, store.Event{Operation: "stop", Outcome: "succeeded"})
	}
	if syncErr := m.updateExecutionFromManifest(id, manifest); err == nil && syncErr != nil {
		err = syncErr
	}
	return manifest, err
}

// Reconcile repairs derived observations and Git facts only. It never removes a window or worktree.
func (m *Manager) Reconcile() ([]store.Manifest, error) {
	if _, err := m.Store.Recover(); err != nil {
		return nil, err
	}
	ids, err := m.Store.TaskIDs()
	if err != nil {
		return nil, err
	}
	manifests := make([]store.Manifest, 0, len(ids))
	for _, id := range ids {
		observation, err := m.Tmux.Observe(id)
		if err != nil {
			return nil, err
		}
		var changed bool
		manifest, err := m.Store.UpdateManifest(id, func(manifest *store.Manifest) error {
			beforeObservation, beforeLifecycle := manifest.Observation, manifest.Lifecycle
			beforeGit := *manifest
			legacyProcess := manifest.ProcessPID == 0 || manifest.ProcessStartTime == 0
			applyObservation(manifest, observation, m.now(), m.heartbeatTimeout())
			if legacyProcess && observation.Available && len(observation.Processes) == 0 && manifest.Lifecycle == "running" {
				manifest.Lifecycle, manifest.Condition = "stopped", "none"
			}
			m.refreshGit(manifest)
			changed = beforeObservation != manifest.Observation || beforeLifecycle != manifest.Lifecycle || !sameGitFacts(beforeGit, *manifest)
			return nil
		})
		if err != nil {
			return nil, err
		}
		if err := m.reconcileResources(id, manifest); err != nil {
			return nil, err
		}
		if _, err := m.ReconcileExecutions(id); err != nil {
			return nil, err
		}
		if changed {
			if _, err := m.Store.AppendEvent(id, store.Event{Operation: "reconcile", Outcome: observationOutcome(manifest.Observation)}); err != nil {
				return nil, err
			}
		}
		manifests = append(manifests, manifest)
	}
	return manifests, nil
}

// Attach resolves durable task state, verifies the current task process and
// tmux task-ID option, then attaches to the verified window. It never changes
// durable state or creates, kills, or renames tmux resources.
func (m *Manager) Attach(id string) error {
	manifest, err := m.manifest(id)
	if err != nil {
		return err
	}
	if manifest.Lifecycle == "stopped" {
		return attachStateError(id, "the task is stopped", "Inspect the task or start a new task before attaching")
	}
	if manifest.Lifecycle == "finished" {
		return attachStateError(id, "the task is finished", "Inspect the finished task instead of attaching")
	}
	if manifest.Lifecycle != "running" {
		return attachStateError(id, "the task is not running", fmt.Sprintf("Run `akagent task inspect %s` and reconcile before attaching", id))
	}
	if manifest.Observation != ObservationFresh || manifest.ProcessPID <= 0 || manifest.ProcessStartTime == 0 {
		return attachStateError(id, "the durable process observation is not fresh", fmt.Sprintf("Run `akagent task reconcile` and retry `akagent task attach %s`", id))
	}
	if manifest.HeartbeatAt.IsZero() || m.now().Sub(manifest.HeartbeatAt) > m.heartbeatTimeout() {
		return attachStateError(id, "the task heartbeat is stale", fmt.Sprintf("Run `akagent task reconcile` and retry `akagent task attach %s`", id))
	}
	observation, err := m.Tmux.Observe(id)
	if err != nil {
		return attachStateError(id, "the tmux observation failed", fmt.Sprintf("Retry `akagent task attach %s`", id))
	}
	if !observation.Available {
		return attachStateError(id, "tmux is unavailable", "Start tmux, then retry the attach")
	}
	if len(observation.Processes) == 0 {
		return attachStateError(id, "the task window is missing", fmt.Sprintf("Run `akagent task reconcile` and inspect task %s", id))
	}
	if len(observation.Processes) != 1 || processState(observation.Processes[0]) != ObservationFresh {
		return attachStateError(id, "the task window observation is contradictory", fmt.Sprintf("Run `akagent task reconcile` and inspect task %s", id))
	}
	process := observation.Processes[0]
	if process.WindowID == "" || manifest.TmuxWindow != process.WindowID || manifest.ProcessPane != process.PaneID || manifest.ProcessPID != process.PID || manifest.ProcessStartTime != process.StartTime {
		return attachStateError(id, "the task process or window was replaced", fmt.Sprintf("Run `akagent task reconcile` and inspect task %s", id))
	}
	if err := m.Tmux.Attach(id, process.WindowID); err != nil {
		return fmt.Errorf("attach to verified tmux task window: %w", err)
	}
	return nil
}

func attachStateError(id, reason, recovery string) error {
	return &store.Error{
		Kind:      store.KindConflict,
		Message:   fmt.Sprintf("Task %s cannot be attached: %s", id, reason),
		Retryable: false,
		Recovery:  recovery,
	}
}

func (m *Manager) Inspect(id string) (store.Manifest, error) { return m.manifest(id) }

// Launch is the short-lived worker entrypoint used by tmux. It reconstructs a
// minimal environment from durable task inputs and replaces itself with the
// selected agent so the recorded PID remains the agent PID. Prompt files are
// passed as Pi file references, which keeps stdin attached to the tmux tty for
// interactive use. It returns only redaction-safe errors because it runs in a
// tmux pane.
func (m *Manager) Launch(id string) error {
	fmt.Fprintf(os.Stderr, "akagent: starting managed Pi task %s\n", id)
	manifest, err := m.manifest(id)
	if err != nil {
		return err
	}
	if manifest.Launch == nil {
		return m.markLaunchFailure(id, "missing launch configuration")
	}
	if m.Credentials == nil {
		return m.markLaunchFailure(id, "credential manifest unavailable")
	}
	credentials, err := m.Credentials()
	if err != nil {
		return m.markLaunchFailure(id, "credential manifest unavailable")
	}
	environment, err := credential.BuildEnvironment(credentials, splitRequirements(manifest.Requirements), os.Environ())
	if err != nil {
		return m.markLaunchFailure(id, "requested credential unavailable")
	}
	environment = append(environment, "AKAGENT_TASK_ID="+id)
	if manifest.Launch.WorkingContext != "" {
		environment = append(environment, "AKAGENT_WORKING_CONTEXT="+manifest.Launch.WorkingContext)
	}
	if err := os.Chdir(manifest.Launch.WorkingDirectory); err != nil {
		return m.markLaunchFailure(id, "task worktree unavailable")
	}
	if prompt := manifest.Launch.PromptReference; prompt != "" {
		info, statErr := os.Lstat(prompt)
		if statErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return m.markLaunchFailure(id, "prompt reference unavailable")
		}
	}
	execAgent := m.ExecAgent
	if execAgent == nil {
		execAgent = syscall.Exec
	}
	if err := execAgent(manifest.Launch.Command, managedAgentArgs(manifest.Launch.Command, manifest.Launch.PromptReference), environment); err != nil {
		return m.markLaunchFailure(id, "agent process could not be started")
	}
	return nil
}

func (m *Manager) markLaunchFailure(id, detail string) error {
	fmt.Fprintf(os.Stderr, "akagent: managed Pi task %s failed to start: %s. Retry the same task start command.\n", id, detail)
	_, updateErr := m.Store.UpdateManifest(id, func(manifest *store.Manifest) error {
		manifest.Lifecycle = "starting"
		manifest.Observation = ObservationMissing
		manifest.ObservedPID, manifest.ObservedStartTime = 0, 0
		manifest.ProcessPID, manifest.ProcessStartTime = 0, 0
		manifest.RecoveryDebt = addDebt(manifest.RecoveryDebt, "launch_failed")
		return nil
	})
	if updateErr != nil {
		return errors.New("managed launch failed and could not be recorded")
	}
	if _, eventErr := m.Store.AppendEvent(id, store.Event{Operation: "launch", Outcome: "failed", Detail: detail}); eventErr != nil {
		return errors.New("managed launch failed and recovery event could not be recorded")
	}
	return errors.New("managed agent launch failed; retry the task start")
}

func managedAgentArgs(command, promptReference string) []string {
	args := []string{command}
	if promptReference != "" {
		args = append(args, "@"+promptReference)
	}
	return args
}

func splitRequirements(value string) []string {
	if value == "" {
		return nil
	}
	return strings.Split(value, ",")
}

func (m *Manager) List() ([]store.Manifest, error) {
	ids, err := m.Store.TaskIDs()
	if err != nil {
		return nil, err
	}
	items := make([]store.Manifest, 0, len(ids))
	for _, id := range ids {
		item, err := m.manifest(id)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (m *Manager) manifest(id string) (store.Manifest, error) {
	envelope, err := m.Store.ReadManifest(id)
	if err != nil {
		return store.Manifest{}, err
	}
	return envelope.DecodeManifest()
}

func (m *Manager) checkCredentials(required, optional []string) ([]string, error) {
	if len(required) == 0 && len(optional) == 0 {
		return nil, nil
	}
	manifest, err := m.Credentials()
	if err != nil {
		return nil, fmt.Errorf("credential manifest could not be loaded")
	}
	checks := credential.Doctor(manifest, m.Checker)
	byID := make(map[string]credential.Check, len(checks))
	for _, check := range checks {
		byID[check.Entry.ID] = check
	}
	for _, id := range required {
		check, ok := byID[id]
		if !ok || check.Status != credential.Ready {
			return nil, fmt.Errorf("required credential %s is unavailable", id)
		}
	}
	warnings := make([]string, 0, len(optional))
	for _, id := range optional {
		check, ok := byID[id]
		if !ok || check.Status != credential.Ready {
			warnings = append(warnings, fmt.Sprintf("optional credential %s is unavailable", id))
		}
	}
	return warnings, nil
}

func (m *Manager) now() time.Time {
	if m.Now == nil {
		return time.Now().UTC()
	}
	return m.Now().UTC()
}

func (m *Manager) heartbeatTimeout() time.Duration {
	if m.HeartbeatTimeout <= 0 {
		return DefaultHeartbeatTimeout
	}
	return m.HeartbeatTimeout
}

func sameStart(a, b store.Manifest) bool {
	return a.Title == b.Title && a.Worker == b.Worker && a.Repository == b.Repository && a.Branch == b.Branch && a.BaseRevision == b.BaseRevision && a.WorktreePath == b.WorktreePath && a.Requirements == b.Requirements && sameLaunch(a.Launch, b.Launch)
}

func sameCreate(a, b store.Manifest) bool {
	return a.Title == b.Title && a.Worker == b.Worker && a.Repository == b.Repository && a.Branch == b.Branch && a.BaseRevision == b.BaseRevision && a.WorktreePath == b.WorktreePath && a.Requirements == b.Requirements
}

func (m *Manager) validateExistingCreate(request CreateRequest, existing store.Manifest) error {
	if existing.Title != request.Title || existing.Repository != request.Repository || existing.Worker != "local" {
		return fmt.Errorf("task inputs conflict with the existing task")
	}
	if strings.Join(unique(request.Requirements), ",") != existing.Requirements {
		return fmt.Errorf("task inputs conflict with the existing task")
	}
	if request.Branch != "" && request.Branch != existing.Branch {
		return fmt.Errorf("task inputs conflict with the existing task")
	}
	if request.WorktreePath != "" {
		abs, err := filepath.Abs(request.WorktreePath)
		if err != nil || abs != existing.WorktreePath {
			return fmt.Errorf("task inputs conflict with the existing task")
		}
	}
	if request.BaseRevision != "" {
		repository, err := m.Store.ReadRepository(request.Repository)
		if err != nil {
			return err
		}
		resolved, err := m.Git.Resolve(repository.Path, request.BaseRevision)
		if err != nil || resolved != existing.BaseRevision {
			return fmt.Errorf("task inputs conflict with the existing task")
		}
	}
	return nil
}

// migrateLegacyCreate converts the only legacy state that is safe to detach:
// an interrupted pre-split start with no recorded process identity. Running
// legacy tasks remain attached to their observed execution for recovery.
func migrateLegacyCreate(manifest *store.Manifest) bool {
	if manifest.Lifecycle != "starting" || manifest.ProcessPID != 0 || manifest.ProcessStartTime != 0 || manifest.TmuxWindow != "" {
		return false
	}
	manifest.Lifecycle = "created"
	manifest.Observation = ""
	manifest.ObservationAt = time.Time{}
	manifest.ObservedPID, manifest.ObservedStartTime = 0, 0
	return true
}

func sameLaunch(a, b *store.LaunchConfig) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func sameRequestedLaunch(existing, requested *store.LaunchConfig) bool {
	if existing == nil || requested == nil {
		return existing == nil && requested == nil
	}
	return existing.Target == requested.Target && existing.Command == requested.Command && existing.PromptReference == requested.PromptReference && existing.WorkingContext == requested.WorkingContext
}

func validBranch(branch string) bool {
	if branch == "" || strings.HasPrefix(branch, "-") || strings.HasSuffix(branch, "/") || strings.ContainsAny(branch, " ~^:?*[\\\\") || strings.Contains(branch, "..") || strings.Contains(branch, "@{") {
		return false
	}
	return regexp.MustCompile(`^[A-Za-z0-9._/-]+$`).MatchString(branch)
}

func within(path, root string) bool {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(absoluteRoot, absolutePath)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && relative != "."
}

func expectedMissingWorktree(manifest store.Manifest, repository store.Repository, repositoryErr error) bool {
	if manifest.Lifecycle != "stopped" && manifest.Lifecycle != "finished" {
		return false
	}
	if manifest.ArchiveState != archiveComplete || manifest.CleanupState != cleanupComplete || manifest.WorktreeCleanupState != cleanupComplete {
		return false
	}
	if repositoryErr == nil {
		return repository.Policy == "worktree"
	}
	return store.IsKind(repositoryErr, store.KindNotFound)
}

func (m *Manager) refreshGit(manifest *store.Manifest) bool {
	if manifest.WorktreePath == "" || m.Git == nil {
		return false
	}
	before := *manifest
	repository, repositoryErr := m.Store.ReadRepository(manifest.Repository)
	status, err := m.Git.Status(manifest.WorktreePath)
	if err != nil || !status.Exists {
		manifest.Committed, manifest.Dirty, manifest.Untracked = false, false, false
		if expectedMissingWorktree(*manifest, repository, repositoryErr) {
			manifest.RecoveryDebt = removeDebt(manifest.RecoveryDebt, "worktree_missing")
			manifest.RecoveryDebt = removeDebt(manifest.RecoveryDebt, "worktree_mismatch")
		} else {
			manifest.RecoveryDebt = addDebt(manifest.RecoveryDebt, "worktree_missing")
		}
		return !sameGitFacts(before, *manifest)
	}
	m.applyGitStatus(manifest, status)
	if repositoryErr != nil || !m.worktreeMatches(repository, *manifest, status, false) {
		manifest.RecoveryDebt = addDebt(manifest.RecoveryDebt, "worktree_mismatch")
	} else {
		manifest.RecoveryDebt = removeDebt(manifest.RecoveryDebt, "worktree_mismatch")
	}
	return !sameGitFacts(before, *manifest)
}

func (m *Manager) worktreeMatches(repository store.Repository, manifest store.Manifest, status GitStatus, requireBase bool) bool {
	expectedRoot := manifest.WorktreePath
	if repository.Policy == "direct" {
		expectedRoot = repository.Path
	} else if !within(manifest.WorktreePath, repositoryWorktreeRoot(repository)) {
		return false
	}
	if !sameGitPath(status.Root, expectedRoot) || status.Branch != manifest.Branch {
		return false
	}
	repositoryCommonDir, err := m.Git.CommonDir(repository.Path)
	if err != nil || !sameGitPath(status.CommonDir, repositoryCommonDir) {
		return false
	}
	if manifest.BaseRevision == "" {
		return true
	}
	if manifest.WorktreeBaseRevision != "" && manifest.WorktreeBaseRevision != manifest.BaseRevision {
		return false
	}
	if requireBase {
		return status.Head == manifest.BaseRevision
	}
	baseRevision := manifest.WorktreeBaseRevision
	if baseRevision == "" {
		baseRevision = manifest.BaseRevision
	}
	base, err := m.Git.IsAncestor(manifest.WorktreePath, baseRevision, status.Head)
	return err == nil && base
}

func sameGitPath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	absoluteA, err := filepath.Abs(a)
	if err != nil {
		return false
	}
	absoluteB, err := filepath.Abs(b)
	if err != nil {
		return false
	}
	resolvedA, err := filepath.EvalSymlinks(absoluteA)
	if err == nil {
		absoluteA = resolvedA
	}
	resolvedB, err := filepath.EvalSymlinks(absoluteB)
	if err == nil {
		absoluteB = resolvedB
	}
	return filepath.Clean(absoluteA) == filepath.Clean(absoluteB)
}

func (m *Manager) applyGitStatus(manifest *store.Manifest, status GitStatus) {
	manifest.Dirty, manifest.Untracked = status.Dirty, status.Untracked
	manifest.Committed = !status.Dirty && !status.Untracked && status.Head != ""
	manifest.RecoveryDebt = removeDebt(manifest.RecoveryDebt, "worktree_missing")
	if status.Dirty || status.Untracked {
		manifest.RecoveryDebt = addDebt(manifest.RecoveryDebt, "uncommitted_work")
	} else {
		manifest.RecoveryDebt = removeDebt(manifest.RecoveryDebt, "uncommitted_work")
	}
}

func sameGitFacts(a, b store.Manifest) bool {
	return a.Committed == b.Committed && a.Dirty == b.Dirty && a.Untracked == b.Untracked && a.RecoveryDebt == b.RecoveryDebt
}

func addDebt(values, value string) string {
	for _, existing := range strings.Split(values, ";") {
		if existing == value {
			return values
		}
	}
	if values == "" {
		return value
	}
	return values + ";" + value
}

func removeDebt(values, value string) string {
	parts := strings.Split(values, ";")
	remaining := make([]string, 0, len(parts))
	for _, existing := range parts {
		if existing != "" && existing != value {
			remaining = append(remaining, existing)
		}
	}
	return strings.Join(remaining, ";")
}

func validCondition(value string) bool {
	return value == "active" || value == "waiting" || value == "blocked" || value == "failed" || value == "none"
}

func outcomeToCondition(outcome string) string {
	if outcome == "failed" {
		return "failed"
	}
	return "none"
}

func unique(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			seen[value] = true
		}
	}
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func cloneMetadata(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func processState(process TmuxProcess) string {
	if process.PID == 0 && process.StartTime == 0 {
		return ""
	}
	if process.PID <= 0 || process.StartTime == 0 {
		return ObservationContradictory
	}
	return ObservationFresh
}

func applyObservation(manifest *store.Manifest, observation TmuxObservation, now time.Time, heartbeatTimeout time.Duration) {
	manifest.ObservationAt = now
	if !observation.Available {
		manifest.Observation = ObservationUnavailable
		manifest.ObservedPID, manifest.ObservedStartTime = 0, 0
		return
	}
	if len(observation.Processes) == 0 {
		manifest.Observation = ObservationMissing
		manifest.ObservedPID, manifest.ObservedStartTime = 0, 0
		return
	}
	if len(observation.Processes) != 1 || processState(observation.Processes[0]) != ObservationFresh {
		manifest.Observation = ObservationContradictory
		manifest.ObservedPID, manifest.ObservedStartTime = 0, 0
		return
	}
	process := observation.Processes[0]
	manifest.TmuxWindow, manifest.ProcessPane = process.WindowID, process.PaneID
	manifest.ObservedPID, manifest.ObservedStartTime = process.PID, process.StartTime
	if manifest.ProcessPID == 0 || manifest.ProcessStartTime == 0 {
		manifest.ProcessPID, manifest.ProcessStartTime = process.PID, process.StartTime
	}
	if manifest.ProcessPID == process.PID && manifest.ProcessStartTime == process.StartTime {
		manifest.Observation = ObservationFresh
		if !manifest.HeartbeatAt.IsZero() && now.Sub(manifest.HeartbeatAt) > heartbeatTimeout {
			manifest.Observation = ObservationStale
		}
		return
	}
	manifest.Observation = ObservationReplaced
}

func observationOutcome(observation string) string {
	if observation == ObservationMissing {
		return "window_missing"
	}
	if observation == ObservationReplaced {
		return "process_replaced"
	}
	return "observation_" + observation
}

// Status computes the protocol status from durable lifecycle state and the
// latest verified observations. It never treats a PID without its start time
// as a live task process.
func Status(manifest store.Manifest, now time.Time, heartbeatTimeout time.Duration) string {
	if manifest.Condition == "failed" {
		return "failed"
	}
	switch manifest.Lifecycle {
	case "created":
		return "created"
	case "starting":
		return "starting"
	case "stopped":
		return "stopped"
	case "finished":
		if manifest.Observation == ObservationMissing {
			return "finished"
		}
		return "unknown"
	case "running":
		if manifest.Observation == "" {
			if manifest.Condition == "waiting" || manifest.Condition == "blocked" || manifest.Condition == "active" {
				return manifest.Condition
			}
			return "running"
		}
		if manifest.Observation != ObservationFresh || manifest.HeartbeatAt.IsZero() || now.Sub(manifest.HeartbeatAt) > heartbeatTimeout {
			return "unknown"
		}
		if manifest.Condition == "waiting" || manifest.Condition == "blocked" {
			return manifest.Condition
		}
		return "active"
	default:
		return "unknown"
	}
}

type commandGit struct{}

func (commandGit) run(path string, args ...string) ([]byte, error) {
	return exec.Command("git", append([]string{"-C", path}, args...)...).Output()
}
func (g commandGit) RepositoryRoot(path string) (string, error) {
	inside, err := g.run(path, "rev-parse", "--is-inside-work-tree")
	if err != nil || strings.TrimSpace(string(inside)) != "true" {
		return "", errors.New("not a Git worktree")
	}
	root, err := g.run(path, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", errors.New("Git worktree root unavailable")
	}
	return strings.TrimSpace(string(root)), nil
}
func (g commandGit) CommonDir(path string) (string, error) {
	commonDir, err := g.run(path, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", errors.New("Git common directory unavailable")
	}
	commonDirPath := strings.TrimSpace(string(commonDir))
	if !filepath.IsAbs(commonDirPath) {
		commonDirPath = filepath.Join(path, commonDirPath)
	}
	return commonDirPath, nil
}
func (g commandGit) Head(path string) (string, error) {
	output, err := g.run(path, "rev-parse", "HEAD")
	if err != nil {
		return "", errors.New("Git HEAD unavailable")
	}
	return strings.TrimSpace(string(output)), nil
}
func (g commandGit) Branch(path string) (string, error) {
	output, err := g.run(path, "branch", "--show-current")
	if err != nil || strings.TrimSpace(string(output)) == "" {
		return "", errors.New("Git branch unavailable")
	}
	return strings.TrimSpace(string(output)), nil
}
func (g commandGit) Resolve(path, revision string) (string, error) {
	output, err := g.run(path, "rev-parse", "--verify", revision+"^{commit}")
	if err != nil {
		return "", errors.New("Git revision unavailable")
	}
	return strings.TrimSpace(string(output)), nil
}
func (g commandGit) Status(path string) (GitStatus, error) {
	root, err := g.RepositoryRoot(path)
	if err != nil {
		return GitStatus{}, err
	}
	commonDir, err := g.CommonDir(path)
	if err != nil {
		return GitStatus{}, err
	}
	branch, err := g.Branch(path)
	if err != nil {
		return GitStatus{}, err
	}
	head, err := g.Head(path)
	if err != nil {
		return GitStatus{}, err
	}
	output, err := g.run(path, "status", "--porcelain=v1", "--branch")
	if err != nil {
		return GitStatus{}, errors.New("Git status unavailable")
	}
	status := GitStatus{Exists: true, Root: root, CommonDir: commonDir, Branch: branch, Head: head}
	for _, line := range strings.Split(string(output), "\n") {
		if line == "" || strings.HasPrefix(line, "##") {
			continue
		}
		if strings.HasPrefix(line, "??") {
			status.Untracked = true
		} else {
			status.Dirty = true
		}
	}
	return status, nil
}
func (g commandGit) IsAncestor(path, ancestor, descendant string) (bool, error) {
	_, err := exec.Command("git", "-C", path, "merge-base", "--is-ancestor", ancestor, descendant).Output()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, errors.New("Git ancestry unavailable")
}
func (g commandGit) AddWorktree(repository, path, branch, base string) error {
	if _, err := g.run(repository, "worktree", "add", "-b", branch, path, base); err != nil {
		return errors.New("Git worktree add failed")
	}
	return nil
}
func (g commandGit) RemoveWorktree(repository, path string) error {
	if _, err := g.run(repository, "worktree", "remove", "--force", path); err != nil {
		return errors.New("Git worktree remove failed")
	}
	return nil
}

type commandTmux struct{}

func (commandTmux) Start(id, branch string) (TmuxProcess, error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	return startTmuxWindow(id, branch, "", shell)
}

func (commandTmux) StartManaged(id, branch string, launch store.LaunchConfig) (TmuxProcess, error) {
	executable, err := os.Executable()
	if err != nil || executable == "" {
		return TmuxProcess{}, errors.New("akagent executable could not be resolved")
	}
	command := shellQuote(executable) + " worker launch " + shellQuote(id)
	return startTmuxWindow(id, branch, launch.WorkingDirectory, command)
}

func (commandTmux) StartExecution(executionID, taskID, label, directory, command string, arguments []string) (TmuxProcess, error) {
	parts := []string{"env", shellQuote("AKAGENT_TASK_ID=" + taskID), shellQuote("AKAGENT_EXECUTION_ID=" + executionID), shellQuote(command)}
	for _, argument := range arguments {
		parts = append(parts, shellQuote(argument))
	}
	return startExecutionTmuxWindow(executionID, taskID, label, directory, strings.Join(parts, " "))
}

func (commandTmux) SetExecutionState(executionID, taskID, state string) error {
	if state != "" && state != "waiting" && state != "blocked" && state != "done" {
		return errors.New("invalid execution tmux state")
	}
	output, err := exec.Command("tmux", "list-windows", "-a", "-F", "#{window_id}\t#{@akagent_task_id}\t#{@akagent_execution_id}").Output()
	if err != nil {
		return nil
	}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 3 || fields[1] != taskID || fields[2] != executionID {
			continue
		}
		args := []string{"set-option", "-w"}
		if state == "" {
			args = append(args, "-u")
		}
		args = append(args, "-t", fields[0], "@agent_state")
		if state != "" {
			args = append(args, state)
		}
		if err := exec.Command("tmux", args...).Run(); err != nil {
			return errors.New("tmux execution state could not be published")
		}
	}
	return nil
}

func startExecutionTmuxWindow(executionID, taskID, label, directory, command string) (TmuxProcess, error) {
	args := []string{"new-window", "-d", "-P", "-F", "#{window_id}", "-n", executionWindowName(label)}
	if directory != "" {
		args = append(args, "-c", directory)
	}
	args = append(args, command)
	output, err := exec.Command("tmux", args...).Output()
	if err != nil {
		return TmuxProcess{}, errors.New("tmux execution window could not be created")
	}
	window := strings.TrimSpace(string(output))
	if window == "" {
		return TmuxProcess{}, errors.New("tmux did not return an execution window ID")
	}
	if err := exec.Command("tmux", "set-option", "-w", "-t", window, "@akagent_task_id", taskID).Run(); err != nil {
		return TmuxProcess{}, errors.New("tmux task metadata could not be set")
	}
	if err := exec.Command("tmux", "set-option", "-w", "-t", window, "@akagent_execution_id", executionID).Run(); err != nil {
		return TmuxProcess{}, errors.New("tmux execution metadata could not be set")
	}
	observation, err := (commandTmux{}).ObserveExecution(executionID, taskID)
	if err != nil {
		return TmuxProcess{WindowID: window}, err
	}
	if observation.Available && len(observation.Processes) == 1 {
		return observation.Processes[0], nil
	}
	return TmuxProcess{WindowID: window}, nil
}

func startTmuxWindow(id, branch, directory, command string) (TmuxProcess, error) {
	args := []string{"new-window", "-d", "-P", "-F", "#{window_id}", "-n", tmuxWindowName(branch)}
	if directory != "" {
		args = append(args, "-c", directory)
	}
	args = append(args, command)
	output, err := exec.Command("tmux", args...).Output()
	if err != nil {
		return TmuxProcess{}, errors.New("tmux window could not be created")
	}
	window := strings.TrimSpace(string(output))
	if window == "" {
		return TmuxProcess{}, errors.New("tmux did not return a window ID")
	}
	if err := exec.Command("tmux", "set-option", "-w", "-t", window, "@akagent_task_id", id).Run(); err != nil {
		return TmuxProcess{}, errors.New("tmux task metadata could not be set")
	}
	observation, err := (commandTmux{}).Observe(id)
	if err != nil {
		return TmuxProcess{WindowID: window}, err
	}
	if observation.Available && len(observation.Processes) == 1 {
		return observation.Processes[0], nil
	}
	return TmuxProcess{WindowID: window}, nil
}

func branchLabel(branch string) string {
	if _, name, ok := strings.Cut(branch, "/"); ok && name != "" {
		return name
	}
	if branch != "" {
		return branch
	}
	return "task"
}

func tmuxWindowName(branch string) string {
	return branchLabel(branch)
}

func executionWindowName(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return "execution"
	}
	return label
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\\"'\\\"'") + "'"
}

func (commandTmux) Observe(id string) (TmuxObservation, error) {
	output, err := exec.Command("tmux", "list-windows", "-a", "-F", "#{window_id}\t#{@akagent_task_id}").Output()
	if err != nil {
		return TmuxObservation{}, nil
	}
	observation := TmuxObservation{Available: true}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 2 || fields[1] != id {
			continue
		}
		paneOutput, paneErr := exec.Command("tmux", "list-panes", "-t", fields[0], "-F", "#{window_id}\t#{pane_id}\t#{pane_pid}").Output()
		if paneErr != nil {
			observation.Processes = append(observation.Processes, TmuxProcess{WindowID: fields[0]})
			continue
		}
		for _, paneLine := range strings.Split(strings.TrimSpace(string(paneOutput)), "\n") {
			paneFields := strings.Split(paneLine, "\t")
			if len(paneFields) != 3 {
				continue
			}
			pid, parseErr := strconv.Atoi(paneFields[2])
			startTime := uint64(0)
			if parseErr == nil {
				startTime, _ = processStartTime(pid)
			}
			observation.Processes = append(observation.Processes, TmuxProcess{WindowID: paneFields[0], PaneID: paneFields[1], PID: pid, StartTime: startTime})
		}
	}
	return observation, nil
}

func (commandTmux) ObserveExecution(executionID, taskID string) (TmuxObservation, error) {
	output, err := exec.Command("tmux", "list-windows", "-a", "-F", "#{window_id}\t#{@akagent_task_id}\t#{@akagent_execution_id}").Output()
	if err != nil {
		return TmuxObservation{}, nil
	}
	observation := TmuxObservation{Available: true}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 3 || fields[1] != taskID || fields[2] != executionID {
			continue
		}
		paneOutput, paneErr := exec.Command("tmux", "list-panes", "-t", fields[0], "-F", "#{window_id}\t#{pane_id}\t#{pane_pid}").Output()
		if paneErr != nil {
			observation.Processes = append(observation.Processes, TmuxProcess{WindowID: fields[0]})
			continue
		}
		for _, paneLine := range strings.Split(strings.TrimSpace(string(paneOutput)), "\n") {
			paneFields := strings.Split(paneLine, "\t")
			if len(paneFields) != 3 {
				continue
			}
			pid, parseErr := strconv.Atoi(paneFields[2])
			startTime := uint64(0)
			if parseErr == nil {
				startTime, _ = processStartTime(pid)
			}
			observation.Processes = append(observation.Processes, TmuxProcess{WindowID: paneFields[0], PaneID: paneFields[1], PID: pid, StartTime: startTime})
		}
	}
	return observation, nil
}

func (commandTmux) AttachExecution(executionID, taskID, windowID string) error {
	values, err := exec.Command("tmux", "display-message", "-p", "-t", windowID, "#{@akagent_task_id}\t#{@akagent_execution_id}").Output()
	if err != nil || strings.TrimSpace(string(values)) != taskID+"\t"+executionID {
		return errors.New("tmux execution window could not be verified")
	}
	command := exec.Command("tmux", "attach-session", "-t", windowID)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		return errors.New("tmux execution window could not be attached")
	}
	return nil
}

func (commandTmux) StopExecution(executionID, taskID string) error {
	observation, err := (commandTmux{}).ObserveExecution(executionID, taskID)
	if err != nil || !observation.Available {
		return nil
	}
	windows := map[string]bool{}
	for _, process := range observation.Processes {
		if process.WindowID != "" {
			windows[process.WindowID] = true
		}
	}
	for window := range windows {
		if err := exec.Command("tmux", "kill-window", "-t", window).Run(); err != nil {
			return errors.New("tmux execution window could not be stopped")
		}
	}
	return nil
}

func (commandTmux) CaptureExecution(executionID, taskID string) (string, error) {
	observation, err := (commandTmux{}).ObserveExecution(executionID, taskID)
	if err != nil || !observation.Available || len(observation.Processes) == 0 {
		return "", errors.New("terminal history is unavailable")
	}
	captured, err := exec.Command("tmux", "capture-pane", "-p", "-S", "-", "-t", observation.Processes[0].WindowID).Output()
	if err != nil {
		return "", errors.New("terminal history is unavailable")
	}
	return string(captured), nil
}

func (commandTmux) Attach(taskID, windowID string) error {
	option, err := exec.Command("tmux", "display-message", "-p", "-t", windowID, "#{@akagent_task_id}").Output()
	if err != nil || strings.TrimSpace(string(option)) != taskID {
		return errors.New("tmux task window could not be verified")
	}
	command := exec.Command("tmux", "attach-session", "-t", windowID)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return errors.New("tmux task window could not be attached")
	}
	return nil
}

func (commandTmux) Stop(id string) error {
	output, err := exec.Command("tmux", "list-windows", "-a", "-F", "#{window_id}\t#{@akagent_task_id}").Output()
	if err != nil {
		return nil
	}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) == 2 && fields[1] == id {
			if err := exec.Command("tmux", "kill-window", "-t", fields[0]).Run(); err != nil {
				return errors.New("tmux task window could not be stopped")
			}
		}
	}
	return nil
}

func processStartTime(pid int) (uint64, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err == nil {
		end := strings.LastIndex(string(data), ")")
		if end >= 0 {
			fields := strings.Fields(string(data)[end+2:])
			if len(fields) > 19 {
				return strconv.ParseUint(fields[19], 10, 64)
			}
		}
	}
	return 0, errors.New("process start time is unavailable")
}
