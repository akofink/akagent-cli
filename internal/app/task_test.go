package app

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akofink/akagent-cli/internal/credential"
	"github.com/akofink/akagent-cli/internal/store"
)

func TestParseLaunchRejectsEmptyPolicyOverrides(t *testing.T) {
	for _, flag := range []string{"--provider", "--model", "--thinking"} {
		t.Run(flag, func(t *testing.T) {
			if _, ok := parseLaunch([]string{"--target", "pi", flag, ""}); ok {
				t.Fatalf("parseLaunch accepted an empty %s override", flag)
			}
		})
	}
}

func TestTaskLifecycleCommandContract(t *testing.T) {
	setupTaskCommandTest(t)
	repositoryPath := filepath.Join(t.TempDir(), "repository")
	if err := os.Mkdir(repositoryPath, 0o700); err != nil {
		t.Fatal(err)
	}

	registerOutput := runCommand(t, []string{"repository", "register", "demo", repositoryPath, "--policy", "direct"})
	wantRepository := fmt.Sprintf("repository:\n  name: demo\n  path: %s\n  policy: direct\n", repositoryPath)
	if registerOutput.code != 0 || registerOutput.stdout != wantRepository {
		t.Fatalf("repository register = (%d, %q), want (0, %q)", registerOutput.code, registerOutput.stdout, wantRepository)
	}
	if repeated := runCommand(t, []string{"repository", "register", "demo", repositoryPath, "--policy", "direct"}); repeated.code != 0 || repeated.stdout != wantRepository {
		t.Fatalf("idempotent repository register = (%d, %q), want (0, %q)", repeated.code, repeated.stdout, wantRepository)
	}

	const taskID = "task-14"
	createArgs := []string{"task", "create", "--task-id", taskID, "--title", "Build feature", "--repository", "demo"}
	createOutput := runCommand(t, createArgs)
	wantCreated := fmt.Sprintf("task:\n  id: task-14\n  title: Build feature\n  status: created\n  worker: local\n  branch: main\n  base_revision: \"0000000000000000000000000000000000000001\"\n  worktree_path: %s\n  condition: none\n  committed: true\n  dirty: false\n  untracked: false\n", repositoryPath)
	if createOutput.code != 0 || createOutput.stdout != wantCreated {
		t.Fatalf("task create = (%d, %q), want (0, %q)", createOutput.code, createOutput.stdout, wantCreated)
	}
	if repeated := runCommand(t, createArgs); repeated.code != 0 || repeated.stdout != wantCreated {
		t.Fatalf("idempotent task create = (%d, %q), want (0, %q)", repeated.code, repeated.stdout, wantCreated)
	}

	wantList := fmt.Sprintf("tasks[1]{id,title,status,worker,branch,base_revision,worktree_path,condition,committed,dirty,untracked}:\n  task-14,Build feature,created,local,main,\"0000000000000000000000000000000000000001\",%s,none,true,false,false\ntotal: 1\n", repositoryPath)
	if listed := runCommand(t, []string{"task", "list"}); listed.code != 0 || listed.stdout != wantList {
		t.Fatalf("task list = (%d, %q), want (0, %q)", listed.code, listed.stdout, wantList)
	}
	wantInspected := wantCreated + fmt.Sprintf("resources[1]{id,repository,branch,base_revision,worktree_path,head,committed,dirty,untracked}:\n  legacy,demo,main,\"0000000000000000000000000000000000000001\",%s,\"0000000000000000000000000000000000000001\",true,false,false\n", repositoryPath)
	if inspected := runCommand(t, []string{"task", "inspect", taskID}); inspected.code != 0 || inspected.stdout != wantInspected {
		t.Fatalf("task inspect = (%d, %q), want (0, %q)", inspected.code, inspected.stdout, wantInspected)
	}

	published := runCommand(t, []string{"task", "publish", taskID, "--condition", "active", "--reason", "coding", "--activity", "tests"})
	wantPublished := fmt.Sprintf("task:\n  id: task-14\n  title: Build feature\n  status: created\n  worker: local\n  branch: main\n  base_revision: \"0000000000000000000000000000000000000001\"\n  worktree_path: %s\n  condition: active\n  reason: coding\n  activity: tests\n  committed: true\n  dirty: false\n  untracked: false\n", repositoryPath)
	if published.code != 0 || published.stdout != wantPublished {
		t.Fatalf("task publish = (%d, %q), want (0, %q)", published.code, published.stdout, wantPublished)
	}
	if repeated := runCommand(t, []string{"task", "publish", taskID, "--condition", "active", "--reason", "coding", "--activity", "tests"}); repeated.code != 0 || repeated.stdout != wantPublished {
		t.Fatalf("idempotent task publish = (%d, %q), want (0, %q)", repeated.code, repeated.stdout, wantPublished)
	}

	stopped := runCommand(t, []string{"task", "stop", taskID})
	wantStopped := fmt.Sprintf("task:\n  id: task-14\n  title: Build feature\n  status: stopped\n  worker: local\n  branch: main\n  base_revision: \"0000000000000000000000000000000000000001\"\n  worktree_path: %s\n  condition: none\n  reason: coding\n  activity: tests\n  committed: true\n  dirty: false\n  untracked: false\n", repositoryPath)
	if stopped.code != 0 || stopped.stdout != wantStopped {
		t.Fatalf("task stop = (%d, %q), want (0, %q)", stopped.code, stopped.stdout, wantStopped)
	}
	if repeated := runCommand(t, []string{"task", "stop", taskID}); repeated.code != 0 || repeated.stdout != wantStopped {
		t.Fatalf("idempotent task stop = (%d, %q), want (0, %q)", repeated.code, repeated.stdout, wantStopped)
	}

	finished := runCommand(t, []string{"task", "finish", taskID, "succeeded", "done"})
	wantFinished := fmt.Sprintf("task:\n  id: task-14\n  title: Build feature\n  status: finished\n  worker: local\n  branch: main\n  base_revision: \"0000000000000000000000000000000000000001\"\n  worktree_path: %s\n  condition: none\n  reason: coding\n  activity: tests\n  result: done\n  committed: true\n  dirty: false\n  untracked: false\n", repositoryPath)
	if finished.code != 0 || finished.stdout != wantFinished {
		t.Fatalf("task finish = (%d, %q), want (0, %q)", finished.code, finished.stdout, wantFinished)
	}
	if repeated := runCommand(t, []string{"task", "finish", taskID, "succeeded", "done"}); repeated.code != 0 || repeated.stdout != wantFinished {
		t.Fatalf("idempotent task finish = (%d, %q), want (0, %q)", repeated.code, repeated.stdout, wantFinished)
	}
}

