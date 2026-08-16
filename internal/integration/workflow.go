package integration

import (
	"strings"

	"github.com/akofink/akagent-cli/internal/lifecycle"
	"github.com/akofink/akagent-cli/internal/store"
)

const WorkflowTarget = "workflow"

// WorkflowLaunchRequest describes a provider-neutral automated command.
// The command is recorded as an execution before it is launched so a failed
// start remains inspectable and retryable through the regular task commands.
type WorkflowLaunchRequest struct {
	ExecutionID string
	Label       string
	ResourceID  string
	Command     string
	Arguments   []string
}

// LaunchWorkflow starts a local workflow command through the generic
// execution lifecycle. It does not interpret the command or its arguments.
// The integration gate is checked before any lifecycle mutation.
func LaunchWorkflow(manager *lifecycle.Manager, taskID string, request WorkflowLaunchRequest) (store.Execution, bool, error) {
	if !Enabled() {
		return store.Execution{}, false, nil
	}
	if strings.TrimSpace(request.ExecutionID) == "" {
		return store.Execution{}, true, &store.Error{Kind: store.KindUsage, Message: "workflow execution ID is required", Recovery: "Retry with `--execution-id <stable-id>` so the integration can be retried safely"}
	}
	if strings.TrimSpace(request.Command) == "" || strings.ContainsAny(request.Command, "\r\n\x00") {
		return store.Execution{}, true, &store.Error{Kind: store.KindUsage, Message: "workflow command must be a non-empty single line", Recovery: "Retry with a non-secret executable path"}
	}
	for _, argument := range request.Arguments {
		if strings.ContainsAny(argument, "\r\n\x00") {
			return store.Execution{}, true, &store.Error{Kind: store.KindUsage, Message: "workflow arguments must be single-line values", Recovery: "Retry without multiline or NUL-containing arguments"}
		}
	}
	resourceID, err := selectResource(manager, taskID, request.ResourceID)
	if err != nil {
		return store.Execution{}, true, err
	}
	label, err := manager.ResolveCompatibilityExecutionLabel(taskID, resourceID, request.Label)
	if err != nil {
		return store.Execution{}, true, err
	}
	execution, _, err := manager.CreateExecution(taskID, lifecycle.ExecutionRequest{
		ID:         request.ExecutionID,
		Label:      label,
		Target:     WorkflowTarget,
		Command:    request.Command,
		Arguments:  append([]string(nil), request.Arguments...),
		ResourceID: resourceID,
	})
	if err != nil {
		return store.Execution{}, true, err
	}
	launched, err := manager.LaunchExecutionRecord(taskID, execution.ID)
	if err != nil {
		return launched, true, &store.Error{
			Kind:      store.KindPartial,
			Message:   "workflow execution launch failed",
			Retryable: true,
			Recovery:  "Inspect the execution, run `akagent task execution reconcile`, and retry the same execution ID",
			Err:       err,
		}
	}
	return launched, true, nil
}
