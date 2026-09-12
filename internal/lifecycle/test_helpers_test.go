package lifecycle

import (
	"os"
	"sync"
	"testing"

	"github.com/akofink/akagent-cli/internal/store"
)

type fakeTmux struct {
	mu          sync.Mutex
	starts      int
	observedIDs []string
}

func (t *fakeTmux) Start(string, string) (TmuxProcess, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.starts++
	return TmuxProcess{}, nil
}

func (t *fakeTmux) Observe(id string) (TmuxObservation, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.observedIDs = append(t.observedIDs, id)
	return TmuxObservation{Available: true}, nil
}

func (t *fakeTmux) Attach(string, string) error { return nil }
func (t *fakeTmux) Stop(string) error           { return nil }

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
	return manager, tmux
}
