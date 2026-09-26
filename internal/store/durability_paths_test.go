package store

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func externalDurabilityFixture(t *testing.T) (*Store, string, ExternalExecutionRequest) {
	t.Helper()
	s := openTest(t)
	id := validTaskID(t)
	if err := s.WriteManifest(id, Manifest{Title: "durability", Lifecycle: "created"}); err != nil {
		t.Fatal(err)
	}
	_, err := s.AdoptExternalResource(id, ExternalResourceRequest{ID: "resource", OperationID: "adopt", CallerID: "owner", Repository: "repo", Branch: "branch", BaseRevision: "base", Head: "head", WorktreePath: "/offline/work"})
	if err != nil {
		t.Fatal(err)
	}
	return s, id, ExternalExecutionRequest{ID: "execution", OperationID: "record", CallerID: "owner", ResourceID: "resource"}
}

func TestExternalExecutionCreationAndProjectionRecovery(t *testing.T) {
	s, id, request := externalDurabilityFixture(t)
	if _, err := s.RecordExternalExecution(id, ExternalExecutionRequest{ID: "missing", OperationID: "missing", CallerID: "owner", ResourceID: "absent"}); !IsKind(err, KindNotFound) {
		t.Fatalf("missing resource = %v", err)
	}
	if _, err := s.RecordExternalExecution(id, ExternalExecutionRequest{ID: "wrong", OperationID: "wrong", CallerID: "stranger", ResourceID: "resource"}); !IsKind(err, KindConflict) {
		t.Fatalf("wrong caller = %v", err)
	}
	created, err := s.RecordExternalExecution(id, request)
	if err != nil || created.Revision != 1 {
		t.Fatalf("create = %#v, %v", created, err)
	}
	manifestEnvelope, err := s.ReadManifest(id)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := manifestEnvelope.DecodeManifest()
	if err != nil {
		t.Fatal(err)
	}
	manifest.ExecutionIDs = ""
	manifest.Receipts = ""
	if err := s.WriteManifest(id, manifest); err != nil {
		t.Fatal(err)
	}
	repaired, err := s.RecordExternalExecution(id, request)
	if err != nil || repaired.Revision != 1 {
		t.Fatalf("projection retry = %#v, %v", repaired, err)
	}
	manifestEnvelope, err = s.ReadManifest(id)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err = manifestEnvelope.DecodeManifest()
	if err != nil {
		t.Fatal(err)
	}
	if !containsCSV(manifest.ExecutionIDs, request.ID) || manifest.Receipts == "" {
		t.Fatalf("missing repaired child projection: %#v", manifest)
	}
	again, err := s.RecordExternalExecution(id, request)
	if err != nil || again.Revision != repaired.Revision {
		t.Fatalf("idempotent retry = %#v, %v", again, err)
	}
	conflicting := request
	conflicting.OperationID = "other"
	conflicting.ResourceID = "resource"
	conflicting.PredecessorID = "missing"
	if _, err := s.RecordExternalExecution(id, conflicting); !IsKind(err, KindNotFound) {
		t.Fatalf("invalid predecessor = %v", err)
	}
}

