package derived

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/akofink/akagent-cli/internal/store"
)

type gitResult struct {
	Worktree Finding
	Branch   Finding
	Head     string
	Remote   string
	Path     string
}

func checkGit(ctx context.Context, runner Runner, resource store.Resource, registeredPath string, taskTerminal bool) gitResult {
	result := gitResult{
		Worktree: finding("worktree", StateUnknown, "repository_unavailable", "Verify the repository path on the owning host"),
		Branch:   finding("branch", StateUnknown, "repository_unavailable", "Verify the branch on the owning host"),
	}
	path := registeredPath
	if path == "" {
		path = resource.WorktreePath
	}
	if path == "" {
		return result
	}
	if _, err := runner.Run(ctx, "git", "-C", path, "rev-parse", "--show-toplevel"); err != nil {
		return result
	}
	if remote, err := runner.Run(ctx, "git", "-C", path, "config", "--get", "remote.origin.url"); err == nil {
		result.Remote = trimmed(remote)
	}
	// A resource may belong to another clone of the registered repository.
	// Accept only its exact top level, matching origin identity and direct Git
	// registration evidence; never substitute an arbitrary nearby checkout.
	owner := resource.WorktreePath
	if owner != "" && !sameGitPath(owner, path) && result.Remote != "" {
		top, topErr := runner.Run(ctx, "git", "-C", owner, "rev-parse", "--show-toplevel")
		remote, remoteErr := runner.Run(ctx, "git", "-C", owner, "config", "--get", "remote.origin.url")
		if topErr == nil && remoteErr == nil && sameGitPath(trimmed(top), owner) && sameOrigin(result.Remote, trimmed(remote)) {
			common, commonErr := runner.Run(ctx, "git", "-C", owner, "rev-parse", "--path-format=absolute", "--git-common-dir")
			listed, listErr := runner.Run(ctx, "git", "-C", owner, "worktree", "list", "--porcelain")
			if commonErr == nil && filepath.IsAbs(trimmed(common)) && listErr == nil && listedWorktree(string(listed), owner) {
				path = owner
			}
		}
	}
	result.Path = path
	result.Worktree = checkWorktree(ctx, runner, resource, path, taskTerminal)
	if resource.Branch == "" {
		result.Branch = finding("branch", StateUnknown, "binding_missing", "Record the intended branch")
		return result
	}
	// refs/heads is an exact local ref; a branch name cannot be interpreted as an option.
	head, err := runner.Run(ctx, "git", "-C", path, "rev-parse", "--verify", "--quiet", "refs/heads/"+resource.Branch+"^{commit}")
	if err != nil {
		if taskTerminal {
			result.Branch = finding("branch", StateCurrent, "removed_after_finish", "No branch action needed")
		} else {
			result.Branch = finding("branch", StateMissing, "local_ref_missing", "Inspect local and remote branches before recreating one")
		}
		return result
	}
	result.Head = trimmed(head)
	result.Branch = finding("branch", StateCurrent, "local_ref", "No branch action needed")
	result.Branch.Detail = result.Head
	return result
}

func sameGitPath(a, b string) bool {
	if resolved, err := filepath.EvalSymlinks(a); err == nil {
		a = resolved
	}
	if resolved, err := filepath.EvalSymlinks(b); err == nil {
		b = resolved
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func sameOrigin(a, b string) bool {
	ownerA, repoA, okA := githubRemote(a)
	ownerB, repoB, okB := githubRemote(b)
	if okA && okB {
		return ownerA == ownerB && repoA == repoB
	}
	return a != "" && a == b
}

func checkWorktree(ctx context.Context, runner Runner, resource store.Resource, path string, taskTerminal bool) Finding {
	if resource.WorktreePath == "" {
		return finding("worktree", StateUnknown, "binding_missing", "Record the intended worktree path")
	}
	listed, err := runner.Run(ctx, "git", "-C", path, "worktree", "list", "--porcelain")
	if err != nil {
		return finding("worktree", StateUnknown, "git_unavailable", "Retry the worktree check")
	}
	if !listedWorktree(string(listed), resource.WorktreePath) {
		if taskTerminal {
			return finding("worktree", StateCurrent, "removed_after_finish", "No worktree action needed")
		}
		return finding("worktree", StateMissing, "not_registered", "Inspect Git worktrees before creating or removing one")
	}
	if _, err := os.Stat(resource.WorktreePath); err != nil {
		return finding("worktree", StateMissing, "path_missing", "Inspect the worktree on the owning host")
	}
	branch, err := runner.Run(ctx, "git", "-C", resource.WorktreePath, "branch", "--show-current")
	if err != nil {
		return finding("worktree", StateUnknown, "git_unavailable", "Retry the worktree check")
	}
	if resource.Branch != "" && trimmed(branch) != resource.Branch {
		return finding("worktree", StateStale, "branch_mismatch", "Inspect the worktree branch before proceeding")
	}
	status, err := runner.Run(ctx, "git", "-C", resource.WorktreePath, "status", "--porcelain=v1", "-z", "--untracked-files=normal")
	if err != nil {
		return finding("worktree", StateUnknown, "git_unavailable", "Retry the worktree check")
	}
	if changed := statusEntries(status); changed > 0 {
		result := finding("worktree", StateStale, "dirty", "Inspect and preserve tracked and untracked changes")
		result.Detail = strconv.Itoa(changed) + " changed paths"
		return result
	}
	if taskTerminal {
		return finding("worktree", StateStale, "retained_after_finish", "Clean up after verifying delivery")
	}
	return finding("worktree", StateCurrent, "registered", "No worktree action needed")
}

// statusEntries counts porcelain v1 -z entries; a rename or copy carries its source as an extra field.
func statusEntries(status []byte) int {
	count := 0
	fields := bytes.Split(bytes.TrimRight(status, "\x00"), []byte{0})
	for i := 0; i < len(fields); i++ {
		if len(fields[i]) == 0 {
			continue
		}
		count++
		if fields[i][0] == 'R' || fields[i][0] == 'C' {
			i++
		}
	}
	return count
}

func listedWorktree(porcelain, target string) bool {
	targets := []string{filepath.Clean(target)}
	if resolved, err := filepath.EvalSymlinks(target); err == nil {
		targets = append(targets, resolved)
	}
	for _, line := range strings.Split(porcelain, "\n") {
		listed, ok := strings.CutPrefix(line, "worktree ")
		if !ok {
			continue
		}
		for _, candidate := range targets {
			if filepath.Clean(listed) == candidate {
				return true
			}
		}
	}
	return false
}
