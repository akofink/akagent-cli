package app

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/akofink/akagent-cli/internal/lifecycle"
	"github.com/akofink/akagent-cli/internal/store"
	"github.com/google/uuid"
)

func taskCommand(args []string, stdout io.Writer) int {
	if len(args) == 0 {
		return writeError(stdout, "usage", "Usage: akagent task <create|external|checkpoint|resource|execution|disposition|list|inspect|check|publish|finish|archive|reconcile>", false, "Run `akagent task list`")
	}
	switch args[0] {
	case "record":
		return removedCommandError(stdout, "task record", "Use the normal task, resource, execution, and repository record commands")
	case "credential", "deploy", "launch", "start", "attach", "stop", "clean":
		return removedCommandError(stdout, "task "+args[0], "Use the normal record commands; external tools own process, Git, credential, and cleanup side effects")
	case "execution":
		if len(args) > 1 && (args[1] == "launch" || args[1] == "attach" || args[1] == "stop") {
			return removedCommandError(stdout, "task execution "+args[1], "Use the normal execution record commands; external tools own process and terminal side effects")
		}
	case "resource":
		if len(args) > 1 && args[1] == "clean" {
			return removedCommandError(stdout, "task resource clean", "External tools own worktree and credential cleanup; retain cleanup debt in the record")
		}
	}
	state, err := store.Open()
	if err != nil {
		return lifecycleError(stdout, err)
	}
	manager := lifecycle.New(state)
	switch args[0] {
	case "check":
		return taskCheckCommand(args[1:], stdout, state, manager)
	case "external":
		return taskExternalCommand(args[1:], stdout, manager)
	case "checkpoint":
		return taskCheckpointCommand(args[1:], stdout, manager)
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
		result, err := manager.CreateRecord(request)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, taskDetailView{Task: view(request.ID, result.Manifest)})
	case "disposition":
		if len(args) < 4 {
			return writeError(stdout, "usage", "Usage: akagent task disposition <task-id> <in-flight|deferred|terminal> --reason <reason> [--expected-revision <revision>]", false, "Set record-only work disposition without changing process or Git state")
		}
		disposition, reason, expectedRevision, ok := parseDisposition(args[2:])
		if !ok {
			return writeError(stdout, "usage", "Usage: akagent task disposition <task-id> <in-flight|deferred|terminal> --reason <reason> [--expected-revision <revision>]", false, "Set a valid disposition and reason; use the current revision for a guarded transition")
		}
		manifest, err := manager.SetDisposition(args[1], disposition, reason, expectedRevision)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, taskDetailView{Task: view(args[1], manifest)})
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
			var resources []store.Resource
			resourcesLoaded := false
			loadResources := func() ([]store.Resource, error) {
				if resourcesLoaded {
					return resources, nil
				}
				resources, err = manager.ListResources(id)
				if err != nil {
					return nil, err
				}
				resourcesLoaded = true
				return resources, nil
			}
			if !options.All {
				resources, err = loadResources()
				if err != nil {
					return lifecycleError(stdout, err)
				}
				if !includeTaskInView(manifest, resources, options.View) {
					continue
				}
			}
			if options.Keyword != "" {
				resources, err = loadResources()
				if err != nil {
					return lifecycleError(stdout, err)
				}
				if !taskMatchesKeyword(manifest, resources, options.Keyword) {
					continue
				}
			}
			if options.Repository != "" || options.Worktree != "" {
				matches := (options.Repository == "" || manifest.Repository == options.Repository) && (options.Worktree == "" || manifest.WorktreePath == options.Worktree)
				if !matches {
					resources, err = loadResources()
					if err != nil {
						return lifecycleError(stdout, err)
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
		result := taskListView{Tasks: items, Total: len(items)}
		switch options.Format {
		case outputFormatHuman:
			return writeHumanTaskList(stdout, result)
		case outputFormatJSON:
			return writeJSON(stdout, result)
		default:
			return write(stdout, result)
		}
	case "inspect":
		argument, format, ok := parseTaskInspect(args[1:])
		if !ok {
			return writeError(stdout, "usage", "Usage: akagent task inspect <task-id|keyword> [--format <toon|human|json>]", false, "Run `akagent task list [keyword]`, add `--format human` for terminal output, or add `--format json` for JSON")
		}
		taskID, err := resolveTaskID(state, manager, argument)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		manifest, err := manager.Inspect(taskID)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		detail, err := taskDetail(manager, taskID, manifest)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		switch format {
		case outputFormatHuman:
			return writeHumanTaskDetail(stdout, detail)
		case outputFormatJSON:
			return writeJSON(stdout, detail)
		default:
			return write(stdout, detail)
		}
	case "publish":
		if len(args) < 4 {
			return writeError(stdout, "usage", "Usage: akagent task publish <task-id> --condition <condition> [--reason <reason>] [--activity <activity>]", false, "Publish active, waiting, blocked, failed, or none")
		}
		condition, reason, activity, ok := parsePublish(args[2:])
		if !ok {
			return taskUsage(stdout)
		}
		manifest, err := manager.PublishRecord(args[1], condition, reason, activity)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, taskDetailView{Task: view(args[1], manifest)})
	case "finish":
		if len(args) != 4 || (args[2] != "succeeded" && args[2] != "failed") {
			return writeError(stdout, "usage", "Usage: akagent task finish <task-id> <succeeded|failed> <result>", false, "Record a concise task outcome")
		}
		manifest, err := manager.FinishRecord(args[1], args[2], args[3])
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, taskDetailView{Task: view(args[1], manifest)})
	case "archive":
		if len(args) != 2 {
			return writeError(stdout, "usage", "Usage: akagent task archive <task-id>", false, "Run `akagent task list`")
		}
		archived, err := manager.ArchiveRecord(args[1], "", "")
		if err != nil {
			return lifecycleError(stdout, err)
		}
		manifest, ok := archived.(store.Manifest)
		if !ok {
			return writeError(stdout, "internal", "Task archive returned an invalid record", false, "Retry the archive")
		}
		return write(stdout, taskDetailView{Task: view(args[1], manifest)})
	case "reconcile":
		if len(args) > 2 || (len(args) == 2 && strings.HasPrefix(args[1], "-")) {
			return taskUsage(stdout)
		}
		var manifests []store.Manifest
		var ids []string
		if len(args) == 2 {
			manifest, err := manager.ReconcileRecord(args[1])
			if err != nil {
				return lifecycleError(stdout, err)
			}
			manifests, ids = []store.Manifest{manifest}, []string{args[1]}
		} else {
			var err error
			manifests, err = manager.List()
			if err != nil {
				return lifecycleError(stdout, err)
			}
			ids, _ = state.TaskIDs()
		}
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

type outputFormat string

const (
	outputFormatTOON  outputFormat = "toon"
	outputFormatHuman outputFormat = "human"
	outputFormatJSON  outputFormat = "json"
)

type taskListOptions struct {
	All        bool
	Repository string
	Worktree   string
	Keyword    string
	View       string
	Format     outputFormat
}

func parseTaskList(args []string) (taskListOptions, bool) {
	options := taskListOptions{Format: outputFormatTOON}
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--all":
			options.All = true
		case "--repository", "--worktree", "--format", "--view":
			if index+1 >= len(args) || args[index+1] == "" {
				return options, false
			}
			switch args[index] {
			case "--repository":
				options.Repository = args[index+1]
			case "--worktree":
				options.Worktree = args[index+1]
			case "--format":
				format, ok := parseOutputFormat(args[index+1])
				if !ok {
					return options, false
				}
				options.Format = format
			case "--view":
				if !validTaskListView(args[index+1]) {
					return options, false
				}
				options.View = args[index+1]
			}
			index++
		default:
			if strings.HasPrefix(args[index], "-") || options.Keyword != "" {
				return options, false
			}
			options.Keyword = args[index]
		}
	}
	return options, true
}

