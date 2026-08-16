package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoryManagementCommandContract(t *testing.T) {
	setupTaskCommandTest(t)
	first := filepath.Join(t.TempDir(), "first")
	second := filepath.Join(t.TempDir(), "second")
	for _, path := range []string{first, second} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	marker := filepath.Join(first, "preserve.txt")
	if err := os.WriteFile(marker, []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if result := runCommand(t, []string{"repository", "register", "demo", first, "--policy", "direct"}); result.code != 0 {
		t.Fatalf("register = (%d, %q)", result.code, result.stdout)
	}

	list := runCommand(t, []string{"repository", "list"})
	wantList := fmt.Sprintf("repositories[1]{name,path,policy}:\n  demo,%s,direct\ntotal: 1\n", first)
	if list.code != 0 || list.stdout != wantList {
		t.Fatalf("list = (%d, %q), want (0, %q)", list.code, list.stdout, wantList)
	}

	inspect := runCommand(t, []string{"repository", "inspect", "demo"})
	wantInspect := fmt.Sprintf("repository:\n  name: demo\n  path: %s\n  policy: direct\n", first)
	if inspect.code != 0 || inspect.stdout != wantInspect {
		t.Fatalf("inspect = (%d, %q), want (0, %q)", inspect.code, inspect.stdout, wantInspect)
	}

	if result := runCommand(t, []string{"repository", "update", "demo", "--policy", "direct"}); result.code != 0 || result.stdout != wantInspect {
		t.Fatalf("equivalent update = (%d, %q), want (0, %q)", result.code, result.stdout, wantInspect)
	}

	updated := runCommand(t, []string{"repository", "update", "demo", "--path", second})
	wantUpdated := fmt.Sprintf("repository:\n  name: demo\n  path: %s\n  policy: direct\n", second)
	if updated.code != 0 || updated.stdout != wantUpdated {
		t.Fatalf("path update = (%d, %q), want (0, %q)", updated.code, updated.stdout, wantUpdated)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "keep\n" {
		t.Fatalf("registration update changed checkout marker: data=%q err=%v", data, err)
	}

	if result := runCommand(t, []string{"repository", "unregister", "demo"}); result.code != 0 || result.stdout != "repository:\n  name: demo\n" {
		t.Fatalf("unregister = (%d, %q)", result.code, result.stdout)
	}
	if _, err := os.Stat(second); err != nil {
		t.Fatalf("unregister removed repository path: %v", err)
	}
	if result := runCommand(t, []string{"repository", "inspect", "demo"}); result.code != 1 || result.stdout == "" {
		t.Fatalf("inspect after unregister = (%d, %q), want not_found", result.code, result.stdout)
	}
}

func TestRepositoryWorktreeRootConfiguration(t *testing.T) {
	setupTaskCommandTest(t)
	repositoryPath := filepath.Join(t.TempDir(), "repository")
	root := filepath.Join(t.TempDir(), "worktrees")
	if err := os.Mkdir(repositoryPath, 0o700); err != nil {
		t.Fatal(err)
	}
	registered := runCommand(t, []string{"repository", "register", "demo", repositoryPath, "--policy", "worktree", "--worktree-root", root})
	want := fmt.Sprintf("repository:\n  name: demo\n  path: %s\n  policy: worktree\n  worktree_root: %s\n", repositoryPath, root)
	if registered.code != 0 || registered.stdout != want {
		t.Fatalf("register = (%d, %q), want (0, %q)", registered.code, registered.stdout, want)
	}
	listed := runCommand(t, []string{"repository", "list"})
	wantList := fmt.Sprintf("repositories[1]{name,path,policy,worktree_root}:\n  demo,%s,worktree,%s\ntotal: 1\n", repositoryPath, root)
	if listed.code != 0 || listed.stdout != wantList {
		t.Fatalf("list = (%d, %q), want (0, %q)", listed.code, listed.stdout, wantList)
	}
	updatedRoot := filepath.Join(t.TempDir(), "updated-worktrees")
	updated := runCommand(t, []string{"repository", "update", "demo", "--worktree-root", updatedRoot})
	wantUpdated := fmt.Sprintf("repository:\n  name: demo\n  path: %s\n  policy: worktree\n  worktree_root: %s\n", repositoryPath, updatedRoot)
	if updated.code != 0 || updated.stdout != wantUpdated {
		t.Fatalf("update = (%d, %q), want (0, %q)", updated.code, updated.stdout, wantUpdated)
	}
	if direct := runCommand(t, []string{"repository", "update", "demo", "--policy", "direct", "--worktree-root", root}); direct.code != 2 || !strings.Contains(direct.stdout, "worktree root requires") {
		t.Fatalf("direct root update = (%d, %q), want policy error", direct.code, direct.stdout)
	}
}

func TestRepositoryUnregisterReferencedTaskIsSafe(t *testing.T) {
	setupTaskCommandTest(t)
	repositoryPath := filepath.Join(t.TempDir(), "repository")
	if err := os.Mkdir(repositoryPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if result := runCommand(t, []string{"repository", "register", "demo", repositoryPath, "--policy", "direct"}); result.code != 0 {
		t.Fatalf("register = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "create", "--task-id", "ref-18", "--title", "Referenced", "--repository", "demo"}); result.code != 0 {
		t.Fatalf("task create = (%d, %q)", result.code, result.stdout)
	}

	result := runCommand(t, []string{"repository", "unregister", "demo"})
	if result.code != 1 || !containsAll(result.stdout, "category: conflict", "ref-18", "record") {
		t.Fatalf("referenced unregister = (%d, %q), want structured conflict", result.code, result.stdout)
	}
	if inspect := runCommand(t, []string{"repository", "inspect", "demo"}); inspect.code != 0 {
		t.Fatalf("registration was removed after conflict: (%d, %q)", inspect.code, inspect.stdout)
	}
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}
