package store

import (
	"os"
	"path/filepath"
	"strings"
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

func TestExecutionSessionReferencesRoundTripAndArchive(t *testing.T) {
	state := openTest(t)
	taskID := validTaskID(t)
	if err := state.WriteManifest(taskID, Manifest{Title: "sessions", Lifecycle: "stopped"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "session.json")
	if err := os.WriteFile(path, []byte("provider state"), 0o600); err != nil {
		t.Fatal(err)
	}
	missingPath := filepath.Join(t.TempDir(), "gone.json")
	if err := os.WriteFile(missingPath, []byte("historical provider state"), 0o600); err != nil {
		t.Fatal(err)
	}
	execution := Execution{
		ID: "sessioned", TaskID: taskID, Label: "provider", Target: "tool", Lifecycle: "stopped", Condition: "none",
		SessionReferences: []SessionReference{
			{Tool: "pi", SessionID: "session-one", ReferencePath: path},
			{Tool: "other", SessionID: "session-two", ReferencePath: missingPath},
		},
	}
	if _, _, err := state.CreateExecution(taskID, execution); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(missingPath); err != nil {
		t.Fatal(err)
	}
	got, err := state.ReadExecution(taskID, execution.ID)
	if err != nil || len(got.SessionReferences) != 2 {
		t.Fatalf("ReadExecution() = %#v, %v", got, err)
	}
	archive := ExecutionArchive{TaskID: taskID, ExecutionID: execution.ID, CapturedAt: time.Now().UTC(), Execution: got}
	if err := state.WriteExecutionArchive(taskID, execution.ID, archive); err != nil {
		t.Fatal(err)
	}
	archived, err := state.ReadExecutionArchive(taskID, execution.ID)
	if err != nil || len(archived.Execution.SessionReferences) != 2 {
		t.Fatalf("ReadExecutionArchive() = %#v, %v", archived, err)
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

func TestManagedExecutionFinishRequiresTerminalTaskAndPreservesProvenance(t *testing.T) {
	state := openTest(t)
	taskID := validTaskID(t)
	if err := state.WriteManifest(taskID, Manifest{Title: "stuck", Lifecycle: "created"}); err != nil {
		t.Fatal(err)
	}
	execution := Execution{ID: "stuck-1", TaskID: taskID, Label: "external", Target: "external", Command: "pi", Lifecycle: "created", Condition: "active"}
	if _, _, err := state.CreateExecution(taskID, execution); err != nil {
		t.Fatal(err)
	}
	if _, err := state.CompleteExternalExecution(taskID, execution.ID, "caller", "before-terminal", "delivery", "succeeded", 0); err == nil || !IsKind(err, KindConflict) || !strings.Contains(err.Error(), "not an externally declared record") {
		t.Fatalf("nonterminal finish = %v", err)
	}
	if _, err := state.UpdateManifest(taskID, func(manifest *Manifest) error {
		manifest.Lifecycle = "finished"
		manifest.ArchiveState = "complete"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := state.CompleteExternalExecution(taskID, execution.ID, "caller", "stale", "delivery", "succeeded", 1); err == nil || !strings.Contains(err.Error(), "revision conflict") {
		t.Fatalf("stale finish = %v", err)
	}
	finished, err := state.CompleteExternalExecution(taskID, execution.ID, "caller", "close-1", "delivery", "succeeded", 0)
	if err != nil {
		t.Fatal(err)
	}
	if finished.Provenance != "" || finished.CallerID != "" || finished.Lifecycle != "finished" || finished.Condition != "none" || finished.Revision != 1 || finished.Result != "succeeded" || finished.ExternalCompletion == nil {
		t.Fatalf("finished managed execution = %#v", finished)
	}
	retry, err := state.CompleteExternalExecution(taskID, execution.ID, "caller", "close-1", "delivery", "succeeded", 0)
	if err != nil || retry.Revision != 1 {
		t.Fatalf("retry = %#v, %v", retry, err)
	}
	if _, err := state.CompleteExternalExecution(taskID, execution.ID, "caller", "close-1", "delivery", "failed", 1); err == nil || !strings.Contains(err.Error(), "different inputs") {
		t.Fatalf("changed retry = %v", err)
	}
	if _, err := state.CompleteExternalExecution(taskID, execution.ID, "caller", "close-2", "delivery", "failed", 1); err == nil || !IsKind(err, KindConflict) {
		t.Fatalf("second close = %v", err)
	}
}

func TestManagedExecutionFinishIsRevisionCheckedUnderConcurrency(t *testing.T) {
	state := openTest(t)
	taskID := "019fe8f2-ac67-7406-a6e6-2717b2cd31c7"
	if err := state.WriteManifest(taskID, Manifest{Title: "race", Lifecycle: "finished", ArchiveState: "complete"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := state.CreateExecution(taskID, Execution{ID: "race-1", TaskID: taskID, Label: "external", Target: "external", Lifecycle: "created", Condition: "active"}); err != nil {
		t.Fatal(err)
	}
	errors := make(chan error, 2)
	var group sync.WaitGroup
	for _, attempt := range []struct{ operationID, result string }{{"race-a", "succeeded"}, {"race-b", "failed"}} {
		group.Add(1)
		go func(operationID, result string) {
			defer group.Done()
			_, err := state.CompleteExternalExecution(taskID, "race-1", "caller", operationID, "delivery", result, 0)
			errors <- err
		}(attempt.operationID, attempt.result)
	}
	group.Wait()
	close(errors)
	successes := 0
	for err := range errors {
		if err == nil {
			successes++
			continue
		}
		if !IsKind(err, KindConflict) {
			t.Fatalf("concurrent finish = %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("successful finishes = %d, want 1", successes)
	}
	got, err := state.ReadExecution(taskID, "race-1")
	if err != nil || got.Revision != 1 || got.Lifecycle != "finished" || got.Provenance != "" {
		t.Fatalf("raced execution = %#v, %v", got, err)
	}
}
