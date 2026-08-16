package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTaskCreateDoesNotRequirePi(t *testing.T) {
	setupTaskCommandTest(t)
	t.Setenv("PATH", "")
	result := runCommand(t, []string{"task", "create", "--task-id", "without-pi", "--title", "No provider required"})
	if result.code != 0 || !strings.Contains(result.stdout, "status: created") {
		t.Fatalf("task create without Pi = (%d, %q)", result.code, result.stdout)
	}
}

func TestOneExecutionCoordinatesMultipleResources(t *testing.T) {
	setupTaskCommandTest(t)
	alphaPath := filepath.Join(t.TempDir(), "alpha")
	betaPath := filepath.Join(t.TempDir(), "beta")
	for _, path := range []string{alphaPath, betaPath} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if result := runCommand(t, []string{"repository", "register", "alpha", alphaPath, "--policy", "direct"}); result.code != 0 {
		t.Fatalf("alpha register = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"repository", "register", "beta", betaPath, "--policy", "direct"}); result.code != 0 {
		t.Fatalf("beta register = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "create", "--task-id", "multi-resource", "--title", "Coordinate resources"}); result.code != 0 {
		t.Fatalf("task create = (%d, %q)", result.code, result.stdout)
	}
	for _, resource := range []struct {
		id   string
		name string
	}{
		{id: "alpha-resource", name: "alpha"},
		{id: "beta-resource", name: "beta"},
	} {
		result := runCommand(t, []string{"task", "resource", "create", "multi-resource", "--resource-id", resource.id, "--repository", resource.name})
		if result.code != 0 || !strings.Contains(result.stdout, "id: "+resource.id) {
			t.Fatalf("resource create %s = (%d, %q)", resource.id, result.code, result.stdout)
		}
	}

	created := runCommand(t, []string{"task", "execution", "create", "multi-resource", "--execution-id", "coordinator", "--label", "coordinate resources", "--target", "shell", "--command", "/bin/sh", "--resource", "alpha-resource"})
	if created.code != 0 || !strings.Contains(created.stdout, "resource_id: alpha-resource") {
		t.Fatalf("execution create = (%d, %q)", created.code, created.stdout)
	}
	launched := runCommand(t, []string{"task", "execution", "launch", "multi-resource", "coordinator"})
	if launched.code != 0 || !strings.Contains(launched.stdout, "status: running") {
		t.Fatalf("execution launch = (%d, %q)", launched.code, launched.stdout)
	}
	resources := runCommand(t, []string{"task", "resource", "list", "multi-resource"})
	if resources.code != 0 || !strings.Contains(resources.stdout, "resources[2]") || !strings.Contains(resources.stdout, "alpha-resource") || !strings.Contains(resources.stdout, "beta-resource") {
		t.Fatalf("resource list after execution = (%d, %q)", resources.code, resources.stdout)
	}
}
