package app

import (
	"io"
	"strings"
	"time"

	"github.com/akofink/akagent-cli/internal/lifecycle"
	"github.com/akofink/akagent-cli/internal/store"
	"github.com/google/uuid"
)

type taskView struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	Worker    string `json:"worker"`
	Condition string `json:"condition,omitempty"`
	Reason    string `json:"reason,omitempty"`
	Activity  string `json:"activity,omitempty"`
	Result    string `json:"result,omitempty"`
	Warnings  string `json:"warnings,omitempty"`
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
		return writeError(stdout, "usage", "Usage: akagent task <start|list|inspect|publish|finish|stop|reconcile>", false, "Run `akagent task list`")
	}
	switch args[0] {
	case "start":
		request, ok := parseStart(args[1:])
		if !ok {
			return writeError(stdout, "usage", "Usage: akagent task start --title <title> --repository <name> [--task-id <id>] [--require <credential>] [--optional <credential>]", false, "Register a repository, then start the task")
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
	if len(args) < 1 || args[0] != "register" || len(args) < 3 {
		return writeError(stdout, "usage", "Usage: akagent repository register <name> <path> [--policy <worktree|direct>]", false, "Register a local repository")
	}
	policy := ""
	if len(args) == 5 && args[3] == "--policy" {
		policy = args[4]
	} else if len(args) != 3 {
		return writeError(stdout, "usage", "Usage: akagent repository register <name> <path> [--policy <worktree|direct>]", false, "Register a local repository")
	}
	state, err := store.Open()
	if err != nil {
		return lifecycleError(stdout, err)
	}
	repository, err := lifecycle.New(state).RegisterRepository(args[1], args[2], policy)
	if err != nil {
		return lifecycleError(stdout, err)
	}
	return write(stdout, repositoryView{Repository: repository})
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
		case "--require":
			request.Requirements = append(request.Requirements, value)
		case "--optional":
			request.Optional = append(request.Optional, value)
		default:
			return request, false
		}
	}
	return request, request.Title != "" && request.Repository != ""
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
	return writeError(stdout, "usage", "Usage: akagent task <start|list|inspect|publish|finish|stop|reconcile>", false, "Run `akagent task list`")
}
func view(id string, manifest store.Manifest) taskView {
	return taskView{ID: id, Title: manifest.Title, Status: status(manifest), Worker: manifest.Worker, Condition: manifest.Condition, Reason: manifest.Reason, Activity: manifest.Activity, Result: manifest.Result, Warnings: manifest.Warnings}
}

func status(manifest store.Manifest) string {
	return lifecycle.Status(manifest, time.Now().UTC(), lifecycle.DefaultHeartbeatTimeout)
}

func lifecycleError(stdout io.Writer, err error) int {
	message := err.Error()
	category, retryable := "internal", false
	switch {
	case store.IsKind(err, store.KindNotFound):
		category = "not_found"
	case store.IsKind(err, store.KindUsage):
		category = "usage"
	case store.IsKind(err, store.KindLocked):
		category, retryable = "retryable", true
	case strings.Contains(message, "conflict"):
		category = "conflict"
	case strings.Contains(message, "credential"):
		category = "capability"
	}
	return writeError(stdout, category, message, retryable, "Inspect the task state and retry")
}
