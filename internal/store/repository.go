package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
)

// Repository is a local clone registration. Policy is deliberately small:
// worktree keeps task work isolated, while direct allows an existing clone.
type Repository struct {
	Name         string   `json:"name"`
	Path         string   `json:"path"`
	Policy       string   `json:"policy"`
	WorktreeRoot string   `json:"worktree_root,omitempty"`
	Instructions []string `json:"instructions,omitempty"`
}

// WriteRepository atomically replaces a repository registration.
func (s *Store) WriteRepository(repository Repository) error {
	if err := validateRepository(repository); err != nil {
		return err
	}
	return s.WithRepositoryLock(repository.Name, func() error {
		return s.writeRepositoryLocked(repository)
	})
}

// RegisterRepository creates a repository registration while holding its
// repository-scoped lock. An equivalent existing registration is a no-op.
func (s *Store) RegisterRepository(repository Repository) (bool, error) {
	if err := validateRepository(repository); err != nil {
		return false, err
	}
	created := false
	err := s.WithRepositoryLock(repository.Name, func() error {
		existing, err := s.readRepository(repository.Name)
		if err == nil {
			if sameRepository(existing, repository) {
				return nil
			}
			return fmt.Errorf("repository registration conflicts with existing %s", repository.Name)
		}
		if !IsKind(err, KindNotFound) {
			return err
		}
		if err := s.writeRepositoryLocked(repository); err != nil {
			return err
		}
		created = true
		return nil
	})
	return created, err
}

func (s *Store) writeRepositoryLocked(repository Repository) error {
	encoded, err := json.MarshalIndent(repository, "", "  ")
	if err != nil {
		return err
	}
	return s.atomicallyWrite(s.repositoryPath(repository.Name), append(encoded, '\n'))
}

// ReadRepository returns a registered local repository.
func (s *Store) ReadRepository(name string) (Repository, error) {
	if err := validateRepositoryName(name); err != nil {
		return Repository{}, err
	}
	return s.readRepository(name)
}

func (s *Store) readRepository(name string) (Repository, error) {
	data, err := s.readOwnedFile(s.repositoryPath(name))
	if err != nil {
		if IsKind(err, KindNotFound) {
			return Repository{}, newError(KindNotFound, fmt.Sprintf("No repository registered as %s", name), "Register it with `akagent repository register`")
		}
		return Repository{}, err
	}
	var repository Repository
	if err := json.Unmarshal(data, &repository); err != nil {
		return Repository{}, malformedError(fmt.Sprintf("Malformed repository record for %s", name), "Register the repository again")
	}
	if err := validateRepository(repository); err != nil || repository.Name != name {
		return Repository{}, malformedError(fmt.Sprintf("Malformed repository record for %s", name), "Register the repository again")
	}
	return repository, nil
}

// UpdateRepository applies an atomic read-modify-write to a registration.
// Returning the existing value for an equivalent update makes retries safe.
func (s *Store) UpdateRepository(name string, update func(*Repository) error) (Repository, error) {
	if err := validateRepositoryName(name); err != nil {
		return Repository{}, err
	}
	var result Repository
	err := s.WithRepositoryLock(name, func() error {
		current, err := s.readRepository(name)
		if err != nil {
			return err
		}
		updated := current
		if err := update(&updated); err != nil {
			return err
		}
		if updated.Name != name {
			return newError(KindUsage, "Repository name cannot be changed by update", "Unregister and register under the new name")
		}
		if err := validateRepository(updated); err != nil {
			return err
		}
		if sameRepository(current, updated) {
			result = current
			return nil
		}
		if updated.Path != current.Path {
			references, err := s.RepositoryReferences(name)
			if err != nil {
				return err
			}
			if len(references) > 0 {
				return &Error{
					Kind:     KindConflict,
					Message:  fmt.Sprintf("repository %s path update conflicts with referenced tasks: %s", name, strings.Join(references, ", ")),
					Recovery: "Stop or archive the referencing tasks, then retry the repository update",
				}
			}
		}
		if err := s.writeRepositoryLocked(updated); err != nil {
			return err
		}
		result = updated
		return nil
	})
	return result, err
}

