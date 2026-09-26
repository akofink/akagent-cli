package app

import (
	"io"
	"path/filepath"
	"strings"

	"github.com/akofink/akagent-cli/internal/lifecycle"
	"github.com/akofink/akagent-cli/internal/store"
)

func taskExternalCommand(args []string, stdout io.Writer, manager *lifecycle.Manager) int {
	if len(args) == 0 {
		return externalTaskUsage(stdout)
	}
	switch args[0] {
	case "create":
		if len(args) < 2 {
			return writeError(stdout, "usage", "Usage: akagent task external create <task-id> --title <title> --caller-id <id> --operation-id <id>", false, "Create an explicitly external task record")
		}
		request, ok := parseExternalTaskCreate(args[2:])
		if !ok {
			return writeError(stdout, "usage", "Usage: akagent task external create <task-id> --title <title> --caller-id <id> --operation-id <id>", false, "Provide a stable caller and operation ID with a non-secret title")
		}
		manifest, err := manager.CreateExternalTask(args[1], request)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, taskDetailView{Task: view(args[1], manifest)})
	case "finish":
		if len(args) < 2 {
			return writeError(stdout, "usage", "Usage: akagent task external finish <task-id> --caller-id <id> --operation-id <id> --expected-revision <revision> --contract <name> --result <result>", false, "Complete an external task explicitly against a named contract")
		}
		callerID, operationID, contract, resultValue, expectedRevision, ok := parseExternalCompletion(args[2:])
		if !ok {
			return writeError(stdout, "usage", "Usage: akagent task external finish <task-id> --caller-id <id> --operation-id <id> --expected-revision <revision> --contract <name> --result <result>", false, "Provide the owning caller, current task revision, contract, and result")
		}
		manifest, err := manager.CompleteExternalTask(args[1], callerID, operationID, contract, resultValue, expectedRevision)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, taskDetailView{Task: view(args[1], manifest)})
	case "resource":
		return taskExternalResourceCommand(args[1:], stdout, manager)
	case "execution":
		return taskExternalExecutionCommand(args[1:], stdout, manager)
	case "archive":
		if len(args) < 2 {
			return writeError(stdout, "usage", "Usage: akagent task external archive <task-id> --caller-id <id> --operation-id <id> --expected-revision <revision>", false, "Archive a completed external task record")
		}
		callerID, operationID, expectedRevision, ok := parseExternalArchive(args[2:])
		if !ok {
			return writeError(stdout, "usage", "Usage: akagent task external archive <task-id> --caller-id <id> --operation-id <id> --expected-revision <revision>", false, "Provide the owning caller, an idempotency key, and the current task revision")
		}
		if _, err := manager.ArchiveExternal(args[1], "", "", callerID, operationID, expectedRevision); err != nil {
			return lifecycleError(stdout, err)
		}
		manifest, err := manager.Inspect(args[1])
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, taskDetailView{Task: view(args[1], manifest)})
	default:
		return externalTaskUsage(stdout)
	}
}

func taskExternalResourceCommand(args []string, stdout io.Writer, manager *lifecycle.Manager) int {
	if len(args) == 0 {
		return externalTaskUsage(stdout)
	}
	switch args[0] {
	case "create":
		if len(args) < 2 {
			return writeError(stdout, "usage", "Usage: akagent task external resource create <task-id> --resource-id <id> --repository <name> --branch <branch> --base <revision> --head <revision> --worktree <absolute-path> --caller-id <id> --operation-id <id>", false, "Create an explicitly external resource record")
		}
		request, ok := parseExternalResourceCreate(args[2:])
		if !ok {
			return writeError(stdout, "usage", "Usage: akagent task external resource create <task-id> --resource-id <id> --repository <name> --branch <branch> --base <revision> --head <revision> --worktree <absolute-path> --caller-id <id> --operation-id <id>", false, "Provide caller-owned repository identity and an absolute referenced worktree path")
		}
		resource, err := manager.CreateExternalResource(args[1], request)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, resourceDetailView{Resource: viewResource(resource)})
	case "archive":
		if len(args) < 3 {
			return writeError(stdout, "usage", "Usage: akagent task external resource archive <task-id> <resource-id> --caller-id <id> --operation-id <id> --expected-revision <revision>", false, "Archive an external resource record")
		}
		callerID, operationID, expectedRevision, ok := parseExternalArchive(args[3:])
		if !ok {
			return writeError(stdout, "usage", "Usage: akagent task external resource archive <task-id> <resource-id> --caller-id <id> --operation-id <id> --expected-revision <revision>", false, "Provide the owning caller, an idempotency key, and the current resource revision")
		}
		if _, err := manager.ArchiveExternal(args[1], args[2], "", callerID, operationID, expectedRevision); err != nil {
			return lifecycleError(stdout, err)
		}
		resource, err := manager.InspectResource(args[1], args[2])
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, resourceDetailView{Resource: viewResource(resource)})
	default:
		return externalTaskUsage(stdout)
	}
}

