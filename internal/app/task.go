package app

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/akofink/akagent-cli/internal/lifecycle"
	"github.com/akofink/akagent-cli/internal/store"
	"github.com/google/uuid"
)

type externalObservationView struct {
	Source       string    `json:"source"`
	ObservedAt   time.Time `json:"observed_at"`
	HostID       string    `json:"host_id"`
	BootID       string    `json:"boot_id"`
	ProcessState string    `json:"process_state,omitempty"`
	Result       string    `json:"result,omitempty"`
	Detail       string    `json:"detail,omitempty"`
}

type externalCompletionView struct {
	Contract   string    `json:"contract"`
	Result     string    `json:"result"`
	CallerID   string    `json:"caller_id"`
	DeclaredAt time.Time `json:"declared_at"`
}

func viewExternalObservation(observation store.ExternalObservation) externalObservationView {
	return externalObservationView{
		Source:       observation.Source,
		ObservedAt:   observation.ObservedAt,
		HostID:       observation.HostID,
		BootID:       observation.BootID,
		ProcessState: observation.ProcessState,
		Result:       observation.Result,
		Detail:       observation.Detail,
	}
}

func viewExternalObservations(observations []store.ExternalObservation) []externalObservationView {
	if len(observations) == 0 {
		return nil
	}
	views := make([]externalObservationView, 0, len(observations))
	for _, observation := range observations {
		views = append(views, viewExternalObservation(observation))
	}
	return views
}

func viewExternalCompletion(completion *store.ExternalCompletion) *externalCompletionView {
	if completion == nil {
		return nil
	}
	return &externalCompletionView{
		Contract:   completion.Contract,
		Result:     completion.Result,
		CallerID:   completion.CallerID,
		DeclaredAt: completion.DeclaredAt,
	}
}

type resourceView struct {
	ID                     string            `json:"id"`
	Provenance             string            `json:"provenance,omitempty"`
	CallerID               string            `json:"caller_id,omitempty"`
	Revision               uint64            `json:"revision,omitempty"`
	Repository             string            `json:"repository"`
	Branch                 string            `json:"branch,omitempty"`
	BaseRevision           string            `json:"base_revision,omitempty"`
	WorktreePath           string            `json:"worktree_path,omitempty"`
	Head                   string            `json:"head,omitempty"`
	Committed              bool              `json:"committed"`
	Dirty                  bool              `json:"dirty"`
	Untracked              bool              `json:"untracked"`
	RecoveryDebt           string            `json:"recovery_debt,omitempty"`
	ArchiveState           string            `json:"archive_state,omitempty"`
	CleanupState           string            `json:"cleanup_state,omitempty"`
	WorktreeCleanupState   string            `json:"worktree_cleanup_state,omitempty"`
	CredentialCleanupState string            `json:"credential_cleanup_state,omitempty"`
	CleanupDebt            bool              `json:"cleanup_debt,omitempty"`
	Metadata               map[string]string `json:"metadata,omitempty"`
	ExternalURLs           []string          `json:"external_urls,omitempty"`
}

