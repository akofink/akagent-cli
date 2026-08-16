package lifecycle

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/akofink/akagent-cli/internal/store"
	"github.com/google/uuid"
)

// ExecutionRequest describes an optional tool-neutral execution. Creating one
// records intent only and never creates a tmux window or starts a process.
type ExecutionRequest struct {
	ID               string
	Label            string
	Target           string
	Command          string
	Arguments        []string
	ResourceID       string
	WorkingDirectory string
}

func (m *Manager) CreateExecution(taskID string, request ExecutionRequest) (store.Execution, bool, error) {
	if taskID == "" || request.Target == "" {
		return store.Execution{}, false, fmt.Errorf("task ID and execution target are required")
	}
	if request.ID == "" {
		id, err := uuid.NewV7()
		if err != nil {
			return store.Execution{}, false, errors.New("failed to generate an execution ID")
		}
		request.ID = id.String()
	}
	if request.Label == "" {
		request.Label = request.Target
	}
	if strings.ContainsAny(request.Label, "\r\n") || strings.TrimSpace(request.Label) == "" {
		return store.Execution{}, false, fmt.Errorf("execution label must be a non-empty single line")
	}
	manifest, err := m.manifest(taskID)
	if err != nil {
		return store.Execution{}, false, err
	}
	if manifest.Lifecycle == "stopped" || manifest.Lifecycle == "finished" {
		return store.Execution{}, false, fmt.Errorf("cannot add an execution to a %s task", manifest.Lifecycle)
	}
	if request.ResourceID != "" {
		resource, resourceErr := m.InspectResource(taskID, request.ResourceID)
		if resourceErr != nil {
			return store.Execution{}, false, resourceErr
		}
		if request.WorkingDirectory == "" {
			request.WorkingDirectory = resource.WorktreePath
		} else {
			workingDirectory, pathErr := filepath.Abs(request.WorkingDirectory)
			resourceDirectory, resourceErr := filepath.Abs(resource.WorktreePath)
			if pathErr != nil || resourceErr != nil || filepath.Clean(workingDirectory) != filepath.Clean(resourceDirectory) {
				return store.Execution{}, false, &store.Error{Kind: store.KindConflict, Message: "execution working directory does not match its selected resource", Recovery: "Retry with the resource worktree path"}
			}
			request.WorkingDirectory = workingDirectory
		}
	}
	execution := store.Execution{ID: request.ID, TaskID: taskID, Label: request.Label, Target: request.Target, Command: request.Command, Arguments: append([]string(nil), request.Arguments...), ResourceID: request.ResourceID, WorkingDirectory: request.WorkingDirectory, Lifecycle: "created", Condition: "none", HeartbeatAt: m.now()}
	created, existing, err := m.Store.CreateExecution(taskID, execution)
	if err != nil {
		return store.Execution{}, false, err
	}
	if _, err := m.Store.UpdateManifest(taskID, func(task *store.Manifest) error {
		task.ExecutionIDs = appendResourceID(task.ExecutionIDs, request.ID)
		return nil
	}); err != nil {
		return store.Execution{}, false, err
	}
	if created {
		if _, err := m.Store.AppendExecutionEvent(taskID, request.ID, store.Event{Operation: "create", Outcome: "intent"}); err != nil {
			return store.Execution{}, false, err
		}
	}
	return existing, created, nil
}

func (m *Manager) ListExecutions(taskID string) ([]store.Execution, error) {
	if err := m.ensureLegacyExecution(taskID); err != nil {
		return nil, err
	}
	if _, err := m.manifest(taskID); err != nil {
		return nil, err
	}
	ids, err := m.Store.ExecutionIDs(taskID)
	if err != nil {
		return nil, err
	}
	result := make([]store.Execution, 0, len(ids))
	for _, id := range ids {
		execution, err := m.Store.ReadExecution(taskID, id)
		if err != nil {
			return nil, err
		}
		result = append(result, execution)
	}
	return result, nil
}

