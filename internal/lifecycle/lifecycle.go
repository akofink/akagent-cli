// Package lifecycle applies record-only task lifecycle transitions.
package lifecycle

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

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

const (
	archivePending  = "pending"
	archiveComplete = "complete"
)

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

type Tmux interface {
	Start(taskID, branch string) (TmuxProcess, error)
	Observe(taskID string) (TmuxObservation, error)
	Attach(taskID, windowID string) error
	Stop(taskID string) error
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
	Provider        string
	Model           string
	Thinking        string
	PromptReference string
	WorkingContext  string
}

type LaunchRequest struct {
	ExecutionID     string
	Label           string
	Target          string
	ResourceID      string
	Provider        string
	Model           string
	Thinking        string
	PromptReference string
	WorkingContext  string
}

type CleanupOptions struct {
	AllowCommitted   bool
	AllowDirty       bool
	AllowUntracked   bool
	AllowWorktree    bool
	AllowCredentials bool
}

type Manager struct {
	Store               *store.Store
	Now                 func() time.Time
	AppendEventIfAbsent func(string, store.Event) (int, bool, error)
	Tmux                Tmux
}

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

type ResourceRequest struct {
	ID           string
	Repository   string
	Branch       string
	BaseRevision string
	Head         string
	WorktreePath string
	Metadata     map[string]string
	ExternalURLs []string
}

type ResourceUpdateRequest struct {
	Metadata     map[string]string
	ExternalURLs []string
}

type ExecutionRequest struct {
	ID               string
	Label            string
	Target           string
	Command          string
	Arguments        []string
	Requirements     []string
	ResourceID       string
	WorkingDirectory string
	PredecessorID    string
	PromptReference  string
	WorkingContext   string
}

type StartResult struct {
	Manifest store.Manifest
	Created  bool
}

func New(state *store.Store) *Manager {
	return &Manager{Store: state, Now: time.Now, AppendEventIfAbsent: state.AppendEventIfAbsent}
}

// Removed orchestration entry points are retained only as typed internal
// refusal shims for legacy callers. They never inspect host state.
func (m *Manager) Create(request CreateRequest) (StartResult, error) {
	return m.CreateRecord(request)
}

func (m *Manager) Start(StartRequest) (StartResult, error) {
	return StartResult{}, externalRecordOperationError("task", "legacy")
}

func (m *Manager) Finish(id, outcome, result string) (store.Manifest, error) {
	manifest, err := m.Inspect(id)
	if err != nil {
		return store.Manifest{}, err
	}
	if manifest.Provenance == store.ProvenanceExternal {
		return store.Manifest{}, externalRecordOperationError("task", id)
	}
	return m.FinishRecord(id, outcome, result)
}

func (m *Manager) ReconcileTask(id string) (store.Manifest, error) {
	return m.ReconcileRecord(id)
}

func (m *Manager) LaunchExecution(string, LaunchRequest) (store.Manifest, error) {
	return store.Manifest{}, externalRecordOperationError("execution", "legacy")
}

func (m *Manager) Launch(string) error {
	return externalRecordOperationError("task", "legacy")
}

func (m *Manager) Attach(string) error {
	return externalRecordOperationError("task", "legacy")
}

func (m *Manager) RunDeployment(string, string) error {
	return externalRecordOperationError("execution", "legacy")
}

func (m *Manager) CleanResource(string, string, CleanupOptions) (store.Resource, error) {
	return store.Resource{}, externalRecordOperationError("resource", "legacy")
}

func (m *Manager) Archive(id string) (store.Manifest, error) {
	archived, err := m.ArchiveRecord(id, "", "")
	if err != nil {
		return store.Manifest{}, err
	}
	manifest, ok := archived.(store.Manifest)
	if !ok {
		return store.Manifest{}, fmt.Errorf("task archive returned an invalid record")
	}
	return manifest, nil
}

func (m *Manager) ArchiveResource(taskID, resourceID string) (store.Resource, error) {
	resource, err := m.InspectResource(taskID, resourceID)
	if err != nil {
		return store.Resource{}, err
	}
	if resource.Provenance == store.ProvenanceExternal {
		return store.Resource{}, externalRecordOperationError("resource", resourceID)
	}
	archived, err := m.ArchiveRecord(taskID, resourceID, "")
	if err != nil {
		return store.Resource{}, err
	}
	resource, ok := archived.(store.Resource)
	if !ok {
		return store.Resource{}, fmt.Errorf("resource archive returned an invalid record")
	}
	return resource, nil
}

func (m *Manager) now() time.Time {
	if m.Now != nil {
		return m.Now().UTC()
	}
	return time.Now().UTC()
}

