// Package store provides a secure worker-local on-disk store for task
// manifests and append-only events. It is independent of tmux, Git worktrees,
// credentials, and CLI command parsing so future lifecycle code can build on
// it. See docs/storage.md for the layout, schema, and concurrency contract.
package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// Store is a worker-local on-disk store.
type Store struct {
	root string
	// unlockFn, when set, replaces the flock unlock during release so tests
	// can exercise release failures. It is nil in normal operation.
	unlockFn func() error
}

// lockWait and lockRetryAfter bound how long Lock waits for a contended
// per-task lock before returning a retryable error. They are variables so
// tests can shorten the backoff.
var (
	lockWait       = 2 * time.Second
	lockRetryAfter = 20 * time.Millisecond
)

// tempPrefix names temporary files created during atomic writes. Recovery
// removes files with this prefix that interrupted writes left behind.
const tempPrefix = ".akagent-write-"

// eventSequenceWidth is the zero-padded width of event file base names.
const eventSequenceWidth = 6

var (
	taskIDPattern    = regexp.MustCompile(`^[0-9a-zA-Z-]{1,64}$`)
	eventFilePattern = regexp.MustCompile(fmt.Sprintf(`^\d{%d}\.json$`, eventSequenceWidth))
)

// Open returns a Store rooted at the XDG state directory, creating the
// directory tree with restrictive permissions when it is missing.
func Open() (*Store, error) {
	root, err := stateRoot()
	if err != nil {
		return nil, err
	}
	return OpenAt(root)
}

// OpenAt returns a Store rooted at root. It creates the directory tree with
// restrictive permissions when missing and rejects an existing tree whose
// directories are accessible by other users or are symbolic links.
func OpenAt(root string) (*Store, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, internalError("resolve the state root", fmt.Sprintf("Use an absolute path: %s", root))
	}
	s := &Store{root: absolute}
	if err := s.ensureRoot(); err != nil {
		return nil, err
	}
	return s, nil
}

// Root returns the absolute state root directory.
func (s *Store) Root() string { return s.root }

// stateRoot resolves the XDG state root for the akagent application.
func stateRoot() (string, error) {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		if !filepath.IsAbs(dir) {
			return "", newError(KindUsage, "XDG_STATE_HOME must be an absolute path", "Set XDG_STATE_HOME to an absolute path or unset it")
		}
		return filepath.Join(dir, "akagent"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", internalError("resolve the home directory", "Set HOME to a writable directory and retry")
	}
	if home == "" {
		return "", newError(KindUsage, "Cannot resolve the home directory", "Set HOME to a writable directory and retry")
	}
	return filepath.Join(home, ".local", "state", "akagent"), nil
}

// WriteManifest atomically replaces the task manifest. The previous valid
// manifest remains intact if the write fails. It acquires the per-task lock.
func (s *Store) WriteManifest(taskID string, manifest Manifest) error {
	if err := validateTaskID(taskID); err != nil {
		return err
	}
	return s.WithLock(taskID, func() error {
		return s.writeManifestLocked(taskID, manifest)
	})
}

// UpdateManifest serializes a read-modify-write mutation under the task lock.
// The callback must only change durable task state and must not perform another
// mutation on the same task.
func (s *Store) UpdateManifest(taskID string, update func(*Manifest) error) (Manifest, error) {
	if err := validateTaskID(taskID); err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	err := s.WithLock(taskID, func() error {
		envelope, err := s.ReadManifest(taskID)
		if err != nil {
			return err
		}
		manifest, err = envelope.DecodeManifest()
		if err != nil {
			return err
		}
		if err := update(&manifest); err != nil {
			return err
		}
		return s.writeManifestLocked(taskID, manifest)
	})
	return manifest, err
}

// ReadManifest reads and validates the task manifest envelope and its typed
// payload. Use DecodeManifest on the result to obtain the payload again.
func (s *Store) ReadManifest(taskID string) (Envelope, error) {
	if err := validateTaskID(taskID); err != nil {
		return Envelope{}, err
	}
	path := s.manifestPath(taskID)
	if err := s.checkTaskDir(taskID); err != nil {
		return Envelope{}, err
	}
	data, err := s.readOwnedFile(path)
	if err != nil {
		if IsKind(err, KindNotFound) {
			return Envelope{}, newError(KindNotFound, fmt.Sprintf("No manifest found for task %s", taskID), fmt.Sprintf("Start task %s before inspecting it", taskID))
		}
		return Envelope{}, err
	}
	envelope, err := decodeEnvelope(path, data, KindManifest, taskID)
	if err != nil {
		return Envelope{}, err
	}
	if _, err := envelope.DecodeManifest(); err != nil {
		return Envelope{}, malformedError(
			fmt.Sprintf("Malformed manifest payload for task %s", taskID),
			fmt.Sprintf("Inspect and repair %s", path))
	}
	return envelope, nil
}

