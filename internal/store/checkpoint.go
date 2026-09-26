package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const KindCheckpoint = "checkpoint"

const (
	maxCheckpointText       = 4096
	maxCheckpointReferences = 64
)

// Checkpoint is the authoritative, provider-neutral handoff for one task.
// It records intent and verification metadata, never process memory or
// provider-owned content.
type Checkpoint struct {
	TaskID                      string                        `json:"task_id"`
	Revision                    uint64                        `json:"revision"`
	IdempotencyKey              string                        `json:"idempotency_key"`
	IdempotencyHistory          []CheckpointIdempotencyRecord `json:"idempotency_history,omitempty"`
	AuditDebt                   []CheckpointAuditDebt         `json:"audit_debt,omitempty"`
	AcknowledgedAt              time.Time                     `json:"acknowledged_at"`
	TaskKind                    string                        `json:"task_kind"`
	CompletionContract          string                        `json:"completion_contract"`
	NextAction                  string                        `json:"next_action"`
	ContextReferences           []ContextReference            `json:"context_references,omitempty"`
	ResourceReferences          []CheckpointResourceReference `json:"resource_references,omitempty"`
	SessionReferences           []CheckpointSessionReference  `json:"session_references,omitempty"`
	PreviousAttemptReferences   []string                      `json:"previous_attempt_references,omitempty"`
	Verification                *CheckpointVerification       `json:"verification,omitempty"`
	UncertainExternalOperations []UncertainExternalOperation  `json:"uncertain_external_operations,omitempty"`
}

// CheckpointIdempotencyRecord is a durable receipt for one acknowledged
// operation. The payload digest prevents an old key from being reused with a
// changed payload after later checkpoint revisions.
type CheckpointIdempotencyRecord struct {
	KeyDigest      string    `json:"key_digest"`
	Revision       uint64    `json:"revision"`
	AcknowledgedAt time.Time `json:"acknowledged_at"`
	PayloadDigest  string    `json:"payload_digest"`
}

// CheckpointAuditDebt identifies an acknowledged revision whose audit event
// still needs verification or repair. It is cleared only after the exact
// revision event is present.
type CheckpointAuditDebt struct {
	Revision  uint64 `json:"revision"`
	KeyDigest string `json:"key_digest"`
}

// ContextReference points to an external handoff or document without
// embedding its contents. Missing references remain valid declarations.
type ContextReference struct {
	Purpose   string    `json:"purpose"`
	Reference string    `json:"reference"`
	FreshAt   time.Time `json:"fresh_at,omitempty"`
	Primary   bool      `json:"primary,omitempty"`
}

// CheckpointResourceReference identifies a resource and preserves the latest
// non-secret observations available when the checkpoint was acknowledged.
type CheckpointResourceReference struct {
	ResourceID   string `json:"resource_id"`
	Repository   string `json:"repository,omitempty"`
	Branch       string `json:"branch,omitempty"`
	BaseRevision string `json:"base_revision,omitempty"`
	Head         string `json:"head,omitempty"`
	WorktreePath string `json:"worktree_path,omitempty"`
	Dirty        bool   `json:"dirty,omitempty"`
	Untracked    bool   `json:"untracked,omitempty"`
}

// CheckpointSessionReference records provider-neutral session provenance.
// The core does not open or parse the referenced provider artifact.
type CheckpointSessionReference struct {
	ExecutionID   string `json:"execution_id"`
	Tool          string `json:"tool"`
	SessionID     string `json:"session_id"`
	ReferencePath string `json:"reference_path,omitempty"`
}

// CheckpointVerification records the last verification scope and revision.
type CheckpointVerification struct {
	Scope      string    `json:"scope"`
	Revision   string    `json:"revision"`
	Result     string    `json:"result"`
	VerifiedAt time.Time `json:"verified_at"`
}

// UncertainExternalOperation is an operation whose outcome is unknown. The
// verify instruction is deliberately declarative and is never executed.
type UncertainExternalOperation struct {
	ReferenceKey string `json:"reference_key"`
	Operation    string `json:"operation"`
	Verify       string `json:"verify"`
}

