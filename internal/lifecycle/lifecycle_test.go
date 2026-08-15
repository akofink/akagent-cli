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
	mu          sync.Mutex
	observation TmuxObservation
	starts      int
}

func (t *fakeTmux) Start(string) (TmuxProcess, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.starts++
	t.observation = TmuxObservation{Available: true, Processes: []TmuxProcess{{WindowID: "@1", PaneID: "%1", PID: 42, StartTime: 100}}}
	return t.observation.Processes[0], nil
}
func (t *fakeTmux) Observe(string) (TmuxObservation, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.observation, nil
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
			_, startErr := manager.Start(StartRequest{ID: fmt.Sprintf("task-concurrent-%d", index), Title: "Concurrent", Repository: "demo-concurrent"})
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
	result, err := manager.Start(StartRequest{ID: "task-facts", Title: "Facts", Repository: "demo-facts"})
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
