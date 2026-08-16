package app

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/akofink/akagent-cli/internal/integration"
	"github.com/akofink/akagent-cli/internal/lifecycle"
	"github.com/akofink/akagent-cli/internal/store"
	"github.com/google/uuid"
)

type resourceView struct {
	ID                   string            `json:"id"`
	Repository           string            `json:"repository"`
	Branch               string            `json:"branch,omitempty"`
	BaseRevision         string            `json:"base_revision,omitempty"`
	WorktreePath         string            `json:"worktree_path,omitempty"`
	Head                 string            `json:"head,omitempty"`
	Committed            bool              `json:"committed"`
	Dirty                bool              `json:"dirty"`
	Untracked            bool              `json:"untracked"`
	RecoveryDebt         string            `json:"recovery_debt,omitempty"`
	ArchiveState         string            `json:"archive_state,omitempty"`
	CleanupState         string            `json:"cleanup_state,omitempty"`
	WorktreeCleanupState string            `json:"worktree_cleanup_state,omitempty"`
	CleanupDebt          bool              `json:"cleanup_debt,omitempty"`
	Metadata             map[string]string `json:"metadata,omitempty"`
	ExternalURLs         []string          `json:"external_urls,omitempty"`
}

type resourceListItem struct {
	ID                   string `json:"id"`
	Repository           string `json:"repository"`
	Branch               string `json:"branch,omitempty"`
	BaseRevision         string `json:"base_revision,omitempty"`
	WorktreePath         string `json:"worktree_path,omitempty"`
	Head                 string `json:"head,omitempty"`
	Committed            bool   `json:"committed"`
	Dirty                bool   `json:"dirty"`
	Untracked            bool   `json:"untracked"`
	RecoveryDebt         string `json:"recovery_debt,omitempty"`
	ArchiveState         string `json:"archive_state,omitempty"`
	CleanupState         string `json:"cleanup_state,omitempty"`
	WorktreeCleanupState string `json:"worktree_cleanup_state,omitempty"`
	CleanupDebt          bool   `json:"cleanup_debt,omitempty"`
	Metadata             string `json:"metadata,omitempty"`
	ExternalURLs         string `json:"external_urls,omitempty"`
}

type resourceListView struct {
	Resources []resourceListItem `json:"resources"`
	Total     int                `json:"total"`
}
type resourceDetailView struct {
	Resource resourceView `json:"resource"`
}

type sessionReferenceView struct {
	Tool          string `json:"tool"`
	SessionID     string `json:"session_id"`
	ReferencePath string `json:"reference_path,omitempty"`
}

type executionView struct {
	ID                string `json:"id"`
	TaskID            string `json:"task_id"`
	Label             string `json:"label"`
	Target            string `json:"target"`
	Command           string `json:"command,omitempty"`
	ResourceID        string `json:"resource_id,omitempty"`
	WorkingDirectory  string `json:"working_directory,omitempty"`
	Status            string `json:"status"`
	Condition         string `json:"condition,omitempty"`
	Reason            string `json:"reason,omitempty"`
	Activity          string `json:"activity,omitempty"`
	Result            string `json:"result,omitempty"`
	TmuxWindow        string `json:"tmux_window,omitempty"`
	ProcessPID        int    `json:"process_pid,omitempty"`
	Observation       string `json:"observation,omitempty"`
	RecoveryDebt      string `json:"recovery_debt,omitempty"`
	ArchiveState      string `json:"archive_state,omitempty"`
	SessionReferences string `json:"session_references,omitempty"`
}

type executionListView struct {
	Executions []executionView `json:"executions"`
	Total      int             `json:"total"`
}
type executionDetailView struct {
	Execution         executionView          `json:"execution"`
	SessionReferences []sessionReferenceView `json:"session_references,omitempty"`
}

type taskView struct {
	ID                     string `json:"id"`
	Title                  string `json:"title"`
	Status                 string `json:"status"`
	Worker                 string `json:"worker"`
	Branch                 string `json:"branch,omitempty"`
	BaseRevision           string `json:"base_revision,omitempty"`
	WorktreePath           string `json:"worktree_path,omitempty"`
	Condition              string `json:"condition,omitempty"`
	Reason                 string `json:"reason,omitempty"`
	Activity               string `json:"activity,omitempty"`
	Result                 string `json:"result,omitempty"`
	Committed              bool   `json:"committed"`
	Dirty                  bool   `json:"dirty"`
	Untracked              bool   `json:"untracked"`
	RecoveryDebt           string `json:"recovery_debt,omitempty"`
	Warnings               string `json:"warnings,omitempty"`
	ArchiveState           string `json:"archive_state,omitempty"`
	CleanupState           string `json:"cleanup_state,omitempty"`
	WorktreeCleanupState   string `json:"worktree_cleanup_state,omitempty"`
	CredentialCleanupState string `json:"credential_cleanup_state,omitempty"`
	CleanupDebt            bool   `json:"cleanup_debt,omitempty"`
	Agent                  string `json:"agent,omitempty"`
	AgentCommand           string `json:"agent_command,omitempty"`
	PromptReference        string `json:"prompt_reference,omitempty"`
	WorkingContext         string `json:"working_context,omitempty"`
	Execution              string `json:"execution,omitempty"`
}

