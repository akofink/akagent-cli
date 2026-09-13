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

func TestExternalRecordCreationCompletionAndArchiveCommands(t *testing.T) {
	setupTaskCommandTest(t)
	malformed := runCommand(t, []string{"task", "external", "create", "malformed", "--caller-id", "caller", "--operation-id", "create"})
	if malformed.code != 2 {
		t.Fatalf("malformed external task create = (%d, %q)", malformed.code, malformed.stdout)
	}
	state, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	ids, err := state.TaskIDs()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Fatalf("malformed external task create left records: %v", ids)
	}

	created := runCommand(t, []string{"task", "external", "create", "external-flow", "--title", "External flow", "--caller-id", "caller", "--operation-id", "task-create"})
	if created.code != 0 || !strings.Contains(created.stdout, "provenance: external") || !strings.Contains(created.stdout, "revision: 1") {
		t.Fatalf("external task create = (%d, %q)", created.code, created.stdout)
	}
	retry := runCommand(t, []string{"task", "external", "create", "external-flow", "--title", "External flow", "--caller-id", "caller", "--operation-id", "task-create"})
	if retry.code != 0 || !strings.Contains(retry.stdout, "revision: 1") {
		t.Fatalf("external task create retry = (%d, %q)", retry.code, retry.stdout)
	}
	resource := runCommand(t, []string{"task", "external", "resource", "create", "external-flow", "--resource-id", "external-resource", "--repository", "offline", "--branch", "external/main", "--base", "base", "--head", "head", "--worktree", "/offline/missing", "--caller-id", "caller", "--operation-id", "resource-create"})
	if resource.code != 0 || !strings.Contains(resource.stdout, "provenance: external") {
		t.Fatalf("external resource create = (%d, %q)", resource.code, resource.stdout)
	}
	execution := runCommand(t, []string{"task", "external", "execution", "create", "external-flow", "--execution-id", "external-execution", "--resource", "external-resource", "--caller-id", "caller", "--operation-id", "execution-create"})
	if execution.code != 0 || !strings.Contains(execution.stdout, "provenance: external") {
		t.Fatalf("external execution create = (%d, %q)", execution.code, execution.stdout)
	}
	managed, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lifecycle.New(managed).Create(lifecycle.CreateRequest{ID: "managed-flow", Title: "Managed flow"}); err != nil {
		t.Fatal(err)
	}
	if result := runCommand(t, []string{"task", "execution", "create", "managed-flow", "--execution-id", "managed-execution", "--target", "external"}); result.code != 0 {
		t.Fatalf("managed execution create = (%d, %q)", result.code, result.stdout)
	}
	observedAt := time.Now().UTC().Format(time.RFC3339Nano)
	managedObservation := runCommand(t, []string{"task", "execution", "observe", "managed-flow", "managed-execution", "--caller-id", "caller", "--operation-id", "observe", "--expected-revision", "1", "--source", "worker", "--observed-at", observedAt, "--host-id", "host", "--boot-id", "boot"})
	if managedObservation.code != 1 || !strings.Contains(managedObservation.stdout, "not an externally declared record") {
		t.Fatalf("managed execution external observation = (%d, %q)", managedObservation.code, managedObservation.stdout)
	}

	observed := runCommand(t, []string{"task", "execution", "observe", "external-flow", "external-execution", "--caller-id", "caller", "--operation-id", "observation", "--expected-revision", "1", "--source", "worker", "--observed-at", observedAt, "--host-id", "host", "--boot-id", "boot", "--process-state", "running"})
	if observed.code != 0 || !strings.Contains(observed.stdout, "revision: 2") {
		t.Fatalf("external observation = (%d, %q)", observed.code, observed.stdout)
	}
	stale := runCommand(t, []string{"task", "execution", "finish", "external-flow", "external-execution", "--caller-id", "caller", "--operation-id", "completion-stale", "--expected-revision", "1", "--contract", "external-v1", "--result", "succeeded"})
	if stale.code != 1 || !strings.Contains(stale.stdout, "revision conflict") {
		t.Fatalf("stale external completion = (%d, %q)", stale.code, stale.stdout)
	}
	wrongCaller := runCommand(t, []string{"task", "execution", "finish", "external-flow", "external-execution", "--caller-id", "other", "--operation-id", "completion-other", "--expected-revision", "2", "--contract", "external-v1", "--result", "succeeded"})
	if wrongCaller.code != 1 || !strings.Contains(wrongCaller.stdout, "another caller") {
		t.Fatalf("wrong external completion caller = (%d, %q)", wrongCaller.code, wrongCaller.stdout)
	}
	finished := runCommand(t, []string{"task", "execution", "finish", "external-flow", "external-execution", "--caller-id", "caller", "--operation-id", "completion", "--expected-revision", "2", "--contract", "external-v1", "--result", "succeeded"})
	if finished.code != 0 || !strings.Contains(finished.stdout, "status: finished") {
		t.Fatalf("external completion = (%d, %q)", finished.code, finished.stdout)
	}
	duplicate := runCommand(t, []string{"task", "execution", "finish", "external-flow", "external-execution", "--caller-id", "caller", "--operation-id", "completion", "--expected-revision", "2", "--contract", "external-v1", "--result", "succeeded"})
	if duplicate.code != 0 || !strings.Contains(duplicate.stdout, "revision: 3") {
		t.Fatalf("external completion retry = (%d, %q)", duplicate.code, duplicate.stdout)
	}
	changed := runCommand(t, []string{"task", "execution", "finish", "external-flow", "external-execution", "--caller-id", "caller", "--operation-id", "completion", "--expected-revision", "3", "--contract", "external-v1", "--result", "failed"})
	if changed.code != 1 || !strings.Contains(changed.stdout, "already used with different inputs") {
		t.Fatalf("changed external completion retry = (%d, %q)", changed.code, changed.stdout)
	}
	late := runCommand(t, []string{"task", "execution", "observe", "external-flow", "external-execution", "--caller-id", "caller", "--operation-id", "late-observation", "--expected-revision", "3", "--source", "worker", "--observed-at", observedAt, "--host-id", "host", "--boot-id", "boot"})
	if late.code != 1 || !strings.Contains(late.stdout, "terminal and immutable") {
		t.Fatalf("late external observation = (%d, %q)", late.code, late.stdout)
	}
	executionArchive := runCommand(t, []string{"task", "external", "execution", "archive", "external-flow", "external-execution", "--caller-id", "caller", "--operation-id", "execution-archive", "--expected-revision", "3"})
	if executionArchive.code != 0 || !strings.Contains(executionArchive.stdout, "archive_state: complete") {
		t.Fatalf("external execution archive = (%d, %q)", executionArchive.code, executionArchive.stdout)
	}
	archivedInspect := runCommand(t, []string{"task", "inspect", "external-flow"})
	if archivedInspect.code != 0 || !strings.Contains(archivedInspect.stdout, "external_observations[1]") || !strings.Contains(archivedInspect.stdout, "external_completion:") {
		t.Fatalf("archived task inspect = (%d, %q)", archivedInspect.code, archivedInspect.stdout)
	}
	archivedList := runCommand(t, []string{"task", "execution", "list", "external-flow"})
	if archivedList.code != 0 || !strings.Contains(archivedList.stdout, "external_observations[1]") || !strings.Contains(archivedList.stdout, "external_completion:") {
		t.Fatalf("archived execution list = (%d, %q)", archivedList.code, archivedList.stdout)
	}
	resourceArchive := runCommand(t, []string{"task", "external", "resource", "archive", "external-flow", "external-resource", "--caller-id", "caller", "--operation-id", "resource-archive", "--expected-revision", "1"})
	if resourceArchive.code != 0 || !strings.Contains(resourceArchive.stdout, "archive_state: complete") {
		t.Fatalf("external resource archive = (%d, %q)", resourceArchive.code, resourceArchive.stdout)
	}
	taskFinished := runCommand(t, []string{"task", "external", "finish", "external-flow", "--caller-id", "caller", "--operation-id", "task-complete", "--expected-revision", "3", "--contract", "external-v1", "--result", "succeeded"})
	if taskFinished.code != 0 || !strings.Contains(taskFinished.stdout, "status: finished") {
		t.Fatalf("external task completion = (%d, %q)", taskFinished.code, taskFinished.stdout)
	}
	taskArchive := runCommand(t, []string{"task", "external", "archive", "external-flow", "--caller-id", "caller", "--operation-id", "task-archive", "--expected-revision", "4"})
	if taskArchive.code != 0 || !strings.Contains(taskArchive.stdout, "archive_state: complete") {
		t.Fatalf("external task archive = (%d, %q)", taskArchive.code, taskArchive.stdout)
	}
	fullyArchivedInspect := runCommand(t, []string{"task", "inspect", "external-flow"})
	if fullyArchivedInspect.code != 0 || !strings.Contains(fullyArchivedInspect.stdout, "external_observations[1]") || !strings.Contains(fullyArchivedInspect.stdout, "external_completion:") {
		t.Fatalf("fully archived task inspect = (%d, %q)", fullyArchivedInspect.code, fullyArchivedInspect.stdout)
	}
	fullyArchivedList := runCommand(t, []string{"task", "execution", "list", "external-flow"})
	if fullyArchivedList.code != 0 || !strings.Contains(fullyArchivedList.stdout, "external_observations[1]") || !strings.Contains(fullyArchivedList.stdout, "external_completion:") {
		t.Fatalf("fully archived execution list = (%d, %q)", fullyArchivedList.code, fullyArchivedList.stdout)
	}
	archiveRetry := runCommand(t, []string{"task", "external", "archive", "external-flow", "--caller-id", "caller", "--operation-id", "task-archive", "--expected-revision", "4"})
	if archiveRetry.code != 0 || !strings.Contains(archiveRetry.stdout, "archive_state: complete") {
		t.Fatalf("external task archive retry = (%d, %q)", archiveRetry.code, archiveRetry.stdout)
	}
}

