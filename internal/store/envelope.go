package store

import (
	"bytes"
	"encoding/json"
	"time"
)

// SchemaVersion identifies the current envelope and payload schema.
// Readers reject envelopes with a schema version they do not understand
// rather than guessing at field meanings. See docs/storage.md.
const SchemaVersion = 1

// Record kinds stored inside an Envelope.
const (
	KindManifest = "manifest"
	KindEvent    = "event"
	KindArchive  = "archive"
)

// Envelope is the typed, versioned wrapper persisted for every record.
// It carries the schema version, kind, task ID, and observation time so each
// record is self-describing and tolerates concurrent reads and changes.
type Envelope struct {
	SchemaVersion int             `json:"schema_version"`
	Kind          string          `json:"kind"`
	TaskID        string          `json:"task_id"`
	ObservedAt    time.Time       `json:"observed_at"`
	Data          json.RawMessage `json:"data"`
}

// LaunchConfig is the durable, non-secret configuration for a managed agent.
// PromptReference identifies a local prompt file rather than embedding prompt
// content in process arguments or task events.
type LaunchConfig struct {
	Target           string `json:"target"`
	Command          string `json:"command"`
	PromptReference  string `json:"prompt_reference,omitempty"`
	WorkingDirectory string `json:"working_directory"`
	WorkingContext   string `json:"working_context,omitempty"`
}

// Manifest is the typed mutable payload of a task manifest.
type Manifest struct {
	Title                  string        `json:"title"`
	Worker                 string        `json:"worker"`
	Repository             string        `json:"repository,omitempty"`
	Branch                 string        `json:"branch,omitempty"`
	BaseRevision           string        `json:"base_revision,omitempty"`
	WorktreePath           string        `json:"worktree_path,omitempty"`
	Lifecycle              string        `json:"lifecycle"`
	Condition              string        `json:"condition"`
	Reason                 string        `json:"reason,omitempty"`
	Activity               string        `json:"activity,omitempty"`
	HeartbeatAt            time.Time     `json:"heartbeat_at,omitempty"`
	TmuxWindow             string        `json:"tmux_window,omitempty"`
	Requirements           string        `json:"requirements,omitempty"`
	Warnings               string        `json:"warnings,omitempty"`
	Result                 string        `json:"result,omitempty"`
	Committed              bool          `json:"committed,omitempty"`
	Dirty                  bool          `json:"dirty,omitempty"`
	Untracked              bool          `json:"untracked,omitempty"`
	RecoveryDebt           string        `json:"recovery_debt,omitempty"`
	ArchiveState           string        `json:"archive_state,omitempty"`
	CleanupState           string        `json:"cleanup_state,omitempty"`
	WorktreeCleanupState   string        `json:"worktree_cleanup_state,omitempty"`
	CredentialCleanupState string        `json:"credential_cleanup_state,omitempty"`
	CleanupDebt            bool          `json:"cleanup_debt,omitempty"`
	Git                    GitFacts      `json:"git,omitempty"`
	ProcessPID             int           `json:"process_pid,omitempty"`
	ProcessStartTime       uint64        `json:"process_start_time,omitempty"`
	ObservedPID            int           `json:"observed_pid,omitempty"`
	ObservedStartTime      uint64        `json:"observed_start_time,omitempty"`
	ProcessPane            string        `json:"process_pane,omitempty"`
	Observation            string        `json:"observation,omitempty"`
	ObservationAt          time.Time     `json:"observation_at,omitempty"`
	Launch                 *LaunchConfig `json:"launch,omitempty"`
}

// GitFacts are non-secret observations captured for recovery and cleanup
// decisions. They never contain credential values or repository file content.
type GitFacts struct {
	Path      string `json:"path,omitempty"`
	Head      string `json:"head,omitempty"`
	Branch    string `json:"branch,omitempty"`
	Dirty     bool   `json:"dirty,omitempty"`
	Untracked bool   `json:"untracked,omitempty"`
	Committed bool   `json:"committed,omitempty"`
}

// Event is the typed immutable payload of an append-only task event.
type Event struct {
	Operation string `json:"operation"`
	Outcome   string `json:"outcome,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

// EventRecord is a durable event together with its sequence and observation
// time. It is also used in task archives.
type EventRecord struct {
	Sequence   int       `json:"sequence"`
	ObservedAt time.Time `json:"observed_at"`
	Event      Event     `json:"event"`
}

// TaskArchive is an immutable snapshot of the durable task state and the
// non-secret resource facts available when archive was requested.
type TaskArchive struct {
	TaskID     string        `json:"task_id"`
	CapturedAt time.Time     `json:"captured_at"`
	Manifest   Manifest      `json:"manifest"`
	Events     []EventRecord `json:"events"`
	Git        GitFacts      `json:"git,omitempty"`
	Terminal   string        `json:"terminal,omitempty"`
	Warnings   []string      `json:"warnings,omitempty"`
}

func newEnvelope(kind, taskID string, data any) (Envelope, error) {
	encoded, err := json.Marshal(data)
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{
		SchemaVersion: SchemaVersion,
		Kind:          kind,
		TaskID:        taskID,
		ObservedAt:    time.Now().UTC(),
		Data:          encoded,
	}, nil
}

func manifestEnvelope(taskID string, manifest Manifest) (Envelope, error) {
	return newEnvelope(KindManifest, taskID, manifest)
}

func eventEnvelope(taskID string, event Event) (Envelope, error) {
	return newEnvelope(KindEvent, taskID, event)
}

func archiveEnvelope(taskID string, archive TaskArchive) (Envelope, error) {
	return newEnvelope(KindArchive, taskID, archive)
}

// DecodeManifest returns the typed manifest payload. Decode failures are
// returned as typed store errors rather than raw json errors.
func (e Envelope) DecodeManifest() (Manifest, error) {
	var manifest Manifest
	if !isObjectPayload(e.Data) {
		return Manifest{}, malformedError("Manifest payload must be a JSON object", "Inspect and repair the manifest record")
	}
	if err := json.Unmarshal(e.Data, &manifest); err != nil {
		return Manifest{}, malformedError("Malformed manifest payload", "Inspect and repair the manifest record")
	}
	return manifest, nil
}

// DecodeEvent returns the typed event payload. Decode failures are returned
// as typed store errors rather than raw json errors.
func (e Envelope) DecodeEvent() (Event, error) {
	var event Event
	if !isObjectPayload(e.Data) {
		return Event{}, malformedError("Event payload must be a JSON object", "Inspect and repair the event record")
	}
	if err := json.Unmarshal(e.Data, &event); err != nil {
		return Event{}, malformedError("Malformed event payload", "Inspect and repair the event record")
	}
	return event, nil
}

// DecodeArchive returns the typed archive payload.
func (e Envelope) DecodeArchive() (TaskArchive, error) {
	var archive TaskArchive
	if !isObjectPayload(e.Data) {
		return TaskArchive{}, malformedError("Archive payload must be a JSON object", "Inspect and repair the archive record")
	}
	if err := json.Unmarshal(e.Data, &archive); err != nil {
		return TaskArchive{}, malformedError("Malformed archive payload", "Inspect and repair the archive record")
	}
	return archive, nil
}

func isObjectPayload(data []byte) bool {
	trimmed := bytes.TrimSpace(data)
	return len(trimmed) > 0 && trimmed[0] == '{'
}
