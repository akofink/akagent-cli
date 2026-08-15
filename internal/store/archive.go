package store

import (
	"fmt"
	"path/filepath"
)

// WriteArchive atomically replaces the task archive under the task lock.
// Replacing an existing archive is safe and makes interrupted archive retries
// idempotent.
func (s *Store) WriteArchive(taskID string, archive TaskArchive) error {
	if err := validateTaskID(taskID); err != nil {
		return err
	}
	if archive.TaskID != taskID {
		return newError(KindUsage, "Archive task ID does not match its path", "Retry archive with the requested task ID")
	}
	if archive.CapturedAt.IsZero() {
		return newError(KindUsage, "Archive capture time is required", "Retry archive")
	}
	return s.WithLock(taskID, func() error {
		if err := s.ensureTaskDir(taskID); err != nil {
			return err
		}
		envelope, err := archiveEnvelope(taskID, archive)
		if err != nil {
			return internalError("encode a task archive", "Retry the operation")
		}
		encoded, err := encodeRecord(envelope)
		if err != nil {
			return err
		}
		return s.atomicallyWrite(s.archivePath(taskID), encoded)
	})
}

// ReadArchive reads the immutable task archive snapshot.
func (s *Store) ReadArchive(taskID string) (TaskArchive, error) {
	if err := validateTaskID(taskID); err != nil {
		return TaskArchive{}, err
	}
	if err := s.checkTaskDir(taskID); err != nil {
		return TaskArchive{}, err
	}
	data, err := s.readOwnedFile(s.archivePath(taskID))
	if err != nil {
		if IsKind(err, KindNotFound) {
			return TaskArchive{}, newError(KindNotFound, fmt.Sprintf("No archive found for task %s", taskID), fmt.Sprintf("Archive task %s before cleaning it", taskID))
		}
		return TaskArchive{}, err
	}
	envelope, err := decodeEnvelope(s.archivePath(taskID), data, KindArchive, taskID)
	if err != nil {
		return TaskArchive{}, err
	}
	archive, err := envelope.DecodeArchive()
	if err != nil || archive.TaskID != taskID || archive.CapturedAt.IsZero() {
		return TaskArchive{}, malformedError(
			fmt.Sprintf("Malformed archive for task %s", taskID),
			fmt.Sprintf("Inspect and repair %s", s.archivePath(taskID)))
	}
	return archive, nil
}

func (s *Store) validateArchiveForRecovery(taskID string, result *RecoveryResult) error {
	path := s.archivePath(taskID)
	data, err := s.readOwnedFile(path)
	if err != nil {
		if !IsKind(err, KindNotFound) {
			result.MalformedRecords = append(result.MalformedRecords, err.Error())
		}
		return nil
	}
	envelope, err := decodeEnvelope(path, data, KindArchive, taskID)
	if err != nil {
		result.MalformedRecords = append(result.MalformedRecords, path)
		return nil
	}
	archive, err := envelope.DecodeArchive()
	if err != nil || archive.TaskID != taskID || archive.CapturedAt.IsZero() {
		result.MalformedRecords = append(result.MalformedRecords, path)
	}
	return nil
}

func (s *Store) archivePath(taskID string) string {
	return filepath.Join(s.taskDir(taskID), "archive.json")
}
