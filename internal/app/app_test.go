package app

import (
	"bytes"
	"errors"
	"regexp"
	"strings"
	"testing"
)

func TestGenerateID(t *testing.T) {
	var stdout bytes.Buffer

	if exitCode := Run([]string{"id", "generate"}, &stdout); exitCode != 0 {
		t.Fatalf("Run() exit code = %d, want 0", exitCode)
	}

	idPattern := regexp.MustCompile(`(?m)^id: [0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !idPattern.MatchString(strings.TrimSpace(stdout.String())) {
		t.Fatalf("Run() output = %q, want UUIDv7 TOON", stdout.String())
	}
}

func TestWorkerInspect(t *testing.T) {
	var stdout bytes.Buffer

	if exitCode := Run([]string{"worker", "inspect"}, &stdout); exitCode != 0 {
		t.Fatalf("Run() exit code = %d, want 0", exitCode)
	}

	for _, expected := range []string{"worker:", "id: local", "protocol_version: 1", "operating_system:"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Errorf("Run() output = %q, want to contain %q", stdout.String(), expected)
		}
	}
}

func TestInspectWorkerDetectsAvailableFeatures(t *testing.T) {
	lookPath := func(name string) (string, error) {
		if name == "git" {
			return "/usr/bin/git", nil
		}
		return "", errors.New("not found")
	}

	worker := inspectWorker(lookPath)
	if len(worker.Features) != 1 || worker.Features[0] != "git-worktree" {
		t.Fatalf("inspectWorker() features = %v, want [git-worktree]", worker.Features)
	}
}

func TestUnknownCommand(t *testing.T) {
	var stdout bytes.Buffer

	if exitCode := Run([]string{"missing"}, &stdout); exitCode != 2 {
		t.Fatalf("Run() exit code = %d, want 2", exitCode)
	}

	for _, expected := range []string{"error:", "category: usage", "retryable: false", "akagent --help"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Errorf("Run() output = %q, want to contain %q", stdout.String(), expected)
		}
	}
}

func TestHelp(t *testing.T) {
	var stdout bytes.Buffer

	if exitCode := Run([]string{"--help"}, &stdout); exitCode != 0 {
		t.Fatalf("Run() exit code = %d, want 0", exitCode)
	}

	for _, expected := range []string{"usage: akagent <command>", "repository register <name> <path>", "task <create|resource|execution|credential|launch|list|inspect|attach|publish|finish|stop|archive|clean|reconcile>"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Errorf("Run() output = %q, want to contain %q", stdout.String(), expected)
		}
	}
}

func TestUpdateRejectsUnknownArguments(t *testing.T) {
	var stdout bytes.Buffer

	if exitCode := Run([]string{"update", "--unknown"}, &stdout); exitCode != 2 {
		t.Fatalf("Run() exit code = %d, want 2", exitCode)
	}
	if !strings.Contains(stdout.String(), "Usage: akagent update [--source <path>]") {
		t.Fatalf("Run() output = %q, want update usage", stdout.String())
	}
}
