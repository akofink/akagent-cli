package app

import (
	"context"
	"os"
	"strings"
	"testing"
)

type panesRunner struct{ panes string }

func (r panesRunner) Run(_ context.Context, name string, _ ...string) ([]byte, error) {
	if name == "tmux" {
		return []byte(r.panes), nil
	}
	return nil, nil
}

func TestRepairSupersededRequiresLiveSuccessorAndAttestation(t *testing.T) {
	setupTaskCommandTest(t)
	t.Setenv("TMUX_PANE", "")
	host, _ := os.Hostname()
	previous := checkRunner
	checkRunner = panesRunner{panes: "%22|@11|\n"}
	t.Cleanup(func() { checkRunner = previous })
	mustRun(t, []string{"task", "create", "--title", "takeover", "--task-id", "takeover-task"},
		[]string{"task", "execution", "create", "takeover-task", "--execution-id", "old", "--target", "external", "--host", host, "--tmux-pane", "%21"},
		[]string{"task", "begin", "takeover-task", "--title", "takeover", "--execution-id", "new", "--command", "pi", "--host", host, "--tmux-pane", "%22"})
	base := []string{"task", "repair", "superseded-execution", "--task-id", "takeover-task", "--execution-id", "old", "--successor-id", "new"}
	if result := runCommand(t, base); result.code != 0 || !strings.Contains(result.stdout, "old,0,") {
		t.Fatalf("preview: %+v", result)
	}
	if result := runCommand(t, append(append([]string{}, base...), "--apply", "--expected-revision", "0", "--contract", "takeover")); result.code == 0 {
		t.Fatalf("accepted missing attestation: %+v", result)
	}
	checkRunner = panesRunner{panes: "%21|@10|\n%22|@11|\n"}
	if check := runCommand(t, []string{"task", "check", "takeover-task"}); !strings.Contains(check.stdout, "multiple_live_executions") {
		t.Fatalf("missing duplicate-live finding: %+v", check)
	}
	if result := runCommand(t, base); result.code == 0 {
		t.Fatalf("accepted live predecessor: %+v", result)
	}
	checkRunner = panesRunner{panes: "%22|@11|\n"}
	apply := append(append([]string{}, base...), "--apply", "--expected-revision", "0", "--contract", "takeover", "--verified-ended")
	mustRun(t, apply)
	if result := runCommand(t, []string{"task", "inspect", "takeover-task"}); !strings.Contains(result.stdout, "result: unverified") {
		t.Fatalf("not closed: %+v", result)
	}
}

func TestRepairTerminalExecutionIsDryRunByDefault(t *testing.T) {
	setupTaskCommandTest(t)
	t.Setenv("TMUX_PANE", "")
	mustRun(t, []string{"task", "create", "--title", "finished", "--task-id", "repair-task"},
		[]string{"task", "execution", "create", "repair-task", "--target", "external", "--execution-id", "attempt"},
		[]string{"task", "finish", "repair-task", "succeeded", "work delivered"},
		[]string{"task", "archive", "repair-task"})
	preview := runCommand(t, []string{"task", "repair", "terminal-executions"})
	if preview.code != 0 || !strings.Contains(preview.stdout, "mode: dry_run") || !strings.Contains(preview.stdout, "total: 1") || !strings.Contains(preview.stdout, "repair-task,attempt,0,") {
		t.Fatalf("preview: %+v", preview)
	}
	check := runCommand(t, []string{"task", "check", "--all", "--offline"})
	if !strings.Contains(check.stdout, "needs_action: 1") {
		t.Fatalf("dry run changed records: %+v", check)
	}
	for _, flags := range [][]string{
		{"--apply", "--expected-revision", "0", "--contract", "history"},
		{"--apply", "--expected-revision", "1", "--contract", "history", "--verified-ended"},
	} {
		args := append([]string{"task", "repair", "terminal-executions", "--task-id", "repair-task", "--execution-id", "attempt"}, flags...)
		if result := runCommand(t, args); result.code == 0 {
			t.Fatalf("accepted invalid closure %v", args)
		}
	}
	apply := []string{"task", "repair", "terminal-executions", "--task-id", "repair-task", "--execution-id", "attempt", "--apply", "--expected-revision", "0", "--contract", "historical-attempt", "--verified-ended"}
	mustRun(t, apply)
	check = runCommand(t, []string{"task", "check", "--all", "--offline"})
	if !strings.Contains(check.stdout, "needs_action: 0") {
		t.Fatalf("repair did not close execution: %+v", check)
	}
	if result := runCommand(t, apply); result.code == 0 {
		t.Fatalf("stale revision accepted: %+v", result)
	}
}

func TestRepairDoesNotSelectOpenOrExternallyOwnedTasks(t *testing.T) {
	setupTaskCommandTest(t)
	t.Setenv("TMUX_PANE", "")
	mustRun(t, []string{"task", "create", "--title", "ongoing", "--task-id", "open-task"},
		[]string{"task", "execution", "create", "open-task", "--target", "external", "--execution-id", "open-attempt"})
	preview := runCommand(t, []string{"task", "repair", "terminal-executions"})
	if preview.code != 0 || !strings.Contains(preview.stdout, "total: 0") {
		t.Fatalf("selected open task: %+v", preview)
	}
}
