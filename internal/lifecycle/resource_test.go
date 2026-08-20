package lifecycle

import (
	"errors"
	"path/filepath"
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
	wantFirstPath := filepath.Join(repository.WorktreeRoot, "one")
	if first.WorktreePath != wantFirstPath {
		t.Fatalf("first resource worktree path = %q, want %q", first.WorktreePath, wantFirstPath)
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
	if _, err := manager.CleanResource("multi-task", "one", CleanupOptions{AllowDirty: true, AllowWorktree: true, AllowCredentials: true}); err != nil {
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

func TestResourceCredentialCleanupIsIndependentAndRetryable(t *testing.T) {
	manager, _ := newTestManager(t)
	if _, err := manager.Create(CreateRequest{ID: "resource-credential", Title: "Resource credentials"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.CreateResource("resource-credential", ResourceRequest{ID: "resource", Repository: "demo"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Stop("resource-credential"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ArchiveResource("resource-credential", "resource"); err != nil {
		t.Fatal(err)
	}
	calls := 0
	manager.CleanupCredentials = func(store.Manifest) error {
		calls++
		return errors.New("credential cleanup unavailable")
	}
	if _, err := manager.CleanResource("resource-credential", "resource", CleanupOptions{}); !store.IsKind(err, store.KindPreservation) {
		t.Fatalf("unapproved resource credential cleanup = %v, want preservation error", err)
	}
	if calls != 0 {
		t.Fatal("resource credential hook ran without explicit approval")
	}
	resource, err := manager.InspectResource("resource-credential", "resource")
	if err != nil {
		t.Fatal(err)
	}
	if resource.CredentialCleanupState != cleanupBlocked || !resource.CleanupDebt {
		t.Fatalf("resource = %#v, want blocked credential cleanup debt", resource)
	}
	if _, err := manager.CleanResource("resource-credential", "resource", CleanupOptions{AllowCredentials: true}); err == nil || !store.IsKind(err, store.KindPartial) {
		t.Fatalf("failed resource credential cleanup = %v, want partial error", err)
	}
	manager.CleanupCredentials = func(store.Manifest) error { calls++; return nil }
	if _, err := manager.CleanResource("resource-credential", "resource", CleanupOptions{AllowCredentials: true}); err != nil {
		t.Fatal(err)
	}
	resource, err = manager.InspectResource("resource-credential", "resource")
	if err != nil {
		t.Fatal(err)
	}
	if resource.CredentialCleanupState != cleanupComplete || resource.CleanupDebt || calls != 2 {
		t.Fatalf("resource = %#v, calls = %d; want complete independent cleanup", resource, calls)
	}
}

func TestCleanResourceClearsResolvedWorktreeRecoveryDebt(t *testing.T) {
	manager, _ := newTestManager(t)
	repository := registerWorktreeRepository(t, manager, "resolved-worktree-debt")
	if _, err := manager.Create(CreateRequest{ID: "resolved-worktree-debt", Title: "Resolved worktree debt"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.CreateResource("resolved-worktree-debt", ResourceRequest{ID: "resource", Repository: repository.Name, Branch: "akofink/resolved-worktree-debt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Stop("resolved-worktree-debt"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ArchiveResource("resolved-worktree-debt", "resource"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Store.UpdateResource("resolved-worktree-debt", "resource", func(resource *store.Resource) error {
		resource.Git.Committed = false
		resource.Git.Dirty = true
		resource.Git.Untracked = true
		resource.RecoveryDebt = "uncommitted_work;launch_failed"
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	resource, err := manager.CleanResource("resolved-worktree-debt", "resource", CleanupOptions{AllowDirty: true, AllowUntracked: true, AllowWorktree: true, AllowCredentials: true})
	if err != nil {
		t.Fatal(err)
	}
	if resource.RecoveryDebt != "launch_failed" {
		t.Fatalf("cleaned resource recovery debt = %q, want unresolved debt preserved without uncommitted work", resource.RecoveryDebt)
	}
	resource, err = manager.InspectResource("resolved-worktree-debt", "resource")
	if err != nil {
		t.Fatal(err)
	}
	if resource.RecoveryDebt != "launch_failed" || resource.CleanupState != cleanupComplete || resource.WorktreeCleanupState != cleanupComplete {
		t.Fatalf("persisted resource = %#v, want complete cleanup and launch debt only", resource)
	}
}

func TestCleanResourcePreservesDebtWhenWorktreeCleanupFails(t *testing.T) {
	manager, _ := newTestManager(t)
	repository := registerWorktreeRepository(t, manager, "failed-worktree-debt")
	if _, err := manager.Create(CreateRequest{ID: "failed-worktree-debt", Title: "Failed worktree cleanup"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.CreateResource("failed-worktree-debt", ResourceRequest{ID: "resource", Repository: repository.Name, Branch: "akofink/failed-worktree-debt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Stop("failed-worktree-debt"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ArchiveResource("failed-worktree-debt", "resource"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Store.UpdateResource("failed-worktree-debt", "resource", func(resource *store.Resource) error {
		resource.Git.Committed = false
		resource.Git.Dirty = true
		resource.Git.Untracked = true
		resource.RecoveryDebt = "uncommitted_work;cleanup_failed"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	manager.CleanupWorktree = func(store.Manifest, store.GitFacts) error {
		return errors.New("worktree cleanup unavailable")
	}

	if _, err := manager.CleanResource("failed-worktree-debt", "resource", CleanupOptions{AllowDirty: true, AllowUntracked: true, AllowWorktree: true, AllowCredentials: true}); err == nil {
		t.Fatal("CleanResource() succeeded despite worktree cleanup failure")
	}
	resource, err := manager.InspectResource("failed-worktree-debt", "resource")
	if err != nil {
		t.Fatal(err)
	}
	if resource.RecoveryDebt != "uncommitted_work;cleanup_failed" {
		t.Fatalf("failed cleanup recovery debt = %q, want all unresolved debt preserved", resource.RecoveryDebt)
	}
}

func TestCleanResourceRetriesWorktreeWhenOtherCleanupStatesAreComplete(t *testing.T) {
	manager, _ := newTestManager(t)
	if _, err := manager.Create(CreateRequest{ID: "resource-worktree-retry", Title: "Retry resource cleanup"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.CreateResource("resource-worktree-retry", ResourceRequest{ID: "resource", Repository: "demo"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Stop("resource-worktree-retry"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ArchiveResource("resource-worktree-retry", "resource"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Store.UpdateResource("resource-worktree-retry", "resource", func(resource *store.Resource) error {
		resource.CleanupState = cleanupComplete
		resource.WorktreeCleanupState = cleanupPartial
		resource.CredentialCleanupState = cleanupComplete
		resource.CleanupDebt = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	called := false
	manager.CleanupWorktree = func(store.Manifest, store.GitFacts) error {
		called = true
		return nil
	}
	if _, err := manager.CleanResource("resource-worktree-retry", "resource", CleanupOptions{}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("CleanResource() skipped incomplete worktree cleanup")
	}
}

func TestResourceMetadataSurvivesUpdateArchiveAndReconcile(t *testing.T) {
	manager, _ := newTestManager(t)
	if _, err := manager.Create(CreateRequest{ID: "metadata-task", Title: "Delivery metadata"}); err != nil {
		t.Fatal(err)
	}
	resource, created, err := manager.CreateResource("metadata-task", ResourceRequest{ID: "delivery", Repository: "demo", Metadata: map[string]string{"review": "ready"}})
	if err != nil || !created {
		t.Fatalf("CreateResource() = %#v, %v, %v", resource, created, err)
	}
	resource, err = manager.UpdateResource("metadata-task", "delivery", ResourceUpdateRequest{Metadata: map[string]string{"review": "published"}, ExternalURLs: []string{"https://forge.example/pull/61"}})
	if err != nil {
		t.Fatal(err)
	}
	if resource.Metadata["review"] != "published" || len(resource.ExternalURLs) != 1 {
		t.Fatalf("updated resource = %#v, want metadata and external URL", resource)
	}
	if _, err := manager.Reconcile(); err != nil {
		t.Fatal(err)
	}
	resource, err = manager.InspectResource("metadata-task", "delivery")
	if err != nil || resource.Metadata["review"] != "published" || resource.ExternalURLs[0] != "https://forge.example/pull/61" {
		t.Fatalf("reconciled resource = %#v, %v", resource, err)
	}
	if _, err := manager.Stop("metadata-task"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Archive("metadata-task"); err != nil {
		t.Fatal(err)
	}
	archive, err := manager.Store.ReadArchive("metadata-task")
	if err != nil {
		t.Fatal(err)
	}
	if len(archive.Resources) != 1 || archive.Resources[0].Metadata["review"] != "published" || archive.Resources[0].ExternalURLs[0] != "https://forge.example/pull/61" {
		t.Fatalf("task archive resources = %#v, want preserved metadata", archive.Resources)
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
