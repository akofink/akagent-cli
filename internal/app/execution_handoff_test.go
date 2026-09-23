package app

import (
	"strings"
	"testing"
)

func TestManagedExecutionHandoffRequiresVerifiedActiveSuccessor(t *testing.T) {
	setupTaskCommandTest(t)
	if result := runCommand(t, []string{"task", "create", "--task-id", "handoff-task", "--title", "Primary handoff"}); result.code != 0 {
		t.Fatalf("task create = (%d, %q)", result.code, result.stdout)
	}
	for _, id := range []string{"predecessor", "successor"} {
		if result := runCommand(t, []string{"task", "execution", "create", "handoff-task", "--execution-id", id, "--target", "tool"}); result.code != 0 {
			t.Fatalf("execution create %s = (%d, %q)", id, result.code, result.stdout)
		}
	}
	if result := runCommand(t, []string{"task", "execution", "publish", "handoff-task", "predecessor", "--condition", "waiting", "--activity", "handed off"}); result.code != 0 {
		t.Fatalf("predecessor publish = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "execution", "publish", "handoff-task", "successor", "--condition", "active", "--activity", "primary active"}); result.code != 0 {
		t.Fatalf("successor publish = (%d, %q)", result.code, result.stdout)
	}

	args := []string{"task", "execution", "handoff", "handoff-task", "predecessor", "--successor-execution", "successor", "--operation-id", "handoff-1", "--expected-revision", "0", "--takeover-verified", "--predecessor-closed"}
	stale := append([]string(nil), args...)
	stale[10] = "1"
	if result := runCommand(t, stale); result.code != 1 || !strings.Contains(result.stdout, "revision conflict") {
		t.Fatalf("stale predecessor revision = (%d, %q), want conflict", result.code, result.stdout)
	}
	result := runCommand(t, args)
	if result.code != 0 || !strings.Contains(result.stdout, "status: finished") || !strings.Contains(result.stdout, "result: handed_off") || !strings.Contains(result.stdout, "successor_execution_id: successor") {
		t.Fatalf("handoff = (%d, %q)", result.code, result.stdout)
	}
	if strings.Contains(result.stdout, "provenance: external") || !strings.Contains(result.stdout, "revision: 1") {
		t.Fatalf("handoff changed provenance or revision unexpectedly: %q", result.stdout)
	}

	retry := runCommand(t, args)
	if retry.code != 0 || !strings.Contains(retry.stdout, "revision: 1") {
		t.Fatalf("idempotent handoff retry = (%d, %q)", retry.code, retry.stdout)
	}
	changed := append([]string(nil), args...)
	changed[8] = "handoff-2"
	if conflict := runCommand(t, changed); conflict.code != 1 || !strings.Contains(conflict.stdout, "conflict") {
		t.Fatalf("different terminal handoff = (%d, %q), want conflict", conflict.code, conflict.stdout)
	}
}

func TestManagedExecutionHandoffRejectsMissingAttestationAndInactiveSuccessor(t *testing.T) {
	setupTaskCommandTest(t)
	if result := runCommand(t, []string{"task", "create", "--task-id", "handoff-guard", "--title", "Handoff guard"}); result.code != 0 {
		t.Fatalf("task create = (%d, %q)", result.code, result.stdout)
	}
	for _, id := range []string{"old", "new"} {
		if result := runCommand(t, []string{"task", "execution", "create", "handoff-guard", "--execution-id", id, "--target", "tool"}); result.code != 0 {
			t.Fatalf("execution create %s = (%d, %q)", id, result.code, result.stdout)
		}
	}
	if result := runCommand(t, []string{"task", "execution", "publish", "handoff-guard", "old", "--condition", "waiting", "--activity", "handed off"}); result.code != 0 {
		t.Fatalf("predecessor publish = (%d, %q)", result.code, result.stdout)
	}
	args := []string{"task", "execution", "handoff", "handoff-guard", "old", "--successor-execution", "new", "--operation-id", "handoff-guard-1", "--expected-revision", "0", "--takeover-verified", "--predecessor-closed"}
	if result := runCommand(t, args[:len(args)-1]); result.code != 2 || !strings.Contains(result.stdout, "usage") {
		t.Fatalf("missing confirmation = (%d, %q), want usage error", result.code, result.stdout)
	}
	if result := runCommand(t, args); result.code != 1 || !strings.Contains(result.stdout, "active managed execution") {
		t.Fatalf("inactive successor = (%d, %q), want conflict", result.code, result.stdout)
	}
	inspected := runCommand(t, []string{"task", "execution", "inspect", "handoff-guard", "old"})
	if inspected.code != 0 || !strings.Contains(inspected.stdout, "activity: handed off") || strings.Contains(inspected.stdout, "handoff_disposition:") {
		t.Fatalf("predecessor changed after refused handoff = (%d, %q)", inspected.code, inspected.stdout)
	}
}