// CreateManifest atomically creates a task manifest if it does not exist.
// It returns the existing manifest and created=false for an idempotent retry.
func (s *Store) CreateManifest(taskID string, manifest Manifest) (bool, Manifest, error) {
	if err := validateTaskID(taskID); err != nil {
		return false, Manifest{}, err
	}
	var created bool
	var existing Manifest
	err := s.WithLock(taskID, func() error {
		envelope, err := s.ReadManifest(taskID)
		if err == nil {
			existing, err = envelope.DecodeManifest()
			return err
		}
		if !IsKind(err, KindNotFound) {
			return err
		}
		if err := s.writeManifestLocked(taskID, manifest); err != nil {
			return err
		}
		created = true
		existing = manifest
		return nil
	})
	return created, existing, err
}

// AppendEvent appends a single immutable event to the task's append-only
// history and returns its 1-based sequence number. It acquires the per-task
// lock.
func (s *Store) AppendEvent(taskID string, event Event) (int, error) {
	if err := validateTaskID(taskID); err != nil {
		return 0, err
	}
	var sequence int
	err := s.WithLock(taskID, func() error {
		if err := s.ensureTaskDir(taskID); err != nil {
			return err
		}
		envelope, err := eventEnvelope(taskID, event)
		if err != nil {
			return internalError("encode a task event", "Retry the operation")
		}
		encoded, err := encodeRecord(envelope)
		if err != nil {
			return err
		}
		next, err := s.nextSequence(taskID)
		if err != nil {
			return err
		}
		if err := s.atomicallyWrite(s.eventPath(taskID, next), encoded); err != nil {
			return err
		}
		sequence = next
		return nil
	})
	return sequence, err
}

// ReadEvents returns the task's events in sequence order. A task with no
// events yields an empty slice. Event file names must form a contiguous,
// zero-padded sequence starting at 1; malformed history is reported rather
// than silently skipped.
func (s *Store) ReadEvents(taskID string) ([]EventRecord, error) {
	if err := validateTaskID(taskID); err != nil {
		return nil, err
	}
	if err := s.checkTaskDir(taskID); err != nil {
		return nil, err
	}
	exists, err := s.eventsDirStatus(taskID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return []EventRecord{}, nil
	}
	sequences, files, err := s.listEventSequences(taskID)
	if err != nil {
		return nil, err
	}
	if err := checkSequenceContiguity(sequences, taskID); err != nil {
		return nil, err
	}

	records := make([]EventRecord, 0, len(sequences))
	for _, sequence := range sequences {
		path := filepath.Join(s.eventsDir(taskID), files[sequence])
		data, err := s.readOwnedFile(path)
		if err != nil {
			return nil, err
		}
		envelope, err := decodeEnvelope(path, data, KindEvent, taskID)
		if err != nil {
			return nil, err
		}
		event, err := envelope.DecodeEvent()
		if err != nil {
			return nil, malformedError(
				fmt.Sprintf("Malformed event payload for task %s at sequence %d", taskID, sequence),
				fmt.Sprintf("Inspect and repair %s", path))
		}
		records = append(records, EventRecord{Sequence: sequence, ObservedAt: envelope.ObservedAt, Event: event})
	}
	return records, nil
}

// TaskIDs returns the stored task IDs in sorted order.
func (s *Store) TaskIDs() ([]string, error) {
	entries, err := os.ReadDir(s.tasksDir())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, internalError("list task state", fmt.Sprintf("Check %s and retry", s.tasksDir()))
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if err := validateTaskID(entry.Name()); err == nil {
			ids = append(ids, entry.Name())
		}
	}
	sort.Strings(ids)
	return ids, nil
}

// Lock acquires the per-task advisory lock, waiting a short bounded time for
// a contended lock before returning a retryable error. The returned release
// function is idempotent.
func (s *Store) Lock(taskID string) (func() error, error) {
	if err := validateTaskID(taskID); err != nil {
		return nil, err
	}
	if err := s.ensureDir(s.locksDir(), "locks directory"); err != nil {
		return nil, err
	}
	path := s.lockPath(taskID)
	lockFile, err := s.openOwnedWithFlags(path, unix.O_RDWR|unix.O_CREAT, 0o600, false)
	if err != nil {
		return nil, err
	}
	if info, statErr := lockFile.Stat(); statErr != nil {
		_ = lockFile.Close()
		return nil, internalError("inspect task lock", fmt.Sprintf("Check %s and retry", path))
	} else if perms := info.Mode().Perm(); perms&0o077 != 0 {
		_ = lockFile.Close()
		return nil, unsafeError(fmt.Sprintf("Lock file %s is accessible by other users (mode %04o)", path, perms), fmt.Sprintf("Run `chmod 0600 %s` and retry", path))
	}

	tryLock := func() (bool, error) {
		err := unix.Flock(int(lockFile.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return true, nil
		}
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return false, nil
		}
		return false, err
	}
	acquired, err := tryLock()
	if err != nil {
		_ = lockFile.Close()
		return nil, internalError("lock task state", "Retry the operation")
	}
	if !acquired {
		deadline := time.Now().Add(lockWait)
		for time.Now().Before(deadline) {
			time.Sleep(lockRetryAfter)
			acquired, err = tryLock()
			if err != nil {
				_ = lockFile.Close()
				return nil, internalError("lock task state", "Retry the operation")
			}
			if acquired {
				break
			}
		}
		if !acquired {
			_ = lockFile.Close()
			return nil, retryableError(
				fmt.Sprintf("Task %s state is locked by another writer", taskID),
				fmt.Sprintf("Wait for the active writer, then retry the operation for task %s", taskID))
		}
	}

	var released bool
	return func() error {
		if released {
			return nil
		}
		released = true
		var releaseErr error
		if s.unlockFn != nil {
			releaseErr = s.unlockFn()
		} else {
			releaseErr = unix.Flock(int(lockFile.Fd()), unix.LOCK_UN)
		}
		closeErr := lockFile.Close()
		if releaseErr != nil {
			return releaseErr
		}
		return closeErr
	}, nil
}

