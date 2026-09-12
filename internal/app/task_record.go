package app

import (
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/akofink/akagent-cli/internal/lifecycle"
	"github.com/akofink/akagent-cli/internal/store"
)

type recordArchiveView struct {
	TaskID      string    `json:"task_id"`
	ResourceID  string    `json:"resource_id,omitempty"`
	ExecutionID string    `json:"execution_id,omitempty"`
	CapturedAt  time.Time `json:"captured_at"`
	Record      string    `json:"record"`
	State       string    `json:"state"`
	EventCount  int       `json:"event_count"`
}

func taskRecordCommand(args []string, stdout io.Writer) int {
	if len(args) == 0 {
		return recordUsage(stdout)
	}
	state, err := store.Open()
	if err != nil {
		return lifecycleError(stdout, err)
	}
	manager := lifecycle.New(state)
	switch args[0] {
	case "task":
		if len(args) < 2 {
			return writeError(stdout, "usage", "Usage: akagent task record task <task-id> --caller-id <id> --operation-id <id>", false, "Declare a state-only task identity")
		}
		request, ok := parseRecordIdentity(args[2:])
		if !ok {
			return writeError(stdout, "usage", "Usage: akagent task record task <task-id> --caller-id <id> --operation-id <id>", false, "Provide stable caller and operation IDs")
		}
		manifest, err := manager.RecordTask(args[1], request)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, taskDetailView{Task: view(args[1], manifest)})
	case "adopt", "resource", "adopt-resource":
		if len(args) < 2 {
			return recordUsage(stdout)
		}
		request, ok := parseExternalResource(args[2:])
		if !ok {
			return writeError(stdout, "usage", "Usage: akagent task record adopt <task-id> --resource-id <id> --caller-id <id> --operation-id <id> --repository <name> --branch <branch> --base <revision> --head <revision> --worktree <absolute-path> [--expected-revision <n>] [--metadata <key=value>]", false, "Declare repository identity without requiring Git or an existing checkout")
		}
		resource, err := manager.RecordResource(args[1], request)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, resourceDetailView{Resource: viewResource(resource)})
	case "execution", "attempt":
		if len(args) < 2 {
			return recordUsage(stdout)
		}
		request, ok := parseExternalExecution(args[2:])
		if !ok {
			return writeError(stdout, "usage", "Usage: akagent task record execution <task-id> --execution-id <id> --resource-id <id> --caller-id <id> --operation-id <id> [--predecessor <execution-id>] [--tool <tool> --session-id <id>]", false, "Record an external execution attempt without starting a process")
		}
		execution, err := manager.RecordExecution(args[1], request)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, executionDetail(execution, manager))
	case "observe":
		if len(args) < 3 {
			return recordUsage(stdout)
		}
		callerID, operationID, expected, observation, ok := parseExternalObservation(args[3:])
		if !ok {
			return writeError(stdout, "usage", "Usage: akagent task record observe <task-id> <execution-id> --caller-id <id> --operation-id <id> --expected-revision <n> --source <source> --observed-at <RFC3339> --host-id <id> --boot-id <id> [--process-state <state>] [--result <result>] [--detail <text>]", false, "Record provenance and caller-submitted observations")
		}
		execution, err := manager.RecordObservation(args[1], args[2], callerID, operationID, expected, observation)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, executionDetail(execution, manager))
	case "complete":
		return taskRecordComplete(manager, args[1:], stdout)
	case "archive":
		return taskRecordArchive(manager, args[1:], stdout)
	default:
		return recordUsage(stdout)
	}
}