// CheckpointWrite supplies the caller's idempotency and optimistic concurrency
// guards. ExpectedRevision is zero when no checkpoint has been acknowledged.
type CheckpointWrite struct {
	IdempotencyKey   string
	ExpectedRevision uint64
	Checkpoint       Checkpoint
}

func checkpointEnvelope(taskID string, checkpoint Checkpoint) (Envelope, error) {
	envelope, err := newEnvelope(KindCheckpoint, taskID, checkpoint)
	return envelope, err
}

func (s *Store) WriteCheckpoint(taskID string, request CheckpointWrite) (Checkpoint, error) {
	if err := validateTaskID(taskID); err != nil {
		return Checkpoint{}, err
	}
	if request.Checkpoint.TaskID != "" && request.Checkpoint.TaskID != taskID {
		return Checkpoint{}, newError(KindUsage, "checkpoint task ID does not match its path", "Retry with the requested task ID")
	}
	if err := validateCheckpointWrite(request); err != nil {
		return Checkpoint{}, err
	}
	returned := Checkpoint{}
	err := s.WithLock(taskID, func() error {
		if _, err := s.ReadManifest(taskID); err != nil {
			return err
		}
		current, err := s.ReadCheckpoint(taskID)
		if err != nil && !IsKind(err, KindNotFound) {
			return err
		}
		if IsKind(err, KindNotFound) {
			current = Checkpoint{}
		}
		payloadDigest := checkpointPayloadDigest(request.Checkpoint)
		if current.Revision != 0 {
			if receipt, found := findCheckpointReceipt(current, request.IdempotencyKey); found {
				if receipt.PayloadDigest != payloadDigest {
					return &Error{Kind: KindConflict, Message: "checkpoint idempotency key conflicts with an acknowledged payload", Recovery: fmt.Sprintf("Inspect the checkpoint for task %s and retry with the original payload or a new idempotency key", taskID)}
				}
				if err := s.repairCheckpointAuditLocked(taskID, &current, receipt); err != nil {
					return err
				}
				returned = request.Checkpoint
				returned.TaskID = taskID
				returned.Revision = receipt.Revision
				returned.IdempotencyKey = request.IdempotencyKey
				returned.AcknowledgedAt = receipt.AcknowledgedAt
				returned.IdempotencyHistory = current.IdempotencyHistory
				returned.AuditDebt = current.AuditDebt
				return nil
			}
		}
		if current.Revision != request.ExpectedRevision {
			return &Error{Kind: KindConflict, Message: fmt.Sprintf("checkpoint revision %d does not match expected revision %d", current.Revision, request.ExpectedRevision), Recovery: fmt.Sprintf("Inspect the checkpoint for task %s and retry with its acknowledged revision", taskID)}
		}
		desired := request.Checkpoint
		desired.TaskID = taskID
		desired.IdempotencyKey = request.IdempotencyKey
		desired.Revision = current.Revision + 1
		desired.AcknowledgedAt = time.Now().UTC()
		desired.IdempotencyHistory = append(append([]CheckpointIdempotencyRecord(nil), current.IdempotencyHistory...), CheckpointIdempotencyRecord{KeyDigest: checkpointKeyDigest(request.IdempotencyKey), Revision: desired.Revision, AcknowledgedAt: desired.AcknowledgedAt, PayloadDigest: payloadDigest})
		desired.AuditDebt = append(append([]CheckpointAuditDebt(nil), current.AuditDebt...), CheckpointAuditDebt{Revision: desired.Revision, KeyDigest: checkpointKeyDigest(request.IdempotencyKey)})
		if err := validateStoredCheckpoint(desired, taskID); err != nil {
			return err
		}
		nextEvent, err := s.nextSequence(taskID)
		if err != nil {
			return err
		}
		if err := s.persistCheckpointLocked(taskID, desired); err != nil {
			return err
		}
		if err := s.writeCheckpointEventLocked(taskID, nextEvent, checkpointEvent(desired)); err != nil {
			return &Error{Kind: KindPartial, Message: fmt.Sprintf("Acknowledged checkpoint revision %d but could not record its event", desired.Revision), Retryable: true, Recovery: fmt.Sprintf("Inspect checkpoint for task %s and retry the same idempotency key", taskID), Err: err}
		}
		desired.AuditDebt = removeCheckpointAuditDebt(desired.AuditDebt, desired.Revision, checkpointKeyDigest(request.IdempotencyKey))
		if err := s.persistCheckpointLocked(taskID, desired); err != nil {
			return &Error{Kind: KindPartial, Message: fmt.Sprintf("Recorded checkpoint revision %d but could not clear its event audit debt", desired.Revision), Retryable: true, Recovery: fmt.Sprintf("Inspect checkpoint for task %s and retry the same idempotency key", taskID), Err: err}
		}
		returned = desired
		return nil
	})
	return returned, err
}

