package lifecycle

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/akofink/akagent-cli/internal/store"
)

// CreateResource adds one immutable Git resource to an existing task without
// selecting or starting an execution.
func (m *Manager) CreateResource(taskID string, request ResourceRequest) (store.Resource, bool, error) {
	if taskID == "" || request.ID == "" || request.Repository == "" {
		return store.Resource{}, false, fmt.Errorf("task ID, resource ID, and repository are required")
	}
	if err := m.migrateLegacyResource(taskID); err != nil {
		return store.Resource{}, false, err
	}
	manifest, err := m.manifest(taskID)
	if err != nil {
		return store.Resource{}, false, err
	}
	if manifest.Lifecycle == "stopped" || manifest.Lifecycle == "finished" {
		return store.Resource{}, false, fmt.Errorf("cannot add a resource to a %s task", manifest.Lifecycle)
	}
	repository, err := m.Store.ReadRepository(request.Repository)
	if err != nil {
		return store.Resource{}, false, err
	}
	var result store.Resource
	var created bool
	err = m.Store.WithRepositoryLock(repository.Name, func() error {
		branch, base, worktree, err := m.startInputs(StartRequest{ID: request.ID, Branch: request.Branch, BaseRevision: request.BaseRevision, WorktreePath: request.WorktreePath}, repository)
		if err != nil {
			return err
		}
		resource := store.Resource{ID: request.ID, TaskID: taskID, Repository: request.Repository, Branch: branch, BaseRevision: base, WorktreePath: worktree, Metadata: cloneMetadata(request.Metadata), ExternalURLs: uniqueStrings(request.ExternalURLs)}
		created, result, err = m.Store.CreateResource(taskID, resource)
		if err != nil {
			return err
		}
		if !created {
			_, err = m.Store.UpdateManifest(taskID, func(task *store.Manifest) error {
				task.ResourceIDs = appendResourceID(task.ResourceIDs, request.ID)
				return nil
			})
			return err
		}
		if err := m.ensureResourceWorktree(repository, &result); err != nil {
			_, _ = m.Store.AppendResourceEvent(taskID, request.ID, store.Event{Operation: "create", Outcome: "failed", Detail: "worktree"})
			return err
		}
		if err := m.Store.WriteResource(taskID, result); err != nil {
			return err
		}
		if _, err := m.Store.AppendResourceEvent(taskID, request.ID, store.Event{Operation: "create", Outcome: "intent"}); err != nil {
			return err
		}
		_, err = m.Store.UpdateManifest(taskID, func(task *store.Manifest) error {
			task.ResourceIDs = appendResourceID(task.ResourceIDs, request.ID)
			return nil
		})
		return err
	})
	return result, created, err
}

func (m *Manager) ListResources(taskID string) ([]store.Resource, error) {
	if _, err := m.manifest(taskID); err != nil {
		return nil, err
	}
	if err := m.migrateLegacyResource(taskID); err != nil {
		return nil, err
	}
	ids, err := m.Store.ResourceIDs(taskID)
	if err != nil {
		return nil, err
	}
	resources := make([]store.Resource, 0, len(ids))
	for _, id := range ids {
		resource, err := m.Store.ReadResource(taskID, id)
		if err != nil {
			return nil, err
		}
		resources = append(resources, resource)
	}
	return resources, nil
}

func (m *Manager) InspectResource(taskID, resourceID string) (store.Resource, error) {
	if _, err := m.manifest(taskID); err != nil {
		return store.Resource{}, err
	}
	if err := m.migrateLegacyResource(taskID); err != nil {
		return store.Resource{}, err
	}
	if resourceID == "" {
		ids, err := m.Store.ResourceIDs(taskID)
		if err != nil {
			return store.Resource{}, err
		}
		if len(ids) != 1 {
			return store.Resource{}, fmt.Errorf("resource ID is required when a task has multiple resources")
		}
		resourceID = ids[0]
	}
	return m.Store.ReadResource(taskID, resourceID)
}

