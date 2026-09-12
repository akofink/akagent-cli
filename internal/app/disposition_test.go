package app

import (
	"strings"
	"testing"
)

func TestTaskDispositionCommandIsRecordOnlyAndRevisionProtected(t *testing.T) {
	setupTaskCommandTest(t)
	const taskID = "disposition-command"
	if result := runCommand(t, []string{"task", "create", "--task-id", taskID, "--title", "Disposition command"}); result.code != 0 {
		t.Fatalf("task create = (%d, %q)", result.code, result.stdout)
	}
	result := runCommand(t, []string{"task", "disposition", taskID, "deferred", "--reason", "awaiting approval"})
	if result.code != 0 || !strings.Contains(result.stdout, "disposition: deferred") || !strings.Contains(result.stdout, "disposition_reason: awaiting approval") || !strings.Contains(result.stdout, "disposition_revision: 1") {
		t.Fatalf("task disposition = (%d, %q), want durable deferred revision", result.code, result.stdout)
	}
	conflict := runCommand(t, []string{"task", "disposition", taskID, "terminal", "--reason", "done", "--expected-revision", "0"})
	if conflict.code != 1 || !strings.Contains(conflict.stdout, "category: conflict") {
		t.Fatalf("stale disposition transition = (%d, %q), want conflict", conflict.code, conflict.stdout)
	}
	inspected := runCommand(t, []string{"task", "inspect", taskID})
	if inspected.code != 0 || !strings.Contains(inspected.stdout, "disposition: deferred") || strings.Contains(inspected.stdout, "disposition: terminal") {
		t.Fatalf("stale transition changed task = (%d, %q)", inspected.code, inspected.stdout)
	}
}
