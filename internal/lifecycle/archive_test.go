package lifecycle

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	manager.GitFacts = func(store.Manifest) (store.GitFacts, error) {
		return store.GitFacts{Dirty: true}, nil
	}
	if _, err := manager.Archive("clean-1"); err != nil {
		t.Fatal(err)
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
	if _, err := manager.Clean("live-1", CleanupOptions{AllowCommitted: true, AllowDirty: true, AllowUntracked: true, AllowWorktree: true}); err == nil {
		t.Fatal("Clean() succeeded for a live task")
	}
}

func TestApprovedWorktreeCleanupPreservesArchiveAndReconcileRecovery(t *testing.T) {
	manager, _ := newTestManager(t)
	repository := registerWorktreeRepository(t, manager, "cleanup-worktree")
	result, err := manager.Start(StartRequest{ID: "cleanup-worktree-task", Title: "Cleanup worktree", Repository: repository.Name, Branch: "akofink/cleanup-worktree"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Stop("cleanup-worktree-task"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Clean("cleanup-worktree-task", CleanupOptions{}); !store.IsKind(err, store.KindPreservation) {
		t.Fatalf("Clean() error = %v, want explicit worktree approval", err)
	}
	if _, err := os.Stat(result.Manifest.WorktreePath); err != nil {
		t.Fatalf("unapproved cleanup removed worktree: %v", err)
	}

	manifest, err := manager.Clean("cleanup-worktree-task", CleanupOptions{AllowWorktree: true})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.WorktreeCleanupState != cleanupComplete || manifest.CleanupDebt {
		t.Fatalf("cleanup manifest = %#v, want complete without debt", manifest)
	}
	if _, err := os.Stat(result.Manifest.WorktreePath); !os.IsNotExist(err) {
		t.Fatalf("worktree still exists after approved cleanup, stat error = %v", err)
	}
	archive, err := manager.Store.ReadArchive("cleanup-worktree-task")
	if err != nil {
		t.Fatal(err)
	}
	if archive.Git.Path != result.Manifest.WorktreePath || archive.Manifest.WorktreePath != result.Manifest.WorktreePath {
		t.Fatalf("archive = %#v, want pre-cleanup worktree facts", archive)
	}
	if _, err := manager.Store.UpdateManifest("cleanup-worktree-task", func(manifest *store.Manifest) error {
		manifest.RecoveryDebt = "worktree_mismatch"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Reconcile(); err != nil {
		t.Fatal(err)
	}
	manifest, err = manager.Inspect("cleanup-worktree-task")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(manifest.RecoveryDebt, "worktree_missing") || strings.Contains(manifest.RecoveryDebt, "worktree_mismatch") {
		t.Fatalf("reconcile retained obsolete worktree debt: %q", manifest.RecoveryDebt)
	}
}

func TestWorktreeCleanupRejectsUnsafeOwnership(t *testing.T) {
	manager, _ := newTestManager(t)
	repository := registerWorktreeRepository(t, manager, "unsafe-cleanup")
	result, err := manager.Start(StartRequest{ID: "unsafe-cleanup-task", Title: "Unsafe cleanup", Repository: repository.Name, Branch: "akofink/unsafe-cleanup"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Stop("unsafe-cleanup-task"); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if _, err := manager.Store.UpdateManifest("unsafe-cleanup-task", func(manifest *store.Manifest) error {
		manifest.WorktreePath = outside
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	_, err = manager.Clean("unsafe-cleanup-task", CleanupOptions{AllowWorktree: true})
	if !store.IsKind(err, store.KindConflict) || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("unsafe cleanup error = %v, want ownership conflict", err)
	}
	if _, err := os.Stat(result.Manifest.WorktreePath); err != nil {
		t.Fatalf("unsafe cleanup removed the owned worktree: %v", err)
	}
}

func TestConcurrentApprovedWorktreeCleanupIsIdempotent(t *testing.T) {
	manager, _ := newTestManager(t)
	repository := registerWorktreeRepository(t, manager, "concurrent-cleanup")
	result, err := manager.Start(StartRequest{ID: "concurrent-cleanup-task", Title: "Concurrent cleanup", Repository: repository.Name, Branch: "akofink/concurrent-cleanup"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Stop("concurrent-cleanup-task"); err != nil {
		t.Fatal(err)
	}
	options := CleanupOptions{AllowWorktree: true}
	var group sync.WaitGroup
	errors := make(chan error, 4)
	for i := 0; i < 4; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			_, cleanupErr := manager.Clean("concurrent-cleanup-task", options)
			errors <- cleanupErr
		}()
	}
	group.Wait()
	close(errors)
	for cleanupErr := range errors {
		if cleanupErr != nil {
			t.Fatalf("concurrent Clean() error = %v", cleanupErr)
		}
	}
	if _, err := os.Stat(result.Manifest.WorktreePath); !os.IsNotExist(err) {
		t.Fatalf("worktree exists after concurrent cleanup, stat error = %v", err)
	}
}