// WithRepositoryLock runs fn while holding a repository-scoped advisory lock.
// Repository locks use a distinct namespace from task locks.
func (s *Store) WithRepositoryLock(name string, fn func() error) error {
	if err := validateRepositoryName(name); err != nil {
		return err
	}
	return s.WithLock("repo-"+name, fn)
}

// WithLock runs fn while holding the per-task advisory lock. It returns a
// retryable error when the lock is contended, returns fn's error when the
// callback fails, and surfaces failures to release the lock.
func (s *Store) WithLock(taskID string, fn func() error) error {
	release, err := s.Lock(taskID)
	if err != nil {
		return err
	}
	fnErr := fn()
	releaseErr := release()
	if fnErr != nil && releaseErr != nil {
		return &Error{
			Kind:     KindPartial,
			Message:  "Task callback and per-task lock release both failed",
			Recovery: "Inspect both failures, then retry the operation",
			Err:      errors.Join(fnErr, releaseErr),
		}
	}
	if fnErr != nil {
		return fnErr
	}
	if releaseErr != nil {
		return newError(KindInternal, "Failed to release the per-task lock", "Retry the operation")
	}
	return nil
}

// RecoveryResult reports what Recover found and changed.
type RecoveryResult struct {
	StaleFilesRemoved []string
	MalformedRecords  []string
	SkippedLocked     []string
}

// Recover repairs interrupted writes and reports malformed records. Under
// each task lock it removes temporary files left by interrupted writes and
// validates stored manifest and event files without deleting them. Tasks
// whose lock is contended are reported and left alone.
func (s *Store) Recover() (RecoveryResult, error) {
	if err := s.ensureRoot(); err != nil {
		return RecoveryResult{}, err
	}
	var result RecoveryResult
	taskIDs, err := s.TaskIDs()
	if err != nil {
		return RecoveryResult{}, err
	}
	for _, taskID := range taskIDs {
		release, err := s.Lock(taskID)
		if err != nil {
			if IsKind(err, KindLocked) {
				result.SkippedLocked = append(result.SkippedLocked, taskID)
				continue
			}
			return RecoveryResult{}, err
		}
		err = s.recoverTaskLocked(taskID, &result)
		releaseErr := release()
		if err != nil {
			return RecoveryResult{}, err
		}
		if releaseErr != nil {
			return RecoveryResult{}, newError(KindInternal, fmt.Sprintf("Failed to release the lock for task %s during recovery", taskID), "Retry recovery")
		}
	}
	return result, nil
}

// ---- internals ----

func (s *Store) recoverTaskLocked(taskID string, result *RecoveryResult) error {
	if err := s.checkTaskDir(taskID); err != nil {
		if IsKind(err, KindNotFound) {
			return nil
		}
		result.MalformedRecords = append(result.MalformedRecords, err.Error())
		return nil
	}
	if err := s.removeStaleTemps(taskID, result); err != nil {
		return err
	}

	if err := s.validateManifestForRecovery(taskID, result); err != nil {
		return err
	}
	if err := s.validateEventsForRecovery(taskID, result); err != nil {
		return err
	}
	if err := s.validateResourcesForRecovery(taskID, result); err != nil {
		return err
	}
	return s.validateArchiveForRecovery(taskID, result)
}

func (s *Store) removeStaleTemps(taskID string, result *RecoveryResult) error {
	taskDir := s.taskDir(taskID)
	root, err := s.openOwned(taskDir, true)
	if err != nil {
		return err
	}
	defer root.Close()
	return removeStaleTempsFromDir(root, taskDir, result)
}

