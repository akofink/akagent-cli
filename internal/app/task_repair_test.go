package app

import (
	"strings"
	"testing"
)

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
