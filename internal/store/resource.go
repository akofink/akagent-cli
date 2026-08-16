package store

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	KindResourceManifest = "resource_manifest"
	KindResourceEvent    = "resource_event"
	KindResourceArchive  = "resource_archive"
)

// Resource is one immutable repository, branch, and worktree association owned
// by a task. Its Git, archive, cleanup, and recovery observations are separate
// from both the task and every other resource.
type Resource struct {
	ID                     string   `json:"id"`
	TaskID                 string   `json:"task_id"`
	Repository             string   `json:"repository"`
	Branch                 string   `json:"branch,omitempty"`
	BaseRevision           string   `json:"base_revision,omitempty"`
	WorktreeBaseRevision   string   `json:"worktree_base_revision,omitempty"`
	WorktreePath           string   `json:"worktree_path,omitempty"`
	Git                    GitFacts `json:"git,omitempty"`
	RecoveryDebt           string   `json:"recovery_debt,omitempty"`
	ArchiveState           string   `json:"archive_state,omitempty"`
	CleanupState           string   `json:"cleanup_state,omitempty"`
	WorktreeCleanupState   string   `json:"worktree_cleanup_state,omitempty"`
	CredentialCleanupState string   `json:"credential_cleanup_state,omitempty"`
	CleanupDebt            bool     `json:"cleanup_debt,omitempty"`
}

// ResourceArchive is an independently recoverable snapshot of one resource.
type ResourceArchive struct {
	TaskID     string        `json:"task_id"`
	ResourceID string        `json:"resource_id"`
	CapturedAt time.Time     `json:"captured_at"`
	Resource   Resource      `json:"resource"`
	Events     []EventRecord `json:"events"`
	Git        GitFacts      `json:"git,omitempty"`
	Warnings   []string      `json:"warnings,omitempty"`
}

func validateResourceID(id string) error {
	if id == "" || !taskIDPattern.MatchString(id) {
		return newError(KindUsage, fmt.Sprintf("Invalid resource ID %q", id), "Use a short stable resource ID or `akagent id generate`")
	}
	return nil
}

func (s *Store) WriteResource(taskID string, resource Resource) error {
	if err := validateTaskID(taskID); err != nil {
		return err
	}
	if err := validateResourceID(resource.ID); err != nil {
		return err
	}
	if resource.TaskID != "" && resource.TaskID != taskID {
		return newError(KindUsage, "Resource task ID does not match its path", "Retry with the requested task ID")
	}
	resource.TaskID = taskID
	return s.WithLock(taskID, func() error { return s.writeResourceLocked(taskID, resource) })
}

func (s *Store) CreateResource(taskID string, resource Resource) (bool, Resource, error) {
	if err := validateTaskID(taskID); err != nil {
		return false, Resource{}, err
	}
	if err := validateResourceID(resource.ID); err != nil {
		return false, Resource{}, err
	}
	resource.TaskID = taskID
	var created bool
	var existing Resource
	err := s.WithLock(taskID, func() error {
		if _, err := s.ReadManifest(taskID); err != nil {
			return err
		}
		current, err := s.ReadResource(taskID, resource.ID)
		if err == nil {
			existing = current
			if !sameResource(current, resource) {
				return &Error{Kind: KindConflict, Message: fmt.Sprintf("resource %s inputs conflict with the existing resource", resource.ID), Recovery: fmt.Sprintf("Inspect resource %s for task %s", resource.ID, taskID)}
			}
			return nil
		}
		if !IsKind(err, KindNotFound) {
			return err
		}
		if err := s.writeResourceLocked(taskID, resource); err != nil {
			return err
		}
		created, existing = true, resource
		return nil
	})
	return created, existing, err
}

