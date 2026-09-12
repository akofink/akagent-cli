package store

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExternalRecordInputBounds(t *testing.T) {
	state := openTest(t)
	taskID := validTaskID(t)
	if err := state.WriteManifest(taskID, Manifest{Title: "bounds", Lifecycle: "created"}); err != nil {
		t.Fatal(err)
	}
	if _, err := state.AdoptExternalResource(taskID, ExternalResourceRequest{ID: "long-metadata", OperationID: "op", CallerID: "caller", Repository: "repo", Branch: "branch", BaseRevision: "base", Head: "head", WorktreePath: "/work", Metadata: map[string]string{"key": strings.Repeat("x", 4097)}}); !IsKind(err, KindUsage) {
		t.Fatalf("oversized metadata error = %v, want usage", err)
	}
	metadata := make(map[string]string, 65)
	for index := 0; index < 65; index++ {
		metadata["key-"+string(rune('a'+index%26))+string(rune('0'+index/26))] = "value"
	}
	if _, err := state.AdoptExternalResource(taskID, ExternalResourceRequest{ID: "many-metadata", OperationID: "op-many", CallerID: "caller", Repository: "repo", Branch: "branch", BaseRevision: "base", Head: "head", WorktreePath: "/work", Metadata: metadata}); !IsKind(err, KindUsage) {
		t.Fatalf("too many metadata entries error = %v, want usage", err)
	}
	references := make([]SessionReference, 33)
	for index := range references {
		references[index] = SessionReference{Tool: "tool", SessionID: "session-" + string(rune('a'+index%26)) + string(rune('0'+index/26))}
	}
	if _, err := state.RecordExternalExecution(taskID, ExternalExecutionRequest{ID: "too-many-sessions", OperationID: "execution", CallerID: "caller", ResourceID: "missing", SessionReferences: references}); !IsKind(err, KindUsage) {
		t.Fatalf("too many session references error = %v, want usage", err)
	}
}

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

