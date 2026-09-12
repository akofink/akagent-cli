package app

import (
	"strings"
	"testing"
)

func TestCheckpointCommandWritesAndInspectsDurableState(t *testing.T) {
	setupTaskCommandTest(t)
	if result := runCommand(t, []string{"task", "create", "--task-id", "checkpoint-cli", "--title", "Checkpoint CLI"}); result.code != 0 {
		t.Fatalf("task create = (%d, %q)", result.code, result.stdout)
	}
	args := []string{"task", "checkpoint", "write", "checkpoint-cli", "--idempotency-key", "cli-write-1", "--expected-revision", "0", "--task-kind", "implementation", "--completion-contract", "signed commit is ready", "--next-action", "run tests", "--context", "handoff=docs/handoff.md", "--resource-reference", "implementation", "--session-reference", "attempt-1=pi:session-1", "--previous-attempt-reference", "attempt-0", "--verification-scope", "focused tests", "--verification-revision", "abc123", "--verification-result", "passed", "--verification-time", "2026-09-12T12:00:00Z", "--uncertain-operation", "publish-1=publish branch"}
	written := runCommand(t, args)
	if written.code != 0 || !strings.Contains(written.stdout, "state: acknowledged") || !strings.Contains(written.stdout, "revision: 1") {
		t.Fatalf("checkpoint write = (%d, %q), want acknowledged revision", written.code, written.stdout)
	}

	repeated := runCommand(t, args)
	if repeated.code != 0 || !strings.Contains(repeated.stdout, "revision: 1") {
		t.Fatalf("repeated checkpoint write = (%d, %q), want revision 1", repeated.code, repeated.stdout)
	}
	inspected := runCommand(t, []string{"task", "checkpoint", "inspect", "checkpoint-cli"})
	if inspected.code != 0 || !strings.Contains(inspected.stdout, "available: true") || !strings.Contains(inspected.stdout, "next_action: run tests") {
		t.Fatalf("checkpoint inspect = (%d, %q), want durable checkpoint", inspected.code, inspected.stdout)
	}
}

func TestCheckpointWriteValidationIsDeterministic(t *testing.T) {
	setupTaskCommandTest(t)
	if result := runCommand(t, []string{"task", "create", "--task-id", "checkpoint-validation", "--title", "Validation"}); result.code != 0 {
		t.Fatalf("task create = (%d, %q)", result.code, result.stdout)
	}
	long := strings.Repeat("x", 4097)
	args := []string{"task", "checkpoint", "write", "checkpoint-validation", "--idempotency-key", "validation-1", "--expected-revision", "0", "--task-kind", long, "--completion-contract", long, "--next-action", "run tests"}
	first := runCommand(t, args)
	second := runCommand(t, args)
	if first.code != 2 || second.code != 2 || first.stdout != second.stdout || !strings.Contains(first.stdout, "task kind must be a bounded single-line value") {
		t.Fatalf("validation outputs differ or have wrong first error: first=(%d, %q), second=(%d, %q)", first.code, first.stdout, second.code, second.stdout)
	}
}

func TestCheckpointInspectRejectsUnknownTask(t *testing.T) {
	setupTaskCommandTest(t)
	inspected := runCommand(t, []string{"task", "checkpoint", "inspect", "does-not-exist"})
	if inspected.code != 1 || !strings.Contains(inspected.stdout, "category: not_found") {
		t.Fatalf("unknown checkpoint inspect = (%d, %q), want typed not-found", inspected.code, inspected.stdout)
	}
}

func TestCheckpointInspectReportsUnavailableLegacyState(t *testing.T) {
	setupTaskCommandTest(t)
	if result := runCommand(t, []string{"task", "create", "--task-id", "legacy-checkpoint-cli", "--title", "Legacy"}); result.code != 0 {
		t.Fatalf("task create = (%d, %q)", result.code, result.stdout)
	}
	inspected := runCommand(t, []string{"task", "checkpoint", "inspect", "legacy-checkpoint-cli"})
	if inspected.code != 0 || !strings.Contains(inspected.stdout, "available: false") || !strings.Contains(inspected.stdout, "state: unavailable") || !strings.Contains(inspected.stdout, "No acknowledged recovery checkpoint") {
		t.Fatalf("checkpoint inspect = (%d, %q), want actionable unavailable state", inspected.code, inspected.stdout)
	}
}