func (m *Manager) InspectExecution(taskID, executionID string) (store.Execution, error) {
	if err := m.ensureLegacyExecution(taskID); err != nil {
		return store.Execution{}, err
	}
	if executionID == "" {
		ids, err := m.Store.ExecutionIDs(taskID)
		if err != nil {
			return store.Execution{}, err
		}
		if len(ids) != 1 {
			return store.Execution{}, fmt.Errorf("execution ID is required when a task has multiple executions")
		}
		executionID = ids[0]
	}
	return m.Store.ReadExecution(taskID, executionID)
}

// LaunchExecutionRecord starts a previously created execution. It does not
// inspect, lock, archive, or mutate any task resource after launch selection.
func (m *Manager) LaunchExecutionRecord(taskID, executionID string) (store.Execution, error) {
	execution, err := m.InspectExecution(taskID, executionID)
	if err != nil {
		return store.Execution{}, err
	}
	if execution.Lifecycle == "running" {
		return execution, nil
	}
	if execution.Lifecycle == "stopped" || execution.Lifecycle == "finished" {
		return store.Execution{}, fmt.Errorf("execution cannot launch after it is %s", execution.Lifecycle)
	}
	if execution.Command == "" {
		return store.Execution{}, &store.Error{Kind: store.KindUsage, Message: "execution command is required before launch", Recovery: fmt.Sprintf("Create execution %s with --command", execution.ID)}
	}
	if execution.WorkingDirectory == "" {
		execution.WorkingDirectory = "."
	}
	execution.Lifecycle, execution.Observation, execution.HeartbeatAt = "starting", ObservationMissing, m.now()
	if err := m.Store.WriteExecution(taskID, execution); err != nil {
		return store.Execution{}, err
	}
	if _, err := m.Store.AppendExecutionEvent(taskID, execution.ID, store.Event{Operation: "launch", Outcome: "intent"}); err != nil {
		return store.Execution{}, err
	}
	var process TmuxProcess
	if tmux, ok := m.Tmux.(ExecutionTmux); ok {
		process, err = tmux.StartExecution(execution.ID, taskID, execution.Label, execution.WorkingDirectory, execution.Command, execution.Arguments)
	} else {
		// Compatibility for injected implementations from the task launch API.
		process, err = m.Tmux.Start(taskID, execution.Label)
	}
	if err != nil {
		execution.RecoveryDebt = addDebt(execution.RecoveryDebt, "launch_failed")
		execution.Observation = ObservationMissing
		_ = m.Store.WriteExecution(taskID, execution)
		m.publishExecutionState(execution)
		_, _ = m.Store.AppendExecutionEvent(taskID, execution.ID, store.Event{Operation: "launch", Outcome: "failed"})
		return execution, fmt.Errorf("start execution tmux window: %w", err)
	}
	execution.Lifecycle = "running"
	execution.TmuxWindow, execution.ProcessPID, execution.ProcessStartTime = process.WindowID, process.PID, process.StartTime
	execution.ObservedPID, execution.ObservedStartTime, execution.ProcessPane = process.PID, process.StartTime, process.PaneID
	execution.Observation, execution.ObservationAt = processState(process), m.now()
	if err := m.Store.WriteExecution(taskID, execution); err != nil {
		return store.Execution{}, err
	}
	if _, err := m.Store.AppendExecutionEvent(taskID, execution.ID, store.Event{Operation: "launch", Outcome: "succeeded"}); err != nil {
		return store.Execution{}, err
	}
	m.publishExecutionState(execution)
	return execution, nil
}