type resourceListItem struct {
	ID                     string            `json:"id"`
	Provenance             string            `json:"provenance,omitempty"`
	CallerID               string            `json:"caller_id,omitempty"`
	Revision               uint64            `json:"revision,omitempty"`
	Repository             string            `json:"repository"`
	Branch                 string            `json:"branch,omitempty"`
	BaseRevision           string            `json:"base_revision,omitempty"`
	WorktreePath           string            `json:"worktree_path,omitempty"`
	Head                   string            `json:"head,omitempty"`
	Committed              bool              `json:"committed"`
	Dirty                  bool              `json:"dirty"`
	Untracked              bool              `json:"untracked"`
	RecoveryDebt           string            `json:"recovery_debt,omitempty"`
	ArchiveState           string            `json:"archive_state,omitempty"`
	CleanupState           string            `json:"cleanup_state,omitempty"`
	WorktreeCleanupState   string            `json:"worktree_cleanup_state,omitempty"`
	CredentialCleanupState string            `json:"credential_cleanup_state,omitempty"`
	CleanupDebt            bool              `json:"cleanup_debt,omitempty"`
	Metadata               map[string]string `json:"metadata,omitempty"`
	ExternalURLs           []string          `json:"external_urls,omitempty"`
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
	ID                   string                    `json:"id"`
	Provenance           string                    `json:"provenance,omitempty"`
	CallerID             string                    `json:"caller_id,omitempty"`
	Revision             uint64                    `json:"revision,omitempty"`
	PredecessorID        string                    `json:"predecessor_id,omitempty"`
	ExternalObservations []externalObservationView `json:"external_observations,omitempty"`
	ExternalCompletion   *externalCompletionView   `json:"external_completion,omitempty"`
	TaskID               string                    `json:"task_id"`
	Label                string                    `json:"label"`
	Target               string                    `json:"target"`
	Command              string                    `json:"command,omitempty"`
	Requirements         string                    `json:"requirements,omitempty"`
	ResourceID           string                    `json:"resource_id,omitempty"`
	WorkingDirectory     string                    `json:"working_directory,omitempty"`
	Status               string                    `json:"status"`
	Condition            string                    `json:"condition,omitempty"`
	Reason               string                    `json:"reason,omitempty"`
	Activity             string                    `json:"activity,omitempty"`
	Result               string                    `json:"result,omitempty"`
	TmuxWindow           string                    `json:"tmux_window,omitempty"`
	ProcessPID           int                       `json:"process_pid,omitempty"`
	Observation          string                    `json:"observation,omitempty"`
	RecoveryDebt         string                    `json:"recovery_debt,omitempty"`
	ArchiveState         string                    `json:"archive_state,omitempty"`
	SessionReferences    []sessionReferenceView    `json:"session_references,omitempty"`
}

type executionListView struct {
	Executions []executionView `json:"executions"`
	Total      int             `json:"total"`
}
type executionDetailView struct {
	Execution         executionView          `json:"execution"`
	SessionReferences []sessionReferenceView `json:"session_references,omitempty"`
}

type checkpointView struct {
	TaskID                      string                              `json:"task_id"`
	Revision                    uint64                              `json:"revision"`
	IdempotencyKey              string                              `json:"idempotency_key"`
	AcknowledgedAt              time.Time                           `json:"acknowledged_at"`
	AuditDebt                   []store.CheckpointAuditDebt         `json:"audit_debt,omitempty"`
	TaskKind                    string                              `json:"task_kind"`
	CompletionContract          string                              `json:"completion_contract"`
	NextAction                  string                              `json:"next_action"`
	ContextReferences           []store.ContextReference            `json:"context_references,omitempty"`
	ResourceReferences          []store.CheckpointResourceReference `json:"resource_references,omitempty"`
	SessionReferences           []store.CheckpointSessionReference  `json:"session_references,omitempty"`
	PreviousAttemptReferences   []string                            `json:"previous_attempt_references,omitempty"`
	Verification                *store.CheckpointVerification       `json:"verification,omitempty"`
	UncertainExternalOperations []store.UncertainExternalOperation  `json:"uncertain_external_operations,omitempty"`
}

type checkpointDetailView struct {
	Available  bool            `json:"available"`
	State      string          `json:"state"`
	Reason     string          `json:"reason,omitempty"`
	Checkpoint *checkpointView `json:"checkpoint,omitempty"`
}

type executionEvidenceSummaryView struct {
	TaskID        string `json:"task_id"`
	ExecutionID   string `json:"execution_id"`
	State         string `json:"state"`
	EvidenceClass string `json:"evidence_class"`
	Reason        string `json:"reason"`
}

type executionEvidenceListItem struct {
	CaptureID         string   `json:"capture_id"`
	SourceKind        string   `json:"source_kind"`
	Provider          string   `json:"provider"`
	ProviderSessionID string   `json:"provider_session_id"`
	State             string   `json:"state"`
	EvidenceClass     string   `json:"evidence_class"`
	Coverage          []string `json:"coverage"`
	RetentionClass    string   `json:"retention_class"`
}

