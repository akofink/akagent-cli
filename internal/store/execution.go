package store

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	KindExecutionManifest = "execution_manifest"
	KindExecutionEvent    = "execution_event"
	KindExecutionArchive  = "execution_archive"
)

// Execution is an optional, tool-neutral process associated with a task.
// Resource state is intentionally not embedded here: an execution can be
// stopped, observed, archived, and recovered without changing its resources.
type Execution struct {
	ID                string    `json:"id"`
	TaskID            string    `json:"task_id"`
	Label             string    `json:"label"`
	Target            string    `json:"target"`
	Command           string    `json:"command,omitempty"`
	Arguments         []string  `json:"arguments,omitempty"`
	ResourceID        string    `json:"resource_id,omitempty"`
	WorkingDirectory  string    `json:"working_directory,omitempty"`
	Lifecycle         string    `json:"lifecycle"`
	Condition         string    `json:"condition"`
	Reason            string    `json:"reason,omitempty"`
	Activity          string    `json:"activity,omitempty"`
	HeartbeatAt       time.Time `json:"heartbeat_at,omitempty"`
	Result            string    `json:"result,omitempty"`
	TmuxWindow        string    `json:"tmux_window,omitempty"`
	ProcessPID        int       `json:"process_pid,omitempty"`
	ProcessStartTime  uint64    `json:"process_start_time,omitempty"`
	ObservedPID       int       `json:"observed_pid,omitempty"`
	ObservedStartTime uint64    `json:"observed_start_time,omitempty"`
	ProcessPane       string    `json:"process_pane,omitempty"`
	Observation       string    `json:"observation,omitempty"`
	ObservationAt     time.Time `json:"observation_at,omitempty"`
	ArchiveState      string    `json:"archive_state,omitempty"`
	RecoveryDebt      string    `json:"recovery_debt,omitempty"`
}

// ExecutionArchive is an independently recoverable execution snapshot.
type ExecutionArchive struct {
	TaskID      string        `json:"task_id"`
	ExecutionID string        `json:"execution_id"`
	CapturedAt  time.Time     `json:"captured_at"`
	Execution   Execution     `json:"execution"`
	Events      []EventRecord `json:"events"`
	Terminal    string        `json:"terminal,omitempty"`
	Warnings    []string      `json:"warnings,omitempty"`
}

func validateExecutionID(id string) error {
	if id == "" || !taskIDPattern.MatchString(id) {
		return newError(KindUsage, fmt.Sprintf("Invalid execution ID %q", id), "Use a short stable execution ID or `akagent id generate`")
	}
	return nil
}

func (s *Store) WriteExecution(taskID string, execution Execution) error {
	if err := validateTaskID(taskID); err != nil {
		return err
	}
	if err := validateExecutionID(execution.ID); err != nil {
		return err
	}
	if execution.TaskID != "" && execution.TaskID != taskID {
		return newError(KindUsage, "Execution task ID does not match its path", "Retry with the requested task ID")
	}
	execution.TaskID = taskID
	return s.WithLock(taskID, func() error { return s.writeExecutionLocked(taskID, execution) })
}

func (s *Store) CreateExecution(taskID string, execution Execution) (bool, Execution, error) {
	if err := validateTaskID(taskID); err != nil {
		return false, Execution{}, err
	}
	if err := validateExecutionID(execution.ID); err != nil {
		return false, Execution{}, err
	}
	execution.TaskID = taskID
	var created bool
	var existing Execution
	err := s.WithLock(taskID, func() error {
		if _, err := s.ReadManifest(taskID); err != nil {
			return err
		}
		current, err := s.ReadExecution(taskID, execution.ID)
		if err == nil {
			existing = current
			if !sameExecutionInputs(current, execution) {
				return &Error{Kind: KindConflict, Message: fmt.Sprintf("execution %s inputs conflict with the existing execution", execution.ID), Recovery: fmt.Sprintf("Inspect execution %s for task %s", execution.ID, taskID)}
			}
			return nil
		}
		if !IsKind(err, KindNotFound) {
			return err
		}
		if err := s.writeExecutionLocked(taskID, execution); err != nil {
			return err
		}
		created, existing = true, execution
		return nil
	})
	return created, existing, err
}

