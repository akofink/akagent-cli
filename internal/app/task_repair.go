package app

import (
	"io"
	"sort"
	"strconv"

	"github.com/akofink/akagent-cli/internal/derived"
	"github.com/akofink/akagent-cli/internal/lifecycle"
	"github.com/akofink/akagent-cli/internal/store"
)

const repairUsage = "Usage: akagent task repair terminal-executions [--task-id <id>] [--execution-id <id>] [--apply --expected-revision <revision> --contract <name> --verified-ended] | superseded-execution --task-id <id> --execution-id <id> --successor-id <id> [--apply --expected-revision <revision> --contract <name> --verified-ended]"

type repairItem struct {
	TaskID      string `json:"task_id"`
	ExecutionID string `json:"execution_id"`
	Revision    uint64 `json:"revision"`
	Action      string `json:"action"`
}

type repairView struct {
	Mode       string       `json:"mode"`
	Candidates []repairItem `json:"candidates"`
	Total      int          `json:"total"`
}

func taskRepair(args []string, stdout io.Writer, state *store.Store, manager *lifecycle.Manager) int {
	if len(args) > 0 && args[0] == "superseded-execution" {
		return repairSuperseded(args[1:], stdout, manager)
	}
	if len(args) < 1 || args[0] != "terminal-executions" {
		return writeError(stdout, "usage", repairUsage, false, "Preview candidates before applying one independently verified closure")
	}
	values, ok := shortcutOptions(args[1:], map[string]bool{"--task-id": true, "--execution-id": true, "--apply": true, "--expected-revision": true, "--contract": true, "--verified-ended": true})
	if !ok || values["--execution-id"] != "" && values["--task-id"] == "" {
		return writeError(stdout, "usage", repairUsage, false, "Select a task before an execution")
	}
	apply := values["--apply"] == "true"
	if apply && (values["--task-id"] == "" || values["--execution-id"] == "" || values["--contract"] == "" || values["--verified-ended"] != "true" || values["--expected-revision"] == "") || !apply && (values["--contract"] != "" || values["--verified-ended"] != "" || values["--expected-revision"] != "") {
		return writeError(stdout, "usage", repairUsage, false, "Apply only one verified attempt with its displayed revision and a named closure contract")
	}
	var expected uint64
	if apply {
		var err error
		expected, err = strconv.ParseUint(values["--expected-revision"], 10, 64)
		if err != nil {
			return writeError(stdout, "usage", repairUsage, false, "Provide the candidate's current revision")
		}
	}
	ids := []string{values["--task-id"]}
	if ids[0] == "" {
		var err error
		ids, err = state.TaskIDs()
		if err != nil {
			return lifecycleError(stdout, err)
		}
	}
	view := repairView{Mode: "dry_run", Candidates: []repairItem{}}
	for _, id := range ids {
		manifest, err := manager.Inspect(id)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		if !derived.TaskTerminal(manifest) {
			continue
		}
		executions, err := manager.ListExecutions(id)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		for _, execution := range executions {
			if execution.Provenance == store.ProvenanceExternal || derived.ExecutionClosed(execution) || values["--execution-id"] != "" && execution.ID != values["--execution-id"] {
				continue
			}
			view.Candidates = append(view.Candidates, repairItem{TaskID: id, ExecutionID: execution.ID, Revision: execution.Revision, Action: "Verify this attempt ended independently; close without claiming delivery success"})
		}
	}
	sort.Slice(view.Candidates, func(i, j int) bool {
		if view.Candidates[i].TaskID == view.Candidates[j].TaskID {
			return view.Candidates[i].ExecutionID < view.Candidates[j].ExecutionID
		}
		return view.Candidates[i].TaskID < view.Candidates[j].TaskID
	})
	view.Total = len(view.Candidates)
	if !apply {
		return write(stdout, view)
	}
	if view.Total != 1 || view.Candidates[0].Revision != expected {
		return writeError(stdout, "conflict", "Candidate changed or is not eligible", false, "Preview again and verify the exact attempt before retrying")
	}
	item := view.Candidates[0]
	if _, err := manager.CompleteExternalExecution(item.TaskID, item.ExecutionID, "agent", "repair-terminal", values["--contract"], "unverified", expected); err != nil {
		return lifecycleError(stdout, err)
	}
	view.Mode = "applied"
	return write(stdout, view)
}

