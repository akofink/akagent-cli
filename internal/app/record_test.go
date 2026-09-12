package app

import (
	"strings"
	"testing"
)

func TestRecordOnlyTaskFlowHasNoHostSideEffects(t *testing.T) {
	setupTaskCommandTest(t)
	if result := runCommand(t, []string{"task", "create", "--task-id", "record-task", "--title", "External task"}); result.code != 0 {
		t.Fatalf("task create = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "record", "task", "record-task", "--caller-id", "caller", "--operation-id", "adopt"}); result.code != 0 || !strings.Contains(result.stdout, "provenance: external") {
		t.Fatalf("task record = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "record", "complete", "record-task", "--caller-id", "caller", "--operation-id", "complete", "--expected-revision", "1", "--contract", "contract-v1", "--result", "done"}); result.code != 0 || !strings.Contains(result.stdout, "status: finished") {
		t.Fatalf("task complete = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "record", "archive", "record-task", "--caller-id", "caller", "--operation-id", "archive", "--expected-revision", "2"}); result.code != 0 || !strings.Contains(result.stdout, "record: task") {
		t.Fatalf("task archive = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "archive", "record-task"}); result.code != 1 || !strings.Contains(result.stdout, "category: conflict") {
		t.Fatalf("legacy archive = (%d, %q), want provenance guard", result.code, result.stdout)
	}
}

func TestRecordOnlyExternalAttemptUsesRevisionAndProvenance(t *testing.T) {
	setupTaskCommandTest(t)
	if result := runCommand(t, []string{"task", "create", "--task-id", "attempt-task", "--title", "External attempt"}); result.code != 0 {
		t.Fatalf("task create = (%d, %q)", result.code, result.stdout)
	}
	args := []string{"task", "record", "adopt", "attempt-task", "--resource-id", "resource", "--caller-id", "caller", "--operation-id", "adopt-resource", "--repository", "offline", "--branch", "external/branch", "--base", "base", "--head", "head", "--worktree", "/missing/external"}
	if result := runCommand(t, args); result.code != 0 {
		t.Fatalf("resource adoption = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "record", "execution", "attempt-task", "--execution-id", "attempt", "--resource-id", "resource", "--caller-id", "caller", "--operation-id", "create-attempt", "--tool", "offline", "--session-id", "session"}); result.code != 0 {
		t.Fatalf("execution record = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "record", "observe", "attempt-task", "attempt", "--caller-id", "caller", "--operation-id", "observe", "--expected-revision", "1", "--source", "offline", "--observed-at", "2026-09-12T13:00:00Z", "--host-id", "host", "--boot-id", "old-boot", "--process-state", "running"}); result.code != 0 || !strings.Contains(result.stdout, "host,old-boot,running") {
		t.Fatalf("external observation = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "record", "complete", "attempt-task", "--execution-id", "attempt", "--caller-id", "caller", "--operation-id", "complete-attempt", "--expected-revision", "2", "--contract", "contract-v1", "--result", "done"}); result.code != 0 || !strings.Contains(result.stdout, "status: finished") {
		t.Fatalf("external completion = (%d, %q)", result.code, result.stdout)
	}
}
