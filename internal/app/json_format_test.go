package app

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestParseOutputFormatIncludesJSON(t *testing.T) {
	for _, value := range []string{"toon", "human", "json"} {
		got, ok := parseOutputFormat(value)
		if !ok || string(got) != value {
			t.Fatalf("parseOutputFormat(%q) = %q, %t", value, got, ok)
		}
	}
	if _, ok := parseOutputFormat("yaml"); ok {
		t.Fatal("parseOutputFormat accepted yaml")
	}
	options, ok := parseTaskList([]string{"--all", "--format", "json"})
	if !ok || options.Format != outputFormatJSON || !options.All {
		t.Fatalf("parseTaskList json = %#v, %t", options, ok)
	}
	argument, format, ok := parseTaskInspect([]string{"json-output", "--format", "json"})
	if !ok || argument != "json-output" || format != outputFormatJSON {
		t.Fatalf("parseTaskInspect json = %q, %q, %t", argument, format, ok)
	}
	if _, ok := parseTaskList([]string{"--format", "yaml"}); ok {
		t.Fatal("parseTaskList accepted yaml")
	}
}

func TestTaskJSONFormatContract(t *testing.T) {
	setupTaskCommandTest(t)
	t.Setenv("AKAGENT_TEST_SECRET", "operator-only-value")

	created := runCommand(t, []string{"task", "create", "--task-id", "json-output", "--title", `JSON <output> & "test"`})
	if created.code != 0 {
		t.Fatalf("task create = (%d, %q)", created.code, created.stdout)
	}
	if result := runCommand(t, []string{"task", "resource", "create", "json-output", "--resource-id", "app", "--repository", "demo", "--metadata", "zeta=1", "--metadata", "alpha=2"}); result.code != 0 {
		t.Fatalf("resource create = (%d, %q)", result.code, result.stdout)
	}

	defaultList := runCommand(t, []string{"task", "list", "--all"})
	if defaultList.code != 0 || !strings.HasPrefix(defaultList.stdout, "tasks[") {
		t.Fatalf("default list = (%d, %q), want TOON", defaultList.code, defaultList.stdout)
	}
	explicitTOON := runCommand(t, []string{"task", "list", "--all", "--format", "toon"})
	if explicitTOON.code != 0 || explicitTOON.stdout != defaultList.stdout {
		t.Fatalf("explicit toon = (%d, %q), want default TOON", explicitTOON.code, explicitTOON.stdout)
	}

	list := runCommand(t, []string{"task", "list", "--all", "--format", "json"})
	if list.code != 0 {
		t.Fatalf("json list = (%d, %q)", list.code, list.stdout)
	}
	wantList := "{\"tasks\":[{\"id\":\"json-output\",\"title\":\"JSON <output> & \\\"test\\\"\",\"status\":\"created\",\"worker\":\"local\",\"condition\":\"none\"}],\"total\":1}\n"
	if list.stdout != wantList {
		t.Fatalf("json list = %q, want %q", list.stdout, wantList)
	}
	repeat := runCommand(t, []string{"task", "list", "--all", "--format", "json"})
	if repeat.stdout != list.stdout {
		t.Fatalf("json list was not deterministic: %q vs %q", list.stdout, repeat.stdout)
	}
	if strings.Contains(list.stdout, `\u003c`) || strings.Contains(list.stdout, "operator-only-value") {
		t.Fatalf("json list leaked escaped HTML or secret value: %q", list.stdout)
	}

	inspect := runCommand(t, []string{"task", "inspect", "json-output", "--format", "json"})
	if inspect.code != 0 {
		t.Fatalf("json inspect = (%d, %q)", inspect.code, inspect.stdout)
	}
	repeatInspect := runCommand(t, []string{"task", "inspect", "json-output", "--format", "json"})
	if inspect.stdout != repeatInspect.stdout {
		t.Fatalf("json inspect was not deterministic")
	}
	if !strings.HasSuffix(inspect.stdout, "\n") || strings.Contains(inspect.stdout, "\n\n") {
		t.Fatalf("json inspect was not compact: %q", inspect.stdout)
	}
	if strings.Contains(inspect.stdout, "operator-only-value") || strings.Contains(inspect.stdout, os.Getenv("AKAGENT_TEST_SECRET")) {
		t.Fatalf("json inspect exposed a secret value")
	}

	var detail taskDetailView
	if err := json.Unmarshal([]byte(inspect.stdout), &detail); err != nil {
		t.Fatalf("json inspect unmarshal: %v (%q)", err, inspect.stdout)
	}
	if detail.Task.ID != "json-output" || detail.Task.Title != `JSON <output> & "test"` || detail.Task.Status != "created" {
		t.Fatalf("json inspect task = %#v", detail.Task)
	}
	if len(detail.Resources) != 1 || detail.Resources[0].ID != "app" || detail.Resources[0].Repository != "demo" {
		t.Fatalf("json inspect resources = %#v", detail.Resources)
	}
	if detail.Resources[0].Metadata["alpha"] != "2" || detail.Resources[0].Metadata["zeta"] != "1" {
		t.Fatalf("json inspect metadata = %#v", detail.Resources[0].Metadata)
	}
	if !strings.Contains(inspect.stdout, `"metadata":{"alpha":"2","zeta":"1"}`) {
		t.Fatalf("json inspect metadata was not deterministically ordered: %q", inspect.stdout)
	}

	missing := runCommand(t, []string{"task", "inspect", "missing-json", "--format", "json"})
	if missing.code != 1 || !strings.HasPrefix(missing.stdout, "error:") || strings.HasPrefix(missing.stdout, "{") {
		t.Fatalf("missing json inspect = (%d, %q), want TOON error", missing.code, missing.stdout)
	}
	invalid := runCommand(t, []string{"task", "list", "--format", "yaml"})
	if invalid.code != 2 || !strings.Contains(invalid.stdout, "--format <toon|human|json>") {
		t.Fatalf("invalid format = (%d, %q)", invalid.code, invalid.stdout)
	}
}