// UpdateResource records provider-neutral delivery metadata without changing
// the resource's immutable repository, branch, base, or worktree inputs.
func (m *Manager) UpdateResource(taskID, resourceID string, request ResourceUpdateRequest) (store.Resource, error) {
	if resourceID == "" {
		return store.Resource{}, fmt.Errorf("resource ID is required")
	}
	var changed bool
	resource, err := m.Store.UpdateResource(taskID, resourceID, func(resource *store.Resource) error {
		if len(request.Metadata) > 0 {
			if resource.Metadata == nil {
				resource.Metadata = map[string]string{}
			}
			for key, value := range request.Metadata {
				if resource.Metadata[key] != value {
					changed = true
				}
				resource.Metadata[key] = value
			}
		}
		for _, reference := range uniqueStrings(request.ExternalURLs) {
			if !containsString(resource.ExternalURLs, reference) {
				resource.ExternalURLs = append(resource.ExternalURLs, reference)
				changed = true
			}
		}
		return nil
	})
	if err != nil {
		return store.Resource{}, err
	}
	if changed {
		if _, err := m.Store.AppendResourceEvent(taskID, resourceID, store.Event{Operation: "metadata", Outcome: "updated"}); err != nil {
			return store.Resource{}, err
		}
	}
	return resource, nil
}

func (m *Manager) ArchiveResource(taskID, resourceID string) (store.Resource, error) {
	m.operationMu.Lock()
	defer m.operationMu.Unlock()
	resource, err := m.InspectResource(taskID, resourceID)
	if err != nil {
		return store.Resource{}, err
	}
	if resource.ArchiveState == archiveComplete {
		if _, err := m.Store.ReadResourceArchive(taskID, resource.ID); err == nil {
			return resource, nil
		}
	}
	task, err := m.manifest(taskID)
	if err != nil {
		return store.Resource{}, err
	}
	live, _, err := m.taskLive(taskID)
	if err != nil {
		return store.Resource{}, err
	}
	if live {
		return store.Resource{}, liveTaskError("archiving")
	}
	if task.Lifecycle != "stopped" && task.Lifecycle != "finished" {
		return store.Resource{}, fmt.Errorf("task must be stopped or finished before archiving a resource")
	}
	resource.ArchiveState = archivePending
	if err := m.Store.WriteResource(taskID, resource); err != nil {
		return store.Resource{}, err
	}
	if _, err := m.Store.AppendResourceEvent(taskID, resource.ID, store.Event{Operation: "archive", Outcome: "intent"}); err != nil {
		return m.resourceArchiveFailure(resource, err)
	}
	events, err := m.Store.ReadResourceEvents(taskID, resource.ID)
	if err != nil {
		return m.resourceArchiveFailure(resource, err)
	}
	facts := resource.Git
	warnings := []string{}
	if m.GitFacts != nil {
		if observed, observeErr := m.GitFacts(resourceAsManifest(task, resource)); observeErr == nil {
			facts = observed
			resource.Git = facts
		} else {
			warnings = append(warnings, "Git facts unavailable")
		}
	}
	archive := store.ResourceArchive{TaskID: taskID, ResourceID: resource.ID, CapturedAt: time.Now().UTC(), Resource: resource, Events: events, Git: facts, Warnings: warnings}
	if err := m.Store.WriteResourceArchive(taskID, resource.ID, archive); err != nil {
		return m.resourceArchiveFailure(resource, err)
	}
	resource.ArchiveState = archiveComplete
	resource.Git = facts
	if err := m.Store.WriteResource(taskID, resource); err != nil {
		return m.resourceArchiveFailure(resource, err)
	}
	if _, err := m.Store.AppendResourceEvent(taskID, resource.ID, store.Event{Operation: "archive", Outcome: "succeeded"}); err != nil {
		return m.resourceArchiveFailure(resource, err)
	}
	events, err = m.Store.ReadResourceEvents(taskID, resource.ID)
	if err != nil {
		return m.resourceArchiveFailure(resource, err)
	}
	archive.Resource, archive.Events = resource, events
	if err := m.Store.WriteResourceArchive(taskID, resource.ID, archive); err != nil {
		return m.resourceArchiveFailure(resource, err)
	}
	return resource, nil
}