type taskListView struct {
	Tasks []taskView `json:"tasks"`
	Total int        `json:"total"`
}
type taskDetailView struct {
	Task       taskView           `json:"task"`
	Resources  []resourceListItem `json:"resources,omitempty"`
	Executions []executionView    `json:"executions,omitempty"`
}
type repositoryView struct {
	Repository store.Repository `json:"repository"`
}

func taskCommand(args []string, stdout io.Writer) int {
	state, err := store.Open()
	if err != nil {
		return lifecycleError(stdout, err)
	}
	manager := lifecycle.New(state)
	if len(args) == 0 {
		return writeError(stdout, "usage", "Usage: akagent task <create|resource|execution|launch|start|list|inspect|attach|publish|finish|stop|archive|clean|reconcile>", false, "Run `akagent task list`")
	}
	switch args[0] {
	case "resource":
		return taskResourceCommand(args[1:], stdout)
	case "execution":
		return taskExecutionCommand(args[1:], stdout)
	case "create":
		request, ok := parseCreate(args[1:])
		if !ok {
			return writeError(stdout, "usage", "Usage: akagent task create --title <title> [--task-id <id>] [--repository <name>] [--branch <branch>] [--base <revision>] [--worktree <path>] [--require <credential>] [--optional <credential>]", false, "Create the task, then add resources with `akagent task resource create`")
		}
		if request.ID == "" {
			id, idErr := uuid.NewV7()
			if idErr != nil {
				return writeError(stdout, "internal", "Failed to generate a task ID", false, "Retry `akagent task create`")
			}
			request.ID = id.String()
		}
		result, err := manager.Create(request)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, taskDetailView{Task: view(request.ID, result.Manifest)})
	case "launch":
		if len(args) < 3 {
			return writeError(stdout, "usage", "Usage: akagent task launch <task-id> --target <shell|pi> [--resource <resource-id>] [--prompt <path>] [--context <value>]", false, "Create a task, then launch an explicit execution")
		}
		request, ok := parseLaunch(args[2:])
		if !ok || (request.Target != "shell" && request.Target != "pi") {
			return writeError(stdout, "usage", "Usage: akagent task launch <task-id> --target <shell|pi> [--resource <resource-id>] [--prompt <path>] [--context <value>]", false, "Use --target shell for a direct shell or --target pi for managed Pi")
		}
		var execution store.Execution
		if request.Target == "pi" {
			execution, err = integration.Launch(manager, args[1], integration.LaunchRequest{Label: request.Label, ResourceID: request.ResourceID, PromptReference: request.PromptReference, WorkingContext: request.WorkingContext})
		} else {
			execution, err = integration.LaunchShell(manager, args[1], request.Label, request.ResourceID)
		}
		if err != nil {
			return lifecycleError(stdout, err)
		}
		manifest, err := manager.Inspect(args[1])
		if err != nil {
			return lifecycleError(stdout, err)
		}
		task := view(args[1], manifest)
		task.Execution = execution.Target
		if execution.Target == "pi" {
			task.Agent = execution.Target
		}
		return write(stdout, taskDetailView{Task: task})
	case "start":
		request, ok := parseStart(args[1:])
		if !ok {
			return writeError(stdout, "usage", "Usage: akagent task start --title <title> --repository <name> [--task-id <id>] [--branch <branch>] [--base <revision>] [--worktree <path>] [--agent pi --prompt <path>] [--context <value>] [--require <credential>] [--optional <credential>]", false, "Register a repository, then start the task")
		}
		if request.ID == "" {
			id, idErr := uuid.NewV7()
			if idErr != nil {
				return writeError(stdout, "internal", "Failed to generate a task ID", false, "Retry `akagent task start`")
			}
			request.ID = id.String()
		}
		if request.Agent != "" {
			if _, err := manager.Create(lifecycle.CreateRequest{ID: request.ID, Title: request.Title, Repository: request.Repository, Branch: request.Branch, BaseRevision: request.BaseRevision, WorktreePath: request.WorktreePath, Requirements: request.Requirements, Optional: request.Optional}); err != nil {
				return lifecycleError(stdout, err)
			}
			if request.Agent != "pi" {
				return lifecycleError(stdout, fmt.Errorf("unsupported agent target %q", request.Agent))
			}
			execution, err := integration.Launch(manager, request.ID, integration.LaunchRequest{ResourceID: "", PromptReference: request.PromptReference, WorkingContext: request.WorkingContext})
			if err != nil {
				return lifecycleError(stdout, err)
			}
			manifest, err := manager.Inspect(request.ID)
			if err != nil {
				return lifecycleError(stdout, err)
			}
			task := view(request.ID, manifest)
			task.Execution, task.Agent = execution.Target, execution.Target
			return write(stdout, taskDetailView{Task: task})
		}
		result, err := manager.Start(request)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, taskDetailView{Task: view(request.ID, result.Manifest)})
	case "list":
		options, ok := parseTaskList(args[1:])
		if !ok {
			return taskListUsage(stdout)
		}
		ids, err := state.TaskIDs()
		if err != nil {
			return lifecycleError(stdout, err)
		}
		items := make([]taskView, 0, len(ids))
		for _, id := range ids {
			manifest, err := manager.Inspect(id)
			if err != nil {
				return lifecycleError(stdout, err)
			}
			if !options.All && !actionable(manifest) {
				resources, resourceErr := manager.ListResources(id)
				if resourceErr != nil {
					return lifecycleError(stdout, resourceErr)
				}
				if manifest.Repository != "" || !actionableResources(resources) {
					continue
				}
			}
			if options.Repository != "" || options.Worktree != "" {
				matches := (options.Repository == "" || manifest.Repository == options.Repository) && (options.Worktree == "" || manifest.WorktreePath == options.Worktree)
				if !matches {
					resources, resourceErr := manager.ListResources(id)
					if resourceErr != nil {
						return lifecycleError(stdout, resourceErr)
					}
					for _, resource := range resources {
						if (options.Repository == "" || resource.Repository == options.Repository) && (options.Worktree == "" || resource.WorktreePath == options.Worktree) {
							matches = true
							break
						}
					}
				}
				if !matches {
					continue
				}
			}
			items = append(items, view(id, manifest))
		}
		return write(stdout, taskListView{Tasks: items, Total: len(items)})
	case "inspect":
		if len(args) != 2 {
			return writeError(stdout, "usage", "Usage: akagent task inspect <task-id>", false, "Run `akagent task list`")
		}
		manifest, err := manager.Inspect(args[1])
		if err != nil {
			return lifecycleError(stdout, err)
		}
		detail, err := taskDetail(manager, args[1], manifest)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, detail)
	case "attach":
		if len(args) != 2 {
			return writeError(stdout, "usage", "Usage: akagent task attach <task-id>", false, "Run `akagent task list`")
		}
		if err := manager.Attach(args[1]); err != nil {
			return lifecycleError(stdout, err)
		}
		return 0
	case "publish":
		if len(args) < 4 {
			return writeError(stdout, "usage", "Usage: akagent task publish <task-id> --condition <condition> [--reason <reason>] [--activity <activity>]", false, "Publish active, waiting, blocked, failed, or none")
		}
		condition, reason, activity, ok := parsePublish(args[2:])
		if !ok {
			return taskUsage(stdout)
		}
		manifest, err := manager.Publish(args[1], condition, reason, activity)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, taskDetailView{Task: view(args[1], manifest)})
	case "finish":
		if len(args) != 4 || (args[2] != "succeeded" && args[2] != "failed") {
			return writeError(stdout, "usage", "Usage: akagent task finish <task-id> <succeeded|failed> <result>", false, "Record a concise task outcome")
		}
		manifest, err := manager.Finish(args[1], args[2], args[3])
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, taskDetailView{Task: view(args[1], manifest)})
	case "stop":
		if len(args) != 2 {
			return writeError(stdout, "usage", "Usage: akagent task stop <task-id>", false, "Run `akagent task list`")
		}
		manifest, err := manager.Stop(args[1])
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, taskDetailView{Task: view(args[1], manifest)})
	case "archive":
		if len(args) != 2 {
			return writeError(stdout, "usage", "Usage: akagent task archive <task-id>", false, "Run `akagent task list`")
		}
		manifest, err := manager.Archive(args[1])
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, taskDetailView{Task: view(args[1], manifest)})
	case "clean":
		if len(args) < 2 {
			return writeError(stdout, "usage", "Usage: akagent task clean <task-id> [--allow-committed] [--allow-dirty] [--allow-untracked] [--allow-worktree]", false, "Inspect the task before authorizing cleanup")
		}
		options, ok := parseCleanup(args[2:])
		if !ok {
			return writeError(stdout, "usage", "Usage: akagent task clean <task-id> [--allow-committed] [--allow-dirty] [--allow-untracked] [--allow-worktree]", false, "Inspect the task before authorizing cleanup")
		}
		manifest, err := manager.Clean(args[1], options)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, taskDetailView{Task: view(args[1], manifest)})
	case "reconcile":
		if len(args) != 1 {
			return taskUsage(stdout)
		}
		manifests, err := manager.Reconcile()
		if err != nil {
			return lifecycleError(stdout, err)
		}
		ids, _ := state.TaskIDs()
		items := make([]taskView, 0, len(manifests))
		for index, manifest := range manifests {
			items = append(items, view(ids[index], manifest))
		}
		return write(stdout, taskListView{Tasks: items, Total: len(items)})
	}
	return taskUsage(stdout)
}

