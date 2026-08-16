package app

import (
	"io"

	"github.com/akofink/akagent-cli/internal/integration"
	"github.com/akofink/akagent-cli/internal/lifecycle"
	"github.com/akofink/akagent-cli/internal/store"
)

type integrationInspectView struct {
	Integration integration.Status `json:"integration"`
}

type workflowLaunchView struct {
	Workflow workflowLaunchStatus `json:"workflow"`
}

type workflowLaunchStatus struct {
	Enabled     bool   `json:"enabled"`
	Skipped     bool   `json:"skipped"`
	TaskID      string `json:"task_id,omitempty"`
	ExecutionID string `json:"execution_id,omitempty"`
	Target      string `json:"target,omitempty"`
	Status      string `json:"status,omitempty"`
}

func integrationCommand(args []string, stdout io.Writer) int {
	if len(args) == 1 && args[0] == "inspect" {
		return write(stdout, integrationInspectView{Integration: integration.Inspect()})
	}
	if len(args) > 0 && args[0] == "launch" {
		if !integration.Enabled() {
			return write(stdout, workflowLaunchView{Workflow: workflowLaunchStatus{Enabled: false, Skipped: true}})
		}
		return workflowLaunchCommand(args[1:], stdout)
	}
	return writeError(stdout, "usage", "Usage: akagent integration <inspect|launch>", false, "Run `akagent integration inspect` or `akagent integration launch`")
}

func workflowLaunchCommand(args []string, stdout io.Writer) int {
	request, ok := parseWorkflowLaunch(args)
	if !ok {
		return writeError(stdout, "usage", "Usage: akagent integration launch <task-id> --execution-id <id> --command <path> [--arg <value>] [--resource <resource-id>] [--label <descriptive-label>]", false, "Use a stable execution ID and a non-secret local command")
	}
	state, err := store.Open()
	if err != nil {
		return lifecycleError(stdout, err)
	}
	execution, ran, err := integration.LaunchWorkflow(lifecycle.New(state), request.TaskID, integration.WorkflowLaunchRequest{
		ExecutionID: request.ExecutionID,
		Label:       request.Label,
		ResourceID:  request.ResourceID,
		Command:     request.Command,
		Arguments:   request.Arguments,
	})
	if err != nil {
		return lifecycleError(stdout, err)
	}
	if !ran {
		return write(stdout, workflowLaunchView{Workflow: workflowLaunchStatus{Enabled: false, Skipped: true}})
	}
	return write(stdout, workflowLaunchView{Workflow: workflowLaunchStatus{
		Enabled:     true,
		TaskID:      request.TaskID,
		ExecutionID: execution.ID,
		Target:      execution.Target,
		Status:      execution.Lifecycle,
	}})
}

type workflowLaunchRequest struct {
	TaskID      string
	ExecutionID string
	Label       string
	ResourceID  string
	Command     string
	Arguments   []string
}

func parseWorkflowLaunch(args []string) (workflowLaunchRequest, bool) {
	if len(args) == 0 {
		return workflowLaunchRequest{}, false
	}
	request := workflowLaunchRequest{TaskID: args[0]}
	for index := 1; index < len(args); index++ {
		if index+1 >= len(args) {
			return workflowLaunchRequest{}, false
		}
		value := args[index+1]
		switch args[index] {
		case "--execution-id":
			request.ExecutionID = value
		case "--command":
			request.Command = value
		case "--arg":
			request.Arguments = append(request.Arguments, value)
		case "--resource":
			request.ResourceID = value
		case "--label":
			request.Label = value
		default:
			return workflowLaunchRequest{}, false
		}
		index++
	}
	return request, request.TaskID != "" && request.ExecutionID != "" && request.Command != ""
}