func taskRecordComplete(manager *lifecycle.Manager, args []string, stdout io.Writer) int {
	if len(args) == 0 {
		return recordUsage(stdout)
	}
	taskID := args[0]
	executionID, callerID, operationID, contract, resultValue, ok := "", "", "", "", "", true
	var expected uint64
	for index := 1; index < len(args); index++ {
		if index+1 >= len(args) {
			ok = false
			break
		}
		flag, value := args[index], args[index+1]
		index++
		switch flag {
		case "--execution-id", "--execution":
			executionID = value
		case "--caller-id":
			callerID = value
		case "--operation-id":
			operationID = value
		case "--contract":
			contract = value
		case "--result":
			resultValue = value
		case "--expected-revision":
			expected, ok = parseRevision(value)
		default:
			ok = false
		}
	}
	if !ok || callerID == "" || operationID == "" || contract == "" || resultValue == "" {
		return writeError(stdout, "usage", "Usage: akagent task record complete <task-id> [--execution-id <id>] --caller-id <id> --operation-id <id> --expected-revision <n> --contract <name> --result <result>", false, "Declare completion explicitly against an external contract")
	}
	if executionID != "" {
		execution, err := manager.CompleteExternalExecution(taskID, executionID, callerID, operationID, contract, resultValue, expected)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, executionDetail(execution, manager))
	}
	manifest, err := manager.CompleteExternalTask(taskID, callerID, operationID, contract, resultValue, expected)
	if err != nil {
		return lifecycleError(stdout, err)
	}
	return write(stdout, taskDetailView{Task: view(taskID, manifest)})
}

func taskRecordArchive(manager *lifecycle.Manager, args []string, stdout io.Writer) int {
	if len(args) == 0 {
		return recordUsage(stdout)
	}
	taskID, resourceID, executionID, callerID, operationID := args[0], "", "", "", ""
	var expected uint64
	ok := true
	for index := 1; index < len(args); index++ {
		if index+1 >= len(args) {
			ok = false
			break
		}
		flag, value := args[index], args[index+1]
		index++
		switch flag {
		case "--resource-id", "--resource":
			resourceID = value
		case "--execution-id", "--execution":
			executionID = value
		case "--caller-id":
			callerID = value
		case "--operation-id":
			operationID = value
		case "--expected-revision":
			expected, ok = parseRevision(value)
		default:
			ok = false
		}
	}
	if !ok || callerID == "" || operationID == "" || (resourceID != "" && executionID != "") {
		return writeError(stdout, "usage", "Usage: akagent task record archive <task-id> [--resource-id <id>|--execution-id <id>] --caller-id <id> --operation-id <id> --expected-revision <n>", false, "Archive only durable state-only records")
	}
	archive, err := manager.ArchiveExternal(taskID, resourceID, executionID, callerID, operationID, expected)
	if err != nil {
		return lifecycleError(stdout, err)
	}
	view := recordArchiveView{State: "complete"}
	switch value := archive.(type) {
	case store.TaskArchive:
		view.TaskID, view.CapturedAt, view.Record, view.EventCount = value.TaskID, value.CapturedAt, "task", len(value.Events)
	case store.ResourceArchive:
		view.TaskID, view.ResourceID, view.CapturedAt, view.Record, view.EventCount = value.TaskID, value.ResourceID, value.CapturedAt, "resource", len(value.Events)
	case store.ExecutionArchive:
		view.TaskID, view.ExecutionID, view.CapturedAt, view.Record, view.EventCount = value.TaskID, value.ExecutionID, value.CapturedAt, "execution", len(value.Events)
	}
	return write(stdout, view)
}

func parseRecordIdentity(args []string) (store.ExternalTaskRequest, bool) {
	request := store.ExternalTaskRequest{}
	for index := 0; index < len(args); index++ {
		if index+1 >= len(args) {
			return request, false
		}
		flag, value := args[index], args[index+1]
		index++
		switch flag {
		case "--caller-id":
			request.CallerID = value
		case "--operation-id":
			request.OperationID = value
		default:
			return request, false
		}
	}
	return request, request.CallerID != "" && request.OperationID != ""
}

