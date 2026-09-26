package store

import (
	"fmt"
	"os"
	"time"
)

func (s *Store) RecordExternalExecution(taskID string, request ExternalExecutionRequest) (Execution, error) {
	if err := validateTaskID(taskID); err != nil {
		return Execution{}, err
	}
	if err := validateExternalExecutionRequest(request); err != nil {
		return Execution{}, err
	}
	var result Execution
	fingerprint := operationFingerprint(request)
	err := s.WithLock(taskID, func() error {
		manifest, err := s.readManifestLocked(taskID)
		if err != nil {
			return err
		}
		originalManifest := manifest
		if manifest.Provenance != ProvenanceExternal {
			return externalOwnershipConflict("task", taskID)
		}
		if manifest.CallerID != request.CallerID {
			return externalCallerConflict("task", taskID)
		}
		resource, err := s.ReadResource(taskID, request.ResourceID)
		if err != nil {
			return err
		}
		if resource.Provenance != ProvenanceExternal {
			return externalOwnershipConflict("resource", request.ResourceID)
		}
		if resource.CallerID != request.CallerID {
			return externalCallerConflict("resource", request.ResourceID)
		}
		if request.PredecessorID != "" {
			if err := s.validatePredecessorLocked(taskID, request.ID, request.PredecessorID, request.CallerID, request.ResourceID); err != nil {
				return err
			}
		}
		current, readErr := s.ReadExecution(taskID, request.ID)
		if readErr == nil {
			if current.Provenance != ProvenanceExternal {
				return externalOwnershipConflict("execution", request.ID)
			}
			if current.CallerID != request.CallerID {
				return externalCallerConflict("execution", request.ID)
			}
			if _, found, receiptErr := findReceipt(current.Receipts, request.OperationID, fingerprint); receiptErr != nil {
				return receiptErr
			} else if found {
				if err := s.ensureExternalTaskChildProjectionLocked(taskID, request.CallerID, request.OperationID, fingerprint, "", request.ID, "record_execution"); err != nil {
					return err
				}
				if err := s.ensureExecutionRecordEventLocked(taskID, request.ID, request.OperationID, fingerprint, current.Revision); err != nil {
					return err
				}
				result = current
				return nil
			}
			if err := s.ensureExecutionReceiptHistoryLocked(taskID, request.ID, current.Receipts); err != nil {
				return err
			}
			if err := s.ensureTaskReceiptHistoryLocked(taskID, manifest.Receipts); err != nil {
				if _, found, receiptErr := findReceipt(manifest.Receipts, request.OperationID, fingerprint); receiptErr != nil || !found {
					return err
				}
			}
			if current.ArchiveState == "complete" || current.Lifecycle == "finished" {
				return terminalMutationConflict("execution", request.ID)
			}
			if current.CallerID != request.CallerID || current.ResourceID != request.ResourceID || current.PredecessorID != request.PredecessorID || !sameSessionReferences(current.SessionReferences, request.SessionReferences) {
				return newError(KindConflict, fmt.Sprintf("external execution %s inputs conflict with its existing record", request.ID), "Inspect the execution and retry with its original immutable inputs")
			}
			current.Receipts = appendReceipt(current.Receipts, request.OperationID, fingerprint, current.Revision)
			if err := s.writeExecutionLockedUnvalidatedPaths(taskID, current); err != nil {
				return err
			}
			if err := s.appendExecutionEventLocked(taskID, request.ID, Event{Operation: "record", Outcome: "recorded", OperationID: request.OperationID, Fingerprint: fingerprint, Revision: current.Revision}); err != nil {
				return err
			}
			result = current
			return nil
		}
		if !IsKind(readErr, KindNotFound) {
			return readErr
		}
		if err := s.ensureTaskReceiptHistoryLocked(taskID, manifest.Receipts); err != nil {
			return err
		}
		if externalTaskTerminal(manifest) {
			return terminalMutationConflict("task", taskID)
		}
		if resource.ArchiveState == "complete" {
			return terminalMutationConflict("resource", request.ResourceID)
		}
		execution := Execution{
			ID: request.ID, TaskID: taskID, Provenance: ProvenanceExternal, CallerID: request.CallerID, Revision: 1,
			PredecessorID: request.PredecessorID, Label: "external", Target: "external", ResourceID: request.ResourceID,
			SessionReferences: append([]SessionReference(nil), request.SessionReferences...), Lifecycle: "created", Condition: "none",
			Receipts: []RecordReceipt{{OperationID: request.OperationID, Fingerprint: fingerprint, Revision: 1}},
		}
		if err := s.writeExecutionLockedUnvalidatedPaths(taskID, execution); err != nil {
			return rollbackRecord(err, func() error { return os.RemoveAll(s.executionDir(taskID, request.ID)) })
		}
		if err := s.appendExecutionEventLocked(taskID, request.ID, Event{Operation: "record", Outcome: "created", OperationID: request.OperationID, Fingerprint: fingerprint, Revision: execution.Revision}); err != nil {
			return rollbackRecord(err, func() error { return os.RemoveAll(s.executionDir(taskID, request.ID)) })
		}
		manifest.ExecutionIDs = appendCSV(manifest.ExecutionIDs, request.ID)
		manifest.Provenance = ProvenanceExternal
		if manifest.CallerID == "" {
			manifest.CallerID = request.CallerID
		}
		if manifest.Revision == 0 {
			manifest.Revision = 1
		} else {
			manifest.Revision++
		}
		manifest.Receipts, err = appendReceiptText(manifest.Receipts, request.OperationID, fingerprint, manifest.Revision)
		if err != nil {
			return rollbackRecord(err, func() error { return os.RemoveAll(s.executionDir(taskID, request.ID)) })
		}
		if err := s.writeManifestLocked(taskID, manifest); err != nil {
			return rollbackRecord(err, func() error { return os.RemoveAll(s.executionDir(taskID, request.ID)) })
		}
		if err := s.appendRecordEventLocked(taskID, Event{Operation: "record_execution", Outcome: "created", OperationID: request.OperationID, Fingerprint: fingerprint, Revision: manifest.Revision}); err != nil {
			return rollbackRecord(err,
				func() error { return s.writeManifestLocked(taskID, originalManifest) },
				func() error { return os.RemoveAll(s.executionDir(taskID, request.ID)) },
			)
		}
		result = execution
		return nil
	})
	return result, err
}

