package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalTaskFlowIsRecordOnly(t *testing.T) {
	setupTaskCommandTest(t)
	if result := runCommand(t, []string{"task", "create", "--task-id", "record-task", "--title", "External task"}); result.code != 0 {
		t.Fatalf("task create = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"repository", "register", "demo", "/missing/repository", "--policy", "direct"}); result.code != 0 {
		t.Fatalf("repository register = (%d, %q)", result.code, result.stdout)
	}
	resource := runCommand(t, []string{"task", "resource", "create", "record-task", "--resource-id", "resource", "--repository", "demo", "--branch", "main", "--base", "base", "--head", "head", "--worktree", "/missing/worktree"})
	if resource.code != 0 || !strings.Contains(resource.stdout, "id: resource") {
		t.Fatalf("resource create = (%d, %q)", resource.code, resource.stdout)
	}
	execution := runCommand(t, []string{"task", "execution", "create", "record-task", "--execution-id", "attempt", "--target", "external", "--command", "/not-started"})
	if execution.code != 0 || !strings.Contains(execution.stdout, "id: attempt") {
		t.Fatalf("execution create = (%d, %q)", execution.code, execution.stdout)
	}
	published := runCommand(t, []string{"task", "execution", "publish", "record-task", "attempt", "--condition", "waiting", "--reason", "external", "--activity", "awaiting observation"})
	if published.code != 0 || !strings.Contains(published.stdout, "condition: waiting") {
		t.Fatalf("execution publish = (%d, %q)", published.code, published.stdout)
	}
	finished := runCommand(t, []string{"task", "finish", "record-task", "succeeded", "recorded"})
	if finished.code != 0 || !strings.Contains(finished.stdout, "status: finished") {
		t.Fatalf("task finish = (%d, %q)", finished.code, finished.stdout)
	}
}

func TestRemovedCommandsRefuseBeforeStoreAccess(t *testing.T) {
	setupTaskCommandTest(t)
	t.Setenv("XDG_STATE_HOME", "/dev/null/akagent-invalid-state")
	for _, args := range [][]string{
		{"credential", "list"},
		{"integration", "launch", "task"},
		{"worker", "launch", "task"},
		{"worker", "launch-pi", "task", "execution"},
		{"worker", "deploy", "task", "command"},
		{"task", "record", "task", "task"},
		{"task", "launch", "task", "--target", "shell"},
		{"task", "execution", "launch", "task", "execution"},
		{"task", "resource", "clean", "task", "resource"},
	} {
		result := runCommand(t, args)
		if result.code != 2 || !strings.Contains(result.stdout, "category: usage") || strings.Contains(result.stdout, "state store") {
			t.Errorf("Run(%q) = (%d, %q), want pre-store structured migration error", args, result.code, result.stdout)
		}
	}
}

func TestRetainedCommandsDoNotInvokeHostTools(t *testing.T) {
	setupTaskCommandTest(t)
	root := t.TempDir()
	logPath := filepath.Join(root, "invocations.log")
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"git", "tmux", "pi", "provider", "deploy"} {
		path := filepath.Join(bin, name)
		script := "#!/bin/sh\nprintf '%s\\n' '" + name + "' >> '" + logPath + "'\nexit 99\n"
		if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	commands := [][]string{
		{"worker", "inspect"},
		{"repository", "register", "demo", "/offline/repository", "--policy", "direct"},
		{"repository", "list"},
		{"task", "create", "--task-id", "canary-task", "--title", "No host tools"},
		{"task", "resource", "create", "canary-task", "--resource-id", "resource", "--repository", "demo", "--branch", "main", "--base", "base", "--head", "head", "--worktree", "/offline/worktree"},
		{"task", "execution", "create", "canary-task", "--execution-id", "execution", "--target", "external", "--command", "/offline/command", "--resource", "resource"},
		{"task", "publish", "canary-task", "--condition", "active", "--activity", "recorded"},
		{"task", "execution", "publish", "canary-task", "execution", "--condition", "waiting", "--activity", "recorded"},
		{"task", "inspect", "canary-task"},
		{"task", "reconcile", "canary-task"},
		{"task", "finish", "canary-task", "succeeded", "recorded"},
		{"task", "archive", "canary-task"},
	}
	for _, command := range commands {
		if result := runCommand(t, command); result.code != 0 {
			t.Fatalf("Run(%q) = (%d, %q)", command, result.code, result.stdout)
		}
	}
	if data, err := os.ReadFile(logPath); err == nil {
		t.Fatalf("retained commands invoked host tools: %q", data)
	} else if !os.IsNotExist(err) {
		t.Fatalf("read host-tool canary log: %v", err)
	}
}

func TestRecordAliasIsRemoved(t *testing.T) {
	setupTaskCommandTest(t)
	result := runCommand(t, []string{"task", "record", "task", "record-task", "--caller-id", "caller", "--operation-id", "operation"})
	if result.code != 2 || !strings.Contains(result.stdout, "task record") || !strings.Contains(result.stdout, "normal task") {
		t.Fatalf("task record = (%d, %q), want migration guidance", result.code, result.stdout)
	}
}
