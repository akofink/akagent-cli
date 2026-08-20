package lifecycle

import (
	"errors"
	"strings"
	"testing"

	"github.com/akofink/akagent-cli/internal/store"
)

type independentExecutionTmux struct {
	observation     TmuxObservation
	started         int
	stopped         int
	states          []string
	leaveLiveOnStop bool
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
	if !t.leaveLiveOnStop {
		t.observation = TmuxObservation{Available: true}
	}
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
	if _, err := manager.Finish("finish-task", "succeeded", "complete"); !store.IsKind(err, store.KindLocked) {
		t.Fatalf("Finish() error = %v, want a live execution retry error", err)
	}
	if _, err := manager.StopExecution("finish-task", "finish-execution"); err != nil {
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

func TestStopExecutionDoesNotRecordStoppedWhileTaggedWindowRemainsLive(t *testing.T) {
	manager, _ := newTestManager(t)
	executionTmux := &independentExecutionTmux{observation: TmuxObservation{Available: true}}
	manager.Tmux = executionTmux
	if _, err := manager.Create(CreateRequest{ID: "stop-race-task", Title: "Stop race"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.CreateExecution("stop-race-task", ExecutionRequest{ID: "stop-race", Target: "shell", Command: "/bin/sh"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.LaunchExecutionRecord("stop-race-task", "stop-race"); err != nil {
		t.Fatal(err)
	}
	executionTmux.leaveLiveOnStop = true
	_, stopErr := manager.StopExecution("stop-race-task", "stop-race")
	var storeErr *store.Error
	if !errors.As(stopErr, &storeErr) || storeErr.Kind != store.KindLocked || !storeErr.Retryable {
		t.Fatalf("StopExecution() error = %v, want structured retryable stop error", stopErr)
	}
	execution, err := manager.InspectExecution("stop-race-task", "stop-race")
	if err != nil {
		t.Fatal(err)
	}
	if execution.Lifecycle != "running" {
		t.Fatalf("execution after failed stop = %#v, want running", execution)
	}
	executionTmux.leaveLiveOnStop = false
	if _, err := manager.StopExecution("stop-race-task", "stop-race"); err != nil {
		t.Fatal(err)
	}
}

func TestReconcileStopsStaleTerminalExecutionWindow(t *testing.T) {
	manager, _ := newTestManager(t)
	tmux := &independentExecutionTmux{observation: TmuxObservation{Available: true}}
	manager.Tmux = tmux
	if _, err := manager.Create(CreateRequest{ID: "stale-stop-task", Title: "Stale stop"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.CreateExecution("stale-stop-task", ExecutionRequest{ID: "stale-stop", Target: "shell", Command: "/bin/sh"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.LaunchExecutionRecord("stale-stop-task", "stale-stop"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Store.UpdateExecution("stale-stop-task", "stale-stop", func(execution *store.Execution) error {
		execution.Lifecycle, execution.Condition = "stopped", "none"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ReconcileExecutions("stale-stop-task"); err != nil {
		t.Fatal(err)
	}
	execution, err := manager.InspectExecution("stale-stop-task", "stale-stop")
	if err != nil {
		t.Fatal(err)
	}
	if execution.Lifecycle != "stopped" || execution.Observation != ObservationMissing || tmux.stopped != 1 {
		t.Fatalf("reconciled execution = %#v, stops=%d; want stopped and missing after cleanup", execution, tmux.stopped)
	}
}

func TestReconcileRecoversExitedExecutionAndPreservesSessionReference(t *testing.T) {
	manager, _ := newTestManager(t)
	tmux := &independentExecutionTmux{observation: TmuxObservation{Available: true}}
	manager.Tmux = tmux
	if _, err := manager.Create(CreateRequest{ID: "orphaned-execution-task", Title: "Orphaned execution"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.CreateExecution("orphaned-execution-task", ExecutionRequest{ID: "orphaned-execution", Target: "pi", Command: "/bin/sh"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.LaunchExecutionRecord("orphaned-execution-task", "orphaned-execution"); err != nil {
		t.Fatal(err)
	}
	reference := store.SessionReference{Tool: "pi", SessionID: "resume-me"}
	if _, err := manager.AddExecutionSessionReference("orphaned-execution-task", "orphaned-execution", reference); err != nil {
		t.Fatal(err)
	}
	tmux.observation = TmuxObservation{Available: true}

	if _, err := manager.ReconcileTask("orphaned-execution-task"); err != nil {
		t.Fatal(err)
	}
	execution, err := manager.InspectExecution("orphaned-execution-task", "orphaned-execution")
	if err != nil {
		t.Fatal(err)
	}
	if execution.Lifecycle != "stopped" || execution.Condition != "none" || execution.Observation != ObservationMissing {
		t.Fatalf("reconciled orphan = %#v, want stopped, none, missing", execution)
	}
	if len(execution.SessionReferences) != 1 || execution.SessionReferences[0] != reference {
		t.Fatalf("reconciled session references = %#v, want %#v", execution.SessionReferences, []store.SessionReference{reference})
	}
}

func TestReconcileRecoversActiveCreatedExecutionButPreservesLaunchIntent(t *testing.T) {
	manager, _ := newTestManager(t)
	tmux := &independentExecutionTmux{observation: TmuxObservation{Available: true}}
	manager.Tmux = tmux
	if _, err := manager.Create(CreateRequest{ID: "created-orphan-task", Title: "Created orphan"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.CreateExecution("created-orphan-task", ExecutionRequest{ID: "active-created", Target: "shell", Command: "/bin/sh"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.PublishExecution("created-orphan-task", "active-created", "active", "delegated", "working"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ReconcileTask("created-orphan-task"); err != nil {
		t.Fatal(err)
	}
	execution, err := manager.InspectExecution("created-orphan-task", "active-created")
	if err != nil {
		t.Fatal(err)
	}
	if execution.Lifecycle != "stopped" || execution.Condition != "none" {
		t.Fatalf("reconciled active created execution = %#v, want stopped and none", execution)
	}

	if _, _, err := manager.CreateExecution("created-orphan-task", ExecutionRequest{ID: "launch-intent", Target: "shell", Command: "/bin/sh"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ReconcileTask("created-orphan-task"); err != nil {
		t.Fatal(err)
	}
	intent, err := manager.InspectExecution("created-orphan-task", "launch-intent")
	if err != nil {
		t.Fatal(err)
	}
	if intent.Lifecycle != "created" || intent.Condition != "none" {
		t.Fatalf("plain launch intent = %#v, want unchanged created and none", intent)
	}
}

func TestReconcileTaskDoesNotTouchAnotherTask(t *testing.T) {
	manager, _ := newTestManager(t)
	tmux := &independentExecutionTmux{observation: TmuxObservation{Available: true}}
	manager.Tmux = tmux
	for _, taskID := range []string{"scoped-orphan", "scoped-other"} {
		if _, err := manager.Create(CreateRequest{ID: taskID, Title: taskID}); err != nil {
			t.Fatal(err)
		}
		if _, _, err := manager.CreateExecution(taskID, ExecutionRequest{ID: "execution", Target: "shell", Command: "/bin/sh"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := manager.Store.UpdateExecution("scoped-orphan", "execution", func(execution *store.Execution) error {
		execution.Lifecycle = "starting"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Store.UpdateExecution("scoped-other", "execution", func(execution *store.Execution) error {
		execution.Lifecycle = "starting"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ReconcileTask("scoped-orphan"); err != nil {
		t.Fatal(err)
	}
	orphan, err := manager.InspectExecution("scoped-orphan", "execution")
	if err != nil {
		t.Fatal(err)
	}
	other, err := manager.InspectExecution("scoped-other", "execution")
	if err != nil {
		t.Fatal(err)
	}
	if orphan.Lifecycle != "stopped" || other.Lifecycle != "starting" {
		t.Fatalf("scoped reconciliation changed executions: orphan=%#v other=%#v", orphan, other)
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
