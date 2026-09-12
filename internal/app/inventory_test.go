package app

import (
	"testing"

	"github.com/akofink/akagent-cli/internal/lifecycle"
	"github.com/akofink/akagent-cli/internal/store"
)

func TestParseTaskListInventoryViews(t *testing.T) {
	for _, name := range []string{"in-flight", "attention", "maintenance", "deferred", "history"} {
		t.Run(name, func(t *testing.T) {
			options, ok := parseTaskList([]string{"--view", name})
			if !ok || options.View != name {
				t.Fatalf("parseTaskList() = %#v, %t, want view %q", options, ok, name)
			}
		})
	}
	if _, ok := parseTaskList([]string{"--view", "unknown"}); ok {
		t.Fatal("parseTaskList accepted an unknown inventory view")
	}
}

func TestTaskInventoryViewsSeparateAcceptedWorkAndMaintenance(t *testing.T) {
	inFlight := store.Manifest{Lifecycle: "running", Condition: "waiting", Disposition: string(lifecycle.DispositionInFlight)}
	deferred := store.Manifest{Lifecycle: "finished", Condition: "none", Disposition: string(lifecycle.DispositionDeferred)}
	terminalDebt := store.Manifest{Lifecycle: "finished", Condition: "succeeded", Result: "done", Disposition: string(lifecycle.DispositionTerminal), CleanupDebt: true}
	terminal := store.Manifest{Lifecycle: "finished", Condition: "succeeded", Result: "done", Disposition: string(lifecycle.DispositionTerminal)}
	legacyTerminalDebt := store.Manifest{Lifecycle: "finished", Condition: "succeeded", Result: "done", CleanupDebt: true}
	unknown := store.Manifest{Lifecycle: "running", Condition: "none", Disposition: "operator-review"}

	if !includeTaskInView(inFlight, nil, "") || !includeTaskInView(inFlight, nil, "in-flight") || !includeTaskInView(inFlight, nil, "attention") {
		t.Fatal("waiting in-flight work was omitted from the expected views")
	}
	if includeTaskInView(deferred, nil, "") || !includeTaskInView(deferred, nil, "deferred") {
		t.Fatal("deferred work was included in the wrong views")
	}
	if includeTaskInView(terminalDebt, nil, "") || includeTaskInView(terminalDebt, nil, "in-flight") || !includeTaskInView(terminalDebt, nil, "maintenance") || !includeTaskInView(terminalDebt, nil, "history") {
		t.Fatal("terminal cleanup debt was not separated into maintenance and history")
	}
	if includeTaskInView(terminal, nil, "") || !includeTaskInView(terminal, nil, "history") || includeTaskInView(terminal, nil, "maintenance") {
		t.Fatal("clean terminal history was classified incorrectly")
	}
	if includeTaskInView(legacyTerminalDebt, nil, "") || !includeTaskInView(legacyTerminalDebt, nil, "maintenance") || !includeTaskInView(legacyTerminalDebt, nil, "history") {
		t.Fatal("legacy terminal cleanup debt was classified incorrectly")
	}
	if !includeTaskInView(unknown, nil, "") || !includeTaskInView(unknown, nil, "in-flight") {
		t.Fatal("unknown disposition was not preserved conservatively")
	}
}