// repairSuperseded closes only one independently verified ended attempt. The live
// successor is a guard, not evidence that the predecessor ended.
func repairSuperseded(args []string, stdout io.Writer, manager *lifecycle.Manager) int {
	values, ok := shortcutOptions(args, map[string]bool{"--task-id": true, "--execution-id": true, "--successor-id": true, "--apply": true, "--expected-revision": true, "--contract": true, "--verified-ended": true})
	apply := values["--apply"] == "true"
	if !ok || values["--task-id"] == "" || values["--execution-id"] == "" || values["--successor-id"] == "" || values["--execution-id"] == values["--successor-id"] ||
		apply && (values["--expected-revision"] == "" || values["--contract"] == "" || values["--verified-ended"] != "true") ||
		!apply && (values["--expected-revision"] != "" || values["--contract"] != "" || values["--verified-ended"] != "") {
		return writeError(stdout, "usage", repairUsage, false, "Name distinct attempts; preview first and apply with revision, contract and verified-ended attestation")
	}
	id := values["--task-id"]
	manifest, err := manager.Inspect(id)
	if err != nil {
		return lifecycleError(stdout, err)
	}
	if derived.TaskTerminal(manifest) {
		return writeError(stdout, "conflict", "Task is terminal", false, "Use terminal-executions for a finished task")
	}
	executions, err := manager.ListExecutions(id)
	if err != nil {
		return lifecycleError(stdout, err)
	}
	var previous, successor *store.Execution
	for i := range executions {
		switch executions[i].ID {
		case values["--execution-id"]:
			previous = &executions[i]
		case values["--successor-id"]:
			successor = &executions[i]
		}
	}
	if previous == nil || successor == nil || derived.ExecutionClosed(*previous) || derived.ExecutionClosed(*successor) {
		return writeError(stdout, "conflict", "Open predecessor and successor required", false, "Inspect both executions before retrying")
	}
	// Read the owning host's live surface. Do not turn an unavailable adapter
	// into a positive successor signal.
	snapshot, err := checkSnapshot(manager, id)
	if err != nil {
		return lifecycleError(stdout, err)
	}
	snapshot.Resources = nil
	report := derived.Check(snapshot, checkRunner, derived.Options{})
	live := false
	for _, f := range report.Executions {
		if f.Surface == "execution:"+previous.ID && f.Code == "live" {
			return writeError(stdout, "conflict", "Predecessor still looks live", false, "Verify its owner and stop it before previewing again")
		}
		if f.Surface == "execution:"+successor.ID && f.Code == "live" {
			live = true
		}
	}
	if !live {
		return writeError(stdout, "conflict", "Successor is not observed live", false, "Check its owning pane on this host")
	}
	view := repairView{Mode: "dry_run", Candidates: []repairItem{{TaskID: id, ExecutionID: previous.ID, Revision: previous.Revision, Action: "Independently verify the previous attempt ended; close without claiming delivery success"}}, Total: 1}
	if !apply {
		return write(stdout, view)
	}
	expected, err := strconv.ParseUint(values["--expected-revision"], 10, 64)
	if err != nil || expected != previous.Revision {
		return writeError(stdout, "conflict", "Candidate revision changed", false, "Preview again and verify the exact attempt")
	}
	if _, err := manager.CompleteExternalExecution(id, previous.ID, "agent", "repair-superseded", values["--contract"], "unverified", expected); err != nil {
		return lifecycleError(stdout, err)
	}
	view.Mode = "applied"
	return write(stdout, view)
}
