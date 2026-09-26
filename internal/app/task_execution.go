package app

import (
	"io"
	"path/filepath"
	"strconv"
	"time"

	"github.com/akofink/akagent-cli/internal/lifecycle"
	"github.com/akofink/akagent-cli/internal/store"
)

func taskExecutionCommand(args []string, stdout io.Writer) int {
	if len(args) == 0 {
		return writeError(stdout, "usage", "Usage: akagent task execution <create|observe|finish|handoff|list|inspect|session|evidence|publish|archive|reconcile>", false, "Run `akagent task execution list <task-id>`")
	}
	if args[0] == "launch" || args[0] == "attach" || args[0] == "stop" {
		return removedCommandError(stdout, "task execution "+args[0], "Use the normal execution record commands; external tools own process and terminal side effects")
	}
	state, err := store.Open()
	if err != nil {
		return lifecycleError(stdout, err)
	}
	manager := lifecycle.New(state)
	switch args[0] {
	case "observe":
		if len(args) < 4 {
			return writeError(stdout, "usage", "Usage: akagent task execution observe <task-id> <execution-id> --caller-id <id> --operation-id <id> --expected-revision <revision> --source <source> --observed-at <RFC3339> --host-id <id> --boot-id <id> [--process-state <state>] [--result <result>] [--detail <text>]", false, "Record provenance and caller-submitted observations")
		}
		callerID, operationID, expectedRevision, observation, ok := parseExternalObservation(args[3:])
		if !ok {
			return writeError(stdout, "usage", "Usage: akagent task execution observe <task-id> <execution-id> --caller-id <id> --operation-id <id> --expected-revision <revision> --source <source> --observed-at <RFC3339> --host-id <id> --boot-id <id> [--process-state <state>] [--result <result>] [--detail <text>]", false, "Provide complete observation provenance and the current execution revision")
		}
		execution, err := manager.RecordObservation(args[1], args[2], callerID, operationID, expectedRevision, observation)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, executionDetail(execution, manager))
	case "handoff":
		if len(args) < 4 {
			return executionHandoffUsage(stdout, "Provide a task, predecessor execution, successor execution, operation ID, current revision, and takeover confirmations")
		}
		request, verified, predecessorClosed, ok := parseExecutionHandoff(args[3:])
		if !ok {
			return executionHandoffUsage(stdout, "Require --takeover-verified and --predecessor-closed after independent verification")
		}
		if !verified || !predecessorClosed {
			return executionHandoffUsage(stdout, "Confirm independently verified successor takeover and predecessor closure")
		}
		execution, err := manager.DisposeManagedExecutionHandoff(args[1], args[2], request)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, executionDetail(execution, manager))
	case "finish", "complete":
		if len(args) < 4 {
			return writeError(stdout, "usage", "Usage: akagent task execution finish <task-id> <execution-id> --caller-id <id> --operation-id <id> --expected-revision <revision> --contract <name> --result <result>", false, "Declare completion explicitly against a named external contract")
		}
		callerID, operationID, contract, resultValue, expectedRevision, ok := parseExternalCompletion(args[3:])
		if !ok {
			return writeError(stdout, "usage", "Usage: akagent task execution finish <task-id> <execution-id> --caller-id <id> --operation-id <id> --expected-revision <revision> --contract <name> --result <result>", false, "Provide the original caller, operation, contract, result, and current execution revision")
		}
		execution, err := manager.CompleteExternalExecution(args[1], args[2], callerID, operationID, contract, resultValue, expectedRevision)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, executionDetail(execution, manager))
	case "create":
		if len(args) < 2 {
			return writeError(stdout, "usage", "Usage: akagent task execution create <task-id> --target <target> [--execution-id <id>] [--label <label>] [--command <command>] [--require <credential>] [--resource <resource-id>] [--worktree <path>]", false, "Create an execution without starting it")
		}
		request, ok := parseExecutionCreate(args[2:])
		if !ok {
			return writeError(stdout, "usage", "Usage: akagent task execution create <task-id> --target <target> [--execution-id <id>] [--label <label>] [--command <command>] [--require <credential>] [--resource <resource-id>] [--worktree <path>]", false, "Provide a target and immutable execution inputs")
		}
		execution, _, err := manager.CreateExecution(args[1], request)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, executionDetail(execution, manager))
	case "list":
		if len(args) != 2 {
			return writeError(stdout, "usage", "Usage: akagent task execution list <task-id>", false, "List the task executions")
		}
		executions, err := manager.ListExecutions(args[1])
		if err != nil {
			return lifecycleError(stdout, err)
		}
		items := make([]executionView, 0, len(executions))
		for _, execution := range executions {
			items = append(items, viewExecution(execution, manager))
		}
		return write(stdout, executionListView{Executions: items, Total: len(items)})
	case "inspect":
		if len(args) < 2 || len(args) > 3 {
			return writeError(stdout, "usage", "Usage: akagent task execution inspect <task-id> [<execution-id>]", false, "Inspect the execution")
		}
		id := ""
		if len(args) == 3 {
			id = args[2]
		}
		execution, err := manager.InspectExecution(args[1], id)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, executionDetail(execution, manager))
	case "session":
		if len(args) < 5 || args[1] != "add" {
			return writeError(stdout, "usage", "Usage: akagent task execution session add <task-id> <execution-id> --tool <tool> --session-id <id> [--reference-path <path>]", false, "Record provider-neutral execution session provenance")
		}
		reference, ok := parseSessionReference(args[4:])
		if !ok {
			return writeError(stdout, "usage", "Usage: akagent task execution session add <task-id> <execution-id> --tool <tool> --session-id <id> [--reference-path <path>]", false, "Provide a tool, session ID, and optional local reference path")
		}
		execution, err := manager.AddExecutionSessionReference(args[2], args[3], reference)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, executionDetail(execution, manager))
	case "evidence":
		if len(args) < 4 || len(args) > 5 {
			return writeError(stdout, "usage", "Usage: akagent task execution evidence <list|inspect> <task-id> <execution-id> [<capture-id>]", false, "Inspect read-only metadata for execution session references")
		}
		switch args[1] {
		case "list":
			if len(args) != 4 {
				return writeError(stdout, "usage", "Usage: akagent task execution evidence list <task-id> <execution-id>", false, "List read-only execution evidence")
			}
			summary, captures, err := manager.ListExecutionEvidence(args[2], args[3])
			if err != nil {
				return lifecycleError(stdout, err)
			}
			return write(stdout, executionEvidenceList(summary, captures))
		case "inspect":
			if len(args) != 5 {
				return writeError(stdout, "usage", "Usage: akagent task execution evidence inspect <task-id> <execution-id> <capture-id>", false, "Inspect one read-only execution evidence record")
			}
			capture, err := manager.InspectExecutionEvidence(args[2], args[3], args[4])
			if err != nil {
				return lifecycleError(stdout, err)
			}
			return write(stdout, executionEvidenceDetail(capture))
		default:
			return writeError(stdout, "usage", "Usage: akagent task execution evidence <list|inspect> <task-id> <execution-id> [<capture-id>]", false, "Inspect read-only metadata for execution session references")
		}
	case "publish":
		if len(args) < 4 {
			return writeError(stdout, "usage", "Usage: akagent task execution publish <task-id> <execution-id> --condition <condition> [--reason <reason>] [--activity <activity>]", false, "Publish execution condition and heartbeat")
		}
		condition, reason, activity, ok := parsePublish(args[3:])
		if !ok {
			return taskUsage(stdout)
		}
		execution, err := manager.PublishExecutionRecord(args[1], args[2], condition, reason, activity)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, executionDetail(execution, manager))
	case "archive":
		if len(args) != 3 {
			return writeError(stdout, "usage", "Usage: akagent task execution archive <task-id> <execution-id>", false, "Provide the task and execution IDs")
		}
		archived, err := manager.ArchiveRecord(args[1], "", args[2])
		if err != nil {
			return lifecycleError(stdout, err)
		}
		execution, ok := archived.(store.Execution)
		if !ok {
			return writeError(stdout, "internal", "Execution archive returned an invalid record", false, "Retry the archive")
		}
		return write(stdout, executionDetail(execution, manager))
	case "reconcile":
		if len(args) != 2 {
			return writeError(stdout, "usage", "Usage: akagent task execution reconcile <task-id>", false, "Reconcile executions without changing resources")
		}
		executions, err := manager.ReconcileRecordExecutions(args[1])
		if err != nil {
			return lifecycleError(stdout, err)
		}
		items := make([]executionView, 0, len(executions))
		for _, execution := range executions {
			items = append(items, viewExecution(execution, manager))
		}
		return write(stdout, executionListView{Executions: items, Total: len(items)})
	default:
		return writeError(stdout, "usage", "Usage: akagent task execution <create|observe|finish|handoff|list|inspect|session|evidence|publish|archive|reconcile>", false, "Run `akagent task execution list <task-id>`")
	}
}