func (s *Store) ReadExecution(taskID, executionID string) (Execution, error) {
	if err := validateTaskID(taskID); err != nil {
		return Execution{}, err
	}
	if err := validateExecutionID(executionID); err != nil {
		return Execution{}, err
	}
	path := s.executionManifestPath(taskID, executionID)
	if err := s.checkTaskDir(taskID); err != nil {
		return Execution{}, err
	}
	data, err := s.readOwnedFile(path)
	if err != nil {
		if IsKind(err, KindNotFound) {
			return Execution{}, newError(KindNotFound, fmt.Sprintf("No execution %s found for task %s", executionID, taskID), fmt.Sprintf("List executions for task %s", taskID))
		}
		return Execution{}, err
	}
	envelope, err := decodeExecutionEnvelope(path, data, KindExecutionManifest, taskID, executionID)
	if err != nil {
		return Execution{}, err
	}
	return envelope.DecodeExecution()
}

func (s *Store) UpdateExecution(taskID, executionID string, update func(*Execution) error) (Execution, error) {
	if err := validateTaskID(taskID); err != nil {
		return Execution{}, err
	}
	if err := validateExecutionID(executionID); err != nil {
		return Execution{}, err
	}
	var execution Execution
	err := s.WithLock(taskID, func() error {
		current, err := s.ReadExecution(taskID, executionID)
		if err != nil {
			return err
		}
		execution = current
		if err := update(&execution); err != nil {
			return err
		}
		if execution.ID != executionID || execution.TaskID != taskID {
			return newError(KindUsage, "Execution identity cannot be changed", "Retry without changing the execution ID")
		}
		return s.writeExecutionLocked(taskID, execution)
	})
	return execution, err
}

func (s *Store) ExecutionIDs(taskID string) ([]string, error) {
	if err := validateTaskID(taskID); err != nil {
		return nil, err
	}
	if err := s.checkTaskDir(taskID); err != nil {
		return nil, err
	}
	dir := s.executionsDir(taskID)
	file, err := s.openOwned(dir, true)
	if err != nil {
		if IsKind(err, KindNotFound) {
			return []string{}, nil
		}
		return nil, err
	}
	defer file.Close()
	entries, err := file.ReadDir(-1)
	if err != nil {
		return nil, internalError(fmt.Sprintf("list executions for task %s", taskID), "Check the task state and retry")
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() && validateExecutionID(entry.Name()) == nil {
			ids = append(ids, entry.Name())
		}
	}
	sort.Strings(ids)
	return ids, nil
}

func (s *Store) AppendExecutionEvent(taskID, executionID string, event Event) (int, error) {
	if err := validateTaskID(taskID); err != nil {
		return 0, err
	}
	if err := validateExecutionID(executionID); err != nil {
		return 0, err
	}
	var sequence int
	err := s.WithLock(taskID, func() error {
		if _, err := s.ReadExecution(taskID, executionID); err != nil {
			return err
		}
		if err := s.ensureExecutionDir(taskID, executionID); err != nil {
			return err
		}
		envelope, err := executionEventEnvelope(taskID, executionID, event)
		if err != nil {
			return internalError("encode an execution event", "Retry the operation")
		}
		encoded, err := encodeRecord(envelope)
		if err != nil {
			return err
		}
		events, err := s.ReadExecutionEvents(taskID, executionID)
		if err != nil {
			return err
		}
		sequence = len(events) + 1
		return s.atomicallyWrite(s.executionEventPath(taskID, executionID, sequence), encoded)
	})
	return sequence, err
}