func (s *Store) ObserveExternalExecution(taskID, executionID, callerID, operationID string, expectedRevision uint64, observation ExternalObservation) (Execution, error) {
	if err := validateExternalObservation(observation); err != nil {
		return Execution{}, err
	}
	if err := validateOperationID(operationID); err != nil {
		return Execution{}, err
	}
	var result Execution
	fingerprint := operationFingerprint(struct {
		CallerID    string
		Observation ExternalObservation
	}{callerID, observation})
	err := s.WithLock(taskID, func() error {
		execution, err := s.ReadExecution(taskID, executionID)
		if err != nil {
			return err
		}
		if err := validateExternalExecutionCaller(execution, callerID); err != nil {
			return err
		}
		if _, found, receiptErr := findReceipt(execution.Receipts, operationID, fingerprint); receiptErr != nil {
			return receiptErr
		} else if found {
			if err := s.ensureExecutionRecordEventLocked(taskID, executionID, operationID, fingerprint, execution.Revision); err != nil {
				return err
			}
			result = execution
			return nil
		}
		if err := s.ensureExecutionReceiptHistoryLocked(taskID, executionID, execution.Receipts); err != nil {
			return err
		}
		if execution.ArchiveState == "complete" || execution.Lifecycle == "finished" {
			return terminalMutationConflict("execution", executionID)
		}
		if err := expectedRevisionCheck(expectedRevision, execution.Revision, "execution", executionID); err != nil {
			return err
		}
		if len(execution.ExternalObservations) > 0 && execution.ExternalObservations[len(execution.ExternalObservations)-1] == observation {
			execution.Receipts = appendReceipt(execution.Receipts, operationID, fingerprint, execution.Revision)
			if err := s.writeExecutionLockedUnvalidatedPaths(taskID, execution); err != nil {
				return err
			}
			if err := s.appendExecutionEventLocked(taskID, executionID, Event{Operation: "observe", Outcome: "duplicate", OperationID: operationID, Fingerprint: fingerprint, Revision: execution.Revision}); err != nil {
				return err
			}
			result = execution
			return nil
		}
		if len(execution.ExternalObservations) >= externalHistoryLimit {
			return newError(KindConflict, "external execution observation history limit reached", "Archive or reopen the record before recording another observation")
		}
		execution.ExternalObservations = append(execution.ExternalObservations, observation)
		execution.Revision++
		execution.Receipts = appendReceipt(execution.Receipts, operationID, fingerprint, execution.Revision)
		if err := s.writeExecutionLockedUnvalidatedPaths(taskID, execution); err != nil {
			return err
		}
		if err := s.appendExecutionEventLocked(taskID, executionID, Event{Operation: "observe", Outcome: "recorded", OperationID: operationID, Fingerprint: fingerprint, Revision: execution.Revision}); err != nil {
			return err
		}
		result = execution
		return nil
	})
	return result, err
}

