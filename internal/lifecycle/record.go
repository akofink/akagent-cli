package lifecycle

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/akofink/akagent-cli/internal/store"
)

// CreateExternalTask creates a caller-owned task without consulting host
// state or relabeling an existing managed task.
func (m *Manager) CreateExternalTask(taskID string, request store.ExternalTaskRequest) (store.Manifest, error) {
	return m.Store.CreateExternalTask(taskID, request)
}

// RecordTask adopts a task identity without consulting host state.
func (m *Manager) RecordTask(taskID string, request store.ExternalTaskRequest) (store.Manifest, error) {
	return m.Store.AdoptExternalTask(taskID, request)
}

// CreateExternalResource records a caller-owned resource only beneath an
// explicitly external task.
func (m *Manager) CreateExternalResource(taskID string, request store.ExternalResourceRequest) (store.Resource, error) {
	manifest, err := m.Inspect(taskID)
	if err != nil {
		return store.Resource{}, err
	}
	if manifest.Provenance != store.ProvenanceExternal {
		return store.Resource{}, externalRecordOperationError("task", taskID)
	}
	return m.Store.AdoptExternalResource(taskID, request)
}

// RecordResource adopts caller-declared repository identity without consulting
// Git, the filesystem, tmux, credentials, or a provider.
func (m *Manager) RecordResource(taskID string, request store.ExternalResourceRequest) (store.Resource, error) {
	return m.Store.AdoptExternalResource(taskID, request)
}

// RecordExecution creates one externally managed execution attempt without
// starting a process or selecting an interaction surface.
func (m *Manager) RecordExecution(taskID string, request store.ExternalExecutionRequest) (store.Execution, error) {
	return m.Store.RecordExternalExecution(taskID, request)
}

// RecordObservation persists caller-submitted provenance and observations.
// Observations remain historical evidence and do not change lifecycle state.
func (m *Manager) RecordObservation(taskID, executionID, callerID, operationID string, expectedRevision uint64, observation store.ExternalObservation) (store.Execution, error) {
	return m.Store.ObserveExternalExecution(taskID, executionID, callerID, operationID, expectedRevision, observation)
}

// CompleteExternalExecution records explicit completion against a caller-named
// contract. It never infers success from a missing process.
func (m *Manager) CompleteExternalExecution(taskID, executionID, callerID, operationID, contract, result string, expectedRevision uint64) (store.Execution, error) {
	return m.Store.CompleteExternalExecution(taskID, executionID, callerID, operationID, contract, result, expectedRevision)
}

// CompleteExternalTask records task completion for an externally adopted task.
func (m *Manager) CompleteExternalTask(taskID, callerID, operationID, contract, result string, expectedRevision uint64) (store.Manifest, error) {
	return m.Store.CompleteExternalTask(taskID, callerID, operationID, contract, result, expectedRevision)
}

// ArchiveExternal archives only durable records and event history. It does not
// capture terminal output, inspect processes, run Git, or clean anything.
func (m *Manager) ArchiveExternal(taskID, resourceID, executionID, callerID, operationID string, expectedRevision uint64) (any, error) {
	switch {
	case resourceID != "":
		archive, err := m.Store.ArchiveExternalResource(taskID, resourceID, callerID, operationID, expectedRevision)
		if err != nil {
			return nil, err
		}
		return archive, nil
	case executionID != "":
		archive, err := m.Store.ArchiveExternalExecution(taskID, executionID, callerID, operationID, expectedRevision)
		if err != nil {
			return nil, err
		}
		return archive, nil
	default:
		archive, err := m.Store.ArchiveExternalTask(taskID, callerID, operationID, expectedRevision)
		if err != nil {
			return nil, err
		}
		return archive, nil
	}
}