func (m *Manager) CleanResource(taskID, resourceID string, options CleanupOptions) (store.Resource, error) {
	resource, err := m.InspectResource(taskID, resourceID)
	if err != nil {
		return store.Resource{}, err
	}
	if resource.CleanupState == cleanupComplete && resource.CredentialCleanupState == cleanupComplete {
		return resource, nil
	}
	if resource.ArchiveState != archiveComplete {
		if _, err := m.ArchiveResource(taskID, resource.ID); err != nil {
			return resource, err
		}
		resource, err = m.InspectResource(taskID, resource.ID)
		if err != nil {
			return store.Resource{}, err
		}
	}
	m.operationMu.Lock()
	defer m.operationMu.Unlock()
	task, err := m.manifest(taskID)
	if err != nil {
		return store.Resource{}, err
	}
	live, available, err := m.taskLive(taskID)
	if err != nil {
		return store.Resource{}, err
	}
	if !available {
		return store.Resource{}, errors.New("task process observation is unavailable")
	}
	if live {
		return store.Resource{}, liveTaskError("cleaning")
	}
	if task.Lifecycle != "stopped" && task.Lifecycle != "finished" {
		return store.Resource{}, fmt.Errorf("task must be stopped or finished before cleaning a resource")
	}
	resource.CleanupState, resource.CleanupDebt = cleanupPending, true
	if err := m.Store.WriteResource(taskID, resource); err != nil {
		return store.Resource{}, err
	}
	if _, err := m.Store.AppendResourceEvent(taskID, resource.ID, store.Event{Operation: "cleanup", Outcome: "intent"}); err != nil {
		return m.resourceCleanupFailure(resource, err)
	}
	var failures []error
	if resource.CredentialCleanupState != cleanupComplete {
		if cleanupErr := m.runResourceCredentialCleanup(taskID, &resource, task, options.AllowCredentials); cleanupErr != nil {
			failures = append(failures, cleanupErr)
		}
	}
	facts := resource.Git
	if preservation := missingAuthorization(facts, options); preservation != "" {
		resource.WorktreeCleanupState = cleanupBlocked
		failures = append(failures, &store.Error{Kind: store.KindPreservation, Message: preservation, Recovery: "Inspect the resource archive and retry with explicit cleanup authorization"})
	} else {
		repository, repoErr := m.Store.ReadRepository(resource.Repository)
		if repoErr != nil {
			failures = append(failures, repoErr)
		} else if repository.Policy == "worktree" && !options.AllowWorktree {
			resource.WorktreeCleanupState = cleanupBlocked
			failures = append(failures, &store.Error{Kind: store.KindPreservation, Message: "Cleanup requires explicit authorization to remove the resource worktree", Recovery: "Inspect the resource archive and retry with --allow-worktree"})
		} else if resource.WorktreeCleanupState != cleanupComplete {
			cleanup := m.CleanupWorktree
			if cleanup == nil {
				cleanup = func(store.Manifest, store.GitFacts) error { return nil }
			}
			if cleanupErr := cleanup(resourceAsManifest(task, resource), facts); cleanupErr != nil {
				resource.WorktreeCleanupState = cleanupPartial
				failures = append(failures, cleanupErr)
			} else {
				resource.WorktreeCleanupState = cleanupComplete
			}
		}
	}
	if len(failures) > 0 {
		resource.CleanupState, resource.CleanupDebt = cleanupPartial, true
		_ = m.Store.WriteResource(taskID, resource)
		return resource, resourceCleanupError(resource, failures)
	}
	resource.CleanupState, resource.CleanupDebt = cleanupComplete, false
	if err := m.Store.WriteResource(taskID, resource); err != nil {
		return m.resourceCleanupFailure(resource, err)
	}
	if _, err := m.Store.AppendResourceEvent(taskID, resource.ID, store.Event{Operation: "cleanup", Outcome: "succeeded"}); err != nil {
		return m.resourceCleanupFailure(resource, err)
	}
	return resource, nil
}

