package store

import "errors"

const externalHistoryLimit = 1024

// rollbackRecord retains the original failure unless cleanup also fails.
// In that case the persisted state is uncertain and callers must reconcile it.
func rollbackRecord(cause error, cleanups ...func() error) error {
	var failures []error
	for _, cleanup := range cleanups {
		if err := cleanup(); err != nil {
			failures = append(failures, err)
		}
	}
	if len(failures) == 0 {
		return cause
	}
	return &Error{
		Kind: KindPartial, Message: "record rollback failed",
		Recovery: "Inspect and reconcile the task before retrying the operation",
		Err:      errors.Join(append([]error{cause}, failures...)...),
	}
}

type ExternalResourceRequest struct {
	ID               string
	OperationID      string
	CallerID         string
	Repository       string
	Branch           string
	BaseRevision     string
	Head             string
	WorktreePath     string
	Metadata         map[string]string
	ExpectedRevision uint64
}

type ExternalExecutionRequest struct {
	ID                string
	OperationID       string
	CallerID          string
	ResourceID        string
	PredecessorID     string
	SessionReferences []SessionReference
}

type ExternalTaskRequest struct {
	Title       string
	CallerID    string
	OperationID string
}

type HandoffDispositionRequest struct {
	SuccessorExecutionID string
	OperationID          string
	ExpectedRevision     uint64
}
