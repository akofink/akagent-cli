package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAtomicWritePreservesPreviousManifest verifies a failed replacement
// leaves the previous valid manifest intact and removes temporary files.
// Test runs as a regular user; a root run cannot be blocked by directory
// permissions, so it is skipped for the read-only variant.
func TestAtomicWritePreservesPreviousManifest(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("test relies on directory write permissions, skipped as root")
	}
	store := openTest(t)
	taskID := validTaskID(t)
	previous := Manifest{Title: "previous", Worker: "local", Lifecycle: "running"}
	if err := store.WriteManifest(taskID, previous); err != nil {
		t.Fatal(err)
	}

	taskDir := store.taskDir(taskID)
	if err := os.Chmod(taskDir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(taskDir, 0o700)

	if err := store.WriteManifest(taskID, Manifest{Title: "should-not-land", Lifecycle: "failed"}); err == nil {
		t.Fatal("WriteManifest() unexpectedly succeeded against a read-only task directory")
	}

	if leftovers := listTempFiles(t, taskDir); len(leftovers) != 0 {
		t.Fatalf("temporary files left after failed write: %v", leftovers)
	}

	if err := os.Chmod(taskDir, 0o700); err != nil {
		t.Fatal(err)
	}
	envelope, err := store.ReadManifest(taskID)
	if err != nil {
		t.Fatalf("ReadManifest() after failed write = %v", err)
	}
	got, err := envelope.DecodeManifest()
	if err != nil {
		t.Fatalf("DecodeManifest() after failed write = %v", err)
	}
	if got != previous {
		t.Fatalf("manifest after failed write = %#v, want previous %#v", got, previous)
	}
}

// TestRenameFailureLeavesNoTemporaryFile verifies that when the final rename
// fails, the temporary file is cleaned up and no record is written.
func TestRenameFailureLeavesNoTemporaryFile(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	if err := store.WriteManifest(taskID, Manifest{Title: "original"}); err != nil {
		t.Fatal(err)
	}

	// Replace the manifest with a directory so the atomic rename target is
	// invalid and rename fails after the temporary file is fully written.
	manifestPath := store.manifestPath(taskID)
	if err := os.Remove(manifestPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(manifestPath, 0o700); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(manifestPath)

	if err := store.WriteManifest(taskID, Manifest{Title: "replacement"}); err == nil {
		t.Fatal("WriteManifest() unexpectedly succeeded onto a directory target")
	}
	if leftovers := listTempFiles(t, store.taskDir(taskID)); len(leftovers) != 0 {
		t.Fatalf("temporary files left after failed rename: %v", leftovers)
	}
	info, err := os.Stat(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatalf("manifest path was not left untouched after failed rename")
	}
}

func listTempFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	var leftovers []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), tempPrefix) {
			leftovers = append(leftovers, filepath.Join(dir, entry.Name()))
		}
	}
	return leftovers
}
