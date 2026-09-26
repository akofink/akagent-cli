package lifecycle

import (
	"errors"
	"fmt"
	"strings"

	"github.com/akofink/akagent-cli/internal/store"
	"github.com/google/uuid"
)

func (m *Manager) CreateExecution(taskID string, request ExecutionRequest) (store.Execution, bool, error) {
	if taskID == "" || request.Target == "" {
		return store.Execution{}, false, fmt.Errorf("task ID and execution target are required")
	}
	manifest, err := m.Inspect(taskID)
	if err != nil {
		return store.Execution{}, false, err
	}
	if manifest.Provenance == store.ProvenanceExternal {
		return store.Execution{}, false, externalRecordOperationError("task", taskID)
	}
	if request.ID == "" {
		id, err := uuid.NewV7()
		if err != nil {
			return store.Execution{}, false, errors.New("failed to generate an execution ID")
		}
		request.ID = id.String()
	}
	if request.Label == "" {
		request.Label = request.Target
	}
	if strings.ContainsAny(request.Label, "\r\n") || strings.TrimSpace(request.Label) == "" {
		return store.Execution{}, false, fmt.Errorf("execution label must be a non-empty single line")
	}
	execution := store.Execution{
		ID: request.ID, TaskID: taskID, Label: request.Label, Target: request.Target,
		Command: request.Command, Arguments: append([]string(nil), request.Arguments...),
		Requirements: strings.Join(unique(request.Requirements), ","), ResourceID: request.ResourceID,
		WorkingDirectory: request.WorkingDirectory, Lifecycle: "created", Condition: "none", HeartbeatAt: m.now(),
	}
	created, existing, err := m.Store.CreateExecution(taskID, execution)
	if err != nil {
		return store.Execution{}, false, err
	}
	if _, err := m.Store.UpdateManifest(taskID, func(task *store.Manifest) error {
		task.ExecutionIDs = appendResourceID(task.ExecutionIDs, request.ID)
		return nil
	}); err != nil {
		return store.Execution{}, false, err
	}
	if created {
		if _, err := m.Store.AppendExecutionEvent(taskID, request.ID, store.Event{Operation: "create", Outcome: "intent"}); err != nil {
			return store.Execution{}, false, err
		}
	}
	return existing, created, nil
}

func (m *Manager) ListExecutions(taskID string) ([]store.Execution, error) {
	manifest, err := m.Inspect(taskID)
	if err != nil {
		return nil, err
	}
	if manifest.ExecutionIDs == "" {
		return []store.Execution{}, nil
	}
	ids := unique(strings.Split(manifest.ExecutionIDs, ","))
	executions := make([]store.Execution, 0, len(ids))
	for _, id := range ids {
		execution, err := m.Store.ReadExecution(taskID, id)
		if err != nil {
			return nil, err
		}
		executions = append(executions, execution)
	}
	return executions, nil
}

func (m *Manager) InspectExecution(taskID, executionID string) (store.Execution, error) {
	executions, err := m.ListExecutions(taskID)
	if err != nil {
		return store.Execution{}, err
	}
	if executionID == "" {
		if len(executions) != 1 {
			return store.Execution{}, fmt.Errorf("execution ID is required when a task has multiple executions")
		}
		return executions[0], nil
	}
	for _, execution := range executions {
		if execution.ID == executionID {
			return execution, nil
		}
	}
	return store.Execution{}, &store.Error{Kind: store.KindNotFound, Message: fmt.Sprintf("execution %s not found", executionID)}
}

func (m *Manager) AddExecutionSessionReference(taskID, executionID string, reference store.SessionReference) (store.Execution, error) {
	if reference.Tool == "" || reference.SessionID == "" {
		return store.Execution{}, fmt.Errorf("session tool and ID are required")
	}
	changed := false
	execution, err := m.Store.UpdateExecution(taskID, executionID, func(execution *store.Execution) error {
		for _, existing := range execution.SessionReferences {
			if existing == reference {
				return nil
			}
		}
		execution.SessionReferences = append(execution.SessionReferences, reference)
		changed = true
		return nil
	})
	if err != nil {
		return store.Execution{}, err
	}
	if changed {
		if _, err := m.Store.AppendExecutionEvent(taskID, executionID, store.Event{Operation: "session_reference", Outcome: "recorded"}); err != nil {
			return store.Execution{}, err
		}
	}
	return execution, nil
}

func (m *Manager) RecordExecutionSessionReference(taskID, executionID string, reference store.SessionReference) (store.Execution, error) {
	return m.AddExecutionSessionReference(taskID, executionID, reference)
}

func (m *Manager) PublishExecution(taskID, executionID, condition, reason, activity string) (store.Execution, error) {
	return m.PublishExecutionRecord(taskID, executionID, condition, reason, activity)
}
