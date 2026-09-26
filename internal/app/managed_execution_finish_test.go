package app

import (
	"strings"
	"testing"
)

func TestManagedExecutionFinishAfterArchivedTask(t *testing.T) {
	setupTaskCommandTest(t)
	if result := runCommand(t, []string{"task", "create", "--task-id", "stuck-task", "--title", "Stuck execution"}); result.code != 0 {
		t.Fatalf("task create = (%d, %q)", result.code, result.stdout)
	}
	created := runCommand(t, []string{"task", "execution", "create", "stuck-task", "--execution-id", "stuck-1", "--target", "external", "--command", "pi"})
	if created.code != 0 || !strings.Contains(created.stdout, "revision: 0") || strings.Contains(created.stdout, "provenance: external") {
		t.Fatalf("execution create = (%d, %q)", created.code, created.stdout)
	}
	finished := runCommand(t, []string{"task", "execution", "finish", "stuck-task", "stuck-1", "--caller-id", "caller", "--operation-id", "close-inflight", "--expected-revision", "0", "--contract", "delivery", "--result", "succeeded"})
	if finished.code != 0 || !strings.Contains(finished.stdout, "status: finished") || !strings.Contains(finished.stdout, "revision: 1") || strings.Contains(finished.stdout, "provenance: external") {
		t.Fatalf("in-flight finish = (%d, %q)", finished.code, finished.stdout)
	}
	observedAt := "2026-09-26T00:00:00Z"
	observed := runCommand(t, []string{"task", "execution", "observe", "stuck-task", "stuck-1", "--caller-id", "caller", "--operation-id", "observe", "--expected-revision", "1", "--source", "worker", "--observed-at", observedAt, "--host-id", "host", "--boot-id", "boot"})
	if observed.code != 1 || !strings.Contains(observed.stdout, "not an externally declared record") {
		t.Fatalf("managed observation = (%d, %q)", observed.code, observed.stdout)
	}
	if result := runCommand(t, []string{"task", "finish", "stuck-task", "succeeded", "parent closed"}); result.code != 0 {
		t.Fatalf("task finish = (%d, %q)", result.code, result.stdout)
	}
}

func TestManagedExecutionFinishOnArchivedTaskIsIdempotent(t *testing.T) {
	setupTaskCommandTest(t)
	if result := runCommand(t, []string{"task", "create", "--task-id", "archived-stuck", "--title", "Archived stuck execution"}); result.code != 0 {
		t.Fatalf("task create = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "execution", "create", "archived-stuck", "--execution-id", "left-active", "--target", "external"}); result.code != 0 {
		t.Fatalf("execution create = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "execution", "publish", "archived-stuck", "left-active", "--condition", "active", "--activity", "still working"}); result.code != 0 {
		t.Fatalf("publish = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "finish", "archived-stuck", "succeeded", "parent archived next"}); result.code != 0 {
		t.Fatalf("task finish = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "archive", "archived-stuck"}); result.code != 0 {
		t.Fatalf("task archive = (%d, %q)", result.code, result.stdout)
	}
	reconciled := runCommand(t, []string{"task", "execution", "reconcile", "archived-stuck"})
	if reconciled.code != 0 || !strings.Contains(reconciled.stdout, "revision: 0") || !strings.Contains(reconciled.stdout, "status: active") {
		t.Fatalf("reconcile = (%d, %q)", reconciled.code, reconciled.stdout)
	}
	closed := runCommand(t, []string{"task", "execution", "finish", "archived-stuck", "left-active", "--caller-id", "caller", "--operation-id", "close-archived", "--expected-revision", "0", "--contract", "delivery", "--result", "succeeded"})
	if closed.code != 0 || !strings.Contains(closed.stdout, "status: finished") || strings.Contains(closed.stdout, "status: active") {
		t.Fatalf("archived finish = (%d, %q)", closed.code, closed.stdout)
	}
	retry := runCommand(t, []string{"task", "execution", "finish", "archived-stuck", "left-active", "--caller-id", "caller", "--operation-id", "close-archived", "--expected-revision", "0", "--contract", "delivery", "--result", "succeeded"})
	if retry.code != 0 || !strings.Contains(retry.stdout, "revision: 1") {
		t.Fatalf("finish retry = (%d, %q)", retry.code, retry.stdout)
	}
	changed := runCommand(t, []string{"task", "execution", "finish", "archived-stuck", "left-active", "--caller-id", "caller", "--operation-id", "close-archived", "--expected-revision", "1", "--contract", "delivery", "--result", "failed"})
	if changed.code != 1 || !strings.Contains(changed.stdout, "different inputs") {
		t.Fatalf("changed finish = (%d, %q)", changed.code, changed.stdout)
	}
	inspected := runCommand(t, []string{"task", "execution", "inspect", "archived-stuck", "left-active"})
	if inspected.code != 0 || !strings.Contains(inspected.stdout, "status: finished") || strings.Contains(inspected.stdout, "provenance: external") {
		t.Fatalf("inspect = (%d, %q)", inspected.code, inspected.stdout)
	}
}
