package app

import (
	"io"
	"os"

	"github.com/akofink/akagent-cli/internal/derived"
	"github.com/akofink/akagent-cli/internal/lifecycle"
	"github.com/akofink/akagent-cli/internal/store"
)

const beginUsage = "Usage: akagent task begin <task-id> --title <title> --execution-id <id> --command <label> [--host <hostname>] [--tmux-pane <pane>]"
const deliverUsage = "Usage: akagent task deliver <task-id> <execution-id> --contract <name> --result <succeeded|failed> --summary <text> --verified"

func shortcutOptions(args []string, allowed map[string]bool) (map[string]string, bool) {
	values := map[string]string{}
	for len(args) > 0 {
		flag := args[0]
		if !allowed[flag] || values[flag] != "" {
			return nil, false
		}
		if flag == "--verified" || flag == "--apply" || flag == "--verified-ended" {
			values[flag] = "true"
			args = args[1:]
			continue
		}
		if len(args) < 2 || args[1] == "" {
			return nil, false
		}
		values[flag] = args[1]
		args = args[2:]
	}
	return values, true
}

func taskBegin(args []string, stdout io.Writer, manager *lifecycle.Manager) int {
	if len(args) < 2 {
		return writeError(stdout, "usage", beginUsage, false, "Provide stable task and execution identities")
	}
	values, ok := shortcutOptions(args[1:], map[string]bool{"--title": true, "--execution-id": true, "--command": true, "--host": true, "--tmux-pane": true})
	if !ok || values["--title"] == "" || values["--execution-id"] == "" || values["--command"] == "" {
		return writeError(stdout, "usage", beginUsage, false, "Provide title, execution ID, and a non-secret command label")
	}
	host, pane := values["--host"], values["--tmux-pane"]
	if host == "" {
		host, _ = os.Hostname()
	}
	if pane == "" {
		pane = os.Getenv("TMUX_PANE")
	}
	if !validExecutionBinding(host, pane) {
		return writeError(stdout, "usage", beginUsage, false, "Provide a valid hostname and tmux pane ID")
	}
	id := args[0]
	manifest, err := manager.CreateRecord(lifecycle.CreateRequest{ID: id, Title: values["--title"]})
	if err != nil {
		return lifecycleError(stdout, err)
	}
	if derived.TaskTerminal(manifest.Manifest) {
		return writeError(stdout, "conflict", "Cannot begin a terminal task", false, "Inspect the task and create a new task for new work")
	}
	execution, _, err := manager.CreateExecution(id, lifecycle.ExecutionRequest{ID: values["--execution-id"], Target: "external", Command: values["--command"], Host: host, TmuxPane: pane})
	if err != nil {
		return lifecycleError(stdout, err)
	}
	if derived.ExecutionClosed(execution) {
		return writeError(stdout, "conflict", "Cannot begin a closed execution", false, "Create a successor execution for new work")
	}
	if manifest.Manifest.Condition != "active" {
		if _, err := manager.PublishRecord(id, "active", "", ""); err != nil {
			return lifecycleError(stdout, err)
		}
	}
	if execution.Condition != "active" {
		if _, err := manager.PublishExecutionRecord(id, execution.ID, "active", "", ""); err != nil {
			return lifecycleError(stdout, err)
		}
	}
	current, err := manager.Inspect(id)
	if err != nil {
		return lifecycleError(stdout, err)
	}
	detail, err := taskDetail(manager, id, current)
	if err != nil {
		return lifecycleError(stdout, err)
	}
	return write(stdout, detail)
}

func taskDeliver(args []string, stdout io.Writer, manager *lifecycle.Manager) int {
	if len(args) < 3 {
		return writeError(stdout, "usage", deliverUsage, false, "Verify delivery independently before declaring completion")
	}
	values, ok := shortcutOptions(args[2:], map[string]bool{"--contract": true, "--result": true, "--summary": true, "--verified": true})
	if !ok || values["--contract"] == "" || values["--summary"] == "" || values["--verified"] != "true" || values["--result"] != "succeeded" && values["--result"] != "failed" {
		return writeError(stdout, "usage", deliverUsage, false, "Name the verified delivery contract and result; missing panes are not verification")
	}
	id, executionID := args[0], args[1]
	manifest, err := manager.Inspect(id)
	if err != nil {
		return lifecycleError(stdout, err)
	}
	if manifest.Provenance == store.ProvenanceExternal || manifest.Lifecycle == "stopped" || manifest.ExternalCompletion != nil {
		return writeError(stdout, "conflict", "Task is not a managed delivery candidate", false, "Inspect the task and use its owning completion command")
	}
	executions, err := manager.ListExecutions(id)
	if err != nil {
		return lifecycleError(stdout, err)
	}
	var selected *store.Execution
	for i := range executions {
		if executions[i].ID == executionID {
			selected = &executions[i]
		} else if !derived.ExecutionClosed(executions[i]) {
			return writeError(stdout, "conflict", "Another execution is still open", false, "Verify and close each execution before delivery")
		}
	}
	if selected == nil || selected.Provenance == store.ProvenanceExternal {
		return writeError(stdout, "conflict", "Managed execution not found", false, "Inspect the owning task and execution")
	}
	if manifest.Lifecycle == "finished" && (manifest.Result != values["--summary"] || manifest.Condition != values["--result"] && manifest.Condition != "none") {
		return writeError(stdout, "conflict", "Task result differs from delivery", false, "Inspect the existing completion")
	}
	if _, err := manager.CompleteExternalExecution(id, executionID, "agent", "deliver", values["--contract"], values["--result"], selected.Revision); err != nil {
		return lifecycleError(stdout, err)
	}
	if _, err := manager.FinishRecord(id, values["--result"], values["--summary"]); err != nil {
		return lifecycleError(stdout, err)
	}
	if _, err := manager.ArchiveRecord(id, "", ""); err != nil {
		return lifecycleError(stdout, err)
	}
	current, err := manager.Inspect(id)
	if err != nil {
		return lifecycleError(stdout, err)
	}
	detail, err := taskDetail(manager, id, current)
	if err != nil {
		return lifecycleError(stdout, err)
	}
	return write(stdout, detail)
}
