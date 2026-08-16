package store

import (
	"testing"
	"time"
)

func TestResourceRoundTripAndIndependentArchive(t *testing.T) {
	state := openTest(t)
	taskID := validTaskID(t)
	if err := state.WriteManifest(taskID, Manifest{Title: "multi", Lifecycle: "stopped"}); err != nil {
		t.Fatal(err)
	}
	one := Resource{ID: "one", TaskID: taskID, Repository: "alpha", Branch: "feature/one", WorktreePath: "/tmp/one", Git: GitFacts{Path: "/tmp/one", Head: "one", Dirty: true}}
	two := Resource{ID: "two", TaskID: taskID, Repository: "beta", Branch: "feature/two", WorktreePath: "/tmp/two", Git: GitFacts{Path: "/tmp/two", Head: "two", Committed: true}}
	if created, _, err := state.CreateResource(taskID, one); err != nil || !created {
		t.Fatalf("CreateResource(one) = %v, %v", created, err)
	}
	if created, _, err := state.CreateResource(taskID, two); err != nil || !created {
		t.Fatalf("CreateResource(two) = %v, %v", created, err)
	}
	ids, err := state.ResourceIDs(taskID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids[0] != "one" || ids[1] != "two" {
		t.Fatalf("ResourceIDs() = %v, want [one two]", ids)
	}
	if _, err := state.AppendResourceEvent(taskID, "one", Event{Operation: "create"}); err != nil {
		t.Fatal(err)
	}
	archive := ResourceArchive{TaskID: taskID, ResourceID: "one", CapturedAt: time.Now().UTC(), Resource: one}
	if err := state.WriteResourceArchive(taskID, "one", archive); err != nil {
		t.Fatal(err)
	}
	if _, err := state.ReadResourceArchive(taskID, "two"); !IsKind(err, KindNotFound) {
		t.Fatalf("ReadResourceArchive(two) = %v, want not found", err)
	}
	got, err := state.ReadResource("one", "one")
	if err == nil || got.ID != "" {
		t.Fatalf("ReadResource with wrong task = %#v, %v", got, err)
	}
}

func TestResourceEventsAreContiguous(t *testing.T) {
	state := openTest(t)
	taskID := validTaskID(t)
	if err := state.WriteManifest(taskID, Manifest{Title: "events"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := state.CreateResource(taskID, Resource{ID: "r", TaskID: taskID, Repository: "repo"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := state.AppendResourceEvent(taskID, "r", Event{Operation: "step"}); err != nil {
			t.Fatal(err)
		}
	}
	events, err := state.ReadResourceEvents(taskID, "r")
	if err != nil || len(events) != 3 {
		t.Fatalf("ReadResourceEvents() = %v, %v", events, err)
	}
}
