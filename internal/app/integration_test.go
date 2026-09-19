package app

import (
	"strings"
	"testing"
)

func TestIntegrationFamilyIsRemoved(t *testing.T) {
	setupTaskCommandTest(t)
	for _, args := range [][]string{
		{"integration"},
		{"integration", "inspect"},
		{"integration", "launch", "task"},
		{"integration", "status"},
	} {
		result := runCommand(t, args)
		if result.code != 2 || !strings.Contains(result.stdout, "The `integration` command family was removed from akagent") || !strings.Contains(result.stdout, "category: usage") {
			t.Fatalf("Run(%q) = (%d, %q), want removed integration family error", args, result.code, result.stdout)
		}
	}
}
