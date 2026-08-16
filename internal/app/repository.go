package app

import (
	"io"

	"github.com/akofink/akagent-cli/internal/lifecycle"
	"github.com/akofink/akagent-cli/internal/store"
)

type repositoryListItem struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	Policy       string `json:"policy"`
	WorktreeRoot string `json:"worktree_root,omitempty"`
}

type repositoryListView struct {
	Repositories []repositoryListItem `json:"repositories"`
	Total        int                  `json:"total"`
}

type repositoryNameView struct {
	Name string `json:"name"`
}

type repositoryUnregisteredView struct {
	Repository repositoryNameView `json:"repository"`
}

func repositoryRegisterCommand(args []string, stdout io.Writer) int {
	name, path, policy, worktreeRoot, ok := parseRepositoryRegister(args)
	if !ok {
		return writeError(stdout, "usage", "Usage: akagent repository register <name> <path> [--policy <worktree|direct>] [--worktree-root <absolute-path>]", false, "Register a local Git repository")
	}
	state, err := store.Open()
	if err != nil {
		return lifecycleError(stdout, err)
	}
	repository, err := lifecycle.New(state).RegisterRepository(name, path, policy, worktreeRoot)
	if err != nil {
		return lifecycleError(stdout, err)
	}
	return write(stdout, repositoryView{Repository: repository})
}

func repositoryListCommand(args []string, stdout io.Writer) int {
	if len(args) != 0 {
		return repositoryUsage(stdout)
	}
	state, err := store.Open()
	if err != nil {
		return lifecycleError(stdout, err)
	}
	repositories, err := lifecycle.New(state).ListRepositories()
	if err != nil {
		return lifecycleError(stdout, err)
	}
	items := make([]repositoryListItem, 0, len(repositories))
	for _, repository := range repositories {
		items = append(items, repositoryListItem{Name: repository.Name, Path: repository.Path, Policy: repository.Policy, WorktreeRoot: repository.WorktreeRoot})
	}
	return write(stdout, repositoryListView{Repositories: items, Total: len(items)})
}

func repositoryInspectCommand(args []string, stdout io.Writer) int {
	if len(args) != 1 {
		return writeError(stdout, "usage", "Usage: akagent repository inspect <name>", false, "Run `akagent repository list`")
	}
	state, err := store.Open()
	if err != nil {
		return lifecycleError(stdout, err)
	}
	repository, err := lifecycle.New(state).InspectRepository(args[0])
	if err != nil {
		return lifecycleError(stdout, err)
	}
	return write(stdout, repositoryView{Repository: repository})
}

func repositoryUpdateCommand(args []string, stdout io.Writer) int {
	name, path, policy, worktreeRoot, ok := parseRepositoryUpdate(args)
	if !ok {
		return writeError(stdout, "usage", "Usage: akagent repository update <name> [--path <path>] [--policy <worktree|direct>] [--worktree-root <absolute-path>]", false, "Update a repository registration without changing its checkout")
	}
	state, err := store.Open()
	if err != nil {
		return lifecycleError(stdout, err)
	}
	repository, err := lifecycle.New(state).UpdateRepository(name, path, policy, worktreeRoot)
	if err != nil {
		return lifecycleError(stdout, err)
	}
	return write(stdout, repositoryView{Repository: repository})
}

func repositoryUnregisterCommand(args []string, stdout io.Writer) int {
	if len(args) != 1 {
		return writeError(stdout, "usage", "Usage: akagent repository unregister <name>", false, "Run `akagent repository list`")
	}
	state, err := store.Open()
	if err != nil {
		return lifecycleError(stdout, err)
	}
	if err := lifecycle.New(state).UnregisterRepository(args[0]); err != nil {
		return lifecycleError(stdout, err)
	}
	return write(stdout, repositoryUnregisteredView{Repository: repositoryNameView{Name: args[0]}})
}

func parseRepositoryRegister(args []string) (name, path, policy, worktreeRoot string, ok bool) {
	if len(args) < 2 || args[0] == "" || args[1] == "" {
		return "", "", "", "", false
	}
	name, path = args[0], args[1]
	policySet, rootSet := false, false
	for args = args[2:]; len(args) > 0; {
		if len(args) < 2 || args[1] == "" {
			return "", "", "", "", false
		}
		switch args[0] {
		case "--policy":
			if policySet || !validRepositoryPolicy(args[1]) {
				return "", "", "", "", false
			}
			policy, policySet = args[1], true
		case "--worktree-root":
			if rootSet {
				return "", "", "", "", false
			}
			worktreeRoot, rootSet = args[1], true
		default:
			return "", "", "", "", false
		}
		args = args[2:]
	}
	return name, path, policy, worktreeRoot, true
}

func parseRepositoryUpdate(args []string) (name, path, policy, worktreeRoot string, ok bool) {
	if len(args) < 3 || args[0] == "" {
		return "", "", "", "", false
	}
	name = args[0]
	pathSet, policySet, rootSet := false, false, false
	for args = args[1:]; len(args) > 0; {
		if len(args) < 2 || args[1] == "" {
			return "", "", "", "", false
		}
		switch args[0] {
		case "--path":
			if pathSet {
				return "", "", "", "", false
			}
			path, pathSet = args[1], true
		case "--policy":
			if policySet || !validRepositoryPolicy(args[1]) {
				return "", "", "", "", false
			}
			policy, policySet = args[1], true
		case "--worktree-root":
			if rootSet {
				return "", "", "", "", false
			}
			worktreeRoot, rootSet = args[1], true
		default:
			return "", "", "", "", false
		}
		args = args[2:]
	}
	return name, path, policy, worktreeRoot, pathSet || policySet || rootSet
}

func validRepositoryPolicy(policy string) bool {
	return policy == "worktree" || policy == "direct"
}

func repositoryUsage(stdout io.Writer) int {
	return writeError(stdout, "usage", "Usage: akagent repository <register|list|inspect|update|unregister>", false, "Run `akagent repository list`")
}
