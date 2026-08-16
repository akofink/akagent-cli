package lifecycle

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/akofink/akagent-cli/internal/credential"
	"github.com/akofink/akagent-cli/internal/store"
)

// DeploymentTarget identifies a local command executed with work-scoped
// credentials. The command and credential IDs are durable metadata; values are
// resolved only by RunDeployment in the worker process.
const DeploymentTarget = "deploy"

// CreateDeployment records a local deployment attempt without starting a
// process. Requirements are credential IDs, never credential values.
func (m *Manager) CreateDeployment(taskID string, request ExecutionRequest) (store.Execution, bool, error) {
	if request.Target == "" {
		request.Target = DeploymentTarget
	}
	if request.Target != DeploymentTarget {
		return store.Execution{}, false, fmt.Errorf("deployment target must be %s", DeploymentTarget)
	}
	if request.Command == "" {
		return store.Execution{}, false, &store.Error{Kind: store.KindUsage, Message: "deployment command is required", Recovery: "Retry with `--command <executable>`"}
	}
	if request.Label == "" {
		request.Label = "deployment"
	}
	return m.CreateExecution(taskID, request)
}

func (m *Manager) blockDeployment(execution store.Execution, taskID string) {
	_, _ = m.Store.UpdateExecution(taskID, execution.ID, func(current *store.Execution) error {
		current.Condition = "blocked"
		current.Reason = "deployment credential readiness failed"
		current.RecoveryDebt = addDebt(current.RecoveryDebt, "credential_unavailable")
		current.HeartbeatAt = m.now()
		return nil
	})
	_, _ = m.Store.AppendExecutionEvent(taskID, execution.ID, store.Event{Operation: "deploy", Outcome: "blocked"})
}

func (m *Manager) checkDeploymentCredentials(execution store.Execution) error {
	if execution.Requirements == "" {
		return nil
	}
	if m.Credentials == nil {
		return errors.New("credential manifest could not be loaded")
	}
	manifest, err := m.Credentials()
	if err != nil || manifest == nil {
		return errors.New("credential manifest could not be loaded")
	}
	checker := m.Checker
	if checker == nil {
		checker = credential.NewChecker()
	}
	checks := credential.Doctor(manifest, checker)
	byID := make(map[string]credential.Check, len(checks))
	for _, check := range checks {
		byID[check.Entry.ID] = check
	}
	for _, id := range splitRequirements(execution.Requirements) {
		check, ok := byID[id]
		if !ok || check.Status != credential.Ready {
			return fmt.Errorf("required deployment credential %s is unavailable", id)
		}
		if check.Entry.Kind() != credential.KindEnv {
			return fmt.Errorf("required deployment credential %s cannot be injected into the local deployment environment", id)
		}
	}
	return nil
}

// RunDeployment is the local deployment worker entrypoint. It reads only
// non-secret execution metadata from the store, builds a minimal environment,
// and runs the command. Completion is recorded before the worker exits so an
// ordinary process exit is distinguishable from an interrupted deployment.
func (m *Manager) RunDeployment(taskID, executionID string) error {
	execution, err := m.InspectExecution(taskID, executionID)
	if err != nil {
		return err
	}
	if execution.Target != DeploymentTarget {
		return errors.New("execution target is not a local deployment")
	}
	if err := m.checkDeploymentCredentials(execution); err != nil {
		return markDeploymentFailure(m, taskID, executionID, "deployment credentials are unavailable", err)
	}
	if m.Credentials == nil {
		return markDeploymentFailure(m, taskID, executionID, "credential manifest could not be loaded", nil)
	}
	manifest, err := m.Credentials()
	if err != nil || manifest == nil {
		return markDeploymentFailure(m, taskID, executionID, "credential manifest could not be loaded", err)
	}
	environment, err := credential.BuildEnvironment(manifest, splitRequirements(execution.Requirements), os.Environ())
	if err != nil {
		return markDeploymentFailure(m, taskID, executionID, "deployment credentials could not be prepared", err)
	}
	environment = append(environment, "AKAGENT_TASK_ID="+taskID, "AKAGENT_EXECUTION_ID="+executionID)
	command := exec.Command(execution.Command, execution.Arguments...)
	command.Dir = execution.WorkingDirectory
	command.Env = environment
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	runErr := command.Run()
	if runErr != nil {
		return markDeploymentFailure(m, taskID, executionID, "deployment command failed", runErr)
	}
	if err := completeDeployment(m, taskID, executionID, "succeeded", "deployment succeeded"); err != nil {
		return err
	}
	return nil
}

func markDeploymentFailure(manager *Manager, taskID, executionID, detail string, cause error) error {
	if err := completeDeployment(manager, taskID, executionID, "failed", "deployment failed"); err != nil {
		return errors.New("deployment failed and its result could not be recorded")
	}
	_ = cause
	return errors.New(detail)
}

func completeDeployment(manager *Manager, taskID, executionID, outcome, result string) error {
	_, err := manager.Store.UpdateExecution(taskID, executionID, func(execution *store.Execution) error {
		execution.Lifecycle = "finished"
		execution.Condition = outcome
		execution.Result = result
		execution.Observation = ObservationMissing
		execution.ObservationAt = manager.now()
		execution.ObservedPID, execution.ObservedStartTime = 0, 0
		return nil
	})
	if err != nil {
		return err
	}
	_, err = manager.Store.AppendExecutionEvent(taskID, executionID, store.Event{Operation: "deploy", Outcome: outcome})
	manager.publishExecutionStateValue(store.Execution{ID: executionID, TaskID: taskID, Lifecycle: "finished"}, "done")
	return err
}
