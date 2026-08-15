package store

import (
	"encoding/json"
	"fmt"
	"sort"
)

// Repository is a local clone registration. Policy is deliberately small:
// worktree keeps task work isolated, while direct allows an existing clone.
type Repository struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Policy string `json:"policy"`
}

// WriteRepository atomically replaces a repository registration.
func (s *Store) WriteRepository(repository Repository) error {
	if err := validateRepository(repository); err != nil {
		return err
	}
	return s.WithLock("repo-"+repository.Name, func() error {
		encoded, err := json.MarshalIndent(repository, "", "  ")
		if err != nil {
			return err
		}
		return s.atomicallyWrite(s.repositoryPath(repository.Name), append(encoded, '\n'))
	})
}

// ReadRepository returns a registered local repository.
func (s *Store) ReadRepository(name string) (Repository, error) {
	if err := validateRepositoryName(name); err != nil {
		return Repository{}, err
	}
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
	if repository.Path == "" {
		return newError(KindUsage, "Repository path is required", "Provide an absolute repository path")
	}
	if repository.Policy != "worktree" && repository.Policy != "direct" {
		return newError(KindUsage, "Repository policy must be worktree or direct", "Register the repository with a supported policy")
	}
	return nil
}

func validateRepositoryName(name string) error {
	if len(name) > 48 {
		return newError(KindUsage, "Repository name is too long", "Use a repository name of 48 characters or fewer")
	}
	return validateTaskID(name)
}
