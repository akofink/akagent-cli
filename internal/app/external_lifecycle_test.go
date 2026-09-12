package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/akofink/akagent-cli/internal/lifecycle"
	"github.com/akofink/akagent-cli/internal/store"
)

func createExternalExecutionForCLI(t *testing.T, taskID string) store.Execution {
	t.Helper()
	state, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	manager := lifecycle.New(state)
	if _, err := manager.Create(lifecycle.CreateRequest{ID: taskID, Title: "external CLI lifecycle"}); err != nil {
		t.Fatal(err)
	}
	resource, err := manager.RecordResource(taskID, store.ExternalResourceRequest{
		ID: "resource", OperationID: "resource-create", CallerID: "caller", Repository: "offline",
		Branch: "akofink/146", BaseRevision: "base", Head: "head", WorktreePath: filepath.Join(t.TempDir(), "missing"),
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := manager.RecordExecution(taskID, store.ExternalExecutionRequest{
		ID: "execution", OperationID: "execution-create", CallerID: "caller", ResourceID: resource.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return execution
}

func TestExternalExecutionObservationAndCompletionCommands(t *testing.T) {
	setupTaskCommandTest(t)
	execution := createExternalExecutionForCLI(t, "external-cli")
	observedAt := time.Now().UTC().Format(time.RFC3339Nano)
	observed := runCommand(t, []string{
		"task", "execution", "observe", "external-cli", execution.ID,
		"--caller-id", "caller", "--operation-id", "observation-1", "--expected-revision", "1",
		"--source", "worker", "--observed-at", observedAt, "--host-id", "host", "--boot-id", "boot",
		"--process-state", "running", "--detail", "historical",
	})
	if observed.code != 0 || !strings.Contains(observed.stdout, "external_observations[1]") || !strings.Contains(observed.stdout, "revision: 2") {
		t.Fatalf("observation command = (%d, %q)", observed.code, observed.stdout)
	}

	stale := runCommand(t, []string{
		"task", "execution", "observe", "external-cli", execution.ID,
		"--caller-id", "caller", "--operation-id", "observation-stale", "--expected-revision", "1",
		"--source", "worker", "--observed-at", observedAt, "--host-id", "host", "--boot-id", "boot",
	})
	if stale.code != 1 || !strings.Contains(stale.stdout, "revision conflict") {
		t.Fatalf("stale observation command = (%d, %q)", stale.code, stale.stdout)
	}

	wrongCaller := runCommand(t, []string{
		"task", "execution", "observe", "external-cli", execution.ID,
		"--caller-id", "other", "--operation-id", "observation-1", "--expected-revision", "2",
		"--source", "worker", "--observed-at", observedAt, "--host-id", "host", "--boot-id", "boot",
	})
	if wrongCaller.code != 1 || !strings.Contains(wrongCaller.stdout, "another caller") {
		t.Fatalf("wrong caller observation command = (%d, %q)", wrongCaller.code, wrongCaller.stdout)
	}

	staleCompletion := runCommand(t, []string{
		"task", "execution", "finish", "external-cli", execution.ID,
		"--caller-id", "caller", "--operation-id", "completion-stale", "--expected-revision", "1",
		"--contract", "external-v1", "--result", "succeeded",
	})
	if staleCompletion.code != 1 || !strings.Contains(staleCompletion.stdout, "revision conflict") {
		t.Fatalf("stale completion command = (%d, %q)", staleCompletion.code, staleCompletion.stdout)
	}

	wrongCallerCompletion := runCommand(t, []string{
		"task", "execution", "finish", "external-cli", execution.ID,
		"--caller-id", "other", "--operation-id", "completion-other", "--expected-revision", "2",
		"--contract", "external-v1", "--result", "succeeded",
	})
	if wrongCallerCompletion.code != 1 || !strings.Contains(wrongCallerCompletion.stdout, "another caller") {
		t.Fatalf("wrong caller completion command = (%d, %q)", wrongCallerCompletion.code, wrongCallerCompletion.stdout)
	}

	finished := runCommand(t, []string{
		"task", "execution", "finish", "external-cli", execution.ID,
		"--caller-id", "caller", "--operation-id", "completion-1", "--expected-revision", "2",
		"--contract", "external-v1", "--result", "succeeded",
	})
	if finished.code != 0 || !strings.Contains(finished.stdout, "status: finished") || !strings.Contains(finished.stdout, "result: succeeded") {
		t.Fatalf("completion command = (%d, %q)", finished.code, finished.stdout)
	}

	duplicate := runCommand(t, []string{
		"task", "execution", "finish", "external-cli", execution.ID,
		"--caller-id", "caller", "--operation-id", "completion-1", "--expected-revision", "2",
		"--contract", "external-v1", "--result", "succeeded",
	})
	if duplicate.code != 0 || !strings.Contains(duplicate.stdout, "revision: 3") {
		t.Fatalf("duplicate completion command = (%d, %q)", duplicate.code, duplicate.stdout)
	}

	changedOperation := runCommand(t, []string{
		"task", "execution", "finish", "external-cli", execution.ID,
		"--caller-id", "caller", "--operation-id", "completion-1", "--expected-revision", "3",
		"--contract", "external-v1", "--result", "failed",
	})
	if changedOperation.code != 1 || !strings.Contains(changedOperation.stdout, "already used with different inputs") {
		t.Fatalf("changed completion operation = (%d, %q)", changedOperation.code, changedOperation.stdout)
	}

	lateObservation := runCommand(t, []string{
		"task", "execution", "observe", "external-cli", execution.ID,
		"--caller-id", "caller", "--operation-id", "observation-late", "--expected-revision", "3",
		"--source", "worker", "--observed-at", observedAt, "--host-id", "host", "--boot-id", "boot",
	})
	if lateObservation.code != 1 || !strings.Contains(lateObservation.stdout, "terminal and immutable") {
		t.Fatalf("late observation command = (%d, %q)", lateObservation.code, lateObservation.stdout)
	}
}

func TestTaskFinishPreservesTerminalResultAndLegacyReadability(t *testing.T) {
	setupTaskCommandTest(t)
	if result := runCommand(t, []string{"task", "create", "--task-id", "finish-immutable", "--title", "finish"}); result.code != 0 {
		t.Fatalf("task create = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "finish", "finish-immutable", "succeeded", "first"}); result.code != 0 {
		t.Fatalf("first finish = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "finish", "finish-immutable", "succeeded", "first"}); result.code != 0 {
		t.Fatalf("same finish retry = (%d, %q)", result.code, result.stdout)
	}
	changed := runCommand(t, []string{"task", "finish", "finish-immutable", "failed", "second"})
	if changed.code != 1 || !strings.Contains(changed.stdout, "terminal and immutable") {
		t.Fatalf("changed finish = (%d, %q)", changed.code, changed.stdout)
	}

	state, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	if err := state.WriteManifest("legacy-readable", store.Manifest{Title: "legacy", Worker: "local", Lifecycle: "finished", Condition: "succeeded", Result: "historical"}); err != nil {
		t.Fatal(err)
	}
	legacy := runCommand(t, []string{"task", "inspect", "legacy-readable"})
	if legacy.code != 0 || !strings.Contains(legacy.stdout, "result: historical") {
		t.Fatalf("legacy inspection = (%d, %q)", legacy.code, legacy.stdout)
	}
}

func TestExternalLifecycleCommandsHaveNoHostSubprocessEffects(t *testing.T) {
	setupTaskCommandTest(t)
	execution := createExternalExecutionForCLI(t, "external-canary")
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
	observedAt := time.Now().UTC().Format(time.RFC3339Nano)
	commands := [][]string{
		{"worker", "inspect"},
		{"task", "execution", "observe", "external-canary", execution.ID, "--caller-id", "caller", "--operation-id", "observe", "--expected-revision", "1", "--source", "worker", "--observed-at", observedAt, "--host-id", "host", "--boot-id", "boot"},
		{"task", "execution", "finish", "external-canary", execution.ID, "--caller-id", "caller", "--operation-id", "finish", "--expected-revision", "2", "--contract", "external-v1", "--result", "done"},
		{"task", "execution", "inspect", "external-canary", execution.ID},
	}
	for _, command := range commands {
		if result := runCommand(t, command); result.code != 0 {
			t.Fatalf("Run(%q) = (%d, %q)", command, result.code, result.stdout)
		}
	}
	if data, err := os.ReadFile(logPath); err == nil {
		t.Fatalf("external lifecycle commands invoked host tools: %q", data)
	} else if !os.IsNotExist(err) {
		t.Fatalf("read host-tool canary log: %v", err)
	}
}
