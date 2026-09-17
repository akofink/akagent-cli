package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/akofink/akagent-cli/internal/output"
	"github.com/akofink/akagent-cli/internal/store"
)

func TestExternalLifecycleProjectionMatrix(t *testing.T) {
	setupTaskCommandTest(t)
	const taskID = "projection-matrix"
	const resourceID = "projection-resource"
	const executionID = "projection-execution"

	if result := runCommand(t, []string{"task", "external", "create", taskID, "--title", "Projection matrix", "--caller-id", "caller", "--operation-id", "task-create"}); result.code != 0 {
		t.Fatalf("task create = (%d, %q)", result.code, result.stdout)
	}
	assertProjectionMatrix(t, taskID, "", "", "task created")
	if result := runCommand(t, []string{"task", "external", "resource", "create", taskID, "--resource-id", resourceID, "--repository", "offline", "--branch", "main", "--base", "base", "--head", "head", "--worktree", "/offline/missing", "--caller-id", "caller", "--operation-id", "resource-create"}); result.code != 0 {
		t.Fatalf("resource create = (%d, %q)", result.code, result.stdout)
	}
	assertProjectionMatrix(t, taskID, resourceID, "", "resource created")

	if result := runCommand(t, []string{"task", "external", "execution", "create", taskID, "--execution-id", executionID, "--resource", resourceID, "--caller-id", "caller", "--operation-id", "execution-create", "--tool", "pi", "--session-id", "projection-session"}); result.code != 0 {
		t.Fatalf("execution create = (%d, %q)", result.code, result.stdout)
	}
	assertProjectionMatrix(t, taskID, resourceID, executionID, "execution created")

	observedAt := time.Now().UTC().Format(time.RFC3339Nano)
	if result := runCommand(t, []string{"task", "execution", "observe", taskID, executionID, "--caller-id", "caller", "--operation-id", "observation", "--expected-revision", "1", "--source", "worker", "--observed-at", observedAt, "--host-id", "host", "--boot-id", "boot", "--process-state", "running", "--result", "unknown", "--detail", "historical"}); result.code != 0 {
		t.Fatalf("observation = (%d, %q)", result.code, result.stdout)
	}
	assertProjectionMatrix(t, taskID, resourceID, executionID, "observation recorded")

	if result := runCommand(t, []string{"task", "execution", "finish", taskID, executionID, "--caller-id", "caller", "--operation-id", "execution-finish", "--expected-revision", "2", "--contract", "external-v1", "--result", "succeeded"}); result.code != 0 {
		t.Fatalf("execution finish = (%d, %q)", result.code, result.stdout)
	}
	assertProjectionMatrix(t, taskID, resourceID, executionID, "execution finished")

	if result := runCommand(t, []string{"task", "external", "execution", "archive", taskID, executionID, "--caller-id", "caller", "--operation-id", "execution-archive", "--expected-revision", "3"}); result.code != 0 {
		t.Fatalf("execution archive = (%d, %q)", result.code, result.stdout)
	}
	assertProjectionMatrix(t, taskID, resourceID, executionID, "execution archived")

	if result := runCommand(t, []string{"task", "external", "resource", "archive", taskID, resourceID, "--caller-id", "caller", "--operation-id", "resource-archive", "--expected-revision", "1"}); result.code != 0 {
		t.Fatalf("resource archive = (%d, %q)", result.code, result.stdout)
	}
	assertProjectionMatrix(t, taskID, resourceID, executionID, "resource archived")

	if result := runCommand(t, []string{"task", "external", "finish", taskID, "--caller-id", "caller", "--operation-id", "task-finish", "--expected-revision", "3", "--contract", "external-v1", "--result", "succeeded"}); result.code != 0 {
		t.Fatalf("task finish = (%d, %q)", result.code, result.stdout)
	}
	assertProjectionMatrix(t, taskID, resourceID, executionID, "task finished")

	if result := runCommand(t, []string{"task", "external", "archive", taskID, "--caller-id", "caller", "--operation-id", "task-archive", "--expected-revision", "4"}); result.code != 0 {
		t.Fatalf("task archive = (%d, %q)", result.code, result.stdout)
	}
	assertProjectionMatrix(t, taskID, resourceID, executionID, "task archived")
}

