package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	store, err := OpenAt(newRoot(t))
	if err != nil {
		t.Fatalf("OpenAt() error = %v", err)
	}
	return store
}

// newRoot returns a fresh root directory with the restrictive permissions the
// store requires, independent of any default ACL on the test temp directory.
func newRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestStateRootHonorsXDGStateHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	root, err := stateRoot()
	if err != nil {
		t.Fatalf("stateRoot() error = %v", err)
	}
	if want := filepath.Join(dir, "akagent"); root != want {
		t.Fatalf("stateRoot() = %q, want %q", root, want)
	}
}

func TestStateRootFallsBackToHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", "") // Windows fallback, kept deterministic
	root, err := stateRoot()
	if err != nil {
		t.Fatalf("stateRoot() error = %v", err)
	}
	if want := filepath.Join(home, ".local", "state", "akagent"); root != want {
		t.Fatalf("stateRoot() = %q, want %q", root, want)
	}
}

func TestStateRootRejectsRelativeXDGStateHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "relative/path")
	if _, err := stateRoot(); !IsKind(err, KindUsage) {
		t.Fatalf("stateRoot() error = %v, want KindUsage", err)
	}
}

func TestOpenCreatesRestrictiveTree(t *testing.T) {
	root := newRoot(t)
	store, err := OpenAt(root)
	if err != nil {
		t.Fatalf("OpenAt() error = %v", err)
	}
	for _, dir := range []string{store.Root(), store.tasksDir(), store.locksDir()} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("stat %s error = %v", dir, err)
		}
		if !info.IsDir() {
			t.Fatalf("%s is not a directory", dir)
		}
		if perms := info.Mode().Perm(); perms != 0o700 {
			t.Fatalf("%s mode = %04o, want 0700", dir, perms)
		}
	}
}

func TestWriteReadManifestRoundTrip(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	manifest := Manifest{
		Title:      "Implement reconciliation",
		Worker:     "local",
		Repository: "example",
		Lifecycle:  "starting",
		Condition:  "active",
	}
	if err := store.WriteManifest(taskID, manifest); err != nil {
		t.Fatalf("WriteManifest() error = %v", err)
	}

	envelope, err := store.ReadManifest(taskID)
	if err != nil {
		t.Fatalf("ReadManifest() error = %v", err)
	}
	if envelope.SchemaVersion != SchemaVersion {
		t.Fatalf("SchemaVersion = %d, want %d", envelope.SchemaVersion, SchemaVersion)
	}
	if envelope.Kind != KindManifest || envelope.TaskID != taskID {
		t.Fatalf("envelope = %#v", envelope)
	}
	got, err := envelope.DecodeManifest()
	if err != nil {
		t.Fatalf("DecodeManifest() error = %v", err)
	}
	if got != manifest {
		t.Fatalf("manifest = %#v, want %#v", got, manifest)
	}
	info, err := os.Stat(store.manifestPath(taskID))
	if err != nil {
		t.Fatal(err)
	}
	if perms := info.Mode().Perm(); perms != 0o600 {
		t.Fatalf("manifest mode = %04o, want 0600", perms)
	}
}

func TestWriteManifestReplacesPrevious(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	if err := store.WriteManifest(taskID, Manifest{Title: "before", Lifecycle: "starting"}); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteManifest(taskID, Manifest{Title: "after", Lifecycle: "running"}); err != nil {
		t.Fatal(err)
	}
	envelope, err := store.ReadManifest(taskID)
	if err != nil {
		t.Fatal(err)
	}
	got, err := envelope.DecodeManifest()
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "after" || got.Lifecycle != "running" {
		t.Fatalf("manifest = %#v, want updated values", got)
	}
}

func TestReadManifestNotFound(t *testing.T) {
	store := openTest(t)
	if _, err := store.ReadManifest(validTaskID(t)); !IsKind(err, KindNotFound) {
		t.Fatalf("ReadManifest() error = %v, want KindNotFound", err)
	}
}

