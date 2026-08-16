package lifecycle

import (
	"errors"
	"strings"
	"testing"

	"github.com/akofink/akagent-cli/internal/store"
)

type independentExecutionTmux struct {
	observation TmuxObservation
	started     int
	stopped     int
	states      []string
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
func (t *independentExecutionTmux) SetExecutionState(_, _, state string) error {
	t.states = append(t.states, state)
	return nil
}

func TestCompatibilityExecutionLabelUsesTaskBranch(t *testing.T) {
	manager, _ := newTestManager(t)
	if err := manager.Store.WriteManifest("label-task", store.Manifest{Title: "Descriptive label", Worker: "local", Lifecycle: "created", Branch: "akofink/69-execution-labels"}); err != nil {
		t.Fatal(err)
	}
	label, err := manager.ResolveCompatibilityExecutionLabel("label-task", "", "")
	if err != nil || label != "69-execution-labels" {
		t.Fatalf("ResolveCompatibilityExecutionLabel() = %q, %v; want branch-derived label", label, err)
	}
}

func TestCompatibilityExecutionLabelRequiresDescriptiveValue(t *testing.T) {
	manager, _ := newTestManager(t)
	if _, err := manager.Create(CreateRequest{ID: "label-required", Title: "Label required"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ResolveCompatibilityExecutionLabel("label-required", "", ""); !store.IsKind(err, store.KindUsage) {
		t.Fatalf("missing compatibility label error = %v, want usage error", err)
	}
	for _, label := range []string{"pi", "shell", "akagent", "019fe8f2-ac67-7406-a6e6-2717b2cd31c6"} {
		if _, err := manager.ResolveCompatibilityExecutionLabel("label-required", "", label); !store.IsKind(err, store.KindUsage) {
			t.Fatalf("label %q error = %v, want usage error", label, err)
		}
	}
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
	if tmux.started != 1 || len(tmux.states) != 1 || tmux.states[0] != "" {
		t.Fatalf("execution launch state = starts=%d states=%#v, want active state cleared", tmux.started, tmux.states)
	}
	if _, err := manager.PublishExecution("execution-task", "exec-one", "waiting", "review", "awaiting review"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.PublishExecution("execution-task", "exec-one", "blocked", "approval", "awaiting approval"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.PublishExecution("execution-task", "exec-one", "active", "coding", "running"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(tmux.states, "|"); got != "|waiting|blocked|" {
		t.Fatalf("execution states = %q, want active clear, waiting, blocked, active clear", got)
	}
	if _, err := manager.StopExecution("execution-task", "exec-one"); err != nil {
		t.Fatal(err)
	}
	if tmux.states[len(tmux.states)-1] != "" {
		t.Fatalf("stopped execution state = %q, want clear", tmux.states[len(tmux.states)-1])
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

func TestExecutionSessionReferencesAreProviderNeutralAndIdempotent(t *testing.T) {
	manager, _ := newTestManager(t)
	if _, err := manager.Create(CreateRequest{ID: "session-task", Title: "Session task"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.CreateExecution("session-task", ExecutionRequest{ID: "session-execution", Target: "tool", Command: "/bin/sh"}); err != nil {
		t.Fatal(err)
	}
	reference := store.SessionReference{Tool: "pi", SessionID: "pi-session"}
	if _, err := manager.AddExecutionSessionReference("session-task", "session-execution", reference); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.RecordExecutionSessionReference("session-task", "session-execution", reference); err != nil {
		t.Fatal(err)
	}
	execution, err := manager.InspectExecution("session-task", "session-execution")
	if err != nil {
		t.Fatal(err)
	}
	if len(execution.SessionReferences) != 1 || execution.SessionReferences[0] != reference {
		t.Fatalf("session references = %#v, want one provider-neutral reference", execution.SessionReferences)
	}
}

func TestTaskFinishPublishesDoneExecutionState(t *testing.T) {
	manager, _ := newTestManager(t)
	tmux := &independentExecutionTmux{observation: TmuxObservation{Available: true}}
	manager.Tmux = tmux
	if _, err := manager.Create(CreateRequest{ID: "finish-task", Title: "Finish task"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.CreateExecution("finish-task", ExecutionRequest{ID: "finish-execution", Target: "shell", Command: "/bin/sh"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.LaunchExecutionRecord("finish-task", "finish-execution"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Finish("finish-task", "succeeded", "complete"); err != nil {
		t.Fatal(err)
	}
	if got := tmux.states[len(tmux.states)-1]; got != "done" {
		t.Fatalf("finished execution state = %q, want done", got)
	}
	tmux.observation = TmuxObservation{Available: true}
	if _, err := manager.ArchiveExecution("finish-task", "finish-execution"); err != nil {
		t.Fatal(err)
	}
	if got := tmux.states[len(tmux.states)-1]; got != "done" {
		t.Fatalf("archived execution state = %q, want done", got)
	}
}

func TestReconcileClearsStaleExecutionState(t *testing.T) {
	manager, _ := newTestManager(t)
	tmux := &independentExecutionTmux{observation: TmuxObservation{Available: true}}
	manager.Tmux = tmux
	if _, err := manager.Create(CreateRequest{ID: "reconcile-execution", Title: "Reconcile execution"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.CreateExecution("reconcile-execution", ExecutionRequest{ID: "reconcile-one", Target: "shell", Command: "/bin/sh"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.LaunchExecutionRecord("reconcile-execution", "reconcile-one"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.PublishExecution("reconcile-execution", "reconcile-one", "waiting", "review", "awaiting review"); err != nil {
		t.Fatal(err)
	}
	tmux.observation = TmuxObservation{Available: true}
	if _, err := manager.ReconcileExecutions("reconcile-execution"); err != nil {
		t.Fatal(err)
	}
	if got := tmux.states[len(tmux.states)-1]; got != "" {
		t.Fatalf("reconciled stale execution state = %q, want clear", got)
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
