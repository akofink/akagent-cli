package store

import (
	"fmt"
	"os"
	"time"
)

// CreateExternalTask creates a caller-owned task record without consulting
// host state or relabeling an existing managed task.
func (s *Store) CreateExternalTask(taskID string, request ExternalTaskRequest) (Manifest, error) {
	if err := validateTaskID(taskID); err != nil {
		return Manifest{}, err
	}
	if err := validateExternalTaskRequest(request); err != nil {
		return Manifest{}, err
	}
	var result Manifest
	fingerprint := operationFingerprint(struct {
		Title    string
		CallerID string
	}{request.Title, request.CallerID})
	err := s.WithLock(taskID, func() error {
		manifest, readErr := s.readManifestLocked(taskID)
		if readErr == nil {
			if manifest.Provenance != ProvenanceExternal {
				return externalOwnershipConflict("task", taskID)
			}
			if manifest.CallerID != request.CallerID {
				return externalCallerConflict("task", taskID)
			}
			if _, found, receiptErr := findReceipt(manifest.Receipts, request.OperationID, fingerprint); receiptErr != nil {
				return receiptErr
			} else if found {
				result = manifest
				return nil
			}
			if manifest.Title != request.Title {
				return newError(KindConflict, fmt.Sprintf("external task %s inputs conflict with its existing record", taskID), "Inspect the task and retry with its original immutable inputs")
			}
			if externalTaskTerminal(manifest) {
				return terminalMutationConflict("task", taskID)
			}
			manifest.Receipts, readErr = appendReceiptText(manifest.Receipts, request.OperationID, fingerprint, manifest.Revision)
			if readErr != nil {
				return readErr
			}
			if err := s.writeManifestLocked(taskID, manifest); err != nil {
				return err
			}
			if err := s.appendRecordEventLocked(taskID, Event{Operation: "record_create", Outcome: "replayed", OperationID: request.OperationID, Fingerprint: fingerprint, Revision: manifest.Revision}); err != nil {
				return err
			}
			result = manifest
			return nil
		}
		if !IsKind(readErr, KindNotFound) {
			return readErr
		}
		createdManifest := Manifest{
			Title: request.Title, Provenance: ProvenanceExternal, CallerID: request.CallerID,
			Revision: 1, Worker: "local", Lifecycle: "created", Condition: "none",
			Disposition: "in-flight", HeartbeatAt: time.Now().UTC(),
		}
		createdManifest.Receipts, readErr = appendReceiptText("", request.OperationID, fingerprint, createdManifest.Revision)
		if readErr != nil {
			return readErr
		}
		if err := s.writeManifestLocked(taskID, createdManifest); err != nil {
			return rollbackRecord(err, func() error { return os.RemoveAll(s.taskDir(taskID)) })
		}
		if err := s.appendRecordEventLocked(taskID, Event{Operation: "record_create", Outcome: "created", OperationID: request.OperationID, Fingerprint: fingerprint, Revision: createdManifest.Revision}); err != nil {
			return rollbackRecord(err, func() error { return os.RemoveAll(s.taskDir(taskID)) })
		}
		result = createdManifest
		return nil
	})
	return result, err
}

