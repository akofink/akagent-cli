// Package lifecycle coordinates durable local task state with tmux observations.
package lifecycle

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/akofink/akagent-cli/internal/credential"
	"github.com/akofink/akagent-cli/internal/store"
)

const DefaultHeartbeatTimeout = 2 * time.Minute

const (
	ObservationFresh         = "fresh"
	ObservationStale         = "stale"
	ObservationMissing       = "missing"
	ObservationReplaced      = "replaced"
	ObservationContradictory = "contradictory"
	ObservationUnavailable   = "unavailable"
)

// TmuxProcess is the non-secret identity observed for one task pane.
// PID alone is deliberately insufficient because it can be reused.
type TmuxProcess struct {
	WindowID  string
	PaneID    string
	PID       int
	StartTime uint64
}

type TmuxObservation struct {
	Available bool
	Processes []TmuxProcess
}

type Tmux interface {
	Start(taskID string) (TmuxProcess, error)
	Observe(taskID string) (TmuxObservation, error)
	Stop(taskID string) error
}

type Manager struct {
	Store            *store.Store
	Tmux             Tmux
	Credentials      func() (*credential.Manifest, error)
	Checker          *credential.Checker
	Now              func() time.Time
	HeartbeatTimeout time.Duration
}

type StartRequest struct {
	ID           string
	Title        string
	Repository   string
	Requirements []string
	Optional     []string
}

type StartResult struct {
	Manifest store.Manifest
	Created  bool
}

func New(state *store.Store) *Manager {
	return &Manager{
		Store: state, Tmux: commandTmux{}, Credentials: func() (*credential.Manifest, error) {
			return credential.Load(credential.ConfigPath())
		}, Checker: credential.NewChecker(), Now: time.Now, HeartbeatTimeout: DefaultHeartbeatTimeout,
	}
}

func (m *Manager) RegisterRepository(name, path, policy string) (store.Repository, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return store.Repository{}, fmt.Errorf("resolve repository path: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return store.Repository{}, fmt.Errorf("repository path is not a directory")
	}
	if policy == "" {
		policy = discoverPolicy(abs)
	}
	repository := store.Repository{Name: name, Path: abs, Policy: policy}
	if existing, err := m.Store.ReadRepository(name); err == nil {
		if existing == repository {
			return existing, nil
		}
		return store.Repository{}, fmt.Errorf("repository registration conflicts with existing %s", name)
	}
	if err := m.Store.WriteRepository(repository); err != nil {
		return store.Repository{}, err
	}
	return repository, nil
}

func discoverPolicy(path string) string {
	if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
		return "worktree"
	}
	return "direct"
}

func (m *Manager) Start(request StartRequest) (StartResult, error) {
	if request.ID == "" || request.Title == "" || request.Repository == "" {
		return StartResult{}, fmt.Errorf("task ID, title, and repository are required")
	}
	if _, err := m.Store.ReadRepository(request.Repository); err != nil {
		return StartResult{}, err
	}
	request.Requirements = unique(request.Requirements)
	request.Optional = unique(request.Optional)
	warnings, err := m.checkCredentials(request.Requirements, request.Optional)
	if err != nil {
		return StartResult{}, err
	}
	now := m.now()
	manifest := store.Manifest{Title: request.Title, Worker: "local", Repository: request.Repository, Lifecycle: "starting", Condition: "none", HeartbeatAt: now, Requirements: strings.Join(request.Requirements, ","), Warnings: strings.Join(warnings, "; ")}
	if envelope, err := m.Store.ReadManifest(request.ID); err == nil {
		existing, decodeErr := envelope.DecodeManifest()
		if decodeErr != nil {
			return StartResult{}, decodeErr
		}
		if !sameStart(existing, manifest) {
			return StartResult{}, fmt.Errorf("task inputs conflict with the existing task")
		}
		// Only resume a persisted, incomplete startup. A completed start remains
		// a no-op even if a later observation found its tmux window missing.
		if existing.Lifecycle == "starting" {
			if err := m.ensureStarted(request.ID, &existing); err != nil {
				return StartResult{}, err
			}
		}
		return StartResult{Manifest: existing}, nil
	} else if !store.IsKind(err, store.KindNotFound) {
		return StartResult{}, err
	}
	if err := m.Store.WriteManifest(request.ID, manifest); err != nil {
		return StartResult{}, err
	}
	if _, err := m.Store.AppendEvent(request.ID, store.Event{Operation: "start", Outcome: "intent"}); err != nil {
		return StartResult{}, err
	}
	if err := m.ensureStarted(request.ID, &manifest); err != nil {
		return StartResult{}, err
	}
	return StartResult{Manifest: manifest, Created: true}, nil
}