type executionEvidenceListView struct {
	Evidence executionEvidenceSummaryView `json:"evidence"`
	Captures []executionEvidenceListItem  `json:"captures"`
	Total    int                          `json:"total"`
}

type executionEvidenceDetailItem struct {
	CaptureID         string   `json:"capture_id"`
	ExecutionID       string   `json:"execution_id"`
	SourceKind        string   `json:"source_kind"`
	Provider          string   `json:"provider"`
	ProviderSessionID string   `json:"provider_session_id"`
	State             string   `json:"state"`
	EvidenceClass     string   `json:"evidence_class"`
	Coverage          []string `json:"coverage"`
	ArtifactReference string   `json:"artifact_reference,omitempty"`
	ArtifactState     string   `json:"artifact_state"`
	RedactionPolicy   string   `json:"redaction_policy"`
	RetentionClass    string   `json:"retention_class"`
	ErrorCategory     string   `json:"error_category,omitempty"`
	Recovery          string   `json:"recovery,omitempty"`
}

type executionEvidenceDetailView struct {
	Evidence executionEvidenceDetailItem `json:"evidence"`
}

type taskView struct {
	ID                     string                  `json:"id"`
	Provenance             string                  `json:"provenance,omitempty"`
	CallerID               string                  `json:"caller_id,omitempty"`
	Revision               uint64                  `json:"revision,omitempty"`
	ExternalCompletion     *externalCompletionView `json:"external_completion,omitempty"`
	Title                  string                  `json:"title"`
	Status                 string                  `json:"status"`
	Worker                 string                  `json:"worker"`
	Branch                 string                  `json:"branch,omitempty"`
	BaseRevision           string                  `json:"base_revision,omitempty"`
	WorktreePath           string                  `json:"worktree_path,omitempty"`
	Condition              string                  `json:"condition,omitempty"`
	Reason                 string                  `json:"reason,omitempty"`
	Activity               string                  `json:"activity,omitempty"`
	Result                 string                  `json:"result,omitempty"`
	Disposition            string                  `json:"disposition,omitempty"`
	DispositionReason      string                  `json:"disposition_reason,omitempty"`
	DispositionRevision    uint64                  `json:"disposition_revision,omitempty"`
	Committed              bool                    `json:"committed,omitempty"`
	Dirty                  bool                    `json:"dirty,omitempty"`
	Untracked              bool                    `json:"untracked,omitempty"`
	RecoveryDebt           string                  `json:"recovery_debt,omitempty"`
	Warnings               string                  `json:"warnings,omitempty"`
	ArchiveState           string                  `json:"archive_state,omitempty"`
	CleanupState           string                  `json:"cleanup_state,omitempty"`
	WorktreeCleanupState   string                  `json:"worktree_cleanup_state,omitempty"`
	CredentialCleanupState string                  `json:"credential_cleanup_state,omitempty"`
	CleanupDebt            bool                    `json:"cleanup_debt,omitempty"`
	Agent                  string                  `json:"agent,omitempty"`
	AgentCommand           string                  `json:"agent_command,omitempty"`
	PromptReference        string                  `json:"prompt_reference,omitempty"`
	WorkingContext         string                  `json:"working_context,omitempty"`
	Execution              string                  `json:"execution,omitempty"`
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
	if len(args) == 0 {
		return writeError(stdout, "usage", "Usage: akagent task <create|external|checkpoint|resource|execution|disposition|list|inspect|publish|finish|archive|reconcile>", false, "Run `akagent task list`")
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

func taskExecutionCommand(args []string, stdout io.Writer) int {
	if len(args) == 0 {
		return writeError(stdout, "usage", "Usage: akagent task execution <create|observe|finish|list|inspect|session|evidence|publish|archive|reconcile>", false, "Run `akagent task execution list <task-id>`")
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
		return writeError(stdout, "usage", "Usage: akagent task execution <create|observe|finish|list|inspect|session|evidence|publish|archive|reconcile>", false, "Run `akagent task execution list <task-id>`")
	}
}

type lifecycleExecutionResult struct {
	Execution store.Execution
	Err       error
}

func parseExternalCompletion(args []string) (callerID, operationID, contract, resultValue string, expectedRevision uint64, ok bool) {
	seen := map[string]bool{}
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
			if !ok {
				return "", "", "", "", 0, false
			}
		default:
			return "", "", "", "", 0, false
		}
	}
	return callerID, operationID, contract, resultValue, expectedRevision, ok && callerID != "" && operationID != "" && contract != "" && resultValue != "" && expectedRevision > 0
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

func taskResourceCommand(args []string, stdout io.Writer) int {
	if len(args) == 0 {
		return writeError(stdout, "usage", "Usage: akagent task resource <create|list|inspect|update|archive>", false, "Run `akagent task resource list <task-id>`")
	}
	if args[0] == "clean" {
		return removedCommandError(stdout, "task resource clean", "External tools own worktree and credential cleanup; retain cleanup debt in the record")
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
		resource, _, err := manager.CreateResourceRecord(args[1], request)
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
		archived, err := manager.ArchiveRecord(args[1], args[2], "")
		if err != nil {
			return lifecycleError(stdout, err)
		}
		resource, ok := archived.(store.Resource)
		if !ok {
			return writeError(stdout, "internal", "Resource archive returned an invalid record", false, "Retry the archive")
		}
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, resourceDetailView{Resource: viewResource(resource)})
	default:
		return writeError(stdout, "usage", "Usage: akagent task resource <create|list|inspect|update|archive>", false, "Run `akagent task resource list <task-id>`")
	}
}