func (s *Store) ReadExecutionEvents(taskID, executionID string) ([]EventRecord, error) {
	if err := validateTaskID(taskID); err != nil {
		return nil, err
	}
	if err := validateExecutionID(executionID); err != nil {
		return nil, err
	}
	if _, err := s.ReadExecution(taskID, executionID); err != nil {
		return nil, err
	}
	dir := s.executionEventsDir(taskID, executionID)
	file, err := s.openOwned(dir, true)
	if err != nil {
		if IsKind(err, KindNotFound) {
			return []EventRecord{}, nil
		}
		return nil, err
	}
	defer file.Close()
	entries, err := file.ReadDir(-1)
	if err != nil {
		return nil, internalError("list execution events", "Check the execution state and retry")
	}
	sequences := make([]int, 0, len(entries))
	files := map[int]string{}
	for _, entry := range entries {
		sequence, ok := parseEventSequence(entry.Name())
		if entry.IsDir() || !ok || strings.HasPrefix(entry.Name(), ".") || files[sequence] != "" {
			return nil, malformedError(fmt.Sprintf("Malformed execution event history for %s/%s", taskID, executionID), fmt.Sprintf("Inspect %s", dir))
		}
		sequences = append(sequences, sequence)
		files[sequence] = entry.Name()
	}
	sort.Ints(sequences)
	if err := checkSequenceContiguity(sequences, taskID+"/"+executionID); err != nil {
		return nil, err
	}
	result := make([]EventRecord, 0, len(sequences))
	for _, sequence := range sequences {
		path := filepath.Join(dir, files[sequence])
		data, err := s.readOwnedFile(path)
		if err != nil {
			return nil, err
		}
		envelope, err := decodeExecutionEnvelope(path, data, KindExecutionEvent, taskID, executionID)
		if err != nil {
			return nil, err
		}
		event, err := envelope.DecodeEvent()
		if err != nil {
			return nil, err
		}
		result = append(result, EventRecord{Sequence: sequence, ObservedAt: envelope.ObservedAt, Event: event})
	}
	return result, nil
}

func (s *Store) WriteExecutionArchive(taskID, executionID string, archive ExecutionArchive) error {
	if err := validateTaskID(taskID); err != nil {
		return err
	}
	if err := validateExecutionID(executionID); err != nil {
		return err
	}
	if archive.TaskID != taskID || archive.ExecutionID != executionID {
		return newError(KindUsage, "Execution archive identity does not match its path", "Retry archive with the requested execution")
	}
	if archive.CapturedAt.IsZero() {
		return newError(KindUsage, "Execution archive capture time is required", "Retry archive")
	}
	return s.WithLock(taskID, func() error {
		if err := s.ensureExecutionDir(taskID, executionID); err != nil {
			return err
		}
		envelope, err := executionArchiveEnvelope(taskID, executionID, archive)
		if err != nil {
			return internalError("encode an execution archive", "Retry the operation")
		}
		encoded, err := encodeRecord(envelope)
		if err != nil {
			return err
		}
		return s.atomicallyWrite(s.executionArchivePath(taskID, executionID), encoded)
	})
}

func (s *Store) ReadExecutionArchive(taskID, executionID string) (ExecutionArchive, error) {
	if err := validateTaskID(taskID); err != nil {
		return ExecutionArchive{}, err
	}
	if err := validateExecutionID(executionID); err != nil {
		return ExecutionArchive{}, err
	}
	path := s.executionArchivePath(taskID, executionID)
	data, err := s.readOwnedFile(path)
	if err != nil {
		if IsKind(err, KindNotFound) {
			return ExecutionArchive{}, newError(KindNotFound, fmt.Sprintf("No archive found for execution %s", executionID), "Archive the execution before removing its history")
		}
		return ExecutionArchive{}, err
	}
	envelope, err := decodeExecutionEnvelope(path, data, KindExecutionArchive, taskID, executionID)
	if err != nil {
		return ExecutionArchive{}, err
	}
	archive, err := envelope.DecodeExecutionArchive()
	if err != nil || archive.TaskID != taskID || archive.ExecutionID != executionID || archive.CapturedAt.IsZero() {
		return ExecutionArchive{}, malformedError(fmt.Sprintf("Malformed archive for execution %s", executionID), fmt.Sprintf("Inspect and repair %s", path))
	}
	return archive, nil
}