func (s *Store) CompleteExternalExecution(taskID, executionID, callerID, operationID, contract, resultValue string, expectedRevision uint64) (Execution, error) {
	if err := validateCompletion(contract, resultValue, callerID); err != nil {
		return Execution{}, err
	}
	if err := validateOperationID(operationID); err != nil {
		return Execution{}, err
	}
	var result Execution
	fingerprint := operationFingerprint(struct{ CallerID, Contract, Result string }{callerID, contract, resultValue})
	err := s.WithLock(taskID, func() error {
		execution, err := s.ReadExecution(taskID, executionID)
		if err != nil {
			return err
		}
		if execution.Provenance != ProvenanceExternal {
			return s.finishManagedExecutionLocked(taskID, execution, callerID, operationID, contract, resultValue, expectedRevision, fingerprint, &result)
		}
		if err := validateExternalExecutionCaller(execution, callerID); err != nil {
			return err
		}
		if _, found, receiptErr := findReceipt(execution.Receipts, operationID, fingerprint); receiptErr != nil {
			return receiptErr
		} else if found {
			if err := s.ensureExecutionRecordEventLocked(taskID, executionID, operationID, fingerprint, execution.Revision); err != nil {
				return err
			}
			result = execution
			return nil
		}
		if execution.ExternalCompletion != nil && completionMatches(execution.ExternalCompletion, contract, resultValue, callerID) {
			execution.Receipts = appendReceipt(execution.Receipts, operationID, fingerprint, execution.Revision)
			if err := s.writeExecutionLockedUnvalidatedPaths(taskID, execution); err != nil {
				return err
			}
			if err := s.appendExecutionEventLocked(taskID, executionID, Event{Operation: "complete", Outcome: "duplicate", OperationID: operationID, Fingerprint: fingerprint, Revision: execution.Revision}); err != nil {
				return err
			}
			result = execution
			return nil
		}
		if err := s.ensureExecutionReceiptHistoryLocked(taskID, executionID, execution.Receipts); err != nil {
			return err
		}
		if execution.ExternalCompletion != nil || execution.ArchiveState == "complete" {
			return terminalMutationConflict("execution", executionID)
		}
		if err := expectedRevisionCheck(expectedRevision, execution.Revision, "execution", executionID); err != nil {
			return err
		}
		now := time.Now().UTC()
		execution.ExternalCompletion = &ExternalCompletion{Contract: contract, Result: resultValue, CallerID: callerID, DeclaredAt: now}
		execution.Lifecycle, execution.Condition, execution.Result = "finished", "none", resultValue
		execution.Revision++
		execution.Receipts = appendReceipt(execution.Receipts, operationID, fingerprint, execution.Revision)
		if err := s.writeExecutionLockedUnvalidatedPaths(taskID, execution); err != nil {
			return err
		}
		if err := s.appendExecutionEventLocked(taskID, executionID, Event{Operation: "complete", Outcome: "declared", OperationID: operationID, Fingerprint: fingerprint, Revision: execution.Revision}); err != nil {
			return err
		}
		result = execution
		return nil
	})
	return result, err
}