func (m *Manager) ensureStarted(id string, manifest *store.Manifest) error {
	if manifest.Lifecycle == "running" {
		observation, err := m.Tmux.Observe(id)
		if err != nil {
			return err
		}
		if observation.Available {
			applyObservation(manifest, observation, m.now(), m.heartbeatTimeout())
			return m.Store.WriteManifest(id, *manifest)
		}
		return nil
	}
	process, err := m.Tmux.Start(id)
	if err != nil {
		_, _ = m.Store.AppendEvent(id, store.Event{Operation: "start", Outcome: "failed"})
		return fmt.Errorf("start tmux task: %w", err)
	}
	manifest.Lifecycle = "running"
	manifest.TmuxWindow = process.WindowID
	manifest.ProcessPID = process.PID
	manifest.ProcessStartTime = process.StartTime
	manifest.ObservedPID = process.PID
	manifest.ObservedStartTime = process.StartTime
	manifest.ProcessPane = process.PaneID
	manifest.Observation = processState(process)
	manifest.ObservationAt = m.now()
	if err := m.Store.WriteManifest(id, *manifest); err != nil {
		return err
	}
	_, err = m.Store.AppendEvent(id, store.Event{Operation: "start", Outcome: "succeeded"})
	return err
}

func (m *Manager) Publish(id, condition, reason, activity string) (store.Manifest, error) {
	if !validCondition(condition) {
		return store.Manifest{}, fmt.Errorf("condition must be active, waiting, blocked, failed, or none")
	}
	var changed bool
	manifest, err := m.Store.UpdateManifest(id, func(manifest *store.Manifest) error {
		changed = manifest.Condition != condition || manifest.Reason != reason || manifest.Activity != activity
		manifest.Condition, manifest.Reason, manifest.Activity, manifest.HeartbeatAt = condition, reason, activity, m.now()
		return nil
	})
	if err != nil {
		return store.Manifest{}, err
	}
	if changed {
		_, err = m.Store.AppendEvent(id, store.Event{Operation: "publish", Outcome: condition})
	}
	return manifest, err
}

func (m *Manager) Finish(id, outcome, result string) (store.Manifest, error) {
	if outcome != "succeeded" && outcome != "failed" {
		return store.Manifest{}, fmt.Errorf("finish outcome must be succeeded or failed")
	}
	observation, err := m.Tmux.Observe(id)
	if err != nil {
		return store.Manifest{}, err
	}
	if !observation.Available {
		return store.Manifest{}, fmt.Errorf("task process observation is unavailable")
	}
	if len(observation.Processes) != 0 {
		return store.Manifest{}, fmt.Errorf("task process is still running")
	}
	var changed bool
	manifest, err := m.Store.UpdateManifest(id, func(manifest *store.Manifest) error {
		if manifest.Lifecycle == "finished" && manifest.Result == result && manifest.Condition == outcomeToCondition(outcome) {
			return nil
		}
		changed = true
		manifest.Lifecycle, manifest.Condition, manifest.Result = "finished", outcomeToCondition(outcome), result
		manifest.Observation, manifest.ObservationAt = ObservationMissing, m.now()
		manifest.ObservedPID, manifest.ObservedStartTime = 0, 0
		return nil
	})
	if err != nil {
		return store.Manifest{}, err
	}
	if changed {
		_, err = m.Store.AppendEvent(id, store.Event{Operation: "finish", Outcome: outcome})
	}
	return manifest, err
}

func (m *Manager) Stop(id string) (store.Manifest, error) {
	manifest, err := m.manifest(id)
	if err != nil {
		return store.Manifest{}, err
	}
	if manifest.Lifecycle == "stopped" || manifest.Lifecycle == "finished" {
		return manifest, nil
	}
	if err := m.Tmux.Stop(id); err != nil {
		return store.Manifest{}, err
	}
	var changed bool
	manifest, err = m.Store.UpdateManifest(id, func(manifest *store.Manifest) error {
		if manifest.Lifecycle == "stopped" || manifest.Lifecycle == "finished" {
			return nil
		}
		changed = true
		manifest.Lifecycle, manifest.Condition = "stopped", "none"
		manifest.Observation, manifest.ObservationAt = ObservationMissing, m.now()
		manifest.ObservedPID, manifest.ObservedStartTime = 0, 0
		return nil
	})
	if err != nil {
		return store.Manifest{}, err
	}
	if changed {
		_, err = m.Store.AppendEvent(id, store.Event{Operation: "stop", Outcome: "succeeded"})
	}
	return manifest, err
}