func parseExternalResource(args []string) (store.ExternalResourceRequest, bool) {
	request := store.ExternalResourceRequest{}
	var metadata map[string]string
	for index := 0; index < len(args); index++ {
		if index+1 >= len(args) {
			return request, false
		}
		flag, value := args[index], args[index+1]
		index++
		switch flag {
		case "--resource-id", "--id":
			request.ID = value
		case "--caller-id":
			request.CallerID = value
		case "--operation-id":
			request.OperationID = value
		case "--repository":
			request.Repository = value
		case "--branch":
			request.Branch = value
		case "--base":
			request.BaseRevision = value
		case "--head":
			request.Head = value
		case "--worktree":
			request.WorktreePath = value
		case "--expected-revision":
			var ok bool
			request.ExpectedRevision, ok = parseRevision(value)
			if !ok {
				return request, false
			}
		case "--metadata":
			key, metadataValue, found := strings.Cut(value, "=")
			if !found || key == "" || metadataValue == "" {
				return request, false
			}
			if metadata == nil {
				metadata = map[string]string{}
			}
			metadata[key] = metadataValue
		default:
			return request, false
		}
	}
	request.Metadata = metadata
	return request, request.ID != "" && request.CallerID != "" && request.OperationID != "" && request.Repository != "" && request.Branch != "" && request.BaseRevision != "" && request.Head != "" && request.WorktreePath != ""
}

func parseExternalExecution(args []string) (store.ExternalExecutionRequest, bool) {
	request := store.ExternalExecutionRequest{}
	pendingTool := ""
	for index := 0; index < len(args); index++ {
		if index+1 >= len(args) {
			return request, false
		}
		flag, value := args[index], args[index+1]
		index++
		switch flag {
		case "--execution-id", "--id":
			request.ID = value
		case "--resource-id", "--resource":
			request.ResourceID = value
		case "--caller-id":
			request.CallerID = value
		case "--operation-id":
			request.OperationID = value
		case "--predecessor":
			request.PredecessorID = value
		case "--tool":
			pendingTool = value
		case "--session-id":
			if pendingTool == "" {
				return request, false
			}
			request.SessionReferences = append(request.SessionReferences, store.SessionReference{Tool: pendingTool, SessionID: value})
			pendingTool = ""
		default:
			return request, false
		}
	}
	return request, request.ID != "" && request.ResourceID != "" && request.CallerID != "" && request.OperationID != "" && pendingTool == ""
}

func parseExternalObservation(args []string) (string, string, uint64, store.ExternalObservation, bool) {
	var callerID, operationID string
	var expected uint64
	observation := store.ExternalObservation{}
	ok := true
	for index := 0; index < len(args); index++ {
		if index+1 >= len(args) {
			return "", "", 0, observation, false
		}
		flag, value := args[index], args[index+1]
		index++
		switch flag {
		case "--caller-id":
			callerID = value
		case "--operation-id":
			operationID = value
		case "--expected-revision":
			expected, ok = parseRevision(value)
		case "--source":
			observation.Source = value
		case "--observed-at":
			observation.ObservedAt, ok = parseObservedAt(value)
		case "--host-id":
			observation.HostID = value
		case "--boot-id":
			observation.BootID = value
		case "--process-state":
			observation.ProcessState = value
		case "--result":
			observation.Result = value
		case "--detail":
			observation.Detail = value
		default:
			ok = false
		}
		if !ok {
			return "", "", 0, observation, false
		}
	}
	return callerID, operationID, expected, observation, ok && callerID != "" && operationID != "" && expected > 0 && observation.Source != "" && !observation.ObservedAt.IsZero() && observation.HostID != "" && observation.BootID != ""
}

func parseRevision(value string) (uint64, bool) {
	revision, err := strconv.ParseUint(value, 10, 64)
	return revision, err == nil
}

func parseObservedAt(value string) (time.Time, bool) {
	observedAt, err := time.Parse(time.RFC3339Nano, value)
	return observedAt.UTC(), err == nil
}

func recordUsage(stdout io.Writer) int {
	return writeError(stdout, "usage", "Usage: akagent task record <task|adopt|execution|observe|complete|archive> ...", false, "Use the state-only record surface; it never invokes host tools")
}