// finishManagedExecutionLocked closes a managed execution without relabeling
// it as caller-owned. The parent task may still be in flight. Completion is
// explicit and is not inferred from host state.
func (s *Store) finishManagedExecutionLocked(taskID string, execution Execution, callerID, operationID, contract, resultValue string, expectedRevision uint64, fingerprint string, result *Execution) error {
	if _, found, receiptErr := findReceipt(execution.Receipts, operationID, fingerprint); receiptErr != nil {
		return receiptErr
	} else if found {
		if err := s.ensureExecutionRecordEventLocked(taskID, execution.ID, operationID, fingerprint, execution.Revision); err != nil {
			return err
		}
		*result = execution
		return nil
	}
	if execution.ExternalCompletion != nil && completionMatches(execution.ExternalCompletion, contract, resultValue, callerID) {
		execution.Receipts = appendReceipt(execution.Receipts, operationID, fingerprint, execution.Revision)
		if err := s.writeExecutionLockedUnvalidatedPaths(taskID, execution); err != nil {
			return err
		}
		if err := s.appendExecutionEventLocked(taskID, execution.ID, Event{Operation: "complete", Outcome: "duplicate", OperationID: operationID, Fingerprint: fingerprint, Revision: execution.Revision}); err != nil {
			return err
		}
		*result = execution
		return nil
	}
	if err := s.ensureExecutionReceiptHistoryLocked(taskID, execution.ID, execution.Receipts); err != nil {
		return err
	}
	if execution.ExternalCompletion != nil || execution.Lifecycle == "finished" || execution.ArchiveState == "complete" {
		return terminalMutationConflict("execution", execution.ID)
	}
	if err := expectedRevisionCheck(expectedRevision, execution.Revision, "execution", execution.ID); err != nil {
		return err
	}
	now := time.Now().UTC()
	execution.ExternalCompletion = &ExternalCompletion{Contract: contract, Result: resultValue, CallerID: callerID, DeclaredAt: now}
	execution.Lifecycle, execution.Condition, execution.Result = "finished", "none", resultValue
	execution.Revision++
	execution.Receipts = appendReceipt(execution.Receipts, operationID, fingerprint, execution.Revision)
	if err := s.writeExecutionLockedUnvalidatedPaths(taskID, execution); err != nil {
		return err
	}
	if err := s.appendExecutionEventLocked(taskID, execution.ID, Event{Operation: "complete", Outcome: "declared", OperationID: operationID, Fingerprint: fingerprint, Revision: execution.Revision}); err != nil {
		return err
	}
	*result = execution
	return nil
}

