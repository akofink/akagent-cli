package lifecycle

import (
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/akofink/akagent-cli/internal/credential"
	"github.com/akofink/akagent-cli/internal/store"
)

type fakeTmux struct {
	observation TmuxObservation
	starts      int
}

func (t *fakeTmux) Start(string) (TmuxProcess, error) {
	t.starts++
	t.observation = TmuxObservation{Available: true, Processes: []TmuxProcess{{WindowID: "@1", PaneID: "%1", PID: 42, StartTime: 100}}}
	return t.observation.Processes[0], nil
}
func (t *fakeTmux) Observe(string) (TmuxObservation, error) { return t.observation, nil }
func (t *fakeTmux) Stop(string) error {
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
	if _, err := manager.RegisterRepository("demo", t.TempDir(), "direct"); err != nil {
		t.Fatal(err)
	}
	return manager, tmux
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
