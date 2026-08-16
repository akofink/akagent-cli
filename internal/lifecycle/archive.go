package lifecycle

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/akofink/akagent-cli/internal/store"
)

// CleanupOptions explicitly authorizes destruction of each kind of recoverable
// Git state. No option is implied by task completion or by archive success.
type CleanupOptions struct {
	AllowCommitted bool
	AllowDirty     bool
	AllowUntracked bool
	AllowWorktree  bool
}

const (
	archivePending   = "pending"
	archiveComplete  = "complete"
	archivePartial   = "partial"
	cleanupPending   = "pending"
	cleanupComplete  = "complete"
	cleanupPartial   = "partial"
	cleanupBlocked   = "blocked"
	cleanupDebtEvent = "cleanup debt remains"
)

// Archive captures the durable task record and available non-secret resource
// observations. A stopped or finished task can be archived repeatedly without
// creating duplicate archives or destructive side effects.
func (m *Manager) Archive(id string) (store.Manifest, error) {
	m.operationMu.Lock()
	defer m.operationMu.Unlock()
	return m.archive(id)
}

func (m *Manager) archive(id string) (store.Manifest, error) {
	manifest, err := m.manifest(id)
	if err != nil {
		return store.Manifest{}, err
	}
	if manifest.ArchiveState == archiveComplete {
		if _, archiveErr := m.Store.ReadArchive(id); archiveErr == nil {
			if syncErr := m.syncExecutionStates(id); syncErr != nil {
				return store.Manifest{}, syncErr
			}
			return manifest, nil
		}
	}

	live, _, err := m.taskLive(id)
	if err != nil {
		return store.Manifest{}, err
	}
	if live {
		return store.Manifest{}, liveTaskError("archiving")
	}
	if manifest.Lifecycle != "stopped" && manifest.Lifecycle != "finished" {
		return store.Manifest{}, fmt.Errorf("task must be stopped or finished before archiving")
	}

	manifest.ArchiveState = archivePending
	if err := m.Store.WriteManifest(id, manifest); err != nil {
		return store.Manifest{}, err
	}
	if err := appendOperationEvent(m.Store, id, "archive", "intent", "archive requested"); err != nil {
		return m.archiveFailure(id, manifest, err)
	}

	events, err := m.Store.ReadEvents(id)
	if err != nil {
		return m.archiveFailure(id, manifest, err)
	}
	facts, warnings := m.archiveGitFacts(manifest)
	terminal := ""
	terminalErr := error(nil)
	if m.TerminalHistory != nil {
		terminal, terminalErr = m.TerminalHistory(id)
	}
	if terminalErr != nil {
		warnings = append(warnings, "terminal history unavailable")
		terminal = ""
	}
	archive := store.TaskArchive{
		TaskID:     id,
		CapturedAt: time.Now().UTC(),
		Manifest:   manifest,
		Events:     events,
		Git:        facts,
		Terminal:   terminal,
		Warnings:   warnings,
	}
	if err := m.Store.WriteArchive(id, archive); err != nil {
		return m.archiveFailure(id, manifest, err)
	}

	manifest.ArchiveState = archiveComplete
	manifest.Git = facts
	if err := m.Store.WriteManifest(id, manifest); err != nil {
		return m.archiveFailure(id, manifest, err)
	}
	if err := appendOperationEvent(m.Store, id, "archive", "succeeded", "archive captured"); err != nil {
		return m.archiveFailure(id, manifest, err)
	}

	// Include the successful archive event and the final manifest state. If this
	// replacement fails, the earlier snapshot remains durable and the task is
	// marked partial so a retry can complete it.
	events, err = m.Store.ReadEvents(id)
	if err != nil {
		return m.archiveFailure(id, manifest, err)
	}
	archive.Manifest = manifest
	archive.Events = events
	if err := m.Store.WriteArchive(id, archive); err != nil {
		return m.archiveFailure(id, manifest, err)
	}
	if err := m.syncExecutionStates(id); err != nil {
		return manifest, err
	}
	return manifest, nil
}

