package app

import (
	"strings"
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

func TestArchiveCompleteFinishedWorkIsHistoryNotInFlight(t *testing.T) {
	leftoverInFlight := store.Manifest{
		Lifecycle: "finished", Condition: "none", Result: "succeeded",
		Disposition: string(lifecycle.DispositionInFlight), ArchiveState: "complete",
	}
	externalComplete := store.Manifest{
		Lifecycle: "finished", Condition: "none", Result: "succeeded",
		Disposition: string(lifecycle.DispositionInFlight), ArchiveState: "complete",
		ExternalCompletion: &store.ExternalCompletion{Contract: "operations-v1", Result: "succeeded", CallerID: "pi"},
	}
	unfinishedArchived := store.Manifest{
		Lifecycle: "created", Condition: "active",
		Disposition: string(lifecycle.DispositionInFlight), ArchiveState: "complete",
	}
	finishedPendingArchive := store.Manifest{
		Lifecycle: "finished", Condition: "none", Result: "succeeded",
		Disposition: string(lifecycle.DispositionInFlight), ArchiveState: "pending",
	}
	deferredComplete := store.Manifest{
		Lifecycle: "finished", Condition: "none", Result: "succeeded",
		Disposition: string(lifecycle.DispositionDeferred), ArchiveState: "complete",
	}

	if includeTaskInView(leftoverInFlight, nil, "") || includeTaskInView(leftoverInFlight, nil, "in-flight") || includeTaskInView(leftoverInFlight, nil, "attention") {
		t.Fatal("archive-complete finished work remained in the default or in-flight views")
	}
	if !includeTaskInView(leftoverInFlight, nil, "history") || includeTaskInView(leftoverInFlight, nil, "maintenance") {
		t.Fatal("archive-complete finished work was not classified as clean history")
	}
	if includeTaskInView(externalComplete, nil, "in-flight") || !includeTaskInView(externalComplete, nil, "history") {
		t.Fatal("archive-complete external completion remained in-flight")
	}
	if !includeTaskInView(unfinishedArchived, nil, "") || !includeTaskInView(unfinishedArchived, nil, "in-flight") {
		t.Fatal("unfinished archive-complete work was hidden from in-flight")
	}
	if !includeTaskInView(finishedPendingArchive, nil, "in-flight") || !includeTaskInView(finishedPendingArchive, nil, "maintenance") {
		t.Fatal("finished work with pending archive left the in-flight or maintenance views")
	}
	if includeTaskInView(deferredComplete, nil, "in-flight") || !includeTaskInView(deferredComplete, nil, "deferred") || includeTaskInView(deferredComplete, nil, "history") {
		t.Fatal("explicit deferred archive-complete work was reclassified")
	}
}

func TestArchiveCompleteExternalTaskLeavesDefaultInventory(t *testing.T) {
	setupTaskCommandTest(t)
	created := runCommand(t, []string{"task", "external", "create", "archived-complete", "--title", "Archived complete", "--caller-id", "caller", "--operation-id", "task-create"})
	if created.code != 0 {
		t.Fatalf("external create = (%d, %q)", created.code, created.stdout)
	}
	inFlight := runCommand(t, []string{"task", "list", "--view", "in-flight"})
	if inFlight.code != 0 || !strings.Contains(inFlight.stdout, "archived-complete") {
		t.Fatalf("created external task missing from in-flight = (%d, %q)", inFlight.code, inFlight.stdout)
	}
	finished := runCommand(t, []string{"task", "external", "finish", "archived-complete", "--caller-id", "caller", "--operation-id", "task-complete", "--expected-revision", "1", "--contract", "external-v1", "--result", "succeeded"})
	if finished.code != 0 {
		t.Fatalf("external finish = (%d, %q)", finished.code, finished.stdout)
	}
	archived := runCommand(t, []string{"task", "external", "archive", "archived-complete", "--caller-id", "caller", "--operation-id", "task-archive", "--expected-revision", "2"})
	if archived.code != 0 || !strings.Contains(archived.stdout, "archive_state: complete") {
		t.Fatalf("external archive = (%d, %q)", archived.code, archived.stdout)
	}
	defaultList := runCommand(t, []string{"task", "list"})
	inFlight = runCommand(t, []string{"task", "list", "--view", "in-flight"})
	attention := runCommand(t, []string{"task", "list", "--view", "attention"})
	history := runCommand(t, []string{"task", "list", "--view", "history"})
	all := runCommand(t, []string{"task", "list", "--all"})
	if defaultList.code != 0 || strings.Contains(defaultList.stdout, "archived-complete") {
		t.Fatalf("default list retained archive-complete finished work = (%d, %q)", defaultList.code, defaultList.stdout)
	}
	if inFlight.code != 0 || strings.Contains(inFlight.stdout, "archived-complete") {
		t.Fatalf("in-flight list retained archive-complete finished work = (%d, %q)", inFlight.code, inFlight.stdout)
	}
	if attention.code != 0 || strings.Contains(attention.stdout, "archived-complete") {
		t.Fatalf("attention list retained archive-complete finished work = (%d, %q)", attention.code, attention.stdout)
	}
	if history.code != 0 || !strings.Contains(history.stdout, "archived-complete") {
		t.Fatalf("history omitted archive-complete finished work = (%d, %q)", history.code, history.stdout)
	}
	if all.code != 0 || !strings.Contains(all.stdout, "archived-complete") {
		t.Fatalf("--all omitted archive-complete finished work = (%d, %q)", all.code, all.stdout)
	}
}
