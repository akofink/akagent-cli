package update

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gofrs/flock"
)

func TestSanitizedGoEnvironmentRemovesAmbientToolchainSettings(t *testing.T) {
	t.Setenv("GOROOT", "/wrong/go")
	t.Setenv("GOTOOLDIR", "/wrong/tool")
	t.Setenv("GOENV", "/wrong/env")
	t.Setenv("GOTOOLCHAIN", "go1.25.0+auto")

	environment := sanitizedGoEnvironment()
	if hasEnvironmentKey(environment, "GOROOT") || hasEnvironmentKey(environment, "GOTOOLDIR") {
		t.Fatalf("sanitized environment retains toolchain paths: %q", environment)
	}
	if !hasEnvironment(environment, "GOENV=off") || !hasEnvironment(environment, "GOTOOLCHAIN=local") {
		t.Fatalf("sanitized environment = %q", environment)
	}
}

func hasEnvironment(environment []string, wanted string) bool {
	for _, entry := range environment {
		if entry == wanted {
			return true
		}
	}
	return false
}

func hasEnvironmentKey(environment []string, wanted string) bool {
	prefix := wanted + "="
	for _, entry := range environment {
		if len(entry) >= len(prefix) && entry[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

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
	runner := func(_ string, env []string, name string, args ...string) ([]byte, error) {
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
			if !hasEnvironment(env, "GOENV=off") || !hasEnvironment(env, "GOTOOLCHAIN=local") || hasEnvironmentKey(env, "GOROOT") || hasEnvironmentKey(env, "GOTOOLDIR") {
				t.Fatalf("go build environment = %q", env)
			}
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
	runner := func(_ string, _ []string, _ string, _ ...string) ([]byte, error) {
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

	_, updateErr := run(sourceDir, executable, func(_ string, _ []string, _ string, _ ...string) ([]byte, error) {
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
	runner := func(_ string, _ []string, name string, args ...string) ([]byte, error) {
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
