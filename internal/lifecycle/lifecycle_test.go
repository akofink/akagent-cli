package lifecycle

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/akofink/akagent-cli/internal/credential"
	"github.com/akofink/akagent-cli/internal/store"
)

type fakeTmux struct {
	exists bool
	starts int
}

func (t *fakeTmux) Start(string) (string, error) { t.exists = true; t.starts++; return "@1", nil }
func (t *fakeTmux) Exists(string) (bool, error)  { return t.exists, nil }
func (t *fakeTmux) Stop(string) error            { t.exists = false; return nil }

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
	tmux := &fakeTmux{}
	manager := New(state)
	manager.Tmux = tmux
	manager.Credentials = func() (*credential.Manifest, error) { return &credential.Manifest{Version: 1}, nil }
	if _, err := manager.RegisterRepository("demo", t.TempDir(), "direct"); err != nil {
		t.Fatal(err)
	}
	return manager, tmux
}

func TestStartIsIdempotent(t *testing.T) {
	manager, tmux := newTestManager(t)
	request := StartRequest{ID: "task-1", Title: "Test lifecycle", Repository: "demo"}
	if _, err := manager.Start(request); err != nil {
		t.Fatal(err)
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

func TestReconcileRecordsMissingWindowWithoutDeletingTask(t *testing.T) {
	manager, tmux := newTestManager(t)
	if _, err := manager.Start(StartRequest{ID: "task-2", Title: "Recover", Repository: "demo"}); err != nil {
		t.Fatal(err)
	}
	tmux.exists = false
	if _, err := manager.Reconcile(); err != nil {
		t.Fatal(err)
	}
	manifest, err := manager.Inspect("task-2")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Lifecycle != "stopped" {
		t.Fatalf("lifecycle = %q, want stopped", manifest.Lifecycle)
	}
	events, err := manager.Store.ReadEvents("task-2")
	if err != nil {
		t.Fatal(err)
	}
	if events[len(events)-1].Event.Outcome != "window_missing" {
		t.Fatalf("last event = %#v", events[len(events)-1])
	}
}

func TestStartAfterReconciliationIsANoOp(t *testing.T) {
	manager, tmux := newTestManager(t)
	request := StartRequest{ID: "task-4", Title: "No restart", Repository: "demo"}
	if _, err := manager.Start(request); err != nil {
		t.Fatal(err)
	}
	tmux.exists = false
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

func TestStartWithoutCredentialRequirementsDoesNotLoadManifest(t *testing.T) {
	manager, _ := newTestManager(t)
	manager.Credentials = func() (*credential.Manifest, error) {
		return nil, errors.New("credential manifest unavailable")
	}
	if _, err := manager.Start(StartRequest{ID: "task-5", Title: "No credentials", Repository: "demo"}); err != nil {
		t.Fatalf("Start() = %v, want no credential lookup", err)
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
	tmux.exists = false
	manifest, err := manager.Finish("task-6", "succeeded", "complete")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Lifecycle != "finished" {
		t.Fatalf("lifecycle = %q, want finished", manifest.Lifecycle)
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