func (m *Manager) runResourceCredentialCleanup(taskID string, resource *store.Resource, task store.Manifest, allowed bool) error {
	resource.CleanupDebt = true
	resource.CredentialCleanupState = cleanupPending
	if err := m.Store.WriteResource(taskID, *resource); err != nil {
		return err
	}
	if _, err := m.Store.AppendResourceEvent(taskID, resource.ID, store.Event{Operation: "cleanup_credentials", Outcome: "intent", Detail: "credential cleanup requested"}); err != nil {
		return &store.Error{Kind: store.KindPartial, Message: "Resource credential cleanup is incomplete", Retryable: true, Recovery: fmt.Sprintf("Retry `akagent task resource clean %s %s`", taskID, resource.ID), Err: err}
	}
	if !allowed {
		resource.CredentialCleanupState = cleanupBlocked
		resource.CleanupDebt = true
		writeErr := m.Store.WriteResource(taskID, *resource)
		_, eventErr := m.Store.AppendResourceEvent(taskID, resource.ID, store.Event{Operation: "cleanup_credentials", Outcome: "preservation_required", Detail: "credential cleanup approval required"})
		if writeErr != nil || eventErr != nil {
			return &store.Error{Kind: store.KindPartial, Message: "Resource credential cleanup approval could not be recorded", Retryable: true, Recovery: fmt.Sprintf("Retry `akagent task resource clean %s %s --allow-credentials`", taskID, resource.ID), Err: errors.Join(writeErr, eventErr)}
		}
		return &store.Error{Kind: store.KindPreservation, Message: "Resource credential cleanup requires explicit authorization", Recovery: fmt.Sprintf("Retry `akagent task resource clean %s %s --allow-credentials`", taskID, resource.ID)}
	}
	if m.CleanupCredentials != nil {
		if err := m.CleanupCredentials(resourceAsManifest(task, *resource)); err != nil {
			resource.CredentialCleanupState = cleanupPartial
			resource.CleanupDebt = true
			writeErr := m.Store.WriteResource(taskID, *resource)
			_, eventErr := m.Store.AppendResourceEvent(taskID, resource.ID, store.Event{Operation: "cleanup_credentials", Outcome: "partial", Detail: cleanupDebtEvent})
			return &store.Error{Kind: store.KindPartial, Message: "Resource credential cleanup is incomplete", Retryable: true, Recovery: fmt.Sprintf("Retry `akagent task resource clean %s %s --allow-credentials`", taskID, resource.ID), Err: errors.Join(writeErr, eventErr)}
		}
	}
	resource.CredentialCleanupState = cleanupComplete
	resource.CleanupDebt = resourceCleanupDebtRemaining(*resource)
	if err := m.Store.WriteResource(taskID, *resource); err != nil {
		return err
	}
	if _, err := m.Store.AppendResourceEvent(taskID, resource.ID, store.Event{Operation: "cleanup_credentials", Outcome: "succeeded", Detail: "credential cleanup complete"}); err != nil {
		resource.CredentialCleanupState = cleanupPartial
		resource.CleanupDebt = true
		_ = m.Store.WriteResource(taskID, *resource)
		return &store.Error{Kind: store.KindPartial, Message: "Resource credential cleanup is incomplete", Retryable: true, Recovery: fmt.Sprintf("Retry `akagent task resource clean %s %s --allow-credentials`", taskID, resource.ID)}
	}
	return nil
}

func resourceCleanupDebtRemaining(resource store.Resource) bool {
	return resource.CleanupState != "" && resource.CleanupState != cleanupComplete ||
		resource.WorktreeCleanupState != "" && resource.WorktreeCleanupState != cleanupComplete
}