func TestExternalCallerOwnershipIsStableAcrossParentsAndRetries(t *testing.T) {
	state := openTest(t)
	taskID := validTaskID(t)
	if err := state.WriteManifest(taskID, Manifest{Title: "ownership", Lifecycle: "created"}); err != nil {
		t.Fatal(err)
	}
	manifest, err := state.AdoptExternalTask(taskID, ExternalTaskRequest{CallerID: "owner-a", OperationID: "adopt-a"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.AdoptExternalTask(taskID, ExternalTaskRequest{CallerID: "owner-b", OperationID: "adopt-b"}); !IsKind(err, KindConflict) {
		t.Fatalf("cross-caller task adoption error = %v, want conflict", err)
	}
	current, err := state.ReadManifest(taskID)
	if err != nil {
		t.Fatal(err)
	}
	got, err := current.DecodeManifest()
	if err != nil {
		t.Fatal(err)
	}
	if got.CallerID != "owner-a" || got.Revision != manifest.Revision {
		t.Fatalf("task owner after conflicting adoption = %#v", got)
	}
	repeated, err := state.AdoptExternalTask(taskID, ExternalTaskRequest{CallerID: "owner-a", OperationID: "adopt-a-second"})
	if err != nil || repeated.Revision != manifest.Revision {
		t.Fatalf("repeat task adoption = %#v, %v; want unchanged revision", repeated, err)
	}
	if _, err := state.CompleteExternalTask(taskID, "owner-b", "complete-b", "contract", "done", got.Revision); !IsKind(err, KindConflict) {
		t.Fatalf("cross-caller task completion error = %v, want conflict", err)
	}
	resource, err := state.AdoptExternalResource(taskID, ExternalResourceRequest{ID: "resource", OperationID: "resource-a", CallerID: "owner-a", Repository: "repo", Branch: "branch", BaseRevision: "base", Head: "head", WorktreePath: filepath.Join(t.TempDir(), "gone")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.RecordExternalExecution(taskID, ExternalExecutionRequest{ID: "execution", OperationID: "execution-b", CallerID: "owner-b", ResourceID: resource.ID}); !IsKind(err, KindConflict) {
		t.Fatalf("cross-caller execution error = %v, want conflict", err)
	}
}

func TestExternalObservationReceiptCannotBypassCallerOwnership(t *testing.T) {
	state := openTest(t)
	taskID := validTaskID(t)
	if err := state.WriteManifest(taskID, Manifest{Title: "observation owner", Lifecycle: "created"}); err != nil {
		t.Fatal(err)
	}
	resource, err := state.AdoptExternalResource(taskID, ExternalResourceRequest{ID: "resource", OperationID: "resource", CallerID: "owner-a", Repository: "repo", Branch: "branch", BaseRevision: "base", Head: "head", WorktreePath: filepath.Join(t.TempDir(), "gone")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.RecordExternalExecution(taskID, ExternalExecutionRequest{ID: "execution", OperationID: "execution", CallerID: "owner-a", ResourceID: resource.ID}); err != nil {
		t.Fatal(err)
	}
	observation := ExternalObservation{Source: "worker", ObservedAt: time.Now().UTC(), HostID: "host", BootID: "boot", Detail: "historical"}
	if _, err := state.ObserveExternalExecution(taskID, "execution", "owner-a", "observation", 1, observation); err != nil {
		t.Fatal(err)
	}
	if _, err := state.ObserveExternalExecution(taskID, "execution", "owner-b", "observation", 1, observation); !IsKind(err, KindConflict) {
		t.Fatalf("cross-caller observation replay error = %v, want conflict", err)
	}
}

func TestExternalTerminalRecordsRejectMutationAndChildren(t *testing.T) {
	state := openTest(t)
	taskID := validTaskID(t)
	if err := state.WriteManifest(taskID, Manifest{Title: "terminal", Lifecycle: "created"}); err != nil {
		t.Fatal(err)
	}
	manifest, err := state.AdoptExternalTask(taskID, ExternalTaskRequest{CallerID: "owner", OperationID: "adopt"})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err = state.CompleteExternalTask(taskID, "owner", "complete", "contract-a", "result-a", manifest.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.CompleteExternalTask(taskID, "owner", "complete-again", "contract-b", "result-b", manifest.Revision); !IsKind(err, KindConflict) {
		t.Fatalf("terminal task completion error = %v, want conflict", err)
	}
	if _, err := state.AdoptExternalResource(taskID, ExternalResourceRequest{ID: "late-resource", OperationID: "late", CallerID: "owner", Repository: "repo", Branch: "branch", BaseRevision: "base", Head: "head", WorktreePath: filepath.Join(t.TempDir(), "gone")}); !IsKind(err, KindConflict) {
		t.Fatalf("terminal task child adoption error = %v, want conflict", err)
	}

	childTaskID := validTaskID(t)
	if err := state.WriteManifest(childTaskID, Manifest{Title: "terminal execution", Lifecycle: "created"}); err != nil {
		t.Fatal(err)
	}
	resource, err := state.AdoptExternalResource(childTaskID, ExternalResourceRequest{ID: "resource", OperationID: "resource", CallerID: "owner", Repository: "repo", Branch: "branch", BaseRevision: "base", Head: "head", WorktreePath: filepath.Join(t.TempDir(), "gone")})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := state.RecordExternalExecution(childTaskID, ExternalExecutionRequest{ID: "execution", OperationID: "execution", CallerID: "owner", ResourceID: resource.ID})
	if err != nil {
		t.Fatal(err)
	}
	execution, err = state.CompleteExternalExecution(childTaskID, execution.ID, "owner", "complete", "contract-a", "result-a", execution.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.CompleteExternalExecution(childTaskID, execution.ID, "owner", "complete-again", "contract-b", "result-b", execution.Revision); !IsKind(err, KindConflict) {
		t.Fatalf("terminal execution completion error = %v, want conflict", err)
	}
	if _, err := state.ObserveExternalExecution(childTaskID, execution.ID, "owner", "late-observation", execution.Revision, ExternalObservation{Source: "worker", ObservedAt: time.Now().UTC(), HostID: "host", BootID: "boot"}); !IsKind(err, KindConflict) {
		t.Fatalf("terminal execution observation error = %v, want conflict", err)
	}
}

func TestExternalRebindingReplacesCompleteCurrentBinding(t *testing.T) {
	state := openTest(t)
	taskID := validTaskID(t)
	if err := state.WriteManifest(taskID, Manifest{Title: "rebinding", Lifecycle: "created"}); err != nil {
		t.Fatal(err)
	}
	first := ExternalResourceRequest{ID: "resource", OperationID: "first", CallerID: "owner", Repository: "repo-a", Branch: "branch-a", BaseRevision: "base-a", Head: "head-a", WorktreePath: "/work/a"}
	resource, err := state.AdoptExternalResource(taskID, first)
	if err != nil {
		t.Fatal(err)
	}
	second := first
	second.OperationID = "second"
	second.ExpectedRevision = resource.Revision
	second.Repository, second.Branch, second.BaseRevision, second.Head, second.WorktreePath = "repo-b", "branch-b", "base-b", "head-b", "/work/b"
	resource, err = state.AdoptExternalResource(taskID, second)
	if err != nil {
		t.Fatal(err)
	}
	if resource.Repository != second.Repository || resource.Branch != second.Branch || resource.BaseRevision != second.BaseRevision || resource.WorktreePath != second.WorktreePath || resource.Git.Head != second.Head || resource.Git.Branch != second.Branch || resource.Git.Path != second.WorktreePath {
		t.Fatalf("current binding after rebind = %#v", resource)
	}
	if len(resource.ObservationHistory) != 1 || resource.ObservationHistory[0].Repository != first.Repository || resource.ObservationHistory[0].WorktreePath != first.WorktreePath {
		t.Fatalf("binding history after rebind = %#v", resource.ObservationHistory)
	}
}

func TestExternalRecordCreationRepairsAllProjectionsAfterEventWriteFailure(t *testing.T) {
	state := openTest(t)
	taskID := validTaskID(t)
	if err := state.WriteManifest(taskID, Manifest{Title: "creation repair", Lifecycle: "created"}); err != nil {
		t.Fatal(err)
	}
	failResource := true
	state.writeHook = func(path string) error {
		if failResource && strings.Contains(path, filepath.Join("resources", "resource", "events")) {
			failResource = false
			return newError(KindPartial, "injected resource event failure", "Retry the same operation")
		}
		return nil
	}
	resourceRequest := ExternalResourceRequest{ID: "resource", OperationID: "resource", CallerID: "owner", Repository: "repo", Branch: "branch", BaseRevision: "base", Head: "head", WorktreePath: "/work"}
	if _, err := state.AdoptExternalResource(taskID, resourceRequest); err == nil {
		t.Fatal("resource adoption unexpectedly succeeded during injected event failure")
	}
	state.writeHook = nil
	if _, err := state.AdoptExternalResource(taskID, resourceRequest); err != nil {
		t.Fatalf("resource adoption retry error = %v", err)
	}
	manifestEnvelope, err := state.ReadManifest(taskID)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := manifestEnvelope.DecodeManifest()
	if err != nil {
		t.Fatal(err)
	}
	if !containsCSV(manifest.ResourceIDs, "resource") {
		t.Fatalf("repaired resource task projection = %#v", manifest)
	}

	failExecution := true
	state.writeHook = func(path string) error {
		if failExecution && strings.Contains(path, filepath.Join("executions", "execution", "events")) {
			failExecution = false
			return newError(KindPartial, "injected execution event failure", "Retry the same operation")
		}
		return nil
	}
	executionRequest := ExternalExecutionRequest{ID: "execution", OperationID: "execution", CallerID: "owner", ResourceID: "resource"}
	if _, err := state.RecordExternalExecution(taskID, executionRequest); err == nil {
		t.Fatal("execution recording unexpectedly succeeded during injected event failure")
	}
	state.writeHook = nil
	if _, err := state.RecordExternalExecution(taskID, executionRequest); err != nil {
		t.Fatalf("execution recording retry error = %v", err)
	}
	manifestEnvelope, err = state.ReadManifest(taskID)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err = manifestEnvelope.DecodeManifest()
	if err != nil {
		t.Fatal(err)
	}
	if !containsCSV(manifest.ExecutionIDs, "execution") {
		t.Fatalf("repaired execution task projection = %#v", manifest)
	}
}

func TestExternalRebindingRepairsReceiptAfterEventWriteFailure(t *testing.T) {
	state := openTest(t)
	taskID := validTaskID(t)
	if err := state.WriteManifest(taskID, Manifest{Title: "repair", Lifecycle: "created"}); err != nil {
		t.Fatal(err)
	}
	first := ExternalResourceRequest{ID: "resource", OperationID: "first", CallerID: "owner", Repository: "repo", Branch: "branch", BaseRevision: "base", Head: "head-a", WorktreePath: "/work/a"}
	if _, err := state.AdoptExternalResource(taskID, first); err != nil {
		t.Fatal(err)
	}
	fail := true
	state.writeHook = func(path string) error {
		if fail && strings.Contains(path, filepath.Join("resources", "resource", "events")) {
			fail = false
			return newError(KindPartial, "injected event failure", "Retry the same operation")
		}
		return nil
	}
	update := first
	update.OperationID = "rebind"
	update.ExpectedRevision = 1
	update.Head = "head-b"
	if _, err := state.AdoptExternalResource(taskID, update); err == nil {
		t.Fatal("rebinding unexpectedly succeeded during injected event failure")
	}
	state.writeHook = nil
	blocked := update
	blocked.OperationID = "blocked-new-operation"
	blocked.ExpectedRevision = 2
	blocked.Head = "head-c"
	if _, err := state.AdoptExternalResource(taskID, blocked); !IsKind(err, KindPartial) {
		t.Fatalf("new operation after unresolved event error = %v, want partial", err)
	}
	repaired, err := state.AdoptExternalResource(taskID, update)
	if err != nil {
		t.Fatalf("rebinding retry error = %v", err)
	}
	if repaired.Revision != 2 {
		t.Fatalf("repaired resource revision = %d, want 2", repaired.Revision)
	}
	events, err := state.ReadResourceEvents(taskID, "resource")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("repaired resource events = %d, want 2", len(events))
	}
}

func TestExternalTaskArchiveRepairsReceiptAfterEventWriteFailure(t *testing.T) {
	state := openTest(t)
	taskID := validTaskID(t)
	if err := state.WriteManifest(taskID, Manifest{Title: "archive repair", Lifecycle: "created"}); err != nil {
		t.Fatal(err)
	}
	manifest, err := state.AdoptExternalTask(taskID, ExternalTaskRequest{CallerID: "owner", OperationID: "adopt"})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err = state.CompleteExternalTask(taskID, "owner", "complete", "contract", "done", manifest.Revision)
	if err != nil {
		t.Fatal(err)
	}
	fail := true
	state.writeHook = func(path string) error {
		if fail && strings.HasSuffix(path, filepath.Join("events", "000003.json")) {
			fail = false
			return newError(KindPartial, "injected task archive event failure", "Retry the same operation")
		}
		return nil
	}
	if _, err := state.ArchiveExternalTask(taskID, "owner", "archive", manifest.Revision); err == nil {
		t.Fatal("task archive unexpectedly succeeded during injected event failure")
	}
	state.writeHook = nil
	archive, err := state.ArchiveExternalTask(taskID, "owner", "archive", manifest.Revision)
	if err != nil {
		t.Fatalf("task archive retry error = %v", err)
	}
	if archive.Manifest.ArchiveState != "complete" {
		t.Fatalf("repaired task archive manifest = %#v", archive.Manifest)
	}
	if _, err := state.ReadArchive(taskID); err != nil {
		t.Fatalf("repaired task archive missing = %v", err)
	}
}

func TestExternalObservationRetrySurvivesReceiptHistoryBound(t *testing.T) {
	state := openTest(t)
	taskID := validTaskID(t)
	if err := state.WriteManifest(taskID, Manifest{Title: "receipt history", Lifecycle: "created"}); err != nil {
		t.Fatal(err)
	}
	resource, err := state.AdoptExternalResource(taskID, ExternalResourceRequest{ID: "resource", OperationID: "resource", CallerID: "owner", Repository: "repo", Branch: "branch", BaseRevision: "base", Head: "head", WorktreePath: "/work"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.RecordExternalExecution(taskID, ExternalExecutionRequest{ID: "execution", OperationID: "execution", CallerID: "owner", ResourceID: resource.ID}); err != nil {
		t.Fatal(err)
	}
	firstObservation := ExternalObservation{Source: "worker", ObservedAt: time.Now().UTC(), HostID: "host", BootID: "boot", Detail: "first"}
	if _, err := state.ObserveExternalExecution(taskID, "execution", "owner", "first-observation", 1, firstObservation); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 40; index++ {
		_, err := state.ObserveExternalExecution(taskID, "execution", "owner", "later-observation-"+string(rune('a'+index%26))+string(rune('0'+index/26)), uint64(index+2), ExternalObservation{Source: "worker", ObservedAt: time.Now().UTC(), HostID: "host", BootID: "boot", Detail: "later"})
		if err != nil {
			t.Fatalf("later observation %d error = %v", index, err)
		}
	}
	execution, err := state.ObserveExternalExecution(taskID, "execution", "owner", "first-observation", 42, firstObservation)
	if err != nil {
		t.Fatalf("delayed identical observation retry error = %v", err)
	}
	if execution.Revision != 42 || len(execution.ExternalObservations) != 41 {
		t.Fatalf("delayed retry changed execution = revision %d observations %d", execution.Revision, len(execution.ExternalObservations))
	}
	changed := firstObservation
	changed.Detail = "changed"
	if _, err := state.ObserveExternalExecution(taskID, "execution", "owner", "first-observation", 42, changed); !IsKind(err, KindConflict) {
		t.Fatalf("reused receipt with changed observation error = %v, want conflict", err)
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
	other, err := state.AdoptExternalResource(taskID, ExternalResourceRequest{ID: "other-resource", OperationID: "other-resource", CallerID: "caller", Repository: "repo", Branch: "other-branch", BaseRevision: "base", Head: "other-head", WorktreePath: filepath.Join(t.TempDir(), "other-gone")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.RecordExternalExecution(taskID, ExternalExecutionRequest{ID: "wrong-parent", OperationID: "wrong-parent", CallerID: "caller", ResourceID: other.ID, PredecessorID: "a"}); !IsKind(err, KindConflict) {
		t.Fatalf("cross-resource predecessor error = %v, want conflict", err)
	}
	if _, err := state.UpdateExecution(taskID, "a", func(execution *Execution) error { execution.PredecessorID = "b"; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := state.RecordExternalExecution(taskID, ExternalExecutionRequest{ID: "c", OperationID: "c-op", CallerID: "caller", ResourceID: resource.ID, PredecessorID: "a"}); !IsKind(err, KindConflict) {
		t.Fatalf("cyclic lineage error = %v, want conflict", err)
	}
}
