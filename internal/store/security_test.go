package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---- Finding 1: reject symlinks and non-regular records so records cannot
// ---- escape the configured state root.

func TestReadManifestRejectsSymlinkedManifest(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	if err := store.WriteManifest(taskID, Manifest{Title: "x"}); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "captured.json")
	if err := os.WriteFile(outside, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(store.manifestPath(taskID)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, store.manifestPath(taskID)); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	if _, err := store.ReadManifest(taskID); !IsKind(err, KindUnsafePath) {
		t.Fatalf("ReadManifest() error = %v, want KindUnsafePath", err)
	}
}

func TestReadManifestRejectsSymlinkedTaskDir(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	if err := store.WriteManifest(taskID, Manifest{Title: "x"}); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.RemoveAll(store.taskDir(taskID)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, store.taskDir(taskID)); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	if _, err := store.ReadManifest(taskID); !IsKind(err, KindUnsafePath) {
		t.Fatalf("ReadManifest() error = %v, want KindUnsafePath", err)
	}
}

func TestReadEventsRejectsSymlinkedEventsDir(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	if _, err := store.AppendEvent(taskID, Event{Operation: "create"}); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.RemoveAll(store.eventsDir(taskID)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, store.eventsDir(taskID)); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	if _, err := store.ReadEvents(taskID); !IsKind(err, KindUnsafePath) {
		t.Fatalf("ReadEvents() error = %v, want KindUnsafePath", err)
	}
}

