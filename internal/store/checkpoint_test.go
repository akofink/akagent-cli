package store

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
)

func checkpointFixture(taskID string) CheckpointWrite {
	return CheckpointWrite{
		IdempotencyKey:   "checkpoint-write-1",
		ExpectedRevision: 0,
		Checkpoint: Checkpoint{
			TaskKind:                    "implementation",
			CompletionContract:          "signed commit is ready for review",
			NextAction:                  "run the required verification",
			ContextReferences:           []ContextReference{{Purpose: "handoff", Reference: "docs/handoff.md"}},
			ResourceReferences:          []CheckpointResourceReference{{ResourceID: "implementation", Branch: "akofink/137-recovery-checkpoints", Head: "abc123"}},
			SessionReferences:           []CheckpointSessionReference{{ExecutionID: "attempt-1", Tool: "pi", SessionID: "session-1"}},
			PreviousAttemptReferences:   []string{"attempt-0"},
			Verification:                &CheckpointVerification{Scope: "focused tests", Revision: "abc123", Result: "passed", VerifiedAt: time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)},
			UncertainExternalOperations: []UncertainExternalOperation{{ReferenceKey: "publish-1", Operation: "publish the branch", Verify: "verify the forge before replaying"}},
		},
	}
}

func createCheckpointTask(t *testing.T, state *Store, taskID string) {
	t.Helper()
	if err := state.WriteManifest(taskID, Manifest{Title: "checkpoint task", Worker: "local", Lifecycle: "created", Condition: "none"}); err != nil {
		t.Fatal(err)
	}
}

func TestCheckpointRevisionCheckedIdempotentWrite(t *testing.T) {
	state := openTest(t)
	const taskID = "checkpoint-task"
	createCheckpointTask(t, state, taskID)
	request := checkpointFixture(taskID)

	got, err := state.WriteCheckpoint(taskID, request)
	if err != nil {
		t.Fatalf("WriteCheckpoint() error = %v", err)
	}
	if got.Revision != 1 || got.TaskID != taskID || got.IdempotencyKey != request.IdempotencyKey {
		t.Fatalf("checkpoint = %#v, want revision 1 and task identity", got)
	}

	repeated, err := state.WriteCheckpoint(taskID, request)
	if err != nil {
		t.Fatalf("idempotent WriteCheckpoint() error = %v", err)
	}
	if !reflect.DeepEqual(repeated, got) {
		t.Fatalf("idempotent checkpoint = %#v, want %#v", repeated, got)
	}

	stale := request
	stale.IdempotencyKey = "checkpoint-write-2"
	_, err = state.WriteCheckpoint(taskID, stale)
	if !IsKind(err, KindConflict) {
		t.Fatalf("stale WriteCheckpoint() error = %v, want conflict", err)
	}

	conflicting := request
	conflicting.Checkpoint.NextAction = "replay an external operation"
	_, err = state.WriteCheckpoint(taskID, conflicting)
	if !IsKind(err, KindConflict) {
		t.Fatalf("conflicting retry error = %v, want conflict", err)
	}

	read, err := state.ReadCheckpoint(taskID)
	if err != nil {
		t.Fatalf("ReadCheckpoint() error = %v", err)
	}
	if !reflect.DeepEqual(read, got) {
		t.Fatalf("stored checkpoint = %#v, want %#v", read, got)
	}

	next := request
	next.IdempotencyKey = "checkpoint-write-2"
	next.ExpectedRevision = 1
	next.Checkpoint.NextAction = "inspect the verification result"
	if _, err := state.WriteCheckpoint(taskID, next); err != nil {
		t.Fatalf("second WriteCheckpoint() error = %v", err)
	}
	reused := request
	reused.ExpectedRevision = 2
	reused.Checkpoint.NextAction = "changed after an intervening revision"
	if _, err := state.WriteCheckpoint(taskID, reused); !IsKind(err, KindConflict) {
		t.Fatalf("reused old idempotency key error = %v, want conflict", err)
	}
}

func TestCheckpointConcurrentWritersHaveOneAcknowledgedRevision(t *testing.T) {
	state := openTest(t)
	const taskID = "checkpoint-race"
	createCheckpointTask(t, state, taskID)
	start := make(chan struct{})
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for index := 0; index < 2; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			request := checkpointFixture(taskID)
			request.IdempotencyKey = "checkpoint-race-" + string(rune('0'+index))
			<-start
			_, err := state.WriteCheckpoint(taskID, request)
			results <- err
		}(index)
	}
	close(start)
	wait.Wait()
	close(results)

	var succeeded, conflicts int
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		case IsKind(err, KindConflict):
			conflicts++
		default:
			t.Fatalf("concurrent write error = %v", err)
		}
	}
	if succeeded != 1 || conflicts != 1 {
		t.Fatalf("concurrent writes succeeded=%d conflicts=%d, want one each", succeeded, conflicts)
	}
	checkpoint, err := state.ReadCheckpoint(taskID)
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint.Revision != 1 {
		t.Fatalf("checkpoint revision = %d, want 1", checkpoint.Revision)
	}
}

