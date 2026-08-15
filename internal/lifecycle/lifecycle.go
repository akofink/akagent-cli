// Package lifecycle coordinates durable local task state with tmux observations.
package lifecycle

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/akofink/akagent-cli/internal/credential"
	"github.com/akofink/akagent-cli/internal/store"
)

type Tmux interface {
	Start(taskID string) (string, error)
	Exists(taskID string) (bool, error)
	Stop(taskID string) error
}

type Manager struct {
	Store       *store.Store
	Tmux        Tmux
	Credentials func() (*credential.Manifest, error)
	Checker     *credential.Checker
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
	return &Manager{Store: state, Tmux: commandTmux{}, Credentials: func() (*credential.Manifest, error) {
		return credential.Load(credential.ConfigPath())
	}, Checker: credential.NewChecker()}
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
	manifest := store.Manifest{Title: request.Title, Worker: "local", Repository: request.Repository, Lifecycle: "starting", Condition: "none", Requirements: strings.Join(request.Requirements, ","), Warnings: strings.Join(warnings, "; ")}
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
	exists, err := m.Tmux.Exists(id)
	if err != nil {
		return err
	}
	if exists && manifest.Lifecycle == "running" {
		return nil
	}
	if !exists {
		window, err := m.Tmux.Start(id)
		if err != nil {
			_, _ = m.Store.AppendEvent(id, store.Event{Operation: "start", Outcome: "failed"})
			return fmt.Errorf("start tmux task: %w", err)
		}
		manifest.TmuxWindow = window
	}
	manifest.Lifecycle = "running"
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
	manifest, err := m.manifest(id)
	if err != nil {
		return store.Manifest{}, err
	}
	if manifest.Condition == condition && manifest.Reason == reason && manifest.Activity == activity {
		return manifest, nil
	}
	manifest.Condition, manifest.Reason, manifest.Activity, manifest.HeartbeatAt = condition, reason, activity, time.Now().UTC()
	if err := m.Store.WriteManifest(id, manifest); err != nil {
		return store.Manifest{}, err
	}
	_, err = m.Store.AppendEvent(id, store.Event{Operation: "publish", Outcome: condition})
	return manifest, err
}

func (m *Manager) Finish(id, outcome, result string) (store.Manifest, error) {
	if outcome != "succeeded" && outcome != "failed" {
		return store.Manifest{}, fmt.Errorf("finish outcome must be succeeded or failed")
	}
	manifest, err := m.manifest(id)
	if err != nil {
		return store.Manifest{}, err
	}
	exists, err := m.Tmux.Exists(id)
	if err != nil {
		return store.Manifest{}, err
	}
	if exists {
		return store.Manifest{}, fmt.Errorf("task process is still running")
	}
	if manifest.Lifecycle == "finished" && manifest.Result == result && manifest.Condition == outcomeToCondition(outcome) {
		return manifest, nil
	}
	manifest.Lifecycle, manifest.Condition, manifest.Result = "finished", outcomeToCondition(outcome), result
	if err := m.Store.WriteManifest(id, manifest); err != nil {
		return store.Manifest{}, err
	}
	_, err = m.Store.AppendEvent(id, store.Event{Operation: "finish", Outcome: outcome})
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
	manifest.Lifecycle, manifest.Condition = "stopped", "none"
	if err := m.Store.WriteManifest(id, manifest); err != nil {
		return store.Manifest{}, err
	}
	_, err = m.Store.AppendEvent(id, store.Event{Operation: "stop", Outcome: "succeeded"})
	return manifest, err
}

// Reconcile repairs only derived lifecycle observations. It never removes a window or worktree.
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
		manifest, err := m.manifest(id)
		if err != nil {
			return nil, err
		}
		exists, err := m.Tmux.Exists(id)
		if err != nil {
			return nil, err
		}
		if manifest.Lifecycle == "running" && !exists {
			manifest.Lifecycle = "stopped"
			manifest.Condition = "none"
			if err := m.Store.WriteManifest(id, manifest); err != nil {
				return nil, err
			}
			if _, err := m.Store.AppendEvent(id, store.Event{Operation: "reconcile", Outcome: "window_missing"}); err != nil {
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

type commandTmux struct{}

func (commandTmux) Start(id string) (string, error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	output, err := exec.Command("tmux", "new-window", "-d", "-P", "-F", "#{window_id}", "-n", "akagent-"+id[:min(8, len(id))], shell).Output()
	if err != nil {
		return "", errors.New("tmux window could not be created")
	}
	window := strings.TrimSpace(string(output))
	if window == "" {
		return "", errors.New("tmux did not return a window ID")
	}
	if err := exec.Command("tmux", "set-option", "-w", "-t", window, "@akagent_task_id", id).Run(); err != nil {
		return "", errors.New("tmux task metadata could not be set")
	}
	return window, nil
}
func (commandTmux) Exists(id string) (bool, error) {
	output, err := exec.Command("tmux", "list-windows", "-a", "-F", "#{@akagent_task_id}").Output()
	if err != nil {
		return false, nil
	}
	for _, value := range strings.Fields(string(output)) {
		if value == id {
			return true, nil
		}
	}
	return false, nil
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
			return nil
		}
	}
	return nil
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
