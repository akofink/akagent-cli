package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akofink/akagent-cli/internal/integration"
)

func TestIntegrationInspectReportsEnabledWhenSignalIsMissing(t *testing.T) {
	unsetIntegrationSignal(t)

	result := runCommand(t, []string{"integration", "inspect"})
	want := "integration:\n  enabled: true\n  signal: AKAGENT_ENABLED\n  reason: AKAGENT_ENABLED is unset\n"
	if result.code != 0 || result.stdout != want {
		t.Fatalf("integration inspect = (%d, %q), want (0, %q)", result.code, result.stdout, want)
	}
}

func TestIntegrationInspectDoesNotExposeEnabledSignalValue(t *testing.T) {
	secretValue := "operator-only-value"
	t.Setenv(integration.EnableEnv, secretValue)

	result := runCommand(t, []string{"integration", "inspect"})
	if result.code != 0 || !strings.Contains(result.stdout, "enabled: true") || !strings.Contains(result.stdout, "not set to 0") {
		t.Fatalf("integration inspect = (%d, %q), want enabled reason", result.code, result.stdout)
	}
	if strings.Contains(result.stdout, secretValue) {
		t.Fatalf("integration inspect exposed the signal value")
	}
}

func TestIntegrationInspectReportsDisabledForZero(t *testing.T) {
	t.Setenv(integration.EnableEnv, "0")

	result := runCommand(t, []string{"integration", "inspect"})
	want := "integration:\n  enabled: false\n  signal: AKAGENT_ENABLED\n  reason: AKAGENT_ENABLED is set to 0\n"
	if result.code != 0 || result.stdout != want {
		t.Fatalf("integration inspect = (%d, %q), want (0, %q)", result.code, result.stdout, want)
	}
}

func TestDirectHumanCommandIgnoresIntegrationGate(t *testing.T) {
	unsetIntegrationSignal(t)

	result := runCommand(t, []string{"id", "generate"})
	if result.code != 0 || !strings.HasPrefix(result.stdout, "id: ") {
		t.Fatalf("id generate = (%d, %q), want successful direct command", result.code, result.stdout)
	}
}

func TestWorkflowLaunchSkipsWithoutCreatingStateWhenDisabled(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv(integration.EnableEnv, "0")

	result := runCommand(t, []string{"integration", "launch", "missing-task", "--execution-id", "workflow-one", "--command", "/bin/sh"})
	want := "workflow:\n  enabled: false\n  skipped: true\n"
	if result.code != 0 || result.stdout != want {
		t.Fatalf("disabled workflow launch = (%d, %q), want (0, %q)", result.code, result.stdout, want)
	}
	if _, err := os.Stat(filepath.Join(root, "state")); !os.IsNotExist(err) {
		t.Fatalf("disabled workflow launch created state: %v", err)
	}
}

func TestWorkflowLaunchUsesGenericExecutionLifecycle(t *testing.T) {
	setupTaskCommandTest(t)
	repositoryPath := filepath.Join(t.TempDir(), "repository")
	if err := os.Mkdir(repositoryPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if result := runCommand(t, []string{"repository", "register", "demo", repositoryPath, "--policy", "direct"}); result.code != 0 {
		t.Fatalf("repository register = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "create", "--task-id", "workflow-task", "--title", "Workflow", "--repository", "demo"}); result.code != 0 {
		t.Fatalf("task create = (%d, %q)", result.code, result.stdout)
	}
	result := runCommand(t, []string{"integration", "launch", "workflow-task", "--execution-id", "workflow-one", "--command", "/bin/sh", "--arg", "-c", "--arg", "printf workflow", "--label", "workflow-run"})
	if result.code != 0 || !strings.Contains(result.stdout, "target: workflow") || !strings.Contains(result.stdout, "status: running") {
		t.Fatalf("workflow launch = (%d, %q), want running workflow execution", result.code, result.stdout)
	}
	repeated := runCommand(t, []string{"integration", "launch", "workflow-task", "--execution-id", "workflow-one", "--command", "/bin/sh", "--arg", "-c", "--arg", "printf workflow", "--label", "workflow-run"})
	if repeated.code != 0 || !strings.Contains(repeated.stdout, "execution_id: workflow-one") {
		t.Fatalf("repeated workflow launch = (%d, %q), want idempotent execution", repeated.code, repeated.stdout)
	}
	executions := runCommand(t, []string{"task", "execution", "inspect", "workflow-task", "workflow-one"})
	if executions.code != 0 || !strings.Contains(executions.stdout, "target: workflow") || !strings.Contains(executions.stdout, "command: /bin/sh") {
		t.Fatalf("workflow execution inspect = (%d, %q)", executions.code, executions.stdout)
	}
}

func TestIntegrationInspectRejectsUnknownArguments(t *testing.T) {
	result := runCommand(t, []string{"integration", "status"})
	if result.code != 2 || !strings.Contains(result.stdout, "Usage: akagent integration <inspect|launch>") {
		t.Fatalf("integration status = (%d, %q), want usage error", result.code, result.stdout)
	}
}

func unsetIntegrationSignal(t *testing.T) {
	t.Helper()
	value, set := os.LookupEnv(integration.EnableEnv)
	if err := os.Unsetenv(integration.EnableEnv); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if set {
			_ = os.Setenv(integration.EnableEnv, value)
		} else {
			_ = os.Unsetenv(integration.EnableEnv)
		}
	})
}