// DisposeManagedExecutionHandoff terminalizes a managed predecessor record only
// when its published handoff state and an active managed successor agree.
// Independent host verification remains the successor's responsibility.
func (s *Store) DisposeManagedExecutionHandoff(taskID, executionID string, request HandoffDispositionRequest) (Execution, error) {
	if err := validateExecutionID(request.SuccessorExecutionID); err != nil {
		return Execution{}, err
	}
	if err := validateOperationID(request.OperationID); err != nil {
		return Execution{}, err
	}
	if executionID == request.SuccessorExecutionID {
		return Execution{}, newError(KindConflict, "A managed execution cannot hand off to itself", "Name a distinct active successor execution")
	}
	var result Execution
	fingerprint := operationFingerprint(struct{ SuccessorExecutionID string }{request.SuccessorExecutionID})
	err := s.WithLock(taskID, func() error {
		manifest, err := s.readManifestLocked(taskID)
		if err != nil {
			return err
		}
		if manifest.Lifecycle == "finished" || manifest.ArchiveState == "complete" {
			return terminalMutationConflict("task", taskID)
		}
		predecessor, err := s.ReadExecution(taskID, executionID)
		if err != nil {
			return err
		}
		if predecessor.Provenance == ProvenanceExternal {
			return externalOwnershipConflict("execution", executionID)
		}
		if _, found, receiptErr := findReceipt(predecessor.Receipts, request.OperationID, fingerprint); receiptErr != nil {
			return receiptErr
		} else if found {
			if err := s.ensureExecutionRecordEventLocked(taskID, executionID, request.OperationID, fingerprint, predecessor.Revision); err != nil {
				return err
			}
			result = predecessor
			return nil
		}
		if err := s.ensureExecutionReceiptHistoryLocked(taskID, executionID, predecessor.Receipts); err != nil {
			return err
		}
		successor, err := s.ReadExecution(taskID, request.SuccessorExecutionID)
		if err != nil {
			return err
		}
		if successor.Provenance == ProvenanceExternal || successor.Lifecycle == "finished" || successor.Condition != "active" {
			return newError(KindConflict, "Successor execution is not an active managed execution", "Verify the successor takeover and inspect its active execution record")
		}
		if predecessor.HandoffDisposition != nil || predecessor.ExternalCompletion != nil || predecessor.Lifecycle == "finished" || predecessor.ArchiveState == "complete" {
			return terminalMutationConflict("execution", executionID)
		}
		if predecessor.Condition != "waiting" || predecessor.Activity != "handed off" {
			return newError(KindConflict, "Predecessor execution is not published as handed off", "Require the predecessor condition waiting and activity handed off")
		}
		if err := expectedRevisionCheck(request.ExpectedRevision, predecessor.Revision, "execution", executionID); err != nil {
			return err
		}
		now := time.Now().UTC()
		predecessor.HandoffDisposition = &HandoffDisposition{SuccessorExecutionID: request.SuccessorExecutionID, VerifiedAt: now}
		predecessor.Lifecycle, predecessor.Condition = "finished", "none"
		predecessor.Result = "handed_off"
		predecessor.Activity = "handed off to successor " + request.SuccessorExecutionID
		predecessor.HeartbeatAt = now
		predecessor.Revision++
		predecessor.Receipts = appendReceipt(predecessor.Receipts, request.OperationID, fingerprint, predecessor.Revision)
		if err := s.writeExecutionLockedUnvalidatedPaths(taskID, predecessor); err != nil {
			return err
		}
		if err := s.appendExecutionEventLocked(taskID, executionID, Event{Operation: "handoff_disposition", Outcome: "declared", Detail: request.SuccessorExecutionID, OperationID: request.OperationID, Fingerprint: fingerprint, Revision: predecessor.Revision}); err != nil {
			return err
		}
		result = predecessor
		return nil
	})
	return result, err
}

func (s *Store) ArchiveExternalExecution(taskID, executionID, callerID, operationID string, expectedRevision uint64) (ExecutionArchive, error) {
	if err := validateCallerID(callerID); err != nil {
		return ExecutionArchive{}, err
	}
	if err := validateOperationID(operationID); err != nil {
		return ExecutionArchive{}, err
	}
	var result ExecutionArchive
	fingerprint := operationFingerprint(struct{ CallerID string }{callerID})
	err := s.WithLock(taskID, func() error {
		execution, err := s.ReadExecution(taskID, executionID)
		if err != nil {
			return err
		}
		if err := validateExternalExecutionCaller(execution, callerID); err != nil {
			return err
		}
		if _, found, receiptErr := findReceipt(execution.Receipts, operationID, fingerprint); receiptErr != nil {
			return receiptErr
		} else if found {
			if err := s.ensureExecutionRecordEventLocked(taskID, executionID, operationID, fingerprint, execution.Revision); err != nil {
				return err
			}
			archive, archiveErr := s.ReadExecutionArchive(taskID, executionID)
			if archiveErr != nil {
				events, eventsErr := s.ReadExecutionEvents(taskID, executionID)
				if eventsErr != nil {
					return eventsErr
				}
				archive = ExecutionArchive{TaskID: taskID, ExecutionID: executionID, CapturedAt: time.Now().UTC(), Execution: execution, Events: events}
				if archiveErr = s.writeExecutionArchiveLocked(taskID, executionID, archive); archiveErr != nil {
					return archiveErr
				}
			}
			result = archive
			return nil
		}
		if err := validateExternalExecutionCaller(execution, callerID); err != nil {
			return err
		}
		if execution.ArchiveState == "complete" {
			if existing, readErr := s.ReadExecutionArchive(taskID, executionID); readErr == nil {
				result = existing
				return nil
			}
			return newError(KindPartial, fmt.Sprintf("external execution %s archive is incomplete", executionID), "Repair the missing execution archive before retrying")
		}
		if err := expectedRevisionCheck(expectedRevision, execution.Revision, "execution", executionID); err != nil {
			return err
		}
		if existing, readErr := s.ReadExecutionArchive(taskID, executionID); readErr == nil && execution.ArchiveState == "complete" {
			result = existing
			return nil
		}
		if execution.Lifecycle != "finished" {
			return newError(KindConflict, fmt.Sprintf("external execution %s must be explicitly completed before archive", executionID), "Record completion against an external completion contract first")
		}
		execution.ArchiveState = "complete"
		execution.Revision++
		execution.Receipts = appendReceipt(execution.Receipts, operationID, fingerprint, execution.Revision)
		if err := s.writeExecutionLockedUnvalidatedPaths(taskID, execution); err != nil {
			return err
		}
		if err := s.appendExecutionEventLocked(taskID, executionID, Event{Operation: "archive", Outcome: "recorded", OperationID: operationID, Fingerprint: fingerprint, Revision: execution.Revision}); err != nil {
			return err
		}
		events, err := s.ReadExecutionEvents(taskID, executionID)
		if err != nil {
			return err
		}
		archive := ExecutionArchive{TaskID: taskID, ExecutionID: executionID, CapturedAt: time.Now().UTC(), Execution: execution, Events: events}
		if err := s.writeExecutionArchiveLocked(taskID, executionID, archive); err != nil {
			return err
		}
		result = archive
		return nil
	})
	return result, err
}