func (s *Store) validateCheckpointForRecovery(taskID string, result *RecoveryResult) error {
	path := s.checkpointPath(taskID)
	data, err := s.readOwnedFile(path)
	if err != nil {
		if !IsKind(err, KindNotFound) {
			result.MalformedRecords = append(result.MalformedRecords, err.Error())
		}
		return nil
	}
	envelope, err := decodeEnvelope(path, data, KindCheckpoint, taskID)
	if err != nil {
		result.MalformedRecords = append(result.MalformedRecords, path)
		return nil
	}
	checkpoint, err := envelope.DecodeCheckpoint()
	if err != nil || validateStoredCheckpoint(checkpoint, taskID) != nil {
		result.MalformedRecords = append(result.MalformedRecords, path)
	}
	return nil
}

func (s *Store) ReadCheckpoint(taskID string) (Checkpoint, error) {
	if err := validateTaskID(taskID); err != nil {
		return Checkpoint{}, err
	}
	path := s.checkpointPath(taskID)
	if err := s.checkTaskDir(taskID); err != nil {
		return Checkpoint{}, err
	}
	data, err := s.readOwnedFile(path)
	if err != nil {
		if IsKind(err, KindNotFound) {
			return Checkpoint{}, newError(KindNotFound, fmt.Sprintf("No recovery checkpoint found for task %s", taskID), fmt.Sprintf("Create a checkpoint for task %s before relying on reboot recovery", taskID))
		}
		return Checkpoint{}, err
	}
	envelope, err := decodeEnvelope(path, data, KindCheckpoint, taskID)
	if err != nil {
		return Checkpoint{}, err
	}
	checkpoint, err := envelope.DecodeCheckpoint()
	if err != nil {
		return Checkpoint{}, malformedError(fmt.Sprintf("Malformed recovery checkpoint for task %s", taskID), fmt.Sprintf("Inspect and repair %s", path))
	}
	if err := validateStoredCheckpoint(checkpoint, taskID); err != nil {
		return Checkpoint{}, malformedError(fmt.Sprintf("Malformed recovery checkpoint for task %s", taskID), fmt.Sprintf("Inspect and repair %s", path))
	}
	return checkpoint, nil
}

func validateCheckpointWrite(request CheckpointWrite) error {
	if err := validateSingleLine(request.IdempotencyKey, "checkpoint idempotency key"); err != nil {
		return err
	}
	checkpoint := request.Checkpoint
	if checkpoint.Revision != 0 || !checkpoint.AcknowledgedAt.IsZero() || checkpoint.IdempotencyKey != "" || len(checkpoint.IdempotencyHistory) != 0 || len(checkpoint.AuditDebt) != 0 {
		return newError(KindUsage, "checkpoint revision, acknowledgement time, idempotency key, idempotency history, and audit debt are assigned by the store", "Retry without store-assigned checkpoint fields")
	}
	return validateCheckpointFields(checkpoint)
}

func validateStoredCheckpoint(checkpoint Checkpoint, taskID string) error {
	if checkpoint.TaskID != taskID || checkpoint.Revision == 0 || checkpoint.AcknowledgedAt.IsZero() {
		return newError(KindUsage, "checkpoint identity or acknowledgement fields are invalid", "Retry with a valid task checkpoint")
	}
	if err := validateSingleLine(checkpoint.IdempotencyKey, "checkpoint idempotency key"); err != nil {
		return err
	}
	return validateCheckpointFields(checkpoint)
}

