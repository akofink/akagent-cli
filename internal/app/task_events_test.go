package app

import (
	"strings"
	"testing"

	"github.com/akofink/akagent-cli/internal/store"
)

func TestTaskEventsReturnsTimestampMetadataOnly(t *testing.T) {
	setupTaskCommandTest(t)
	state, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	if err := state.WriteManifest("event-time-test", store.Manifest{Title: "Synthetic", Worker: "local", Lifecycle: "created", Condition: "active"}); err != nil {
		t.Fatal(err)
	}
	if _, err := state.AppendEvent("event-time-test", store.Event{Operation: "created", Detail: "synthetic-private-detail", OperationID: "synthetic-operation-id"}); err != nil {
		t.Fatal(err)
	}
	// The command contract is tested against synthetic immutable stored history.
	before, err := state.ReadEvents("event-time-test")
	if err != nil || len(before) != 1 {
		t.Fatalf("ReadEvents() = (%v, %v)", before, err)
	}
	result := runCommand(t, []string{"task", "events", "event-time-test"})
	if result.code != 0 {
		t.Fatalf("task events code = %d, output = %s", result.code, result.stdout)
	}
	for _, want := range []string{"event-time-test", "events[1]{sequence,observed_at,operation}:", "1,", ",created"} {
		if !strings.Contains(result.stdout, want) {
			t.Errorf("task events output missing %q: %s", want, result.stdout)
		}
	}
	for _, forbidden := range []string{"synthetic-private-detail", "synthetic-operation-id", "fingerprint"} {
		if strings.Contains(result.stdout, forbidden) {
			t.Errorf("task events exposed %q", forbidden)
		}
	}
	after, err := state.ReadEvents("event-time-test")
	if err != nil || len(after) != len(before) {
		t.Fatalf("task events changed stored history: before=%d after=%d err=%v", len(before), len(after), err)
	}
}

func TestTaskEventsRequiresTaskID(t *testing.T) {
	setupTaskCommandTest(t)
	result := runCommand(t, []string{"task", "events"})
	if result.code != 2 || !strings.Contains(result.stdout, "Usage: akagent task events <task-id>") {
		t.Fatalf("task events usage = (%d, %q)", result.code, result.stdout)
	}
}