// UnregisterRepository removes only the registration record. It preserves the
// repository on disk and refuses removal while task manifests reference it.
func (s *Store) UnregisterRepository(name string) error {
	if err := validateRepositoryName(name); err != nil {
		return err
	}
	return s.WithRepositoryLock(name, func() error {
		if _, err := s.readRepository(name); err != nil {
			return err
		}
		references, err := s.RepositoryReferences(name)
		if err != nil {
			return err
		}
		if len(references) > 0 {
			return &Error{
				Kind:     KindConflict,
				Message:  fmt.Sprintf("repository %s is referenced by tasks: %s", name, strings.Join(references, ", ")),
				Recovery: "Stop or archive the referencing tasks, then retry unregister; the registration record remains intact",
			}
		}
		return s.removeRepositoryLocked(name)
	})
}

// RepositoryReferences returns task IDs whose durable manifests reference name.
func (s *Store) RepositoryReferences(name string) ([]string, error) {
	if err := validateRepositoryName(name); err != nil {
		return nil, err
	}
	ids, err := s.TaskIDs()
	if err != nil {
		return nil, err
	}
	references := make([]string, 0)
	for _, id := range ids {
		envelope, err := s.ReadManifest(id)
		if err != nil {
			return nil, err
		}
		manifest, err := envelope.DecodeManifest()
		if err != nil {
			return nil, err
		}
		if manifest.Repository == name {
			references = append(references, id)
		}
	}
	return references, nil
}

func (s *Store) removeRepositoryLocked(name string) error {
	directory, err := s.openOwned(s.repositoriesDir(), true)
	if err != nil {
		return err
	}
	defer directory.Close()
	if err := unix.Unlinkat(int(directory.Fd()), name+".json", 0); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return newError(KindNotFound, fmt.Sprintf("No repository registered as %s", name), "Register it with `akagent repository register`")
		}
		return internalError(fmt.Sprintf("remove repository registration %s", name), "Check the repository state and retry")
	}
	if err := unix.Fsync(int(directory.Fd())); err != nil && !errors.Is(err, unix.EINVAL) {
		return &Error{Kind: KindPartial, Message: fmt.Sprintf("Unregistered repository %s but could not sync its directory", name), Retryable: true, Recovery: "Retry unregister to confirm the registration is absent"}
	}
	return nil
}

// RepositoryNames returns registered repository names in deterministic order.
func (s *Store) RepositoryNames() ([]string, error) {
	directory, err := s.openOwned(s.repositoriesDir(), true)
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	entries, err := directory.ReadDir(-1)
	if err != nil {
		return nil, internalError("list repository registrations", "Check the repository state and retry")
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if len(name) > len(".json") && name[len(name)-len(".json"):] == ".json" {
			candidate := name[:len(name)-len(".json")]
			if validateRepositoryName(candidate) == nil {
				names = append(names, candidate)
			}
		}
	}
	sort.Strings(names)
	return names, nil
}

func validateRepository(repository Repository) error {
	if err := validateRepositoryName(repository.Name); err != nil {
		return err
	}
	if repository.Path == "" || !filepath.IsAbs(repository.Path) {
		return newError(KindUsage, "Repository path must be absolute", "Provide an absolute repository path")
	}
	if repository.Policy != "worktree" && repository.Policy != "direct" {
		return newError(KindUsage, "Repository policy must be worktree or direct", "Register the repository with a supported policy")
	}
	if repository.WorktreeRoot != "" {
		if !filepath.IsAbs(repository.WorktreeRoot) {
			return newError(KindUsage, "Repository worktree root must be absolute", "Provide an absolute worktree root")
		}
		if repository.Policy != "worktree" {
			return newError(KindUsage, "Repository worktree root requires the worktree policy", "Register the repository with `--policy worktree`")
		}
	}
	for _, instruction := range repository.Instructions {
		if instruction == "" || !filepath.IsAbs(instruction) {
			return newError(KindUsage, "Repository instruction paths must be absolute", "Register the repository again with valid instructions")
		}
	}
	return nil
}

func sameRepository(a, b Repository) bool {
	if a.Name != b.Name || a.Path != b.Path || a.Policy != b.Policy || a.WorktreeRoot != b.WorktreeRoot {
		return false
	}
	if len(a.Instructions) != len(b.Instructions) {
		return false
	}
	for i := range a.Instructions {
		if a.Instructions[i] != b.Instructions[i] {
			return false
		}
	}
	return true
}

func validateRepositoryName(name string) error {
	if len(name) > 48 {
		return newError(KindUsage, "Repository name is too long", "Use a repository name of 48 characters or fewer")
	}
	return validateTaskID(name)
}