func parseTaskInspect(args []string) (string, outputFormat, bool) {
	format := outputFormatTOON
	argument := ""
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--format":
			if index+1 >= len(args) {
				return "", format, false
			}
			parsed, ok := parseOutputFormat(args[index+1])
			if !ok {
				return "", format, false
			}
			format = parsed
			index++
		default:
			if strings.HasPrefix(args[index], "-") || argument != "" {
				return "", format, false
			}
			argument = args[index]
		}
	}
	return argument, format, argument != ""
}

func parseOutputFormat(value string) (outputFormat, bool) {
	switch outputFormat(value) {
	case outputFormatTOON, outputFormatHuman, outputFormatJSON:
		return outputFormat(value), true
	default:
		return "", false
	}
}

func resolveTaskID(state *store.Store, manager *lifecycle.Manager, arg string) (string, error) {
	ids, err := state.TaskIDs()
	if err != nil {
		return "", err
	}
	for _, id := range ids {
		if id == arg {
			return id, nil
		}
	}
	if _, err := uuid.Parse(arg); err == nil {
		if _, err := manager.Inspect(arg); err != nil {
			return "", err
		}
		return arg, nil
	}

	matches := make([]string, 0)
	for _, id := range ids {
		manifest, err := manager.Inspect(id)
		if err != nil {
			return "", err
		}
		resources, err := manager.ListResources(id)
		if err != nil {
			return "", err
		}
		if taskMatchesKeyword(manifest, resources, arg) {
			matches = append(matches, id)
		}
	}

	switch len(matches) {
	case 0:
		return "", &store.Error{
			Kind:     store.KindNotFound,
			Message:  fmt.Sprintf("No tasks matched keyword %s", arg),
			Recovery: "Run `akagent task list [keyword]` to find matching tasks",
		}
	case 1:
		return matches[0], nil
	default:
		return "", &store.Error{
			Kind:     store.KindConflict,
			Message:  fmt.Sprintf("Task keyword %s matched multiple tasks: %s", arg, strings.Join(matches, ", ")),
			Recovery: "Use a more specific keyword or inspect the matching tasks with `akagent task list [keyword]`",
		}
	}
}