func TestCheckpointEventInterruptionLeavesAuthoritativeRecord(t *testing.T) {
	state := openTest(t)
	const taskID = "checkpoint-partial"
	createCheckpointTask(t, state, taskID)
	state.checkpointEventFn = func(string, Event) error { return errors.New("synthetic event interruption") }
	request := checkpointFixture(taskID)

	_, err := state.WriteCheckpoint(taskID, request)
	if !IsKind(err, KindPartial) {
		t.Fatalf("interrupted WriteCheckpoint() error = %v, want partial", err)
	}
	state.checkpointEventFn = nil
	checkpoint, err := state.WriteCheckpoint(taskID, request)
	if err != nil {
		t.Fatalf("retry after interrupted event error = %v", err)
	}
	if checkpoint.Revision != 1 {
		t.Fatalf("recovered checkpoint revision = %d, want 1", checkpoint.Revision)
	}
	events, err := state.ReadEvents(taskID)
	if err != nil {
		t.Fatalf("ReadEvents() after repair error = %v", err)
	}
	if len(events) != 1 || events[0].Event.Operation != "checkpoint" {
		t.Fatalf("events after repair = %#v, want one checkpoint audit event", events)
	}
}

func TestCheckpointAuditDebtSurvivesInterveningRevisionAndRepairsHistoricalRetry(t *testing.T) {
	state := openTest(t)
	const taskID = "checkpoint-interleave"
	createCheckpointTask(t, state, taskID)
	failed := true
	state.checkpointEventFn = func(_ string, _ Event) error {
		if failed {
			failed = false
			return errors.New("synthetic first event interruption")
		}
		return nil
	}
	first := checkpointFixture(taskID)
	if _, err := state.WriteCheckpoint(taskID, first); !IsKind(err, KindPartial) {
		t.Fatalf("first interrupted write error = %v, want partial", err)
	}
	state.checkpointEventFn = nil
	second := first
	second.IdempotencyKey = "checkpoint-interleave-2"
	second.ExpectedRevision = 1
	second.Checkpoint.NextAction = "inspect the second revision"
	if _, err := state.WriteCheckpoint(taskID, second); err != nil {
		t.Fatalf("intervening revision write error = %v", err)
	}
	state.checkpointEventFn = nil
	current, err := state.ReadCheckpoint(taskID)
	if err != nil {
		t.Fatalf("ReadCheckpoint() after intervening revision error = %v", err)
	}
	if current.Revision != 2 || len(current.AuditDebt) != 1 || current.AuditDebt[0].Revision != 1 {
		t.Fatalf("checkpoint after intervening revision = %#v, want revision 2 with revision 1 audit debt", current)
	}

	var wait sync.WaitGroup
	errs := make(chan error, 2)
	revisions := make(chan uint64, 2)
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			retry, retryErr := state.WriteCheckpoint(taskID, first)
			errs <- retryErr
			revisions <- retry.Revision
		}()
	}
	wait.Wait()
	close(errs)
	close(revisions)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent historical retry error = %v", err)
		}
	}
	for revision := range revisions {
		if revision != 1 {
			t.Fatalf("historical retry revision = %d, want original revision 1", revision)
		}
	}
	current, err = state.ReadCheckpoint(taskID)
	if err != nil {
		t.Fatalf("ReadCheckpoint() error = %v", err)
	}
	if len(current.AuditDebt) != 0 {
		t.Fatalf("audit debt after historical repair = %#v, want none", current.AuditDebt)
	}
	events, err := state.ReadEvents(taskID)
	if err != nil {
		t.Fatalf("ReadEvents() error = %v", err)
	}
	checkpointEvents := 0
	for _, event := range events {
		if event.Event.Operation == "checkpoint" {
			checkpointEvents++
		}
	}
	if checkpointEvents != 2 {
		t.Fatalf("checkpoint audit events = %d, want exactly two", checkpointEvents)
	}
	if _, err := state.WriteCheckpoint(taskID, first); err != nil {
		t.Fatalf("repeated historical retry error = %v", err)
	}
	events, err = state.ReadEvents(taskID)
	if err != nil {
		t.Fatal(err)
	}
	checkpointEvents = 0
	for _, event := range events {
		if event.Event.Operation == "checkpoint" {
			checkpointEvents++
		}
	}
	if checkpointEvents != 2 {
		t.Fatalf("checkpoint audit events after repeat = %d, want exactly two", checkpointEvents)
	}
}

func TestReadCheckpointUnavailableForLegacyTask(t *testing.T) {
	state := openTest(t)
	createCheckpointTask(t, state, "legacy-checkpoint")
	if _, err := state.ReadCheckpoint("legacy-checkpoint"); !IsKind(err, KindNotFound) {
		t.Fatalf("ReadCheckpoint() error = %v, want not found", err)
	}
}
