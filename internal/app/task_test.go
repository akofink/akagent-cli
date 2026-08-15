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
	startArgs := []string{"task", "start", "--task-id", taskID, "--title", "Build feature", "--repository", "demo"}
	startOutput := runCommand(t, startArgs)
	wantStarted := fmt.Sprintf("task:\n  id: task-14\n  title: Build feature\n  status: running\n  worker: local\n  branch: main\n  base_revision: \"0000000000000000000000000000000000000001\"\n  worktree_path: %s\n  condition: none\n  committed: true\n  dirty: false\n  untracked: false\n", repositoryPath)
	if startOutput.code != 0 || startOutput.stdout != wantStarted {
		t.Fatalf("task start = (%d, %q), want (0, %q)", startOutput.code, startOutput.stdout, wantStarted)
	}
	if repeated := runCommand(t, startArgs); repeated.code != 0 || repeated.stdout != wantStarted {
		t.Fatalf("idempotent task start = (%d, %q), want (0, %q)", repeated.code, repeated.stdout, wantStarted)
	}

	wantList := fmt.Sprintf("tasks[1]{id,title,status,worker,branch,base_revision,worktree_path,condition,committed,dirty,untracked}:\n  task-14,Build feature,running,local,main,\"0000000000000000000000000000000000000001\",%s,none,true,false,false\ntotal: 1\n", repositoryPath)
	if listed := runCommand(t, []string{"task", "list"}); listed.code != 0 || listed.stdout != wantList {
		t.Fatalf("task list = (%d, %q), want (0, %q)", listed.code, listed.stdout, wantList)
	}
	if inspected := runCommand(t, []string{"task", "inspect", taskID}); inspected.code != 0 || inspected.stdout != wantStarted {
		t.Fatalf("task inspect = (%d, %q), want (0, %q)", inspected.code, inspected.stdout, wantStarted)
	}

	published := runCommand(t, []string{"task", "publish", taskID, "--condition", "active", "--reason", "coding", "--activity", "tests"})
	wantPublished := fmt.Sprintf("task:\n  id: task-14\n  title: Build feature\n  status: active\n  worker: local\n  branch: main\n  base_revision: \"0000000000000000000000000000000000000001\"\n  worktree_path: %s\n  condition: active\n  reason: coding\n  activity: tests\n  committed: true\n  dirty: false\n  untracked: false\n", repositoryPath)
	if published.code != 0 || published.stdout != wantPublished {
		t.Fatalf("task publish = (%d, %q), want (0, %q)", published.code, published.stdout, wantPublished)
	}
	if repeated := runCommand(t, []string{"task", "publish", taskID, "--condition", "active", "--reason", "coding", "--activity", "tests"}); repeated.code != 0 || repeated.stdout != wantPublished {
		t.Fatalf("idempotent task publish = (%d, %q), want (0, %q)", repeated.code, repeated.stdout, wantPublished)
	}

	if finishing := runCommand(t, []string{"task", "finish", taskID, "succeeded", "done"}); finishing.code != 1 || !strings.Contains(finishing.stdout, "category: internal") || !strings.Contains(finishing.stdout, "task process is still running") {
		t.Fatalf("finish while running = (%d, %q), want internal error", finishing.code, finishing.stdout)
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

func TestTaskReconcileCommandContract(t *testing.T) {
	setupTaskCommandTest(t)
	repositoryPath := filepath.Join(t.TempDir(), "repository")
	if err := os.Mkdir(repositoryPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if result := runCommand(t, []string{"repository", "register", "demo", repositoryPath, "--policy", "direct"}); result.code != 0 {
		t.Fatalf("repository register = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "start", "--task-id", "reconcile-14", "--title", "Reconcile", "--repository", "demo"}); result.code != 0 {
		t.Fatalf("task start = (%d, %q)", result.code, result.stdout)
	}
	if err := os.WriteFile(os.Getenv("AKAGENT_FAKE_TMUX_STATE"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	result := runCommand(t, []string{"task", "reconcile"})
	want := fmt.Sprintf("tasks[1]{id,title,status,worker,branch,base_revision,worktree_path,condition,committed,dirty,untracked}:\n  reconcile-14,Reconcile,stopped,local,main,\"0000000000000000000000000000000000000001\",%s,none,true,false,false\ntotal: 1\n", repositoryPath)
	if result.code != 0 || result.stdout != want {
		t.Fatalf("task reconcile = (%d, %q), want (0, %q)", result.code, result.stdout, want)
	}
}

func TestTaskMissingResourceHasStructuredCategory(t *testing.T) {
	setupTaskCommandTest(t)
	result := runCommand(t, []string{"task", "inspect", "missing-14"})
	want := "error:\n  category: not_found\n  message: No tasks found for task ID missing-14\n  retryable: false\n  recovery: Inspect the task state and retry\n"
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
	result := runCommand(t, []string{"task", "start", "--title", "No side effects", "--repository", "demo", "--bogus", "value"})
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

	missing := runCommand(t, []string{"task", "start", "--task-id", "capability-14", "--title", "Needs capability", "--repository", "demo", "--require", "required"})
	if missing.code != 1 || !strings.Contains(missing.stdout, "category: capability") || !strings.Contains(missing.stdout, "required credential required is unavailable") {
		t.Fatalf("required capability failure did not return the expected capability error")
	}
	assertDoesNotContainCredentialValue(t, missing.stdout, secret)

	optional := runCommand(t, []string{"task", "start", "--task-id", "optional-14", "--title", "Optional capability", "--repository", "demo", "--optional", "optional"})
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
	if result := runCommand(t, []string{"task", "start", "--task-id", taskID, "--title", "Locked task", "--repository", "demo"}); result.code != 0 {
		t.Fatalf("task start = (%d, %q)", result.code, result.stdout)
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