func (s *Store) AdoptExternalTask(taskID string, request ExternalTaskRequest) (Manifest, error) {
	if err := validateTaskID(taskID); err != nil {
		return Manifest{}, err
	}
	if err := validateCallerID(request.CallerID); err != nil {
		return Manifest{}, err
	}
	if err := validateOperationID(request.OperationID); err != nil {
		return Manifest{}, err
	}
	var result Manifest
	fingerprint := operationFingerprint(request.CallerID)
	err := s.WithLock(taskID, func() error {
		manifest, err := s.readManifestLocked(taskID)
		if err != nil {
			return err
		}
		if manifest.Provenance == ProvenanceExternal && manifest.CallerID != request.CallerID {
			return externalCallerConflict("task", taskID)
		}
		if _, found, receiptErr := findReceipt(manifest.Receipts, request.OperationID, fingerprint); receiptErr != nil {
			return receiptErr
		} else if found {
			if err := s.ensureTaskRecordEventLocked(taskID, request.OperationID, fingerprint, manifest.Revision); err != nil {
				return err
			}
			result = manifest
			return nil
		}
		if manifest.Provenance == ProvenanceExternal {
			if err := s.ensureTaskReceiptHistoryLocked(taskID, manifest.Receipts); err != nil {
				return err
			}
			if externalTaskTerminal(manifest) {
				return terminalMutationConflict("task", taskID)
			}
			manifest.Receipts, err = appendReceiptText(manifest.Receipts, request.OperationID, fingerprint, manifest.Revision)
			if err != nil {
				return err
			}
			if err := s.writeManifestLocked(taskID, manifest); err != nil {
				return err
			}
			if err := s.ensureTaskRecordEventLocked(taskID, request.OperationID, fingerprint, manifest.Revision); err != nil {
				return err
			}
			result = manifest
			return nil
		}
		if externalTaskTerminal(manifest) {
			return terminalMutationConflict("task", taskID)
		}
		if manifest.Provenance != "" && manifest.Provenance != ProvenanceExternal {
			return externalOwnershipConflict("task", taskID)
		}
		if manifest.TmuxWindow != "" || manifest.ProcessPID != 0 || manifest.Launch != nil || manifest.ResourceIDs != "" || manifest.ExecutionIDs != "" {
			return newError(KindConflict, fmt.Sprintf("task %s contains managed lifecycle state", taskID), "Adopt a resource or execution explicitly without retargeting legacy managed state")
		}
		manifest.Provenance = ProvenanceExternal
		manifest.CallerID = request.CallerID
		if manifest.Revision == 0 {
			manifest.Revision = 1
		} else {
			manifest.Revision++
		}
		manifest.Receipts, err = appendReceiptText(manifest.Receipts, request.OperationID, fingerprint, manifest.Revision)
		if err != nil {
			return err
		}
		if err := s.writeManifestLocked(taskID, manifest); err != nil {
			return err
		}
		if err := s.appendRecordEventLocked(taskID, Event{Operation: "record_adopt", Outcome: "task", OperationID: request.OperationID, Fingerprint: fingerprint, Revision: manifest.Revision}); err != nil {
			return err
		}
		result = manifest
		return nil
	})
	return result, err
}

func (s *Store) CompleteExternalTask(taskID, callerID, operationID, contract, resultValue string, expectedRevision uint64) (Manifest, error) {
	if err := validateCompletion(contract, resultValue, callerID); err != nil {
		return Manifest{}, err
	}
	if err := validateOperationID(operationID); err != nil {
		return Manifest{}, err
	}
	var result Manifest
	fingerprint := operationFingerprint(struct{ CallerID, Contract, Result string }{callerID, contract, resultValue})
	err := s.WithLock(taskID, func() error {
		manifest, err := s.readManifestLocked(taskID)
		if err != nil {
			return err
		}
		if manifest.Provenance != ProvenanceExternal {
			return externalOwnershipConflict("task", taskID)
		}
		if err := validateExternalTaskCaller(manifest, taskID, callerID); err != nil {
			return err
		}
		if _, found, receiptErr := findReceipt(manifest.Receipts, operationID, fingerprint); receiptErr != nil {
			return receiptErr
		} else if found {
			if err := s.ensureTaskRecordEventLocked(taskID, operationID, fingerprint, manifest.Revision); err != nil {
				return err
			}
			result = manifest
			return nil
		}
		if manifest.ExternalCompletion != nil && completionMatches(manifest.ExternalCompletion, contract, resultValue, callerID) {
			manifest.Receipts, err = appendReceiptText(manifest.Receipts, operationID, fingerprint, manifest.Revision)
			if err != nil {
				return err
			}
			if err := s.writeManifestLocked(taskID, manifest); err != nil {
				return err
			}
			if err := s.ensureTaskRecordEventLocked(taskID, operationID, fingerprint, manifest.Revision); err != nil {
				return err
			}
			result = manifest
			return nil
		}
		if err := s.ensureTaskReceiptHistoryLocked(taskID, manifest.Receipts); err != nil {
			return err
		}
		if manifest.ExternalCompletion != nil || manifest.ArchiveState == "complete" {
			return terminalMutationConflict("task", taskID)
		}
		if err := expectedRevisionCheck(expectedRevision, manifest.Revision, "task", taskID); err != nil {
			return err
		}
		manifest.ExternalCompletion = &ExternalCompletion{Contract: contract, Result: resultValue, CallerID: callerID, DeclaredAt: time.Now().UTC()}
		manifest.Lifecycle, manifest.Condition, manifest.Result = "finished", "none", resultValue
		manifest.Revision++
		manifest.Receipts, err = appendReceiptText(manifest.Receipts, operationID, fingerprint, manifest.Revision)
		if err != nil {
			return err
		}
		if err := s.writeManifestLocked(taskID, manifest); err != nil {
			return err
		}
		if err := s.appendRecordEventLocked(taskID, Event{Operation: "complete", Outcome: "declared", OperationID: operationID, Fingerprint: fingerprint, Revision: manifest.Revision}); err != nil {
			return err
		}
		result = manifest
		return nil
	})
	return result, err
}