func (s *Store) ReadResource(taskID, resourceID string) (Resource, error) {
	if err := validateTaskID(taskID); err != nil {
		return Resource{}, err
	}
	if err := validateResourceID(resourceID); err != nil {
		return Resource{}, err
	}
	path := s.resourceManifestPath(taskID, resourceID)
	if err := s.checkTaskDir(taskID); err != nil {
		return Resource{}, err
	}
	data, err := s.readOwnedFile(path)
	if err != nil {
		if IsKind(err, KindNotFound) {
			return Resource{}, newError(KindNotFound, fmt.Sprintf("No resource %s found for task %s", resourceID, taskID), fmt.Sprintf("List resources for task %s", taskID))
		}
		return Resource{}, err
	}
	envelope, err := decodeResourceEnvelope(path, data, KindResourceManifest, taskID, resourceID)
	if err != nil {
		return Resource{}, err
	}
	return envelope.DecodeResource()
}

func (s *Store) UpdateResource(taskID, resourceID string, update func(*Resource) error) (Resource, error) {
	if err := validateTaskID(taskID); err != nil {
		return Resource{}, err
	}
	if err := validateResourceID(resourceID); err != nil {
		return Resource{}, err
	}
	var resource Resource
	err := s.WithLock(taskID, func() error {
		current, err := s.ReadResource(taskID, resourceID)
		if err != nil {
			return err
		}
		resource = current
		if err := update(&resource); err != nil {
			return err
		}
		if resource.ID != resourceID || resource.TaskID != taskID {
			return newError(KindUsage, "Resource identity cannot be changed", "Retry without changing the resource ID")
		}
		return s.writeResourceLocked(taskID, resource)
	})
	return resource, err
}

func (s *Store) ResourceIDs(taskID string) ([]string, error) {
	if err := validateTaskID(taskID); err != nil {
		return nil, err
	}
	if err := s.checkTaskDir(taskID); err != nil {
		return nil, err
	}
	dir := s.resourcesDir(taskID)
	file, err := s.openOwned(dir, true)
	if err != nil {
		if IsKind(err, KindNotFound) {
			return []string{}, nil
		}
		return nil, err
	}
	defer file.Close()
	entries, err := file.ReadDir(-1)
	if err != nil {
		return nil, internalError(fmt.Sprintf("list resources for task %s", taskID), "Check the task state and retry")
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || validateResourceID(entry.Name()) != nil {
			continue
		}
		ids = append(ids, entry.Name())
	}
	sort.Strings(ids)
	return ids, nil
}

func (s *Store) AppendResourceEvent(taskID, resourceID string, event Event) (int, error) {
	if err := validateTaskID(taskID); err != nil {
		return 0, err
	}
	if err := validateResourceID(resourceID); err != nil {
		return 0, err
	}
	var sequence int
	err := s.WithLock(taskID, func() error {
		if _, err := s.ReadResource(taskID, resourceID); err != nil {
			return err
		}
		if err := s.ensureResourceDir(taskID, resourceID); err != nil {
			return err
		}
		envelope, err := resourceEventEnvelope(taskID, resourceID, event)
		if err != nil {
			return internalError("encode a resource event", "Retry the operation")
		}
		encoded, err := encodeRecord(envelope)
		if err != nil {
			return err
		}
		next, err := s.nextResourceSequence(taskID, resourceID)
		if err != nil {
			return err
		}
		if err := s.atomicallyWrite(s.resourceEventPath(taskID, resourceID, next), encoded); err != nil {
			return err
		}
		sequence = next
		return nil
	})
	return sequence, err
}