func executionHandoffUsage(stdout io.Writer, guidance string) int {
	return writeError(stdout, "usage", "Usage: akagent task execution handoff <task-id> <predecessor-id> --successor-execution <id> --operation-id <id> --expected-revision <revision> --takeover-verified --predecessor-closed", false, guidance)
}

func parseExecutionHandoff(args []string) (request store.HandoffDispositionRequest, verified, predecessorClosed, ok bool) {
	seen := map[string]bool{}
	revisionProvided := false
	for len(args) > 0 {
		flag := args[0]
		args = args[1:]
		if seen[flag] {
			return store.HandoffDispositionRequest{}, false, false, false
		}
		seen[flag] = true
		switch flag {
		case "--takeover-verified":
			verified = true
		case "--predecessor-closed":
			predecessorClosed = true
		case "--successor-execution", "--operation-id", "--expected-revision":
			if len(args) == 0 {
				return store.HandoffDispositionRequest{}, false, false, false
			}
			value := args[0]
			args = args[1:]
			switch flag {
			case "--successor-execution":
				request.SuccessorExecutionID = value
			case "--operation-id":
				request.OperationID = value
			case "--expected-revision":
				var parsed bool
				request.ExpectedRevision, parsed = parseRevision(value)
				revisionProvided = parsed
				if !parsed {
					return store.HandoffDispositionRequest{}, false, false, false
				}
			}
		default:
			return store.HandoffDispositionRequest{}, false, false, false
		}
	}
	return request, verified, predecessorClosed, request.SuccessorExecutionID != "" && request.OperationID != "" && revisionProvided
}

