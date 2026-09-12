package lifecycle

import (
	"errors"
	"sync"
	"testing"

	"github.com/akofink/akagent-cli/internal/store"
)

func TestSetDispositionIsRecordOnlyAndIdempotent(t *testing.T) {
	manager, tmux := newTestManager(t)
	if _, err := manager.Create(CreateRequest{ID: "disposition-1", Title: "Disposition", Repository: "demo"}); err != nil {
		t.Fatal(err)
	}
	before, err := manager.Inspect("disposition-1")
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := manager.SetDisposition("disposition-1", DispositionDeferred, "awaiting approval", nil)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Disposition != string(DispositionDeferred) || manifest.DispositionReason != "awaiting approval" || manifest.DispositionRevision != 1 {
		t.Fatalf("manifest = %#v, want deferred revision 1", manifest)
	}
	if tmux.starts != 0 || len(tmux.observedIDs) != 0 {
		t.Fatalf("SetDisposition touched tmux: starts=%d observations=%v", tmux.starts, tmux.observedIDs)
	}
	again, err := manager.SetDisposition("disposition-1", DispositionDeferred, "awaiting approval", uint64Ptr(1))
	if err != nil {
		t.Fatal(err)
	}
	if again.DispositionRevision != 1 {
		t.Fatalf("idempotent revision = %d, want 1", again.DispositionRevision)
	}
	events, err := manager.Store.ReadEvents("disposition-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[1].Event.Operation != "disposition" {
		t.Fatalf("events = %#v, want create and disposition events", events)
	}
	if before.Lifecycle != "created" {
		t.Fatalf("before lifecycle = %q, want created", before.Lifecycle)
	}
}

func TestFinishClosesExplicitInFlightDisposition(t *testing.T) {
	manager, tmux := newTestManager(t)
	if _, err := manager.Start(StartRequest{ID: "disposition-finish", Title: "Finish disposition", Repository: "demo"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.SetDisposition("disposition-finish", DispositionInFlight, "accepted commitment", nil); err != nil {
		t.Fatal(err)
	}
	tmux.observation = TmuxObservation{Available: true}
	manifest, err := manager.Finish("disposition-finish", "succeeded", "verified")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Lifecycle != "finished" || manifest.Disposition != string(DispositionTerminal) || manifest.DispositionRevision != 2 {
		t.Fatalf("finished manifest = %#v, want terminal disposition", manifest)
	}
}

func TestDispositionRepairIsRevisionScopedAcrossABA(t *testing.T) {
	manager, _ := newTestManager(t)
	if _, err := manager.Create(CreateRequest{ID: "disposition-aba", Title: "ABA", Repository: "demo"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.SetDisposition("disposition-aba", DispositionTerminal, "first", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.SetDisposition("disposition-aba", DispositionDeferred, "second", uint64Ptr(1)); err != nil {
		t.Fatal(err)
	}
	manager.AppendEventIfAbsent = func(string, store.Event) (int, bool, error) { return 0, false, errors.New("event append unavailable") }
	if _, err := manager.SetDisposition("disposition-aba", DispositionTerminal, "first", uint64Ptr(2)); err == nil {
		t.Fatal("A -> B -> A succeeded when final event append failed")
	}
	manager.AppendEventIfAbsent = manager.Store.AppendEventIfAbsent
	if _, err := manager.SetDisposition("disposition-aba", DispositionTerminal, "first", uint64Ptr(3)); err != nil {
		t.Fatal("ABA event repair failed: ", err)
	}
	events, err := manager.Store.ReadEvents("disposition-aba")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 || events[3].Event.Revision != 3 || events[3].Event.Detail != "first" {
		t.Fatalf("ABA events = %#v, want revision-scoped final audit", events)
	}
}

func TestConcurrentIdenticalDispositionRepairAppendsOnce(t *testing.T) {
	manager, _ := newTestManager(t)
	if _, err := manager.Create(CreateRequest{ID: "disposition-retry-race", Title: "Retry race", Repository: "demo"}); err != nil {
		t.Fatal(err)
	}
	manager.AppendEventIfAbsent = func(string, store.Event) (int, bool, error) { return 0, false, errors.New("event append unavailable") }
	if _, err := manager.SetDisposition("disposition-retry-race", DispositionTerminal, "done", nil); err == nil {
		t.Fatal("initial disposition unexpectedly succeeded")
	}
	manager.AppendEventIfAbsent = manager.Store.AppendEventIfAbsent
	var group sync.WaitGroup
	for i := 0; i < 12; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			_, _ = manager.SetDisposition("disposition-retry-race", DispositionTerminal, "done", uint64Ptr(1))
		}()
	}
	group.Wait()
	events, err := manager.Store.ReadEvents("disposition-retry-race")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[1].Event.Revision != 1 {
		t.Fatalf("concurrent repaired events = %#v, want one revision 1 audit", events)
	}
}

func TestDispositionPersistsWhenEventAppendFails(t *testing.T) {
	manager, _ := newTestManager(t)
	if _, err := manager.Create(CreateRequest{ID: "disposition-event-failure", Title: "Event failure", Repository: "demo"}); err != nil {
		t.Fatal(err)
	}
	manager.AppendEventIfAbsent = func(string, store.Event) (int, bool, error) { return 0, false, errors.New("event append unavailable") }
	if _, err := manager.SetDisposition("disposition-event-failure", DispositionTerminal, "completed elsewhere", nil); !store.IsKind(err, store.KindPartial) {
		t.Fatalf("SetDisposition() error = %v, want structured partial error", err)
	}
	manifest, err := manager.Inspect("disposition-event-failure")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Disposition != string(DispositionTerminal) || manifest.DispositionReason != "completed elsewhere" || manifest.DispositionRevision != 1 {
		t.Fatalf("manifest after event failure = %#v, want persisted disposition", manifest)
	}
	manager.AppendEventIfAbsent = manager.Store.AppendEventIfAbsent
	if _, err := manager.SetDisposition("disposition-event-failure", DispositionTerminal, "completed elsewhere", uint64Ptr(1)); err != nil {
		t.Fatalf("retry after event append failure = %v", err)
	}
	events, err := manager.Store.ReadEvents("disposition-event-failure")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[1].Event.Operation != "disposition" {
		t.Fatalf("events after retry = %#v, want repaired disposition event", events)
	}
}

func TestSetDispositionProtectsRevisionUnderConcurrentUpdates(t *testing.T) {
	manager, _ := newTestManager(t)
	if _, err := manager.Create(CreateRequest{ID: "disposition-2", Title: "Concurrent", Repository: "demo"}); err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	var mu sync.Mutex
	successes := 0
	for i := 0; i < 12; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			if _, err := manager.SetDisposition("disposition-2", DispositionTerminal, "completed", uint64Ptr(0)); err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			}
		}()
	}
	group.Wait()
	manifest, err := manager.Inspect("disposition-2")
	if err != nil {
		t.Fatal(err)
	}
	if successes != 1 || manifest.DispositionRevision != 1 || manifest.Disposition != string(DispositionTerminal) {
		t.Fatalf("successes=%d manifest=%#v, want one revision-protected transition", successes, manifest)
	}
}

