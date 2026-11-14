package app

import (
	"strings"
	"testing"
)

func requireContains(t *testing.T, label string, result commandResult, code int, want ...string) {
	t.Helper()
	if result.code != code {
		t.Fatalf("%s = (%d, %q), want exit %d", label, result.code, result.stdout, code)
	}
	for _, fragment := range want {
		if !strings.Contains(result.stdout, fragment) {
			t.Fatalf("%s output lacks %q: %q", label, fragment, result.stdout)
		}
	}
}

func TestExecutionViewsNameProvenanceExplicitly(t *testing.T) {
	setupTaskCommandTest(t)
	t.Setenv("TMUX_PANE", "%7")
	begun := runCommand(t, []string{"task", "begin", "prov-task", "--title", "Provenance", "--execution-id", "prov-1", "--command", "agent"})
	// `task begin` records target external as metadata; provenance stays managed.
	requireContains(t, "begin", begun, 0, "provenance: managed", "target: external")
	if strings.Contains(begun.stdout, "provenance: external") {
		t.Fatalf("begin rendered external provenance: %q", begun.stdout)
	}
	requireContains(t, "inspect", runCommand(t, []string{"task", "inspect", "prov-task"}), 0, "provenance: managed")
	requireContains(t, "execution inspect", runCommand(t, []string{"task", "execution", "inspect", "prov-task", "prov-1"}), 0, "provenance: managed")
	requireContains(t, "execution list", runCommand(t, []string{"task", "execution", "list", "prov-task"}), 0, "managed")
	requireContains(t, "json inspect", runCommand(t, []string{"task", "inspect", "prov-task", "--format", "json"}), 0, `"provenance":"managed"`)
	requireContains(t, "human inspect", runCommand(t, []string{"task", "inspect", "prov-task", "--format", "human"}), 0, "provenance: managed")

	mustRun(t, []string{"task", "external", "create", "ext-task", "--title", "External", "--caller-id", "owner", "--operation-id", "create-ext"},
		[]string{"task", "external", "resource", "create", "ext-task", "--resource-id", "src", "--repository", "demo", "--branch", "agent/ext", "--base", strings.Repeat("a", 40), "--head", strings.Repeat("b", 40), "--worktree", "/abs/ext", "--caller-id", "owner", "--operation-id", "create-src"})
	external := runCommand(t, []string{"task", "external", "execution", "create", "ext-task", "--execution-id", "ext-1", "--resource", "src", "--caller-id", "owner", "--operation-id", "create-exec"})
	requireContains(t, "external execution", external, 0, "provenance: external")
}

func TestHandoffRefusalNamesEveryFailedPreconditionAndRecovery(t *testing.T) {
	setupTaskCommandTest(t)
	t.Setenv("TMUX_PANE", "%2")
	mustRun(t, []string{"task", "begin", "old-orch", "--title", "Old primary", "--execution-id", "old-orch-01", "--command", "pi"})
	t.Setenv("TMUX_PANE", "%31")
	mustRun(t, []string{"task", "begin", "new-orch", "--title", "New primary", "--execution-id", "new-orch-01", "--command", "claude"})

	// The observed incident: successor began as a child on another task and
	// the predecessor never published handed off.
	handoff := func(task, predecessor, successor, operation, revision string) commandResult {
		return runCommand(t, []string{"task", "execution", "handoff", task, predecessor, "--successor-execution", successor, "--operation-id", operation, "--expected-revision", revision, "--takeover-verified", "--predecessor-closed"})
	}
	refused := handoff("old-orch", "old-orch-01", "new-orch-01", "handoff-1", "3")
	requireContains(t, "cross-task handoff", refused, 1,
		"category: conflict",
		"Handoff of execution old-orch-01 refused",
		"(1) successor execution new-orch-01 is not an execution of task old-orch (it is recorded on task new-orch)",
		"(2) predecessor execution old-orch-01 is not published as handed off (condition active",
		"(3) predecessor execution old-orch-01 revision conflict (current revision 0, expected 3)",
		"akagent task begin old-orch --title <exact task title>",
		"akagent task execution publish old-orch old-orch-01 --condition waiting",
		"close it with an operator-directed `akagent task execution finish`",
		"--expected-revision 0",
	)
	// The superseded repair also needs a same-task successor, so it is not
	// offered for a successor on another task.
	if strings.Contains(refused.stdout, "external") || strings.Contains(refused.stdout, "superseded-execution") {
		t.Fatalf("managed cross-task refusal mentions external or an unusable repair: %q", refused.stdout)
	}
	requireContains(t, "unknown successor", handoff("old-orch", "old-orch-01", "nobody", "handoff-2", "0"), 1, "no execution with that ID is recorded")

	// One remaining failure is reported without numbering.
	mustRun(t, []string{"task", "execution", "publish", "old-orch", "old-orch-01", "--condition", "waiting", "--activity", "handed off"})
	single := handoff("old-orch", "old-orch-01", "new-orch-01", "handoff-3", "0")
	requireContains(t, "single failure", single, 1, "refused: successor execution new-orch-01 is not an execution of task old-orch")
	if strings.Contains(single.stdout, "(1)") {
		t.Fatalf("single failure numbered: %q", single.stdout)
	}

	// An inactive same-task successor names its state.
	mustRun(t, []string{"task", "execution", "create", "old-orch", "--execution-id", "old-orch-02", "--target", "external"})
	requireContains(t, "inactive successor", handoff("old-orch", "old-orch-01", "old-orch-02", "handoff-4", "0"), 1, "successor execution old-orch-02 is not an active managed execution (condition none)")
	mustRun(t, []string{"task", "execution", "publish", "old-orch", "old-orch-01", "--condition", "active"})
	requireContains(t, "same-task unpublished predecessor", handoff("old-orch", "old-orch-01", "old-orch-02", "handoff-4b", "0"), 1,
		"task repair superseded-execution --task-id old-orch --execution-id old-orch-01 --successor-id old-orch-02")
	mustRun(t, []string{"task", "execution", "publish", "old-orch", "old-orch-01", "--condition", "waiting", "--activity", "handed off"})

	mustRun(t, []string{"task", "execution", "publish", "old-orch", "old-orch-02", "--condition", "active"})
	requireContains(t, "accepted handoff", handoff("old-orch", "old-orch-01", "old-orch-02", "handoff-5", "0"), 0, "result: handed_off", "provenance: managed")
	requireContains(t, "repeat with new operation", handoff("old-orch", "old-orch-01", "old-orch-02", "handoff-6", "1"), 1,
		"predecessor execution old-orch-01 is already closed (handed off to successor old-orch-02)", "No handoff is needed")

	mustRun(t, []string{"task", "finish", "new-orch", "succeeded", "done"})
	mustRun(t, []string{"task", "execution", "create", "new-orch", "--execution-id", "new-orch-02", "--target", "tool"})
	requireContains(t, "terminal task", handoff("new-orch", "new-orch-01", "new-orch-02", "handoff-7", "0"), 1, "task new-orch is terminal (finished, result done)")
}