func (m *Manager) PublishExecution(taskID, executionID, condition, reason, activity string) (store.Execution, error) {
	if !validCondition(condition) {
		return store.Execution{}, fmt.Errorf("condition must be active, waiting, blocked, failed, or none")
	}
	var changed bool
	execution, err := m.Store.UpdateExecution(taskID, executionID, func(execution *store.Execution) error {
		changed = execution.Condition != condition || execution.Reason != reason || execution.Activity != activity
		execution.Condition, execution.Reason, execution.Activity, execution.HeartbeatAt = condition, reason, activity, m.now()
		return nil
	})
	if err != nil {
		return store.Execution{}, err
	}
	if changed {
		_, err = m.Store.AppendExecutionEvent(taskID, executionID, store.Event{Operation: "publish", Outcome: condition})
	}
	m.publishExecutionState(execution)
	return execution, err
}

func (m *Manager) StopExecution(taskID, executionID string) (store.Execution, error) {
	execution, err := m.InspectExecution(taskID, executionID)
	if err != nil {
		return store.Execution{}, err
	}
	if execution.Lifecycle == "stopped" || execution.Lifecycle == "finished" {
		m.publishExecutionState(execution)
		return execution, nil
	}
	if tmux, ok := m.Tmux.(ExecutionTmux); ok {
		if err := tmux.StopExecution(executionID, taskID); err != nil {
			return store.Execution{}, err
		}
	} else if err := m.Tmux.Stop(taskID); err != nil {
		return store.Execution{}, err
	}
	execution, err = m.Store.UpdateExecution(taskID, executionID, func(execution *store.Execution) error {
		execution.Lifecycle, execution.Condition = "stopped", "none"
		execution.Observation, execution.ObservationAt = ObservationMissing, m.now()
		execution.ObservedPID, execution.ObservedStartTime = 0, 0
		return nil
	})
	if err != nil {
		return store.Execution{}, err
	}
	_, err = m.Store.AppendExecutionEvent(taskID, executionID, store.Event{Operation: "stop", Outcome: "succeeded"})
	m.publishExecutionState(execution)
	return execution, err
}

func (m *Manager) AttachExecution(taskID, executionID string) error {
	execution, err := m.InspectExecution(taskID, executionID)
	if err != nil {
		return err
	}
	if execution.Lifecycle != "running" {
		return executionAttachError(taskID, executionID, "the execution is not running")
	}
	if execution.Observation != ObservationFresh || execution.ProcessPID <= 0 || execution.ProcessStartTime == 0 || execution.HeartbeatAt.IsZero() || m.now().Sub(execution.HeartbeatAt) > m.heartbeatTimeout() {
		return executionAttachError(taskID, executionID, "the execution observation is not fresh")
	}
	tmux, ok := m.Tmux.(ExecutionTmux)
	if !ok {
		return executionAttachError(taskID, executionID, "execution tmux verification is unavailable")
	}
	observation, err := tmux.ObserveExecution(executionID, taskID)
	if err != nil || !observation.Available {
		return executionAttachError(taskID, executionID, "tmux observation is unavailable")
	}
	if len(observation.Processes) != 1 {
		return executionAttachError(taskID, executionID, "the execution window observation is contradictory")
	}
	process := observation.Processes[0]
	if processState(process) != ObservationFresh || process.WindowID != execution.TmuxWindow || process.PaneID != execution.ProcessPane || process.PID != execution.ProcessPID || process.StartTime != execution.ProcessStartTime {
		return executionAttachError(taskID, executionID, "the execution process or window was replaced")
	}
	if err := tmux.AttachExecution(executionID, taskID, process.WindowID); err != nil {
		return fmt.Errorf("attach to verified execution window: %w", err)
	}
	return nil
}

func executionAttachError(taskID, executionID, reason string) error {
	return &store.Error{Kind: store.KindConflict, Message: fmt.Sprintf("Execution %s for task %s cannot be attached: %s", executionID, taskID, reason), Recovery: fmt.Sprintf("Run `akagent task execution reconcile %s %s` and retry", taskID, executionID)}
}

