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

// Manifest is the typed mutable payload of a task manifest.
type Manifest struct {
	Title        string    `json:"title"`
	Worker       string    `json:"worker"`
	Repository   string    `json:"repository,omitempty"`
	Lifecycle    string    `json:"lifecycle"`
	Condition    string    `json:"condition"`
	Reason       string    `json:"reason,omitempty"`
	Activity     string    `json:"activity,omitempty"`
	HeartbeatAt  time.Time `json:"heartbeat_at,omitempty"`
	TmuxWindow   string    `json:"tmux_window,omitempty"`
	Requirements string    `json:"requirements,omitempty"`
	Warnings     string    `json:"warnings,omitempty"`
	Result       string    `json:"result,omitempty"`
	CleanupDebt  bool      `json:"cleanup_debt,omitempty"`
}

// Event is the typed immutable payload of an append-only task event.
type Event struct {
	Operation string `json:"operation"`
	Outcome   string `json:"outcome,omitempty"`
	Detail    string `json:"detail,omitempty"`
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

func isObjectPayload(data []byte) bool {
	trimmed := bytes.TrimSpace(data)
	return len(trimmed) > 0 && trimmed[0] == '{'
}