func (s *Store) ReadResourceEvents(taskID, resourceID string) ([]EventRecord, error) {
	if err := validateTaskID(taskID); err != nil {
		return nil, err
	}
	if err := validateResourceID(resourceID); err != nil {
		return nil, err
	}
	if _, err := s.ReadResource(taskID, resourceID); err != nil {
		return nil, err
	}
	dir := s.resourceEventsDir(taskID, resourceID)
	file, err := s.openOwned(dir, true)
	if err != nil {
		if IsKind(err, KindNotFound) {
			return []EventRecord{}, nil
		}
		return nil, err
	}
	defer file.Close()
	entries, err := file.ReadDir(-1)
	if err != nil {
		return nil, internalError("list resource events", "Check the resource state and retry")
	}
	sequences := make([]int, 0, len(entries))
	files := map[int]string{}
	for _, entry := range entries {
		sequence, ok := parseEventSequence(entry.Name())
		if entry.IsDir() || !ok || strings.HasPrefix(entry.Name(), ".") || files[sequence] != "" {
			return nil, malformedError(fmt.Sprintf("Malformed resource event history for %s/%s", taskID, resourceID), fmt.Sprintf("Inspect %s", dir))
		}
		sequences = append(sequences, sequence)
		files[sequence] = entry.Name()
	}
	sort.Ints(sequences)
	if err := checkSequenceContiguity(sequences, taskID+"/"+resourceID); err != nil {
		return nil, err
	}
	result := make([]EventRecord, 0, len(sequences))
	for _, sequence := range sequences {
		path := filepath.Join(dir, files[sequence])
		data, err := s.readOwnedFile(path)
		if err != nil {
			return nil, err
		}
		envelope, err := decodeResourceEnvelope(path, data, KindResourceEvent, taskID, resourceID)
		if err != nil {
			return nil, err
		}
		event, err := envelope.DecodeEvent()
		if err != nil {
			return nil, err
		}
		result = append(result, EventRecord{Sequence: sequence, ObservedAt: envelope.ObservedAt, Event: event})
	}
	return result, nil
}

func (s *Store) WriteResourceArchive(taskID, resourceID string, archive ResourceArchive) error {
	if err := validateTaskID(taskID); err != nil {
		return err
	}
	if err := validateResourceID(resourceID); err != nil {
		return err
	}
	if archive.TaskID != taskID || archive.ResourceID != resourceID {
		return newError(KindUsage, "Resource archive identity does not match its path", "Retry archive with the requested resource")
	}
	if archive.CapturedAt.IsZero() {
		return newError(KindUsage, "Resource archive capture time is required", "Retry archive")
	}
	return s.WithLock(taskID, func() error {
		if err := s.ensureResourceDir(taskID, resourceID); err != nil {
			return err
		}
		envelope, err := resourceArchiveEnvelope(taskID, resourceID, archive)
		if err != nil {
			return internalError("encode a resource archive", "Retry the operation")
		}
		encoded, err := encodeRecord(envelope)
		if err != nil {
			return err
		}
		return s.atomicallyWrite(s.resourceArchivePath(taskID, resourceID), encoded)
	})
}

func (s *Store) ReadResourceArchive(taskID, resourceID string) (ResourceArchive, error) {
	if err := validateTaskID(taskID); err != nil {
		return ResourceArchive{}, err
	}
	if err := validateResourceID(resourceID); err != nil {
		return ResourceArchive{}, err
	}
	path := s.resourceArchivePath(taskID, resourceID)
	data, err := s.readOwnedFile(path)
	if err != nil {
		if IsKind(err, KindNotFound) {
			return ResourceArchive{}, newError(KindNotFound, fmt.Sprintf("No archive found for resource %s", resourceID), "Archive the resource before cleaning it")
		}
		return ResourceArchive{}, err
	}
	envelope, err := decodeResourceEnvelope(path, data, KindResourceArchive, taskID, resourceID)
	if err != nil {
		return ResourceArchive{}, err
	}
	archive, err := envelope.DecodeResourceArchive()
	if err != nil || archive.TaskID != taskID || archive.ResourceID != resourceID || archive.CapturedAt.IsZero() {
		return ResourceArchive{}, malformedError(fmt.Sprintf("Malformed archive for resource %s", resourceID), fmt.Sprintf("Inspect and repair %s", path))
	}
	return archive, nil
}

func (s *Store) writeResourceLocked(taskID string, resource Resource) error {
	if err := s.ensureResourceDir(taskID, resource.ID); err != nil {
		return err
	}
	envelope, err := resourceManifestEnvelope(taskID, resource.ID, resource)
	if err != nil {
		return internalError("encode a resource manifest", "Retry the operation")
	}
	encoded, err := encodeRecord(envelope)
	if err != nil {
		return err
	}
	return s.atomicallyWrite(s.resourceManifestPath(taskID, resource.ID), encoded)
}