func removeStaleTempsFromDir(dir *os.File, dirPath string, result *RecoveryResult) error {
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return internalError(fmt.Sprintf("scan task directory %s for stale files", dirPath), fmt.Sprintf("Check %s and retry", dirPath))
	}
	for _, entry := range entries {
		name := entry.Name()
		path := filepath.Join(dirPath, name)
		if strings.HasPrefix(name, tempPrefix) {
			flags := 0
			if entry.IsDir() {
				flags = unix.AT_REMOVEDIR
			}
			if err := unix.Unlinkat(int(dir.Fd()), name, flags); err != nil {
				return internalError(fmt.Sprintf("remove stale temporary file %s", path), fmt.Sprintf("Remove %s manually, then retry", path))
			}
			result.StaleFilesRemoved = append(result.StaleFilesRemoved, path)
			continue
		}
		if !entry.IsDir() {
			continue
		}
		fd, err := unix.Openat(int(dir.Fd()), name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if err != nil {
			return pathOpenError(path, err)
		}
		child := os.NewFile(uintptr(fd), path)
		if child == nil {
			_ = unix.Close(fd)
			return internalError(fmt.Sprintf("open %s during recovery", path), "Retry recovery")
		}
		err = removeStaleTempsFromDir(child, path, result)
		closeErr := child.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return internalError(fmt.Sprintf("close %s during recovery", path), "Retry recovery")
		}
	}
	return nil
}

func (s *Store) validateManifestForRecovery(taskID string, result *RecoveryResult) error {
	path := s.manifestPath(taskID)
	data, err := s.readOwnedFile(path)
	if err != nil {
		if !IsKind(err, KindNotFound) {
			result.MalformedRecords = append(result.MalformedRecords, err.Error())
		}
		return nil
	}
	envelope, err := decodeEnvelope(path, data, KindManifest, taskID)
	if err != nil {
		result.MalformedRecords = append(result.MalformedRecords, path)
		return nil
	}
	if _, err := envelope.DecodeManifest(); err != nil {
		result.MalformedRecords = append(result.MalformedRecords, path)
	}
	return nil
}

func (s *Store) validateResourcesForRecovery(taskID string, result *RecoveryResult) error {
	ids, err := s.ResourceIDs(taskID)
	if err != nil {
		if !IsKind(err, KindNotFound) {
			result.MalformedRecords = append(result.MalformedRecords, err.Error())
		}
		return nil
	}
	for _, resourceID := range ids {
		if _, err := s.ReadResource(taskID, resourceID); err != nil {
			result.MalformedRecords = append(result.MalformedRecords, s.resourceManifestPath(taskID, resourceID))
			continue
		}
		if _, err := s.ReadResourceEvents(taskID, resourceID); err != nil {
			result.MalformedRecords = append(result.MalformedRecords, s.resourceEventsDir(taskID, resourceID))
		}
		if _, err := s.ReadResourceArchive(taskID, resourceID); err != nil && !IsKind(err, KindNotFound) {
			result.MalformedRecords = append(result.MalformedRecords, s.resourceArchivePath(taskID, resourceID))
		}
	}
	return nil
}

func (s *Store) validateEventsForRecovery(taskID string, result *RecoveryResult) error {
	exists, err := s.eventsDirStatus(taskID)
	if err != nil {
		result.MalformedRecords = append(result.MalformedRecords, err.Error())
		return nil
	}
	if !exists {
		return nil
	}
	sequences, files, err := s.listEventSequences(taskID)
	if err != nil {
		result.MalformedRecords = append(result.MalformedRecords, err.Error())
		return nil
	}
	if err := checkSequenceContiguity(sequences, taskID); err != nil {
		result.MalformedRecords = append(result.MalformedRecords, err.Error())
	}
	for _, sequence := range sequences {
		path := filepath.Join(s.eventsDir(taskID), files[sequence])
		data, err := s.readOwnedFile(path)
		if err != nil {
			result.MalformedRecords = append(result.MalformedRecords, err.Error())
			continue
		}
		envelope, err := decodeEnvelope(path, data, KindEvent, taskID)
		if err != nil {
			result.MalformedRecords = append(result.MalformedRecords, path)
			continue
		}
		if _, err := envelope.DecodeEvent(); err != nil {
			result.MalformedRecords = append(result.MalformedRecords, path)
		}
	}
	return nil
}

func (s *Store) writeManifestLocked(taskID string, manifest Manifest) error {
	if err := s.ensureTaskDir(taskID); err != nil {
		return err
	}
	envelope, err := manifestEnvelope(taskID, manifest)
	if err != nil {
		return internalError("encode the task manifest", "Retry the operation")
	}
	encoded, err := encodeRecord(envelope)
	if err != nil {
		return err
	}
	return s.atomicallyWrite(s.manifestPath(taskID), encoded)
}

// nextSequence returns the next event sequence number (1-based) under the
// per-task lock, rejecting a malformed or gapped history.
func (s *Store) nextSequence(taskID string) (int, error) {
	sequences, _, err := s.listEventSequences(taskID)
	if err != nil {
		return 0, err
	}
	if err := checkSequenceContiguity(sequences, taskID); err != nil {
		return 0, err
	}
	if len(sequences) == 0 {
		return 1, nil
	}
	return sequences[len(sequences)-1] + 1, nil
}