// CreateRecord persists task intent and optional caller-declared repository
// facts without checking credentials, Git, or the filesystem.
func (m *Manager) CreateRecord(request CreateRequest) (StartResult, error) {
	if request.ID == "" || request.Title == "" {
		return StartResult{}, fmt.Errorf("task ID and title are required")
	}
	request.Requirements = unique(request.Requirements)
	manifest := store.Manifest{
		Title: request.Title, Worker: "local", Repository: request.Repository,
		Branch: request.Branch, BaseRevision: request.BaseRevision, WorktreePath: request.WorktreePath,
		Lifecycle: "created", Condition: "none", Disposition: string(DispositionInFlight),
		HeartbeatAt: m.now(), Requirements: strings.Join(request.Requirements, ","),
	}
	created, existing, err := m.Store.CreateManifest(request.ID, manifest)
	if err != nil {
		return StartResult{}, err
	}
	if !created {
		if existing.Title != manifest.Title || existing.Worker != manifest.Worker || existing.Repository != manifest.Repository || existing.Branch != manifest.Branch || existing.BaseRevision != manifest.BaseRevision || existing.WorktreePath != manifest.WorktreePath || existing.Requirements != manifest.Requirements {
			return StartResult{}, fmt.Errorf("task inputs conflict with the existing task")
		}
		return StartResult{Manifest: existing}, nil
	}
	if _, err := m.Store.AppendEvent(request.ID, store.Event{Operation: "create", Outcome: "intent"}); err != nil {
		return StartResult{}, err
	}
	if request.Repository == "" {
		return StartResult{Manifest: manifest, Created: true}, nil
	}
	resource := store.Resource{
		ID: "legacy", TaskID: request.ID, Repository: request.Repository,
		Branch: request.Branch, BaseRevision: request.BaseRevision, WorktreePath: request.WorktreePath,
		Git: store.GitFacts{Path: request.WorktreePath, Head: request.BaseRevision, Branch: request.Branch},
	}
	if _, _, err := m.Store.CreateResource(request.ID, resource); err != nil {
		return StartResult{}, err
	}
	if _, err := m.Store.AppendResourceEvent(request.ID, resource.ID, store.Event{Operation: "create", Outcome: "intent"}); err != nil {
		return StartResult{}, err
	}
	manifest, err = m.Store.UpdateManifest(request.ID, func(task *store.Manifest) error {
		task.ResourceIDs = "legacy"
		return nil
	})
	if err != nil {
		return StartResult{}, err
	}
	return StartResult{Manifest: manifest, Created: true}, nil
}

// UpdateRepositoryRecord updates repository metadata without checking or
// changing the registered checkout.
func (m *Manager) UpdateRepositoryRecord(name, path, policy, worktreeRoot string) (store.Repository, error) {
	if name == "" || (path == "" && policy == "" && worktreeRoot == "") {
		return store.Repository{}, fmt.Errorf("repository name and an update are required")
	}
	absolutePath := ""
	var err error
	if path != "" {
		absolutePath, err = filepath.Abs(path)
		if err != nil {
			return store.Repository{}, err
		}
	}
	return m.Store.UpdateRepository(name, func(repository *store.Repository) error {
		if absolutePath != "" {
			repository.Path = absolutePath
		}
		if policy != "" {
			if policy != "worktree" && policy != "direct" {
				return fmt.Errorf("repository policy must be worktree or direct")
			}
			repository.Policy = policy
		}
		if worktreeRoot != "" {
			root, rootErr := filepath.Abs(worktreeRoot)
			if rootErr != nil {
				return rootErr
			}
			repository.WorktreeRoot = root
		} else if repository.Policy == "direct" {
			repository.WorktreeRoot = ""
		}
		return nil
	})
}