func TestReadManifestRejectsNonRegularRecord(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	if err := store.WriteManifest(taskID, Manifest{Title: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(store.manifestPath(taskID)); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(store.manifestPath(taskID), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadManifest(taskID); !IsKind(err, KindUnsafePath) {
		t.Fatalf("ReadManifest() error = %v, want KindUnsafePath", err)
	}
}

func TestLockRejectsSymlinkedLockPath(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	outside := filepath.Join(t.TempDir(), "outside.lock")
	if err := os.WriteFile(outside, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, store.lockPath(taskID)); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	if _, err := store.Lock(taskID); !IsKind(err, KindUnsafePath) {
		t.Fatalf("Lock() error = %v, want KindUnsafePath", err)
	}
}

func TestAtomicReplacementDoesNotFollowManifestSymlink(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	if err := store.WriteManifest(taskID, Manifest{Title: "initial"}); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.json")
	outsideData := []byte("outside sentinel")
	if err := os.WriteFile(outside, outsideData, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(store.manifestPath(taskID)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, store.manifestPath(taskID)); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	if err := store.WriteManifest(taskID, Manifest{Title: "replacement"}); !IsKind(err, KindUnsafePath) {
		t.Fatalf("WriteManifest() error = %v, want KindUnsafePath", err)
	}
	gotOutside, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotOutside) != string(outsideData) {
		t.Fatalf("outside target changed to %q, want sentinel", gotOutside)
	}
	info, err := os.Lstat(store.manifestPath(taskID))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("manifest symlink was replaced instead of rejected")
	}
}

func TestAtomicEventReplacementRejectsSymlinkTarget(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	if _, err := store.AppendEvent(taskID, Event{Operation: "initial"}); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.json")
	outsideData := []byte("outside sentinel")
	if err := os.WriteFile(outside, outsideData, 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(store.eventsDir(taskID), "000002.json")
	if err := os.Symlink(outside, target); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	envelope, err := eventEnvelope(taskID, Event{Operation: "replacement"})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := encodeRecord(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.atomicallyWrite(target, encoded); !IsKind(err, KindUnsafePath) {
		t.Fatalf("atomicallyWrite() error = %v, want KindUnsafePath", err)
	}
	gotOutside, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotOutside) != string(outsideData) {
		t.Fatalf("outside target changed to %q, want sentinel", gotOutside)
	}
	if info, err := os.Lstat(target); err != nil {
		t.Fatal(err)
	} else if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("event symlink was replaced instead of rejected")
	}
}

func TestReadRejectsSymlinkedIntermediateTasksDirectory(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	if err := store.WriteManifest(taskID, Manifest{Title: "x"}); err != nil {
		t.Fatal(err)
	}
	alternate := filepath.Join(store.Root(), "alternate-tasks")
	if err := os.MkdirAll(filepath.Join(alternate, taskID), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(alternate, taskID, "manifest.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(store.tasksDir()); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Base(alternate), store.tasksDir()); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	if _, err := store.ReadManifest(taskID); !IsKind(err, KindUnsafePath) {
		t.Fatalf("ReadManifest() error = %v, want KindUnsafePath", err)
	}
}

func TestWriteRejectsSymlinkedIntermediateTasksDirectory(t *testing.T) {
	store := openTest(t)
	outside := t.TempDir()
	if err := os.RemoveAll(store.tasksDir()); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, store.tasksDir()); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	if err := store.WriteManifest(validTaskID(t), Manifest{Title: "x"}); !IsKind(err, KindUnsafePath) {
		t.Fatalf("WriteManifest() error = %v, want KindUnsafePath", err)
	}
}

func TestOpenRejectsSymlinkedStoreDirectory(t *testing.T) {
	root := newRoot(t)
	if _, err := OpenAt(root); err != nil {
		t.Fatalf("OpenAt() error = %v", err)
	}
	outside := t.TempDir()
	if err := os.RemoveAll(filepath.Join(root, "tasks")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "tasks")); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	if _, err := OpenAt(root); !IsKind(err, KindUnsafePath) {
		t.Fatalf("OpenAt() error = %v, want KindUnsafePath", err)
	}
}

func TestRecoverReportsSymlinkedEventsDirectory(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	if _, err := store.AppendEvent(taskID, Event{Operation: "create"}); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.RemoveAll(store.eventsDir(taskID)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, store.eventsDir(taskID)); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	result, err := store.Recover()
	if err != nil {
		t.Fatalf("Recover() error = %v", err)
	}
	if len(result.MalformedRecords) != 1 {
		t.Fatalf("Recover() MalformedRecords = %v, want the symlinked events directory reported", result.MalformedRecords)
	}
}

// ---- Finding 2: validate events directory permissions on read and recovery.

func TestReadEventsRejectsHiddenEntry(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	if _, err := store.AppendEvent(taskID, Event{Operation: "create"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.eventsDir(taskID), ".hidden.json")
	if err := os.WriteFile(path, []byte("junk"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadEvents(taskID); !IsKind(err, KindMalformed) {
		t.Fatalf("ReadEvents() error = %v, want KindMalformed", err)
	}
}

func TestReadEventsRejectsDirectoryEntry(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	if _, err := store.AppendEvent(taskID, Event{Operation: "create"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.eventsDir(taskID), "000002.json")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadEvents(taskID); !IsKind(err, KindMalformed) {
		t.Fatalf("ReadEvents() error = %v, want KindMalformed", err)
	}
}

func TestReadEventsUnsafeEventsDirectory(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	if _, err := store.AppendEvent(taskID, Event{Operation: "create"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(store.eventsDir(taskID), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadEvents(taskID); !IsKind(err, KindUnsafe) {
		t.Fatalf("ReadEvents() error = %v, want KindUnsafe", err)
	}
}

func TestRecoverReportsUnsafeEventsDirectory(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	if _, err := store.AppendEvent(taskID, Event{Operation: "create"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(store.eventsDir(taskID), 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := store.Recover()
	if err != nil {
		t.Fatalf("Recover() error = %v", err)
	}
	if len(result.MalformedRecords) != 1 {
		t.Fatalf("Recover() MalformedRecords = %v, want the unsafe events directory reported", result.MalformedRecords)
	}
}

// ---- Finding 3: reject missing observation time and malformed typed
// ---- payloads with typed errors.

func TestMissingObservationTimeIsTyped(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	record := fmt.Sprintf(`{
  "schema_version": 1,
  "kind": "manifest",
  "task_id": %q,
  "data": {"title": "x"}
}`, taskID)
	writeRawManifest(t, store, taskID, record)
	_, err := store.ReadManifest(taskID)
	if !IsKind(err, KindMalformed) {
		t.Fatalf("ReadManifest() error = %v, want KindMalformed", err)
	}
	if !strings.Contains(err.Error(), "observation") {
		t.Fatalf("ReadManifest() error = %q, want observation time hint", err.Error())
	}
}

func TestNullManifestPayloadIsTyped(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	record := fmt.Sprintf(`{
  "schema_version": 1,
  "kind": "manifest",
  "task_id": %q,
  "observed_at": "2026-08-09T21:59:00Z",
  "data": null
}`, taskID)
	writeRawManifest(t, store, taskID, record)
	if _, err := store.ReadManifest(taskID); !IsKind(err, KindMalformed) {
		t.Fatalf("ReadManifest() error = %v, want KindMalformed", err)
	}
}

func TestNullEventPayloadIsTyped(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	if _, err := store.AppendEvent(taskID, Event{Operation: "ok"}); err != nil {
		t.Fatal(err)
	}
	record := fmt.Sprintf(`{
  "schema_version": 1,
  "kind": "event",
  "task_id": %q,
  "observed_at": "2026-08-09T21:59:00Z",
  "data": null
}`, taskID)
	if err := os.WriteFile(filepath.Join(store.eventsDir(taskID), "000001.json"), []byte(record), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadEvents(taskID); !IsKind(err, KindMalformed) {
		t.Fatalf("ReadEvents() error = %v, want KindMalformed", err)
	}
}

func TestMalformedManifestPayloadIsTyped(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	record := fmt.Sprintf(`{
  "schema_version": 1,
  "kind": "manifest",
  "task_id": %q,
  "observed_at": "2026-08-09T21:59:00Z",
  "data": "not-an-object"
}`, taskID)
	writeRawManifest(t, store, taskID, record)
	_, err := store.ReadManifest(taskID)
	if !IsKind(err, KindMalformed) {
		t.Fatalf("ReadManifest() error = %v, want KindMalformed", err)
	}
}

func TestMalformedEventPayloadIsTyped(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	if _, err := store.AppendEvent(taskID, Event{Operation: "ok"}); err != nil {
		t.Fatal(err)
	}
	record := fmt.Sprintf(`{
  "schema_version": 1,
  "kind": "event",
  "task_id": %q,
  "observed_at": "2026-08-09T21:59:00Z",
  "data": "not-an-object"
}`, taskID)
	if err := os.WriteFile(filepath.Join(store.eventsDir(taskID), "000001.json"), []byte(record), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadEvents(taskID); !IsKind(err, KindMalformed) {
		t.Fatalf("ReadEvents() error = %v, want KindMalformed", err)
	}
}

func TestRecoverReportsMalformedManifestPayload(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	record := fmt.Sprintf(`{
  "schema_version": 1,
  "kind": "manifest",
  "task_id": %q,
  "observed_at": "2026-08-09T21:59:00Z",
  "data": "not-an-object"
}`, taskID)
	writeRawManifest(t, store, taskID, record)
	result, err := store.Recover()
	if err != nil {
		t.Fatalf("Recover() error = %v", err)
	}
	if len(result.MalformedRecords) != 1 {
		t.Fatalf("Recover() MalformedRecords = %v, want the malformed payload reported", result.MalformedRecords)
	}
}

// ---- Finding 4: enforce positive, zero-padded, gap-free event sequences.

func TestReadEventsRejectsNonZeroPaddedSequence(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	if _, err := store.AppendEvent(taskID, Event{Operation: "one"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendEvent(taskID, Event{Operation: "two"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(
		filepath.Join(store.eventsDir(taskID), "000001.json"),
		filepath.Join(store.eventsDir(taskID), "1.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadEvents(taskID); !IsKind(err, KindMalformed) {
		t.Fatalf("ReadEvents() error = %v, want KindMalformed", err)
	}
}

func TestReadEventsRejectsGapInSequences(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	for _, op := range []string{"one", "two", "three"} {
		if _, err := store.AppendEvent(taskID, Event{Operation: op}); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(filepath.Join(store.eventsDir(taskID), "000002.json")); err != nil {
		t.Fatal(err)
	}
	_, err := store.ReadEvents(taskID)
	if !IsKind(err, KindMalformed) {
		t.Fatalf("ReadEvents() error = %v, want KindMalformed", err)
	}
	if !strings.Contains(err.Error(), "gap") {
		t.Fatalf("ReadEvents() error = %q, want gap hint", err.Error())
	}
}

func TestAppendEventRejectsGap(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	if _, err := store.AppendEvent(taskID, Event{Operation: "one"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendEvent(taskID, Event{Operation: "two"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(store.eventsDir(taskID), "000001.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendEvent(taskID, Event{Operation: "three"}); !IsKind(err, KindMalformed) {
		t.Fatalf("AppendEvent() error = %v, want KindMalformed", err)
	}
}

func TestRecoverReportsHiddenEventEntry(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	if _, err := store.AppendEvent(taskID, Event{Operation: "one"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.eventsDir(taskID), ".hidden.json")
	if err := os.WriteFile(path, []byte("junk"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := store.Recover()
	if err != nil {
		t.Fatalf("Recover() error = %v", err)
	}
	if len(result.MalformedRecords) != 1 {
		t.Fatalf("Recover() MalformedRecords = %v, want hidden entry reported", result.MalformedRecords)
	}
}

func TestRecoverReportsMalformedSequenceName(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	if _, err := store.AppendEvent(taskID, Event{Operation: "one"}); err != nil {
		t.Fatal(err)
	}
	stray := filepath.Join(store.eventsDir(taskID), "unexpected.txt")
	if err := os.WriteFile(stray, []byte("junk"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := store.Recover()
	if err != nil {
		t.Fatalf("Recover() error = %v", err)
	}
	if len(result.MalformedRecords) != 1 {
		t.Fatalf("Recover() MalformedRecords = %v, want the stray file reported", result.MalformedRecords)
	}
}

// ---- helpers ----

func writeRawManifest(t *testing.T, store *Store, taskID, record string) {
	t.Helper()
	if err := store.WriteManifest(taskID, Manifest{Title: "seed"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.manifestPath(taskID), []byte(record), 0o600); err != nil {
		t.Fatal(err)
	}
}