func taskMatchesKeyword(manifest store.Manifest, resources []store.Resource, keyword string) bool {
	if strings.Contains(manifest.Title, keyword) || strings.Contains(manifest.Branch, keyword) {
		return true
	}
	for _, resource := range resources {
		if strings.Contains(resource.Branch, keyword) {
			return true
		}
	}
	return false
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

func parseDisposition(args []string) (lifecycle.WorkDisposition, string, *uint64, bool) {
	var disposition lifecycle.WorkDisposition
	var reason string
	var revision *uint64
	for index := 0; index < len(args); index++ {
		flag := args[index]
		if !strings.HasPrefix(flag, "-") {
			if disposition != "" {
				return "", "", nil, false
			}
			disposition = lifecycle.WorkDisposition(flag)
			continue
		}
		if index+1 >= len(args) || args[index+1] == "" {
			return "", "", nil, false
		}
		value := args[index+1]
		index++
		switch flag {
		case "--reason":
			if reason != "" {
				return "", "", nil, false
			}
			reason = value
		case "--expected-revision":
			if revision != nil {
				return "", "", nil, false
			}
			parsed, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				return "", "", nil, false
			}
			revision = &parsed
		default:
			return "", "", nil, false
		}
	}
	if disposition != lifecycle.DispositionInFlight && disposition != lifecycle.DispositionDeferred && disposition != lifecycle.DispositionTerminal {
		return "", "", nil, false
	}
	return disposition, reason, revision, reason != ""
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
	return writeError(stdout, "usage", "Usage: akagent task <create|external|checkpoint|resource|execution|disposition|list|inspect|publish|finish|archive|reconcile>", false, "Run `akagent task list`")
}

func taskListUsage(stdout io.Writer) int {
	return writeError(stdout, "usage", "Usage: akagent task list [keyword] [--all] [--view <in-flight|attention|maintenance|deferred|history>] [--repository <name>] [--worktree <path>] [--format <toon|human|json>]", false, "Use a bounded inventory view; `--all` retains the historical compatibility view")
}

func validTaskListView(view string) bool {
	switch view {
	case "in-flight", "attention", "maintenance", "deferred", "history":
		return true
	default:
		return false
	}
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
	}
	return writeError(stdout, category, message, retryable, recovery)
}
