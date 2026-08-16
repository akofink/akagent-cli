package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRepositoryWorktreeRootValidation(t *testing.T) {
	state, err := OpenAt(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	if err := state.WriteRepository(Repository{Name: "relative", Path: "/checkout", Policy: "worktree", WorktreeRoot: "worktrees"}); !IsKind(err, KindUsage) {
		t.Fatalf("relative worktree root error = %v, want usage", err)
	}
	if err := state.WriteRepository(Repository{Name: "direct", Path: "/checkout", Policy: "direct", WorktreeRoot: "/worktrees"}); !IsKind(err, KindUsage) {
		t.Fatalf("direct worktree root error = %v, want usage", err)
	}
	if err := state.WriteRepository(Repository{Name: "legacy", Path: "/checkout", Policy: "worktree"}); err != nil {
		t.Fatalf("legacy empty worktree root = %v, want accepted", err)
	}
}

func TestUpdateRepositoryIsIdempotentAndConflictSafe(t *testing.T) {
	state, err := OpenAt(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	repository := Repository{Name: "demo", Path: filepath.Join(t.TempDir(), "checkout"), Policy: "direct"}
	if err := state.WriteRepository(repository); err != nil {
		t.Fatal(err)
	}
	updated, err := state.UpdateRepository("demo", func(value *Repository) error {
		value.Policy = "direct"
		return nil
	})
	if err != nil || !sameRepository(updated, repository) {
		t.Fatalf("equivalent update = %#v, %v; want unchanged repository", updated, err)
	}

	if err := state.WriteManifest("task-18", Manifest{Repository: "demo"}); err != nil {
		t.Fatal(err)
	}
	_, err = state.UpdateRepository("demo", func(value *Repository) error {
		value.Path = filepath.Join(t.TempDir(), "other")
		return nil
	})
	if !IsKind(err, KindConflict) {
		t.Fatalf("referenced path update error = %v, want conflict", err)
	}
	current, err := state.ReadRepository("demo")
	if err != nil {
		t.Fatal(err)
	}
	if !sameRepository(current, repository) {
		t.Fatalf("conflicting update changed registration: %#v", current)
	}
}

func TestUnregisterRepositoryPreservesCheckoutAndRejectsReferences(t *testing.T) {
	state, err := OpenAt(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	checkout := filepath.Join(t.TempDir(), "checkout")
	if err := os.Mkdir(checkout, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(checkout, "marker")
	if err := os.WriteFile(marker, []byte("preserve\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteRepository(Repository{Name: "demo", Path: checkout, Policy: "direct"}); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteManifest("task-18", Manifest{Repository: "demo"}); err != nil {
		t.Fatal(err)
	}
	if err := state.UnregisterRepository("demo"); !IsKind(err, KindConflict) {
		t.Fatalf("referenced unregister error = %v, want conflict", err)
	}
	if _, err := state.ReadRepository("demo"); err != nil {
		t.Fatalf("referenced unregister removed record: %v", err)
	}

	if err := os.RemoveAll(filepath.Join(state.Root(), "tasks", "task-18")); err != nil {
		t.Fatal(err)
	}
	if err := state.UnregisterRepository("demo"); err != nil {
		t.Fatal(err)
	}
	if _, err := state.ReadRepository("demo"); !IsKind(err, KindNotFound) {
		t.Fatalf("unregistered read error = %v, want not_found", err)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "preserve\n" {
		t.Fatalf("unregister changed checkout: data=%q err=%v", data, err)
	}
}
