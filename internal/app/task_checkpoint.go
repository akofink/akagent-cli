package app

import (
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/akofink/akagent-cli/internal/lifecycle"
	"github.com/akofink/akagent-cli/internal/store"
)

func taskCheckpointCommand(args []string, stdout io.Writer, manager *lifecycle.Manager) int {
	if len(args) == 0 {
		return writeError(stdout, "usage", "Usage: akagent task checkpoint <write|inspect> <task-id> ...", false, "Inspect a task checkpoint or write the next acknowledged revision")
	}
	switch args[0] {
	case "write":
		if len(args) < 2 {
			return writeError(stdout, "usage", "Usage: akagent task checkpoint write <task-id> --idempotency-key <key> --expected-revision <revision> --task-kind <kind> --completion-contract <contract> --next-action <action> [--context <purpose=reference>] [--resource-reference <id>] [--session-reference <execution-id=tool:session-id>] [--previous-attempt-reference <reference>] [--verification-scope <scope>] [--verification-revision <revision>] [--verification-result <result>] [--verification-time <RFC3339>] [--uncertain-operation <key=operation>]", false, "Use a new idempotency key and the last acknowledged revision")
		}
		request, ok := parseCheckpointWrite(args[2:])
		if !ok {
			return writeError(stdout, "usage", "Usage: akagent task checkpoint write <task-id> --idempotency-key <key> --expected-revision <revision> --task-kind <kind> --completion-contract <contract> --next-action <action> [--context <purpose=reference>] [--resource-reference <id>] [--session-reference <execution-id=tool:session-id>] [--previous-attempt-reference <reference>] [--verification-scope <scope>] [--verification-revision <revision>] [--verification-result <result>] [--verification-time <RFC3339>] [--uncertain-operation <key=operation>]", false, "Provide bounded, non-secret checkpoint fields")
		}
		checkpoint, err := manager.WriteCheckpoint(args[1], request)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, checkpointDetailView{Available: true, State: "acknowledged", Checkpoint: viewCheckpoint(&checkpoint)})
	case "inspect":
		if len(args) != 2 {
			return writeError(stdout, "usage", "Usage: akagent task checkpoint inspect <task-id>", false, "Provide a task ID")
		}
		inspection, err := manager.InspectCheckpoint(args[1])
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, checkpointDetailView{Available: inspection.Available, State: inspection.State, Reason: inspection.Reason, Checkpoint: viewCheckpoint(inspection.Checkpoint)})
	default:
		return writeError(stdout, "usage", "Usage: akagent task checkpoint <write|inspect> <task-id> ...", false, "Inspect a task checkpoint or write the next acknowledged revision")
	}
}

type checkpointCLIRequest struct {
	Request              lifecycle.CheckpointRequest
	expectedRevisionSet  bool
	verificationScope    string
	verificationRevision string
	verificationResult   string
	verificationTime     time.Time
	verificationSet      bool
}