func TestExternalArchiveRetriesRestoreMissingSnapshot(t *testing.T) {
	s, id, request := externalDurabilityFixture(t)
	execution, err := s.RecordExternalExecution(id, request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ArchiveExternalExecution(id, execution.ID, "owner", "archive-execution", execution.Revision); !IsKind(err, KindConflict) {
		t.Fatalf("uncompleted execution archive = %v", err)
	}
	execution, err = s.CompleteExternalExecution(id, execution.ID, "owner", "complete", "contract", "succeeded", execution.Revision)
	if err != nil {
		t.Fatal(err)
	}
	resource, err := s.ReadResource(id, "resource")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ArchiveExternalResource(id, resource.ID, "stranger", "archive-resource", resource.Revision); !IsKind(err, KindConflict) {
		t.Fatalf("foreign resource archive = %v", err)
	}
	if _, err := s.ArchiveExternalResource(id, resource.ID, "owner", "archive-resource", resource.Revision+1); !IsKind(err, KindConflict) {
		t.Fatalf("stale resource archive = %v", err)
	}
	resourceArchive, err := s.ArchiveExternalResource(id, resource.ID, "owner", "archive-resource", resource.Revision)
	if err != nil || resourceArchive.Resource.ArchiveState != "complete" {
		t.Fatalf("resource archive = %#v, %v", resourceArchive, err)
	}
	executionArchive, err := s.ArchiveExternalExecution(id, execution.ID, "owner", "archive-execution", execution.Revision)
	if err != nil || executionArchive.Execution.ArchiveState != "complete" {
		t.Fatalf("execution archive = %#v, %v", executionArchive, err)
	}
	for _, tc := range []struct {
		name, path string
		retry      func() (uint64, error)
	}{
		{"resource", s.resourceArchivePath(id, resource.ID), func() (uint64, error) {
			a, e := s.ArchiveExternalResource(id, resource.ID, "owner", "archive-resource", resource.Revision)
			return a.Resource.Revision, e
		}},
		{"execution", s.executionArchivePath(id, execution.ID), func() (uint64, error) {
			a, e := s.ArchiveExternalExecution(id, execution.ID, "owner", "archive-execution", execution.Revision)
			return a.Execution.Revision, e
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.Remove(tc.path); err != nil {
				t.Fatal(err)
			}
			revision, err := tc.retry()
			if err != nil || revision == 0 {
				t.Fatalf("retry failed to restore snapshot: revision %d, %v", revision, err)
			}
			if _, err := os.Stat(tc.path); err != nil {
				t.Fatalf("archive not restored: %v", err)
			}
		})
	}
	if _, err := s.ArchiveExternalResource(id, resource.ID, "owner", "different-operation", 0); err != nil {
		t.Fatalf("already archived resource = %v", err)
	}
	if _, err := s.ArchiveExternalExecution(id, execution.ID, "owner", "different-operation", 0); err != nil {
		t.Fatalf("already archived execution = %v", err)
	}
}

func TestReadResourceArchiveRejectsCorruptionWithoutChangingFile(t *testing.T) {
	s, id, _ := externalDurabilityFixture(t)
	resource, err := s.ReadResource(id, "resource")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadResourceArchive(id, "resource"); !IsKind(err, KindNotFound) {
		t.Fatalf("missing archive = %v", err)
	}
	archive := ResourceArchive{TaskID: id, ResourceID: "resource", CapturedAt: time.Now().UTC(), Resource: resource}
	if err := s.WriteResourceArchive(id, "resource", archive); err != nil {
		t.Fatal(err)
	}
	if got, err := s.ReadResourceArchive(id, "resource"); err != nil || got.Resource.ID != resource.ID {
		t.Fatalf("valid archive = %#v, %v", got, err)
	}
	path := s.resourceArchivePath(id, "resource")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{truncated"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadResourceArchive(id, "resource"); !IsKind(err, KindMalformed) {
		t.Fatalf("corrupted archive = %v", err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "{truncated" {
		t.Fatalf("archive changed by read: %q, %v", got, err)
	}
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadResourceArchive(id, "resource"); err != nil {
		t.Fatalf("restored archive = %v", err)
	}
}

func TestRecoverValidatesArchiveAndCheckpointSnapshots(t *testing.T) {
	s := openTest(t)
	id := validTaskID(t)
	createCheckpointTask(t, s, id)
	checkpoint, err := s.WriteCheckpoint(id, checkpointFixture(id))
	if err != nil {
		t.Fatal(err)
	}
	archive := TaskArchive{TaskID: id, CapturedAt: time.Now().UTC(), Checkpoint: &checkpoint}
	if err := s.WriteArchive(id, archive); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, path string
		malformed  []byte
	}{
		{"archive", s.archivePath(id), []byte("{broken")},
		{"checkpoint", s.checkpointPath(id), []byte("{broken")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original, err := os.ReadFile(tc.path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := os.WriteFile(tc.path, original, 0o600); err != nil {
					t.Error(err)
				}
			})
			if err := os.WriteFile(tc.path, tc.malformed, 0o600); err != nil {
				t.Fatal(err)
			}
			result, err := s.Recover()
			if err != nil {
				t.Fatal(err)
			}
			if !contains(result.MalformedRecords, tc.path) {
				t.Fatalf("missing malformed %s: %#v", tc.name, result)
			}
			got, err := os.ReadFile(tc.path)
			if err != nil || !reflect.DeepEqual(got, tc.malformed) {
				t.Fatalf("recovery modified record: %q, %v", got, err)
			}
		})
	}
	// A well-formed envelope can still carry a semantically invalid snapshot.
	archive.Resources = []Resource{{ID: "wrong", TaskID: "another-task"}}
	if err := s.WriteArchive(id, archive); err != nil {
		t.Fatal(err)
	}
	result, err := s.Recover()
	if err != nil || !contains(result.MalformedRecords, s.archivePath(id)) {
		t.Fatalf("invalid archived resource = %#v, %v", result, err)
	}
}

