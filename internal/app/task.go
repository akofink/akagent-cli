package app

import (
	"errors"
	"io"
	"strings"
	"time"

	"github.com/akofink/akagent-cli/internal/lifecycle"
	"github.com/akofink/akagent-cli/internal/store"
	"github.com/google/uuid"
)

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
}

type taskListView struct {
	Tasks []taskView `json:"tasks"`
	Total int        `json:"total"`
}
type taskDetailView struct {
	Task taskView `json:"task"`
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
		return writeError(stdout, "usage", "Usage: akagent task <start|list|inspect|attach|publish|finish|stop|archive|clean|reconcile>", false, "Run `akagent task list`")
	}
	switch args[0] {
	case "start":
		request, ok := parseStart(args[1:])
		if !ok {
			return writeError(stdout, "usage", "Usage: akagent task start --title <title> --repository <name> [--task-id <id>] [--agent pi --prompt <path>] [--context <value>] [--require <credential>] [--optional <credential>]", false, "Register a repository, then start the task")
		}
		if request.ID == "" {
			id, idErr := uuid.NewV7()
			if idErr != nil {
				return writeError(stdout, "internal", "Failed to generate a task ID", false, "Retry `akagent task start`")
			}
			request.ID = id.String()
		}
		result, err := manager.Start(request)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, taskDetailView{Task: view(request.ID, result.Manifest)})
	case "list":
		if len(args) != 1 {
			return taskUsage(stdout)
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
		return write(stdout, taskDetailView{Task: view(args[1], manifest)})
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
			return writeError(stdout, "usage", "Usage: akagent task clean <task-id> [--allow-committed] [--allow-dirty] [--allow-untracked]", false, "Inspect the task before authorizing cleanup")
		}
		options, ok := parseCleanup(args[2:])
		if !ok {
			return writeError(stdout, "usage", "Usage: akagent task clean <task-id> [--allow-committed] [--allow-dirty] [--allow-untracked]", false, "Inspect the task before authorizing cleanup")
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
	return writeError(stdout, "usage", "Usage: akagent task <start|list|inspect|attach|publish|finish|stop|archive|clean|reconcile>", false, "Run `akagent task list`")
}
func view(id string, manifest store.Manifest) taskView {
	result := taskView{ID: id, Title: manifest.Title, Status: status(manifest), Worker: manifest.Worker, Branch: manifest.Branch, BaseRevision: manifest.BaseRevision, WorktreePath: manifest.WorktreePath, Condition: manifest.Condition, Reason: manifest.Reason, Activity: manifest.Activity, Result: manifest.Result, Committed: manifest.Committed, Dirty: manifest.Dirty, Untracked: manifest.Untracked, RecoveryDebt: manifest.RecoveryDebt, Warnings: manifest.Warnings, ArchiveState: taskState(manifest.ArchiveState), CleanupState: taskState(manifest.CleanupState), WorktreeCleanupState: taskState(manifest.WorktreeCleanupState), CredentialCleanupState: taskState(manifest.CredentialCleanupState), CleanupDebt: manifest.CleanupDebt}
	if manifest.Launch != nil {
		result.Agent = manifest.Launch.Target
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