func parseCheckpointWrite(args []string) (lifecycle.CheckpointRequest, bool) {
	var parsed checkpointCLIRequest
	for len(args) > 0 {
		if len(args) < 2 {
			return lifecycle.CheckpointRequest{}, false
		}
		flag, value := args[0], args[1]
		args = args[2:]
		switch flag {
		case "--idempotency-key", "--key":
			parsed.Request.IdempotencyKey = value
		case "--expected-revision":
			revision, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				return lifecycle.CheckpointRequest{}, false
			}
			parsed.Request.ExpectedRevision = revision
			parsed.expectedRevisionSet = true
		case "--task-kind":
			parsed.Request.Checkpoint.TaskKind = value
		case "--completion-contract":
			parsed.Request.Checkpoint.CompletionContract = value
		case "--next-action":
			parsed.Request.Checkpoint.NextAction = value
		case "--context", "--context-reference":
			purpose, reference, ok := strings.Cut(value, "=")
			if !ok || purpose == "" || reference == "" {
				return lifecycle.CheckpointRequest{}, false
			}
			parsed.Request.Checkpoint.ContextReferences = append(parsed.Request.Checkpoint.ContextReferences, store.ContextReference{Purpose: purpose, Reference: reference})
		case "--resource-reference":
			parsed.Request.Checkpoint.ResourceReferences = append(parsed.Request.Checkpoint.ResourceReferences, store.CheckpointResourceReference{ResourceID: value})
		case "--session-reference":
			executionID, session, ok := strings.Cut(value, "=")
			if !ok {
				return lifecycle.CheckpointRequest{}, false
			}
			parts := strings.SplitN(session, ":", 2)
			if executionID == "" || len(parts) != 2 || parts[0] == "" || parts[1] == "" {
				return lifecycle.CheckpointRequest{}, false
			}
			parsed.Request.Checkpoint.SessionReferences = append(parsed.Request.Checkpoint.SessionReferences, store.CheckpointSessionReference{ExecutionID: executionID, Tool: parts[0], SessionID: parts[1]})
		case "--previous-attempt-reference":
			parsed.Request.Checkpoint.PreviousAttemptReferences = append(parsed.Request.Checkpoint.PreviousAttemptReferences, value)
		case "--verification-scope":
			parsed.verificationScope, parsed.verificationSet = value, true
		case "--verification-revision":
			parsed.verificationRevision, parsed.verificationSet = value, true
		case "--verification-result":
			parsed.verificationResult, parsed.verificationSet = value, true
		case "--verification-time":
			verifiedAt, err := time.Parse(time.RFC3339Nano, value)
			if err != nil {
				return lifecycle.CheckpointRequest{}, false
			}
			parsed.verificationTime, parsed.verificationSet = verifiedAt.UTC(), true
		case "--uncertain-operation":
			key, operation, ok := strings.Cut(value, "=")
			if !ok || key == "" || operation == "" {
				return lifecycle.CheckpointRequest{}, false
			}
			parsed.Request.Checkpoint.UncertainExternalOperations = append(parsed.Request.Checkpoint.UncertainExternalOperations, store.UncertainExternalOperation{ReferenceKey: key, Operation: operation, Verify: "verify the external system before replaying"})
		default:
			return lifecycle.CheckpointRequest{}, false
		}
	}
	if parsed.verificationSet {
		parsed.Request.Checkpoint.Verification = &store.CheckpointVerification{Scope: parsed.verificationScope, Revision: parsed.verificationRevision, Result: parsed.verificationResult, VerifiedAt: parsed.verificationTime}
	}
	return parsed.Request, parsed.expectedRevisionSet && parsed.Request.IdempotencyKey != "" && parsed.Request.Checkpoint.TaskKind != "" && parsed.Request.Checkpoint.CompletionContract != "" && parsed.Request.Checkpoint.NextAction != ""
}

func viewCheckpoint(checkpoint *store.Checkpoint) *checkpointView {
	if checkpoint == nil {
		return nil
	}
	return &checkpointView{
		TaskID:                      checkpoint.TaskID,
		Revision:                    checkpoint.Revision,
		IdempotencyKey:              checkpoint.IdempotencyKey,
		AcknowledgedAt:              checkpoint.AcknowledgedAt,
		AuditDebt:                   checkpoint.AuditDebt,
		TaskKind:                    checkpoint.TaskKind,
		CompletionContract:          checkpoint.CompletionContract,
		NextAction:                  checkpoint.NextAction,
		ContextReferences:           checkpoint.ContextReferences,
		ResourceReferences:          checkpoint.ResourceReferences,
		SessionReferences:           checkpoint.SessionReferences,
		PreviousAttemptReferences:   checkpoint.PreviousAttemptReferences,
		Verification:                checkpoint.Verification,
		UncertainExternalOperations: checkpoint.UncertainExternalOperations,
	}
}

type checkpointView struct {
	TaskID                      string                              `json:"task_id"`
	Revision                    uint64                              `json:"revision"`
	IdempotencyKey              string                              `json:"idempotency_key"`
	AcknowledgedAt              time.Time                           `json:"acknowledged_at"`
	AuditDebt                   []store.CheckpointAuditDebt         `json:"audit_debt,omitempty"`
	TaskKind                    string                              `json:"task_kind"`
	CompletionContract          string                              `json:"completion_contract"`
	NextAction                  string                              `json:"next_action"`
	ContextReferences           []store.ContextReference            `json:"context_references,omitempty"`
	ResourceReferences          []store.CheckpointResourceReference `json:"resource_references,omitempty"`
	SessionReferences           []store.CheckpointSessionReference  `json:"session_references,omitempty"`
	PreviousAttemptReferences   []string                            `json:"previous_attempt_references,omitempty"`
	Verification                *store.CheckpointVerification       `json:"verification,omitempty"`
	UncertainExternalOperations []store.UncertainExternalOperation  `json:"uncertain_external_operations,omitempty"`
}

type checkpointDetailView struct {
	Available  bool            `json:"available"`
	State      string          `json:"state"`
	Reason     string          `json:"reason,omitempty"`
	Checkpoint *checkpointView `json:"checkpoint,omitempty"`
}