func (m *Manager) ensureResourceWorktree(repository store.Repository, resource *store.Resource) error {
	manifest := resourceAsManifest(store.Manifest{}, *resource)
	status, err := m.Git.Status(resource.WorktreePath)
	if err == nil && status.Exists {
		if !m.worktreeMatches(repository, manifest, status, true) {
			return fmt.Errorf("resource worktree does not match its immutable inputs")
		}
		resource.WorktreeBaseRevision = status.Head
		resource.Git = gitFactsFromStatus(status)
		applyResourceGitStatus(resource, status)
		return nil
	}
	if repository.Policy == "direct" {
		return fmt.Errorf("registered direct worktree does not match resource inputs")
	}
	if _, err := os.Stat(resource.WorktreePath); err == nil {
		return fmt.Errorf("resource worktree path exists but is not the expected Git worktree")
	}
	if err := os.MkdirAll(filepath.Dir(resource.WorktreePath), 0o755); err != nil {
		return fmt.Errorf("create the resource worktree parent")
	}
	if err := m.Git.AddWorktree(repository.Path, resource.WorktreePath, resource.Branch, resource.BaseRevision); err != nil {
		return fmt.Errorf("create resource Git worktree")
	}
	status, err = m.Git.Status(resource.WorktreePath)
	if err != nil || !status.Exists || !m.worktreeMatches(repository, manifest, status, true) {
		return fmt.Errorf("created resource worktree could not be validated")
	}
	resource.WorktreeBaseRevision = status.Head
	resource.Git = gitFactsFromStatus(status)
	applyResourceGitStatus(resource, status)
	return nil
}

func gitFactsFromStatus(status GitStatus) store.GitFacts {
	return store.GitFacts{Path: status.Root, Head: status.Head, Branch: status.Branch, Dirty: status.Dirty, Untracked: status.Untracked, Committed: !status.Dirty && !status.Untracked && status.Head != ""}
}
func applyResourceGitStatus(resource *store.Resource, status GitStatus) {
	resource.Git = gitFactsFromStatus(status)
	if status.Dirty || status.Untracked {
		resource.RecoveryDebt = addDebt(resource.RecoveryDebt, "uncommitted_work")
	} else {
		resource.RecoveryDebt = removeDebt(resource.RecoveryDebt, "uncommitted_work")
	}
}
func resourceAsManifest(task store.Manifest, resource store.Resource) store.Manifest {
	task.Repository, task.Branch, task.BaseRevision, task.WorktreeBaseRevision, task.WorktreePath = resource.Repository, resource.Branch, resource.BaseRevision, resource.WorktreeBaseRevision, resource.WorktreePath
	task.Committed, task.Dirty, task.Untracked, task.RecoveryDebt = resource.Git.Committed, resource.Git.Dirty, resource.Git.Untracked, resource.RecoveryDebt
	task.CredentialCleanupState = resource.CredentialCleanupState
	task.Git = resource.Git
	return task
}

func (m *Manager) ensureLegacyResource(taskID string, manifest *store.Manifest) error {
	if manifest.Repository == "" || manifest.ResourceIDs != "" {
		return nil
	}
	resource := store.Resource{ID: "legacy", TaskID: taskID, Repository: manifest.Repository, Branch: manifest.Branch, BaseRevision: manifest.BaseRevision, WorktreeBaseRevision: manifest.WorktreeBaseRevision, WorktreePath: manifest.WorktreePath, Git: manifest.Git, RecoveryDebt: manifest.RecoveryDebt, ArchiveState: manifest.ArchiveState, CleanupState: manifest.CleanupState, WorktreeCleanupState: manifest.WorktreeCleanupState, CredentialCleanupState: manifest.CredentialCleanupState, CleanupDebt: manifest.CleanupDebt}
	if resource.Git.Path == "" && m.Git != nil {
		if status, statusErr := m.Git.Status(resource.WorktreePath); statusErr == nil && status.Exists {
			resource.Git = gitFactsFromStatus(status)
			applyResourceGitStatus(&resource, status)
		}
	}
	if _, _, err := m.Store.CreateResource(taskID, resource); err != nil {
		return err
	}
	_, err := m.Store.UpdateManifest(taskID, func(task *store.Manifest) error {
		task.ResourceIDs = appendResourceID(task.ResourceIDs, "legacy")
		return nil
	})
	return err
}

func (m *Manager) migrateLegacyResource(taskID string) error {
	manifest, err := m.manifest(taskID)
	if err != nil {
		return err
	}
	return m.ensureLegacyResource(taskID, &manifest)
}