func (s *Store) writeExecutionLocked(taskID string, execution Execution) error {
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

func (s *Store) ensureExecutionDir(taskID, executionID string) error {
	if err := s.ensureTaskDir(taskID); err != nil {
		return err
	}
	for _, entry := range []struct{ dir, label string }{{s.executionsDir(taskID), "executions directory"}, {s.executionDir(taskID, executionID), "execution directory"}, {s.executionEventsDir(taskID, executionID), "execution events directory"}} {
		if err := s.ensureDir(entry.dir, entry.label); err != nil {
			return err
		}
	}
	return nil
}

func sameExecutionInputs(a, b Execution) bool {
	return a.ID == b.ID && a.TaskID == b.TaskID && a.Label == b.Label && a.Target == b.Target && a.Command == b.Command && strings.Join(a.Arguments, "\x00") == strings.Join(b.Arguments, "\x00") && a.ResourceID == b.ResourceID && a.WorkingDirectory == b.WorkingDirectory
}

func executionManifestEnvelope(taskID, executionID string, execution Execution) (Envelope, error) {
	envelope, err := newEnvelope(KindExecutionManifest, taskID, execution)
	envelope.ExecutionID = executionID
	return envelope, err
}
func executionEventEnvelope(taskID, executionID string, event Event) (Envelope, error) {
	envelope, err := newEnvelope(KindExecutionEvent, taskID, event)
	envelope.ExecutionID = executionID
	return envelope, err
}
func executionArchiveEnvelope(taskID, executionID string, archive ExecutionArchive) (Envelope, error) {
	envelope, err := newEnvelope(KindExecutionArchive, taskID, archive)
	envelope.ExecutionID = executionID
	return envelope, err
}

func decodeExecutionEnvelope(path string, data []byte, kind, taskID, executionID string) (Envelope, error) {
	envelope, err := decodeEnvelope(path, data, kind, taskID)
	if err != nil {
		return Envelope{}, err
	}
	if envelope.ExecutionID != executionID {
		return Envelope{}, malformedError(fmt.Sprintf("Execution record at %s has the wrong execution ID", path), fmt.Sprintf("Inspect and repair %s", path))
	}
	return envelope, nil
}

func (e Envelope) DecodeExecution() (Execution, error) {
	var execution Execution
	if !isObjectPayload(e.Data) || json.Unmarshal(e.Data, &execution) != nil {
		return Execution{}, malformedError("Malformed execution manifest payload", "Inspect and repair the execution record")
	}
	if execution.ID == "" || execution.TaskID == "" || execution.TaskID != e.TaskID || execution.ID != e.ExecutionID {
		return Execution{}, malformedError("Execution manifest is missing or has mismatched identity", "Inspect and repair the execution record")
	}
	return execution, nil
}
func (e Envelope) DecodeExecutionArchive() (ExecutionArchive, error) {
	var archive ExecutionArchive
	if !isObjectPayload(e.Data) || json.Unmarshal(e.Data, &archive) != nil {
		return ExecutionArchive{}, malformedError("Malformed execution archive payload", "Inspect and repair the execution archive")
	}
	return archive, nil
}

func (s *Store) executionsDir(taskID string) string {
	return filepath.Join(s.taskDir(taskID), "executions")
}
func (s *Store) executionDir(taskID, executionID string) string {
	return filepath.Join(s.executionsDir(taskID), executionID)
}
func (s *Store) executionManifestPath(taskID, executionID string) string {
	return filepath.Join(s.executionDir(taskID, executionID), "manifest.json")
}
func (s *Store) executionEventsDir(taskID, executionID string) string {
	return filepath.Join(s.executionDir(taskID, executionID), "events")
}
func (s *Store) executionEventPath(taskID, executionID string, sequence int) string {
	return filepath.Join(s.executionEventsDir(taskID, executionID), fmt.Sprintf("%0*d.json", eventSequenceWidth, sequence))
}
func (s *Store) executionArchivePath(taskID, executionID string) string {
	return filepath.Join(s.executionDir(taskID, executionID), "archive.json")
}