func (s *Store) ensureResourceDir(taskID, resourceID string) error {
	if err := s.ensureTaskDir(taskID); err != nil {
		return err
	}
	for _, entry := range []struct{ dir, label string }{{s.resourcesDir(taskID), "resources directory"}, {s.resourceDir(taskID, resourceID), "resource directory"}, {s.resourceEventsDir(taskID, resourceID), "resource events directory"}} {
		if err := s.ensureDir(entry.dir, entry.label); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) nextResourceSequence(taskID, resourceID string) (int, error) {
	events, err := s.ReadResourceEvents(taskID, resourceID)
	if err != nil {
		return 0, err
	}
	return len(events) + 1, nil
}

func sameResource(a, b Resource) bool {
	return a.ID == b.ID && a.TaskID == b.TaskID && a.Repository == b.Repository && a.Branch == b.Branch && a.BaseRevision == b.BaseRevision && a.WorktreePath == b.WorktreePath
}

func decodeResourceEnvelope(path string, data []byte, kind, taskID, resourceID string) (Envelope, error) {
	envelope, err := decodeEnvelope(path, data, kind, taskID)
	if err != nil {
		return Envelope{}, err
	}
	if envelope.ResourceID != resourceID {
		return Envelope{}, malformedError(fmt.Sprintf("Resource record at %s has the wrong resource ID", path), fmt.Sprintf("Inspect and repair %s", path))
	}
	return envelope, nil
}

func resourceManifestEnvelope(taskID, resourceID string, resource Resource) (Envelope, error) {
	envelope, err := newEnvelope(KindResourceManifest, taskID, resource)
	envelope.ResourceID = resourceID
	return envelope, err
}
func resourceEventEnvelope(taskID, resourceID string, event Event) (Envelope, error) {
	envelope, err := newEnvelope(KindResourceEvent, taskID, event)
	envelope.ResourceID = resourceID
	return envelope, err
}
func resourceArchiveEnvelope(taskID, resourceID string, archive ResourceArchive) (Envelope, error) {
	envelope, err := newEnvelope(KindResourceArchive, taskID, archive)
	envelope.ResourceID = resourceID
	return envelope, err
}

func (e Envelope) DecodeResource() (Resource, error) {
	var resource Resource
	if !isObjectPayload(e.Data) || json.Unmarshal(e.Data, &resource) != nil {
		return Resource{}, malformedError("Malformed resource manifest payload", "Inspect and repair the resource record")
	}
	if resource.ID == "" || resource.TaskID == "" || resource.TaskID != e.TaskID {
		return Resource{}, malformedError("Resource manifest is missing or has mismatched identity", "Inspect and repair the resource record")
	}
	return resource, nil
}
func (e Envelope) DecodeResourceArchive() (ResourceArchive, error) {
	var archive ResourceArchive
	if !isObjectPayload(e.Data) || json.Unmarshal(e.Data, &archive) != nil {
		return ResourceArchive{}, malformedError("Malformed resource archive payload", "Inspect and repair the resource archive")
	}
	return archive, nil
}

func (s *Store) resourcesDir(taskID string) string {
	return filepath.Join(s.taskDir(taskID), "resources")
}
func (s *Store) resourceDir(taskID, resourceID string) string {
	return filepath.Join(s.resourcesDir(taskID), resourceID)
}
func (s *Store) resourceManifestPath(taskID, resourceID string) string {
	return filepath.Join(s.resourceDir(taskID, resourceID), "manifest.json")
}
func (s *Store) resourceEventsDir(taskID, resourceID string) string {
	return filepath.Join(s.resourceDir(taskID, resourceID), "events")
}
func (s *Store) resourceEventPath(taskID, resourceID string, sequence int) string {
	return filepath.Join(s.resourceEventsDir(taskID, resourceID), fmt.Sprintf("%0*d.json", eventSequenceWidth, sequence))
}
func (s *Store) resourceArchivePath(taskID, resourceID string) string {
	return filepath.Join(s.resourceDir(taskID, resourceID), "archive.json")
}
