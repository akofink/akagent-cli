package lifecycle

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akofink/akagent-cli/internal/store"
)

func TestArchiveRecordChildrenRequireTerminalStateAndRecoverMissingSnapshot(t *testing.T) {
	manager, _ := newTestManager(t)
	const taskID = "archive-children"
	if err := manager.Store.WriteManifest(taskID, store.Manifest{Title: "archive children", Worker: "local", Lifecycle: "created", Condition: "none"}); err != nil {
		t.Fatal(err)
	}
	resource, created, err := manager.CreateResourceRecord(taskID, ResourceRequest{ID: "resource", Repository: "repo", Branch: "branch", WorktreePath: "/offline/work"})
	if err != nil || !created {
		t.Fatalf("create resource = %t, %v", created, err)
	}
	execution, created, err := manager.CreateExecution(taskID, ExecutionRequest{ID: "attempt", Target: "external"})
	if err != nil || !created {
		t.Fatalf("create execution = %t, %v", created, err)
	}
	if _, err := manager.ArchiveResource(taskID, resource.ID); err == nil {
		t.Fatal("active task allowed resource archive")
	}
	if _, err := manager.ArchiveRecord(taskID, "", execution.ID); err == nil {
		t.Fatal("active execution allowed archive")
	}
	if _, err := manager.Store.UpdateManifest(taskID, func(value *store.Manifest) error {
		value.Lifecycle = "finished"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ArchiveRecord(taskID, "", execution.ID); err == nil {
		t.Fatal("unfinished execution allowed archive")
	}
	if _, err := manager.Store.UpdateExecution(taskID, execution.ID, func(value *store.Execution) error { value.Lifecycle = "finished"; return nil }); err != nil {
		t.Fatal(err)
	}
	archivedResource, err := manager.ArchiveResource(taskID, resource.ID)
	if err != nil || archivedResource.ArchiveState != "complete" {
		t.Fatalf("archive resource = %#v, %v", archivedResource, err)
	}
	archivedExecution, err := manager.ArchiveRecord(taskID, "", execution.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := archivedExecution.(store.Execution); !ok || got.ArchiveState != "complete" {
		t.Fatalf("archive execution = %#v", archivedExecution)
	}
	for _, tc := range []struct {
		name, id string
		retry    func() (any, error)
	}{
		{"resource", resource.ID, func() (any, error) { return manager.ArchiveRecord(taskID, resource.ID, "") }},
		{"execution", execution.ID, func() (any, error) { return manager.ArchiveRecord(taskID, "", execution.ID) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(manager.Store.Root(), "tasks", taskID, tc.name+"s", tc.id, "archive.json")
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if _, err := tc.retry(); err != nil {
				t.Fatalf("retry missing snapshot: %v", err)
			}
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("missing restored archive: %v", err)
			}
			if _, err := tc.retry(); err != nil {
				t.Fatalf("idempotent retry: %v", err)
			}
		})
	}
}

func TestLegacyArchiveResourceRefusesCallerOwnedResource(t *testing.T) {
	manager, _ := newTestManager(t)
	const id = "external-archive-guard"
	if err := manager.Store.WriteManifest(id, store.Manifest{Title: "external", Lifecycle: "created"}); err != nil {
		t.Fatal(err)
	}
	request := store.ExternalResourceRequest{ID: "caller-owned", OperationID: "adopt", CallerID: "owner", Repository: "repo", Branch: "branch", BaseRevision: "base", Head: "head", WorktreePath: "/missing"}
	if _, err := manager.Store.AdoptExternalResource(id, request); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ArchiveResource(id, request.ID); err == nil {
		t.Fatal("legacy archive allowed caller-owned resource")
	}
	resource, err := manager.Store.ReadResource(id, request.ID)
	if err != nil || resource.ArchiveState == "complete" {
		t.Fatalf("guard changed record: %#v, %v", resource, err)
	}
}