func repositoryCommand(args []string, stdout io.Writer) int {
	if len(args) == 0 {
		return repositoryUsage(stdout)
	}
	switch args[0] {
	case "register":
		return repositoryRegisterCommand(args[1:], stdout)
	case "list":
		return repositoryListCommand(args[1:], stdout)
	case "inspect":
		return repositoryInspectCommand(args[1:], stdout)
	case "update":
		return repositoryUpdateCommand(args[1:], stdout)
	case "unregister":
		return repositoryUnregisterCommand(args[1:], stdout)
	default:
		return repositoryUsage(stdout)
	}
}

type taskListOptions struct {
	All        bool
	Repository string
	Worktree   string
}

func parseTaskList(args []string) (taskListOptions, bool) {
	var options taskListOptions
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--all":
			options.All = true
		case "--repository", "--worktree":
			if index+1 >= len(args) || args[index+1] == "" {
				return options, false
			}
			if args[index] == "--repository" {
				options.Repository = args[index+1]
			} else {
				options.Worktree = args[index+1]
			}
			index++
		default:
			return options, false
		}
	}
	return options, true
}

func taskExecutionCommand(args []string, stdout io.Writer) int {
	if len(args) == 0 {
		return writeError(stdout, "usage", "Usage: akagent task execution <create|launch|list|inspect|session|publish|attach|stop|archive|reconcile>", false, "Run `akagent task execution list <task-id>`")
	}
	state, err := store.Open()
	if err != nil {
		return lifecycleError(stdout, err)
	}
	manager := lifecycle.New(state)
	switch args[0] {
	case "create":
		if len(args) < 2 {
			return writeError(stdout, "usage", "Usage: akagent task execution create <task-id> --target <target> [--execution-id <id>] [--label <label>] [--command <command>] [--resource <resource-id>] [--worktree <path>]", false, "Create an execution without starting it")
		}
		request, ok := parseExecutionCreate(args[2:])
		if !ok {
			return writeError(stdout, "usage", "Usage: akagent task execution create <task-id> --target <target> [--execution-id <id>] [--label <label>] [--command <command>] [--resource <resource-id>] [--worktree <path>]", false, "Provide a target and immutable execution inputs")
		}
		execution, _, err := manager.CreateExecution(args[1], request)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, executionDetail(execution, manager))
	case "launch":
		if len(args) < 3 {
			return writeError(stdout, "usage", "Usage: akagent task execution launch <task-id> <execution-id>", false, "Create an execution, then launch it")
		}
		execution, err := manager.LaunchExecutionRecord(args[1], args[2])
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
	case "publish":
		if len(args) < 4 {
			return writeError(stdout, "usage", "Usage: akagent task execution publish <task-id> <execution-id> --condition <condition> [--reason <reason>] [--activity <activity>]", false, "Publish execution condition and heartbeat")
		}
		condition, reason, activity, ok := parsePublish(args[3:])
		if !ok {
			return taskUsage(stdout)
		}
		execution, err := manager.PublishExecution(args[1], args[2], condition, reason, activity)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, executionDetail(execution, manager))
	case "attach", "stop", "archive":
		if len(args) != 3 {
			return writeError(stdout, "usage", "Usage: akagent task execution <attach|stop|archive> <task-id> <execution-id>", false, "Provide the task and execution IDs")
		}
		if args[0] == "attach" {
			if err := manager.AttachExecution(args[1], args[2]); err != nil {
				return lifecycleError(stdout, err)
			}
			return 0
		}
		var execution lifecycleExecutionResult
		if args[0] == "stop" {
			execution.Execution, execution.Err = manager.StopExecution(args[1], args[2])
		} else {
			execution.Execution, execution.Err = manager.ArchiveExecution(args[1], args[2])
		}
		if execution.Err != nil {
			return lifecycleError(stdout, execution.Err)
		}
		return write(stdout, executionDetail(execution.Execution, manager))
	case "reconcile":
		if len(args) != 2 {
			return writeError(stdout, "usage", "Usage: akagent task execution reconcile <task-id>", false, "Reconcile executions without changing resources")
		}
		executions, err := manager.ReconcileExecutions(args[1])
		if err != nil {
			return lifecycleError(stdout, err)
		}
		items := make([]executionView, 0, len(executions))
		for _, execution := range executions {
			items = append(items, viewExecution(execution, manager))
		}
		return write(stdout, executionListView{Executions: items, Total: len(items)})
	default:
		return writeError(stdout, "usage", "Usage: akagent task execution <create|launch|list|inspect|session|publish|attach|stop|archive|reconcile>", false, "Run `akagent task execution list <task-id>`")
	}
}

