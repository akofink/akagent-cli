package app

import (
	"os"
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

func TestIntegrationInspectRejectsUnknownArguments(t *testing.T) {
	result := runCommand(t, []string{"integration", "status"})
	if result.code != 2 || !strings.Contains(result.stdout, "Usage: akagent integration inspect") {
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
