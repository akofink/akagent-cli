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

func TestLegacyExternalTaskEntryPointsRejectBeforeHostSideEffects(t *testing.T) {
	manager, tmux := newTestManager(t)
	if _, err := manager.Create(CreateRequest{ID: "external-entrypoints", Title: "External entrypoints"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.RecordTask("external-entrypoints", store.ExternalTaskRequest{CallerID: "caller", OperationID: "adopt"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.CreateResource("external-entrypoints", ResourceRequest{ID: "managed-resource", Repository: "demo"}); !store.IsKind(err, store.KindConflict) {
		t.Fatalf("external task resource creation error = %v, want conflict", err)
	}
	if _, _, err := manager.CreateExecution("external-entrypoints", ExecutionRequest{ID: "managed-execution", Target: "shell", Command: "/bin/sh"}); !store.IsKind(err, store.KindConflict) {
		t.Fatalf("external task execution creation error = %v, want conflict", err)
	}
	if _, err := manager.LaunchExecution("external-entrypoints", LaunchRequest{Target: "shell"}); !store.IsKind(err, store.KindConflict) {
		t.Fatalf("external task launch error = %v, want conflict", err)
	}
	if _, err := manager.Start(StartRequest{ID: "external-entrypoints", Title: "External entrypoints", Repository: "demo"}); !store.IsKind(err, store.KindConflict) {
		t.Fatalf("external task start error = %v, want conflict", err)
	}
	if err := manager.Launch("external-entrypoints"); !store.IsKind(err, store.KindConflict) {
		t.Fatalf("external task worker error = %v, want conflict", err)
	}
	if err := manager.Attach("external-entrypoints"); !store.IsKind(err, store.KindConflict) {
		t.Fatalf("external task attach error = %v, want conflict", err)
	}
	if _, err := manager.RecordResource("external-entrypoints", store.ExternalResourceRequest{ID: "external-resource", OperationID: "resource", CallerID: "caller", Repository: "offline", Branch: "branch", BaseRevision: "base", Head: "head", WorktreePath: filepath.Join(t.TempDir(), "missing")}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.RecordExecution("external-entrypoints", store.ExternalExecutionRequest{ID: "external-execution", OperationID: "execution", CallerID: "caller", ResourceID: "external-resource"}); err != nil {
		t.Fatal(err)
	}
	if err := manager.RunDeployment("external-entrypoints", "external-execution"); !store.IsKind(err, store.KindConflict) {
		t.Fatalf("external deployment worker error = %v, want conflict", err)
	}
	if tmux.starts != 0 || len(tmux.observedIDs) != 0 {
		t.Fatalf("external task entrypoints touched tmux: starts=%d observations=%v", tmux.starts, tmux.observedIDs)
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