type lifecycleExecutionResult struct {
	Execution store.Execution
	Err       error
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

func taskResourceCommand(args []string, stdout io.Writer) int {
	if len(args) == 0 {
		return writeError(stdout, "usage", "Usage: akagent task resource <create|list|inspect|update|archive|clean>", false, "Run `akagent task resource list <task-id>`")
	}
	state, err := store.Open()
	if err != nil {
		return lifecycleError(stdout, err)
	}
	manager := lifecycle.New(state)
	switch args[0] {
	case "create", "add":
		if len(args) < 2 {
			return writeError(stdout, "usage", "Usage: akagent task resource create <task-id> --repository <name> [--resource-id <id>] [--branch <branch>] [--base <revision>] [--worktree <path>] [--metadata <key=value>] [--external-url <https-url>]", false, "Create the task first, then add a Git resource")
		}
		request, ok := parseResourceCreate(args[2:])
		if !ok {
			return writeError(stdout, "usage", "Usage: akagent task resource create <task-id> --repository <name> [--resource-id <id>] [--branch <branch>] [--base <revision>] [--worktree <path>] [--metadata <key=value>] [--external-url <https-url>]", false, "Provide a repository and immutable Git inputs")
		}
		if request.ID == "" {
			id, idErr := uuid.NewV7()
			if idErr != nil {
				return writeError(stdout, "internal", "Failed to generate a resource ID", false, "Retry resource creation")
			}
			request.ID = id.String()
		}
		resource, _, err := manager.CreateResource(args[1], request)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, resourceDetailView{Resource: viewResource(resource)})
	case "list":
		if len(args) != 2 {
			return writeError(stdout, "usage", "Usage: akagent task resource list <task-id>", false, "Run `akagent task list`")
		}
		resources, err := manager.ListResources(args[1])
		if err != nil {
			return lifecycleError(stdout, err)
		}
		items := make([]resourceListItem, 0, len(resources))
		for _, resource := range resources {
			items = append(items, viewResourceList(resource))
		}
		return write(stdout, resourceListView{Resources: items, Total: len(items)})
	case "inspect":
		if len(args) < 2 || len(args) > 3 {
			return writeError(stdout, "usage", "Usage: akagent task resource inspect <task-id> [<resource-id>]", false, "Run `akagent task resource list <task-id>`")
		}
		resourceID := ""
		if len(args) == 3 {
			resourceID = args[2]
		}
		resource, err := manager.InspectResource(args[1], resourceID)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, resourceDetailView{Resource: viewResource(resource)})
	case "update":
		if len(args) < 3 {
			return writeError(stdout, "usage", "Usage: akagent task resource update <task-id> <resource-id> [--metadata <key=value>] [--external-url <https-url>]", false, "Record non-secret delivery metadata for the resource")
		}
		request, ok := parseResourceUpdate(args[3:])
		if !ok {
			return writeError(stdout, "usage", "Usage: akagent task resource update <task-id> <resource-id> [--metadata <key=value>] [--external-url <https-url>]", false, "Record non-secret delivery metadata for the resource")
		}
		resource, err := manager.UpdateResource(args[1], args[2], request)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, resourceDetailView{Resource: viewResource(resource)})
	case "archive":
		if len(args) != 3 {
			return writeError(stdout, "usage", "Usage: akagent task resource archive <task-id> <resource-id>", false, "Inspect the resource before archiving it")
		}
		resource, err := manager.ArchiveResource(args[1], args[2])
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, resourceDetailView{Resource: viewResource(resource)})
	case "clean":
		if len(args) < 3 {
			return writeError(stdout, "usage", "Usage: akagent task resource clean <task-id> <resource-id> [--allow-committed] [--allow-dirty] [--allow-untracked] [--allow-worktree]", false, "Inspect the resource before authorizing cleanup")
		}
		options, ok := parseCleanup(args[3:])
		if !ok {
			return writeError(stdout, "usage", "Usage: akagent task resource clean <task-id> <resource-id> [--allow-committed] [--allow-dirty] [--allow-untracked] [--allow-worktree]", false, "Inspect the resource before authorizing cleanup")
		}
		resource, err := manager.CleanResource(args[1], args[2], options)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, resourceDetailView{Resource: viewResource(resource)})
	default:
		return writeError(stdout, "usage", "Usage: akagent task resource <create|list|inspect|update|archive|clean>", false, "Run `akagent task resource list <task-id>`")
	}
}

