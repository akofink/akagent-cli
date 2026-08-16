package lifecycle

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/akofink/akagent-cli/internal/credential"
	"github.com/akofink/akagent-cli/internal/store"
)

type fakeTmux struct {
	mu              sync.Mutex
	observation     TmuxObservation
	starts          int
	attaches        []string
	observedIDs     []string
	managedStarts   []store.LaunchConfig
	startBranches   []string
	managedBranches []string
	managedFailure  int
}

func (t *fakeTmux) Start(_ string, branch string) (TmuxProcess, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.starts++
	t.startBranches = append(t.startBranches, branch)
	t.observation = TmuxObservation{Available: true, Processes: []TmuxProcess{{WindowID: "@1", PaneID: "%1", PID: 42, StartTime: 100}}}
	return t.observation.Processes[0], nil
}
func (t *fakeTmux) StartManaged(_ string, branch string, launch store.LaunchConfig) (TmuxProcess, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.managedStarts = append(t.managedStarts, launch)
	t.managedBranches = append(t.managedBranches, branch)
	if t.managedFailure > 0 {
		t.managedFailure--
		return TmuxProcess{}, errors.New("managed launch window failed")
	}
	t.starts++
	t.observation = TmuxObservation{Available: true, Processes: []TmuxProcess{{WindowID: "@managed", PaneID: "%managed", PID: 52, StartTime: 120}}}
	return t.observation.Processes[0], nil
}

func (t *fakeTmux) Observe(id string) (TmuxObservation, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.observedIDs = append(t.observedIDs, id)
	return t.observation, nil
}
func (t *fakeTmux) Attach(_ string, windowID string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.attaches = append(t.attaches, windowID)
	return nil
}
func (t *fakeTmux) Stop(string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.observation = TmuxObservation{Available: true}
	return nil
}

