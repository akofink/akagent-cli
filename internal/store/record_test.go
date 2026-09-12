package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestExternalRecordsWorkWithoutReferencedHostState(t *testing.T) {
	state := openTest(t)
	taskID := validTaskID(t)
	if err := state.WriteManifest(taskID, Manifest{Title: "external", Worker: "local", Lifecycle: "created", Condition: "none"}); err != nil {
		t.Fatal(err)
	}
	request := ExternalResourceRequest{ID: "resource", OperationID: "adopt-1", CallerID: "caller", Repository: "offline-repository", Branch: "akofink/external", BaseRevision: "base", Head: "head-1", WorktreePath: filepath.Join(t.TempDir(), "missing"), Metadata: map[string]string{"source": "test"}}
	resource, err := state.AdoptExternalResource(taskID, request)
	if err != nil {
		t.Fatal(err)
	}
	if resource.Provenance != ProvenanceExternal || resource.Revision != 1 {
		t.Fatalf("adopted resource = %#v", resource)
	}
	repeated, err := state.AdoptExternalResource(taskID, request)
	if err != nil || repeated.Revision != 1 {
		t.Fatalf("idempotent adoption = %#v, %v", repeated, err)
	}
	request.OperationID, request.ExpectedRevision, request.Head = "adopt-2", 1, "head-2"
	updated, err := state.AdoptExternalResource(taskID, request)
	if err != nil || updated.Revision != 2 || len(updated.ObservationHistory) != 1 {
		t.Fatalf("updated resource = %#v, %v", updated, err)
	}
	request.ExpectedRevision = 1
	request.OperationID = "stale-update"
	if _, err := state.AdoptExternalResource(taskID, request); !IsKind(err, KindConflict) {
		t.Fatalf("stale adoption error = %v, want conflict", err)
	}

	execution, err := state.RecordExternalExecution(taskID, ExternalExecutionRequest{ID: "attempt", OperationID: "execution-1", CallerID: "caller", ResourceID: "resource", SessionReferences: []SessionReference{{Tool: "offline", SessionID: "session"}}})
	if err != nil {
		t.Fatal(err)
	}
	if execution.Revision != 1 || execution.Lifecycle != "created" {
		t.Fatalf("external execution = %#v", execution)
	}
	observation := ExternalObservation{Source: "offline", ObservedAt: time.Now().UTC(), HostID: "host", BootID: "previous-boot", ProcessState: "running"}
	execution, err = state.ObserveExternalExecution(taskID, "attempt", "caller", "observation-1", 1, observation)
	if err != nil || execution.Revision != 2 || len(execution.ExternalObservations) != 1 {
		t.Fatalf("external observation = %#v, %v", execution, err)
	}
	execution, err = state.CompleteExternalExecution(taskID, "attempt", "caller", "complete-1", "external-v1", "succeeded", 2)
	if err != nil || execution.Lifecycle != "finished" || execution.Revision != 3 {
		t.Fatalf("external completion = %#v, %v", execution, err)
	}
	archive, err := state.ArchiveExternalExecution(taskID, "attempt", "caller", "archive-1", 3)
	if err != nil || archive.Execution.ArchiveState != "complete" || archive.Terminal != "" {
		t.Fatalf("external archive = %#v, %v", archive, err)
	}
}

