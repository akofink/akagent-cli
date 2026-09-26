package lifecycle

import (
	"fmt"
	"strings"

	"github.com/akofink/akagent-cli/internal/store"
)

// WorkDisposition separates accepted work intent from process and cleanup
// observations. It is deliberately independent of task lifecycle state.
type WorkDisposition string

const (
	DispositionInFlight WorkDisposition = "in-flight"
	DispositionDeferred WorkDisposition = "deferred"
	DispositionTerminal WorkDisposition = "terminal"
)

func (d WorkDisposition) valid() bool {
	return d == DispositionInFlight || d == DispositionDeferred || d == DispositionTerminal
}

// KnownWorkDisposition reports whether a persisted disposition is one of the
// supported values. Unknown values remain readable and are handled
// conservatively by inventory views.
func KnownWorkDisposition(d WorkDisposition) bool { return d.valid() }

// WorkDispositionOf returns the explicit disposition, or the conservative
// legacy interpretation for manifests written before disposition existed.
// Legacy work is terminal only when it records an explicit completed outcome.
func WorkDispositionOf(manifest store.Manifest) WorkDisposition {
	disposition := WorkDisposition(manifest.Disposition)
	if manifest.Disposition != "" {
		return disposition
	}
	if manifest.Lifecycle == "finished" &&
		(manifest.Condition == "succeeded" || manifest.Condition == "failed" || strings.TrimSpace(manifest.Result) != "") {
		return DispositionTerminal
	}
	return DispositionInFlight
}

func (m *Manager) SetDisposition(id string, disposition WorkDisposition, reason string, expectedRevision *uint64) (store.Manifest, error) {
	if !disposition.valid() {
		return store.Manifest{}, validationError("disposition must be in-flight, deferred, or terminal")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return store.Manifest{}, validationError("disposition reason is required")
	}

	changed := false
	manifest, err := m.Store.UpdateManifest(id, func(manifest *store.Manifest) error {
		if expectedRevision != nil && manifest.DispositionRevision != *expectedRevision {
			return &store.Error{
				Kind:      store.KindConflict,
				Message:   fmt.Sprintf("task disposition revision is %d, expected %d", manifest.DispositionRevision, *expectedRevision),
				Recovery:  fmt.Sprintf("Inspect task %s and retry with its current disposition revision", id),
				Retryable: false,
			}
		}
		if manifest.Disposition == string(disposition) && manifest.DispositionReason == reason {
			return nil
		}
		manifest.Disposition = string(disposition)
		manifest.DispositionReason = reason
		manifest.DispositionRevision++
		changed = true
		return nil
	})
	if err != nil {
		return store.Manifest{}, err
	}
	event := store.Event{Operation: "disposition", Outcome: string(disposition), Detail: reason, Revision: manifest.DispositionRevision}
	if changed || manifest.DispositionRevision > 0 {
		if err := m.appendDispositionEvent(id, event); err != nil {
			return store.Manifest{}, &store.Error{
				Kind:      store.KindPartial,
				Message:   fmt.Sprintf("Task %s disposition persisted but its audit event could not be appended", id),
				Retryable: true,
				Recovery:  fmt.Sprintf("Retry the same disposition command for task %s with --expected-revision %d", id, manifest.DispositionRevision),
				Err:       err,
			}
		}
	}
	return manifest, nil
}

func (m *Manager) appendDispositionEvent(id string, event store.Event) error {
	appendEvent := m.AppendEventIfAbsent
	if appendEvent == nil {
		appendEvent = m.Store.AppendEventIfAbsent
	}
	_, _, err := appendEvent(id, event)
	return err
}