func newTestManager(t *testing.T) (*Manager, *fakeTmux) {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	state, err := store.OpenAt(root)
	if err != nil {
		t.Fatal(err)
	}
	tmux := &fakeTmux{observation: TmuxObservation{Available: true}}
	manager := New(state)
	manager.Tmux = tmux
	manager.Credentials = func() (*credential.Manifest, error) { return &credential.Manifest{Version: 1}, nil }
	repositoryPath := t.TempDir()
	for _, args := range [][]string{{"init"}, {"config", "user.email", "test@example.com"}, {"config", "user.name", "Test"}} {
		if err := runGit(repositoryPath, args...); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(repositoryPath, "README"), []byte("test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runGit(repositoryPath, "add", "README"); err != nil {
		t.Fatal(err)
	}
	if err := runGit(repositoryPath, "commit", "-m", "initial"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.RegisterRepository("demo", repositoryPath, "direct"); err != nil {
		t.Fatal(err)
	}
	return manager, tmux
}

func runGit(path string, args ...string) error {
	return exec.Command("git", append([]string{"-C", path}, args...)...).Run()
}

func TestCreateDoesNotStartTmuxOrInspectExecution(t *testing.T) {
	manager, tmux := newTestManager(t)
	result, err := manager.Create(CreateRequest{ID: "created-1", Title: "Durable task", Repository: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Created || result.Manifest.Lifecycle != "created" {
		t.Fatalf("Create() = %#v, want a created durable task", result)
	}
	if tmux.starts != 0 || len(tmux.observedIDs) != 0 {
		t.Fatalf("Create() touched tmux: starts=%d observations=%v", tmux.starts, tmux.observedIDs)
	}
	events, err := manager.Store.ReadEvents("created-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Event.Operation != "create" {
		t.Fatalf("create events = %#v, want one create event", events)
	}
}

func TestLaunchExecutionStartsExplicitShellAfterCreate(t *testing.T) {
	manager, tmux := newTestManager(t)
	if _, err := manager.Create(CreateRequest{ID: "launch-1", Title: "Explicit shell", Repository: "demo"}); err != nil {
		t.Fatal(err)
	}
	manifest, err := manager.LaunchExecution("launch-1", LaunchRequest{Target: "shell"})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Lifecycle != "running" || manifest.Launch == nil || manifest.Launch.Target != "shell" {
		t.Fatalf("LaunchExecution() = %#v, want running shell execution", manifest)
	}
	if tmux.starts != 1 || len(tmux.managedStarts) != 0 {
		t.Fatalf("launch starts = %d managed=%d, want one direct shell", tmux.starts, len(tmux.managedStarts))
	}
}

func TestCreateMigratesInterruptedLegacyStartWithoutStartingTmux(t *testing.T) {
	manager, tmux := newTestManager(t)
	repository, err := manager.Store.ReadRepository("demo")
	if err != nil {
		t.Fatal(err)
	}
	base, err := manager.Git.Head(repository.Path)
	if err != nil {
		t.Fatal(err)
	}
	branch, err := manager.Git.Branch(repository.Path)
	if err != nil {
		t.Fatal(err)
	}
	legacy := store.Manifest{Title: "Legacy task", Worker: "local", Repository: "demo", Branch: branch, BaseRevision: base, WorktreePath: repository.Path, Lifecycle: "starting", Condition: "none"}
	if err := manager.Store.WriteManifest("legacy-1", legacy); err != nil {
		t.Fatal(err)
	}
	result, err := manager.Create(CreateRequest{ID: "legacy-1", Title: legacy.Title, Repository: legacy.Repository, Branch: legacy.Branch, BaseRevision: legacy.BaseRevision, WorktreePath: legacy.WorktreePath})
	if err != nil {
		t.Fatal(err)
	}
	if result.Manifest.Lifecycle != "created" || tmux.starts != 0 {
		t.Fatalf("migrated task = %#v, tmux starts = %d; want created without launch", result.Manifest, tmux.starts)
	}
	events, err := manager.Store.ReadEvents("legacy-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Event.Operation != "migrate" {
		t.Fatalf("migration events = %#v, want one migration event", events)
	}
}

func TestManagedStartPersistsExplicitLaunchConfiguration(t *testing.T) {
	manager, tmux := newTestManager(t)
	prompt := filepath.Join(t.TempDir(), "prompt.md")
	if err := os.WriteFile(prompt, []byte("full prompt must not be copied to process arguments\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager.ResolveAgent = func(string) (string, error) { return "/usr/local/bin/pi", nil }
	result, err := manager.Start(StartRequest{ID: "managed-1", Title: "Managed Pi", Repository: "demo", Agent: "pi", PromptReference: prompt, WorkingContext: "issue-22"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Manifest.Launch == nil || result.Manifest.Launch.Target != "pi" || result.Manifest.Launch.Command != "/usr/local/bin/pi" || result.Manifest.Launch.PromptReference != prompt || result.Manifest.Launch.WorkingContext != "issue-22" {
		t.Fatalf("launch config = %#v, want explicit durable Pi configuration", result.Manifest.Launch)
	}
	if len(tmux.managedStarts) != 1 || tmux.managedStarts[0].WorkingDirectory != result.Manifest.WorktreePath {
		t.Fatalf("managed starts = %#v, want task worktree launch", tmux.managedStarts)
	}
	if len(tmux.managedBranches) != 1 || tmux.managedBranches[0] != result.Manifest.Branch {
		t.Fatalf("managed branches = %#v, want direct repository branch %q", tmux.managedBranches, result.Manifest.Branch)
	}
	if result.Manifest.ProcessPID != 52 || result.Manifest.ProcessStartTime != 120 {
		t.Fatalf("process identity = %d/%d, want managed process identity", result.Manifest.ProcessPID, result.Manifest.ProcessStartTime)
	}
	if repeated, err := manager.Start(StartRequest{ID: "managed-1", Title: "Managed Pi", Repository: "demo", Agent: "pi", PromptReference: prompt, WorkingContext: "issue-22"}); err != nil || repeated.Created {
		t.Fatalf("repeated managed start = %#v, %v; want idempotent no-op", repeated, err)
	}
}

func TestManagedLaunchUsesInteractivePiWithPromptReference(t *testing.T) {
	manager, _ := newTestManager(t)
	prompt := filepath.Join(t.TempDir(), "prompt with spaces.md")
	promptContents := "prompt contents must not become process arguments\n"
	if err := os.WriteFile(prompt, []byte(promptContents), 0o600); err != nil {
		t.Fatal(err)
	}
	manager.ResolveAgent = func(string) (string, error) { return "/usr/local/bin/pi", nil }
	request := StartRequest{ID: "managed-interactive", Title: "Interactive Pi", Repository: "demo", Agent: "pi", PromptReference: prompt}
	if _, err := manager.Start(request); err != nil {
		t.Fatal(err)
	}

	originalDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(originalDirectory) }()
	var gotCommand string
	var gotArgs []string
	manager.ExecAgent = func(command string, args, _ []string) error {
		gotCommand = command
		gotArgs = append([]string(nil), args...)
		return errors.New("test exec failure")
	}
	if err := manager.Launch(request.ID); err == nil {
		t.Fatal("Launch succeeded despite injected exec failure")
	}
	if gotCommand != "/usr/local/bin/pi" {
		t.Fatalf("agent command = %q, want Pi executable", gotCommand)
	}
	wantArgs := []string{"/usr/local/bin/pi", "@" + prompt}
	if strings.Join(gotArgs, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("agent args = %#v, want %#v", gotArgs, wantArgs)
	}
	if strings.Contains(strings.Join(gotArgs, "\x00"), promptContents) {
		t.Fatal("prompt contents reached agent arguments")
	}
}

func TestManagedLaunchMissingPromptRemainsRecoverable(t *testing.T) {
	manager, _ := newTestManager(t)
	prompt := filepath.Join(t.TempDir(), "prompt.md")
	if err := os.WriteFile(prompt, []byte("prompt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager.ResolveAgent = func(string) (string, error) { return "/usr/local/bin/pi", nil }
	request := StartRequest{ID: "managed-missing-prompt", Title: "Missing prompt", Repository: "demo", Agent: "pi", PromptReference: prompt}
	if _, err := manager.Start(request); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(prompt); err != nil {
		t.Fatal(err)
	}
	if err := manager.Launch(request.ID); err == nil {
		t.Fatal("Launch succeeded with a missing prompt file")
	}
	manifest, err := manager.Inspect(request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Lifecycle != "starting" || manifest.Observation != ObservationMissing || manifest.RecoveryDebt != "launch_failed" {
		t.Fatalf("manifest after missing prompt = %#v, want recoverable launch failure", manifest)
	}
}

func TestManagedLaunchWindowFailureRemainsRetryable(t *testing.T) {
	manager, tmux := newTestManager(t)
	prompt := filepath.Join(t.TempDir(), "prompt.md")
	if err := os.WriteFile(prompt, []byte("prompt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager.ResolveAgent = func(string) (string, error) { return "/usr/local/bin/pi", nil }
	tmux.managedFailure = 1
	request := StartRequest{ID: "managed-retry", Title: "Retry Pi", Repository: "demo", Agent: "pi", PromptReference: prompt}
	if _, err := manager.Start(request); err == nil {
		t.Fatal("managed start succeeded despite tmux launch failure")
	}
	failed, err := manager.Inspect(request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Lifecycle != "starting" || failed.Launch == nil {
		t.Fatalf("failed launch manifest = %#v, want durable starting state", failed)
	}
	if _, err := manager.Start(request); err != nil {
		t.Fatalf("retry managed start = %v", err)
	}
	if len(tmux.managedStarts) != 2 {
		t.Fatalf("managed starts = %d, want failed launch plus retry", len(tmux.managedStarts))
	}
}

func TestStartPersistsProcessIdentityAndIsIdempotent(t *testing.T) {
	manager, tmux := newTestManager(t)
	request := StartRequest{ID: "task-1", Title: "Test lifecycle", Repository: "demo"}
	result, err := manager.Start(request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Manifest.ProcessPID != 42 || result.Manifest.ProcessStartTime != 100 || result.Manifest.Observation != ObservationFresh {
		t.Fatalf("start manifest = %#v, want the verified process identity", result.Manifest)
	}
	if result, err := manager.Start(request); err != nil || result.Created {
		t.Fatalf("second Start() = %#v, %v; want no-op", result, err)
	}
	if tmux.starts != 1 {
		t.Fatalf("tmux starts = %d, want 1", tmux.starts)
	}
	events, err := manager.Store.ReadEvents("task-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
}

func TestAttachUsesOnlyVerifiedTaskWindow(t *testing.T) {
	manager, tmux := newTestManager(t)
	if _, err := manager.Start(StartRequest{ID: "task-attach", Title: "Attach", Repository: "demo"}); err != nil {
		t.Fatal(err)
	}
	before, err := manager.Inspect("task-attach")
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Attach("task-attach"); err != nil {
		t.Fatal(err)
	}
	after, err := manager.Inspect("task-attach")
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("Attach() mutated durable state: before=%#v after=%#v", before, after)
	}
	if len(tmux.attaches) != 1 || tmux.attaches[0] != "@1" {
		t.Fatalf("tmux attaches = %#v, want one verified window", tmux.attaches)
	}
	if len(tmux.observedIDs) != 1 || tmux.observedIDs[0] != "task-attach" {
		t.Fatalf("tmux observed IDs = %#v, want task ID verification", tmux.observedIDs)
	}
}

func TestAttachRejectsUnverifiedTaskStateWithoutMutation(t *testing.T) {
	cases := []struct {
		name   string
		setup  func(*Manager, *fakeTmux) error
		reason string
	}{
		{
			name: "missing window",
			setup: func(_ *Manager, tmux *fakeTmux) error {
				tmux.observation = TmuxObservation{Available: true}
				return nil
			},
			reason: "window is missing",
		},
		{
			name: "contradictory windows",
			setup: func(_ *Manager, tmux *fakeTmux) error {
				tmux.observation.Processes = append(tmux.observation.Processes, TmuxProcess{WindowID: "@2", PaneID: "%2", PID: 43, StartTime: 200})
				return nil
			},
			reason: "observation is contradictory",
		},
		{
			name: "stale heartbeat",
			setup: func(manager *Manager, _ *fakeTmux) error {
				fixed := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
				manager.Now = func() time.Time { return fixed.Add(DefaultHeartbeatTimeout + time.Second) }
				return nil
			},
			reason: "heartbeat is stale",
		},
		{
			name: "stopped task",
			setup: func(manager *Manager, _ *fakeTmux) error {
				_, err := manager.Stop("task-attach")
				return err
			},
			reason: "task is stopped",
		},
		{
			name: "finished task",
			setup: func(manager *Manager, _ *fakeTmux) error {
				if _, err := manager.Stop("task-attach"); err != nil {
					return err
				}
				_, err := manager.Finish("task-attach", "succeeded", "done")
				return err
			},
			reason: "task is finished",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			manager, tmux := newTestManager(t)
			fixed := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
			if test.name == "stale heartbeat" {
				manager.Now = func() time.Time { return fixed }
			}
			if _, err := manager.Start(StartRequest{ID: "task-attach", Title: "Attach", Repository: "demo"}); err != nil {
				t.Fatal(err)
			}
			if test.name == "stale heartbeat" {
				manager.Now = func() time.Time { return fixed.Add(DefaultHeartbeatTimeout + time.Second) }
			}
			if err := test.setup(manager, tmux); err != nil {
				t.Fatal(err)
			}
			before, err := manager.Inspect("task-attach")
			if err != nil {
				t.Fatal(err)
			}
			err = manager.Attach("task-attach")
			if err == nil || !strings.Contains(err.Error(), test.reason) || !store.IsKind(err, store.KindConflict) {
				t.Fatalf("Attach() error = %v, want conflict containing %q", err, test.reason)
			}
			after, inspectErr := manager.Inspect("task-attach")
			if inspectErr != nil {
				t.Fatal(inspectErr)
			}
			if before != after {
				t.Fatalf("failed Attach() mutated durable state: before=%#v after=%#v", before, after)
			}
			if len(tmux.attaches) != 0 {
				t.Fatalf("failed Attach() attached to tmux window: %#v", tmux.attaches)
			}
		})
	}
}

func TestCommandTmuxAttachVerifiesTaskOptionBeforeAttaching(t *testing.T) {
	bin := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "tmux.log")
	tmuxPath := filepath.Join(bin, "tmux")
	const script = `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$AKAGENT_TEST_TMUX_LOG"
case "$1" in
  display-message) printf '%s\n' "$AKAGENT_TEST_TMUX_OPTION" ;;
  attach-session) : ;;
  *) exit 2 ;;
esac
`
	if err := os.WriteFile(tmuxPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AKAGENT_TEST_TMUX_LOG", logPath)
	for _, test := range []struct {
		name     string
		option   string
		wantErr  bool
		wantCall string
	}{
		{name: "matching option", option: "task-attach", wantCall: "display-message -p -t @1 #{@akagent_task_id}"},
		{name: "wrong option", option: "other-task", wantErr: true, wantCall: "display-message -p -t @1 #{@akagent_task_id}"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(logPath, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("AKAGENT_TEST_TMUX_OPTION", test.option)
			err := (commandTmux{}).Attach("task-attach", "@1")
			if (err != nil) != test.wantErr {
				t.Fatalf("Attach() error = %v, wantErr %v", err, test.wantErr)
			}
			content, readErr := os.ReadFile(logPath)
			if readErr != nil {
				t.Fatal(readErr)
			}
			calls := strings.Split(strings.TrimSpace(string(content)), "\n")
			if calls[0] != test.wantCall {
				t.Fatalf("first tmux call = %q, want %q", calls[0], test.wantCall)
			}
			if test.wantErr && len(calls) != 1 {
				t.Fatalf("tmux calls = %q, want no attach after failed verification", calls)
			}
			if !test.wantErr && (len(calls) != 2 || calls[1] != "attach-session -t @1") {
				t.Fatalf("tmux calls = %q, want verified attach", calls)
			}
		})
	}
}

func TestCommandTmuxStartUsesBranchDisplayNameAndTaskMetadata(t *testing.T) {
	bin := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "tmux.log")
	tmuxPath := filepath.Join(bin, "tmux")
	const script = `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$AKAGENT_TEST_TMUX_LOG"
case "$1" in
  new-window) printf '@1\n' ;;
  set-option) : ;;
  list-windows) printf '@1\ttask-51\n' ;;
  list-panes) printf '%s\t%s\t%s\n' '@1' '%1' '0' ;;
  *) exit 2 ;;
esac
`
	if err := os.WriteFile(tmuxPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AKAGENT_TEST_TMUX_LOG", logPath)
	t.Setenv("SHELL", "/bin/sh")
	if _, err := (commandTmux{}).Start("task-51", "akofink/51-task-labels"); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	calls := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(calls) < 2 || calls[0] != "new-window -d -P -F #{window_id} -n 51-task-labels /bin/sh" {
		t.Fatalf("tmux start calls = %q, want descriptive window name", calls)
	}
	if calls[1] != "set-option -w -t @1 @akagent_task_id task-51" {
		t.Fatalf("tmux metadata call = %q, want task ID metadata", calls[1])
	}
}

func TestReconcileRecordsMissingObservationWithoutDeletingTask(t *testing.T) {
	manager, tmux := newTestManager(t)
	if _, err := manager.Start(StartRequest{ID: "task-2", Title: "Recover", Repository: "demo"}); err != nil {
		t.Fatal(err)
	}
	tmux.observation = TmuxObservation{Available: true}
	if _, err := manager.Reconcile(); err != nil {
		t.Fatal(err)
	}
	manifest, err := manager.Inspect("task-2")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Lifecycle != "running" || manifest.Observation != ObservationMissing {
		t.Fatalf("manifest = %#v, want running with missing observation", manifest)
	}
	if got := Status(manifest, time.Now(), DefaultHeartbeatTimeout); got != "unknown" {
		t.Fatalf("Status() = %q, want unknown", got)
	}
	events, err := manager.Store.ReadEvents("task-2")
	if err != nil {
		t.Fatal(err)
	}
	if events[len(events)-1].Event.Outcome != "window_missing" {
		t.Fatalf("last event = %#v", events[len(events)-1])
	}
}

func TestStartAfterReconciliationDoesNotRestartTask(t *testing.T) {
	manager, tmux := newTestManager(t)
	request := StartRequest{ID: "task-4", Title: "No restart", Repository: "demo"}
	if _, err := manager.Start(request); err != nil {
		t.Fatal(err)
	}
	tmux.observation = TmuxObservation{Available: true}
	if _, err := manager.Reconcile(); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Start(request); err != nil {
		t.Fatal(err)
	}
	if tmux.starts != 1 {
		t.Fatalf("tmux starts = %d, want 1", tmux.starts)
	}
}

func TestReconcileRejectsPIDReuseByStartTime(t *testing.T) {
	manager, tmux := newTestManager(t)
	if _, err := manager.Start(StartRequest{ID: "task-7", Title: "PID reuse", Repository: "demo"}); err != nil {
		t.Fatal(err)
	}
	tmux.observation.Processes[0].StartTime = 101
	if _, err := manager.Reconcile(); err != nil {
		t.Fatal(err)
	}
	manifest, err := manager.Inspect("task-7")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Observation != ObservationReplaced || manifest.ProcessStartTime != 100 {
		t.Fatalf("manifest = %#v, want replaced observation and original identity", manifest)
	}
	if got := Status(manifest, time.Now(), DefaultHeartbeatTimeout); got != "unknown" {
		t.Fatalf("Status() = %q, want unknown", got)
	}
}

func TestReconcileRecordsContradictoryObservation(t *testing.T) {
	manager, tmux := newTestManager(t)
	if _, err := manager.Start(StartRequest{ID: "task-8", Title: "Contradiction", Repository: "demo"}); err != nil {
		t.Fatal(err)
	}
	tmux.observation.Processes = append(tmux.observation.Processes, TmuxProcess{WindowID: "@2", PaneID: "%2", PID: 43, StartTime: 200})
	if _, err := manager.Reconcile(); err != nil {
		t.Fatal(err)
	}
	manifest, err := manager.Inspect("task-8")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Observation != ObservationContradictory {
		t.Fatalf("observation = %q, want contradictory", manifest.Observation)
	}
}

func TestStaleHeartbeatIsUnknown(t *testing.T) {
	manager, _ := newTestManager(t)
	now := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
	manager.Now = func() time.Time { return now }
	if _, err := manager.Start(StartRequest{ID: "task-9", Title: "Heartbeat", Repository: "demo"}); err != nil {
		t.Fatal(err)
	}
	manifest, err := manager.Publish("task-9", "waiting", "input", "awaiting input")
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(DefaultHeartbeatTimeout + time.Second)
	if got := Status(manifest, now, DefaultHeartbeatTimeout); got != "unknown" {
		t.Fatalf("Status() before reread = %q, want unknown", got)
	}
	if _, err := manager.Reconcile(); err != nil {
		t.Fatal(err)
	}
	manifest, err = manager.Inspect("task-9")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Observation != ObservationStale {
		t.Fatalf("observation = %q, want stale", manifest.Observation)
	}
	if got := Status(manifest, now, DefaultHeartbeatTimeout); got != "unknown" {
		t.Fatalf("Status() = %q, want unknown", got)
	}
}

func TestConcurrentPublishAndReconcilePreserveValidState(t *testing.T) {
	manager, tmux := newTestManager(t)
	if _, err := manager.Start(StartRequest{ID: "task-10", Title: "Concurrent", Repository: "demo"}); err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	for i := 0; i < 12; i++ {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			if i%2 == 0 {
				_, _ = manager.Publish("task-10", "active", "", "step")
				return
			}
			_, _ = manager.Reconcile()
		}(i)
	}
	group.Wait()
	manifest, err := manager.Inspect("task-10")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Observation != ObservationFresh || manifest.ProcessPID != 42 || tmux.starts != 1 {
		t.Fatalf("manifest = %#v, tmux starts = %d; want serialized valid state", manifest, tmux.starts)
	}
}

func TestFinishRequiresTaskProcessToExit(t *testing.T) {
	manager, tmux := newTestManager(t)
	if _, err := manager.Start(StartRequest{ID: "task-6", Title: "Finish", Repository: "demo"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Finish("task-6", "succeeded", "complete"); err == nil {
		t.Fatal("Finish() succeeded while the task process was still running")
	}
	tmux.observation = TmuxObservation{Available: true}
	manifest, err := manager.Finish("task-6", "succeeded", "complete")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Lifecycle != "finished" || manifest.Observation != ObservationMissing {
		t.Fatalf("manifest = %#v, want finished with missing process", manifest)
	}
}

func TestRegisterRejectsNonGitDirectory(t *testing.T) {
	manager, _ := newTestManager(t)
	if _, err := manager.RegisterRepository("not-git", t.TempDir(), "direct"); err == nil {
		t.Fatal("RegisterRepository() accepted a non-Git directory")
	}
}

func registerWorktreeRepository(t *testing.T, manager *Manager, name string) store.Repository {
	t.Helper()
	repository, err := manager.Store.ReadRepository("demo")
	if err != nil {
		t.Fatal(err)
	}
	repository.Name = name
	repository.Policy = "worktree"
	repository.WorktreeRoot = filepath.Join(t.TempDir(), "worktrees")
	if _, err := manager.Store.RegisterRepository(repository); err != nil {
		t.Fatal(err)
	}
	return repository
}

func TestManagedWorktreeReconcileDoesNotCreateMismatchDebt(t *testing.T) {
	manager, _ := newTestManager(t)
	registerWorktreeRepository(t, manager, "managed-worktree")
	prompt := filepath.Join(t.TempDir(), "prompt.md")
	if err := os.WriteFile(prompt, []byte("prompt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager.ResolveAgent = func(string) (string, error) { return "/usr/local/bin/pi", nil }
	if _, err := manager.Start(StartRequest{ID: "managed-worktree-task", Title: "Managed worktree", Repository: "managed-worktree", Branch: "akofink/managed-worktree", Agent: "pi", PromptReference: prompt}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Reconcile(); err != nil {
		t.Fatal(err)
	}
	manifest, err := manager.Inspect("managed-worktree-task")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(manifest.RecoveryDebt, "worktree_mismatch") {
		t.Fatalf("managed worktree recovery debt = %q, want no mismatch", manifest.RecoveryDebt)
	}
}

func TestShellWorktreeReconcileDoesNotCreateMismatchDebt(t *testing.T) {
	manager, _ := newTestManager(t)
	registerWorktreeRepository(t, manager, "shell-worktree")
	result, err := manager.Start(StartRequest{ID: "shell-worktree-task", Title: "Shell worktree", Repository: "shell-worktree", Branch: "akofink/shell-worktree"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(result.Manifest.WorktreePath, "committed"), []byte("valid\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runGit(result.Manifest.WorktreePath, "add", "committed"); err != nil {
		t.Fatal(err)
	}
	if err := runGit(result.Manifest.WorktreePath, "commit", "-m", "valid task commit"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Reconcile(); err != nil {
		t.Fatal(err)
	}
	manifest, err := manager.Inspect("shell-worktree-task")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(manifest.RecoveryDebt, "worktree_mismatch") {
		t.Fatalf("shell worktree recovery debt = %q, want no mismatch", manifest.RecoveryDebt)
	}
}

func TestReconcilePreservesWorktreeMismatchDetection(t *testing.T) {
	t.Run("wrong path", func(t *testing.T) {
		manager, _ := newTestManager(t)
		registerWorktreeRepository(t, manager, "wrong-path")
		if _, err := manager.Start(StartRequest{ID: "wrong-path-task", Title: "Wrong path", Repository: "wrong-path", Branch: "akofink/wrong-path"}); err != nil {
			t.Fatal(err)
		}
		if _, err := manager.Store.UpdateManifest("wrong-path-task", func(manifest *store.Manifest) error {
			manifest.Repository = "demo"
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := manager.Reconcile(); err != nil {
			t.Fatal(err)
		}
		manifest, err := manager.Inspect("wrong-path-task")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(manifest.RecoveryDebt, "worktree_mismatch") {
			t.Fatalf("recovery debt = %q, want path mismatch", manifest.RecoveryDebt)
		}
	})

	t.Run("wrong branch", func(t *testing.T) {
		manager, _ := newTestManager(t)
		registerWorktreeRepository(t, manager, "wrong-branch")
		result, err := manager.Start(StartRequest{ID: "wrong-branch-task", Title: "Wrong branch", Repository: "wrong-branch", Branch: "akofink/wrong-branch"})
		if err != nil {
			t.Fatal(err)
		}
		if err := runGit(result.Manifest.WorktreePath, "switch", "-c", "wrong"); err != nil {
			t.Fatal(err)
		}
		if _, err := manager.Reconcile(); err != nil {
			t.Fatal(err)
		}
		manifest, err := manager.Inspect("wrong-branch-task")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(manifest.RecoveryDebt, "worktree_mismatch") {
			t.Fatalf("recovery debt = %q, want branch mismatch", manifest.RecoveryDebt)
		}
	})

	t.Run("wrong base", func(t *testing.T) {
		manager, _ := newTestManager(t)
		registerWorktreeRepository(t, manager, "wrong-base")
		result, err := manager.Start(StartRequest{ID: "wrong-base-task", Title: "Wrong base", Repository: "wrong-base", Branch: "akofink/wrong-base"})
		if err != nil {
			t.Fatal(err)
		}
		base, err := gitOutput(result.Manifest.WorktreePath, "rev-parse", "HEAD")
		if err != nil {
			t.Fatal(err)
		}
		tree, err := gitOutput(result.Manifest.WorktreePath, "rev-parse", "HEAD^{tree}")
		if err != nil {
			t.Fatal(err)
		}
		wrongBase, err := exec.Command("git", "-C", result.Manifest.WorktreePath, "commit-tree", tree, "-m", "unrelated base", "-p", base).Output()
		if err != nil {
			t.Fatal(err)
		}
		wrongBase = []byte(strings.TrimSpace(string(wrongBase)))
		if _, err := manager.Store.UpdateManifest("wrong-base-task", func(manifest *store.Manifest) error {
			manifest.BaseRevision = string(wrongBase)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := manager.Reconcile(); err != nil {
			t.Fatal(err)
		}
		manifest, err := manager.Inspect("wrong-base-task")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(manifest.RecoveryDebt, "worktree_mismatch") {
			t.Fatalf("recovery debt = %q, want base mismatch", manifest.RecoveryDebt)
		}
	})
}

func TestWorktreeStartRequiresExplicitDescriptiveBranch(t *testing.T) {
	manager, _ := newTestManager(t)
	registerWorktreeRepository(t, manager, "requires-branch")
	_, err := manager.Start(StartRequest{ID: "requires-branch-task", Title: "Requires branch", Repository: "requires-branch"})
	if !store.IsKind(err, store.KindUsage) || !strings.Contains(err.Error(), "explicit descriptive --branch") {
		t.Fatalf("Start() error = %v, want explicit branch usage error", err)
	}
}

func TestDirectStartKeepsCurrentBranchWhenBranchIsOmitted(t *testing.T) {
	manager, tmux := newTestManager(t)
	result, err := manager.Start(StartRequest{ID: "direct-branch", Title: "Direct branch", Repository: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	repository, err := manager.Store.ReadRepository("demo")
	if err != nil {
		t.Fatal(err)
	}
	currentBranch, err := manager.Git.Branch(repository.Path)
	if err != nil {
		t.Fatal(err)
	}
	if result.Manifest.Branch != currentBranch || len(tmux.startBranches) != 1 || tmux.startBranches[0] != currentBranch {
		t.Fatalf("direct start branches = %#v and manifest branch %q, want current branch %q", tmux.startBranches, result.Manifest.Branch, currentBranch)
	}
}

func TestTmuxWindowNameRemovesOwnerPrefix(t *testing.T) {
	for branch, want := range map[string]string{
		"akofink/51-task-labels": "51-task-labels",
		"feature/review-build":   "review-build",
		"main":                   "main",
	} {
		if got := tmuxWindowName(branch); got != want {
			t.Errorf("tmuxWindowName(%q) = %q, want %q", branch, got, want)
		}
	}
}

func TestWorktreeStartOwnsImmutableGitInputs(t *testing.T) {
	manager, tmux := newTestManager(t)
	repository, err := manager.Store.ReadRepository("demo")
	if err != nil {
		t.Fatal(err)
	}
	repository.Name = "demo-worktree"
	repository.Policy = "worktree"
	repository.WorktreeRoot = filepath.Join(t.TempDir(), "worktrees")
	if _, err := manager.Store.RegisterRepository(repository); err != nil {
		t.Fatal(err)
	}
	request := StartRequest{ID: "task-worktree", Title: "Own worktree", Repository: "demo-worktree", Branch: "feature/task-worktree"}
	result, err := manager.Start(request)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Created || result.Manifest.Branch != request.Branch || result.Manifest.BaseRevision == "" {
		t.Fatalf("Start() = %#v, want created branch and base", result)
	}
	if _, err := os.Stat(result.Manifest.WorktreePath); err != nil {
		t.Fatalf("worktree path %q: %v", result.Manifest.WorktreePath, err)
	}
	if tmux.starts != 1 {
		t.Fatalf("tmux starts = %d, want 1", tmux.starts)
	}
	if repeated, err := manager.Start(request); err != nil || repeated.Created {
		t.Fatalf("equivalent Start() = %#v, %v; want no-op", repeated, err)
	}
	if _, err := manager.Start(StartRequest{ID: request.ID, Title: request.Title, Repository: request.Repository, Branch: "feature/other"}); err == nil {
		t.Fatal("conflicting branch was accepted")
	}
}

func TestConcurrentWorktreeStartsAreSerializedByRepositoryLock(t *testing.T) {
	manager, _ := newTestManager(t)
	repository, err := manager.Store.ReadRepository("demo")
	if err != nil {
		t.Fatal(err)
	}
	repository.Name = "demo-concurrent"
	repository.Policy = "worktree"
	repository.WorktreeRoot = filepath.Join(t.TempDir(), "worktrees")
	if _, err := manager.Store.RegisterRepository(repository); err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	errors := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			_, startErr := manager.Start(StartRequest{ID: fmt.Sprintf("task-concurrent-%d", index), Title: "Concurrent", Repository: "demo-concurrent", Branch: fmt.Sprintf("akofink/concurrent-%d", index)})
			errors <- startErr
		}(i)
	}
	wait.Wait()
	close(errors)
	for startErr := range errors {
		if startErr != nil {
			t.Fatalf("concurrent Start() error = %v", startErr)
		}
	}
}

func TestReconcileReportsWorktreeRecoveryAndGitFacts(t *testing.T) {
	manager, _ := newTestManager(t)
	repository, err := manager.Store.ReadRepository("demo")
	if err != nil {
		t.Fatal(err)
	}
	repository.Name = "demo-facts"
	repository.Policy = "worktree"
	repository.WorktreeRoot = filepath.Join(t.TempDir(), "worktrees")
	if _, err := manager.Store.RegisterRepository(repository); err != nil {
		t.Fatal(err)
	}
	result, err := manager.Start(StartRequest{ID: "task-facts", Title: "Facts", Repository: "demo-facts", Branch: "akofink/task-facts"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(result.Manifest.WorktreePath, "README"), []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(result.Manifest.WorktreePath, "untracked"), []byte("recover\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Reconcile(); err != nil {
		t.Fatal(err)
	}
	manifest, err := manager.Inspect("task-facts")
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.Dirty || !manifest.Untracked || manifest.Committed || !strings.Contains(manifest.RecoveryDebt, "uncommitted_work") {
		t.Fatalf("Git facts = %#v, want dirty and untracked recovery facts", manifest)
	}
	if err := os.RemoveAll(result.Manifest.WorktreePath); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Reconcile(); err != nil {
		t.Fatal(err)
	}
	manifest, err = manager.Inspect("task-facts")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(manifest.RecoveryDebt, "worktree_missing") {
		t.Fatalf("recovery debt = %q, want missing worktree", manifest.RecoveryDebt)
	}
}

func TestRequiredCredentialErrorDoesNotExposeValue(t *testing.T) {
	manager, _ := newTestManager(t)
	const secret = "never-print-this-secret"
	manager.Credentials = func() (*credential.Manifest, error) {
		return &credential.Manifest{Version: 1, Entries: []credential.Entry{{ID: "llm", Source: "env:LLM_TOKEN"}}}, nil
	}
	manager.Checker = &credential.Checker{LookupEnv: func(string) string { return secret }}
	if _, err := manager.Start(StartRequest{ID: "task-3", Title: "Secrets", Repository: "demo", Requirements: []string{"missing"}}); err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("Start() error = %v; must report capability without its value", err)
	}
}

func TestStartWithoutCredentialRequirementsDoesNotLoadManifest(t *testing.T) {
	manager, _ := newTestManager(t)
	manager.Credentials = func() (*credential.Manifest, error) {
		return nil, errors.New("credential manifest unavailable")
	}
	if _, err := manager.Start(StartRequest{ID: "task-5", Title: "No credentials", Repository: "demo"}); err != nil {
		t.Fatalf("Start() = %v, want no credential lookup", err)
	}
}