// Reconcile repairs derived observations only. It never removes a window or worktree.
func (m *Manager) Reconcile() ([]store.Manifest, error) {
	if _, err := m.Store.Recover(); err != nil {
		return nil, err
	}
	ids, err := m.Store.TaskIDs()
	if err != nil {
		return nil, err
	}
	manifests := make([]store.Manifest, 0, len(ids))
	for _, id := range ids {
		observation, err := m.Tmux.Observe(id)
		if err != nil {
			return nil, err
		}
		var changed bool
		manifest, err := m.Store.UpdateManifest(id, func(manifest *store.Manifest) error {
			beforeObservation, beforeLifecycle := manifest.Observation, manifest.Lifecycle
			legacyProcess := manifest.ProcessPID == 0 || manifest.ProcessStartTime == 0
			applyObservation(manifest, observation, m.now(), m.heartbeatTimeout())
			if legacyProcess && observation.Available && len(observation.Processes) == 0 && manifest.Lifecycle == "running" {
				manifest.Lifecycle, manifest.Condition = "stopped", "none"
			}
			changed = beforeObservation != manifest.Observation || beforeLifecycle != manifest.Lifecycle
			return nil
		})
		if err != nil {
			return nil, err
		}
		if changed {
			if _, err := m.Store.AppendEvent(id, store.Event{Operation: "reconcile", Outcome: observationOutcome(manifest.Observation)}); err != nil {
				return nil, err
			}
		}
		manifests = append(manifests, manifest)
	}
	return manifests, nil
}

func (m *Manager) Inspect(id string) (store.Manifest, error) { return m.manifest(id) }