func (m *Manager) archiveFailure(id string, manifest store.Manifest, cause error) (store.Manifest, error) {
	manifest.ArchiveState = archivePartial
	writeErr := m.Store.WriteManifest(id, manifest)
	eventErr := appendOperationEvent(m.Store, id, "archive", "partial", "archive retry required")
	if writeErr != nil {
		cause = errors.Join(cause, writeErr)
	}
	if eventErr != nil {
		cause = errors.Join(cause, eventErr)
	}
	return manifest, &store.Error{
		Kind:      store.KindPartial,
		Message:   "Task archive is incomplete",
		Retryable: true,
		Recovery:  fmt.Sprintf("Retry `akagent task archive %s`", id),
		Err:       cause,
	}
}

// Clean removes only resources owned by the task. It archives first, checks
// the verified tmux identity, and records worktree and credential cleanup
// independently so one failed resource can be retried without losing debt.
func (m *Manager) Clean(id string, options CleanupOptions) (store.Manifest, error) {
	m.operationMu.Lock()
	defer m.operationMu.Unlock()

	manifest, err := m.manifest(id)
	if err != nil {
		return store.Manifest{}, err
	}
	if manifest.CleanupState == cleanupComplete {
		return manifest, nil
	}
	live, available, err := m.taskLive(id)
	if err != nil {
		return store.Manifest{}, err
	}
	if !available {
		return store.Manifest{}, errors.New("task process observation is unavailable")
	}
	if live {
		return store.Manifest{}, liveTaskError("cleaning")
	}
	if manifest.Lifecycle != "stopped" && manifest.Lifecycle != "finished" {
		return store.Manifest{}, fmt.Errorf("task must be stopped or finished before cleaning")
	}
	repository, err := m.Store.ReadRepository(manifest.Repository)
	if err != nil {
		return store.Manifest{}, err
	}
	if manifest.ArchiveState != archiveComplete {
		if _, err := m.archive(id); err != nil {
			return store.Manifest{}, err
		}
		manifest, err = m.manifest(id)
		if err != nil {
			return store.Manifest{}, err
		}
	}

	manifest.CleanupState = cleanupPending
	manifest.CleanupDebt = true
	if err := m.Store.WriteManifest(id, manifest); err != nil {
		return store.Manifest{}, err
	}
	if err := appendOperationEvent(m.Store, id, "cleanup", "intent", "cleanup requested"); err != nil {
		return m.cleanupFailure(id, manifest, err)
	}

	var failures []error
	if manifest.CredentialCleanupState != cleanupComplete {
		credentialCleanup := m.CleanupCredentials
		if credentialCleanup == nil {
			credentialCleanup = func(store.Manifest) error { return nil }
		}
		if err := credentialCleanup(manifest); err != nil {
			manifest.CredentialCleanupState = cleanupPartial
			failures = append(failures, errors.New("credential cleanup failed"))
			if eventErr := appendOperationEvent(m.Store, id, "cleanup_credentials", "partial", cleanupDebtEvent); eventErr != nil {
				failures = append(failures, errors.New("credential cleanup debt event could not be recorded"))
			}
		} else {
			manifest.CredentialCleanupState = cleanupComplete
			if eventErr := appendOperationEvent(m.Store, id, "cleanup_credentials", "succeeded", "credential cleanup complete"); eventErr != nil {
				failures = append(failures, errors.New("credential cleanup event could not be recorded"))
			}
		}
	}

	facts := manifest.Git
	if facts.Path == "" && m.GitFacts != nil {
		if observed, observeErr := m.GitFacts(manifest); observeErr == nil {
			facts = observed
			manifest.Git = facts
		}
	}
	if preservation := missingAuthorization(facts, options); preservation != "" {
		manifest.WorktreeCleanupState = cleanupBlocked
		failures = append(failures, &store.Error{
			Kind:     store.KindPreservation,
			Message:  preservation,
			Recovery: "Inspect the archived Git facts and retry with explicit cleanup authorization",
		})
		if eventErr := appendOperationEvent(m.Store, id, "cleanup_worktree", "preservation_required", "recovery debt preserved"); eventErr != nil {
			failures = append(failures, errors.New("worktree preservation event could not be recorded"))
		}
	} else if repository.Policy == "worktree" && !options.AllowWorktree {
		manifest.WorktreeCleanupState = cleanupBlocked
		failures = append(failures, &store.Error{
			Kind:     store.KindPreservation,
			Message:  "Cleanup requires explicit authorization to remove the task worktree",
			Recovery: "Inspect the archived Git facts and retry with --allow-worktree",
		})
		if eventErr := appendOperationEvent(m.Store, id, "cleanup_worktree", "preservation_required", "worktree approval required"); eventErr != nil {
			failures = append(failures, errors.New("worktree preservation event could not be recorded"))
		}
	} else if manifest.WorktreeCleanupState != cleanupComplete {
		worktreeCleanup := m.CleanupWorktree
		if worktreeCleanup == nil {
			worktreeCleanup = func(store.Manifest, store.GitFacts) error { return nil }
		}
		if err := worktreeCleanup(manifest, facts); err != nil {
			manifest.WorktreeCleanupState = cleanupPartial
			failures = append(failures, err)
			if eventErr := appendOperationEvent(m.Store, id, "cleanup_worktree", "partial", cleanupDebtEvent); eventErr != nil {
				failures = append(failures, errors.New("worktree cleanup debt event could not be recorded"))
			}
		} else {
			manifest.WorktreeCleanupState = cleanupComplete
			if eventErr := appendOperationEvent(m.Store, id, "cleanup_worktree", "succeeded", "worktree cleanup complete"); eventErr != nil {
				failures = append(failures, errors.New("worktree cleanup event could not be recorded"))
			}
		}
	}

	if len(failures) > 0 {
		manifest.CleanupDebt = true
		manifest.CleanupState = cleanupPartial
		if err := m.Store.WriteManifest(id, manifest); err != nil {
			failures = append(failures, err)
		}
		kind := store.KindPartial
		message := "Task cleanup is incomplete"
		retryable := true
		recovery := fmt.Sprintf("Retry `akagent task clean %s` after resolving the reported debt", id)
		for _, failure := range failures {
			var typed *store.Error
			switch {
			case errors.As(failure, &typed) && typed.Kind == store.KindPreservation:
				kind = store.KindPreservation
				message = typed.Message
				retryable = false
			case errors.As(failure, &typed) && typed.Kind == store.KindConflict:
				kind = store.KindConflict
				message = typed.Message
				retryable = false
			}
			if kind != store.KindPartial {
				if typed != nil && typed.Recovery != "" {
					recovery = typed.Recovery
				}
				break
			}
		}
		result := &store.Error{Kind: kind, Message: message, Retryable: retryable, Recovery: recovery}
		if kind == store.KindPartial {
			result.Err = errors.Join(failures...)
		}
		return manifest, result
	}

	manifest.CleanupDebt = false
	manifest.CleanupState = cleanupComplete
	if err := m.Store.WriteManifest(id, manifest); err != nil {
		return m.cleanupFailure(id, manifest, err)
	}
	if err := appendOperationEvent(m.Store, id, "cleanup", "succeeded", "cleanup complete"); err != nil {
		return m.cleanupFailure(id, manifest, err)
	}
	return manifest, nil
}