// checkTaskDir validates that the task directory exists, is not a symbolic
// link, and is not accessible by other users.
func (s *Store) checkTaskDir(taskID string) error {
	taskDir := s.taskDir(taskID)
	file, err := s.openOwned(taskDir, true)
	if err != nil {
		if IsKind(err, KindNotFound) {
			return newError(KindNotFound, fmt.Sprintf("No tasks found for task ID %s", taskID), fmt.Sprintf("Create task %s before reading its state", taskID))
		}
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return internalError(fmt.Sprintf("inspect the task directory for %s", taskID), fmt.Sprintf("Check %s and retry", taskDir))
	}
	return checkOwnedDir(taskDir, "task directory", info.Mode())
}

// eventsDirStatus validates the task events directory. A missing directory
// (no events recorded) yields exists=false without error.
func (s *Store) eventsDirStatus(taskID string) (bool, error) {
	dir := s.eventsDir(taskID)
	file, err := s.openOwned(dir, true)
	if err != nil {
		if IsKind(err, KindNotFound) {
			return false, nil
		}
		return false, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return false, internalError(fmt.Sprintf("inspect the events directory for task %s", taskID), fmt.Sprintf("Check %s and retry", dir))
	}
	return true, checkOwnedDir(dir, "events directory", info.Mode())
}