func (m *Manager) List() ([]store.Manifest, error) {
	ids, err := m.Store.TaskIDs()
	if err != nil {
		return nil, err
	}
	items := make([]store.Manifest, 0, len(ids))
	for _, id := range ids {
		item, err := m.manifest(id)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (m *Manager) manifest(id string) (store.Manifest, error) {
	envelope, err := m.Store.ReadManifest(id)
	if err != nil {
		return store.Manifest{}, err
	}
	return envelope.DecodeManifest()
}

func (m *Manager) checkCredentials(required, optional []string) ([]string, error) {
	if len(required) == 0 && len(optional) == 0 {
		return nil, nil
	}
	manifest, err := m.Credentials()
	if err != nil {
		return nil, fmt.Errorf("credential manifest could not be loaded")
	}
	checks := credential.Doctor(manifest, m.Checker)
	byID := make(map[string]credential.Check, len(checks))
	for _, check := range checks {
		byID[check.Entry.ID] = check
	}
	for _, id := range required {
		check, ok := byID[id]
		if !ok || check.Status != credential.Ready {
			return nil, fmt.Errorf("required credential %s is unavailable", id)
		}
	}
	warnings := make([]string, 0, len(optional))
	for _, id := range optional {
		check, ok := byID[id]
		if !ok || check.Status != credential.Ready {
			warnings = append(warnings, fmt.Sprintf("optional credential %s is unavailable", id))
		}
	}
	return warnings, nil
}

func (m *Manager) now() time.Time {
	if m.Now == nil {
		return time.Now().UTC()
	}
	return m.Now().UTC()
}

func (m *Manager) heartbeatTimeout() time.Duration {
	if m.HeartbeatTimeout <= 0 {
		return DefaultHeartbeatTimeout
	}
	return m.HeartbeatTimeout
}

func sameStart(a, b store.Manifest) bool {
	return a.Title == b.Title && a.Worker == b.Worker && a.Repository == b.Repository && a.Requirements == b.Requirements
}

func validCondition(value string) bool {
	return value == "active" || value == "waiting" || value == "blocked" || value == "failed" || value == "none"
}

func outcomeToCondition(outcome string) string {
	if outcome == "failed" {
		return "failed"
	}
	return "none"
}

func unique(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			seen[value] = true
		}
	}
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func processState(process TmuxProcess) string {
	if process.PID == 0 && process.StartTime == 0 {
		return ""
	}
	if process.PID <= 0 || process.StartTime == 0 {
		return ObservationContradictory
	}
	return ObservationFresh
}

func applyObservation(manifest *store.Manifest, observation TmuxObservation, now time.Time, heartbeatTimeout time.Duration) {
	manifest.ObservationAt = now
	if !observation.Available {
		manifest.Observation = ObservationUnavailable
		manifest.ObservedPID, manifest.ObservedStartTime = 0, 0
		return
	}
	if len(observation.Processes) == 0 {
		manifest.Observation = ObservationMissing
		manifest.ObservedPID, manifest.ObservedStartTime = 0, 0
		return
	}
	if len(observation.Processes) != 1 || processState(observation.Processes[0]) != ObservationFresh {
		manifest.Observation = ObservationContradictory
		manifest.ObservedPID, manifest.ObservedStartTime = 0, 0
		return
	}
	process := observation.Processes[0]
	manifest.TmuxWindow, manifest.ProcessPane = process.WindowID, process.PaneID
	manifest.ObservedPID, manifest.ObservedStartTime = process.PID, process.StartTime
	if manifest.ProcessPID == 0 || manifest.ProcessStartTime == 0 {
		manifest.ProcessPID, manifest.ProcessStartTime = process.PID, process.StartTime
	}
	if manifest.ProcessPID == process.PID && manifest.ProcessStartTime == process.StartTime {
		manifest.Observation = ObservationFresh
		if !manifest.HeartbeatAt.IsZero() && now.Sub(manifest.HeartbeatAt) > heartbeatTimeout {
			manifest.Observation = ObservationStale
		}
		return
	}
	manifest.Observation = ObservationReplaced
}

func observationOutcome(observation string) string {
	if observation == ObservationMissing {
		return "window_missing"
	}
	if observation == ObservationReplaced {
		return "process_replaced"
	}
	return "observation_" + observation
}

// Status computes the protocol status from durable lifecycle state and the
// latest verified observations. It never treats a PID without its start time
// as a live task process.
func Status(manifest store.Manifest, now time.Time, heartbeatTimeout time.Duration) string {
	if manifest.Condition == "failed" {
		return "failed"
	}
	switch manifest.Lifecycle {
	case "starting":
		return "starting"
	case "stopped":
		return "stopped"
	case "finished":
		if manifest.Observation == ObservationMissing {
			return "finished"
		}
		return "unknown"
	case "running":
		if manifest.Observation == "" {
			if manifest.Condition == "waiting" || manifest.Condition == "blocked" || manifest.Condition == "active" {
				return manifest.Condition
			}
			return "running"
		}
		if manifest.Observation != ObservationFresh || manifest.HeartbeatAt.IsZero() || now.Sub(manifest.HeartbeatAt) > heartbeatTimeout {
			return "unknown"
		}
		if manifest.Condition == "waiting" || manifest.Condition == "blocked" {
			return manifest.Condition
		}
		return "active"
	default:
		return "unknown"
	}
}

type commandTmux struct{}

func (commandTmux) Start(id string) (TmuxProcess, error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	output, err := exec.Command("tmux", "new-window", "-d", "-P", "-F", "#{window_id}", "-n", "akagent-"+id[:min(8, len(id))], shell).Output()
	if err != nil {
		return TmuxProcess{}, errors.New("tmux window could not be created")
	}
	window := strings.TrimSpace(string(output))
	if window == "" {
		return TmuxProcess{}, errors.New("tmux did not return a window ID")
	}
	if err := exec.Command("tmux", "set-option", "-w", "-t", window, "@akagent_task_id", id).Run(); err != nil {
		return TmuxProcess{}, errors.New("tmux task metadata could not be set")
	}
	observation, err := (commandTmux{}).Observe(id)
	if err != nil {
		return TmuxProcess{WindowID: window}, err
	}
	if observation.Available && len(observation.Processes) == 1 {
		return observation.Processes[0], nil
	}
	return TmuxProcess{WindowID: window}, nil
}

func (commandTmux) Observe(id string) (TmuxObservation, error) {
	output, err := exec.Command("tmux", "list-windows", "-a", "-F", "#{window_id}\t#{@akagent_task_id}").Output()
	if err != nil {
		return TmuxObservation{}, nil
	}
	observation := TmuxObservation{Available: true}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 2 || fields[1] != id {
			continue
		}
		paneOutput, paneErr := exec.Command("tmux", "list-panes", "-t", fields[0], "-F", "#{window_id}\t#{pane_id}\t#{pane_pid}").Output()
		if paneErr != nil {
			observation.Processes = append(observation.Processes, TmuxProcess{WindowID: fields[0]})
			continue
		}
		for _, paneLine := range strings.Split(strings.TrimSpace(string(paneOutput)), "\n") {
			paneFields := strings.Split(paneLine, "\t")
			if len(paneFields) != 3 {
				continue
			}
			pid, parseErr := strconv.Atoi(paneFields[2])
			startTime := uint64(0)
			if parseErr == nil {
				startTime, _ = processStartTime(pid)
			}
			observation.Processes = append(observation.Processes, TmuxProcess{WindowID: paneFields[0], PaneID: paneFields[1], PID: pid, StartTime: startTime})
		}
	}
	return observation, nil
}

func (commandTmux) Stop(id string) error {
	output, err := exec.Command("tmux", "list-windows", "-a", "-F", "#{window_id}\t#{@akagent_task_id}").Output()
	if err != nil {
		return nil
	}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) == 2 && fields[1] == id {
			if err := exec.Command("tmux", "kill-window", "-t", fields[0]).Run(); err != nil {
				return errors.New("tmux task window could not be stopped")
			}
		}
	}
	return nil
}

func processStartTime(pid int) (uint64, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err == nil {
		end := strings.LastIndex(string(data), ")")
		if end >= 0 {
			fields := strings.Fields(string(data)[end+2:])
			if len(fields) > 19 {
				return strconv.ParseUint(fields[19], 10, 64)
			}
		}
	}
	return 0, errors.New("process start time is unavailable")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