func TestDeliverRefusesClosedAttemptWithWorkingRecovery(t *testing.T) {
	setupTaskCommandTest(t)
	t.Setenv("TMUX_PANE", "%2")
	mustRun(t, []string{"task", "begin", "yielded", "--title", "Yielded primary", "--execution-id", "yielded-01", "--command", "pi"},
		[]string{"task", "execution", "finish", "yielded", "yielded-01", "--caller-id", "orchestrator", "--operation-id", "operator-close", "--expected-revision", "0", "--contract", "operator-directed-primary-replacement", "--result", "handed_off"})

	deliver := []string{"task", "deliver", "yielded", "yielded-01", "--contract", "operator-direction", "--result", "succeeded", "--summary", "replaced", "--verified"}
	refused := runCommand(t, deliver)
	requireContains(t, "deliver closed attempt", refused, 1,
		"Execution yielded-01 is already closed (finished with contract operator-directed-primary-replacement, result handed_off)",
		"akagent task finish yielded <succeeded|failed> <summary>",
		"akagent task archive yielded",
	)
	if strings.Contains(refused.stdout, "reopen") || strings.Contains(refused.stdout, "external") {
		t.Fatalf("deliver refusal names a nonexistent operation or wrong provenance: %q", refused.stdout)
	}
	requireContains(t, "task unchanged", runCommand(t, []string{"task", "inspect", "yielded"}), 0, "status: active")

	// The named recovery works.
	mustRun(t, []string{"task", "finish", "yielded", "succeeded", "replaced"}, []string{"task", "archive", "yielded"})
	requireContains(t, "recovered", runCommand(t, []string{"task", "inspect", "yielded"}), 0, "archive_state: complete")

	mustRun(t, []string{"task", "begin", "busy", "--title", "Busy", "--execution-id", "busy-01", "--command", "pi"},
		[]string{"task", "execution", "create", "busy", "--execution-id", "busy-02", "--target", "tool"})
	requireContains(t, "other open attempt", runCommand(t, []string{"task", "deliver", "busy", "busy-01", "--contract", "c", "--result", "succeeded", "--summary", "s", "--verified"}), 1,
		"Another execution busy-02 of task busy is still open", "akagent task execution finish busy busy-02", "--expected-revision 0")
	requireContains(t, "unknown attempt", runCommand(t, []string{"task", "deliver", "yielded", "missing", "--contract", "c", "--result", "succeeded", "--summary", "replaced", "--verified"}), 1,
		"Execution missing is not an execution of task yielded", "akagent task execution list yielded")
}

func TestTerminalMutationConflictNamesNoReopenOperation(t *testing.T) {
	setupTaskCommandTest(t)
	mustRun(t, []string{"task", "create", "--task-id", "closed-task", "--title", "Closed"},
		[]string{"task", "execution", "create", "closed-task", "--execution-id", "closed-1", "--target", "external"},
		[]string{"task", "execution", "finish", "closed-task", "closed-1", "--caller-id", "agent", "--operation-id", "first", "--expected-revision", "0", "--contract", "one", "--result", "succeeded"})
	changed := runCommand(t, []string{"task", "execution", "finish", "closed-task", "closed-1", "--caller-id", "agent", "--operation-id", "second", "--expected-revision", "1", "--contract", "two", "--result", "failed"})
	requireContains(t, "terminal mutation", changed, 1, "execution closed-1 is terminal and immutable", "never reopened")
	if strings.Contains(changed.stdout, "external execution") || strings.Contains(changed.stdout, "future reopen") {
		t.Fatalf("terminal conflict misnames provenance or recovery: %q", changed.stdout)
	}
}