func (s *Store) ensureRoot() error {
	for _, entry := range []struct {
		dir   string
		label string
	}{
		{s.root, "state root"},
		{s.tasksDir(), "tasks directory"},
		{s.repositoriesDir(), "repositories directory"},
		{s.locksDir(), "locks directory"},
	} {
		if err := s.ensureDir(entry.dir, entry.label); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ensureTaskDir(taskID string) error {
	for _, entry := range []struct {
		dir   string
		label string
	}{
		{s.taskDir(taskID), "task directory"},
		{s.eventsDir(taskID), "events directory"},
	} {
		if err := s.ensureDir(entry.dir, entry.label); err != nil {
			return err
		}
	}
	return nil
}

// ensureDir creates dir with restrictive 0700 permissions when missing, or
// validates an existing directory's mode, type, and symlink ownership.
func (s *Store) ensureDir(dir, label string) error {
	if filepath.Clean(dir) == filepath.Clean(s.root) {
		info, err := os.Lstat(dir)
		if err == nil {
			return checkOwnedDir(dir, label, info.Mode())
		}
		if !errors.Is(err, os.ErrNotExist) {
			return internalError(fmt.Sprintf("inspect the %s at %s", label, dir), fmt.Sprintf("Retry after checking %s", dir))
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return internalError(fmt.Sprintf("create the %s at %s", label, dir), fmt.Sprintf("Ensure %s is writable, then retry", filepath.Dir(dir)))
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			return internalError(fmt.Sprintf("restrict permissions on the %s at %s", label, dir), fmt.Sprintf("Run `chmod 0700 %s` and retry", dir))
		}
		return nil
	}
	return s.ensureOwnedDirPath(dir, label)
}

func (s *Store) ensureOwnedDirPath(dir, label string) error {
	rel, err := s.relativePath(dir)
	if err != nil {
		return err
	}
	rootFD, err := unix.Open(s.root, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return pathOpenError(dir, err)
	}
	currentFD := rootFD
	defer func() { _ = unix.Close(currentFD) }()
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "" || part == "." || part == ".." {
			return unsafePathError(fmt.Sprintf("State path %s contains an unsafe component", dir), fmt.Sprintf("Repair %s and retry", dir))
		}
		fd, openErr := unix.Openat(currentFD, part, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if errors.Is(openErr, unix.ENOENT) {
			if mkdirErr := unix.Mkdirat(currentFD, part, 0o700); mkdirErr != nil && !errors.Is(mkdirErr, unix.EEXIST) {
				return internalError(fmt.Sprintf("create the %s at %s", label, dir), fmt.Sprintf("Ensure %s is writable, then retry", dir))
			}
			fd, openErr = unix.Openat(currentFD, part, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		}
		if openErr != nil {
			return pathOpenError(dir, openErr)
		}
		var stat unix.Stat_t
		if statErr := unix.Fstat(fd, &stat); statErr != nil {
			_ = unix.Close(fd)
			return internalError(fmt.Sprintf("inspect the %s at %s", label, dir), fmt.Sprintf("Check %s and retry", dir))
		}
		mode := os.FileMode(stat.Mode & 0o777)
		if stat.Mode&unix.S_IFMT == unix.S_IFDIR {
			mode |= os.ModeDir
		}
		if err := checkOwnedDir(dir, label, mode); err != nil {
			_ = unix.Close(fd)
			return err
		}
		_ = unix.Close(currentFD)
		currentFD = fd
	}
	return nil
}

// checkOwnedDir rejects a directory that is a symbolic link, not a
// directory, or accessible by other users.
func checkOwnedDir(dir, label string, mode os.FileMode) error {
	if mode&os.ModeSymlink != 0 {
		return unsafePathError(
			fmt.Sprintf("%s path %s is a symbolic link", label, dir),
			fmt.Sprintf("Remove %s and retry", dir))
	}
	if !mode.IsDir() {
		return newError(KindInternal, fmt.Sprintf("%s path %s is not a directory", label, dir), fmt.Sprintf("Remove or relocate %s, then retry", dir))
	}
	return checkDirPermissions(dir, mode)
}

// checkDirPermissions rejects a directory other users can access.
func checkDirPermissions(dir string, mode os.FileMode) error {
	if perms := mode.Perm(); perms&0o077 != 0 {
		return unsafeError(
			fmt.Sprintf("State directory %s is accessible by other users (mode %04o)", dir, perms),
			fmt.Sprintf("Run `chmod 0700 %s` and retry", dir))
	}
	return nil
}

// readOwnedFile opens and reads a record through descriptor-relative
// O_NOFOLLOW traversal. This rejects symlinks at every component and avoids
// a separate path-based Lstat/ReadFile check/use race.
func (s *Store) readOwnedFile(path string) ([]byte, error) {
	file, err := s.openOwned(path, false)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, internalError(fmt.Sprintf("inspect %s", path), fmt.Sprintf("Check %s and retry", path))
	}
	if !info.Mode().IsRegular() {
		return nil, unsafePathError(
			fmt.Sprintf("State file %s is not a regular file", path),
			fmt.Sprintf("Inspect and repair %s, then retry", path))
	}
	if perms := info.Mode().Perm(); perms&0o077 != 0 {
		return nil, unsafeError(
			fmt.Sprintf("State file %s is accessible by other users (mode %04o)", path, perms),
			fmt.Sprintf("Run `chmod 0600 %s` and retry", path))
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, internalError(fmt.Sprintf("read %s", path), fmt.Sprintf("Check %s and retry", path))
	}
	return data, nil
}

// openOwned opens path relative to the state-root descriptor with O_NOFOLLOW
// on every component. The returned file descriptor is therefore stable with
// respect to path replacement and cannot traverse an intermediate symlink.
func (s *Store) openOwned(path string, wantDir bool) (*os.File, error) {
	return s.openOwnedWithFlags(path, unix.O_RDONLY, 0, wantDir)
}

func (s *Store) openOwnedWithFlags(path string, finalFlags int, perm os.FileMode, wantDir bool) (*os.File, error) {
	rel, err := s.relativePath(path)
	if err != nil {
		return nil, err
	}
	rootFD, err := unix.Open(s.root, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, pathOpenError(path, err)
	}
	currentFD := rootFD
	parts := strings.Split(rel, string(filepath.Separator))
	for i, part := range parts {
		if part == "" || part == "." || part == ".." {
			_ = unix.Close(currentFD)
			return nil, unsafePathError(fmt.Sprintf("State path %s contains an unsafe component", path), fmt.Sprintf("Repair %s and retry", path))
		}
		flags := unix.O_RDONLY
		if i == len(parts)-1 {
			flags = finalFlags
		}
		flags |= unix.O_CLOEXEC | unix.O_NOFOLLOW
		fd, openErr := unix.Openat(currentFD, part, flags, uint32(perm.Perm()))
		if openErr != nil {
			_ = unix.Close(currentFD)
			return nil, pathOpenError(path, openErr)
		}
		if i < len(parts)-1 {
			var stat unix.Stat_t
			if statErr := unix.Fstat(fd, &stat); statErr != nil {
				_ = unix.Close(fd)
				_ = unix.Close(currentFD)
				return nil, internalError(fmt.Sprintf("inspect %s", path), fmt.Sprintf("Check %s and retry", path))
			}
			if stat.Mode&unix.S_IFMT != unix.S_IFDIR {
				_ = unix.Close(fd)
				_ = unix.Close(currentFD)
				return nil, unsafePathError(fmt.Sprintf("State path %s contains a non-directory component", path), fmt.Sprintf("Repair %s and retry", path))
			}
		}
		_ = unix.Close(currentFD)
		currentFD = fd
	}
	file := os.NewFile(uintptr(currentFD), path)
	if file == nil {
		_ = unix.Close(currentFD)
		return nil, internalError(fmt.Sprintf("open %s", path), fmt.Sprintf("Check %s and retry", path))
	}
	info, statErr := file.Stat()
	if statErr != nil {
		_ = file.Close()
		return nil, internalError(fmt.Sprintf("inspect %s", path), fmt.Sprintf("Check %s and retry", path))
	}
	if wantDir && !info.IsDir() {
		_ = file.Close()
		return nil, unsafePathError(fmt.Sprintf("State path %s is not a directory", path), fmt.Sprintf("Repair %s and retry", path))
	}
	if !wantDir && !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, unsafePathError(fmt.Sprintf("State path %s is not a regular file", path), fmt.Sprintf("Repair %s and retry", path))
	}
	return file, nil
}

func pathOpenError(path string, err error) *Error {
	switch {
	case errors.Is(err, unix.ENOENT):
		return newError(KindNotFound, fmt.Sprintf("No record found at %s", path), fmt.Sprintf("Check %s and retry", path))
	case errors.Is(err, unix.ELOOP), errors.Is(err, unix.ENOTDIR):
		return unsafePathError(fmt.Sprintf("State path %s contains a symbolic link or non-directory component", path), fmt.Sprintf("Repair %s and retry", path))
	default:
		return internalError(fmt.Sprintf("open %s", path), fmt.Sprintf("Check %s and retry", path))
	}
}

func (s *Store) relativePath(path string) (string, error) {
	rel, err := filepath.Rel(s.root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", unsafePathError(fmt.Sprintf("State path %s escapes the configured root", path), fmt.Sprintf("Repair %s and retry", path))
	}
	return rel, nil
}

func validateTaskID(taskID string) error {
	if taskID == "" || !taskIDPattern.MatchString(taskID) {
		return newError(KindUsage, fmt.Sprintf("Invalid task ID %q", taskID), "Use a UUIDv7 generated by `akagent id generate`")
	}
	return nil
}

func encodeRecord(envelope Envelope) ([]byte, error) {
	encoded, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return nil, internalError("encode a state record", "Retry the operation")
	}
	return append(encoded, '\n'), nil
}