func TestReconcileDoesNotAutoCompleteUnknownOrStoppedLegacyWork(t *testing.T) {
	manager, tmux := newTestManager(t)
	if err := manager.Store.WriteManifest("legacy-unknown", store.Manifest{Title: "Unknown", Worker: "local", Lifecycle: "running", Condition: "none"}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Store.WriteManifest("legacy-stopped", store.Manifest{Title: "Stopped", Worker: "local", Lifecycle: "stopped", Condition: "none"}); err != nil {
		t.Fatal(err)
	}
	tmux.observation = TmuxObservation{Available: false}
	for _, id := range []string{"legacy-unknown", "legacy-stopped"} {
		manifest, err := manager.ReconcileTask(id)
		if err != nil {
			t.Fatal(err)
		}
		if manifest.Lifecycle == "finished" || WorkDispositionOf(manifest) == DispositionTerminal {
			t.Fatalf("reconciled %s = %#v, unexpectedly completed legacy work", id, manifest)
		}
	}
}

func TestWorkDispositionOfLegacyManifestIsConservative(t *testing.T) {
	for name, test := range map[string]struct {
		manifest store.Manifest
		want     WorkDisposition
	}{
		"completed outcome": {
			manifest: store.Manifest{Lifecycle: "finished", Condition: "succeeded", Result: "done"},
			want:     DispositionTerminal,
		},
		"stopped without outcome": {
			manifest: store.Manifest{Lifecycle: "stopped", Condition: "none", CleanupDebt: true},
			want:     DispositionInFlight,
		},
		"waiting process": {
			manifest: store.Manifest{Lifecycle: "running", Condition: "waiting"},
			want:     DispositionInFlight,
		},
		"explicit deferred": {
			manifest: store.Manifest{Lifecycle: "finished", Disposition: string(DispositionDeferred)},
			want:     DispositionDeferred,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := WorkDispositionOf(test.manifest); got != test.want {
				t.Fatalf("WorkDispositionOf() = %q, want %q", got, test.want)
			}
		})
	}
}

func uint64Ptr(value uint64) *uint64 { return &value }
