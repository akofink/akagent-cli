package store

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
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
	if repository.WorktreeRoot != "" && !filepath.IsAbs(repository.WorktreeRoot) {
		return newError(KindUsage, "Repository worktree root must be absolute", "Provide an absolute worktree root")
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