func assertProjectionMatrix(t *testing.T, taskID, resourceID, executionID, phase string) {
	t.Helper()
	commands := [][]string{
		{"task", "list"},
		{"task", "list", "--all"},
		{"task", "list", "--all", "--format", "human"},
		{"task", "list", "--all", "--format", "json"},
		{"task", "inspect", taskID},
		{"task", "inspect", taskID, "--format", "human"},
		{"task", "inspect", taskID, "--format", "json"},
		{"task", "checkpoint", "inspect", taskID},
	}
	if resourceID != "" {
		commands = append(commands,
			[]string{"task", "resource", "list", taskID},
			[]string{"task", "resource", "inspect", taskID, resourceID},
		)
	}
	if executionID != "" {
		commands = append(commands,
			[]string{"task", "execution", "list", taskID},
			[]string{"task", "execution", "inspect", taskID, executionID},
			[]string{"task", "execution", "evidence", "list", taskID, executionID},
		)
	}
	for _, command := range commands {
		result := runCommand(t, command)
		if result.code != 0 {
			t.Fatalf("%s: %q = (%d, %q)", phase, command, result.code, result.stdout)
		}
	}

	if executionID == "" {
		return
	}
	evidence := runCommand(t, []string{"task", "execution", "evidence", "list", taskID, executionID})
	captureID := ""
	for _, token := range strings.FieldsFunc(evidence.stdout, func(r rune) bool {
		return r == ',' || r == '\n' || r == ' ' || r == '\t'
	}) {
		if strings.HasPrefix(token, "ref-") {
			captureID = token
			break
		}
	}
	if captureID != "" {
		inspected := runCommand(t, []string{"task", "execution", "evidence", "inspect", taskID, executionID, captureID})
		if inspected.code != 0 {
			t.Fatalf("%s: evidence inspect = (%d, %q)", phase, inspected.code, inspected.stdout)
		}
	}
}

func TestProjectionViewsPreserveTypedOptionalData(t *testing.T) {
	observedAt := time.Date(2026, 9, 13, 20, 0, 0, 0, time.UTC)
	completion := &store.ExternalCompletion{Contract: "external-v1", Result: "succeeded", CallerID: "caller", DeclaredAt: observedAt}
	value := taskDetailView{
		Task: taskView{
			ID:                 "projection-task",
			Provenance:         store.ProvenanceExternal,
			CallerID:           "caller",
			Revision:           4,
			ExternalCompletion: viewExternalCompletion(completion),
			Title:              "Projection task",
			Status:             "finished",
			Worker:             "local",
		},
		Resources: []resourceListItem{{
			ID:           "projection-resource",
			Repository:   "offline",
			Metadata:     map[string]string{"delivery": "ready", "unknown": "preserved"},
			ExternalURLs: []string{"https://forge.example/pull/151"},
		}},
		Executions: []executionView{{
			ID:     "projection-execution",
			TaskID: "projection-task",
			Label:  "external",
			Target: "tool",
			Status: "finished",
			ExternalObservations: []externalObservationView{{
				Source:       "worker",
				ObservedAt:   observedAt,
				HostID:       "host",
				BootID:       "boot",
				ProcessState: "running",
				Result:       "unknown",
				Detail:       "historical",
			}},
			ExternalCompletion: viewExternalCompletion(completion),
			SessionReferences: []sessionReferenceView{{
				Tool:          "pi",
				SessionID:     "session-151",
				ReferencePath: "/private/tmp/session-151.json",
			}},
		}},
	}
	encoded, err := output.Encode(value)
	if err != nil {
		t.Fatalf("Encode projection matrix = %v", err)
	}
	for _, expected := range []string{"external_completion", "external-v1", "historical", "delivery", "session-151", "https://forge.example/pull/151"} {
		if !strings.Contains(encoded, expected) {
			t.Fatalf("Encode projection matrix missing %q in %q", expected, encoded)
		}
	}
}