func validateCheckpointFields(checkpoint Checkpoint) error {
	if err := validateBoundedText(checkpoint.TaskKind, "task kind", true); err != nil {
		return err
	}
	if err := validateBoundedText(checkpoint.CompletionContract, "completion contract", true); err != nil {
		return err
	}
	if err := validateBoundedText(checkpoint.NextAction, "next action", true); err != nil {
		return err
	}
	if len(checkpoint.ContextReferences) > maxCheckpointReferences || len(checkpoint.ResourceReferences) > maxCheckpointReferences || len(checkpoint.SessionReferences) > maxCheckpointReferences || len(checkpoint.PreviousAttemptReferences) > maxCheckpointReferences || len(checkpoint.UncertainExternalOperations) > maxCheckpointReferences || len(checkpoint.AuditDebt) > maxCheckpointReferences {
		return newError(KindUsage, "checkpoint contains too many references or audit debts", "Use at most 64 values of each bounded type")
	}
	seenKeys := make(map[string]struct{}, len(checkpoint.IdempotencyHistory))
	receiptsByRevision := make(map[uint64]string, len(checkpoint.IdempotencyHistory))
	for _, record := range checkpoint.IdempotencyHistory {
		if err := validateCheckpointReceipt(record, checkpoint.Revision, seenKeys); err != nil {
			return err
		}
		receiptsByRevision[record.Revision] = record.KeyDigest
	}
	for _, debt := range checkpoint.AuditDebt {
		if len(debt.KeyDigest) != sha256.Size*2 {
			return newError(KindMalformed, "checkpoint audit debt contains an invalid key digest", "Inspect and repair the checkpoint record")
		}
		if _, err := hex.DecodeString(debt.KeyDigest); err != nil || debt.Revision == 0 || debt.Revision > checkpoint.Revision {
			return newError(KindMalformed, "checkpoint audit debt contains an invalid revision", "Inspect and repair the checkpoint record")
		}
		if receiptsByRevision[debt.Revision] != debt.KeyDigest {
			return newError(KindMalformed, "checkpoint audit debt does not identify an acknowledged receipt", "Inspect and repair the checkpoint record")
		}
	}
	for _, reference := range checkpoint.ContextReferences {
		if err := validateBoundedText(reference.Purpose, "context reference purpose", true); err != nil {
			return err
		}
		if err := validateBoundedText(reference.Reference, "context reference", true); err != nil {
			return err
		}
	}
	for _, reference := range checkpoint.ResourceReferences {
		if err := validateBoundedText(reference.ResourceID, "resource reference ID", true); err != nil {
			return err
		}
		if err := validateBoundedText(reference.Repository, "resource repository", false); err != nil {
			return err
		}
		if err := validateBoundedText(reference.Branch, "resource branch", false); err != nil {
			return err
		}
		if err := validateBoundedText(reference.BaseRevision, "resource base revision", false); err != nil {
			return err
		}
		if err := validateBoundedText(reference.Head, "resource head", false); err != nil {
			return err
		}
		if err := validateBoundedText(reference.WorktreePath, "resource worktree path", false); err != nil {
			return err
		}
	}
	for _, reference := range checkpoint.SessionReferences {
		if err := validateBoundedText(reference.ExecutionID, "session execution ID", true); err != nil {
			return err
		}
		if err := validateBoundedText(reference.Tool, "session tool", true); err != nil {
			return err
		}
		if err := validateBoundedText(reference.SessionID, "session ID", true); err != nil {
			return err
		}
		if err := validateBoundedText(reference.ReferencePath, "session reference path", false); err != nil {
			return err
		}
	}
	for _, reference := range checkpoint.PreviousAttemptReferences {
		if err := validateBoundedText(reference, "previous attempt reference", true); err != nil {
			return err
		}
	}
	if checkpoint.Verification != nil {
		if err := validateBoundedText(checkpoint.Verification.Scope, "verification scope", true); err != nil {
			return err
		}
		if err := validateBoundedText(checkpoint.Verification.Revision, "verification revision", true); err != nil {
			return err
		}
		if err := validateBoundedText(checkpoint.Verification.Result, "verification result", true); err != nil {
			return err
		}
		if checkpoint.Verification.VerifiedAt.IsZero() {
			return newError(KindUsage, "verification time is required when verification is provided", "Retry with --verification-time in RFC3339 format")
		}
	}
	for _, operation := range checkpoint.UncertainExternalOperations {
		if err := validateBoundedText(operation.ReferenceKey, "uncertain operation reference key", true); err != nil {
			return err
		}
		if err := validateBoundedText(operation.Operation, "uncertain operation", true); err != nil {
			return err
		}
		if err := validateBoundedText(operation.Verify, "uncertain operation verification", true); err != nil {
			return err
		}
	}
	return nil
}