func (m *Manager) ArchiveExecution(taskID, executionID string) (store.Execution, error) {
	execution, err := m.InspectExecution(taskID, executionID)
	if err != nil {
		return store.Execution{}, err
	}
	if execution.ArchiveState == "complete" {
		if _, err := m.Store.ReadExecutionArchive(taskID, executionID); err == nil {
			m.publishExecutionState(execution)
			return execution, nil
		}
	}
	live, available, err := m.executionLive(taskID, executionID)
	if err != nil {
		return store.Execution{}, err
	}
	if !available {
		return store.Execution{}, errors.New("execution process observation is unavailable")
	}
	if live {
		return store.Execution{}, liveExecutionError("archiving")
	}
	if execution.Lifecycle != "stopped" && execution.Lifecycle != "finished" {
		return store.Execution{}, fmt.Errorf("execution must be stopped or finished before archiving")
	}
	execution.ArchiveState = "pending"
	if err := m.Store.WriteExecution(taskID, execution); err != nil {
		return store.Execution{}, err
	}
	if _, err := m.Store.AppendExecutionEvent(taskID, executionID, store.Event{Operation: "archive", Outcome: "intent"}); err != nil {
		return store.Execution{}, err
	}
	events, err := m.Store.ReadExecutionEvents(taskID, executionID)
	if err != nil {
		return store.Execution{}, err
	}
	terminal := ""
	var warnings []string
	if tmux, ok := m.Tmux.(ExecutionTmux); ok {
		terminal, err = tmux.CaptureExecution(executionID, taskID)
		if err != nil {
			warnings = append(warnings, "terminal history unavailable")
		}
	}
	archive := store.ExecutionArchive{TaskID: taskID, ExecutionID: executionID, CapturedAt: time.Now().UTC(), Execution: execution, Events: events, Terminal: terminal, Warnings: warnings}
	if err := m.Store.WriteExecutionArchive(taskID, executionID, archive); err != nil {
		execution.ArchiveState = "partial"
		_ = m.Store.WriteExecution(taskID, execution)
		return execution, err
	}
	execution.ArchiveState = "complete"
	if err := m.Store.WriteExecution(taskID, execution); err != nil {
		return store.Execution{}, err
	}
	if _, err := m.Store.AppendExecutionEvent(taskID, executionID, store.Event{Operation: "archive", Outcome: "succeeded"}); err != nil {
		return store.Execution{}, err
	}
	m.publishExecutionState(execution)
	return execution, nil
}

func (m *Manager) ReconcileExecutions(taskID string) ([]store.Execution, error) {
	if err := m.ensureLegacyExecution(taskID); err != nil {
		return nil, err
	}
	ids, err := m.Store.ExecutionIDs(taskID)
	if err != nil {
		return nil, err
	}
	result := make([]store.Execution, 0, len(ids))
	for _, id := range ids {
		execution, err := m.Store.ReadExecution(taskID, id)
		if err != nil {
			return nil, err
		}
		if execution.Lifecycle == "running" {
			observation, observeErr := m.observeExecution(taskID, id)
			if observeErr != nil {
				return nil, observeErr
			}
			before := execution
			now := m.now()
			applyExecutionObservation(&execution, observation, now, m.heartbeatTimeout())
			if observation.Available && len(observation.Processes) == 0 {
				execution.Lifecycle, execution.Condition = "stopped", "none"
			}
			if !reflect.DeepEqual(execution, before) {
				if err := m.Store.WriteExecution(taskID, execution); err != nil {
					return nil, err
				}
				_, _ = m.Store.AppendExecutionEvent(taskID, id, store.Event{Operation: "reconcile", Outcome: observationOutcome(execution.Observation)})
			}
			m.publishExecutionStateValue(execution, reconciledExecutionState(execution, now, m.heartbeatTimeout()))
		} else {
			m.publishExecutionState(execution)
		}
		result = append(result, execution)
	}
	return result, nil
}