func parseResourceCreate(args []string) (lifecycle.ResourceRequest, bool) {
	var request lifecycle.ResourceRequest
	metadata, urls, ok := parseResourceMetadata(args, &request.ID, &request.Repository, &request.Branch, &request.BaseRevision, &request.WorktreePath)
	request.Metadata, request.ExternalURLs = metadata, urls
	return request, ok && request.Repository != ""
}

func parseResourceUpdate(args []string) (lifecycle.ResourceUpdateRequest, bool) {
	var request lifecycle.ResourceUpdateRequest
	metadata, urls, ok := parseResourceMetadata(args, nil, nil, nil, nil, nil)
	request.Metadata, request.ExternalURLs = metadata, urls
	return request, ok && (len(metadata) > 0 || len(urls) > 0)
}

func parseResourceMetadata(args []string, id, repository, branch, base, worktree *string) (map[string]string, []string, bool) {
	var metadata map[string]string
	var urls []string
	for len(args) > 0 {
		if len(args) < 2 {
			return nil, nil, false
		}
		flag, value := args[0], args[1]
		args = args[2:]
		switch flag {
		case "--resource-id", "--id":
			if id == nil {
				return nil, nil, false
			}
			*id = value
		case "--repository":
			if repository == nil {
				return nil, nil, false
			}
			*repository = value
		case "--branch":
			if branch == nil {
				return nil, nil, false
			}
			*branch = value
		case "--base":
			if base == nil {
				return nil, nil, false
			}
			*base = value
		case "--worktree":
			if worktree == nil {
				return nil, nil, false
			}
			*worktree = value
		case "--metadata":
			key, metadataValue, found := strings.Cut(value, "=")
			if !found || key == "" || metadataValue == "" {
				return nil, nil, false
			}
			if metadata == nil {
				metadata = map[string]string{}
			}
			metadata[key] = metadataValue
		case "--external-url", "--external-reference", "--url":
			if value == "" {
				return nil, nil, false
			}
			urls = append(urls, value)
		default:
			return nil, nil, false
		}
	}
	return metadata, urls, true
}

