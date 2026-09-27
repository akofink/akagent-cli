package app

import (
	"strings"
	"testing"
)

func TestBeginAndDeliverWithResourceAndRetry(t *testing.T) {
	setupTaskCommandTest(t)
	t.Setenv("TMUX_PANE", "%13")
	begin := []string{"task", "begin", "lean-task", "--title", "Lean work", "--execution-id", "attempt", "--command", "pi"}
	mustRun(t, begin)
	mustRun(t, []string{"task", "resource", "create", "lean-task", "--resource-id", "repo", "--repository", "demo", "--branch", "agent/lean", "--worktree", "/abs/lean"})
	mustRun(t, begin)
	before := runCommand(t, []string{"task", "inspect", "lean-task", "--format", "json"})
	if !strings.Contains(before.stdout, `"tmux_pane":"%13"`) || !strings.Contains(before.stdout, `"condition":"active"`) {
		t.Fatalf("begin: %s", before.stdout)
	}
	mustRun(t, []string{"task", "publish", "lean-task", "--condition", "blocked", "--reason", "waiting on review"})
	deliver := []string{"task", "deliver", "lean-task", "attempt", "--contract", "implementation", "--result", "succeeded", "--summary", "PR merged and verified", "--verified"}
	mustRun(t, deliver)
	inspection := runCommand(t, []string{"task", "inspect", "lean-task", "--format", "json"})
	if inspection.code != 0 || strings.Contains(inspection.stdout, `"reason":"waiting on review"`) || !strings.Contains(inspection.stdout, `"condition":"succeeded"`) {
		t.Fatalf("delivered task retained blocked reason: %+v", inspection)
	}
	mustRun(t, deliver)
	result := runCommand(t, []string{"task", "check", "lean-task", "--offline"})
	if result.code != 0 || strings.Contains(result.stdout, "task_terminal") || !strings.Contains(result.stdout, `"execution:attempt",current,closed`) {
		t.Fatalf("delivery: %+v", result)
	}
	if result := runCommand(t, begin); result.code != 1 {
		t.Fatalf("terminal begin accepted: %+v", result)
	}
}

func TestBeginAndDeliverRejectConflicts(t *testing.T) {
	setupTaskCommandTest(t)
	t.Setenv("TMUX_PANE", "")
	begin := []string{"task", "begin", "shortcut-task", "--title", "work", "--execution-id", "attempt", "--command", "pi"}
	mustRun(t, begin)
	for _, args := range [][]string{
		{"task", "begin", "shortcut-task", "--title", "different", "--execution-id", "attempt", "--command", "pi"},
		{"task", "deliver", "shortcut-task", "attempt", "--contract", "test", "--result", "succeeded", "--summary", "done"},
		{"task", "deliver", "shortcut-task", "attempt", "--contract", "test", "--result", "succeeded", "--summary", "done", "--verified", "--verified"},
	} {
		if got := runCommand(t, args); got.code == 0 {
			t.Fatalf("accepted conflict or unverified completion: %v", args)
		}
	}
	mustRun(t, []string{"task", "execution", "create", "shortcut-task", "--execution-id", "other", "--target", "external"})
	if got := runCommand(t, []string{"task", "deliver", "shortcut-task", "attempt", "--contract", "test", "--result", "succeeded", "--summary", "done", "--verified"}); got.code != 1 {
		t.Fatalf("accepted open successor: %+v", got)
	}
}

func TestDeliverResumesAfterExecutionClose(t *testing.T) {
	setupTaskCommandTest(t)
	t.Setenv("TMUX_PANE", "")
	mustRun(t, []string{"task", "begin", "partial-task", "--title", "work", "--execution-id", "attempt", "--command", "pi"},
		[]string{"task", "execution", "finish", "partial-task", "attempt", "--caller-id", "agent", "--operation-id", "deliver", "--expected-revision", "0", "--contract", "implementation", "--result", "succeeded"})
	mustRun(t, []string{"task", "deliver", "partial-task", "attempt", "--contract", "implementation", "--result", "succeeded", "--summary", "verified", "--verified"})
}