func (m *Manager) syncResourceFacts(taskID string, task store.Manifest) error {
	if task.ResourceIDs == "" || task.WorktreePath == "" {
		return nil
	}
	for _, resourceID := range strings.Split(task.ResourceIDs, ",") {
		if resourceID == "" {
			continue
		}
		resource, err := m.Store.ReadResource(taskID, resourceID)
		if err != nil {
			return err
		}
		if resource.WorktreePath != task.WorktreePath {
			continue
		}
		resource.Git = task.Git
		resource.RecoveryDebt = task.RecoveryDebt
		if err := m.Store.WriteResource(taskID, resource); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) reconcileResources(taskID string, task store.Manifest) error {
	if task.ResourceIDs == "" {
		return nil
	}
	for _, resourceID := range strings.Split(task.ResourceIDs, ",") {
		if resourceID == "" {
			continue
		}
		resource, err := m.Store.ReadResource(taskID, resourceID)
		if err != nil {
			return err
		}
		repository, repositoryErr := m.Store.ReadRepository(resource.Repository)
		status, statusErr := m.Git.Status(resource.WorktreePath)
		before := resource
		if statusErr != nil || !status.Exists {
			if (task.Lifecycle == "stopped" || task.Lifecycle == "finished") && resource.ArchiveState == archiveComplete && resource.CleanupState == cleanupComplete && resource.WorktreeCleanupState == cleanupComplete {
				resource.RecoveryDebt = removeDebt(resource.RecoveryDebt, "worktree_missing")
			} else {
				resource.RecoveryDebt = addDebt(resource.RecoveryDebt, "worktree_missing")
			}
		} else {
			applyResourceGitStatus(&resource, status)
			if repositoryErr != nil || !m.worktreeMatches(repository, resourceAsManifest(task, resource), status, false) {
				resource.RecoveryDebt = addDebt(resource.RecoveryDebt, "worktree_mismatch")
			} else {
				resource.RecoveryDebt = removeDebt(resource.RecoveryDebt, "worktree_mismatch")
			}
		}
		if !reflect.DeepEqual(resource, before) {
			if err := m.Store.WriteResource(taskID, resource); err != nil {
				return err
			}
			_, _ = m.Store.AppendResourceEvent(taskID, resource.ID, store.Event{Operation: "reconcile", Outcome: "facts"})
		}
	}
	return nil
}

func (m *Manager) resourceArchiveFailure(resource store.Resource, cause error) (store.Resource, error) {
	resource.ArchiveState = archivePartial
	_ = m.Store.WriteResource(resource.TaskID, resource)
	return resource, &store.Error{Kind: store.KindPartial, Message: "Resource archive is incomplete", Retryable: true, Recovery: fmt.Sprintf("Retry `akagent task resource archive %s %s`", resource.TaskID, resource.ID), Err: cause}
}
func resourceCleanupError(resource store.Resource, failures []error) error {
	for _, failure := range failures {
		var typed *store.Error
		if errors.As(failure, &typed) {
			if typed.Kind == store.KindPreservation {
				return typed
			}
		}
	}
	return &store.Error{Kind: store.KindPartial, Message: "Resource cleanup is incomplete", Retryable: true, Recovery: fmt.Sprintf("Retry `akagent task resource clean %s %s`", resource.TaskID, resource.ID), Err: errors.Join(failures...)}
}
func (m *Manager) resourceCleanupFailure(resource store.Resource, cause error) (store.Resource, error) {
	resource.CleanupState, resource.CleanupDebt = cleanupPartial, true
	_ = m.Store.WriteResource(resource.TaskID, resource)
	return resource, &store.Error{Kind: store.KindPartial, Message: "Resource cleanup is incomplete", Retryable: true, Recovery: fmt.Sprintf("Retry `akagent task resource clean %s %s`", resource.TaskID, resource.ID), Err: cause}
}
func appendResourceID(values, value string) string {
	for _, item := range strings.Split(values, ",") {
		if item == value {
			return values
		}
	}
	if values == "" {
		return value
	}
	return values + "," + value
}