func validateBoundedText(value, name string, required bool) error {
	if required && strings.TrimSpace(value) == "" {
		return newError(KindUsage, fmt.Sprintf("%s is required", name), fmt.Sprintf("Retry with a non-empty %s", name))
	}
	if len(value) > maxCheckpointText || strings.ContainsAny(value, "\r\n\x00") {
		return newError(KindUsage, fmt.Sprintf("%s must be a bounded single-line value", name), fmt.Sprintf("Retry with a %s no longer than %d bytes", name, maxCheckpointText))
	}
	return nil
}

func validateSingleLine(value, name string) error {
	return validateBoundedText(value, name, true)
}

func (e Envelope) DecodeCheckpoint() (Checkpoint, error) {
	var checkpoint Checkpoint
	if !isObjectPayload(e.Data) || json.Unmarshal(e.Data, &checkpoint) != nil {
		return Checkpoint{}, malformedError("Malformed recovery checkpoint payload", "Inspect and repair the recovery checkpoint")
	}
	if checkpoint.TaskID == "" || checkpoint.TaskID != e.TaskID {
		return Checkpoint{}, malformedError("Recovery checkpoint is missing or has mismatched task identity", "Inspect and repair the recovery checkpoint")
	}
	return checkpoint, nil
}

func (s *Store) persistCheckpointLocked(taskID string, checkpoint Checkpoint) error {
	if err := validateStoredCheckpoint(checkpoint, taskID); err != nil {
		return err
	}
	envelope, err := checkpointEnvelope(taskID, checkpoint)
	if err != nil {
		return internalError("encode a recovery checkpoint", "Retry the operation")
	}
	encoded, err := encodeRecord(envelope)
	if err != nil {
		return err
	}
	return s.atomicallyWrite(s.checkpointPath(taskID), encoded)
}

func (s *Store) writeCheckpointEventLocked(taskID string, sequence int, event Event) error {
	if s.checkpointEventFn != nil {
		return s.checkpointEventFn(taskID, event)
	}
	return s.appendEventLocked(taskID, sequence, event)
}

func (s *Store) repairCheckpointAuditLocked(taskID string, checkpoint *Checkpoint, receipt CheckpointIdempotencyRecord) error {
	events, err := s.ReadEvents(taskID)
	if err != nil {
		return checkpointAuditPartial(taskID, receipt.Revision, "cannot inspect its event audit", err)
	}
	found := false
	for _, record := range events {
		if record.Event.Operation != "checkpoint" || record.Event.Outcome != "acknowledged" {
			continue
		}
		if record.Event.Detail == checkpointEventDetail(receipt) || record.Event.Detail == fmt.Sprintf("revision %d", receipt.Revision) {
			found = true
			break
		}
	}
	if !found {
		next, err := s.nextSequence(taskID)
		if err != nil {
			return checkpointAuditPartial(taskID, receipt.Revision, "cannot repair its event audit", err)
		}
		if err := s.writeCheckpointEventLocked(taskID, next, checkpointEventFromReceipt(receipt)); err != nil {
			return checkpointAuditPartial(taskID, receipt.Revision, "event audit remains incomplete", err)
		}
	}
	before := len(checkpoint.AuditDebt)
	checkpoint.AuditDebt = removeCheckpointAuditDebt(checkpoint.AuditDebt, receipt.Revision, receipt.KeyDigest)
	if len(checkpoint.AuditDebt) == before {
		return nil
	}
	if err := s.persistCheckpointLocked(taskID, *checkpoint); err != nil {
		return checkpointAuditPartial(taskID, receipt.Revision, "cannot clear its event audit debt", err)
	}
	return nil
}

func checkpointAuditPartial(taskID string, revision uint64, detail string, err error) error {
	return &Error{Kind: KindPartial, Message: fmt.Sprintf("Checkpoint revision %d is durable but %s", revision, detail), Retryable: true, Recovery: fmt.Sprintf("Inspect task %s and retry the same idempotency key", taskID), Err: err}
}

