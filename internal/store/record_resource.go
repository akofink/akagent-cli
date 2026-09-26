package store

import (
	"fmt"
	"os"
	"time"
)

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