func parseCreate(args []string) (lifecycle.CreateRequest, bool) {
	var request lifecycle.CreateRequest
	for len(args) > 0 {
		if len(args) < 2 {
			return request, false
		}
		flag, value := args[0], args[1]
		args = args[2:]
		switch flag {
		case "--task-id":
			request.ID = value
		case "--title":
			request.Title = value
		case "--repository":
			request.Repository = value
		case "--branch":
			request.Branch = value
		case "--base":
			request.BaseRevision = value
		case "--worktree":
			request.WorktreePath = value
		case "--require":
			request.Requirements = append(request.Requirements, value)
		case "--optional":
			request.Optional = append(request.Optional, value)
		default:
			return request, false
		}
	}
	return request, request.Title != ""
}

func parseLaunch(args []string) (lifecycle.LaunchRequest, bool) {
	var request lifecycle.LaunchRequest
	for len(args) > 0 {
		if len(args) < 2 {
			return request, false
		}
		flag, value := args[0], args[1]
		args = args[2:]
		switch flag {
		case "--target", "--execution":
			request.Target = value
		case "--execution-id":
			request.ExecutionID = value
		case "--label":
			request.Label = value
		case "--resource":
			request.ResourceID = value
		case "--agent":
			request.Target = value
		case "--prompt", "--prompt-ref", "--prompt-reference":
			request.PromptReference = value
		case "--context", "--working-context":
			request.WorkingContext = value
		default:
			return request, false
		}
	}
	return request, request.Target != ""
}

