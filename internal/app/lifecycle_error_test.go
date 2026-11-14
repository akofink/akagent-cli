package app

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/akofink/akagent-cli/internal/store"
)

func TestLifecycleErrorClassification(t *testing.T) {
	const defaultRecovery = "Inspect the task state and retry"
	tests := []struct {
		name string
		err  error
		code int
		want string
	}{
		{"plain conflict word", errors.New("task inputs conflict with the existing task"), 1, "error:\n  category: internal\n  message: task inputs conflict with the existing task\n  retryable: false\n  recovery: " + defaultRecovery + "\n"},
		{"plain credential word", errors.New("credential not available"), 1, "error:\n  category: internal\n  message: credential not available\n  retryable: false\n  recovery: " + defaultRecovery + "\n"},
		{"typed conflict", &store.Error{Kind: store.KindConflict, Message: "task inputs conflict with the existing task"}, 1, "error:\n  category: conflict\n  message: task inputs conflict with the existing task\n  retryable: false\n  recovery: " + defaultRecovery + "\n"},
		{"wrapped conflict", fmt.Errorf("failed to record: %w", &store.Error{Kind: store.KindConflict, Message: "immutable", Recovery: "Inspect and retry"}), 1, "error:\n  category: conflict\n  message: " + `"failed to record: immutable"` + "\n  retryable: false\n  recovery: Inspect and retry\n"},
		{"typed usage", &store.Error{Kind: store.KindUsage, Message: "invalid input", Recovery: "Provide a valid task ID"}, 2, "error:\n  category: usage\n  message: invalid input\n  retryable: false\n  recovery: Provide a valid task ID\n"},
		{"typed partial", &store.Error{Kind: store.KindPartial, Message: "partial write", Retryable: true, Recovery: "Reconcile"}, 1, "error:\n  category: partial\n  message: partial write\n  retryable: true\n  recovery: Reconcile\n"},
		{"typed lock", &store.Error{Kind: store.KindLocked, Message: "busy", Recovery: "Wait for the active writer"}, 1, "error:\n  category: retryable\n  message: busy\n  retryable: true\n  recovery: Wait for the active writer\n"},
		{"typed not found", &store.Error{Kind: store.KindNotFound, Message: "missing", Recovery: "Run `akagent task list` to find the task ID"}, 1, "error:\n  category: not_found\n  message: missing\n  retryable: false\n  recovery: Run `akagent task list` to find the task ID\n"},
		{"typed preservation", &store.Error{Kind: store.KindPreservation, Message: "preserve", Recovery: "Keep the record"}, 1, "error:\n  category: preservation_required\n  message: preserve\n  retryable: false\n  recovery: Keep the record\n"},
		{"typed malformed", &store.Error{Kind: store.KindMalformed, Message: "malformed", Recovery: "Preserve the record and inspect its schema"}, 1, "error:\n  category: internal\n  message: malformed\n  retryable: false\n  recovery: Preserve the record and inspect its schema\n"},
		{"typed unsafe permissions", &store.Error{Kind: store.KindUnsafe, Message: "unsafe permissions", Recovery: "Restrict state directory permissions"}, 1, "error:\n  category: internal\n  message: unsafe permissions\n  retryable: false\n  recovery: Restrict state directory permissions\n"},
		{"typed unsafe path", &store.Error{Kind: store.KindUnsafePath, Message: "unsafe path", Recovery: "Remove the symlink from the state directory"}, 1, "error:\n  category: internal\n  message: unsafe path\n  retryable: false\n  recovery: Remove the symlink from the state directory\n"},
		{"typed internal", &store.Error{Kind: store.KindInternal, Message: "internal failure", Recovery: "Inspect the underlying storage failure"}, 1, "error:\n  category: internal\n  message: internal failure\n  retryable: false\n  recovery: Inspect the underlying storage failure\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			if code := lifecycleError(&out, tt.err); code != tt.code || out.String() != tt.want {
				t.Fatalf("lifecycleError() = (%d, %q), want (%d, %q)", code, out.String(), tt.code, tt.want)
			}
		})
	}
}

func TestRecordCreateConflictErrorContract(t *testing.T) {
	setupTaskCommandTest(t)
	args := []string{"task", "create", "--task-id", "classification-contract", "--title", "Original"}
	if result := runCommand(t, args); result.code != 0 {
		t.Fatalf("first create = %+v", result)
	}
	result := runCommand(t, []string{"task", "create", "--task-id", "classification-contract", "--title", "Different"})
	want := "error:\n  category: conflict\n  message: task inputs conflict with the existing task\n  retryable: false\n  recovery: Inspect the task state and retry\n"
	if result.code != 1 || result.stdout != want {
		t.Fatalf("conflict = (%d, %q), want (1, %q)", result.code, result.stdout, want)
	}
}