func (s *Store) ArchiveExternalTask(taskID, callerID, operationID string, expectedRevision uint64) (TaskArchive, error) {
	if err := validateCallerID(callerID); err != nil {
		return TaskArchive{}, err
	}
	if err := validateOperationID(operationID); err != nil {
		return TaskArchive{}, err
	}
	var result TaskArchive
	fingerprint := operationFingerprint(struct{ CallerID string }{callerID})
	err := s.WithLock(taskID, func() error {
		manifest, err := s.readManifestLocked(taskID)
		if err != nil {
			return err
		}
		if manifest.Provenance != ProvenanceExternal {
			return externalOwnershipConflict("task", taskID)
		}
		if err := validateExternalTaskCaller(manifest, taskID, callerID); err != nil {
			return err
		}
		if _, found, receiptErr := findReceipt(manifest.Receipts, operationID, fingerprint); receiptErr != nil {
			return receiptErr
		} else if found {
			if err := s.ensureTaskRecordEventLocked(taskID, operationID, fingerprint, manifest.Revision); err != nil {
				return err
			}
			archive, archiveErr := s.ReadArchive(taskID)
			if archiveErr != nil {
				events, eventsErr := s.ReadEvents(taskID)
				if eventsErr != nil {
					return eventsErr
				}
				resources, resourcesErr := s.listResourcesLocked(taskID)
				if resourcesErr != nil {
					return resourcesErr
				}
				executions, executionsErr := s.listExecutionsLocked(taskID)
				if executionsErr != nil {
					return executionsErr
				}
				archive = TaskArchive{TaskID: taskID, CapturedAt: time.Now().UTC(), Manifest: manifest, Events: events, Resources: resources, Executions: executions, Git: manifest.Git}
				if archiveErr = s.writeArchiveLocked(taskID, archive); archiveErr != nil {
					return archiveErr
				}
			}
			result = archive
			return nil
		}
		if manifest.ExternalCompletion == nil || manifest.Lifecycle != "finished" {
			return newError(KindConflict, fmt.Sprintf("external task %s must be explicitly completed before archive", taskID), "Record task completion against an external completion contract first")
		}
		if manifest.CallerID != "" && manifest.CallerID != callerID {
			return newError(KindConflict, fmt.Sprintf("external task %s belongs to another completion caller", taskID), "Use the caller ID recorded by task completion")
		}
		if archive, readErr := s.ReadArchive(taskID); readErr == nil && manifest.ArchiveState == "complete" {
			result = archive
			return nil
		}
		if err := expectedRevisionCheck(expectedRevision, manifest.Revision, "task", taskID); err != nil {
			return err
		}
		manifest.ArchiveState = "complete"
		manifest.Revision++
		manifest.Receipts, err = appendReceiptText(manifest.Receipts, operationID, fingerprint, manifest.Revision)
		if err != nil {
			return err
		}
		if err := s.writeManifestLocked(taskID, manifest); err != nil {
			return err
		}
		if err := s.appendRecordEventLocked(taskID, Event{Operation: "archive", Outcome: "recorded", OperationID: operationID, Fingerprint: fingerprint, Revision: manifest.Revision}); err != nil {
			return err
		}
		events, err := s.ReadEvents(taskID)
		if err != nil {
			return err
		}
		resources, err := s.listResourcesLocked(taskID)
		if err != nil {
			return err
		}
		executions, err := s.listExecutionsLocked(taskID)
		if err != nil {
			return err
		}
		archive := TaskArchive{TaskID: taskID, CapturedAt: time.Now().UTC(), Manifest: manifest, Events: events, Resources: resources, Executions: executions, Git: manifest.Git}
		if err := s.writeArchiveLocked(taskID, archive); err != nil {
			return err
		}
		result = archive
		return nil
	})
	return result, err
}

func (s *Store) readManifestLocked(taskID string) (Manifest, error) {
	envelope, err := s.ReadManifest(taskID)
	if err != nil {
		return Manifest{}, err
	}
	return envelope.DecodeManifest()
}

func (s *Store) writeArchiveLocked(taskID string, archive TaskArchive) error {
	envelope, err := archiveEnvelope(taskID, archive)
	if err != nil {
		return internalError("encode a task archive", "Retry the operation")
	}
	encoded, err := encodeRecord(envelope)
	if err != nil {
		return err
	}
	return s.atomicallyWrite(s.archivePath(taskID), encoded)
}