func parseStart(args []string) (lifecycle.StartRequest, bool) {
	var request lifecycle.StartRequest
	for len(args) > 0 {
		if len(args) < 2 {
			return request, false
		}
		flag, value := args[0], args[1]
		args = args[2:]
		switch flag {
		case "--task-id":
			request.ID = value
		case "--title":
			request.Title = value
		case "--repository":
			request.Repository = value
		case "--branch":
			request.Branch = value
		case "--base":
			request.BaseRevision = value
		case "--worktree":
			request.WorktreePath = value
		case "--require":
			request.Requirements = append(request.Requirements, value)
		case "--optional":
			request.Optional = append(request.Optional, value)
		case "--agent", "--command":
			request.Agent = value
		case "--prompt", "--prompt-ref", "--prompt-reference":
			request.PromptReference = value
		case "--context", "--working-context":
			request.WorkingContext = value
		default:
			return request, false
		}
	}
	return request, request.Title != "" && request.Repository != ""
}
func parseCleanup(args []string) (lifecycle.CleanupOptions, bool) {
	var options lifecycle.CleanupOptions
	for _, arg := range args {
		switch arg {
		case "--allow-committed":
			options.AllowCommitted = true
		case "--allow-dirty":
			options.AllowDirty = true
		case "--allow-untracked":
			options.AllowUntracked = true
		case "--allow-worktree":
			options.AllowWorktree = true
		default:
			return options, false
		}
	}
	return options, true
}

func parsePublish(args []string) (condition, reason, activity string, ok bool) {
	for len(args) > 0 {
		if len(args) < 2 {
			return "", "", "", false
		}
		flag, value := args[0], args[1]
		args = args[2:]
		switch flag {
		case "--condition":
			condition = value
		case "--reason":
			reason = value
		case "--activity":
			activity = value
		default:
			return "", "", "", false
		}
	}
	return condition, reason, activity, condition != ""
}
func taskUsage(stdout io.Writer) int {
	return writeError(stdout, "usage", "Usage: akagent task <create|resource|execution|launch|start|list|inspect|attach|publish|finish|stop|archive|clean|reconcile>", false, "Run `akagent task list`")
}

func taskListUsage(stdout io.Writer) int {
	return writeError(stdout, "usage", "Usage: akagent task list [--all] [--repository <name>] [--worktree <path>]", false, "Run `akagent task list`")
}

func actionable(manifest store.Manifest) bool {
	return manifest.ArchiveState != "complete" ||
		manifest.CleanupState != "complete" ||
		manifest.CleanupDebt ||
		strings.TrimSpace(manifest.RecoveryDebt) != ""
}

func actionableResources(resources []store.Resource) bool {
	for _, resource := range resources {
		if resource.ArchiveState != "complete" || resource.CleanupState != "complete" || resource.CleanupDebt || strings.TrimSpace(resource.RecoveryDebt) != "" {
			return true
		}
	}
	return false
}

func viewResource(resource store.Resource) resourceView {
	return resourceView{ID: resource.ID, Repository: resource.Repository, Branch: resource.Branch, BaseRevision: resource.BaseRevision, WorktreePath: resource.WorktreePath, Head: resource.Git.Head, Committed: resource.Git.Committed, Dirty: resource.Git.Dirty, Untracked: resource.Git.Untracked, RecoveryDebt: resource.RecoveryDebt, ArchiveState: taskState(resource.ArchiveState), CleanupState: taskState(resource.CleanupState), WorktreeCleanupState: taskState(resource.WorktreeCleanupState), CleanupDebt: resource.CleanupDebt, Metadata: resource.Metadata, ExternalURLs: resource.ExternalURLs}
}

func viewResourceList(resource store.Resource) resourceListItem {
	metadata := make([]string, 0, len(resource.Metadata))
	for key, value := range resource.Metadata {
		metadata = append(metadata, key+"="+value)
	}
	sort.Strings(metadata)
	return resourceListItem{ID: resource.ID, Repository: resource.Repository, Branch: resource.Branch, BaseRevision: resource.BaseRevision, WorktreePath: resource.WorktreePath, Head: resource.Git.Head, Committed: resource.Git.Committed, Dirty: resource.Git.Dirty, Untracked: resource.Git.Untracked, RecoveryDebt: resource.RecoveryDebt, ArchiveState: taskState(resource.ArchiveState), CleanupState: taskState(resource.CleanupState), WorktreeCleanupState: taskState(resource.WorktreeCleanupState), CleanupDebt: resource.CleanupDebt, Metadata: strings.Join(metadata, ";"), ExternalURLs: strings.Join(resource.ExternalURLs, ",")}
}

func viewExecution(execution store.Execution, manager *lifecycle.Manager) executionView {
	return executionView{ID: execution.ID, TaskID: execution.TaskID, Label: execution.Label, Target: execution.Target, Command: execution.Command, ResourceID: execution.ResourceID, WorkingDirectory: execution.WorkingDirectory, Status: lifecycle.ExecutionStatus(execution, time.Now().UTC(), lifecycle.DefaultHeartbeatTimeout), Condition: execution.Condition, Reason: execution.Reason, Activity: execution.Activity, Result: execution.Result, TmuxWindow: execution.TmuxWindow, ProcessPID: execution.ProcessPID, Observation: execution.Observation, RecoveryDebt: execution.RecoveryDebt, ArchiveState: taskState(execution.ArchiveState), SessionReferences: compactSessionReferences(execution.SessionReferences)}
}

