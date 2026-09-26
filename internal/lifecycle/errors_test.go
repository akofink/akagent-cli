package lifecycle

import (
	"testing"

	"github.com/akofink/akagent-cli/internal/store"
)

func TestLifecycleValidationKinds(t *testing.T) {
	manager, _ := newTestManager(t)
	tests := []struct {
		name string
		err  func() error
		kind store.ErrorKind
		want string
	}{
		{"create input", func() error { _, err := manager.CreateRecord(CreateRequest{}); return err }, store.KindInternal, "task ID and title are required"},
		{"publish input", func() error { _, err := manager.PublishRecord("task", "invalid", "", ""); return err }, store.KindInternal, "condition must be active, waiting, blocked, failed, or none"},
		{"execution input", func() error { _, _, err := manager.CreateExecution("", ExecutionRequest{}); return err }, store.KindInternal, "task ID and execution target are required"},
		{"resource input", func() error { _, _, err := manager.CreateResourceRecord("", ResourceRequest{}); return err }, store.KindInternal, "task ID, resource ID, and repository are required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.err()
			if !store.IsKind(err, tt.kind) || err.Error() != tt.want {
				t.Fatalf("error = %v, want kind %s and message %q", err, tt.kind, tt.want)
			}
		})
	}

	if _, err := manager.CreateRecord(CreateRequest{ID: "same-task", Title: "Original"}); err != nil {
		t.Fatal(err)
	}
	_, err := manager.CreateRecord(CreateRequest{ID: "same-task", Title: "Different"})
	if !store.IsKind(err, store.KindConflict) || err.Error() != "task inputs conflict with the existing task" {
		t.Fatalf("conflict = %v, want typed conflict with stable message", err)
	}
}
