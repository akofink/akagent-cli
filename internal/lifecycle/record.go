package lifecycle

import (
	"fmt"

	"github.com/akofink/akagent-cli/internal/store"
)

// RecordTask adopts a task identity without consulting host state.
func (m *Manager) RecordTask(taskID string, request store.ExternalTaskRequest) (store.Manifest, error) {
	return m.Store.AdoptExternalTask(taskID, request)
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

func externalRecordOperationError(kind, id string) error {
	return &store.Error{Kind: store.KindConflict, Message: fmt.Sprintf("%s %s is an externally declared record", kind, id), Recovery: "Use `akagent task record` so the operation remains state-only"}
}