// parseEventSequence parses the zero-padded sequence from an event file name.
// It rejects non-conforming names, including hidden temporary files.
func parseEventSequence(name string) (int, bool) {
	if !eventFilePattern.MatchString(name) {
		return 0, false
	}
	sequence, err := strconv.Atoi(name[:len(name)-len(".json")])
	if err != nil || sequence < 1 {
		return 0, false
	}
	return sequence, true
}

// listEventSequences lists the task's stored event files and returns their
// sequences in sorted order. Files that do not use the zero-padded sequence
// naming are reported as malformed rather than silently skipped.
func (s *Store) listEventSequences(taskID string) ([]int, map[int]string, error) {
	dir := s.eventsDir(taskID)
	file, err := s.openOwned(dir, true)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()
	entries, err := file.ReadDir(-1)
	if err != nil {
		return nil, nil, internalError(fmt.Sprintf("list events for task %s", taskID), fmt.Sprintf("Check %s and retry", dir))
	}
	sequences := make([]int, 0, len(entries))
	files := make(map[int]string, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			return nil, nil, malformedError(
				fmt.Sprintf("Event entry %q for task %s is a directory", name, taskID),
				fmt.Sprintf("Inspect and remove %s", filepath.Join(dir, name)))
		}
		if strings.HasPrefix(name, ".") {
			return nil, nil, malformedError(
				fmt.Sprintf("Event entry %q for task %s is hidden", name, taskID),
				fmt.Sprintf("Inspect and remove %s", filepath.Join(dir, name)))
		}
		sequence, ok := parseEventSequence(name)
		if !ok {
			return nil, nil, malformedError(
				fmt.Sprintf("Event file %q for task %s has a malformed sequence name", name, taskID),
				fmt.Sprintf("Inspect and repair %s", filepath.Join(dir, name)))
		}
		if _, exists := files[sequence]; exists {
			return nil, nil, malformedError(
				fmt.Sprintf("Duplicate event sequence %d for task %s", sequence, taskID),
				fmt.Sprintf("Inspect and repair %s", dir))
		}
		sequences = append(sequences, sequence)
		files[sequence] = name
	}
	sort.Ints(sequences)
	return sequences, files, nil
}

// checkSequenceContiguity rejects event histories with gaps or sequences that
// do not start at 1.
func checkSequenceContiguity(sequences []int, taskID string) error {
	for i, sequence := range sequences {
		if sequence != i+1 {
			return malformedError(
				fmt.Sprintf("Event history for task %s has a gap at sequence %d", taskID, i+1),
				fmt.Sprintf("Inspect and repair the events for %s", taskID))
		}
	}
	return nil
}

// decodeEnvelope parses and validates a record envelope against the expected
// kind and task ID. Records with a schema version other than the current one
// or without an observation time are rejected rather than misinterpreted.
func decodeEnvelope(path string, data []byte, kind, taskID string) (Envelope, error) {
	var envelope Envelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return Envelope{}, malformedError(
			fmt.Sprintf("Malformed %s record at %s", kind, path),
			fmt.Sprintf("Inspect and repair or remove %s, then retry", path))
	}
	if envelope.SchemaVersion != SchemaVersion {
		return Envelope{}, malformedError(
			fmt.Sprintf("Unsupported schema version %d in %s record at %s", envelope.SchemaVersion, kind, path),
			fmt.Sprintf("Upgrade akagent or repair %s", path))
	}
	if envelope.Kind != kind {
		return Envelope{}, malformedError(
			fmt.Sprintf("Record at %s has kind %q but expected %q", path, envelope.Kind, kind),
			fmt.Sprintf("Inspect and repair %s", path))
	}
	if envelope.TaskID != taskID {
		return Envelope{}, malformedError(
			fmt.Sprintf("Record at %s belongs to task %s, not task %s", path, envelope.TaskID, taskID),
			fmt.Sprintf("Inspect and repair %s", path))
	}
	if envelope.ObservedAt.IsZero() {
		return Envelope{}, malformedError(
			fmt.Sprintf("%s record at %s is missing its observation time", kind, path),
			fmt.Sprintf("Inspect and repair %s", path))
	}
	return envelope, nil
}

