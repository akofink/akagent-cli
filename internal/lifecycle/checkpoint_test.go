package lifecycle

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akofink/akagent-cli/internal/store"
)

func TestInspectCheckpointReportsLegacyTaskAsUnavailable(t *testing.T) {
	manager, _ := newTestManager(t)
	state := manager.Store
	if err := state.WriteManifest("legacy-checkpoint", store.Manifest{Title: "legacy", Worker: "local", Lifecycle: "created", Condition: "none"}); err != nil {
		t.Fatal(err)
	}

	inspection, err := New(state).InspectCheckpoint("legacy-checkpoint")
	if err != nil {
		t.Fatalf("InspectCheckpoint() error = %v", err)
	}
	if inspection.Available || inspection.State != "unavailable" || inspection.Checkpoint != nil {
		t.Fatalf("inspection = %#v, want explicit unavailable state", inspection)
	}
	if inspection.Reason == "" {
		t.Fatal("inspection reason is empty")
	}
}

func TestInspectCheckpointRejectsUnknownTask(t *testing.T) {
	manager, _ := newTestManager(t)
	if _, err := manager.InspectCheckpoint("missing-checkpoint-task"); !store.IsKind(err, store.KindNotFound) {
		t.Fatalf("InspectCheckpoint() error = %v, want not found", err)
	}
}

func TestWriteCheckpointIsStateOnlyAndRevisionChecked(t *testing.T) {
	manager, _ := newTestManager(t)
	state := manager.Store
	if err := state.WriteManifest("lifecycle-checkpoint", store.Manifest{Title: "checkpoint", Worker: "local", Lifecycle: "created", Condition: "none"}); err != nil {
		t.Fatal(err)
	}
	request := CheckpointRequest{
		IdempotencyKey: "lifecycle-write-1",
		Checkpoint: store.Checkpoint{
			TaskKind:           "review",
			CompletionContract: "review artifact is recorded",
			NextAction:         "inspect the review artifact",
		},
	}
	checkpoint, err := manager.WriteCheckpoint("lifecycle-checkpoint", request)
	if err != nil {
		t.Fatalf("WriteCheckpoint() error = %v", err)
	}
	if checkpoint.Revision != 1 {
		t.Fatalf("checkpoint revision = %d, want 1", checkpoint.Revision)
	}
	request.Checkpoint.NextAction = "replay the review"
	if _, err := manager.WriteCheckpoint("lifecycle-checkpoint", request); !store.IsKind(err, store.KindConflict) {
		t.Fatalf("conflicting retry error = %v, want conflict", err)
	}
}

func TestArchiveIncludesAcknowledgedCheckpoint(t *testing.T) {
	manager, _ := newTestManager(t)
	state := manager.Store
	const taskID = "archived-checkpoint"
	if err := state.WriteManifest(taskID, store.Manifest{Title: "checkpoint archive", Worker: "local", Lifecycle: "stopped", Condition: "none"}); err != nil {
		t.Fatal(err)
	}
	checkpoint, err := manager.WriteCheckpoint(taskID, CheckpointRequest{
		IdempotencyKey: "archive-checkpoint-1",
		Checkpoint:     store.Checkpoint{TaskKind: "implementation", CompletionContract: "archive contains the handoff", NextAction: "read the archived handoff"},
	})
	if err != nil {
		t.Fatalf("WriteCheckpoint() error = %v", err)
	}
	if _, err := manager.Archive(taskID); err != nil {
		t.Fatalf("Archive() error = %v", err)
	}
	if err := os.Remove(filepath.Join(state.Root(), "tasks", taskID, "checkpoint.json")); err != nil {
		t.Fatalf("remove live checkpoint = %v", err)
	}
	archive, err := state.ReadArchive(taskID)
	if err != nil {
		t.Fatalf("ReadArchive() error = %v", err)
	}
	if archive.Checkpoint == nil || archive.Checkpoint.Revision != checkpoint.Revision || archive.Checkpoint.NextAction != checkpoint.NextAction {
		t.Fatalf("archive checkpoint = %#v, want acknowledged checkpoint %#v", archive.Checkpoint, checkpoint)
	}
}