func parseResourceCreate(args []string) (lifecycle.ResourceRequest, bool) {
	var request lifecycle.ResourceRequest
	metadata, urls, ok := parseResourceMetadata(args, &request.ID, &request.Repository, &request.Branch, &request.BaseRevision, &request.WorktreePath, &request.Head)
	request.Metadata, request.ExternalURLs = metadata, urls
	return request, ok && request.Repository != ""
}

func parseResourceUpdate(args []string) (lifecycle.ResourceUpdateRequest, bool) {
	var request lifecycle.ResourceUpdateRequest
	metadata, urls, ok := parseResourceMetadata(args, nil, nil, nil, nil, nil, nil)
	request.Metadata, request.ExternalURLs = metadata, urls
	return request, ok && (len(metadata) > 0 || len(urls) > 0)
}

func parseResourceMetadata(args []string, id, repository, branch, base, worktree, head *string) (map[string]string, []string, bool) {
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
		case "--head":
			if head == nil {
				return nil, nil, false
			}
			*head = value
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
func taskCheckpointCommand(args []string, stdout io.Writer, manager *lifecycle.Manager) int {
	if len(args) == 0 {
		return writeError(stdout, "usage", "Usage: akagent task checkpoint <write|inspect> <task-id> ...", false, "Inspect a task checkpoint or write the next acknowledged revision")
	}
	switch args[0] {
	case "write":
		if len(args) < 2 {
			return writeError(stdout, "usage", "Usage: akagent task checkpoint write <task-id> --idempotency-key <key> --expected-revision <revision> --task-kind <kind> --completion-contract <contract> --next-action <action> [--context <purpose=reference>] [--resource-reference <id>] [--session-reference <execution-id=tool:session-id>] [--previous-attempt-reference <reference>] [--verification-scope <scope>] [--verification-revision <revision>] [--verification-result <result>] [--verification-time <RFC3339>] [--uncertain-operation <key=operation>]", false, "Use a new idempotency key and the last acknowledged revision")
		}
		request, ok := parseCheckpointWrite(args[2:])
		if !ok {
			return writeError(stdout, "usage", "Usage: akagent task checkpoint write <task-id> --idempotency-key <key> --expected-revision <revision> --task-kind <kind> --completion-contract <contract> --next-action <action> [--context <purpose=reference>] [--resource-reference <id>] [--session-reference <execution-id=tool:session-id>] [--previous-attempt-reference <reference>] [--verification-scope <scope>] [--verification-revision <revision>] [--verification-result <result>] [--verification-time <RFC3339>] [--uncertain-operation <key=operation>]", false, "Provide bounded, non-secret checkpoint fields")
		}
		checkpoint, err := manager.WriteCheckpoint(args[1], request)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, checkpointDetailView{Available: true, State: "acknowledged", Checkpoint: viewCheckpoint(&checkpoint)})
	case "inspect":
		if len(args) != 2 {
			return writeError(stdout, "usage", "Usage: akagent task checkpoint inspect <task-id>", false, "Provide a task ID")
		}
		inspection, err := manager.InspectCheckpoint(args[1])
		if err != nil {
			return lifecycleError(stdout, err)
		}
		return write(stdout, checkpointDetailView{Available: inspection.Available, State: inspection.State, Reason: inspection.Reason, Checkpoint: viewCheckpoint(inspection.Checkpoint)})
	default:
		return writeError(stdout, "usage", "Usage: akagent task checkpoint <write|inspect> <task-id> ...", false, "Inspect a task checkpoint or write the next acknowledged revision")
	}
}

