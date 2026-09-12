package lifecycle

import (
	"path/filepath"
	"testing"

	"github.com/akofink/akagent-cli/internal/store"
)

func TestExternalTaskRecoveryDoesNotTreatMissingProcessesAsCompletion(t *testing.T) {
	manager, tmux := newTestManager(t)
	if _, err := manager.Create(CreateRequest{ID: "external-task", Title: "External task"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.RecordTask("external-task", store.ExternalTaskRequest{CallerID: "caller", OperationID: "adopt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ReconcileTask("external-task"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Finish("external-task", "succeeded", "done"); !store.IsKind(err, store.KindConflict) {
		t.Fatalf("legacy finish error = %v, want conflict", err)
	}
	if len(tmux.observedIDs) != 0 {
		t.Fatalf("external task recovery inspected tmux: %v", tmux.observedIDs)
	}
}

func TestLegacyResourceCleanupRejectsExternalProvenanceWithoutHostObservation(t *testing.T) {
	manager, tmux := newTestManager(t)
	if _, err := manager.Create(CreateRequest{ID: "external-resource-task", Title: "External resource"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.RecordResource("external-resource-task", store.ExternalResourceRequest{
		ID: "external-resource", OperationID: "adopt", CallerID: "caller", Repository: "offline", Branch: "external/branch", BaseRevision: "base", Head: "head", WorktreePath: filepath.Join(t.TempDir(), "missing"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.CleanResource("external-resource-task", "external-resource", CleanupOptions{}); !store.IsKind(err, store.KindConflict) {
		t.Fatalf("external resource cleanup error = %v, want conflict", err)
	}
	if _, err := manager.ArchiveResource("external-resource-task", "external-resource"); !store.IsKind(err, store.KindConflict) {
		t.Fatalf("external resource archive error = %v, want conflict", err)
	}
	if len(tmux.observedIDs) != 0 {
		t.Fatalf("legacy resource paths inspected tmux: %v", tmux.observedIDs)
	}
}
