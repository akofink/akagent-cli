package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

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