func TestReadManifestUnsafeFilePermissions(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	if err := store.WriteManifest(taskID, Manifest{Title: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(store.manifestPath(taskID), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := store.ReadManifest(taskID)
	if !IsKind(err, KindUnsafe) {
		t.Fatalf("ReadManifest() error = %v, want KindUnsafe", err)
	}
	var storeErr *Error
	if !errors.As(err, &storeErr) {
		t.Fatalf("ReadManifest() error %T is not a *store.Error", err)
	}
	if !strings.Contains(storeErr.Recovery, "chmod 0600") {
		t.Fatalf("ReadManifest() recovery = %q, want chmod hint", storeErr.Recovery)
	}
}

func TestReadUnsafeTaskDirectory(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	if err := store.WriteManifest(taskID, Manifest{Title: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(store.taskDir(taskID), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadManifest(taskID); !IsKind(err, KindUnsafe) {
		t.Fatalf("ReadManifest() error = %v, want KindUnsafe", err)
	}
}

func TestOpenRejectsUnsafeRoot(t *testing.T) {
	root := newRoot(t)
	if _, err := OpenAt(root); err != nil {
		t.Fatalf("OpenAt() error = %v", err)
	}
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenAt(root); !IsKind(err, KindUnsafe) {
		t.Fatalf("OpenAt() error = %v, want KindUnsafe", err)
	}
}

func TestMalformedManifestIsTyped(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	if err := store.WriteManifest(taskID, Manifest{Title: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.manifestPath(taskID), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := store.ReadManifest(taskID)
	if !IsKind(err, KindMalformed) {
		t.Fatalf("ReadManifest() error = %v, want KindMalformed", err)
	}
	if !strings.Contains(err.Error(), store.manifestPath(taskID)) {
		t.Fatalf("ReadManifest() error = %q, want to mention path", err.Error())
	}
}

func TestUnsupportedSchemaVersionIsTyped(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	if err := store.WriteManifest(taskID, Manifest{Title: "x"}); err != nil {
		t.Fatal(err)
	}
	schemaOverride := "999999"
	raw, err := os.ReadFile(store.manifestPath(taskID))
	if err != nil {
		t.Fatal(err)
	}
	corrupted := strings.Replace(string(raw), `"schema_version": 1`, `"schema_version": `+schemaOverride, 1)
	if err := os.WriteFile(store.manifestPath(taskID), []byte(corrupted), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = store.ReadManifest(taskID)
	if !IsKind(err, KindMalformed) {
		t.Fatalf("ReadManifest() error = %v, want KindMalformed", err)
	}
	if !strings.Contains(err.Error(), "schema version") {
		t.Fatalf("ReadManifest() error = %q, want schema version hint", err.Error())
	}
}

func TestAppendAndReadEventsInOrder(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	events := []Event{
		{Operation: "create"},
		{Operation: "publish", Outcome: "ok", Detail: "active"},
		{Operation: "stop", Outcome: "ok"},
	}
	for _, event := range events {
		sequence, err := store.AppendEvent(taskID, event)
		if err != nil {
			t.Fatalf("AppendEvent() error = %v", err)
		}
		if want := len(asFilenames(t, store, taskID)); sequence != want {
			t.Fatalf("AppendEvent() sequence = %d, want %d", sequence, want)
		}
	}

	records, err := store.ReadEvents(taskID)
	if err != nil {
		t.Fatalf("ReadEvents() error = %v", err)
	}
	if len(records) != len(events) {
		t.Fatalf("ReadEvents() len = %d, want %d", len(records), len(events))
	}
	for i, record := range records {
		if record.Sequence != i+1 {
			t.Fatalf("record[%d] Sequence = %d, want %d", i, record.Sequence, i+1)
		}
		if record.Event != events[i] {
			t.Fatalf("record[%d] Event = %#v, want %#v", i, record.Event, events[i])
		}
		if record.ObservedAt.IsZero() {
			t.Fatalf("record[%d] ObservedAt is zero", i)
		}
	}
}

func asFilenames(t *testing.T, store *Store, taskID string) []string {
	t.Helper()
	entries, err := os.ReadDir(store.eventsDir(taskID))
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		files = append(files, entry.Name())
	}
	return files
}

func TestArchiveRoundTrip(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	archive := TaskArchive{
		TaskID:     taskID,
		CapturedAt: time.Now().UTC(),
		Manifest:   Manifest{Title: "archived", Lifecycle: "stopped"},
		Events:     []EventRecord{{Sequence: 1, ObservedAt: time.Now().UTC(), Event: Event{Operation: "stop"}}},
		Git:        GitFacts{Head: "abc123", Dirty: true},
		Terminal:   "safe history",
	}
	if err := store.WriteArchive(taskID, archive); err != nil {
		t.Fatalf("WriteArchive() error = %v", err)
	}
	got, err := store.ReadArchive(taskID)
	if err != nil {
		t.Fatalf("ReadArchive() error = %v", err)
	}
	if got.TaskID != archive.TaskID || got.Manifest != archive.Manifest || got.Git != archive.Git || got.Terminal != archive.Terminal {
		t.Fatalf("archive = %#v, want %#v", got, archive)
	}
	info, err := os.Stat(store.archivePath(taskID))
	if err != nil {
		t.Fatal(err)
	}
	if perms := info.Mode().Perm(); perms != 0o600 {
		t.Fatalf("archive mode = %04o, want 0600", perms)
	}
}

func TestReadEventsEmpty(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	// A task must exist before reading its events.
	if err := store.WriteManifest(taskID, Manifest{Title: "x"}); err != nil {
		t.Fatal(err)
	}
	records, err := store.ReadEvents(taskID)
	if err != nil {
		t.Fatalf("ReadEvents() error = %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("ReadEvents() len = %d, want 0", len(records))
	}
}

func TestReadEventsMalformed(t *testing.T) {
	store := openTest(t)
	taskID := validTaskID(t)
	if _, err := store.AppendEvent(taskID, Event{Operation: "ok"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendEvent(taskID, Event{Operation: "bad"}); err != nil {
		t.Fatal(err)
	}
	badPath := filepath.Join(store.eventsDir(taskID), "000002.json")
	if err := os.WriteFile(badPath, []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadEvents(taskID); !IsKind(err, KindMalformed) {
		t.Fatalf("ReadEvents() error = %v, want KindMalformed", err)
	}
}

func TestTaskIDsListsValidTasks(t *testing.T) {
	store := openTest(t)
	ids := []string{"019fe8f2-ac67-7406-a6e6-2717b2cd31c6", "019fe8f2-ac67-7406-a6e6-2717b2cd31c7"}
	for _, id := range ids {
		if err := store.WriteManifest(id, Manifest{Title: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(store.tasksDir(), "not-a-valid-id!"), 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := store.TaskIDs()
	if err != nil {
		t.Fatalf("TaskIDs() error = %v", err)
	}
	if len(got) != len(ids) {
		t.Fatalf("TaskIDs() = %v, want 2 tasks", got)
	}
	for i, id := range got {
		if strings.Contains(id, "!") {
			t.Fatalf("TaskIDs() included invalid task %q", id)
		}
		if !contains(ids, id) {
			t.Fatalf("TaskIDs() = %v, unexpected", got)
		}
		if i > 0 && got[i-1] >= got[i] {
			t.Fatalf("TaskIDs() = %v, not sorted", got)
		}
	}
}

func TestInvalidTaskIDIsTypedUsage(t *testing.T) {
	store := openTest(t)
	for _, action := range []func() error{
		func() error { return store.WriteManifest("../evil", Manifest{Title: "x"}) },
		func() error { _, err := store.ReadManifest("../evil"); return err },
		func() error { _, err := store.AppendEvent("../evil", Event{Operation: "x"}); return err },
		func() error { _, err := store.Lock("../evil"); return err },
	} {
		if err := action(); !IsKind(err, KindUsage) {
			t.Fatalf("error = %v, want KindUsage", err)
		}
	}
	entries, err := os.ReadDir(store.tasksDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("invalid task ID touched the filesystem: %v entries created", len(entries))
	}
}

func validTaskID(t *testing.T) string {
	t.Helper()
	return "019fe8f2-ac67-7406-a6e6-2717b2cd31c6"
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func withConcurrencyLockWait(t *testing.T, fn func()) {
	t.Helper()
	originalWait := lockWait
	originalRetry := lockRetryAfter
	lockWait = 2 * time.Second
	lockRetryAfter = 2 * time.Millisecond
	defer func() {
		lockWait = originalWait
		lockRetryAfter = originalRetry
	}()
	fn()
}

func withShortLockWait(t *testing.T, fn func()) {
	t.Helper()
	originalWait := lockWait
	originalRetry := lockRetryAfter
	lockWait = 30 * time.Millisecond
	lockRetryAfter = 2 * time.Millisecond
	defer func() {
		lockWait = originalWait
		lockRetryAfter = originalRetry
	}()
	fn()
}
