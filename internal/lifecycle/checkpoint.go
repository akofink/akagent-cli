package lifecycle

import (
	"fmt"

	"github.com/akofink/akagent-cli/internal/store"
)

// CheckpointRequest contains only state to acknowledge for a task. It never
// starts a process, inspects Git or tmux, or reads provider-owned content.
type CheckpointRequest struct {
	IdempotencyKey   string
	ExpectedRevision uint64
	Checkpoint       store.Checkpoint
}

// WriteCheckpoint acknowledges one revision-checked recovery checkpoint.
// Repeating the same idempotency key and payload is an acknowledged no-op.
func (m *Manager) WriteCheckpoint(taskID string, request CheckpointRequest) (store.Checkpoint, error) {
	if taskID == "" {
		return store.Checkpoint{}, validationError("task ID is required")
	}
	request.Checkpoint.TaskID = ""
	return m.Store.WriteCheckpoint(taskID, store.CheckpointWrite{
		IdempotencyKey:   request.IdempotencyKey,
		ExpectedRevision: request.ExpectedRevision,
		Checkpoint:       request.Checkpoint,
	})
}

// CheckpointInspection is independent from process, Git, tmux, providers, and
// network observations. An absent checkpoint is explicitly unavailable.
type CheckpointInspection struct {
	Available  bool
	State      string
	Reason     string
	Checkpoint *store.Checkpoint
}

func (m *Manager) InspectCheckpoint(taskID string) (CheckpointInspection, error) {
	if _, err := m.Store.ReadManifest(taskID); err != nil {
		return CheckpointInspection{}, err
	}
	checkpoint, err := m.Store.ReadCheckpoint(taskID)
	if err == nil {
		return CheckpointInspection{Available: true, State: "available", Checkpoint: &checkpoint}, nil
	}
	if store.IsKind(err, store.KindNotFound) {
		return CheckpointInspection{
			Available: false,
			State:     "unavailable",
			Reason:    fmt.Sprintf("No acknowledged recovery checkpoint exists for task %s", taskID),
		}, nil
	}
	return CheckpointInspection{}, err
}