func TestExternalObservationWritersUseRevisionsAndReceipts(t *testing.T) {
	state := openTest(t)
	taskID := validTaskID(t)
	if err := state.WriteManifest(taskID, Manifest{Title: "writers", Lifecycle: "created"}); err != nil {
		t.Fatal(err)
	}
	if _, err := state.AdoptExternalResource(taskID, ExternalResourceRequest{ID: "resource", OperationID: "adopt", CallerID: "caller", Repository: "repo", Branch: "branch", BaseRevision: "base", Head: "head", WorktreePath: filepath.Join(t.TempDir(), "gone")}); err != nil {
		t.Fatal(err)
	}
	if _, err := state.RecordExternalExecution(taskID, ExternalExecutionRequest{ID: "attempt", OperationID: "execution", CallerID: "caller", ResourceID: "resource"}); err != nil {
		t.Fatal(err)
	}
	observedAt := time.Now().UTC()
	observations := []ExternalObservation{{Source: "worker-a", ObservedAt: observedAt, HostID: "host", BootID: "boot", Detail: "a"}, {Source: "worker-b", ObservedAt: observedAt, HostID: "host", BootID: "boot", Detail: "b"}}
	errs := make(chan error, 2)
	for index, observation := range observations {
		go func(index int, observation ExternalObservation) {
			_, err := state.ObserveExternalExecution(taskID, "attempt", "caller", "observe-"+string(rune('a'+index)), 1, observation)
			errs <- err
		}(index, observation)
	}
	var successes int
	var conflicts int
	for range observations {
		switch err := <-errs; {
		case err == nil:
			successes++
		case IsKind(err, KindConflict):
			conflicts++
		default:
			t.Fatalf("concurrent observation error = %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent observation results = successes %d conflicts %d", successes, conflicts)
	}
	if _, err := state.ObserveExternalExecution(taskID, "attempt", "caller", "observe-a", 1, observations[0]); err != nil && !IsKind(err, KindConflict) {
		t.Fatalf("receipt retry error = %v", err)
	}
}

func TestExternalTaskCanFinishAndArchiveWithoutChildren(t *testing.T) {
	state := openTest(t)
	taskID := validTaskID(t)
	if err := state.WriteManifest(taskID, Manifest{Title: "empty external", Worker: "local", Lifecycle: "created", Condition: "none"}); err != nil {
		t.Fatal(err)
	}
	manifest, err := state.AdoptExternalTask(taskID, ExternalTaskRequest{CallerID: "caller", OperationID: "adopt-task"})
	if err != nil || manifest.Revision != 1 {
		t.Fatalf("task adoption = %#v, %v", manifest, err)
	}
	manifest, err = state.CompleteExternalTask(taskID, "caller", "complete-task", "contract", "done", 1)
	if err != nil || manifest.Lifecycle != "finished" || manifest.Revision != 2 {
		t.Fatalf("task completion = %#v, %v", manifest, err)
	}
	archive, err := state.ArchiveExternalTask(taskID, "caller", "archive-task", 2)
	if err != nil || archive.Manifest.ArchiveState != "complete" || len(archive.Resources) != 0 || len(archive.Executions) != 0 {
		t.Fatalf("task archive = %#v, %v", archive, err)
	}
}

func TestExternalLineageRejectsCyclesAndWrongParents(t *testing.T) {
	state := openTest(t)
	taskID := validTaskID(t)
	if err := state.WriteManifest(taskID, Manifest{Title: "lineage", Lifecycle: "created"}); err != nil {
		t.Fatal(err)
	}
	resource, err := state.AdoptExternalResource(taskID, ExternalResourceRequest{ID: "resource", OperationID: "adopt", CallerID: "caller", Repository: "repo", Branch: "branch", BaseRevision: "base", Head: "head", WorktreePath: filepath.Join(t.TempDir(), "gone")})
	if err != nil {
		t.Fatal(err)
	}
	_, err = state.RecordExternalExecution(taskID, ExternalExecutionRequest{ID: "a", OperationID: "a-op", CallerID: "caller", ResourceID: resource.ID})
	if err != nil {
		t.Fatal(err)
	}
	_, err = state.RecordExternalExecution(taskID, ExternalExecutionRequest{ID: "b", OperationID: "b-op", CallerID: "caller", ResourceID: resource.ID, PredecessorID: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.UpdateExecution(taskID, "a", func(execution *Execution) error { execution.PredecessorID = "b"; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := state.RecordExternalExecution(taskID, ExternalExecutionRequest{ID: "c", OperationID: "c-op", CallerID: "caller", ResourceID: resource.ID, PredecessorID: "a"}); !IsKind(err, KindConflict) {
		t.Fatalf("cyclic lineage error = %v, want conflict", err)
	}
}