// CreateResourceRecord persists caller-declared repository facts without
// resolving a repository registration or touching a worktree.
func (m *Manager) CreateResourceRecord(taskID string, request ResourceRequest) (store.Resource, bool, error) {
	if taskID == "" || request.ID == "" || request.Repository == "" {
		return store.Resource{}, false, fmt.Errorf("task ID, resource ID, and repository are required")
	}
	manifest, err := m.Inspect(taskID)
	if err != nil {
		return store.Resource{}, false, err
	}
	if manifest.Lifecycle == "stopped" || manifest.Lifecycle == "finished" {
		return store.Resource{}, false, fmt.Errorf("cannot add a resource to a %s task", manifest.Lifecycle)
	}
	resource := store.Resource{
		ID: request.ID, TaskID: taskID, Repository: request.Repository, Branch: request.Branch,
		BaseRevision: request.BaseRevision, WorktreePath: request.WorktreePath,
		Metadata: cloneMetadata(request.Metadata), ExternalURLs: uniqueStrings(request.ExternalURLs),
		Git: store.GitFacts{Path: request.WorktreePath, Head: request.Head, Branch: request.Branch},
	}
	created, existing, err := m.Store.CreateResource(taskID, resource)
	if err != nil {
		return store.Resource{}, false, err
	}
	if created {
		if _, err := m.Store.AppendResourceEvent(taskID, request.ID, store.Event{Operation: "create", Outcome: "intent"}); err != nil {
			return store.Resource{}, false, err
		}
	}
	if _, err := m.Store.UpdateManifest(taskID, func(task *store.Manifest) error {
		task.ResourceIDs = appendResourceID(task.ResourceIDs, request.ID)
		return nil
	}); err != nil {
		return store.Resource{}, false, err
	}
	return existing, created, nil
}

// PublishRecord updates task state without synchronizing an execution to a
// host interaction surface.
func (m *Manager) PublishRecord(id, condition, reason, activity string) (store.Manifest, error) {
	if !validCondition(condition) {
		return store.Manifest{}, fmt.Errorf("condition must be active, waiting, blocked, failed, or none")
	}
	manifest, err := m.Store.UpdateManifest(id, func(manifest *store.Manifest) error {
		manifest.Condition, manifest.Reason, manifest.Activity, manifest.HeartbeatAt = condition, reason, activity, m.now()
		return nil
	})
	if err != nil {
		return store.Manifest{}, err
	}
	if _, err := m.Store.AppendEvent(id, store.Event{Operation: "publish", Outcome: condition}); err != nil {
		return store.Manifest{}, err
	}
	return manifest, nil
}

// FinishRecord records explicit task completion without inferring process or
// Git state.
func (m *Manager) FinishRecord(id, outcome, result string) (store.Manifest, error) {
	if outcome != "succeeded" && outcome != "failed" {
		return store.Manifest{}, fmt.Errorf("finish outcome must be succeeded or failed")
	}
	changed := false
	manifest, err := m.Store.UpdateManifest(id, func(manifest *store.Manifest) error {
		if manifest.Lifecycle == "finished" || manifest.ExternalCompletion != nil || manifest.ArchiveState == "complete" {
			if manifest.Result == result && (manifest.Condition == outcome || manifest.Condition == "none") {
				return nil
			}
			return terminalFinishConflict(id)
		}
		manifest.Lifecycle, manifest.Condition, manifest.Result = "finished", outcomeToCondition(outcome), result
		manifest.Disposition = string(DispositionTerminal)
		manifest.Observation, manifest.ObservationAt = ObservationMissing, m.now()
		manifest.ObservedPID, manifest.ObservedStartTime = 0, 0
		if manifest.DispositionReason == "" {
			manifest.DispositionReason = "task finished: " + outcome
		}
		manifest.DispositionRevision++
		manifest.HeartbeatAt = m.now()
		changed = true
		return nil
	})
	if err != nil {
		return store.Manifest{}, err
	}
	if !changed {
		return manifest, nil
	}
	if _, err := m.Store.AppendEvent(id, store.Event{Operation: "finish", Outcome: outcome}); err != nil {
		return store.Manifest{}, err
	}
	return manifest, nil
}

// ReconcileRecord reads durable state and repairs only store artifacts.
func (m *Manager) ReconcileRecord(taskID string) (store.Manifest, error) {
	if _, err := m.Store.Recover(); err != nil {
		return store.Manifest{}, err
	}
	return m.Inspect(taskID)
}