func TestExternalExecutionObservationAndCompletionCommands(t *testing.T) {
	setupTaskCommandTest(t)
	execution := createExternalExecutionForCLI(t, "external-cli")
	observedAt := time.Now().UTC().Format(time.RFC3339Nano)
	malformed := runCommand(t, []string{
		"task", "execution", "observe", "external-cli", execution.ID,
		"--caller-id", "caller", "--operation-id", "observation-malformed", "--expected-revision", "1",
		"--source", "worker", "--observed-at", "not-a-timestamp", "--host-id", "host", "--boot-id", "boot",
	})
	if malformed.code != 2 {
		t.Fatalf("malformed observation command = (%d, %q)", malformed.code, malformed.stdout)
	}

	observed := runCommand(t, []string{
		"task", "execution", "observe", "external-cli", execution.ID,
		"--caller-id", "caller", "--operation-id", "observation-1", "--expected-revision", "1",
		"--source", "worker", "--observed-at", observedAt, "--host-id", "host", "--boot-id", "boot",
		"--process-state", "running", "--detail", "historical",
	})
	if observed.code != 0 || !strings.Contains(observed.stdout, "external_observations[1]") || !strings.Contains(observed.stdout, "revision: 2") {
		t.Fatalf("observation command = (%d, %q)", observed.code, observed.stdout)
	}

	taskInspect := runCommand(t, []string{"task", "inspect", "external-cli"})
	if taskInspect.code != 0 || !strings.Contains(taskInspect.stdout, "external_observations[1]") || !strings.Contains(taskInspect.stdout, "historical") {
		t.Fatalf("observed task inspect = (%d, %q)", taskInspect.code, taskInspect.stdout)
	}
	executionList := runCommand(t, []string{"task", "execution", "list", "external-cli"})
	if executionList.code != 0 || !strings.Contains(executionList.stdout, "external_observations[1]") || !strings.Contains(executionList.stdout, "historical") {
		t.Fatalf("observed execution list = (%d, %q)", executionList.code, executionList.stdout)
	}
	humanInspect := runCommand(t, []string{"task", "inspect", "external-cli", "--format", "human"})
	if humanInspect.code != 0 || !strings.Contains(humanInspect.stdout, "external_observations (1)") || !strings.Contains(humanInspect.stdout, "detail: historical") {
		t.Fatalf("observed human task inspect = (%d, %q)", humanInspect.code, humanInspect.stdout)
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

	terminalInspect := runCommand(t, []string{"task", "inspect", "external-cli"})
	if terminalInspect.code != 0 || !strings.Contains(terminalInspect.stdout, "external_observations[1]") || !strings.Contains(terminalInspect.stdout, "external_completion:") {
		t.Fatalf("terminal task inspect = (%d, %q)", terminalInspect.code, terminalInspect.stdout)
	}
	terminalList := runCommand(t, []string{"task", "execution", "list", "external-cli"})
	if terminalList.code != 0 || !strings.Contains(terminalList.stdout, "external_observations[1]") || !strings.Contains(terminalList.stdout, "external_completion:") {
		t.Fatalf("terminal execution list = (%d, %q)", terminalList.code, terminalList.stdout)
	}
	terminalHuman := runCommand(t, []string{"task", "inspect", "external-cli", "--format", "human"})
	if terminalHuman.code != 0 || !strings.Contains(terminalHuman.stdout, "external_completion_contract: external-v1") || !strings.Contains(terminalHuman.stdout, "detail: historical") {
		t.Fatalf("terminal human task inspect = (%d, %q)", terminalHuman.code, terminalHuman.stdout)
	}
}

func TestExternalRecordCommandsHaveNoHostSubprocessEffects(t *testing.T) {
	setupTaskCommandTest(t)
	root := t.TempDir()
	logPath := filepath.Join(root, "invocations.log")
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"git", "tmux", "pi", "provider", "deploy", "credential"} {
		path := filepath.Join(bin, name)
		script := "#!/bin/sh\\nprintf '%s\\n' '" + name + "' >> '" + logPath + "'\\nexit 99\\n"
		if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	commands := [][]string{
		{"task", "external", "create", "canary-external", "--title", "Canary", "--caller-id", "caller", "--operation-id", "task-create"},
		{"task", "external", "resource", "create", "canary-external", "--resource-id", "resource", "--repository", "offline", "--branch", "main", "--base", "base", "--head", "head", "--worktree", "/offline/missing", "--caller-id", "caller", "--operation-id", "resource-create"},
		{"task", "external", "execution", "create", "canary-external", "--execution-id", "execution", "--resource", "resource", "--caller-id", "caller", "--operation-id", "execution-create"},
	}
	for _, command := range commands {
		if result := runCommand(t, command); result.code != 0 {
			t.Fatalf("Run(%q) = (%d, %q)", command, result.code, result.stdout)
		}
	}
	observedAt := time.Now().UTC().Format(time.RFC3339Nano)
	for _, command := range [][]string{
		{"task", "execution", "observe", "canary-external", "execution", "--caller-id", "caller", "--operation-id", "observe", "--expected-revision", "1", "--source", "worker", "--observed-at", observedAt, "--host-id", "host", "--boot-id", "boot"},
		{"task", "execution", "finish", "canary-external", "execution", "--caller-id", "caller", "--operation-id", "finish", "--expected-revision", "2", "--contract", "external-v1", "--result", "succeeded"},
		{"task", "external", "execution", "archive", "canary-external", "execution", "--caller-id", "caller", "--operation-id", "archive", "--expected-revision", "3"},
	} {
		if result := runCommand(t, command); result.code != 0 {
			t.Fatalf("Run(%q) = (%d, %q)", command, result.code, result.stdout)
		}
	}
	if data, err := os.ReadFile(logPath); err == nil {
		t.Fatalf("external record commands invoked host tools: %q", data)
	} else if !os.IsNotExist(err) {
		t.Fatalf("read host-tool canary log: %v", err)
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
