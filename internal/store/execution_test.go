package store

import (
	"sync"
	"testing"
	"time"
)

func TestExecutionRoundTripAndIndependentArchive(t *testing.T) {
	state := openTest(t)
	taskID := validTaskID(t)
	if err := state.WriteManifest(taskID, Manifest{Title: "executions", Lifecycle: "created"}); err != nil {
		t.Fatal(err)
	}
	execution := Execution{ID: "shell-one", TaskID: taskID, Label: "review-shell", Target: "shell", Command: "/bin/sh", Lifecycle: "stopped", Condition: "none"}
	if created, _, err := state.CreateExecution(taskID, execution); err != nil || !created {
		t.Fatalf("CreateExecution() = %v, %v", created, err)
	}
	if _, err := state.AppendExecutionEvent(taskID, execution.ID, Event{Operation: "create"}); err != nil {
		t.Fatal(err)
	}
	archive := ExecutionArchive{TaskID: taskID, ExecutionID: execution.ID, CapturedAt: time.Now().UTC(), Execution: execution}
	if err := state.WriteExecutionArchive(taskID, execution.ID, archive); err != nil {
		t.Fatal(err)
	}
	got, err := state.ReadExecution(taskID, execution.ID)
	if err != nil || got.Label != execution.Label {
		t.Fatalf("ReadExecution() = %#v, %v", got, err)
	}
	if _, err := state.ReadExecutionArchive(taskID, "missing"); !IsKind(err, KindNotFound) {
		t.Fatalf("ReadExecutionArchive(missing) = %v, want not found", err)
	}
}

func TestExecutionEventsStayContiguousUnderConcurrency(t *testing.T) {
	state := openTest(t)
	taskID := validTaskID(t)
	if err := state.WriteManifest(taskID, Manifest{Title: "concurrent", Lifecycle: "created"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := state.CreateExecution(taskID, Execution{ID: "execution", TaskID: taskID, Label: "concurrent", Target: "shell", Lifecycle: "created"}); err != nil {
		t.Fatal(err)
	}
	const writers = 12
	var group sync.WaitGroup
	errors := make(chan error, writers)
	for i := 0; i < writers; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := state.AppendExecutionEvent(taskID, "execution", Event{Operation: "observe"})
			if err != nil {
				errors <- err
			}
		}()
	}
	group.Wait()
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	events, err := state.ReadExecutionEvents(taskID, "execution")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != writers {
		t.Fatalf("ReadExecutionEvents() = %d, want %d", len(events), writers)
	}
	for i, event := range events {
		if event.Sequence != i+1 {
			t.Fatalf("event %d has sequence %d", i, event.Sequence)
		}
	}
}
