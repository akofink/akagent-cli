package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const externalHistoryLimit = 32

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
	CallerID    string
	OperationID string
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
		current, readErr := s.ReadResource(taskID, request.ID)
		if readErr != nil && !IsKind(readErr, KindNotFound) {
			return readErr
		}
		if readErr == nil {
			if _, found, receiptErr := findReceipt(current.Receipts, request.OperationID, fingerprint); receiptErr != nil {
				return receiptErr
			} else if found {
				result = current
				return nil
			}
			if current.Provenance != ProvenanceExternal {
				return externalOwnershipConflict("resource", request.ID)
			}
			if current.CallerID != request.CallerID {
				return newError(KindConflict, fmt.Sprintf("external resource %s belongs to another caller", request.ID), "Use the original caller ID or choose a new resource ID")
			}
			if !sameExternalResourceBinding(current, request) {
				if request.ExpectedRevision == 0 || request.ExpectedRevision != current.Revision {
					return revisionConflict("resource", request.ID, current.Revision)
				}
				current.ObservationHistory = appendExternalResourceObservation(current.ObservationHistory, current, time.Now().UTC())
				current.Git.Path, current.Git.Head, current.Git.Branch = request.WorktreePath, request.Head, request.Branch
				current.Metadata = mergeStringMap(current.Metadata, request.Metadata)
				current.Revision++
				current.Receipts = appendReceipt(current.Receipts, request.OperationID, fingerprint, current.Revision)
				if err := s.writeResourceLocked(taskID, current); err != nil {
					return err
				}
				if err := s.appendResourceEventLocked(taskID, request.ID, Event{Operation: "adopt", Outcome: "updated"}); err != nil {
					return err
				}
			} else if request.ExpectedRevision != 0 && request.ExpectedRevision != current.Revision {
				return revisionConflict("resource", request.ID, current.Revision)
			} else {
				current.Receipts = appendReceipt(current.Receipts, request.OperationID, fingerprint, current.Revision)
				if err := s.writeResourceLocked(taskID, current); err != nil {
					return err
				}
			}
			result = current
			return nil
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
			return err
		}
		if err := s.appendResourceEventLocked(taskID, request.ID, Event{Operation: "adopt", Outcome: "recorded"}); err != nil {
			return err
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
			return err
		}
		if err := s.writeManifestLocked(taskID, manifest); err != nil {
			return err
		}
		if _, err := s.appendEventLocked(taskID, Event{Operation: "record_adopt", Outcome: "resource"}); err != nil {
			return err
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
		if _, found, receiptErr := findReceipt(manifest.Receipts, request.OperationID, fingerprint); receiptErr != nil {
			return receiptErr
		} else if found {
			result = manifest
			return nil
		}
		if manifest.Provenance != "" && manifest.Provenance != ProvenanceExternal {
			return externalOwnershipConflict("task", taskID)
		}
		if manifest.TmuxWindow != "" || manifest.ProcessPID != 0 || manifest.Launch != nil || manifest.ResourceIDs != "" || manifest.ExecutionIDs != "" {
			return newError(KindConflict, fmt.Sprintf("task %s contains managed lifecycle state", taskID), "Adopt a resource or execution explicitly without retargeting legacy managed state")
		}
		manifest.Provenance = ProvenanceExternal
		manifest.CallerID = request.CallerID
		manifest.Revision = 1
		manifest.Receipts, err = appendReceiptText(manifest.Receipts, request.OperationID, fingerprint, manifest.Revision)
		if err != nil {
			return err
		}
		if err := s.writeManifestLocked(taskID, manifest); err != nil {
			return err
		}
		if _, err := s.appendEventLocked(taskID, Event{Operation: "record_adopt", Outcome: "task"}); err != nil {
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
		resource, err := s.ReadResource(taskID, request.ResourceID)
		if err != nil {
			return err
		}
		if resource.Provenance != ProvenanceExternal {
			return externalOwnershipConflict("resource", request.ResourceID)
		}
		if request.PredecessorID != "" {
			if err := s.validatePredecessorLocked(taskID, request.ID, request.PredecessorID); err != nil {
				return err
			}
		}
		current, readErr := s.ReadExecution(taskID, request.ID)
		if readErr == nil {
			if _, found, receiptErr := findReceipt(current.Receipts, request.OperationID, fingerprint); receiptErr != nil {
				return receiptErr
			} else if found {
				result = current
				return nil
			}
			if current.Provenance != ProvenanceExternal {
				return externalOwnershipConflict("execution", request.ID)
			}
			if current.CallerID != request.CallerID || current.ResourceID != request.ResourceID || current.PredecessorID != request.PredecessorID || !sameSessionReferences(current.SessionReferences, request.SessionReferences) {
				return newError(KindConflict, fmt.Sprintf("external execution %s inputs conflict with its existing record", request.ID), "Inspect the execution and retry with its original immutable inputs")
			}
			current.Receipts = appendReceipt(current.Receipts, request.OperationID, fingerprint, current.Revision)
			if err := s.writeExecutionLockedUnvalidatedPaths(taskID, current); err != nil {
				return err
			}
			result = current
			return nil
		}
		if !IsKind(readErr, KindNotFound) {
			return readErr
		}
		execution := Execution{
			ID: request.ID, TaskID: taskID, Provenance: ProvenanceExternal, CallerID: request.CallerID, Revision: 1,
			PredecessorID: request.PredecessorID, Label: "external", Target: "external", ResourceID: request.ResourceID,
			SessionReferences: append([]SessionReference(nil), request.SessionReferences...), Lifecycle: "created", Condition: "none",
			Receipts: []RecordReceipt{{OperationID: request.OperationID, Fingerprint: fingerprint, Revision: 1}},
		}
		if err := s.writeExecutionLockedUnvalidatedPaths(taskID, execution); err != nil {
			return err
		}
		if err := s.appendExecutionEventLocked(taskID, request.ID, Event{Operation: "record", Outcome: "created"}); err != nil {
			return err
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
			return err
		}
		if err := s.writeManifestLocked(taskID, manifest); err != nil {
			return err
		}
		if _, err := s.appendEventLocked(taskID, Event{Operation: "record_execution", Outcome: "created"}); err != nil {
			return err
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
	fingerprint := operationFingerprint(observation)
	err := s.WithLock(taskID, func() error {
		execution, err := s.ReadExecution(taskID, executionID)
		if err != nil {
			return err
		}
		if _, found, receiptErr := findReceipt(execution.Receipts, operationID, fingerprint); receiptErr != nil {
			return receiptErr
		} else if found {
			result = execution
			return nil
		}
		if err := validateExternalExecutionOwner(execution, callerID, expectedRevision); err != nil {
			return err
		}
		if len(execution.ExternalObservations) > 0 && execution.ExternalObservations[len(execution.ExternalObservations)-1] == observation {
			result = execution
			return nil
		}
		execution.ExternalObservations = append(execution.ExternalObservations, observation)
		if len(execution.ExternalObservations) > externalHistoryLimit {
			execution.ExternalObservations = execution.ExternalObservations[len(execution.ExternalObservations)-externalHistoryLimit:]
		}
		execution.Revision++
		execution.Receipts = appendReceipt(execution.Receipts, operationID, fingerprint, execution.Revision)
		if err := s.writeExecutionLockedUnvalidatedPaths(taskID, execution); err != nil {
			return err
		}
		if err := s.appendExecutionEventLocked(taskID, executionID, Event{Operation: "observe", Outcome: "recorded"}); err != nil {
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
			return externalOwnershipConflict("execution", executionID)
		}
		if _, found, receiptErr := findReceipt(execution.Receipts, operationID, fingerprint); receiptErr != nil {
			return receiptErr
		} else if found {
			result = execution
			return nil
		}
		if execution.ExternalCompletion != nil && completionMatches(execution.ExternalCompletion, contract, resultValue, callerID) {
			result = execution
			return nil
		}
		if err := validateExternalExecutionOwner(execution, callerID, expectedRevision); err != nil {
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
		if err := s.appendExecutionEventLocked(taskID, executionID, Event{Operation: "complete", Outcome: "declared"}); err != nil {
			return err
		}
		result = execution
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
		if _, found, receiptErr := findReceipt(manifest.Receipts, operationID, fingerprint); receiptErr != nil {
			return receiptErr
		} else if found {
			result = manifest
			return nil
		}
		if manifest.ExternalCompletion != nil && completionMatches(manifest.ExternalCompletion, contract, resultValue, callerID) {
			result = manifest
			return nil
		}
		if err := expectedRevisionCheck(expectedRevision, manifest.Revision, "task", taskID); err != nil {
			return err
		}
		manifest.ExternalCompletion = &ExternalCompletion{Contract: contract, Result: resultValue, CallerID: callerID, DeclaredAt: time.Now().UTC()}
		manifest.Lifecycle, manifest.Condition, manifest.Result = "finished", "none", resultValue
		manifest.CallerID = callerID
		manifest.Revision++
		manifest.Receipts, err = appendReceiptText(manifest.Receipts, operationID, fingerprint, manifest.Revision)
		if err != nil {
			return err
		}
		if err := s.writeManifestLocked(taskID, manifest); err != nil {
			return err
		}
		if _, err := s.appendEventLocked(taskID, Event{Operation: "complete", Outcome: "declared"}); err != nil {
			return err
		}
		result = manifest
		return nil
	})
	return result, err
}

func (s *Store) ArchiveExternalResource(taskID, resourceID, callerID, operationID string, expectedRevision uint64) (ResourceArchive, error) {
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
		if _, found, receiptErr := findReceipt(resource.Receipts, operationID, fingerprint); receiptErr != nil {
			return receiptErr
		} else if found {
			archive, archiveErr := s.ReadResourceArchive(taskID, resourceID)
			if archiveErr != nil {
				return archiveErr
			}
			result = archive
			return nil
		}
		if err := validateExternalResourceOwner(resource, callerID, expectedRevision); err != nil {
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
		if err := s.appendResourceEventLocked(taskID, resourceID, Event{Operation: "archive", Outcome: "recorded"}); err != nil {
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
		if _, found, receiptErr := findReceipt(execution.Receipts, operationID, fingerprint); receiptErr != nil {
			return receiptErr
		} else if found {
			archive, archiveErr := s.ReadExecutionArchive(taskID, executionID)
			if archiveErr != nil {
				return archiveErr
			}
			result = archive
			return nil
		}
		if err := validateExternalExecutionOwner(execution, callerID, expectedRevision); err != nil {
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
		if err := s.appendExecutionEventLocked(taskID, executionID, Event{Operation: "archive", Outcome: "recorded"}); err != nil {
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
		if _, found, receiptErr := findReceipt(manifest.Receipts, operationID, fingerprint); receiptErr != nil {
			return receiptErr
		} else if found {
			archive, archiveErr := s.ReadArchive(taskID)
			if archiveErr != nil {
				return archiveErr
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
		if _, err := s.appendEventLocked(taskID, Event{Operation: "archive", Outcome: "recorded"}); err != nil {
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
	if !filepath.IsAbs(request.WorktreePath) || strings.ContainsAny(request.WorktreePath, "\r\n\x00") {
		return newError(KindUsage, "external resource worktree must be an absolute path", "Provide the absolute referenced worktree path; it need not exist")
	}
	for key, value := range request.Metadata {
		if strings.TrimSpace(key) == "" || strings.ContainsAny(key, "\r\n") || strings.ContainsAny(value, "\r\n") {
			return newError(KindUsage, "external resource metadata must be non-empty single lines", "Retry with non-secret single-line metadata")
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

func validateExternalResourceOwner(resource Resource, callerID string, expected uint64) error {
	if resource.Provenance != ProvenanceExternal {
		return externalOwnershipConflict("resource", resource.ID)
	}
	if resource.CallerID != callerID {
		return newError(KindConflict, fmt.Sprintf("external resource %s belongs to another caller", resource.ID), "Use the original caller ID")
	}
	return expectedRevisionCheck(expected, resource.Revision, "resource", resource.ID)
}

func validateExternalExecutionOwner(execution Execution, callerID string, expected uint64) error {
	if execution.Provenance != ProvenanceExternal {
		return externalOwnershipConflict("execution", execution.ID)
	}
	if execution.CallerID != callerID {
		return newError(KindConflict, fmt.Sprintf("external execution %s belongs to another caller", execution.ID), "Use the original caller ID")
	}
	return expectedRevisionCheck(expected, execution.Revision, "execution", execution.ID)
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
	receipts = append(receipts, RecordReceipt{OperationID: operationID, Fingerprint: fingerprint, Revision: revision})
	if len(receipts) > externalHistoryLimit {
		receipts = receipts[len(receipts)-externalHistoryLimit:]
	}
	return receipts
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

func appendExternalResourceObservation(history []ExternalResourceObservation, resource Resource, now time.Time) []ExternalResourceObservation {
	entry := ExternalResourceObservation{CallerID: resource.CallerID, ObservedAt: now, Repository: resource.Repository, Branch: resource.Branch, BaseRevision: resource.BaseRevision, WorktreePath: resource.WorktreePath, Head: resource.Git.Head}
	history = append(history, entry)
	if len(history) > externalHistoryLimit {
		history = history[len(history)-externalHistoryLimit:]
	}
	return history
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

func (s *Store) validatePredecessorLocked(taskID, executionID, predecessorID string) error {
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
