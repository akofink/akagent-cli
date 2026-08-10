package update

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gofrs/flock"
)

func TestRunUpdatesSourceAndReplacesBinary(t *testing.T) {
	sourceDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(sourceDir, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	installDir := t.TempDir()
	executable := filepath.Join(installDir, "akagent")
	if err := os.WriteFile(executable, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	revisions := []string{"before\n", "after\n"}
	runner := func(_ string, name string, args ...string) ([]byte, error) {
		command := append([]string{name}, args...)
		switch {
		case reflect.DeepEqual(command, []string{"git", "status", "--porcelain"}):
			return nil, nil
		case reflect.DeepEqual(command, []string{"git", "branch", "--show-current"}):
			return []byte("main\n"), nil
		case reflect.DeepEqual(command, []string{"git", "rev-parse", "HEAD"}):
			revision := revisions[0]
			revisions = revisions[1:]
			return []byte(revision), nil
		case reflect.DeepEqual(command, []string{"git", "fetch", "origin"}):
			return nil, nil
		case reflect.DeepEqual(command, []string{"git", "merge", "--ff-only", "origin/main"}):
			return nil, nil
		case len(command) == 6 && reflect.DeepEqual(command[:4], []string{"git", "worktree", "add", "--detach"}):
			return nil, os.MkdirAll(command[4], 0o700)
		case len(command) == 5 && reflect.DeepEqual(command[:4], []string{"git", "worktree", "remove", "--force"}):
			return nil, nil
		case len(command) == 5 && reflect.DeepEqual(command[:3], []string{"go", "build", "-o"}):
			return nil, os.WriteFile(command[3], []byte("new"), 0o600)
		default:
			return nil, errors.New("unexpected command")
		}
	}

	result, updateErr := run(sourceDir, executable, runner)
	if updateErr != nil {
		t.Fatalf("run() error = %#v", updateErr)
	}
	if !result.SourceChanged || result.SourceBefore != "before" || result.SourceAfter != "after" || !result.Reinstalled {
		t.Fatalf("run() result = %#v", result)
	}
	installed, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if string(installed) != "new" {
		t.Fatalf("installed binary = %q, want new", installed)
	}
}

func TestRunRefusesDirtySource(t *testing.T) {
	sourceDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(sourceDir, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	runner := func(_ string, _ string, _ ...string) ([]byte, error) {
		return []byte(" M internal/app/app.go\n"), nil
	}

	_, updateErr := run(sourceDir, filepath.Join(t.TempDir(), "akagent"), runner)
	if updateErr == nil || updateErr.Category != "conflict" {
		t.Fatalf("run() error = %#v, want conflict", updateErr)
	}
}

func TestRunRefusesConcurrentUpdate(t *testing.T) {
	sourceDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(sourceDir, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "akagent")
	updateLock := flock.New(executable + ".update.lock")
	if err := updateLock.Lock(); err != nil {
		t.Fatal(err)
	}
	defer updateLock.Unlock()

	_, updateErr := run(sourceDir, executable, func(_ string, _ string, _ ...string) ([]byte, error) {
		t.Fatal("runner called while update lock held")
		return nil, nil
	})
	if updateErr == nil || updateErr.Category != "retryable" {
		t.Fatalf("run() error = %#v, want retryable", updateErr)
	}
}

func TestRunPreservesBinaryWhenBuildFails(t *testing.T) {
	sourceDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(sourceDir, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "akagent")
	if err := os.WriteFile(executable, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	revisionCalls := 0
	runner := func(_ string, name string, args ...string) ([]byte, error) {
		command := append([]string{name}, args...)
		switch {
		case reflect.DeepEqual(command, []string{"git", "status", "--porcelain"}):
			return nil, nil
		case reflect.DeepEqual(command, []string{"git", "branch", "--show-current"}):
			return []byte("main\n"), nil
		case reflect.DeepEqual(command, []string{"git", "rev-parse", "HEAD"}):
			revisionCalls++
			return []byte("same\n"), nil
		case reflect.DeepEqual(command, []string{"git", "fetch", "origin"}), reflect.DeepEqual(command, []string{"git", "merge", "--ff-only", "origin/main"}):
			return nil, nil
		case len(command) == 6 && reflect.DeepEqual(command[:4], []string{"git", "worktree", "add", "--detach"}):
			return nil, os.MkdirAll(command[4], 0o700)
		case len(command) == 5 && reflect.DeepEqual(command[:4], []string{"git", "worktree", "remove", "--force"}):
			return nil, nil
		case len(command) >= 2 && command[0] == "go" && command[1] == "build":
			return nil, errors.New("build failed")
		default:
			return nil, errors.New("unexpected command")
		}
	}

	_, updateErr := run(sourceDir, executable, runner)
	if updateErr == nil || updateErr.Message != "Failed to build the updated akagent binary" {
		t.Fatalf("run() error = %#v", updateErr)
	}
	if revisionCalls != 2 {
		t.Fatalf("revision calls = %d, want 2", revisionCalls)
	}
	installed, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if string(installed) != "old" {
		t.Fatalf("installed binary = %q, want old", installed)
	}
}