func (m *Manager) Inspect(id string) (store.Manifest, error) {
	if id == "" {
		return store.Manifest{}, fmt.Errorf("task ID is required")
	}
	envelope, err := m.Store.ReadManifest(id)
	if err != nil {
		return store.Manifest{}, err
	}
	var manifest store.Manifest
	if err := json.Unmarshal(envelope.Data, &manifest); err != nil {
		return store.Manifest{}, err
	}
	return manifest, nil
}

func (m *Manager) List() ([]store.Manifest, error) {
	ids, err := m.Store.TaskIDs()
	if err != nil {
		return nil, err
	}
	manifests := make([]store.Manifest, 0, len(ids))
	for _, id := range ids {
		manifest, err := m.Inspect(id)
		if err != nil {
			return nil, err
		}
		manifests = append(manifests, manifest)
	}
	return manifests, nil
}

func (m *Manager) RecordRepository(name, path, policy, worktreeRoot string) (store.Repository, error) {
	if name == "" || path == "" {
		return store.Repository{}, fmt.Errorf("repository name and path are required")
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return store.Repository{}, err
	}
	if policy == "" {
		policy = "worktree"
	}
	if policy != "worktree" && policy != "direct" {
		return store.Repository{}, fmt.Errorf("repository policy must be worktree or direct")
	}
	if worktreeRoot != "" {
		worktreeRoot, err = filepath.Abs(worktreeRoot)
		if err != nil {
			return store.Repository{}, err
		}
	} else if policy == "worktree" {
		worktreeRoot = derivedWorktreeRoot(absolutePath, name)
	}
	repository := store.Repository{Name: name, Path: absolutePath, Policy: policy, WorktreeRoot: worktreeRoot}
	if _, err := m.Store.RegisterRepository(repository); err != nil {
		return store.Repository{}, err
	}
	return repository, nil
}

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

func (m *Manager) InspectRepository(name string) (store.Repository, error) {
	return m.Store.ReadRepository(name)
}

func (m *Manager) UnregisterRepository(name string) error {
	return m.Store.UnregisterRepository(name)
}

func (m *Manager) Status(manifest store.Manifest) string {
	return Status(manifest, m.now(), DefaultHeartbeatTimeout)
}

func Status(manifest store.Manifest, now time.Time, timeout time.Duration) string {
	if manifest.Condition == "failed" {
		return "failed"
	}
	if manifest.Lifecycle == "finished" {
		return "finished"
	}
	if manifest.Lifecycle == "stopped" {
		return "stopped"
	}
	if manifest.Condition == "waiting" || manifest.Condition == "blocked" {
		return manifest.Condition
	}
	if manifest.Lifecycle == "created" {
		return "created"
	}
	if manifest.Lifecycle == "starting" {
		return "starting"
	}
	if manifest.Lifecycle == "running" {
		if manifest.HeartbeatAt.IsZero() || now.Sub(manifest.HeartbeatAt) > timeout {
			return "unknown"
		}
		return "active"
	}
	return "unknown"
}

func ExecutionStatus(execution store.Execution, now time.Time, timeout time.Duration) string {
	if execution.Condition == "failed" {
		return "failed"
	}
	if execution.Lifecycle == "finished" {
		return "finished"
	}
	if execution.Lifecycle == "stopped" {
		return "stopped"
	}
	if execution.Condition == "waiting" || execution.Condition == "blocked" {
		return execution.Condition
	}
	if execution.Lifecycle == "created" {
		return "created"
	}
	if execution.Lifecycle == "starting" {
		return "starting"
	}
	if execution.Lifecycle == "running" && (!execution.HeartbeatAt.IsZero() && now.Sub(execution.HeartbeatAt) <= timeout) {
		return "active"
	}
	if execution.Lifecycle == "running" {
		return "unknown"
	}
	return "unknown"
}

func unique(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func uniqueStrings(values []string) []string { return unique(values) }

func cloneMetadata(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func appendResourceID(existing, id string) string {
	values := strings.Split(existing, ",")
	values = append(values, id)
	return strings.Join(unique(values), ",")
}

func derivedWorktreeRoot(path, name string) string {
	return filepath.Join(filepath.Dir(path), ".akagent", "worktrees", name)
}

func validCondition(condition string) bool {
	switch condition {
	case "active", "waiting", "blocked", "failed", "none":
		return true
	default:
		return false
	}
}

func outcomeToCondition(outcome string) string {
	if outcome == "succeeded" || outcome == "failed" {
		return outcome
	}
	return "none"
}

func externalRecordOperationError(kind, id string) error {
	return &store.Error{Kind: store.KindConflict, Message: fmt.Sprintf("%s %s is externally managed", kind, id), Recovery: "Use the retained task, resource, or execution record commands for externally managed records"}
}

func terminalFinishConflict(id string) error {
	return &store.Error{Kind: store.KindConflict, Message: fmt.Sprintf("task %s is terminal and immutable", id), Recovery: "Retry the original outcome and result, or use the revision-checked external completion contract"}
}
