package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/akofink/akagent-cli/internal/derived"
)

type unavailableRunner struct{ calls *int }

func (r unavailableRunner) Run(context.Context, string, ...string) ([]byte, error) {
	*r.calls++
	return nil, errors.New("unavailable")
}

func setupCheckTest(t *testing.T) *int {
	t.Helper()
	setupTaskCommandTest(t)
	calls := 0
	previous := checkRunner
	checkRunner = unavailableRunner{calls: &calls}
	t.Cleanup(func() { checkRunner = previous })
	return &calls
}

func mustRun(t *testing.T, commands ...[]string) {
	t.Helper()
	for _, args := range commands {
		if result := runCommand(t, args); result.code != 0 {
			t.Fatalf("%v: %s", args, result.stdout)
		}
	}
}

func TestTaskCheckSingleTask(t *testing.T) {
	calls := setupCheckTest(t)
	mustRun(t,
		[]string{"task", "create", "--title", "check", "--task-id", "check-task"},
		[]string{"task", "resource", "create", "check-task", "--resource-id", "source", "--repository", "not-registered", "--branch", "agent/check", "--worktree", "/no/such/worktree"},
		[]string{"task", "execution", "create", "check-task", "--execution-id", "attempt", "--target", "external"},
	)
	before := runCommand(t, []string{"task", "inspect", "check-task", "--format", "json"})

	result := runCommand(t, []string{"task", "check", "check-task", "--format", "json"})
	if result.code != 0 {
		t.Fatalf("check: %+v", result)
	}
	var report derived.Report
	if err := json.Unmarshal([]byte(result.stdout), &report); err != nil {
		t.Fatalf("json: %v %s", err, result.stdout)
	}
	if report.TaskID != "check-task" || len(report.Resources) != 1 || report.Resources[0].Findings[0].Code != "repository_unavailable" || report.Executions[0].Code != "adapter_unavailable" {
		t.Fatalf("report: %+v", report)
	}
	if *calls == 0 {
		t.Fatalf("live check made no adapter calls")
	}
	if after := runCommand(t, []string{"task", "inspect", "check-task", "--format", "json"}); after.stdout != before.stdout {
		t.Fatalf("check mutated the task:\n%s\n%s", before.stdout, after.stdout)
	}

	*calls = 0
	offline := runCommand(t, []string{"task", "check", "check-task", "--offline"})
	if offline.code != 0 || *calls != 0 || !strings.Contains(offline.stdout, "worktree,unknown,offline") || !strings.Contains(offline.stdout, "needs_action: 0") {
		t.Fatalf("offline: calls=%d %+v", *calls, offline)
	}

	missing := runCommand(t, []string{"task", "check", "no-such-task"})
	if missing.code != 1 {
		t.Fatalf("missing task: %+v", missing)
	}
}

func TestTaskCheckAll(t *testing.T) {
	setupCheckTest(t)
	mustRun(t,
		[]string{"task", "create", "--title", "open", "--task-id", "open-task"},
		[]string{"task", "create", "--title", "done", "--task-id", "done-task"},
		[]string{"task", "execution", "create", "done-task", "--execution-id", "leftover", "--target", "external"},
		[]string{"task", "finish", "done-task", "succeeded", "delivered"},
	)
	result := runCommand(t, []string{"task", "check", "--all", "--offline"})
	if result.code != 0 {
		t.Fatalf("check all: %+v", result)
	}
	for _, want := range []string{"checked: 2", "task_id: done-task", `"execution:leftover",stale,task_terminal`, "needs_action: 1"} {
		if !strings.Contains(result.stdout, want) {
			t.Fatalf("missing %q in:\n%s", want, result.stdout)
		}
	}
	if strings.Contains(result.stdout, "open-task") {
		t.Fatalf("listed a task without findings:\n%s", result.stdout)
	}
}

func TestTaskCheckUsage(t *testing.T) {
	setupCheckTest(t)
	for _, args := range [][]string{
		{"task", "check"},
		{"task", "check", "one", "two"},
		{"task", "check", "one", "--all"},
		{"task", "check", "one", "--format", "xml"},
		{"task", "check", "one", "--format"},
		{"task", "check", "one", "--format", "json", "--format", "json"},
		{"task", "check", "one", "--verbose"},
	} {
		if result := runCommand(t, args); result.code != 2 || !strings.Contains(result.stdout, "task check") {
			t.Fatalf("%v: %+v", args, result)
		}
	}
}