func TestRecoverKeepsUnsafeArchiveAndCheckpointPaths(t *testing.T) {
	s := openTest(t)
	id := validTaskID(t)
	createCheckpointTask(t, s, id)
	outside := filepath.Join(t.TempDir(), "sentinel")
	if err := os.WriteFile(outside, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{s.archivePath(id), s.checkpointPath(id)} {
		if err := os.Symlink(outside, path); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		result, err := s.Recover()
		if err != nil || len(result.MalformedRecords) == 0 {
			t.Fatalf("unsafe path %s = %#v, %v", path, result, err)
		}
		if got, err := os.ReadFile(outside); err != nil || string(got) != "unchanged" {
			t.Fatalf("outside record changed: %q, %v", got, err)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	result, err := s.Recover()
	if err != nil || len(result.MalformedRecords) != 0 || len(result.StaleFilesRemoved) != 0 {
		t.Fatalf("missing optional snapshots = %#v, %v", result, err)
	}
}

func TestRecoverTaskLockedSkipsMissingAndReportsUnsafeTask(t *testing.T) {
	s := openTest(t)
	id := validTaskID(t)
	result := RecoveryResult{}
	if err := s.recoverTaskLocked(id, &result); err != nil || len(result.MalformedRecords) != 0 {
		t.Fatalf("missing task recovery = %#v, %v", result, err)
	}
	createCheckpointTask(t, s, id)
	if err := os.Chmod(s.taskDir(id), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(s.taskDir(id), 0o700) })
	if err := s.recoverTaskLocked(id, &result); err != nil || len(result.MalformedRecords) != 1 {
		t.Fatalf("unsafe task recovery = %#v, %v", result, err)
	}
}

func TestExternalExecutionRetryRepairsMissingTaskEvent(t *testing.T) {
	s, id, request := externalDurabilityFixture(t)
	execution, err := s.RecordExternalExecution(id, request)
	if err != nil {
		t.Fatal(err)
	}
	events, err := s.ReadEvents(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) < 2 {
		t.Fatalf("task events = %#v", events)
	}
	last := s.eventPath(id, len(events))
	if err := os.Remove(last); err != nil {
		t.Fatal(err)
	}
	retried, err := s.RecordExternalExecution(id, request)
	if err != nil || retried.Revision != execution.Revision {
		t.Fatalf("retry = %#v, %v", retried, err)
	}
	repaired, err := s.ReadEvents(id)
	if err != nil || len(repaired) != len(events) || repaired[len(repaired)-1].Event.Operation != "record_repair" {
		t.Fatalf("repaired task audit = %#v, %v", repaired, err)
	}
}