func parseExternalCompletion(args []string) (callerID, operationID, contract, resultValue string, expectedRevision uint64, ok bool) {
	seen := map[string]bool{}
	revisionProvided := false
	ok = true
	for len(args) > 0 {
		if len(args) < 2 {
			return "", "", "", "", 0, false
		}
		flag, value := args[0], args[1]
		args = args[2:]
		if seen[flag] {
			return "", "", "", "", 0, false
		}
		seen[flag] = true
		switch flag {
		case "--caller-id":
			callerID = value
		case "--operation-id":
			operationID = value
		case "--contract":
			contract = value
		case "--result":
			resultValue = value
		case "--expected-revision":
			expectedRevision, ok = parseRevision(value)
			revisionProvided = ok
			if !ok {
				return "", "", "", "", 0, false
			}
		default:
			return "", "", "", "", 0, false
		}
	}
	return callerID, operationID, contract, resultValue, expectedRevision, ok && callerID != "" && operationID != "" && contract != "" && resultValue != "" && revisionProvided
}

func parseExternalObservation(args []string) (string, string, uint64, store.ExternalObservation, bool) {
	var callerID, operationID string
	var expectedRevision uint64
	observation := store.ExternalObservation{}
	seen := map[string]bool{}
	for len(args) > 0 {
		if len(args) < 2 {
			return "", "", 0, observation, false
		}
		flag, value := args[0], args[1]
		args = args[2:]
		if seen[flag] {
			return "", "", 0, observation, false
		}
		seen[flag] = true
		switch flag {
		case "--caller-id":
			callerID = value
		case "--operation-id":
			operationID = value
		case "--expected-revision":
			var ok bool
			expectedRevision, ok = parseRevision(value)
			if !ok {
				return "", "", 0, observation, false
			}
		case "--source":
			observation.Source = value
		case "--observed-at":
			var ok bool
			observation.ObservedAt, ok = parseObservedAt(value)
			if !ok {
				return "", "", 0, observation, false
			}
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
			return "", "", 0, observation, false
		}
	}
	return callerID, operationID, expectedRevision, observation, callerID != "" && operationID != "" && expectedRevision > 0 && observation.Source != "" && !observation.ObservedAt.IsZero() && observation.HostID != "" && observation.BootID != ""
}

func parseRevision(value string) (uint64, bool) {
	revision, err := strconv.ParseUint(value, 10, 64)
	return revision, err == nil
}

func parseObservedAt(value string) (time.Time, bool) {
	observedAt, err := time.Parse(time.RFC3339Nano, value)
	return observedAt.UTC(), err == nil
}

func parseSessionReference(args []string) (store.SessionReference, bool) {
	var reference store.SessionReference
	for len(args) > 0 {
		if len(args) < 2 {
			return reference, false
		}
		flag, value := args[0], args[1]
		args = args[2:]
		switch flag {
		case "--tool":
			reference.Tool = value
		case "--session-id", "--session":
			reference.SessionID = value
		case "--reference-path", "--path":
			absolute, err := filepath.Abs(value)
			if err != nil {
				return reference, false
			}
			reference.ReferencePath = absolute
		default:
			return reference, false
		}
	}
	return reference, reference.Tool != "" && reference.SessionID != ""
}

func parseExecutionCreate(args []string) (lifecycle.ExecutionRequest, bool) {
	var request lifecycle.ExecutionRequest
	for len(args) > 0 {
		if len(args) < 2 {
			return request, false
		}
		flag, value := args[0], args[1]
		args = args[2:]
		switch flag {
		case "--execution-id", "--id":
			request.ID = value
		case "--label":
			request.Label = value
		case "--target":
			request.Target = value
		case "--command":
			request.Command = value
		case "--require":
			request.Requirements = append(request.Requirements, value)
		case "--resource":
			request.ResourceID = value
		case "--worktree":
			request.WorkingDirectory = value
		default:
			return request, false
		}
	}
	return request, request.Target != ""
}