func TestLegacyProjectionMatrix(t *testing.T) {
	setupTaskCommandTest(t)
	state, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	if err := state.WriteManifest("legacy-projection", store.Manifest{Title: "Legacy projection", Worker: "local", Lifecycle: "created", Condition: "none"}); err != nil {
		t.Fatal(err)
	}
	for _, command := range [][]string{{"task", "list"}, {"task", "list", "--all"}, {"task", "list", "--all", "--format", "human"}, {"task", "inspect", "legacy-projection"}, {"task", "inspect", "legacy-projection", "--format", "human"}, {"task", "checkpoint", "inspect", "legacy-projection"}} {
		result := runCommand(t, command)
		if result.code != 0 || !strings.Contains(result.stdout, "legacy-projection") && strings.Contains(strings.Join(command, " "), "task list") {
			t.Fatalf("legacy projection %q = (%d, %q)", command, result.code, result.stdout)
		}
	}
	if result := runCommand(t, []string{"task", "finish", "legacy-projection", "succeeded", "legacy result"}); result.code != 0 {
		t.Fatalf("legacy finish = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "archive", "legacy-projection"}); result.code != 0 {
		t.Fatalf("legacy archive = (%d, %q)", result.code, result.stdout)
	}
	for _, command := range [][]string{{"task", "list", "--all"}, {"task", "inspect", "legacy-projection"}, {"task", "inspect", "legacy-projection", "--format", "human"}} {
		result := runCommand(t, command)
		if result.code != 0 || !strings.Contains(result.stdout, "legacy result") {
			t.Fatalf("archived legacy projection %q = (%d, %q)", command, result.code, result.stdout)
		}
	}
}

func TestUnknownAndMalformedOptionalProjectionFields(t *testing.T) {
	setupTaskCommandTest(t)
	if result := runCommand(t, []string{"task", "external", "create", "unknown-optional", "--title", "Unknown optional", "--caller-id", "caller", "--operation-id", "create"}); result.code != 0 {
		t.Fatalf("unknown task create = (%d, %q)", result.code, result.stdout)
	}
	if result := runCommand(t, []string{"task", "external", "finish", "unknown-optional", "--caller-id", "caller", "--operation-id", "finish", "--expected-revision", "1", "--contract", "external-v1", "--result", "succeeded"}); result.code != 0 {
		t.Fatalf("unknown task finish = (%d, %q)", result.code, result.stdout)
	}
	addUnknownCompletionField(t, "unknown-optional")
	for _, command := range [][]string{{"task", "inspect", "unknown-optional"}, {"task", "list", "--all"}, {"task", "inspect", "unknown-optional", "--format", "human"}} {
		if result := runCommand(t, command); result.code != 0 || !strings.Contains(result.stdout, "external-v1") {
			t.Fatalf("unknown optional field %q = (%d, %q)", command, result.code, result.stdout)
		}
	}

	if result := runCommand(t, []string{"task", "external", "create", "malformed-optional", "--title", "Malformed optional", "--caller-id", "caller", "--operation-id", "create"}); result.code != 0 {
		t.Fatalf("malformed task create = (%d, %q)", result.code, result.stdout)
	}
	setManifestOptionalField(t, "malformed-optional", "external_completion", "malformed")
	result := runCommand(t, []string{"task", "inspect", "malformed-optional"})
	if result.code != 1 || !strings.Contains(result.stdout, "error:") {
		t.Fatalf("malformed optional field = (%d, %q), want structured error", result.code, result.stdout)
	}
}

func addUnknownCompletionField(t *testing.T, taskID string) {
	t.Helper()
	setManifestOptionalField(t, taskID, "external_completion.unknown_optional", "preserve")
}

func setManifestOptionalField(t *testing.T, taskID, path, value string) {
	t.Helper()
	state, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(state.Root(), "tasks", taskID, "manifest.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	payload, ok := envelope["data"].(map[string]any)
	if ok {
		data = payload
	} else {
		encoded, ok := envelope["data"].(string)
		if !ok || json.Unmarshal([]byte(encoded), &data) != nil {
			t.Fatalf("manifest data has unexpected shape: %#v", envelope["data"])
		}
	}
	parts := strings.Split(path, ".")
	current := data
	for _, part := range parts[:len(parts)-1] {
		next, ok := current[part].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[part] = next
		}
		current = next
	}
	current[parts[len(parts)-1]] = value
	encodedData, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	envelope["data"] = json.RawMessage(encodedData)
	encodedEnvelope, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, encodedEnvelope, 0o600); err != nil {
		t.Fatal(err)
	}
}