// atomicallyWrite replaces path with data through the descriptor of its
// containing directory. Temp creation and rename therefore cannot follow a
// swapped parent symlink or redirect the replacement outside the store.
func (s *Store) atomicallyWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	target := filepath.Base(path)
	parent, err := s.openOwned(dir, true)
	if err != nil {
		return err
	}
	defer parent.Close()

	var temporaryName string
	var temporary *os.File
	for attempt := 0; attempt < 8; attempt++ {
		temporaryName, err = randomTempName()
		if err != nil {
			return internalError("prepare an atomic-write temporary name", "Retry the operation")
		}
		fd, openErr := unix.Openat(int(parent.Fd()), temporaryName, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
		if openErr == nil {
			temporary = os.NewFile(uintptr(fd), filepath.Join(dir, temporaryName))
			break
		}
		if !errors.Is(openErr, unix.EEXIST) {
			return internalError(fmt.Sprintf("prepare a temporary file beside %s", path), fmt.Sprintf("Ensure %s is writable and has free space, then retry", dir))
		}
	}
	if temporary == nil {
		return internalError(fmt.Sprintf("prepare a temporary file beside %s", path), "Retry the operation")
	}
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = unix.Unlinkat(int(parent.Fd()), temporaryName, 0)
		}
	}()

	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return internalError(fmt.Sprintf("write the temporary file for %s", path), fmt.Sprintf("Ensure %s is writable, then retry", dir))
	}
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return internalError(fmt.Sprintf("restrict the temporary file for %s", path), "Retry the operation")
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return internalError(fmt.Sprintf("flush the temporary file for %s", path), "Retry the operation")
	}
	if err := temporary.Close(); err != nil {
		return internalError(fmt.Sprintf("close the temporary file for %s", path), "Retry the operation")
	}
	if err := checkReplacementTarget(int(parent.Fd()), target, path); err != nil {
		return err
	}
	if err := unix.Renameat(int(parent.Fd()), temporaryName, int(parent.Fd()), target); err != nil {
		return internalError(fmt.Sprintf("replace %s atomically", path), fmt.Sprintf("Ensure %s is replaceable, then retry", path))
	}
	removeTemp = false
	if err := unix.Fsync(int(parent.Fd())); err != nil && !errors.Is(err, unix.EINVAL) {
		return &Error{Kind: KindPartial, Message: fmt.Sprintf("Replaced %s but could not sync its directory", path), Recovery: "Retry the operation to confirm durability"}
	}
	return nil
}

func checkReplacementTarget(parentFD int, target, path string) error {
	var stat unix.Stat_t
	if err := unix.Fstatat(parentFD, target, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return nil
		}
		return internalError(fmt.Sprintf("inspect the replacement target %s", path), fmt.Sprintf("Check %s and retry", path))
	}
	kind := stat.Mode & unix.S_IFMT
	if kind == unix.S_IFLNK {
		return unsafePathError(fmt.Sprintf("Replacement target %s is a symbolic link", path), fmt.Sprintf("Remove %s and restore the record, then retry", path))
	}
	if kind != unix.S_IFREG {
		return unsafePathError(fmt.Sprintf("Replacement target %s is not a regular file", path), fmt.Sprintf("Inspect and repair %s, then retry", path))
	}
	return nil
}

func randomTempName() (string, error) {
	var bytes [12]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return tempPrefix + hex.EncodeToString(bytes[:]), nil
}

// ---- paths ----

func (s *Store) tasksDir() string        { return filepath.Join(s.root, "tasks") }
func (s *Store) repositoriesDir() string { return filepath.Join(s.root, "repositories") }
func (s *Store) locksDir() string        { return filepath.Join(s.root, "locks") }

func (s *Store) taskDir(taskID string) string   { return filepath.Join(s.tasksDir(), taskID) }
func (s *Store) eventsDir(taskID string) string { return filepath.Join(s.taskDir(taskID), "events") }
func (s *Store) manifestPath(taskID string) string {
	return filepath.Join(s.taskDir(taskID), "manifest.json")
}
func (s *Store) lockPath(taskID string) string { return filepath.Join(s.locksDir(), taskID+".lock") }
func (s *Store) repositoryPath(name string) string {
	return filepath.Join(s.repositoriesDir(), name+".json")
}

func (s *Store) eventPath(taskID string, sequence int) string {
	return filepath.Join(s.eventsDir(taskID), fmt.Sprintf("%0*d.json", eventSequenceWidth, sequence))
}