type checkpointCLIRequest struct {
	Request              lifecycle.CheckpointRequest
	expectedRevisionSet  bool
	verificationScope    string
	verificationRevision string
	verificationResult   string
	verificationTime     time.Time
	verificationSet      bool
}

func parseCheckpointWrite(args []string) (lifecycle.CheckpointRequest, bool) {
	var parsed checkpointCLIRequest
	for len(args) > 0 {
		if len(args) < 2 {
			return lifecycle.CheckpointRequest{}, false
		}
		flag, value := args[0], args[1]
		args = args[2:]
		switch flag {
		case "--idempotency-key", "--key":
			parsed.Request.IdempotencyKey = value
		case "--expected-revision":
			revision, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				return lifecycle.CheckpointRequest{}, false
			}
			parsed.Request.ExpectedRevision = revision
			parsed.expectedRevisionSet = true
		case "--task-kind":
			parsed.Request.Checkpoint.TaskKind = value
		case "--completion-contract":
			parsed.Request.Checkpoint.CompletionContract = value
		case "--next-action":
			parsed.Request.Checkpoint.NextAction = value
		case "--context", "--context-reference":
			purpose, reference, ok := strings.Cut(value, "=")
			if !ok || purpose == "" || reference == "" {
				return lifecycle.CheckpointRequest{}, false
			}
			parsed.Request.Checkpoint.ContextReferences = append(parsed.Request.Checkpoint.ContextReferences, store.ContextReference{Purpose: purpose, Reference: reference})
		case "--resource-reference":
			parsed.Request.Checkpoint.ResourceReferences = append(parsed.Request.Checkpoint.ResourceReferences, store.CheckpointResourceReference{ResourceID: value})
		case "--session-reference":
			executionID, session, ok := strings.Cut(value, "=")
			if !ok {
				return lifecycle.CheckpointRequest{}, false
			}
			parts := strings.SplitN(session, ":", 2)
			if executionID == "" || len(parts) != 2 || parts[0] == "" || parts[1] == "" {
				return lifecycle.CheckpointRequest{}, false
			}
			parsed.Request.Checkpoint.SessionReferences = append(parsed.Request.Checkpoint.SessionReferences, store.CheckpointSessionReference{ExecutionID: executionID, Tool: parts[0], SessionID: parts[1]})
		case "--previous-attempt-reference":
			parsed.Request.Checkpoint.PreviousAttemptReferences = append(parsed.Request.Checkpoint.PreviousAttemptReferences, value)
		case "--verification-scope":
			parsed.verificationScope, parsed.verificationSet = value, true
		case "--verification-revision":
			parsed.verificationRevision, parsed.verificationSet = value, true
		case "--verification-result":
			parsed.verificationResult, parsed.verificationSet = value, true
		case "--verification-time":
			verifiedAt, err := time.Parse(time.RFC3339Nano, value)
			if err != nil {
				return lifecycle.CheckpointRequest{}, false
			}
			parsed.verificationTime, parsed.verificationSet = verifiedAt.UTC(), true
		case "--uncertain-operation":
			key, operation, ok := strings.Cut(value, "=")
			if !ok || key == "" || operation == "" {
				return lifecycle.CheckpointRequest{}, false
			}
			parsed.Request.Checkpoint.UncertainExternalOperations = append(parsed.Request.Checkpoint.UncertainExternalOperations, store.UncertainExternalOperation{ReferenceKey: key, Operation: operation, Verify: "verify the external system before replaying"})
		default:
			return lifecycle.CheckpointRequest{}, false
		}
	}
	if parsed.verificationSet {
		parsed.Request.Checkpoint.Verification = &store.CheckpointVerification{Scope: parsed.verificationScope, Revision: parsed.verificationRevision, Result: parsed.verificationResult, VerifiedAt: parsed.verificationTime}
	}
	return parsed.Request, parsed.expectedRevisionSet && parsed.Request.IdempotencyKey != "" && parsed.Request.Checkpoint.TaskKind != "" && parsed.Request.Checkpoint.CompletionContract != "" && parsed.Request.Checkpoint.NextAction != ""
}

