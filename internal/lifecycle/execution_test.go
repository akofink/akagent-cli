package lifecycle

import (
	"errors"
	"testing"

	"github.com/akofink/akagent-cli/internal/store"
)

type independentExecutionTmux struct {
	observation TmuxObservation
	started     int
	stopped     int
}

func (t *independentExecutionTmux) Start(_ string, _ string) (TmuxProcess, error) {
	return TmuxProcess{}, errors.New("legacy tmux should not be used")
}
func (t *independentExecutionTmux) Observe(_ string) (TmuxObservation, error) {
	return TmuxObservation{Available: true}, nil
}
func (t *independentExecutionTmux) Attach(_, _ string) error { return nil }
func (t *independentExecutionTmux) Stop(_ string) error      { return nil }
func (t *independentExecutionTmux) StartExecution(_, _, _, _, _ string, _ []string) (TmuxProcess, error) {
	t.started++
	t.observation = TmuxObservation{Available: true, Processes: []TmuxProcess{{WindowID: "@execution", PaneID: "%execution", PID: 77, StartTime: 700}}}
	return t.observation.Processes[0], nil
}
func (t *independentExecutionTmux) ObserveExecution(_, _ string) (TmuxObservation, error) {
	return t.observation, nil
}
func (t *independentExecutionTmux) AttachExecution(_, _, _ string) error { return nil }
func (t *independentExecutionTmux) StopExecution(_, _ string) error {
	t.stopped++
	t.observation = TmuxObservation{Available: true}
	return nil
}
func (t *independentExecutionTmux) CaptureExecution(_, _ string) (string, error) {
	return "history", nil
}

func TestExecutionLifecycleIsIndependentFromResources(t *testing.T) {
	manager, _ := newTestManager(t)
	tmux := &independentExecutionTmux{observation: TmuxObservation{Available: true}}
	manager.Tmux = tmux
	if _, err := manager.Create(CreateRequest{ID: "execution-task", Title: "Execution task"}); err != nil {
		t.Fatal(err)
	}
	execution, created, err := manager.CreateExecution("execution-task", ExecutionRequest{ID: "exec-one", Label: "review shell", Target: "shell", Command: "/bin/sh"})
	if err != nil || !created || execution.Lifecycle != "created" {
		t.Fatalf("CreateExecution() = %#v, %v, %v", execution, created, err)
	}
	if tmux.started != 0 {
		t.Fatal("creating an execution started tmux")
	}
	if _, err := manager.LaunchExecutionRecord("execution-task", "exec-one"); err != nil {
		t.Fatal(err)
	}
	if tmux.started != 1 {
		t.Fatalf("execution starts = %d, want 1", tmux.started)
	}
	if _, err := manager.StopExecution("execution-task", "exec-one"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ArchiveExecution("execution-task", "exec-one"); err != nil {
		t.Fatal(err)
	}
	resources, err := manager.ListResources("execution-task")
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 0 {
		t.Fatalf("execution lifecycle changed resources: %#v", resources)
	}
}

func TestLegacyExecutionMigrationPreservesTaskObservation(t *testing.T) {
	manager, _ := newTestManager(t)
	if err := manager.Store.WriteManifest("legacy-execution", store.Manifest{Title: "legacy", Worker: "local", Lifecycle: "running", Condition: "none", Branch: "review", TmuxWindow: "@1", ProcessPID: 41, ProcessStartTime: 401, Observation: ObservationFresh, Launch: &store.LaunchConfig{Target: "other", Command: "/usr/bin/tool"}}); err != nil {
		t.Fatal(err)
	}
	executions, err := manager.ListExecutions("legacy-execution")
	if err != nil || len(executions) != 1 {
		t.Fatalf("ListExecutions() = %#v, %v", executions, err)
	}
	if executions[0].ID != "legacy" || executions[0].Target != "other" || executions[0].ProcessPID != 41 {
		t.Fatalf("migrated execution = %#v", executions[0])
	}
	manifest, err := manager.Inspect("legacy-execution")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ExecutionIDs != "legacy" || manifest.ProcessPID != 41 {
		t.Fatalf("migration changed legacy task identity: %#v", manifest)
	}
}
