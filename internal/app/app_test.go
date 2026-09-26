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

	for _, expected := range []string{"worker:", "id: local", "protocol_version: 2", "features[4]: registry,checkpoint,observation,handoff", "operating_system:", "build_revision: unknown"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Errorf("Run() output = %q, want to contain %q", stdout.String(), expected)
		}
	}
}

func TestInspectWorkerReportsDeclarativeRecordCapabilities(t *testing.T) {
	worker := inspectWorker()
	if len(worker.Features) != 4 || worker.Features[0] != "registry" || worker.Features[1] != "checkpoint" || worker.Features[2] != "observation" || worker.Features[3] != "handoff" {
		t.Fatalf("inspectWorker() features = %v, want declarative record capabilities", worker.Features)
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

	for _, expected := range []string{"usage: akagent <command>", "repository register <name> <path>", "task <create|external|checkpoint|resource|execution|list|inspect|publish|finish|archive|reconcile>", "task disposition <task-id> <in-flight|deferred|terminal>", "task list [keyword] [--view <in-flight|attention|maintenance|deferred|history>]"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Errorf("Run() output = %q, want to contain %q", stdout.String(), expected)
		}
	}
}

func TestHomeHelpPromotesSelfService(t *testing.T) {
	var stdout bytes.Buffer

	if exitCode := Run(nil, &stdout); exitCode != 0 {
		t.Fatalf("Run() exit code = %d, want 0", exitCode)
	}

	for _, expected := range []string{"Manage local coding-agent tasks", "self-service task lifecycle management", "akagent worker inspect"} {
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
