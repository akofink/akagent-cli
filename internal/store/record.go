package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

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

func (s *Store) AdoptExternalResource(taskID string, request ExternalResourceRequest) (Resource, error) {
	if err := validateTaskID(taskID); err != nil {
		return Resource{}, err
	}
	if err := validateExternalResourceRequest(request); err != nil {
		return Resource{}, err
	}
	var result Resource
	fingerprint := operationFingerprint(struct {
		ID, CallerID, Repository, Branch, BaseRevision, Head, WorktreePath string
		Metadata                                                           map[string]string
	}{request.ID, request.CallerID, request.Repository, request.Branch, request.BaseRevision, request.Head, request.WorktreePath, request.Metadata})
	err := s.WithLock(taskID, func() error {
		manifest, err := s.readManifestLocked(taskID)
		if err != nil {
			return err
		}
		originalManifest := manifest
		if manifest.Provenance == ProvenanceExternal && manifest.CallerID != request.CallerID {
			return externalCallerConflict("task", taskID)
		}
		if manifest.Provenance != ProvenanceExternal && (manifest.ResourceIDs != "" || manifest.ExecutionIDs != "" || manifest.TmuxWindow != "" || manifest.ProcessPID != 0 || manifest.Launch != nil) {
			return newError(KindConflict, fmt.Sprintf("task %s contains managed lifecycle state", taskID), "Adopt only an empty task or use the existing managed lifecycle surface")
		}
		current, readErr := s.ReadResource(taskID, request.ID)
		if readErr != nil && !IsKind(readErr, KindNotFound) {
			return readErr
		}
		if readErr == nil {
			if current.Provenance != ProvenanceExternal {
				return externalOwnershipConflict("resource", request.ID)
			}
			if current.CallerID != request.CallerID {
				return externalCallerConflict("resource", request.ID)
			}
			if _, found, receiptErr := findReceipt(current.Receipts, request.OperationID, fingerprint); receiptErr != nil {
				return receiptErr
			} else if found {
				if err := s.ensureExternalTaskChildProjectionLocked(taskID, request.CallerID, request.OperationID, fingerprint, request.ID, "", "record_adopt"); err != nil {
					return err
				}
				if err := s.ensureResourceRecordEventLocked(taskID, request.ID, request.OperationID, fingerprint, current.Revision); err != nil {
					return err
				}
				result = current
				return nil
			}
			if err := s.ensureTaskReceiptHistoryLocked(taskID, manifest.Receipts); err != nil {
				if _, found, receiptErr := findReceipt(manifest.Receipts, request.OperationID, fingerprint); receiptErr != nil || !found {
					return err
				}
			}
			if err := s.ensureResourceReceiptHistoryLocked(taskID, request.ID, current.Receipts); err != nil {
				return err
			}
			if externalTaskTerminal(manifest) || current.ArchiveState == "complete" {
				return terminalMutationConflict("resource", request.ID)
			}
			if !sameExternalResourceBinding(current, request) {
				if request.ExpectedRevision == 0 || request.ExpectedRevision != current.Revision {
					return revisionConflict("resource", request.ID, current.Revision)
				}
				current.ObservationHistory, err = appendExternalResourceObservation(current.ObservationHistory, current, time.Now().UTC())
				if err != nil {
					return err
				}
				current.Repository, current.Branch, current.BaseRevision, current.WorktreePath = request.Repository, request.Branch, request.BaseRevision, request.WorktreePath
				current.Git.Path, current.Git.Head, current.Git.Branch = request.WorktreePath, request.Head, request.Branch
				current.Metadata = mergeStringMap(current.Metadata, request.Metadata)
				current.Revision++
				current.Receipts = appendReceipt(current.Receipts, request.OperationID, fingerprint, current.Revision)
				if err := s.writeResourceLocked(taskID, current); err != nil {
					return err
				}
				if err := s.appendResourceEventLocked(taskID, request.ID, Event{Operation: "adopt", Outcome: "updated", OperationID: request.OperationID, Fingerprint: fingerprint, Revision: current.Revision}); err != nil {
					return err
				}
			} else if request.ExpectedRevision != 0 && request.ExpectedRevision != current.Revision {
				return revisionConflict("resource", request.ID, current.Revision)
			} else {
				current.Receipts = appendReceipt(current.Receipts, request.OperationID, fingerprint, current.Revision)
				if err := s.writeResourceLocked(taskID, current); err != nil {
					return err
				}
				if err := s.appendResourceEventLocked(taskID, request.ID, Event{Operation: "adopt", Outcome: "recorded", OperationID: request.OperationID, Fingerprint: fingerprint, Revision: current.Revision}); err != nil {
					return err
				}
			}
			result = current
			return nil
		}
		if err := s.ensureTaskReceiptHistoryLocked(taskID, manifest.Receipts); err != nil {
			return err
		}
		if externalTaskTerminal(manifest) {
			return terminalMutationConflict("task", taskID)
		}
		if request.ExpectedRevision != 0 {
			return revisionConflict("resource", request.ID, 0)
		}
		resource := Resource{
			ID: request.ID, TaskID: taskID, Provenance: ProvenanceExternal, CallerID: request.CallerID, Revision: 1,
			Receipts:   []RecordReceipt{{OperationID: request.OperationID, Fingerprint: fingerprint, Revision: 1}},
			Repository: request.Repository, Branch: request.Branch, BaseRevision: request.BaseRevision, WorktreePath: request.WorktreePath,
			Metadata: cloneStringMap(request.Metadata, nil), Git: GitFacts{Path: request.WorktreePath, Head: request.Head, Branch: request.Branch},
		}
		if err := s.writeResourceLocked(taskID, resource); err != nil {
			return rollbackRecord(err, func() error { return os.RemoveAll(s.resourceDir(taskID, request.ID)) })
		}
		if err := s.appendResourceEventLocked(taskID, request.ID, Event{Operation: "adopt", Outcome: "recorded", OperationID: request.OperationID, Fingerprint: fingerprint, Revision: resource.Revision}); err != nil {
			return rollbackRecord(err, func() error { return os.RemoveAll(s.resourceDir(taskID, request.ID)) })
		}
		manifest.ResourceIDs = appendCSV(manifest.ResourceIDs, request.ID)
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
			return rollbackRecord(err, func() error { return os.RemoveAll(s.resourceDir(taskID, request.ID)) })
		}
		if err := s.writeManifestLocked(taskID, manifest); err != nil {
			return rollbackRecord(err, func() error { return os.RemoveAll(s.resourceDir(taskID, request.ID)) })
		}
		if err := s.appendRecordEventLocked(taskID, Event{Operation: "record_adopt", Outcome: "resource", OperationID: request.OperationID, Fingerprint: fingerprint, Revision: manifest.Revision}); err != nil {
			return rollbackRecord(err,
				func() error { return s.writeManifestLocked(taskID, originalManifest) },
				func() error { return os.RemoveAll(s.resourceDir(taskID, request.ID)) },
			)
		}
		result = resource
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

func (s *Store) ArchiveExternalResource(taskID, resourceID, callerID, operationID string, expectedRevision uint64) (ResourceArchive, error) {
	if err := validateCallerID(callerID); err != nil {
		return ResourceArchive{}, err
	}
	if err := validateOperationID(operationID); err != nil {
		return ResourceArchive{}, err
	}
	var result ResourceArchive
	fingerprint := operationFingerprint(struct{ CallerID string }{callerID})
	err := s.WithLock(taskID, func() error {
		resource, err := s.ReadResource(taskID, resourceID)
		if err != nil {
			return err
		}
		if err := validateExternalResourceCaller(resource, callerID); err != nil {
			return err
		}
		if _, found, receiptErr := findReceipt(resource.Receipts, operationID, fingerprint); receiptErr != nil {
			return receiptErr
		} else if found {
			if err := s.ensureResourceRecordEventLocked(taskID, resourceID, operationID, fingerprint, resource.Revision); err != nil {
				return err
			}
			archive, archiveErr := s.ReadResourceArchive(taskID, resourceID)
			if archiveErr != nil {
				events, eventsErr := s.ReadResourceEvents(taskID, resourceID)
				if eventsErr != nil {
					return eventsErr
				}
				archive = ResourceArchive{TaskID: taskID, ResourceID: resourceID, CapturedAt: time.Now().UTC(), Resource: resource, Events: events, Git: resource.Git}
				if archiveErr = s.writeResourceArchiveLocked(taskID, resourceID, archive); archiveErr != nil {
					return archiveErr
				}
			}
			result = archive
			return nil
		}
		if resource.ArchiveState == "complete" {
			if existing, readErr := s.ReadResourceArchive(taskID, resourceID); readErr == nil {
				result = existing
				return nil
			}
			return newError(KindPartial, fmt.Sprintf("external resource %s archive is incomplete", resourceID), "Repair the missing resource archive before retrying")
		}
		if err := expectedRevisionCheck(expectedRevision, resource.Revision, "resource", resourceID); err != nil {
			return err
		}
		if existing, readErr := s.ReadResourceArchive(taskID, resourceID); readErr == nil && resource.ArchiveState == "complete" {
			result = existing
			return nil
		}
		resource.ArchiveState = "complete"
		resource.Revision++
		resource.Receipts = appendReceipt(resource.Receipts, operationID, fingerprint, resource.Revision)
		if err := s.writeResourceLocked(taskID, resource); err != nil {
			return err
		}
		if err := s.appendResourceEventLocked(taskID, resourceID, Event{Operation: "archive", Outcome: "recorded", OperationID: operationID, Fingerprint: fingerprint, Revision: resource.Revision}); err != nil {
			return err
		}
		events, err := s.ReadResourceEvents(taskID, resourceID)
		if err != nil {
			return err
		}
		archive := ResourceArchive{TaskID: taskID, ResourceID: resourceID, CapturedAt: time.Now().UTC(), Resource: resource, Events: events, Git: resource.Git}
		if err := s.writeResourceArchiveLocked(taskID, resourceID, archive); err != nil {
			return err
		}
		result = archive
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

func validateExternalTaskRequest(request ExternalTaskRequest) error {
	if err := validateCallerID(request.CallerID); err != nil {
		return err
	}
	if err := validateOperationID(request.OperationID); err != nil {
		return err
	}
	if strings.TrimSpace(request.Title) == "" || strings.ContainsAny(request.Title, "\r\n\x00") || len(request.Title) > 4096 {
		return newError(KindUsage, "external task title must be a bounded single line", "Provide a non-secret task title")
	}
	return nil
}

func validateExternalResourceRequest(request ExternalResourceRequest) error {
	if err := validateOperationID(request.OperationID); err != nil {
		return err
	}
	if err := validateResourceID(request.ID); err != nil {
		return err
	}
	if err := validateCallerID(request.CallerID); err != nil {
		return err
	}
	if strings.TrimSpace(request.Repository) == "" || strings.TrimSpace(request.Branch) == "" || strings.TrimSpace(request.BaseRevision) == "" || strings.TrimSpace(request.Head) == "" {
		return newError(KindUsage, "external resource repository, branch, base, and head are required", "Provide caller-declared repository identity and revisions")
	}
	for _, value := range []string{request.Repository, request.Branch, request.BaseRevision, request.Head} {
		if strings.ContainsAny(value, "\r\n\x00") || len(value) > 4096 {
			return newError(KindUsage, "external resource identity values must be bounded single lines", "Retry with redacted non-secret identity values")
		}
	}
	if !filepath.IsAbs(request.WorktreePath) || strings.ContainsAny(request.WorktreePath, "\r\n\x00") || len(request.WorktreePath) > 4096 {
		return newError(KindUsage, "external resource worktree must be a bounded absolute path", "Provide a bounded absolute referenced worktree path; it need not exist")
	}
	if len(request.Metadata) > 64 {
		return newError(KindUsage, "external resource metadata has too many entries", "Provide at most 64 non-secret metadata entries")
	}
	for key, value := range request.Metadata {
		if strings.TrimSpace(key) == "" || strings.ContainsAny(key, "\r\n\x00") || len(key) > 256 || strings.ContainsAny(value, "\r\n\x00") || len(value) > 4096 {
			return newError(KindUsage, "external resource metadata keys and values must be bounded single lines", "Retry with bounded non-secret metadata")
		}
	}
	return nil
}

func validateExternalExecutionRequest(request ExternalExecutionRequest) error {
	if err := validateOperationID(request.OperationID); err != nil {
		return err
	}
	if err := validateExecutionID(request.ID); err != nil {
		return err
	}
	if err := validateCallerID(request.CallerID); err != nil {
		return err
	}
	if err := validateResourceID(request.ResourceID); err != nil {
		return err
	}
	if request.PredecessorID != "" {
		if err := validateExecutionID(request.PredecessorID); err != nil {
			return err
		}
		if request.PredecessorID == request.ID {
			return newError(KindConflict, "external execution cannot be its own predecessor", "Choose a prior execution ID")
		}
	}
	if len(request.SessionReferences) > 32 {
		return newError(KindUsage, "external execution has too many session references", "Provide at most 32 provider-neutral session references")
	}
	for _, reference := range request.SessionReferences {
		if err := validateSessionReferenceShape(reference); err != nil {
			return err
		}
	}
	return nil
}

func validateOperationID(value string) error {
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") || len(value) > 256 {
		return newError(KindUsage, "operation ID must be a non-empty bounded single line", "Provide a stable per-operation idempotency key")
	}
	return nil
}

func validateCallerID(value string) error {
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") || len(value) > 256 {
		return newError(KindUsage, "caller ID must be a non-empty bounded single line", "Provide a stable non-secret caller ID")
	}
	return nil
}

func validateExternalObservation(observation ExternalObservation) error {
	if strings.TrimSpace(observation.Source) == "" || strings.TrimSpace(observation.HostID) == "" || strings.TrimSpace(observation.BootID) == "" || observation.ObservedAt.IsZero() {
		return newError(KindUsage, "external observations require source, observed time, host ID, and boot ID", "Provide complete observation provenance")
	}
	for _, value := range []string{observation.Source, observation.HostID, observation.BootID, observation.ProcessState, observation.Result, observation.Detail} {
		if strings.ContainsAny(value, "\r\n\x00") || len(value) > 4096 {
			return newError(KindUsage, "external observation values must be bounded single lines", "Retry with redacted non-secret observation values")
		}
	}
	return nil
}

func validateExternalCompletion(completion ExternalCompletion) error {
	if completion.DeclaredAt.IsZero() {
		return newError(KindUsage, "external completion declaration time is required", "Repair the external completion record")
	}
	return validateCompletion(completion.Contract, completion.Result, completion.CallerID)
}

func validateCompletion(contract, resultValue, callerID string) error {
	if err := validateCallerID(callerID); err != nil {
		return err
	}
	if strings.TrimSpace(contract) == "" || strings.TrimSpace(resultValue) == "" || strings.ContainsAny(contract+resultValue, "\r\n\x00") || len(contract) > 4096 || len(resultValue) > 4096 {
		return newError(KindUsage, "completion contract and result are required bounded single-line values", "Declare completion against a named external contract")
	}
	return nil
}

func sameExternalResourceBinding(current Resource, request ExternalResourceRequest) bool {
	metadataSame := request.Metadata == nil || sameStringMap(current.Metadata, mergeStringMap(current.Metadata, request.Metadata))
	return current.Repository == request.Repository && current.Branch == request.Branch && current.BaseRevision == request.BaseRevision && current.WorktreePath == request.WorktreePath && current.Git.Head == request.Head && metadataSame
}

func validateExternalTaskCaller(manifest Manifest, taskID, callerID string) error {
	if manifest.Provenance != ProvenanceExternal {
		return externalOwnershipConflict("task", taskID)
	}
	if manifest.CallerID != callerID {
		return externalCallerConflict("task", taskID)
	}
	return nil
}

func validateExternalResourceCaller(resource Resource, callerID string) error {
	if resource.Provenance != ProvenanceExternal {
		return externalOwnershipConflict("resource", resource.ID)
	}
	if resource.CallerID != callerID {
		return externalCallerConflict("resource", resource.ID)
	}
	return nil
}

func validateExternalExecutionCaller(execution Execution, callerID string) error {
	if execution.Provenance != ProvenanceExternal {
		return externalOwnershipConflict("execution", execution.ID)
	}
	if execution.CallerID != callerID {
		return externalCallerConflict("execution", execution.ID)
	}
	return nil
}

func externalTaskTerminal(manifest Manifest) bool {
	return manifest.Lifecycle == "finished" || manifest.ExternalCompletion != nil || manifest.ArchiveState == "complete"
}

func externalCallerConflict(kind, id string) error {
	return newError(KindConflict, fmt.Sprintf("external %s %s belongs to another caller", kind, id), "Use the original stable caller ID or choose a new record ID")
}

func terminalMutationConflict(kind, id string) error {
	return newError(KindConflict, fmt.Sprintf("external %s %s is terminal and immutable", kind, id), "Use an explicit future reopen operation before changing a terminal record")
}

func expectedRevisionCheck(expected, current uint64, kind, id string) error {
	if expected != current {
		return revisionConflict(kind, id, current)
	}
	return nil
}

func revisionConflict(kind, id string, current uint64) error {
	return newError(KindConflict, fmt.Sprintf("%s %s revision conflict (current revision %d)", kind, id, current), "Inspect the record and retry with its current expected revision")
}

func externalOwnershipConflict(kind, id string) error {
	return newError(KindConflict, fmt.Sprintf("%s %s is not an externally declared record", kind, id), "Use the state-only record surface without changing legacy managed ownership")
}

func completionMatches(completion *ExternalCompletion, contract, result, callerID string) bool {
	return completion != nil && completion.Contract == contract && completion.Result == result && completion.CallerID == callerID
}

func operationFingerprint(value any) string {
	encoded, _ := json.Marshal(value)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func validateReceipts(receipts []RecordReceipt) error {
	for _, receipt := range receipts {
		if err := validateOperationID(receipt.OperationID); err != nil || len(receipt.Fingerprint) != 64 || receipt.Revision == 0 {
			return newError(KindUsage, "record receipt is invalid", "Repair the record receipt history")
		}
		if _, err := hex.DecodeString(receipt.Fingerprint); err != nil {
			return newError(KindUsage, "record receipt is invalid", "Repair the record receipt history")
		}
	}
	return nil
}

func findReceipt(value any, operationID, fingerprint string) (RecordReceipt, bool, error) {
	var receipts []RecordReceipt
	switch value := value.(type) {
	case []RecordReceipt:
		receipts = value
	case string:
		if value == "" {
			return RecordReceipt{}, false, nil
		}
		if err := json.Unmarshal([]byte(value), &receipts); err != nil {
			return RecordReceipt{}, false, malformedError("Malformed record receipts", "Inspect and repair the record")
		}
	default:
		return RecordReceipt{}, false, internalError("read record receipts", "Retry the operation")
	}
	if err := validateReceipts(receipts); err != nil {
		return RecordReceipt{}, false, err
	}
	for _, receipt := range receipts {
		if receipt.OperationID != operationID {
			continue
		}
		if receipt.Fingerprint != fingerprint {
			return RecordReceipt{}, false, newError(KindConflict, fmt.Sprintf("operation ID %s was already used with different inputs", operationID), "Choose a new operation ID or retry the original inputs")
		}
		return receipt, true, nil
	}
	return RecordReceipt{}, false, nil
}

func appendReceipt(receipts []RecordReceipt, operationID, fingerprint string, revision uint64) []RecordReceipt {
	return append(receipts, RecordReceipt{OperationID: operationID, Fingerprint: fingerprint, Revision: revision})
}

func appendReceiptText(encoded, operationID, fingerprint string, revision uint64) (string, error) {
	var receipts []RecordReceipt
	if encoded != "" {
		if err := json.Unmarshal([]byte(encoded), &receipts); err != nil {
			return "", malformedError("Malformed record receipts", "Inspect and repair the record")
		}
	}
	receipts = appendReceipt(receipts, operationID, fingerprint, revision)
	encodedBytes, err := json.Marshal(receipts)
	if err != nil {
		return "", internalError("encode record receipts", "Retry the operation")
	}
	return string(encodedBytes), nil
}

func sameStringMap(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if b[key] != value {
			return false
		}
	}
	return true
}

func mergeStringMap(existing, updates map[string]string) map[string]string {
	result := cloneStringMap(existing, nil)
	if updates == nil {
		return result
	}
	if result == nil {
		result = map[string]string{}
	}
	for key, value := range updates {
		result[key] = value
	}
	return result
}

func cloneStringMap(primary, fallback map[string]string) map[string]string {
	if primary == nil {
		primary = fallback
	}
	if primary == nil {
		return nil
	}
	result := make(map[string]string, len(primary))
	for key, value := range primary {
		result[key] = value
	}
	return result
}

func appendExternalResourceObservation(history []ExternalResourceObservation, resource Resource, now time.Time) ([]ExternalResourceObservation, error) {
	if len(history) >= externalHistoryLimit {
		return nil, newError(KindConflict, "external resource observation history limit reached", "Archive or reopen the record before recording another binding revision")
	}
	entry := ExternalResourceObservation{CallerID: resource.CallerID, ObservedAt: now, Repository: resource.Repository, Branch: resource.Branch, BaseRevision: resource.BaseRevision, WorktreePath: resource.WorktreePath, Head: resource.Git.Head}
	return append(history, entry), nil
}

func appendCSV(values, value string) string {
	for _, current := range strings.Split(values, ",") {
		if current == value {
			return values
		}
	}
	if values == "" {
		return value
	}
	return values + "," + value
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

func (s *Store) ensureResourceRecordEventLocked(taskID, resourceID, operationID, fingerprint string, revision uint64) error {
	events, err := s.ReadResourceEvents(taskID, resourceID)
	if err != nil {
		return err
	}
	if eventHasReceipt(events, operationID, fingerprint) {
		return nil
	}
	return s.appendResourceEventLocked(taskID, resourceID, Event{Operation: "record_repair", Outcome: "repaired", OperationID: operationID, Fingerprint: fingerprint, Revision: revision})
}

func (s *Store) appendResourceEventLocked(taskID, resourceID string, event Event) error {
	if err := s.ensureResourceDir(taskID, resourceID); err != nil {
		return err
	}
	envelope, err := resourceEventEnvelope(taskID, resourceID, event)
	if err != nil {
		return internalError("encode a resource record event", "Retry the operation")
	}
	encoded, err := encodeRecord(envelope)
	if err != nil {
		return err
	}
	events, err := s.ReadResourceEvents(taskID, resourceID)
	if err != nil {
		return err
	}
	return s.atomicallyWrite(s.resourceEventPath(taskID, resourceID, len(events)+1), encoded)
}

func (s *Store) ensureExecutionRecordEventLocked(taskID, executionID, operationID, fingerprint string, revision uint64) error {
	events, err := s.ReadExecutionEvents(taskID, executionID)
	if err != nil {
		return err
	}
	if eventHasReceipt(events, operationID, fingerprint) {
		return nil
	}
	return s.appendExecutionEventLocked(taskID, executionID, Event{Operation: "record_repair", Outcome: "repaired", OperationID: operationID, Fingerprint: fingerprint, Revision: revision})
}

func (s *Store) appendExecutionEventLocked(taskID, executionID string, event Event) error {
	if err := s.ensureExecutionDir(taskID, executionID); err != nil {
		return err
	}
	envelope, err := executionEventEnvelope(taskID, executionID, event)
	if err != nil {
		return internalError("encode an execution record event", "Retry the operation")
	}
	encoded, err := encodeRecord(envelope)
	if err != nil {
		return err
	}
	events, err := s.ReadExecutionEvents(taskID, executionID)
	if err != nil {
		return err
	}
	return s.atomicallyWrite(s.executionEventPath(taskID, executionID, len(events)+1), encoded)
}

func (s *Store) ensureExternalTaskChildProjectionLocked(taskID, callerID, operationID, fingerprint, resourceID, executionID, operation string) error {
	manifest, err := s.readManifestLocked(taskID)
	if err != nil {
		return err
	}
	if manifest.Provenance == ProvenanceExternal && manifest.CallerID != callerID {
		return externalCallerConflict("task", taskID)
	}
	childPresent := (resourceID != "" && containsCSV(manifest.ResourceIDs, resourceID)) || (executionID != "" && containsCSV(manifest.ExecutionIDs, executionID))
	if childPresent {
		if _, found, receiptErr := findReceipt(manifest.Receipts, operationID, fingerprint); receiptErr != nil {
			return receiptErr
		} else if found {
			return s.ensureTaskRecordEventLocked(taskID, operationID, fingerprint, manifest.Revision)
		}
		return nil
	}
	if externalTaskTerminal(manifest) {
		return terminalMutationConflict("task", taskID)
	}
	if resourceID != "" {
		manifest.ResourceIDs = appendCSV(manifest.ResourceIDs, resourceID)
	}
	if executionID != "" {
		manifest.ExecutionIDs = appendCSV(manifest.ExecutionIDs, executionID)
	}
	manifest.Provenance = ProvenanceExternal
	if manifest.CallerID == "" {
		manifest.CallerID = callerID
	}
	if manifest.Revision == 0 {
		manifest.Revision = 1
	} else {
		manifest.Revision++
	}
	manifest.Receipts, err = appendReceiptText(manifest.Receipts, operationID, fingerprint, manifest.Revision)
	if err != nil {
		return err
	}
	if err := s.writeManifestLocked(taskID, manifest); err != nil {
		return err
	}
	err = s.appendRecordEventLocked(taskID, Event{Operation: operation, Outcome: "repaired", OperationID: operationID, Fingerprint: fingerprint, Revision: manifest.Revision})
	return err
}

func containsCSV(values, value string) bool {
	for _, current := range strings.Split(values, ",") {
		if current == value {
			return true
		}
	}
	return false
}

func receiptHistoryComplete(receipts []RecordReceipt, events []EventRecord) error {
	for _, receipt := range receipts {
		if !eventHasReceipt(events, receipt.OperationID, receipt.Fingerprint) {
			return newError(KindPartial, fmt.Sprintf("record operation %s has no durable audit event", receipt.OperationID), "Retry that operation to repair its audit projection before issuing a new write")
		}
	}
	return nil
}

func (s *Store) ensureResourceReceiptHistoryLocked(taskID, resourceID string, receipts []RecordReceipt) error {
	events, err := s.ReadResourceEvents(taskID, resourceID)
	if err != nil {
		return err
	}
	return receiptHistoryComplete(receipts, events)
}

func (s *Store) ensureExecutionReceiptHistoryLocked(taskID, executionID string, receipts []RecordReceipt) error {
	events, err := s.ReadExecutionEvents(taskID, executionID)
	if err != nil {
		return err
	}
	return receiptHistoryComplete(receipts, events)
}

func (s *Store) ensureTaskReceiptHistoryLocked(taskID string, encoded string) error {
	var receipts []RecordReceipt
	if encoded != "" {
		if err := json.Unmarshal([]byte(encoded), &receipts); err != nil {
			return malformedError("Malformed record receipts", "Inspect and repair the record")
		}
	}
	events, err := s.ReadEvents(taskID)
	if err != nil {
		return err
	}
	return receiptHistoryComplete(receipts, events)
}

func eventHasReceipt(events []EventRecord, operationID, fingerprint string) bool {
	for _, event := range events {
		if event.Event.OperationID == operationID && event.Event.Fingerprint == fingerprint {
			return true
		}
	}
	return false
}

func (s *Store) appendRecordEventLocked(taskID string, event Event) error {
	next, err := s.nextSequence(taskID)
	if err != nil {
		return err
	}
	return s.appendEventLocked(taskID, next, event)
}

func (s *Store) ensureTaskRecordEventLocked(taskID, operationID, fingerprint string, revision uint64) error {
	events, err := s.ReadEvents(taskID)
	if err != nil {
		return err
	}
	if eventHasReceipt(events, operationID, fingerprint) {
		return nil
	}
	err = s.appendRecordEventLocked(taskID, Event{Operation: "record_repair", Outcome: "repaired", OperationID: operationID, Fingerprint: fingerprint, Revision: revision})
	return err
}

func (s *Store) writeResourceArchiveLocked(taskID, resourceID string, archive ResourceArchive) error {
	envelope, err := resourceArchiveEnvelope(taskID, resourceID, archive)
	if err != nil {
		return internalError("encode a resource archive", "Retry the operation")
	}
	encoded, err := encodeRecord(envelope)
	if err != nil {
		return err
	}
	return s.atomicallyWrite(s.resourceArchivePath(taskID, resourceID), encoded)
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

func (s *Store) listResourcesLocked(taskID string) ([]Resource, error) {
	ids, err := s.ResourceIDs(taskID)
	if err != nil {
		return nil, err
	}
	result := make([]Resource, 0, len(ids))
	for _, id := range ids {
		resource, err := s.ReadResource(taskID, id)
		if err != nil {
			return nil, err
		}
		result = append(result, resource)
	}
	return result, nil
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