func viewCheckpoint(checkpoint *store.Checkpoint) *checkpointView {
	if checkpoint == nil {
		return nil
	}
	return &checkpointView{
		TaskID:                      checkpoint.TaskID,
		Revision:                    checkpoint.Revision,
		IdempotencyKey:              checkpoint.IdempotencyKey,
		AcknowledgedAt:              checkpoint.AcknowledgedAt,
		AuditDebt:                   checkpoint.AuditDebt,
		TaskKind:                    checkpoint.TaskKind,
		CompletionContract:          checkpoint.CompletionContract,
		NextAction:                  checkpoint.NextAction,
		ContextReferences:           checkpoint.ContextReferences,
		ResourceReferences:          checkpoint.ResourceReferences,
		SessionReferences:           checkpoint.SessionReferences,
		PreviousAttemptReferences:   checkpoint.PreviousAttemptReferences,
		Verification:                checkpoint.Verification,
		UncertainExternalOperations: checkpoint.UncertainExternalOperations,
	}
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

func includeTaskInView(manifest store.Manifest, resources []store.Resource, requested string) bool {
	if requested == "" {
		requested = "in-flight"
	}
	disposition := inventoryDisposition(manifest)
	switch requested {
	case "in-flight":
		return acceptedInFlight(disposition)
	case "deferred":
		return disposition == lifecycle.DispositionDeferred
	case "history":
		return disposition == lifecycle.DispositionTerminal
	case "maintenance":
		return hasMaintenanceDebt(manifest, resources)
	case "attention":
		return acceptedInFlight(disposition) && hasAttention(manifest)
	default:
		return false
	}
}

func inventoryDisposition(manifest store.Manifest) lifecycle.WorkDisposition {
	disposition := lifecycle.WorkDispositionOf(manifest)
	if archiveCompleteFinished(manifest) && disposition != lifecycle.DispositionDeferred {
		return lifecycle.DispositionTerminal
	}
	return disposition
}

func archiveCompleteFinished(manifest store.Manifest) bool {
	if manifest.ArchiveState != "complete" {
		return false
	}
	return manifest.Lifecycle == "finished" || manifest.ExternalCompletion != nil
}

func acceptedInFlight(disposition lifecycle.WorkDisposition) bool {
	return disposition != lifecycle.DispositionDeferred && disposition != lifecycle.DispositionTerminal
}

func hasMaintenanceDebt(manifest store.Manifest, resources []store.Resource) bool {
	if strings.TrimSpace(manifest.RecoveryDebt) != "" || manifest.CleanupDebt || incompleteTaskState(manifest.ArchiveState) || incompleteTaskState(manifest.CleanupState) || incompleteTaskState(manifest.WorktreeCleanupState) || incompleteTaskState(manifest.CredentialCleanupState) {
		return true
	}
	for _, resource := range resources {
		if strings.TrimSpace(resource.RecoveryDebt) != "" || resource.CleanupDebt || incompleteTaskState(resource.ArchiveState) || incompleteTaskState(resource.CleanupState) || incompleteTaskState(resource.WorktreeCleanupState) || incompleteTaskState(resource.CredentialCleanupState) {
			return true
		}
	}
	return false
}

func incompleteTaskState(value string) bool {
	return value != "" && value != "none" && value != "complete"
}

func hasAttention(manifest store.Manifest) bool {
	if manifest.Condition == "waiting" || manifest.Condition == "blocked" || manifest.Condition == "failed" || strings.TrimSpace(manifest.RecoveryDebt) != "" {
		return true
	}
	return status(manifest) == "unknown"
}

func viewResource(resource store.Resource) resourceView {
	return resourceView{ID: resource.ID, Provenance: resource.Provenance, CallerID: resource.CallerID, Revision: resource.Revision, Repository: resource.Repository, Branch: resource.Branch, BaseRevision: resource.BaseRevision, WorktreePath: resource.WorktreePath, Head: resource.Git.Head, Committed: resource.Git.Committed, Dirty: resource.Git.Dirty, Untracked: resource.Git.Untracked, RecoveryDebt: resource.RecoveryDebt, ArchiveState: taskState(resource.ArchiveState), CleanupState: taskState(resource.CleanupState), WorktreeCleanupState: taskState(resource.WorktreeCleanupState), CredentialCleanupState: taskState(resource.CredentialCleanupState), CleanupDebt: resource.CleanupDebt, Metadata: resource.Metadata, ExternalURLs: resource.ExternalURLs}
}

func viewResourceList(resource store.Resource) resourceListItem {
	metadata := make(map[string]string, len(resource.Metadata))
	for key, value := range resource.Metadata {
		metadata[key] = value
	}
	urls := append([]string(nil), resource.ExternalURLs...)
	return resourceListItem{ID: resource.ID, Provenance: resource.Provenance, CallerID: resource.CallerID, Revision: resource.Revision, Repository: resource.Repository, Branch: resource.Branch, BaseRevision: resource.BaseRevision, WorktreePath: resource.WorktreePath, Head: resource.Git.Head, Committed: resource.Git.Committed, Dirty: resource.Git.Dirty, Untracked: resource.Git.Untracked, RecoveryDebt: resource.RecoveryDebt, ArchiveState: taskState(resource.ArchiveState), CleanupState: taskState(resource.CleanupState), WorktreeCleanupState: taskState(resource.WorktreeCleanupState), CredentialCleanupState: taskState(resource.CredentialCleanupState), CleanupDebt: resource.CleanupDebt, Metadata: metadata, ExternalURLs: urls}
}

func viewExecution(execution store.Execution, manager *lifecycle.Manager) executionView {
	executionStatus := lifecycle.ExecutionStatus(execution, time.Now().UTC(), lifecycle.DefaultHeartbeatTimeout)
	if execution.Provenance == store.ProvenanceExternal && execution.ExternalCompletion != nil {
		executionStatus = "finished"
	}
	sessionReferences := make([]sessionReferenceView, 0, len(execution.SessionReferences))
	for _, reference := range execution.SessionReferences {
		sessionReferences = append(sessionReferences, sessionReferenceView{Tool: reference.Tool, SessionID: reference.SessionID, ReferencePath: reference.ReferencePath})
	}
	return executionView{ID: execution.ID, Provenance: execution.Provenance, CallerID: execution.CallerID, Revision: execution.Revision, PredecessorID: execution.PredecessorID, ExternalObservations: viewExternalObservations(execution.ExternalObservations), ExternalCompletion: viewExternalCompletion(execution.ExternalCompletion), TaskID: execution.TaskID, Label: execution.Label, Target: execution.Target, Command: execution.Command, Requirements: execution.Requirements, ResourceID: execution.ResourceID, WorkingDirectory: execution.WorkingDirectory, Status: executionStatus, Condition: execution.Condition, Reason: execution.Reason, Activity: execution.Activity, Result: execution.Result, TmuxWindow: execution.TmuxWindow, ProcessPID: execution.ProcessPID, Observation: execution.Observation, RecoveryDebt: execution.RecoveryDebt, ArchiveState: taskState(execution.ArchiveState), SessionReferences: sessionReferences}
}

func executionDetail(execution store.Execution, manager *lifecycle.Manager) executionDetailView {
	references := make([]sessionReferenceView, 0, len(execution.SessionReferences))
	for _, reference := range execution.SessionReferences {
		references = append(references, sessionReferenceView{Tool: reference.Tool, SessionID: reference.SessionID, ReferencePath: reference.ReferencePath})
	}
	return executionDetailView{Execution: viewExecution(execution, manager), SessionReferences: references}
}

func executionEvidenceList(summary lifecycle.EvidenceSummary, captures []lifecycle.EvidenceCapture) executionEvidenceListView {
	items := make([]executionEvidenceListItem, 0, len(captures))
	for _, capture := range captures {
		items = append(items, executionEvidenceListItem{CaptureID: capture.CaptureID, SourceKind: capture.SourceKind, Provider: capture.Provider, ProviderSessionID: capture.ProviderSessionID, State: capture.State, EvidenceClass: capture.EvidenceClass, Coverage: append([]string(nil), capture.Coverage...), RetentionClass: capture.RetentionClass})
	}
	return executionEvidenceListView{
		Evidence: executionEvidenceSummaryView{TaskID: summary.TaskID, ExecutionID: summary.ExecutionID, State: summary.State, EvidenceClass: summary.EvidenceClass, Reason: summary.Reason},
		Captures: items,
		Total:    len(items),
	}
}

func executionEvidenceDetail(capture lifecycle.EvidenceCapture) executionEvidenceDetailView {
	return executionEvidenceDetailView{Evidence: executionEvidenceDetailItem{CaptureID: capture.CaptureID, ExecutionID: capture.ExecutionID, SourceKind: capture.SourceKind, Provider: capture.Provider, ProviderSessionID: capture.ProviderSessionID, State: capture.State, EvidenceClass: capture.EvidenceClass, Coverage: append([]string(nil), capture.Coverage...), ArtifactReference: capture.ArtifactReference, ArtifactState: capture.ArtifactState, RedactionPolicy: capture.RedactionPolicy, RetentionClass: capture.RetentionClass, ErrorCategory: capture.ErrorCategory, Recovery: capture.Recovery}}
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
	if len(resourceViews) == 0 {
		resourceViews = nil
	}
	if len(executionViews) == 0 {
		executionViews = nil
	}
	return taskDetailView{Task: view(id, manifest), Resources: resourceViews, Executions: executionViews}, nil
}

func view(id string, manifest store.Manifest) taskView {
	taskStatus := status(manifest)
	if manifest.Provenance == store.ProvenanceExternal && manifest.ExternalCompletion != nil {
		taskStatus = "finished"
	}
	result := taskView{ID: id, Provenance: manifest.Provenance, CallerID: manifest.CallerID, Revision: manifest.Revision, ExternalCompletion: viewExternalCompletion(manifest.ExternalCompletion), Title: manifest.Title, Status: taskStatus, Worker: manifest.Worker, Branch: manifest.Branch, BaseRevision: manifest.BaseRevision, WorktreePath: manifest.WorktreePath, Condition: manifest.Condition, Reason: manifest.Reason, Activity: manifest.Activity, Result: manifest.Result, Committed: manifest.Committed, Dirty: manifest.Dirty, Untracked: manifest.Untracked, RecoveryDebt: manifest.RecoveryDebt, Warnings: manifest.Warnings, ArchiveState: taskState(manifest.ArchiveState), CleanupState: taskState(manifest.CleanupState), WorktreeCleanupState: taskState(manifest.WorktreeCleanupState), CredentialCleanupState: taskState(manifest.CredentialCleanupState), CleanupDebt: manifest.CleanupDebt}
	effectiveDisposition := lifecycle.WorkDispositionOf(manifest)
	if effectiveDisposition != lifecycle.DispositionInFlight || (manifest.Disposition != "" && manifest.DispositionRevision > 0) {
		result.Disposition = string(effectiveDisposition)
		result.DispositionReason = manifest.DispositionReason
		result.DispositionRevision = manifest.DispositionRevision
	}
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
