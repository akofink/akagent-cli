package app

import (
	"testing"

	"github.com/akofink/akagent-cli/internal/lifecycle"
	"github.com/akofink/akagent-cli/internal/store"
)

func TestDefaultTaskViewEqualsInFlightForEveryDisposition(t *testing.T) {
	manifests := []store.Manifest{
		{Lifecycle: "created", Condition: "none", Disposition: string(lifecycle.DispositionInFlight), DispositionRevision: 0, ArchiveState: "complete"},
		{Lifecycle: "stopped", Condition: "none"},
		{Lifecycle: "running", Condition: "none", Observation: lifecycle.ObservationUnavailable, Disposition: "operator-review"},
		{Lifecycle: "finished", Condition: "succeeded", Result: "done"},
		{Lifecycle: "finished", Condition: "none", Disposition: string(lifecycle.DispositionDeferred)},
		{Lifecycle: "finished", Condition: "succeeded", Result: "done", Disposition: string(lifecycle.DispositionTerminal), CleanupDebt: true},
	}
	for index, manifest := range manifests {
		defaultView := includeTaskInView(manifest, nil, "")
		inFlightView := includeTaskInView(manifest, nil, "in-flight")
		if defaultView != inFlightView {
			t.Fatalf("manifest %d default view=%t, in-flight view=%t", index, defaultView, inFlightView)
		}
	}
}