// ReconcileRecordExecutions returns durable execution records without probing
// tmux, processes, Git, providers, or credentials.
func (m *Manager) ReconcileRecordExecutions(taskID string) ([]store.Execution, error) {
	return m.ListExecutions(taskID)
}

// PublishExecutionRecord updates execution state without publishing to tmux.
func (m *Manager) PublishExecutionRecord(taskID, executionID, condition, reason, activity string) (store.Execution, error) {
	if !validCondition(condition) {
		return store.Execution{}, fmt.Errorf("condition must be active, waiting, blocked, failed, or none")
	}
	execution, err := m.Store.UpdateExecution(taskID, executionID, func(execution *store.Execution) error {
		execution.Condition, execution.Reason, execution.Activity, execution.HeartbeatAt = condition, reason, activity, m.now()
		return nil
	})
	if err != nil {
		return store.Execution{}, err
	}
	if _, err := m.Store.AppendExecutionEvent(taskID, executionID, store.Event{Operation: "publish", Outcome: condition}); err != nil {
		return store.Execution{}, err
	}
	return execution, nil
}

// ArchiveRecord writes only durable manifests, events, and references. It
// never captures terminal output or observes a host resource.
func (m *Manager) ArchiveRecord(taskID, resourceID, executionID string) (any, error) {
	switch {
	case resourceID != "":
		return m.archiveResourceRecord(taskID, resourceID)
	case executionID != "":
		return m.archiveExecutionRecord(taskID, executionID)
	default:
		return m.archiveTaskRecord(taskID)
	}
}

func (m *Manager) archiveTaskRecord(taskID string) (store.Manifest, error) {
	manifest, err := m.Inspect(taskID)
	if err != nil {
		return store.Manifest{}, err
	}
	if manifest.ArchiveState == archiveComplete {
		if _, err := m.Store.ReadArchive(taskID); err == nil {
			return manifest, nil
		}
	}
	if manifest.Lifecycle != "stopped" && manifest.Lifecycle != "finished" {
		return store.Manifest{}, fmt.Errorf("task must be stopped or finished before archiving")
	}
	manifest.ArchiveState = archivePending
	if err := m.Store.WriteManifest(taskID, manifest); err != nil {
		return store.Manifest{}, err
	}
	if _, err := m.Store.AppendEvent(taskID, store.Event{Operation: "archive", Outcome: "intent"}); err != nil {
		return store.Manifest{}, err
	}
	events, err := m.Store.ReadEvents(taskID)
	if err != nil {
		return store.Manifest{}, err
	}
	resources, err := m.ListResources(taskID)
	if err != nil {
		return store.Manifest{}, err
	}
	executions, err := m.ListExecutions(taskID)
	if err != nil {
		return store.Manifest{}, err
	}
	var checkpoint *store.Checkpoint
	if value, checkpointErr := m.Store.ReadCheckpoint(taskID); checkpointErr == nil {
		checkpoint = &value
	} else if !store.IsKind(checkpointErr, store.KindNotFound) {
		return store.Manifest{}, checkpointErr
	}
	archive := store.TaskArchive{TaskID: taskID, CapturedAt: time.Now().UTC(), Manifest: manifest, Checkpoint: checkpoint, Events: events, Resources: resources, Executions: executions, Git: manifest.Git}
	if err := m.Store.WriteArchive(taskID, archive); err != nil {
		return store.Manifest{}, err
	}
	manifest.ArchiveState = archiveComplete
	if err := m.Store.WriteManifest(taskID, manifest); err != nil {
		return store.Manifest{}, err
	}
	if _, err := m.Store.AppendEvent(taskID, store.Event{Operation: "archive", Outcome: "succeeded"}); err != nil {
		return store.Manifest{}, err
	}
	archive.Manifest = manifest
	archive.Events, err = m.Store.ReadEvents(taskID)
	if err != nil {
		return store.Manifest{}, err
	}
	return manifest, m.Store.WriteArchive(taskID, archive)
}