func taskExternalExecutionCommand(args []string, stdout io.Writer, manager *lifecycle.Manager) int {
	if len(args) == 0 {
		return externalTaskUsage(stdout)
	}
	switch args[0] {
	case "create":
		if len(args) < 2 {
			return writeError(stdout, "usage", "Usage: akagent task external execution create <task-id> --execution-id <id> --resource <resource-id> --caller-id <id> --operation-id <id> [--predecessor <execution-id>] [--tool <tool> --session-id <id> [--reference-path <absolute-path>] ]", false, "Create an explicitly external execution record")
		}
		request, ok := parseExternalExecutionCreate(args[2:])
		if !ok {
			return writeError(stdout, "usage", "Usage: akagent task external execution create <task-id> --execution-id <id> --resource <resource-id> --caller-id <id> --operation-id <id> [--predecessor <execution-id>] [--tool <tool> --session-id <id> [--reference-path <absolute-path>] ]", false, "Provide the owning external resource and stable caller operation identity")
		}
		execution, err := manager.RecordExecution(args[1], request)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, executionDetail(execution, manager))
	case "archive":
		if len(args) < 3 {
			return writeError(stdout, "usage", "Usage: akagent task external execution archive <task-id> <execution-id> --caller-id <id> --operation-id <id> --expected-revision <revision>", false, "Archive a completed external execution record")
		}
		callerID, operationID, expectedRevision, ok := parseExternalArchive(args[3:])
		if !ok {
			return writeError(stdout, "usage", "Usage: akagent task external execution archive <task-id> <execution-id> --caller-id <id> --operation-id <id> --expected-revision <revision>", false, "Provide the owning caller, an idempotency key, and the current execution revision")
		}
		if _, err := manager.ArchiveExternal(args[1], "", args[2], callerID, operationID, expectedRevision); err != nil {
			return lifecycleError(stdout, err)
		}
		execution, err := manager.InspectExecution(args[1], args[2])
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, executionDetail(execution, manager))
	default:
		return externalTaskUsage(stdout)
	}
}

func parseExternalTaskCreate(args []string) (store.ExternalTaskRequest, bool) {
	var request store.ExternalTaskRequest
	seen := map[string]bool{}
	for len(args) > 0 {
		if len(args) < 2 || seen[args[0]] {
			return request, false
		}
		flag, value := args[0], args[1]
		args = args[2:]
		seen[flag] = true
		switch flag {
		case "--title":
			request.Title = value
		case "--caller-id":
			request.CallerID = value
		case "--operation-id":
			request.OperationID = value
		default:
			return request, false
		}
	}
	return request, request.Title != "" && request.CallerID != "" && request.OperationID != ""
}

func parseExternalResourceCreate(args []string) (store.ExternalResourceRequest, bool) {
	var request store.ExternalResourceRequest
	var metadata map[string]string
	seen := map[string]bool{}
	for len(args) > 0 {
		if len(args) < 2 || seen[args[0]] {
			return request, false
		}
		flag, value := args[0], args[1]
		args = args[2:]
		seen[flag] = true
		switch flag {
		case "--resource-id":
			request.ID = value
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
		case "--caller-id":
			request.CallerID = value
		case "--operation-id":
			request.OperationID = value
		case "--expected-revision":
			revision, ok := parseRevision(value)
			if !ok {
				return request, false
			}
			request.ExpectedRevision = revision
		case "--metadata":
			key, metadataValue, ok := strings.Cut(value, "=")
			if !ok || key == "" || metadataValue == "" {
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
	return request, request.ID != "" && request.Repository != "" && request.Branch != "" && request.BaseRevision != "" && request.Head != "" && request.WorktreePath != "" && request.CallerID != "" && request.OperationID != ""
}

func parseExternalExecutionCreate(args []string) (store.ExternalExecutionRequest, bool) {
	var request store.ExternalExecutionRequest
	var tool, sessionID, referencePath string
	seen := map[string]bool{}
	for len(args) > 0 {
		if len(args) < 2 || seen[args[0]] {
			return request, false
		}
		flag, value := args[0], args[1]
		args = args[2:]
		seen[flag] = true
		switch flag {
		case "--execution-id":
			request.ID = value
		case "--resource":
			request.ResourceID = value
		case "--caller-id":
			request.CallerID = value
		case "--operation-id":
			request.OperationID = value
		case "--predecessor":
			request.PredecessorID = value
		case "--tool":
			tool = value
		case "--session-id":
			sessionID = value
		case "--reference-path":
			referencePath = value
		default:
			return request, false
		}
	}
	if tool != "" || sessionID != "" || referencePath != "" {
		if tool == "" || sessionID == "" {
			return request, false
		}
		if referencePath != "" {
			absolute, err := filepath.Abs(referencePath)
			if err != nil {
				return request, false
			}
			referencePath = absolute
		}
		request.SessionReferences = []store.SessionReference{{Tool: tool, SessionID: sessionID, ReferencePath: referencePath}}
	}
	return request, request.ID != "" && request.ResourceID != "" && request.CallerID != "" && request.OperationID != ""
}

func parseExternalArchive(args []string) (callerID, operationID string, expectedRevision uint64, ok bool) {
	seen := map[string]bool{}
	for len(args) > 0 {
		if len(args) < 2 || seen[args[0]] {
			return "", "", 0, false
		}
		flag, value := args[0], args[1]
		args = args[2:]
		seen[flag] = true
		switch flag {
		case "--caller-id":
			callerID = value
		case "--operation-id":
			operationID = value
		case "--expected-revision":
			var parsed bool
			expectedRevision, parsed = parseRevision(value)
			if !parsed {
				return "", "", 0, false
			}
		default:
			return "", "", 0, false
		}
	}
	return callerID, operationID, expectedRevision, callerID != "" && operationID != "" && expectedRevision > 0
}

func externalTaskUsage(stdout io.Writer) int {
	return writeError(stdout, "usage", "Usage: akagent task external <create|finish|resource|execution|archive> ...", false, "Use an explicit caller-owned record operation; external tools own host side effects")
}
