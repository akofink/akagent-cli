package app

import (
	"io"
	"sort"
	"strconv"

	"github.com/akofink/akagent-cli/internal/derived"
	"github.com/akofink/akagent-cli/internal/lifecycle"
	"github.com/akofink/akagent-cli/internal/store"
)

const repairUsage = "Usage: akagent task repair terminal-executions [--task-id <id>] [--execution-id <id>] [--apply --expected-revision <revision> --contract <name> --verified-ended]"

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