func (m *Manager) executionLive(taskID, executionID string) (bool, bool, error) {
	observation, err := m.observeExecution(taskID, executionID)
	if err != nil {
		return false, false, err
	}
	if !observation.Available {
		return false, false, nil
	}
	return len(observation.Processes) > 0, true, nil
}
func (m *Manager) observeExecution(taskID, executionID string) (TmuxObservation, error) {
	if tmux, ok := m.Tmux.(ExecutionTmux); ok {
		return tmux.ObserveExecution(executionID, taskID)
	}
	return m.Tmux.Observe(taskID)
}
func liveExecutionError(operation string) error {
	return &store.Error{Kind: store.KindLocked, Message: "Execution process is still running", Retryable: true, Recovery: fmt.Sprintf("Stop the execution, then retry %s", operation)}
}

func applyExecutionObservation(execution *store.Execution, observation TmuxObservation, now time.Time, timeout time.Duration) {
	execution.ObservationAt = now
	if !observation.Available {
		execution.Observation = ObservationUnavailable
		execution.ObservedPID, execution.ObservedStartTime = 0, 0
		return
	}
	if len(observation.Processes) == 0 {
		execution.Observation = ObservationMissing
		execution.ObservedPID, execution.ObservedStartTime = 0, 0
		return
	}
	if len(observation.Processes) != 1 || processState(observation.Processes[0]) != ObservationFresh {
		execution.Observation = ObservationContradictory
		execution.ObservedPID, execution.ObservedStartTime = 0, 0
		return
	}
	process := observation.Processes[0]
	execution.TmuxWindow, execution.ProcessPane = process.WindowID, process.PaneID
	execution.ObservedPID, execution.ObservedStartTime = process.PID, process.StartTime
	if execution.ProcessPID == 0 || execution.ProcessStartTime == 0 {
		execution.ProcessPID, execution.ProcessStartTime = process.PID, process.StartTime
	}
	if execution.ProcessPID != process.PID || execution.ProcessStartTime != process.StartTime {
		execution.Observation = ObservationReplaced
		return
	}
	execution.Observation = ObservationFresh
	if !execution.HeartbeatAt.IsZero() && now.Sub(execution.HeartbeatAt) > timeout {
		execution.Observation = ObservationStale
	}
}

func ExecutionStatus(execution store.Execution, now time.Time, timeout time.Duration) string {
	return Status(store.Manifest{Lifecycle: execution.Lifecycle, Condition: execution.Condition, HeartbeatAt: execution.HeartbeatAt, Observation: execution.Observation}, now, timeout)
}

// ensureLegacyExecution lazily migrates the pre-execution task fields while
// preserving them for old clients and recovery commands.
func (m *Manager) ensureLegacyExecution(taskID string) error {
	return m.ensureLegacyExecutionWithID(taskID, "", "")
}

func (m *Manager) ensureLegacyExecutionWithID(taskID, requestedID, requestedLabel string) error {
	manifest, err := m.manifest(taskID)
	if err != nil {
		return err
	}
	if manifest.ExecutionIDs != "" {
		return nil
	}
	if manifest.Launch == nil && manifest.ProcessPID == 0 && manifest.TmuxWindow == "" {
		return nil
	}
	resourceID := ""
	if ids := strings.Split(manifest.ResourceIDs, ","); len(ids) == 1 {
		resourceID = ids[0]
	}
	executionID := requestedID
	if executionID == "" {
		executionID = "legacy"
	}
	label := requestedLabel
	if label == "" {
		label = tmuxWindowName(manifest.Branch)
	}
	execution := store.Execution{ID: executionID, TaskID: taskID, Label: label, Target: "shell", ResourceID: resourceID, WorkingDirectory: manifest.WorktreePath, Lifecycle: manifest.Lifecycle, Condition: manifest.Condition, Reason: manifest.Reason, Activity: manifest.Activity, HeartbeatAt: manifest.HeartbeatAt, Result: manifest.Result, TmuxWindow: manifest.TmuxWindow, ProcessPID: manifest.ProcessPID, ProcessStartTime: manifest.ProcessStartTime, ObservedPID: manifest.ObservedPID, ObservedStartTime: manifest.ObservedStartTime, ProcessPane: manifest.ProcessPane, Observation: manifest.Observation, ObservationAt: manifest.ObservationAt, ArchiveState: manifest.ArchiveState, RecoveryDebt: manifest.RecoveryDebt}
	if manifest.Launch != nil {
		execution.Target, execution.Command = manifest.Launch.Target, manifest.Launch.Command
	}
	if execution.Lifecycle == "" {
		execution.Lifecycle = "created"
	}
	if execution.Condition == "" {
		execution.Condition = "none"
	}
	if _, _, err := m.Store.CreateExecution(taskID, execution); err != nil {
		return err
	}
	_, err = m.Store.UpdateManifest(taskID, func(task *store.Manifest) error { task.ExecutionIDs = executionID; return nil })
	if err != nil {
		return err
	}
	_, err = m.Store.AppendExecutionEvent(taskID, executionID, store.Event{Operation: "migrate", Outcome: "created", Detail: "legacy task execution detached"})
	return err
}

