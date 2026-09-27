package app

import (
	"encoding/json"
	"os"
	"testing"
)

func TestExecutionCapturesTmuxBindingAtCreation(t *testing.T) {
	setupTaskCommandTest(t)
	t.Setenv("TMUX_PANE", "%123")
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	mustRun(t, []string{"task", "create", "--task-id", "binding-task", "--title", "binding"},
		[]string{"task", "execution", "create", "binding-task", "--execution-id", "attempt", "--target", "external"})
	result := runCommand(t, []string{"task", "inspect", "binding-task", "--format", "json"})
	var view struct {
		Executions []struct {
			Host string `json:"host"`
			Pane string `json:"tmux_pane"`
		} `json:"executions"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &view); err != nil || len(view.Executions) != 1 || view.Executions[0].Host != host || view.Executions[0].Pane != "%123" {
		t.Fatalf("binding: %v %+v %s", err, view, result.stdout)
	}
	mustRun(t, []string{"task", "execution", "create", "binding-task", "--execution-id", "attempt", "--target", "external"})
	t.Setenv("TMUX_PANE", "%124")
	if retry := runCommand(t, []string{"task", "execution", "create", "binding-task", "--execution-id", "attempt", "--target", "external"}); retry.code != 1 {
		t.Fatalf("changed pane should conflict: %+v", retry)
	}
}