func (m *Manager) cleanupFailure(id string, manifest store.Manifest, cause error) (store.Manifest, error) {
	manifest.CleanupDebt = true
	manifest.CleanupState = cleanupPartial
	writeErr := m.Store.WriteManifest(id, manifest)
	if writeErr != nil {
		cause = errors.Join(cause, writeErr)
	}
	return manifest, &store.Error{
		Kind:      store.KindPartial,
		Message:   "Task cleanup is incomplete",
		Retryable: true,
		Recovery:  fmt.Sprintf("Retry `akagent task clean %s`", id),
		Err:       cause,
	}
}

func (m *Manager) removeWorktree(manifest store.Manifest, _ store.GitFacts) error {
	repository, err := m.Store.ReadRepository(manifest.Repository)
	if err != nil {
		return err
	}
	if repository.Policy != "worktree" {
		return nil
	}
	root := repositoryWorktreeRoot(repository)
	path, err := filepath.Abs(manifest.WorktreePath)
	if err != nil || path == repository.Path || !within(path, root) {
		return &store.Error{
			Kind:     store.KindConflict,
			Message:  "Task worktree is outside its registered cleanup root",
			Recovery: "Run `akagent task reconcile` and inspect the task before retrying cleanup",
		}
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return errors.New("task worktree could not be inspected")
	}
	status, err := m.Git.Status(path)
	if err != nil || !status.Exists || !m.worktreeMatches(repository, manifest, status, false) {
		return &store.Error{
			Kind:     store.KindConflict,
			Message:  "Task worktree does not match its durable ownership",
			Recovery: "Run `akagent task reconcile` and inspect the task before retrying cleanup",
		}
	}
	if err := m.Git.RemoveWorktree(repository.Path, path); err != nil {
		return err
	}
	return nil
}