func (m *Manager) updateExecutionFromManifest(taskID string, manifest store.Manifest) error {
	ids := strings.Split(manifest.ExecutionIDs, ",")
	if len(ids) != 1 || ids[0] == "" {
		return nil
	}
	id := ids[0]
	execution, err := m.Store.UpdateExecution(taskID, id, func(execution *store.Execution) error {
		execution.Lifecycle, execution.Condition, execution.Reason, execution.Activity = manifest.Lifecycle, manifest.Condition, manifest.Reason, manifest.Activity
		execution.HeartbeatAt, execution.Result = manifest.HeartbeatAt, manifest.Result
		execution.TmuxWindow, execution.ProcessPID, execution.ProcessStartTime = manifest.TmuxWindow, manifest.ProcessPID, manifest.ProcessStartTime
		execution.ObservedPID, execution.ObservedStartTime, execution.ProcessPane = manifest.ObservedPID, manifest.ObservedStartTime, manifest.ProcessPane
		execution.Observation, execution.ObservationAt, execution.ArchiveState, execution.RecoveryDebt = manifest.Observation, manifest.ObservationAt, manifest.ArchiveState, manifest.RecoveryDebt
		return nil
	})
	if err != nil {
		return err
	}
	m.publishExecutionState(execution)
	return nil
}

func (m *Manager) syncExecutionStates(taskID string) error {
	ids, err := m.Store.ExecutionIDs(taskID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		execution, err := m.Store.ReadExecution(taskID, id)
		if err != nil {
			return err
		}
		m.publishExecutionState(execution)
	}
	return nil
}

func (m *Manager) publishExecutionState(execution store.Execution) {
	m.publishExecutionStateValue(execution, desiredExecutionState(execution))
}

func (m *Manager) publishExecutionStateValue(execution store.Execution, state string) {
	if tmux, ok := m.Tmux.(ExecutionStateTmux); ok {
		_ = tmux.SetExecutionState(execution.ID, execution.TaskID, state)
	}
}

func desiredExecutionState(execution store.Execution) string {
	if execution.Lifecycle == "finished" {
		return "done"
	}
	if execution.Lifecycle != "running" {
		return ""
	}
	switch execution.Condition {
	case "waiting", "blocked":
		return execution.Condition
	default:
		return ""
	}
}

func reconciledExecutionState(execution store.Execution, now time.Time, timeout time.Duration) string {
	if execution.Lifecycle == "finished" {
		return "done"
	}
	if execution.Lifecycle != "running" || execution.Observation != ObservationFresh || execution.HeartbeatAt.IsZero() || now.Sub(execution.HeartbeatAt) > timeout {
		return ""
	}
	return desiredExecutionState(execution)
}

func shellCommand() string {
	if value := os.Getenv("SHELL"); value != "" {
		return value
	}
	return "/bin/sh"
}
