package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akofink/akagent-cli/internal/store"
)

// TestMain isolates commands even when an individual test omits its setup helper.
func TestMain(m *testing.M) {
	root, err := os.MkdirTemp("", "akagent-app-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for key, value := range map[string]string{
		"HOME":           filepath.Join(root, "home"),
		"XDG_STATE_HOME": filepath.Join(root, "state"),
	} {
		if err := os.Setenv(key, value); err != nil {
			fmt.Fprintln(os.Stderr, err)
			_ = os.RemoveAll(root)
			os.Exit(1)
		}
	}
	code := m.Run()
	if err := os.RemoveAll(root); err != nil {
		fmt.Fprintln(os.Stderr, err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

func TestPackageDefaultStoreIsIsolated(t *testing.T) {
	state, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	if root := filepath.Dir(os.Getenv("XDG_STATE_HOME")); !strings.HasPrefix(filepath.Base(root), "akagent-app-test-") || state.Root() != filepath.Join(root, "state", "akagent") {
		t.Fatalf("default store escaped the package test directory: %q", state.Root())
	}
	if result := runCommand(t, []string{"task", "create", "--task-id", "package-isolation-contract", "--title", "Isolated"}); result.code != 0 {
		t.Fatalf("create without per-test setup = %+v", result)
	}
}
