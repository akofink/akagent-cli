package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecoverRemovesStaleTemporaryFiles(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	if err := store.WriteManifest(taskID, Manifest{Title: "x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendEvent(taskID, Event{Operation: "one"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendEvent(taskID, Event{Operation: "two"}); err != nil {
		t.Fatal(err)
	}

	stale := []string{
		filepath.Join(store.taskDir(taskID), tempPrefix+"manifest-interrupted"),
		filepath.Join(store.eventsDir(taskID), tempPrefix+"event-interrupted"),
	}
	for _, path := range stale {
		if err := os.WriteFile(path, []byte("partial"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	result, err := store.Recover()
	if err != nil {
		t.Fatalf("Recover() error = %v", err)
	}
	if len(result.StaleFilesRemoved) != 2 {
		t.Fatalf("Recover() StaleFilesRemoved = %v, want both stale files", result.StaleFilesRemoved)
	}
	for _, path := range stale {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("stale file %s not removed", path)
		}
	}

	if _, err := store.ReadManifest(taskID); err != nil {
		t.Fatalf("ReadManifest() after recovery = %v", err)
	}
	records, err := store.ReadEvents(taskID)
	if err != nil || len(records) != 2 {
		t.Fatalf("ReadEvents() after recovery = %v, %v; want 2 events", records, err)
	}
	if len(result.MalformedRecords) != 0 {
		t.Fatalf("Recover() MalformedRecords = %v, want none", result.MalformedRecords)
	}
}

func TestRecoverRemovesStaleTempSymlinkWithoutFollowing(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	if err := store.WriteManifest(taskID, Manifest{Title: "x"}); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	sentinel := []byte("outside sentinel")
	if err := os.WriteFile(outside, sentinel, 0o600); err != nil {
		t.Fatal(err)
	}
	tempPath := filepath.Join(store.eventsDir(taskID), tempPrefix+"outside")
	if err := os.Symlink(outside, tempPath); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	result, err := store.Recover()
	if err != nil {
		t.Fatalf("Recover() error = %v", err)
	}
	if len(result.StaleFilesRemoved) != 1 {
		t.Fatalf("Recover() StaleFilesRemoved = %v, want temp symlink removed", result.StaleFilesRemoved)
	}
	if _, err := os.Lstat(tempPath); !os.IsNotExist(err) {
		t.Fatalf("stale temp symlink remains: %v", err)
	}
	got, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(sentinel) {
		t.Fatalf("outside target changed to %q, want sentinel", got)
	}
}

func TestRecoverReportsMalformedRecordsWithoutDeleting(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	if err := store.WriteManifest(taskID, Manifest{Title: "x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendEvent(taskID, Event{Operation: "ok"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendEvent(taskID, Event{Operation: "broken"}); err != nil {
		t.Fatal(err)
	}

	manifestPath := store.manifestPath(taskID)
	eventPath := filepath.Join(store.eventsDir(taskID), "000002.json")
	if err := os.WriteFile(manifestPath, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(eventPath, []byte("{ truncated"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := store.Recover()
	if err != nil {
		t.Fatalf("Recover() error = %v", err)
	}
	if len(result.MalformedRecords) != 2 {
		t.Fatalf("Recover() MalformedRecords = %v, want 2", result.MalformedRecords)
	}
	for _, path := range []string{manifestPath, eventPath} {
		found := false
		for _, reported := range result.MalformedRecords {
			if strings.Contains(reported, filepath.Base(path)) {
				found = true
			}
		}
		if !found {
			t.Fatalf("Recover() MalformedRecords %v missing %s", result.MalformedRecords, path)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("Recover() deleted malformed record %s: %v", path, err)
		}
	}

	if _, err := store.ReadManifest(taskID); !IsKind(err, KindMalformed) {
		t.Fatalf("ReadManifest() after recovery = %v, want still KindMalformed", err)
	}
	if _, err := store.ReadEvents(taskID); !IsKind(err, KindMalformed) {
		t.Fatalf("ReadEvents() after recovery = %v, want still KindMalformed", err)
	}
}

func TestRecoverSkipsLockedTasks(t *testing.T) {
	store := openTest(t)
	lockedID := validTaskID(t)
	freeID := "019fe8f2-ac67-7406-a6e6-2717b2cd31c7"
	for _, id := range []string{lockedID, freeID} {
		if err := store.WriteManifest(id, Manifest{Title: "x"}); err != nil {
			t.Fatal(err)
		}
	}

	// A second concurrent task must not be cleaned while its lock is held.
	release, err := store.Lock(freeID)
	if err != nil {
		t.Fatalf("Lock() error = %v", err)
	}
	defer release()

	withShortLockWait(t, func() {
		result, err := store.Recover()
		if err != nil {
			t.Fatalf("Recover() error = %v", err)
		}
		if len(result.SkippedLocked) != 1 || result.SkippedLocked[0] != freeID {
			t.Fatalf("Recover() SkippedLocked = %v, want [%s]", result.SkippedLocked, freeID)
		}
		if len(result.StaleFilesRemoved)+len(result.MalformedRecords) != 0 {
			t.Fatalf("Recover() unexpectedly modified %#v", result)
		}
	})
}