func (m *Manager) archiveResourceRecord(taskID, resourceID string) (store.Resource, error) {
	resource, err := m.InspectResource(taskID, resourceID)
	if err != nil {
		return store.Resource{}, err
	}
	if resource.ArchiveState == archiveComplete {
		if _, err := m.Store.ReadResourceArchive(taskID, resourceID); err == nil {
			return resource, nil
		}
	}
	task, err := m.Inspect(taskID)
	if err != nil {
		return store.Resource{}, err
	}
	if task.Lifecycle != "stopped" && task.Lifecycle != "finished" {
		return store.Resource{}, fmt.Errorf("task must be stopped or finished before archiving a resource")
	}
	resource.ArchiveState = archivePending
	if err := m.Store.WriteResource(taskID, resource); err != nil {
		return store.Resource{}, err
	}
	if _, err := m.Store.AppendResourceEvent(taskID, resourceID, store.Event{Operation: "archive", Outcome: "intent"}); err != nil {
		return store.Resource{}, err
	}
	events, err := m.Store.ReadResourceEvents(taskID, resourceID)
	if err != nil {
		return store.Resource{}, err
	}
	archive := store.ResourceArchive{TaskID: taskID, ResourceID: resourceID, CapturedAt: time.Now().UTC(), Resource: resource, Events: events, Git: resource.Git}
	if err := m.Store.WriteResourceArchive(taskID, resourceID, archive); err != nil {
		return store.Resource{}, err
	}
	resource.ArchiveState = archiveComplete
	if err := m.Store.WriteResource(taskID, resource); err != nil {
		return store.Resource{}, err
	}
	if _, err := m.Store.AppendResourceEvent(taskID, resourceID, store.Event{Operation: "archive", Outcome: "succeeded"}); err != nil {
		return store.Resource{}, err
	}
	archive.Resource = resource
	archive.Events, err = m.Store.ReadResourceEvents(taskID, resourceID)
	if err != nil {
		return store.Resource{}, err
	}
	return resource, m.Store.WriteResourceArchive(taskID, resourceID, archive)
}

func (m *Manager) archiveExecutionRecord(taskID, executionID string) (store.Execution, error) {
	execution, err := m.InspectExecution(taskID, executionID)
	if err != nil {
		return store.Execution{}, err
	}
	if execution.ArchiveState == archiveComplete {
		if _, err := m.Store.ReadExecutionArchive(taskID, executionID); err == nil {
			return execution, nil
		}
	}
	if execution.Lifecycle != "stopped" && execution.Lifecycle != "finished" {
		return store.Execution{}, fmt.Errorf("execution must be stopped or finished before archiving")
	}
	execution.ArchiveState = archivePending
	if err := m.Store.WriteExecution(taskID, execution); err != nil {
		return store.Execution{}, err
	}
	if _, err := m.Store.AppendExecutionEvent(taskID, executionID, store.Event{Operation: "archive", Outcome: "intent"}); err != nil {
		return store.Execution{}, err
	}
	events, err := m.Store.ReadExecutionEvents(taskID, executionID)
	if err != nil {
		return store.Execution{}, err
	}
	archive := store.ExecutionArchive{TaskID: taskID, ExecutionID: executionID, CapturedAt: time.Now().UTC(), Execution: execution, Events: events}
	if err := m.Store.WriteExecutionArchive(taskID, executionID, archive); err != nil {
		return store.Execution{}, err
	}
	execution.ArchiveState = archiveComplete
	if err := m.Store.WriteExecution(taskID, execution); err != nil {
		return store.Execution{}, err
	}
	if _, err := m.Store.AppendExecutionEvent(taskID, executionID, store.Event{Operation: "archive", Outcome: "succeeded"}); err != nil {
		return store.Execution{}, err
	}
	archive.Execution = execution
	archive.Events, err = m.Store.ReadExecutionEvents(taskID, executionID)
	if err != nil {
		return store.Execution{}, err
	}
	return execution, m.Store.WriteExecutionArchive(taskID, executionID, archive)
}