func executionDetail(execution store.Execution, manager *lifecycle.Manager) executionDetailView {
	references := make([]sessionReferenceView, 0, len(execution.SessionReferences))
	for _, reference := range execution.SessionReferences {
		references = append(references, sessionReferenceView{Tool: reference.Tool, SessionID: reference.SessionID, ReferencePath: reference.ReferencePath})
	}
	return executionDetailView{Execution: viewExecution(execution, manager), SessionReferences: references}
}

func taskDetail(manager *lifecycle.Manager, id string, manifest store.Manifest) (taskDetailView, error) {
	resources, err := manager.ListResources(id)
	if err != nil {
		return taskDetailView{}, err
	}
	executions, err := manager.ListExecutions(id)
	if err != nil {
		return taskDetailView{}, err
	}
	resourceViews := make([]resourceListItem, 0, len(resources))
	for _, resource := range resources {
		resourceViews = append(resourceViews, viewResourceList(resource))
	}
	executionViews := make([]executionView, 0, len(executions))
	for _, execution := range executions {
		executionViews = append(executionViews, viewExecution(execution, manager))
	}
	return taskDetailView{Task: view(id, manifest), Resources: resourceViews, Executions: executionViews}, nil
}

func compactSessionReferences(references []store.SessionReference) string {
	values := make([]string, 0, len(references))
	for _, reference := range references {
		value := reference.Tool + ":" + reference.SessionID
		if reference.ReferencePath != "" {
			value += "@" + reference.ReferencePath
		}
		values = append(values, value)
	}
	return strings.Join(values, ",")
}

func view(id string, manifest store.Manifest) taskView {
	result := taskView{ID: id, Title: manifest.Title, Status: status(manifest), Worker: manifest.Worker, Branch: manifest.Branch, BaseRevision: manifest.BaseRevision, WorktreePath: manifest.WorktreePath, Condition: manifest.Condition, Reason: manifest.Reason, Activity: manifest.Activity, Result: manifest.Result, Committed: manifest.Committed, Dirty: manifest.Dirty, Untracked: manifest.Untracked, RecoveryDebt: manifest.RecoveryDebt, Warnings: manifest.Warnings, ArchiveState: taskState(manifest.ArchiveState), CleanupState: taskState(manifest.CleanupState), WorktreeCleanupState: taskState(manifest.WorktreeCleanupState), CredentialCleanupState: taskState(manifest.CredentialCleanupState), CleanupDebt: manifest.CleanupDebt}
	if manifest.Launch != nil {
		result.Execution = manifest.Launch.Target
		if manifest.Launch.Target == "pi" {
			result.Agent = manifest.Launch.Target
		}
		result.AgentCommand = manifest.Launch.Command
		result.PromptReference = manifest.Launch.PromptReference
		result.WorkingContext = manifest.Launch.WorkingContext
	}
	return result
}

func taskState(value string) string {
	if value == "none" {
		return ""
	}
	return value
}

func status(manifest store.Manifest) string {
	return lifecycle.Status(manifest, time.Now().UTC(), lifecycle.DefaultHeartbeatTimeout)
}

func lifecycleError(stdout io.Writer, err error) int {
	message := err.Error()
	category, retryable, recovery := "internal", false, "Inspect the task state and retry"
	var storeErr *store.Error
	if errors.As(err, &storeErr) && (storeErr.Kind == store.KindConflict || storeErr.Kind == store.KindPartial || storeErr.Kind == store.KindPreservation) {
		if storeErr.Recovery != "" {
			recovery = storeErr.Recovery
		}
		retryable = storeErr.Retryable
	}
	switch {
	case store.IsKind(err, store.KindNotFound):
		category = "not_found"
	case store.IsKind(err, store.KindUsage):
		category = "usage"
	case store.IsKind(err, store.KindLocked):
		category, retryable = "retryable", true
	case store.IsKind(err, store.KindConflict):
		category = "conflict"
	case store.IsKind(err, store.KindPartial):
		category, retryable = "partial", true
	case store.IsKind(err, store.KindPreservation):
		category = "preservation_required"
	case strings.Contains(message, "conflict"):
		category = "conflict"
	case strings.Contains(message, "credential"):
		category = "capability"
	}
	return writeError(stdout, category, message, retryable, recovery)
}