func (s *Store) validatePredecessorLocked(taskID, executionID, predecessorID, callerID, resourceID string) error {
	seen := map[string]bool{executionID: true}
	current := predecessorID
	for current != "" {
		if seen[current] {
			return newError(KindConflict, "external execution predecessor lineage is cyclic", "Choose a predecessor outside the existing lineage")
		}
		seen[current] = true
		execution, err := s.ReadExecution(taskID, current)
		if err != nil {
			return err
		}
		if execution.Provenance != ProvenanceExternal || execution.CallerID != callerID || execution.ResourceID != resourceID {
			return newError(KindConflict, "external execution predecessor crosses caller or resource ownership", "Choose a predecessor from the same external caller and resource")
		}
		current = execution.PredecessorID
	}
	return nil
}

func sameSessionReferences(a, b []SessionReference) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if a[index] != b[index] {
			return false
		}
	}
	return true
}

func (s *Store) writeExecutionArchiveLocked(taskID, executionID string, archive ExecutionArchive) error {
	envelope, err := executionArchiveEnvelope(taskID, executionID, archive)
	if err != nil {
		return internalError("encode an execution archive", "Retry the operation")
	}
	encoded, err := encodeRecord(envelope)
	if err != nil {
		return err
	}
	return s.atomicallyWrite(s.executionArchivePath(taskID, executionID), encoded)
}

func (s *Store) writeExecutionLockedUnvalidatedPaths(taskID string, execution Execution) error {
	if err := validateStoredExecution(execution); err != nil {
		return err
	}
	if err := s.ensureExecutionDir(taskID, execution.ID); err != nil {
		return err
	}
	envelope, err := executionManifestEnvelope(taskID, execution.ID, execution)
	if err != nil {
		return internalError("encode an execution manifest", "Retry the operation")
	}
	encoded, err := encodeRecord(envelope)
	if err != nil {
		return err
	}
	return s.atomicallyWrite(s.executionManifestPath(taskID, execution.ID), encoded)
}

func (s *Store) listExecutionsLocked(taskID string) ([]Execution, error) {
	ids, err := s.ExecutionIDs(taskID)
	if err != nil {
		return nil, err
	}
	result := make([]Execution, 0, len(ids))
	for _, id := range ids {
		execution, err := s.ReadExecution(taskID, id)
		if err != nil {
			return nil, err
		}
		result = append(result, execution)
	}
	return result, nil
}