func (m *Manager) taskLive(id string) (bool, bool, error) {
	observation, err := m.Tmux.Observe(id)
	if err != nil {
		return false, false, err
	}
	if !observation.Available {
		return false, false, nil
	}
	return len(observation.Processes) > 0, true, nil
}

func liveTaskError(operation string) error {
	return &store.Error{
		Kind:      store.KindLocked,
		Message:   "Task process is still running",
		Retryable: true,
		Recovery:  fmt.Sprintf("Stop the task, then retry %s", operation),
	}
}

func missingAuthorization(facts store.GitFacts, options CleanupOptions) string {
	switch {
	case facts.Committed && !options.AllowCommitted:
		return "Cleanup requires authorization to discard committed worktree changes"
	case facts.Dirty && !options.AllowDirty:
		return "Cleanup requires authorization to discard dirty worktree changes"
	case facts.Untracked && !options.AllowUntracked:
		return "Cleanup requires authorization to discard untracked worktree files"
	default:
		return ""
	}
}

func appendOperationEvent(state *store.Store, id, operation, outcome, detail string) error {
	_, err := state.AppendEvent(id, store.Event{Operation: operation, Outcome: outcome, Detail: detail})
	return err
}

func (m *Manager) archiveGitFacts(manifest store.Manifest) (store.GitFacts, []string) {
	if m.GitFacts == nil {
		return store.GitFacts{}, []string{"Git facts unavailable"}
	}
	facts, err := m.GitFacts(manifest)
	if err != nil {
		return store.GitFacts{}, []string{"Git facts unavailable"}
	}
	return facts, nil
}

func (m *Manager) inspectGitFacts(manifest store.Manifest) (store.GitFacts, error) {
	repository, err := m.Store.ReadRepository(manifest.Repository)
	if err != nil {
		return store.GitFacts{}, err
	}
	path := manifest.Git.Path
	if path == "" {
		path = repository.Path
		if repository.Policy == "worktree" {
			path = manifest.WorktreePath
		}
	}
	head, err := gitOutput(path, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return store.GitFacts{}, errors.New("Git HEAD could not be observed")
	}
	branch, _ := gitOutput(path, "symbolic-ref", "--short", "-q", "HEAD")
	status, err := gitOutput(path, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return store.GitFacts{}, errors.New("Git status could not be observed")
	}
	facts := store.GitFacts{Path: path, Head: head, Branch: branch, Committed: manifest.BaseRevision != "" && head != manifest.BaseRevision}
	for _, line := range strings.Split(status, "\n") {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "##") {
			continue
		}
		if strings.HasPrefix(line, "??") {
			facts.Untracked = true
		} else {
			facts.Dirty = true
		}
	}
	return facts, nil
}

func gitOutput(path string, args ...string) (string, error) {
	commandArgs := append([]string{"-C", path}, args...)
	output, err := exec.Command("git", commandArgs...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (m *Manager) captureTerminalHistory(id string) (string, error) {
	provider, ok := m.Tmux.(interface{ Capture(string) (string, error) })
	if !ok {
		return "", errors.New("terminal history is unavailable")
	}
	return provider.Capture(id)
}

func (commandTmux) Capture(id string) (string, error) {
	output, err := exec.Command("tmux", "list-windows", "-a", "-F", "#{window_id}\t#{@akagent_task_id}").Output()
	if err != nil {
		return "", errors.New("terminal history is unavailable")
	}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) == 2 && fields[1] == id {
			captured, captureErr := exec.Command("tmux", "capture-pane", "-p", "-S", "-", "-t", fields[0]).Output()
			if captureErr != nil {
				return "", errors.New("terminal history is unavailable")
			}
			return string(captured), nil
		}
	}
	return "", errors.New("terminal history is unavailable")
}
