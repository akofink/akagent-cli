package app

import (
	"strings"
	"time"

	"github.com/akofink/akagent-cli/internal/lifecycle"
	"github.com/akofink/akagent-cli/internal/store"
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
	Revision             uint64                    `json:"revision"`
	PredecessorID        string                    `json:"predecessor_id,omitempty"`
	ExternalObservations []externalObservationView `json:"external_observations,omitempty"`
	ExternalCompletion   *externalCompletionView   `json:"external_completion,omitempty"`
	HandoffDisposition   *store.HandoffDisposition `json:"handoff_disposition,omitempty"`
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
	return executionView{ID: execution.ID, Provenance: execution.Provenance, CallerID: execution.CallerID, Revision: execution.Revision, PredecessorID: execution.PredecessorID, ExternalObservations: viewExternalObservations(execution.ExternalObservations), ExternalCompletion: viewExternalCompletion(execution.ExternalCompletion), HandoffDisposition: execution.HandoffDisposition, TaskID: execution.TaskID, Label: execution.Label, Target: execution.Target, Command: execution.Command, Requirements: execution.Requirements, ResourceID: execution.ResourceID, WorkingDirectory: execution.WorkingDirectory, Status: executionStatus, Condition: execution.Condition, Reason: execution.Reason, Activity: execution.Activity, Result: execution.Result, TmuxWindow: execution.TmuxWindow, ProcessPID: execution.ProcessPID, Observation: execution.Observation, RecoveryDebt: execution.RecoveryDebt, ArchiveState: taskState(execution.ArchiveState), SessionReferences: sessionReferences}
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
