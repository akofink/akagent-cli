package app

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPublishedActiveConditionIsReportedAsActiveStatus(t *testing.T) {
	setupTaskCommandTest(t)

	steps := [][]string{
		{"task", "create", "--task-id", "published", "--title", "Published work"},
		{"task", "execution", "create", "published", "--execution-id", "agent", "--target", "external", "--command", "pi"},
	}
	for _, args := range steps {
		if result := runCommand(t, args); result.code != 0 {
			t.Fatalf("%v = (%d, %q)", args, result.code, result.stdout)
		}
	}

	before := runCommand(t, []string{"task", "list", "--format", "json"})
	if !strings.Contains(before.stdout, `"status":"created"`) {
		t.Fatalf("unpublished list = %q, want created status", before.stdout)
	}

	for _, args := range [][]string{
		{"task", "publish", "published", "--condition", "active", "--activity", "working"},
		{"task", "execution", "publish", "published", "agent", "--condition", "active", "--activity", "working"},
	} {
		if result := runCommand(t, args); result.code != 0 {
			t.Fatalf("%v = (%d, %q)", args, result.code, result.stdout)
		}
	}

	inspect := runCommand(t, []string{"task", "inspect", "published", "--format", "json"})
	var detail struct {
		Task       map[string]any   `json:"task"`
		Executions []map[string]any `json:"executions"`
	}
	if err := json.Unmarshal([]byte(inspect.stdout), &detail); err != nil {
		t.Fatalf("inspect json = %q: %v", inspect.stdout, err)
	}
	if detail.Task["status"] != "active" {
		t.Fatalf("task status = %v, want active", detail.Task["status"])
	}
	if len(detail.Executions) != 1 || detail.Executions[0]["status"] != "active" {
		t.Fatalf("executions = %v, want one active execution", detail.Executions)
	}
	for _, field := range []string{"committed", "dirty", "untracked"} {
		if _, ok := detail.Task[field]; ok {
			t.Fatalf("task view includes unset legacy field %q: %v", field, detail.Task)
		}
	}

	human := runCommand(t, []string{"task", "inspect", "published", "--format", "human"})
	if strings.Contains(human.stdout, "committed:") {
		t.Fatalf("human inspect shows unset task git flags: %q", human.stdout)
	}

	for _, args := range [][]string{
		{"task", "publish", "published", "--condition", "waiting", "--reason", "review"},
	} {
		if result := runCommand(t, args); result.code != 0 {
			t.Fatalf("%v = (%d, %q)", args, result.code, result.stdout)
		}
	}
	waiting := runCommand(t, []string{"task", "list", "--format", "json"})
	if !strings.Contains(waiting.stdout, `"status":"waiting"`) {
		t.Fatalf("waiting list = %q, want waiting status", waiting.stdout)
	}
}
