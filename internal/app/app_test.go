package app

import (
	"bytes"
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

	for _, expected := range []string{"worker:", "id: local", "protocol_version: 1", "features[2]: tmux,git-worktree"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Errorf("Run() output = %q, want to contain %q", stdout.String(), expected)
		}
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

	for _, expected := range []string{"usage: akagent <command>", "commands[2]: id generate,worker inspect"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Errorf("Run() output = %q, want to contain %q", stdout.String(), expected)
		}
	}
}
