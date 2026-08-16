package lifecycle

import (
	"strings"
	"testing"

	"github.com/akofink/akagent-cli/internal/store"
)

func TestTaskOwnsMultipleIndependentResources(t *testing.T) {
	manager, _ := newTestManager(t)
	repository := registerWorktreeRepository(t, manager, "multi-resource")
	if result, err := manager.Create(CreateRequest{ID: "multi-task", Title: "Coordinated change"}); err != nil || !result.Created || result.Manifest.Repository != "" {
		t.Fatalf("Create() = %#v, %v; want task without a resource", result, err)
	}
	first, created, err := manager.CreateResource("multi-task", ResourceRequest{ID: "one", Repository: repository.Name, Branch: "akofink/one"})
	if err != nil || !created {
		t.Fatalf("CreateResource(one) = %#v, %v, want created", first, err)
	}
	second, created, err := manager.CreateResource("multi-task", ResourceRequest{ID: "two", Repository: repository.Name, Branch: "akofink/two"})
	if err != nil || !created {
		t.Fatalf("CreateResource(two) = %#v, %v, want created", second, err)
	}
	resources, err := manager.ListResources("multi-task")
	if err != nil || len(resources) != 2 {
		t.Fatalf("ListResources() = %#v, %v; want two resources", resources, err)
	}
	if resources[0].ID != "one" || resources[1].ID != "two" {
		t.Fatalf("resources = %#v, want deterministic IDs", resources)
	}
	if first.WorktreePath == second.WorktreePath || first.Branch == second.Branch {
		t.Fatalf("resources share immutable Git inputs: %#v %#v", first, second)
	}
	if _, err := manager.Stop("multi-task"); err != nil {
		t.Fatal(err)
	}
	manager.GitFacts = func(manifest store.Manifest) (store.GitFacts, error) {
		if strings.HasSuffix(manifest.WorktreePath, "one") {
			return store.GitFacts{Path: manifest.WorktreePath, Head: "one", Dirty: true}, nil
		}
		return store.GitFacts{Path: manifest.WorktreePath, Head: "two", Committed: true}, nil
	}
	if _, err := manager.ArchiveResource("multi-task", "one"); err != nil {
		t.Fatalf("ArchiveResource(one) = %v", err)
	}
	if _, err := manager.ArchiveResource("multi-task", "two"); err != nil {
		t.Fatalf("ArchiveResource(two) = %v", err)
	}
	one, err := manager.InspectResource("multi-task", "one")
	if err != nil {
		t.Fatal(err)
	}
	two, err := manager.InspectResource("multi-task", "two")
	if err != nil {
		t.Fatal(err)
	}
	if one.ArchiveState != archiveComplete || two.ArchiveState != archiveComplete || !one.Git.Dirty || !two.Git.Committed {
		t.Fatalf("resource archive state = %#v %#v", one, two)
	}
	if _, err := manager.CleanResource("multi-task", "one", CleanupOptions{AllowDirty: true, AllowWorktree: true}); err != nil {
		t.Fatalf("CleanResource(one) = %v", err)
	}
	two, err = manager.InspectResource("multi-task", "two")
	if err != nil {
		t.Fatal(err)
	}
	if two.CleanupState == cleanupComplete {
		t.Fatal("cleaning one resource changed the other resource")
	}
}

func TestLegacySingleResourceMigrationCreatesSeparateRecord(t *testing.T) {
	manager, _ := newTestManager(t)
	result, err := manager.Create(CreateRequest{ID: "legacy-resource", Title: "Legacy", Repository: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Manifest.ResourceIDs) != 0 {
		t.Fatalf("legacy create returned resource association %q, want compatibility result", result.Manifest.ResourceIDs)
	}
	resources, err := manager.ListResources("legacy-resource")
	if err != nil || len(resources) != 1 || resources[0].ID != "legacy" {
		t.Fatalf("migrated resources = %#v, %v", resources, err)
	}
	manifest, err := manager.Inspect("legacy-resource")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ResourceIDs != "legacy" {
		t.Fatalf("migrated task resource IDs = %q, want legacy", manifest.ResourceIDs)
	}
}