func (s *Store) appendEventLocked(taskID string, sequence int, event Event) error {
	if err := s.ensureTaskDir(taskID); err != nil {
		return err
	}
	envelope, err := eventEnvelope(taskID, event)
	if err != nil {
		return internalError("encode a task event", "Retry the operation")
	}
	encoded, err := encodeRecord(envelope)
	if err != nil {
		return err
	}
	return s.atomicallyWrite(s.eventPath(taskID, sequence), encoded)
}

func checkpointKeyDigest(key string) string {
	digest := sha256.Sum256([]byte(key))
	return hex.EncodeToString(digest[:])
}

func checkpointPayloadDigest(checkpoint Checkpoint) string {
	checkpoint.TaskID = ""
	checkpoint.Revision = 0
	checkpoint.IdempotencyKey = ""
	checkpoint.IdempotencyHistory = nil
	checkpoint.AuditDebt = nil
	checkpoint.AcknowledgedAt = time.Time{}
	encoded, _ := json.Marshal(checkpoint)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func findCheckpointReceipt(checkpoint Checkpoint, key string) (CheckpointIdempotencyRecord, bool) {
	digest := checkpointKeyDigest(key)
	for _, record := range checkpoint.IdempotencyHistory {
		if record.KeyDigest == digest {
			return record, true
		}
	}
	return CheckpointIdempotencyRecord{}, false
}

func validateCheckpointReceipt(record CheckpointIdempotencyRecord, currentRevision uint64, seen map[string]struct{}) error {
	if len(record.KeyDigest) != sha256.Size*2 {
		return newError(KindMalformed, "checkpoint idempotency history contains an invalid key digest", "Inspect and repair the checkpoint record")
	}
	if _, err := hex.DecodeString(record.KeyDigest); err != nil || record.Revision == 0 || record.Revision > currentRevision {
		return newError(KindMalformed, "checkpoint idempotency history contains an invalid revision", "Inspect and repair the checkpoint record")
	}
	if record.PayloadDigest == "" && record.AcknowledgedAt.IsZero() {
		// Checkpoints written before receipt metadata was introduced remain
		// readable, but cannot provide historical payload comparison.
		seen[record.KeyDigest] = struct{}{}
		return nil
	}
	if record.AcknowledgedAt.IsZero() || len(record.PayloadDigest) != sha256.Size*2 {
		return newError(KindMalformed, "checkpoint idempotency history contains an invalid receipt", "Inspect and repair the checkpoint record")
	}
	if _, err := hex.DecodeString(record.PayloadDigest); err != nil {
		return newError(KindMalformed, "checkpoint idempotency history contains an invalid payload digest", "Inspect and repair the checkpoint record")
	}
	if _, exists := seen[record.KeyDigest]; exists {
		return newError(KindMalformed, "checkpoint idempotency history contains a duplicate key", "Inspect and repair the checkpoint record")
	}
	seen[record.KeyDigest] = struct{}{}
	return nil
}

func checkpointEvent(checkpoint Checkpoint) Event {
	receipt, _ := findCheckpointReceipt(checkpoint, checkpoint.IdempotencyKey)
	return checkpointEventFromReceipt(receipt)
}

func checkpointEventFromReceipt(receipt CheckpointIdempotencyRecord) Event {
	return Event{Operation: "checkpoint", Outcome: "acknowledged", Detail: fmt.Sprintf("revision %d key %s", receipt.Revision, receipt.KeyDigest)}
}

func checkpointEventDetail(receipt CheckpointIdempotencyRecord) string {
	return fmt.Sprintf("revision %d key %s", receipt.Revision, receipt.KeyDigest)
}

func removeCheckpointAuditDebt(debts []CheckpointAuditDebt, revision uint64, keyDigest string) []CheckpointAuditDebt {
	remaining := make([]CheckpointAuditDebt, 0, len(debts))
	for _, debt := range debts {
		if debt.Revision != revision || debt.KeyDigest != keyDigest {
			remaining = append(remaining, debt)
		}
	}
	if len(remaining) == 0 {
		return nil
	}
	return remaining
}