func TestTaskStartRejectsWithMigrationGuidanceWithoutMutatingState(t *testing.T) {
	setupTaskCommandTest(t)
	if result := runCommand(t, []string{"task", "create", "--task-id", "existing-65", "--title", "Existing"}); result.code != 0 {
		t.Fatalf("task create = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "execution", "create", "existing-65", "--execution-id", "execution-65", "--target", "shell", "--command", "/bin/sh"}); result.code != 0 {
		t.Fatalf("execution create = (%d, %q)", result.code, result.stdout)
	}
	result := runCommand(t, []string{"task", "start", "--task-id", "existing-65", "--title", "Legacy", "--repository", "demo"})
	if result.code != 2 || !strings.Contains(result.stdout, "category: usage") || !strings.Contains(result.stdout, "shortcut was removed") || !strings.Contains(result.stdout, "task create") || !strings.Contains(result.stdout, "task launch") {
		t.Fatalf("task start = (%d, %q), want structured migration guidance", result.code, result.stdout)
	}
	inspected := runCommand(t, []string{"task", "inspect", "existing-65"})
	if inspected.code != 0 || !strings.Contains(inspected.stdout, "title: Existing") {
		t.Fatalf("task start changed an existing task = (%d, %q)", inspected.code, inspected.stdout)
	}
	executions := runCommand(t, []string{"task", "execution", "list", "existing-65"})
	if executions.code != 0 || !strings.Contains(executions.stdout, "execution-65") {
		t.Fatalf("task start changed an existing execution = (%d, %q)", executions.code, executions.stdout)
	}
}

func TestTaskCreateHasNoTmuxSideEffectUntilExplicitLaunch(t *testing.T) {
	setupTaskCommandTest(t)
	repositoryPath := filepath.Join(t.TempDir(), "repository")
	if err := os.Mkdir(repositoryPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if result := runCommand(t, []string{"repository", "register", "demo", repositoryPath, "--policy", "direct"}); result.code != 0 {
		t.Fatalf("repository register = (%d, %q)", result.code, result.stdout)
	}
	created := runCommand(t, []string{"task", "create", "--task-id", "create-56", "--title", "Create only", "--repository", "demo"})
	if created.code != 0 || !strings.Contains(created.stdout, "status: created") {
		t.Fatalf("task create = (%d, %q), want created status", created.code, created.stdout)
	}
	if _, err := os.Stat(os.Getenv("AKAGENT_FAKE_TMUX_STATE")); !os.IsNotExist(err) {
		t.Fatalf("task create touched fake tmux state: %v", err)
	}
	launched := runCommand(t, []string{"task", "launch", "create-56", "--target", "shell", "--label", "create-only"})
	if launched.code != 0 || !strings.Contains(launched.stdout, "execution: shell") {
		t.Fatalf("task launch = (%d, %q), want shell execution", launched.code, launched.stdout)
	}
	if _, err := os.Stat(os.Getenv("AKAGENT_FAKE_TMUX_STATE")); err != nil {
		t.Fatalf("task launch did not create fake tmux state: %v", err)
	}
}

func TestCompatibilityLaunchUsesDescriptiveBranchLabel(t *testing.T) {
	setupTaskCommandTest(t)
	repositoryPath := filepath.Join(t.TempDir(), "repository")
	if err := os.Mkdir(repositoryPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if result := runCommand(t, []string{"repository", "register", "demo", repositoryPath, "--policy", "direct"}); result.code != 0 {
		t.Fatalf("repository register = (%d, %q)", result.code, result.stdout)
	}
	for _, taskID := range []string{"shell-label", "pi-label"} {
		created := runCommand(t, []string{"task", "create", "--task-id", taskID, "--title", taskID, "--repository", "demo"})
		if created.code != 0 {
			t.Fatalf("task create %s = (%d, %q)", taskID, created.code, created.stdout)
		}
	}
	if launched := runCommand(t, []string{"task", "launch", "shell-label", "--target", "shell"}); launched.code != 0 {
		t.Fatalf("shell launch = (%d, %q)", launched.code, launched.stdout)
	}
	piPath := filepath.Join(strings.Split(os.Getenv("PATH"), string(os.PathListSeparator))[0], "pi")
	if err := os.WriteFile(piPath, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if launched := runCommand(t, []string{"task", "launch", "pi-label", "--target", "pi"}); launched.code != 0 {
		t.Fatalf("Pi launch = (%d, %q)", launched.code, launched.stdout)
	}
	log, err := os.ReadFile(os.Getenv("AKAGENT_FAKE_TMUX_LOG"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(log); !strings.Contains(got, "-n main") {
		t.Fatalf("tmux launch log = %q, want descriptive branch label", got)
	}
	for _, test := range []struct {
		taskID string
		target string
	}{
		{taskID: "shell-label", target: "shell"},
		{taskID: "pi-label", target: "pi"},
	} {
		executions := runCommand(t, []string{"task", "execution", "list", test.taskID})
		if executions.code != 0 || !strings.Contains(executions.stdout, ",main,"+test.target+",") {
			t.Fatalf("execution list %s = (%d, %q), want descriptive label", test.taskID, executions.code, executions.stdout)
		}
	}
}

func TestTaskResourcesCanBeCreatedAndListedIndependently(t *testing.T) {
	setupTaskCommandTest(t)
	repositoryPath := filepath.Join(t.TempDir(), "repository")
	if err := os.Mkdir(repositoryPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if result := runCommand(t, []string{"repository", "register", "demo", repositoryPath, "--policy", "direct"}); result.code != 0 {
		t.Fatalf("repository register = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "create", "--task-id", "resource-task", "--title", "Multiple resources"}); result.code != 0 || !strings.Contains(result.stdout, "status: created") {
		t.Fatalf("task create = (%d, %q)", result.code, result.stdout)
	}
	for _, id := range []string{"one", "two"} {
		result := runCommand(t, []string{"task", "resource", "create", "resource-task", "--resource-id", id, "--repository", "demo"})
		if result.code != 0 || !strings.Contains(result.stdout, "id: "+id) {
			t.Fatalf("resource create %s = (%d, %q)", id, result.code, result.stdout)
		}
	}
	listed := runCommand(t, []string{"task", "resource", "list", "resource-task"})
	if listed.code != 0 || !strings.Contains(listed.stdout, "resources[2]") || !strings.Contains(listed.stdout, "one") || !strings.Contains(listed.stdout, "two") {
		t.Fatalf("resource list = (%d, %q), want two resources", listed.code, listed.stdout)
	}
}

func TestWorktreeTaskCreateRequiresDescriptiveBranch(t *testing.T) {
	setupTaskCommandTest(t)
	repositoryPath := filepath.Join(t.TempDir(), "repository")
	if err := os.Mkdir(repositoryPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if result := runCommand(t, []string{"repository", "register", "demo", repositoryPath, "--policy", "worktree"}); result.code != 0 {
		t.Fatalf("repository register = (%d, %q)", result.code, result.stdout)
	}
	result := runCommand(t, []string{"task", "create", "--task-id", "missing-branch", "--title", "Missing branch", "--repository", "demo"})
	if result.code != 2 || !strings.Contains(result.stdout, "category: usage") || !strings.Contains(result.stdout, "explicit descriptive --branch") {
		t.Fatalf("worktree task create = (%d, %q), want explicit branch usage error", result.code, result.stdout)
	}
}

func TestTaskListEmptyCommandContract(t *testing.T) {
	setupTaskCommandTest(t)

	result := runCommand(t, []string{"task", "list"})
	want := "tasks: []\ntotal: 0\n"
	if result.code != 0 || result.stdout != want {
		t.Fatalf("empty task list = (%d, %q), want (0, %q)", result.code, result.stdout, want)
	}
}

func TestTaskLookupSupportsUUIDAndTitleOrBranchKeywords(t *testing.T) {
	setupTaskCommandTest(t)
	const uuidTaskID = "019fe8f2-ac67-7406-a6e6-2717b2cd31c6"
	for _, task := range []struct {
		id, title string
	}{
		{id: uuidTaskID, title: "Deploy API"},
		{id: "branch-task", title: "Unrelated task"},
		{id: "id-only-task", title: "No keyword match"},
	} {
		if result := runCommand(t, []string{"task", "create", "--task-id", task.id, "--title", task.title}); result.code != 0 {
			t.Fatalf("task create %s = (%d, %q)", task.id, result.code, result.stdout)
		}
	}

	state, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.UpdateManifest("branch-task", func(manifest *store.Manifest) error {
		manifest.Branch = "akofink/keyword-branch"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteResource(uuidTaskID, store.Resource{ID: "resource-branch", Repository: "demo", Branch: "akofink/resource-keyword"}); err != nil {
		t.Fatal(err)
	}

	for _, keyword := range []string{uuidTaskID, "Deploy", "keyword-branch", "resource-keyword"} {
		result := runCommand(t, []string{"task", "inspect", keyword})
		if result.code != 0 || !strings.Contains(result.stdout, "task:") {
			t.Fatalf("task inspect %q = (%d, %q), want one match", keyword, result.code, result.stdout)
		}
	}

	byTitle := runCommand(t, []string{"task", "list", "Deploy"})
	if byTitle.code != 0 || !strings.Contains(byTitle.stdout, "total: 1") || !strings.Contains(byTitle.stdout, uuidTaskID) {
		t.Fatalf("task list by title = (%d, %q), want title-task only", byTitle.code, byTitle.stdout)
	}
	byBranch := runCommand(t, []string{"task", "list", "keyword-branch"})
	if byBranch.code != 0 || !strings.Contains(byBranch.stdout, "total: 1") || !strings.Contains(byBranch.stdout, "branch-task") {
		t.Fatalf("task list by branch = (%d, %q), want branch-task only", byBranch.code, byBranch.stdout)
	}
	byResourceBranch := runCommand(t, []string{"task", "list", "resource-keyword"})
	if byResourceBranch.code != 0 || !strings.Contains(byResourceBranch.stdout, "total: 1") || !strings.Contains(byResourceBranch.stdout, uuidTaskID) {
		t.Fatalf("task list by resource branch = (%d, %q), want resource task only", byResourceBranch.code, byResourceBranch.stdout)
	}
	idOnly := runCommand(t, []string{"task", "list", "id-only-task"})
	if idOnly.code != 0 || idOnly.stdout != "tasks: []\ntotal: 0\n" {
		t.Fatalf("task list by ID = (%d, %q), want no keyword match", idOnly.code, idOnly.stdout)
	}

	if result := runCommand(t, []string{"task", "create", "--task-id", "second-title-task", "--title", "Deploy worker"}); result.code != 0 {
		t.Fatalf("second task create = (%d, %q)", result.code, result.stdout)
	}
	ambiguous := runCommand(t, []string{"task", "inspect", "Deploy"})
	if ambiguous.code != 1 || !strings.Contains(ambiguous.stdout, "category: conflict") || !strings.Contains(ambiguous.stdout, "matched multiple tasks") || !strings.Contains(ambiguous.stdout, uuidTaskID) || !strings.Contains(ambiguous.stdout, "second-title-task") {
		t.Fatalf("ambiguous task inspect = (%d, %q), want structured conflict", ambiguous.code, ambiguous.stdout)
	}
	missing := runCommand(t, []string{"task", "inspect", "not-present"})
	if missing.code != 1 || !strings.Contains(missing.stdout, "category: not_found") || !strings.Contains(missing.stdout, "No tasks matched keyword") {
		t.Fatalf("missing task keyword inspect = (%d, %q), want structured not found", missing.code, missing.stdout)
	}
}

func TestTaskListHeterogeneousRowsCommandContract(t *testing.T) {
	setupTaskCommandTest(t)
	repositoryPath := filepath.Join(t.TempDir(), "repository")
	if err := os.Mkdir(repositoryPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if result := runCommand(t, []string{"repository", "register", "demo", repositoryPath, "--policy", "direct"}); result.code != 0 {
		t.Fatalf("repository register = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "create", "--task-id", "a-14", "--title", "Stopped", "--repository", "demo"}); result.code != 0 {
		t.Fatalf("stopped task create = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "stop", "a-14"}); result.code != 0 {
		t.Fatalf("stopped task stop = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "create", "--task-id", "b-14", "--title", "Finished", "--repository", "demo"}); result.code != 0 {
		t.Fatalf("finished task create = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "publish", "b-14", "--condition", "active", "--reason", "coding", "--activity", "tests"}); result.code != 0 {
		t.Fatalf("finished task publish = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "stop", "b-14"}); result.code != 0 {
		t.Fatalf("finished task stop = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "finish", "b-14", "succeeded", "done"}); result.code != 0 {
		t.Fatalf("finished task finish = (%d, %q)", result.code, result.stdout)
	}

	result := runCommand(t, []string{"task", "list"})
	want := fmt.Sprintf("tasks[2]{id,title,status,worker,branch,base_revision,worktree_path,condition,reason,activity,result,committed,dirty,untracked}:\n  a-14,Stopped,stopped,local,main,\"0000000000000000000000000000000000000001\",%s,none,null,null,null,true,false,false\n  b-14,Finished,finished,local,main,\"0000000000000000000000000000000000000001\",%s,none,coding,tests,done,true,false,false\ntotal: 2\n", repositoryPath, repositoryPath)
	if result.code != 0 || result.stdout != want {
		t.Fatalf("heterogeneous task list = (%d, %q), want (0, %q)", result.code, result.stdout, want)
	}
}

func TestTaskListFiltersArchivedHistoryAndComposesScopes(t *testing.T) {
	setupTaskCommandTest(t)
	alphaPath := filepath.Join(t.TempDir(), "alpha")
	betaPath := filepath.Join(t.TempDir(), "beta")
	for _, path := range []string{alphaPath, betaPath} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for name, path := range map[string]string{"alpha": alphaPath, "beta": betaPath} {
		if result := runCommand(t, []string{"repository", "register", name, path, "--policy", "worktree"}); result.code != 0 {
			t.Fatalf("repository %s register = (%d, %q)", name, result.code, result.stdout)
		}
	}
	worktreeRoots := map[string]string{
		"alpha": filepath.Join(filepath.Dir(alphaPath), ".akagent", "worktrees", "alpha"),
		"beta":  filepath.Join(filepath.Dir(betaPath), ".akagent", "worktrees", "beta"),
	}
	alphaWorktreeRoot := worktreeRoots["alpha"]
	if result := runCommand(t, []string{"task", "create", "--task-id", "alpha-history", "--title", "Archived", "--repository", "alpha", "--branch", "main", "--worktree", filepath.Join(alphaWorktreeRoot, "alpha-history")}); result.code != 0 {
		t.Fatalf("archived task create = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "stop", "alpha-history"}); result.code != 0 {
		t.Fatalf("archived task stop = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "clean", "alpha-history", "--allow-committed", "--allow-dirty", "--allow-untracked", "--allow-worktree", "--allow-credentials"}); result.code != 0 {
		t.Fatalf("archived task clean = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "create", "--task-id", "alpha-pending", "--title", "Pending cleanup", "--repository", "alpha", "--branch", "main", "--worktree", filepath.Join(alphaWorktreeRoot, "alpha-pending")}); result.code != 0 {
		t.Fatalf("pending task create = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "stop", "alpha-pending"}); result.code != 0 {
		t.Fatalf("pending task stop = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "archive", "alpha-pending"}); result.code != 0 {
		t.Fatalf("pending task archive = (%d, %q)", result.code, result.stdout)
	}
	for _, task := range []struct {
		id, title, repository string
	}{
		{id: "alpha-active", title: "Alpha", repository: "alpha"},
		{id: "beta-active", title: "Beta", repository: "beta"},
	} {
		if result := runCommand(t, []string{"task", "create", "--task-id", task.id, "--title", task.title, "--repository", task.repository, "--branch", "main", "--worktree", filepath.Join(worktreeRoots[task.repository], task.id)}); result.code != 0 {
			t.Fatalf("%s task create = (%d, %q)", task.id, result.code, result.stdout)
		}
	}

	state, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := state.ReadManifest("alpha-active")
	if err != nil {
		t.Fatal(err)
	}
	alphaActive, err := envelope.DecodeManifest()
	if err != nil {
		t.Fatal(err)
	}

	defaultList := runCommand(t, []string{"task", "list"})
	if defaultList.code != 0 || !strings.Contains(defaultList.stdout, "total: 2") || !strings.Contains(defaultList.stdout, "alpha-active") || strings.Contains(defaultList.stdout, "alpha-pending") || !strings.Contains(defaultList.stdout, "beta-active") || strings.Contains(defaultList.stdout, "alpha-history") {
		t.Fatalf("default task list = (%d, %q), want only actionable tasks", defaultList.code, defaultList.stdout)
	}
	allAlpha := runCommand(t, []string{"task", "list", "--all", "--repository", "alpha"})
	if allAlpha.code != 0 || !strings.Contains(allAlpha.stdout, "total: 3") || !strings.Contains(allAlpha.stdout, "alpha-history") || !strings.Contains(allAlpha.stdout, "alpha-pending") || !strings.Contains(allAlpha.stdout, "alpha-active") || strings.Contains(allAlpha.stdout, "beta-active") {
		t.Fatalf("repository-filtered task list = (%d, %q), want all alpha tasks", allAlpha.code, allAlpha.stdout)
	}
	scoped := runCommand(t, []string{"task", "list", "--all", "--repository", "alpha", "--worktree", alphaActive.WorktreePath})
	if scoped.code != 0 || !strings.Contains(scoped.stdout, "total: 1") || !strings.Contains(scoped.stdout, "alpha-active") || strings.Contains(scoped.stdout, "alpha-history") || strings.Contains(scoped.stdout, "beta-active") {
		t.Fatalf("composed task filters = (%d, %q), want one alpha worktree task", scoped.code, scoped.stdout)
	}
	resourceIDs, err := state.ResourceIDs("alpha-history")
	if err != nil {
		t.Fatal(err)
	}
	if len(resourceIDs) != 1 {
		t.Fatalf("alpha-history resource IDs = %v, want one resource", resourceIDs)
	}
	if _, err := state.UpdateResource("alpha-history", resourceIDs[0], func(resource *store.Resource) error {
		resource.RecoveryDebt = "launch_failed"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	debtList := runCommand(t, []string{"task", "list"})
	if debtList.code != 0 || !strings.Contains(debtList.stdout, "total: 3") || !strings.Contains(debtList.stdout, "alpha-history") {
		t.Fatalf("resource debt task list = (%d, %q), want archived task retained", debtList.code, debtList.stdout)
	}
}

func TestTaskListHidesArchivedTasksWithAbsentCleanupState(t *testing.T) {
	setupTaskCommandTest(t)
	for index := 0; index < 10; index++ {
		taskID := fmt.Sprintf("archived-%02d", index)
		if result := runCommand(t, []string{"task", "create", "--task-id", taskID, "--title", taskID}); result.code != 0 {
			t.Fatalf("task create %s = (%d, %q)", taskID, result.code, result.stdout)
		}
		if result := runCommand(t, []string{"task", "stop", taskID}); result.code != 0 {
			t.Fatalf("task stop %s = (%d, %q)", taskID, result.code, result.stdout)
		}
		if result := runCommand(t, []string{"task", "archive", taskID}); result.code != 0 {
			t.Fatalf("task archive %s = (%d, %q)", taskID, result.code, result.stdout)
		}
	}
	if result := runCommand(t, []string{"task", "create", "--task-id", "active-110", "--title", "Active task"}); result.code != 0 {
		t.Fatalf("active task create = (%d, %q)", result.code, result.stdout)
	}

	defaultList := runCommand(t, []string{"task", "list"})
	if defaultList.code != 0 || !strings.Contains(defaultList.stdout, "total: 1") || !strings.Contains(defaultList.stdout, "active-110") {
		t.Fatalf("default task list = (%d, %q), want only the active task", defaultList.code, defaultList.stdout)
	}
	for index := 0; index < 10; index++ {
		if strings.Contains(defaultList.stdout, fmt.Sprintf("archived-%02d", index)) {
			t.Fatalf("default task list = (%d, %q), unexpectedly included archived task", defaultList.code, defaultList.stdout)
		}
	}

	all := runCommand(t, []string{"task", "list", "--all"})
	if all.code != 0 || !strings.Contains(all.stdout, "total: 11") {
		t.Fatalf("all task list = (%d, %q), want all 11 tasks", all.code, all.stdout)
	}
}

func TestTaskListHidesArchivedResourcesWithAbsentCleanupState(t *testing.T) {
	setupTaskCommandTest(t)
	repositoryPath := filepath.Join(t.TempDir(), "repository")
	if err := os.Mkdir(repositoryPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if result := runCommand(t, []string{"repository", "register", "demo", repositoryPath, "--policy", "direct"}); result.code != 0 {
		t.Fatalf("repository register = (%d, %q)", result.code, result.stdout)
	}
	const taskID = "archived-resource-110"
	if result := runCommand(t, []string{"task", "create", "--task-id", taskID, "--title", "Archived resource"}); result.code != 0 {
		t.Fatalf("task create = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "resource", "create", taskID, "--resource-id", "resource", "--repository", "demo"}); result.code != 0 {
		t.Fatalf("resource create = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "stop", taskID}); result.code != 0 {
		t.Fatalf("task stop = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "resource", "archive", taskID, "resource"}); result.code != 0 {
		t.Fatalf("resource archive = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "archive", taskID}); result.code != 0 {
		t.Fatalf("task archive = (%d, %q)", result.code, result.stdout)
	}

	defaultList := runCommand(t, []string{"task", "list"})
	if defaultList.code != 0 || defaultList.stdout != "tasks: []\ntotal: 0\n" {
		t.Fatalf("default task list = (%d, %q), want no actionable tasks", defaultList.code, defaultList.stdout)
	}
	all := runCommand(t, []string{"task", "list", "--all"})
	if all.code != 0 || !strings.Contains(all.stdout, taskID) || !strings.Contains(all.stdout, "total: 1") {
		t.Fatalf("all task list = (%d, %q), want archived resource task", all.code, all.stdout)
	}
}

func TestTaskListHidesTaskArchiveWithUnrecordedResourceArchive(t *testing.T) {
	setupTaskCommandTest(t)
	repositoryPath := filepath.Join(t.TempDir(), "repository")
	if err := os.Mkdir(repositoryPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if result := runCommand(t, []string{"repository", "register", "demo", repositoryPath, "--policy", "direct"}); result.code != 0 {
		t.Fatalf("repository register = (%d, %q)", result.code, result.stdout)
	}
	const taskID = "task-archive-resource-112"
	if result := runCommand(t, []string{"task", "create", "--task-id", taskID, "--title", "Archived task resource"}); result.code != 0 {
		t.Fatalf("task create = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "resource", "create", taskID, "--resource-id", "resource", "--repository", "demo"}); result.code != 0 {
		t.Fatalf("resource create = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "stop", taskID}); result.code != 0 {
		t.Fatalf("task stop = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "archive", taskID}); result.code != 0 {
		t.Fatalf("task archive = (%d, %q)", result.code, result.stdout)
	}

	defaultList := runCommand(t, []string{"task", "list"})
	if defaultList.code != 0 || defaultList.stdout != "tasks: []\ntotal: 0\n" {
		t.Fatalf("default task list = (%d, %q), want archived task hidden", defaultList.code, defaultList.stdout)
	}
	all := runCommand(t, []string{"task", "list", "--all"})
	if all.code != 0 || !strings.Contains(all.stdout, taskID) || !strings.Contains(all.stdout, "total: 1") {
		t.Fatalf("all task list = (%d, %q), want archived task", all.code, all.stdout)
	}

	state, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.UpdateResource(taskID, "resource", func(resource *store.Resource) error {
		resource.CleanupState = "partial"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	pending := runCommand(t, []string{"task", "list"})
	if pending.code != 0 || !strings.Contains(pending.stdout, taskID) || !strings.Contains(pending.stdout, "total: 1") {
		t.Fatalf("pending resource cleanup task list = (%d, %q), want archived task retained", pending.code, pending.stdout)
	}
}

func TestResourceCleanupClearsResolvedDebtFromDefaultTaskList(t *testing.T) {
	setupTaskCommandTest(t)
	repositoryPath := filepath.Join(t.TempDir(), "repository")
	if err := os.Mkdir(repositoryPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if result := runCommand(t, []string{"repository", "register", "demo", repositoryPath, "--policy", "worktree"}); result.code != 0 {
		t.Fatalf("repository register = (%d, %q)", result.code, result.stdout)
	}
	const taskID = "cleanup-debt-114"
	if result := runCommand(t, []string{"task", "create", "--task-id", taskID, "--title", "Cleanup debt", "--repository", "demo", "--branch", "main"}); result.code != 0 {
		t.Fatalf("task create = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "stop", taskID}); result.code != 0 {
		t.Fatalf("task stop = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "archive", taskID}); result.code != 0 {
		t.Fatalf("task archive = (%d, %q)", result.code, result.stdout)
	}

	state, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	resourceIDs, err := state.ResourceIDs(taskID)
	if err != nil || len(resourceIDs) != 1 {
		t.Fatalf("resource IDs = %v, %v; want one resource", resourceIDs, err)
	}
	if _, err := state.UpdateResource(taskID, resourceIDs[0], func(resource *store.Resource) error {
		resource.Git.Committed = false
		resource.Git.Dirty = true
		resource.Git.Untracked = true
		resource.RecoveryDebt = "uncommitted_work"
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	cleaned := runCommand(t, []string{"task", "resource", "clean", taskID, resourceIDs[0], "--allow-dirty", "--allow-untracked", "--allow-worktree", "--allow-credentials"})
	if cleaned.code != 0 || !strings.Contains(cleaned.stdout, "cleanup_state: complete") {
		t.Fatalf("resource cleanup = (%d, %q), want completed cleanup", cleaned.code, cleaned.stdout)
	}
	defaultList := runCommand(t, []string{"task", "list"})
	if defaultList.code != 0 || defaultList.stdout != "tasks: []\ntotal: 0\n" {
		t.Fatalf("default task list after resolved cleanup = (%d, %q), want no actionable tasks", defaultList.code, defaultList.stdout)
	}
	resource, err := state.ReadResource(taskID, resourceIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	if resource.RecoveryDebt != "" {
		t.Fatalf("cleaned resource recovery debt = %q, want empty", resource.RecoveryDebt)
	}
}

func TestTaskListRetainsIndependentCleanupDebt(t *testing.T) {
	setupTaskCommandTest(t)
	const taskID = "cleanup-debt-104"
	if result := runCommand(t, []string{"task", "create", "--task-id", taskID, "--title", "Cleanup debt"}); result.code != 0 {
		t.Fatalf("task create = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "stop", taskID}); result.code != 0 {
		t.Fatalf("task stop = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "archive", taskID}); result.code != 0 {
		t.Fatalf("task archive = (%d, %q)", result.code, result.stdout)
	}
	state, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.UpdateManifest(taskID, func(manifest *store.Manifest) error {
		manifest.CleanupState = "complete"
		manifest.WorktreeCleanupState = "partial"
		manifest.CredentialCleanupState = "complete"
		manifest.CleanupDebt = false
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	result := runCommand(t, []string{"task", "list"})
	if result.code != 0 || !strings.Contains(result.stdout, taskID) || !strings.Contains(result.stdout, "total: 1") {
		t.Fatalf("independent cleanup debt list = (%d, %q), want task retained", result.code, result.stdout)
	}
	all := runCommand(t, []string{"task", "list", "--all"})
	if all.code != 0 || !strings.Contains(all.stdout, taskID) || !strings.Contains(all.stdout, "total: 1") {
		t.Fatalf("all task list = (%d, %q), want historical task", all.code, all.stdout)
	}
}

func TestCredentialCleanupCommandContract(t *testing.T) {
	setupTaskCommandTest(t)
	const taskID = "credential-86"
	if result := runCommand(t, []string{"task", "create", "--task-id", taskID, "--title", "Credential cleanup"}); result.code != 0 {
		t.Fatalf("task create = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "stop", taskID}); result.code != 0 {
		t.Fatalf("task stop = (%d, %q)", result.code, result.stdout)
	}
	blocked := runCommand(t, []string{"credential", "clean", taskID})
	if blocked.code != 1 || !strings.Contains(blocked.stdout, "category: preservation_required") || !strings.Contains(blocked.stdout, "--allow-credentials") {
		t.Fatalf("unapproved credential cleanup = (%d, %q), want structured approval error", blocked.code, blocked.stdout)
	}
	approved := runCommand(t, []string{"task", "credential", "clean", taskID, "--allow-credentials"})
	if approved.code != 0 || !strings.Contains(approved.stdout, "credential_cleanup_state: complete") {
		t.Fatalf("approved credential cleanup = (%d, %q), want completed credential state", approved.code, approved.stdout)
	}
}

func TestApprovedWorktreeCleanupCommandContract(t *testing.T) {
	setupTaskCommandTest(t)
	repositoryPath := filepath.Join(t.TempDir(), "repository")
	if err := os.Mkdir(repositoryPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if result := runCommand(t, []string{"repository", "register", "demo", repositoryPath, "--policy", "worktree"}); result.code != 0 {
		t.Fatalf("repository register = (%d, %q)", result.code, result.stdout)
	}
	const taskID = "cleanup-14"
	if result := runCommand(t, []string{"task", "create", "--task-id", taskID, "--title", "Cleanup", "--repository", "demo", "--branch", "main"}); result.code != 0 {
		t.Fatalf("task create = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "stop", taskID}); result.code != 0 {
		t.Fatalf("task stop = (%d, %q)", result.code, result.stdout)
	}
	blocked := runCommand(t, []string{"task", "clean", taskID})
	if blocked.code != 1 || !strings.Contains(blocked.stdout, "category: preservation_required") || !strings.Contains(blocked.stdout, "--allow-credentials") {
		t.Fatalf("unapproved cleanup = (%d, %q), want structured approval error", blocked.code, blocked.stdout)
	}
	approved := runCommand(t, []string{"task", "clean", taskID, "--allow-worktree", "--allow-credentials"})
	if approved.code != 0 || !strings.Contains(approved.stdout, "worktree_cleanup_state: complete") {
		t.Fatalf("approved cleanup = (%d, %q), want completed cleanup state", approved.code, approved.stdout)
	}
}

func TestTaskReconcileCommandContract(t *testing.T) {
	setupTaskCommandTest(t)
	repositoryPath := filepath.Join(t.TempDir(), "repository")
	if err := os.Mkdir(repositoryPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if result := runCommand(t, []string{"repository", "register", "demo", repositoryPath, "--policy", "direct"}); result.code != 0 {
		t.Fatalf("repository register = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "create", "--task-id", "reconcile-14", "--title", "Reconcile", "--repository", "demo"}); result.code != 0 {
		t.Fatalf("task create = (%d, %q)", result.code, result.stdout)
	}
	if err := os.WriteFile(os.Getenv("AKAGENT_FAKE_TMUX_STATE"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	result := runCommand(t, []string{"task", "reconcile"})
	want := fmt.Sprintf("tasks[1]{id,title,status,worker,branch,base_revision,worktree_path,condition,committed,dirty,untracked}:\n  reconcile-14,Reconcile,created,local,main,\"0000000000000000000000000000000000000001\",%s,none,true,false,false\ntotal: 1\n", repositoryPath)
	if result.code != 0 || result.stdout != want {
		t.Fatalf("task reconcile = (%d, %q), want (0, %q)", result.code, result.stdout, want)
	}
	if result := runCommand(t, []string{"task", "create", "--task-id", "reconcile-other", "--title", "Other"}); result.code != 0 {
		t.Fatalf("other task create = (%d, %q)", result.code, result.stdout)
	}
	result = runCommand(t, []string{"task", "reconcile", "reconcile-14"})
	if result.code != 0 || result.stdout != want {
		t.Fatalf("scoped task reconcile = (%d, %q), want (0, %q)", result.code, result.stdout, want)
	}
}

func TestSerializationFailureHasStructuredCategory(t *testing.T) {
	var stdout bytes.Buffer
	value := map[string]any{"items": []any{map[string]any{"id": "one"}, []any{"nested"}}}

	if code := write(&stdout, value); code != 1 {
		t.Fatalf("write() exit = %d, want 1", code)
	}
	want := "error:\n  category: internal\n  message: Failed to serialize protocol output\n  retryable: false\n  recovery: Retry the command\n"
	if stdout.String() != want {
		t.Fatalf("serialization failure = %q, want %q", stdout.String(), want)
	}
}

func TestTaskMissingResourceHasStructuredCategory(t *testing.T) {
	setupTaskCommandTest(t)
	result := runCommand(t, []string{"task", "inspect", "missing-14"})
	want := "error:\n  category: not_found\n  message: No tasks matched keyword missing-14\n  retryable: false\n  recovery: Inspect the task state and retry\n"
	if result.code != 1 || result.stdout != want {
		t.Fatalf("missing task inspect = (%d, %q), want (1, %q)", result.code, result.stdout, want)
	}
}

func TestRepositoryRegistrationConflictHasStructuredCategory(t *testing.T) {
	setupTaskCommandTest(t)
	first := filepath.Join(t.TempDir(), "first")
	second := filepath.Join(t.TempDir(), "second")
	if err := os.Mkdir(first, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(second, 0o700); err != nil {
		t.Fatal(err)
	}
	if result := runCommand(t, []string{"repository", "register", "demo", first, "--policy", "direct"}); result.code != 0 {
		t.Fatalf("initial registration = (%d, %q)", result.code, result.stdout)
	}
	result := runCommand(t, []string{"repository", "register", "demo", second, "--policy", "direct"})
	want := "error:\n  category: conflict\n  message: repository registration conflicts with existing demo\n  retryable: false\n  recovery: Inspect the task state and retry\n"
	if result.code != 1 || result.stdout != want {
		t.Fatalf("conflicting registration = (%d, %q), want (1, %q)", result.code, result.stdout, want)
	}
}

func TestTaskUnknownFlagsFailBeforeLifecycleSideEffects(t *testing.T) {
	setupTaskCommandTest(t)
	result := runCommand(t, []string{"task", "create", "--title", "No side effects", "--repository", "demo", "--bogus", "value"})
	if result.code != 2 || !strings.HasPrefix(result.stdout, "error:\n  category: usage\n") {
		t.Fatalf("unknown task flag = (%d, %q), want usage error", result.code, result.stdout)
	}
	if _, err := os.Stat(os.Getenv("AKAGENT_FAKE_TMUX_STATE")); !os.IsNotExist(err) {
		t.Fatalf("unknown task flag touched fake tmux state: stat error = %v", err)
	}
	stateRoot := filepath.Join(os.Getenv("XDG_STATE_HOME"), "akagent", "tasks")
	entries, err := os.ReadDir(stateRoot)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("unknown task flag created task records: %v", entries)
	}
}

func TestLifecycleMalformedFlagsReturnUsage(t *testing.T) {
	setupTaskCommandTest(t)
	cases := [][]string{
		{"repository", "register", "demo", t.TempDir(), "--bogus", "value"},
		{"task", "list", "--bogus"},
		{"task", "inspect", "task-14", "--bogus"},
		{"task", "attach"},
		{"task", "attach", "task-14", "--bogus"},
		{"task", "publish", "task-14", "--condition", "active", "--bogus", "value"},
		{"task", "finish", "task-14", "succeeded"},
		{"task", "clean", "task-14", "--bogus"},
		{"task", "stop", "task-14", "--bogus"},
		{"task", "reconcile", "--bogus"},
	}
	for _, args := range cases {
		result := runCommand(t, args)
		if result.code != 2 || !strings.Contains(result.stdout, "category: usage") {
			t.Errorf("Run(%q) = (%d, %q), want usage error", args, result.code, result.stdout)
		}
	}
}

func TestTaskCapabilityFailureAndOptionalWarningRedactCredentialValues(t *testing.T) {
	setupTaskCommandTest(t)
	secret := "runtime-secret-value-14"
	t.Setenv("AKAGENT_TEST_SECRET", secret)
	manifestPath := filepath.Join(t.TempDir(), "credentials.toon")
	manifest := "version: 1\ncredentials[2]{id,type,source,required_for}:\n  required,token,env:AKAGENT_MISSING,task\n  optional,token,env:AKAGENT_OPTIONAL,\n"
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(credential.ConfigEnv, manifestPath)
	repositoryPath := filepath.Join(t.TempDir(), "repository")
	if err := os.Mkdir(repositoryPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if result := runCommand(t, []string{"repository", "register", "demo", repositoryPath, "--policy", "direct"}); result.code != 0 {
		t.Fatalf("repository register = (%d, %q)", result.code, result.stdout)
	}

	missing := runCommand(t, []string{"task", "create", "--task-id", "capability-14", "--title", "Needs capability", "--repository", "demo", "--require", "required"})
	if missing.code != 1 || !strings.Contains(missing.stdout, "category: capability") || !strings.Contains(missing.stdout, "required credential required is unavailable") {
		t.Fatalf("required capability failure did not return the expected capability error")
	}
	assertDoesNotContainCredentialValue(t, missing.stdout, secret)

	optional := runCommand(t, []string{"task", "create", "--task-id", "optional-14", "--title", "Optional capability", "--repository", "demo", "--optional", "optional"})
	if optional.code != 0 || !strings.Contains(optional.stdout, "warnings: optional credential optional is unavailable") {
		t.Fatalf("optional capability warning did not return the expected successful warning")
	}
	assertDoesNotContainCredentialValue(t, optional.stdout, secret)
}

func TestTaskRetryableStoreErrorIsStructured(t *testing.T) {
	setupTaskCommandTest(t)
	repositoryPath := filepath.Join(t.TempDir(), "repository")
	if err := os.Mkdir(repositoryPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if result := runCommand(t, []string{"repository", "register", "demo", repositoryPath, "--policy", "direct"}); result.code != 0 {
		t.Fatalf("repository register = (%d, %q)", result.code, result.stdout)
	}
	const taskID = "locked-14"
	if result := runCommand(t, []string{"task", "create", "--task-id", taskID, "--title", "Locked task", "--repository", "demo"}); result.code != 0 {
		t.Fatalf("task create = (%d, %q)", result.code, result.stdout)
	}

	state, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	release, err := state.Lock(taskID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = release() }()

	result := runCommand(t, []string{"task", "publish", taskID, "--condition", "waiting"})
	want := "error:\n  category: retryable\n  message: Task locked-14 state is locked by another writer\n  retryable: true\n  recovery: Inspect the task state and retry\n"
	if result.code != 1 || result.stdout != want {
		t.Fatalf("locked task publish = (%d, %q), want (1, %q)", result.code, result.stdout, want)
	}
}

func TestExecutionEvidenceCommandReportsReferenceAvailabilityWithoutContent(t *testing.T) {
	setupTaskCommandTest(t)
	if result := runCommand(t, []string{"task", "create", "--task-id", "evidence-task", "--title", "Evidence task"}); result.code != 0 {
		t.Fatalf("task create = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "execution", "create", "evidence-task", "--execution-id", "evidence-exec", "--target", "pi", "--command", "pi"}); result.code != 0 {
		t.Fatalf("execution create = (%d, %q)", result.code, result.stdout)
	}
	referencePath := filepath.Join(t.TempDir(), "session.jsonl")
	secretContent := "SECRET_PROMPT_CONTENT_SHOULD_NOT_APPEAR"
	if err := os.WriteFile(referencePath, []byte(secretContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if result := runCommand(t, []string{"task", "execution", "session", "add", "evidence-task", "evidence-exec", "--tool", "pi", "--session-id", "pi-session", "--reference-path", referencePath}); result.code != 0 {
		t.Fatalf("session add = (%d, %q)", result.code, result.stdout)
	}
	if err := os.Remove(referencePath); err != nil {
		t.Fatal(err)
	}

	listed := runCommand(t, []string{"task", "execution", "evidence", "list", "evidence-task", "evidence-exec"})
	for _, expected := range []string{"evidence:", "state: recorded", "reason: session_references_recorded", "captures[1]", "provider_session_id,state,evidence_class", "pi-session", "unavailable", "total: 1"} {
		if listed.code != 0 || !strings.Contains(listed.stdout, expected) {
			t.Fatalf("evidence list = (%d, %q), want %q", listed.code, listed.stdout, expected)
		}
	}
	if strings.Contains(listed.stdout, secretContent) {
		t.Fatalf("evidence list exposed provider content: %q", listed.stdout)
	}
	captureID := ""
	for _, line := range strings.Split(listed.stdout, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ref-") {
			captureID = strings.Split(line, ",")[0]
			break
		}
	}
	if captureID == "" {
		t.Fatalf("evidence list did not expose capture ID: %q", listed.stdout)
	}
	inspected := runCommand(t, []string{"task", "execution", "evidence", "inspect", "evidence-task", "evidence-exec", captureID})
	for _, expected := range []string{"evidence:", "capture_id: " + captureID, "artifact_reference: " + referencePath, "artifact_state: unavailable", "redaction_policy: metadata_only", "retention_class: metadata", "error_category: artifact_missing"} {
		if inspected.code != 0 || !strings.Contains(inspected.stdout, expected) {
			t.Fatalf("evidence inspect = (%d, %q), want %q", inspected.code, inspected.stdout, expected)
		}
	}
	if strings.Contains(inspected.stdout, secretContent) {
		t.Fatalf("evidence inspect exposed provider content: %q", inspected.stdout)
	}
}

func TestExecutionEvidenceCommandReportsNoSessionReferences(t *testing.T) {
	setupTaskCommandTest(t)
	if result := runCommand(t, []string{"task", "create", "--task-id", "no-evidence-task", "--title", "No evidence task"}); result.code != 0 {
		t.Fatalf("task create = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "execution", "create", "no-evidence-task", "--execution-id", "no-evidence-exec", "--target", "shell", "--command", "/bin/sh"}); result.code != 0 {
		t.Fatalf("execution create = (%d, %q)", result.code, result.stdout)
	}

	listed := runCommand(t, []string{"task", "execution", "evidence", "list", "no-evidence-task", "no-evidence-exec"})
	want := "evidence:\n  task_id: no-evidence-task\n  execution_id: no-evidence-exec\n  state: unavailable\n  evidence_class: unavailable\n  reason: no_session_references\ncaptures: []\ntotal: 0\n"
	if listed.code != 0 || listed.stdout != want {
		t.Fatalf("empty evidence list = (%d, %q), want (0, %q)", listed.code, listed.stdout, want)
	}
}

func TestMalformedCredentialManifestRedactsCommandError(t *testing.T) {
	setupTaskCommandTest(t)
	secret := "malformed-runtime-secret-14"
	manifestPath := filepath.Join(t.TempDir(), "credentials.toon")
	content := "credentials[1]{id,type,source,required_for}:\n  \"" + secret + "\n"
	if err := os.WriteFile(manifestPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(credential.ConfigEnv, manifestPath)

	result := runCommand(t, []string{"credential", "list"})
	if result.code != 1 || !strings.Contains(result.stdout, "category: capability") {
		t.Fatalf("malformed credential manifest did not return the expected capability error")
	}
	assertDoesNotContainCredentialValue(t, result.stdout, secret)
}

type commandResult struct {
	code   int
	stdout string
}

func runCommand(t *testing.T, args []string) commandResult {
	t.Helper()
	var stdout bytes.Buffer
	return commandResult{code: Run(args, &stdout), stdout: stdout.String()}
}

func assertDoesNotContainCredentialValue(t *testing.T, output, secret string) {
	t.Helper()
	if strings.Contains(output, secret) {
		t.Fatalf("command output contained a credential value")
	}
}

func setupTaskCommandTest(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("AKAGENT_FAKE_TMUX_STATE", filepath.Join(root, "tmux-state"))
	t.Setenv("AKAGENT_FAKE_TMUX_LOG", filepath.Join(root, "tmux.log"))
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	fakeTmux := filepath.Join(bin, "tmux")
	const script = `#!/bin/sh
set -eu
state="${AKAGENT_FAKE_TMUX_STATE:?}"
touch "$state"
case "${1:-}" in
list-windows)
  cat "$state"
  ;;
new-window)
  printf '%s\n' "$*" >> "${AKAGENT_FAKE_TMUX_LOG:-/dev/null}"
  count=$(wc -l < "$state" | tr -d ' ')
  window="@$((count + 1))"
  printf '%s\t\n' "$window" >> "$state"
  printf '%s\n' "$window"
  ;;
set-option)
  target="$4"
  value="$6"
  tmp="$state.tmp"
  awk -F '\t' -v target="$target" -v value="$value" 'BEGIN { OFS="\t" } { if ($1 == target) print $1, value; else print }' "$state" > "$tmp"
  mv "$tmp" "$state"
  ;;
kill-window)
  target="$3"
  tmp="$state.tmp"
  awk -F '\t' -v target="$target" '$1 != target' "$state" > "$tmp"
  mv "$tmp" "$state"
  ;;
*)
  exit 2
  ;;
esac
`
	if err := os.WriteFile(fakeTmux, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	fakeGit := filepath.Join(bin, "git")
	fakeGitScript := `#!/bin/sh
set -eu
path="$2"
shift 2
case "${1:-}" in
rev-parse)
  case "${2:-}" in
  --is-inside-work-tree) printf 'true\n' ;;
  --show-toplevel) printf '%s\n' "$path" ;;
  --git-common-dir) printf '/fake/common\n' ;;
  HEAD) printf '0000000000000000000000000000000000000001\n' ;;
  *) printf '0000000000000000000000000000000000000001\n' ;;
  esac
  ;;
branch) printf 'main\n' ;;
status) printf '## main\n' ;;
worktree) ;;
merge-base) exit 0 ;;
*) exit 2 ;;
esac
`
	if err := os.WriteFile(fakeGit, []byte(fakeGitScript), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}
