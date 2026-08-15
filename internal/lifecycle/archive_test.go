package lifecycle

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/akofink/akagent-cli/internal/store"
)

func stoppedManager(t *testing.T, id string) (*Manager, *fakeTmux) {
	t.Helper()
	manager, tmux := newTestManager(t)
	if _, err := manager.Start(StartRequest{ID: id, Title: "Archive me", Repository: "demo"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Stop(id); err != nil {
		t.Fatal(err)
	}
	return manager, tmux
}

func TestArchiveCapturesDurableAndAvailableHistoryIdempotently(t *testing.T) {
	manager, _ := stoppedManager(t, "archive-1")
	manager.GitFacts = func(store.Manifest) (store.GitFacts, error) {
		return store.GitFacts{Path: "/repo/worktree", Head: "abc123", Branch: "feature/archive"}, nil
	}
	manager.TerminalHistory = func(string) (string, error) { return "terminal output\n", nil }

	manifest, err := manager.Archive("archive-1")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ArchiveState != archiveComplete {
		t.Fatalf("ArchiveState = %q, want complete", manifest.ArchiveState)
	}
	archive, err := manager.Store.ReadArchive("archive-1")
	if err != nil {
		t.Fatal(err)
	}
	if archive.Terminal != "terminal output\n" || archive.Git.Head != "abc123" {
		t.Fatalf("archive = %#v, want terminal and Git facts", archive)
	}
	if len(archive.Events) < 3 {
		t.Fatalf("archive events = %d, want startup and archive history", len(archive.Events))
	}
	events, err := manager.Store.ReadEvents("archive-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Archive("archive-1"); err != nil {
		t.Fatal(err)
	}
	again, err := manager.Store.ReadEvents("archive-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != len(events) {
		t.Fatalf("idempotent archive added events: before %d, after %d", len(events), len(again))
	}
}

func TestArchivePersistsPartialStateAndRetries(t *testing.T) {
	manager, _ := stoppedManager(t, "archive-2")
	archivePath := filepath.Join(manager.Store.Root(), "tasks", "archive-2", "archive.json")
	if err := os.Mkdir(archivePath, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Archive("archive-2"); err == nil || !store.IsKind(err, store.KindPartial) {
		t.Fatalf("Archive() error = %v, want retryable partial error", err)
	}
	manifest, err := manager.Inspect("archive-2")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ArchiveState != archivePartial {
		t.Fatalf("ArchiveState = %q, want partial", manifest.ArchiveState)
	}
	if err := os.Remove(archivePath); err != nil {
		t.Fatal(err)
	}
	if manifest, err = manager.Archive("archive-2"); err != nil || manifest.ArchiveState != archiveComplete {
		t.Fatalf("retry Archive() = %#v, %v; want complete", manifest, err)
	}
}

func TestCleanRequiresExplicitPreservationAuthorization(t *testing.T) {
	manager, _ := stoppedManager(t, "clean-1")
	if _, err := manager.Archive("clean-1"); err != nil {
		t.Fatal(err)
	}
	manager.GitFacts = func(store.Manifest) (store.GitFacts, error) {
		return store.GitFacts{Dirty: true}, nil
	}
	cleaned := false
	manager.CleanupWorktree = func(store.Manifest, store.GitFacts) error {
		cleaned = true
		return nil
	}
	if _, err := manager.Clean("clean-1", CleanupOptions{}); !store.IsKind(err, store.KindPreservation) {
		t.Fatalf("Clean() error = %v, want preservation error", err)
	}
	if cleaned {
		t.Fatal("Clean() removed a worktree without authorization")
	}
	manifest, err := manager.Inspect("clean-1")
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.CleanupDebt || manifest.WorktreeCleanupState != cleanupBlocked {
		t.Fatalf("manifest = %#v, want preserved cleanup debt", manifest)
	}
	if _, err := manager.Clean("clean-1", CleanupOptions{AllowDirty: true}); err != nil {
		t.Fatal(err)
	}
	manifest, err = manager.Inspect("clean-1")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.CleanupDebt || manifest.CleanupState != cleanupComplete || !cleaned {
		t.Fatalf("manifest = %#v, want completed cleanup", manifest)
	}
}

func TestCleanTracksCredentialDebtIndependentlyAndRetries(t *testing.T) {
	manager, _ := stoppedManager(t, "clean-2")
	manager.CleanupCredentials = func(store.Manifest) error { return errors.New("credential cleanup unavailable") }
	if _, err := manager.Clean("clean-2", CleanupOptions{}); err == nil || !store.IsKind(err, store.KindPartial) {
		t.Fatalf("Clean() error = %v, want partial error", err)
	}
	manifest, err := manager.Inspect("clean-2")
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.CleanupDebt || manifest.CredentialCleanupState != cleanupPartial || manifest.WorktreeCleanupState != cleanupComplete {
		t.Fatalf("manifest = %#v, want independent cleanup states", manifest)
	}
	manager.CleanupCredentials = func(store.Manifest) error { return nil }
	if _, err := manager.Clean("clean-2", CleanupOptions{}); err != nil {
		t.Fatal(err)
	}
	manifest, err = manager.Inspect("clean-2")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.CleanupDebt || manifest.CleanupState != cleanupComplete || manifest.CredentialCleanupState != cleanupComplete {
		t.Fatalf("manifest = %#v, want debt cleared after retry", manifest)
	}
}

func TestArchiveAndCleanRefuseLiveTask(t *testing.T) {
	manager, _ := newTestManager(t)
	if _, err := manager.Start(StartRequest{ID: "live-1", Title: "Live", Repository: "demo"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Archive("live-1"); err == nil {
		t.Fatal("Archive() succeeded for a live task")
	}
	if _, err := manager.Clean("live-1", CleanupOptions{}); err == nil {
		t.Fatal("Clean() succeeded for a live task")
	}
}
