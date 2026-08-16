package integration

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/akofink/akagent-cli/internal/credential"
	"github.com/akofink/akagent-cli/internal/lifecycle"
	"github.com/akofink/akagent-cli/internal/store"
	"github.com/google/uuid"
)

const PiTarget = "pi"

// LaunchRequest is the compatibility request for the optional Pi integration.
// The task and resource lifecycle remains independent of this request.
type LaunchRequest struct {
	Label           string
	ResourceID      string
	PromptReference string
	WorkingContext  string
}

// Launch creates and starts a generic execution for the optional Pi
// integration. Pi is checked only when this integration is selected, so task
// and resource commands do not require Pi to be installed.
func Launch(manager *lifecycle.Manager, taskID string, request LaunchRequest) (store.Execution, error) {
	if _, err := exec.LookPath("pi"); err != nil {
		return store.Execution{}, &store.Error{Kind: store.KindConflict, Message: "Pi agent command is unavailable", Recovery: "Install Pi or use `--target shell`"}
	}
	prompt := request.PromptReference
	if prompt != "" {
		resolved, err := filepath.Abs(prompt)
		if err != nil {
			return store.Execution{}, errors.New("resolve the prompt reference")
		}
		prompt = resolved
		if err := validatePrompt(prompt); err != nil {
			return store.Execution{}, err
		}
	}
	if strings.ContainsAny(request.WorkingContext, "\r\n") {
		return store.Execution{}, errors.New("working context must be a single non-secret line")
	}

	resourceID, err := selectResource(manager, taskID, request.ResourceID)
	if err != nil {
		return store.Execution{}, err
	}
	label, err := manager.ResolveCompatibilityExecutionLabel(taskID, resourceID, request.Label)
	if err != nil {
		return store.Execution{}, err
	}
	executionID, err := uuid.NewV7()
	if err != nil {
		return store.Execution{}, errors.New("failed to generate an execution ID")
	}
	executable, err := os.Executable()
	if err != nil || executable == "" {
		return store.Execution{}, errors.New("akagent executable could not be resolved")
	}
	arguments := []string{"worker", "launch-pi", taskID, executionID.String(), "--"}
	if prompt != "" {
		arguments = append(arguments, "@"+prompt)
	}
	if request.WorkingContext != "" {
		arguments = append(arguments, "--context", request.WorkingContext)
	}
	execution, _, err := manager.CreateExecution(taskID, lifecycle.ExecutionRequest{
		ID:         executionID.String(),
		Label:      label,
		Target:     PiTarget,
		Command:    executable,
		Arguments:  arguments,
		ResourceID: resourceID,
	})
	if err != nil {
		return store.Execution{}, err
	}
	return manager.LaunchExecutionRecord(taskID, execution.ID)
}

// LaunchShell creates and starts an explicitly requested direct human shell
// execution through the same generic execution primitives used by Pi.
func LaunchShell(manager *lifecycle.Manager, taskID, label, resourceID string) (store.Execution, error) {
	resourceID, err := selectResource(manager, taskID, resourceID)
	if err != nil {
		return store.Execution{}, err
	}
	label, err = manager.ResolveCompatibilityExecutionLabel(taskID, resourceID, label)
	if err != nil {
		return store.Execution{}, err
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	execution, _, err := manager.CreateExecution(taskID, lifecycle.ExecutionRequest{
		Target:     "shell",
		Label:      label,
		Command:    shell,
		ResourceID: resourceID,
	})
	if err != nil {
		return store.Execution{}, err
	}
	return manager.LaunchExecutionRecord(taskID, execution.ID)
}

func selectResource(manager *lifecycle.Manager, taskID, requested string) (string, error) {
	if requested != "" {
		if _, err := manager.InspectResource(taskID, requested); err != nil {
			return "", err
		}
		return requested, nil
	}
	resources, err := manager.ListResources(taskID)
	if err != nil {
		return "", err
	}
	if len(resources) > 1 {
		return "", &store.Error{Kind: store.KindConflict, Message: "execution requires a selected resource when a task has multiple resources", Recovery: "Retry with `--resource <resource-id>`"}
	}
	if len(resources) == 1 {
		return resources[0].ID, nil
	}
	return "", nil
}

func validatePrompt(reference string) error {
	if reference == "" {
		return nil
	}
	prompt, err := filepath.Abs(reference)
	if err != nil {
		return errors.New("resolve the prompt reference")
	}
	info, err := os.Lstat(prompt)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("prompt reference must identify a regular local file")
	}
	return nil
}

// LaunchPi is the worker entrypoint for the optional integration. It replaces
// the worker process with Pi, preserving the process identity recorded for the
// generic execution. The argument list after `--` is integration-owned and
// contains only a prompt file reference and non-secret context.
func LaunchPi(manager *lifecycle.Manager, taskID, executionID string, args []string, stderr io.Writer, execAgent func(string, []string, []string) error) error {
	execution, err := manager.InspectExecution(taskID, executionID)
	if err != nil {
		return err
	}
	if execution.Target != PiTarget {
		return markPiFailure(manager, taskID, executionID, "execution target is not Pi")
	}
	command, err := exec.LookPath("pi")
	if err != nil {
		return markPiFailure(manager, taskID, executionID, "Pi agent command is unavailable")
	}
	prompt, context, err := parsePiArguments(args)
	if err != nil {
		return markPiFailure(manager, taskID, executionID, err.Error())
	}
	if err := validatePrompt(prompt); err != nil {
		return markPiFailure(manager, taskID, executionID, err.Error())
	}
	if strings.ContainsAny(context, "\r\n") {
		return markPiFailure(manager, taskID, executionID, "working context must be a single non-secret line")
	}
	if err := os.Chdir(execution.WorkingDirectory); err != nil {
		return markPiFailure(manager, taskID, executionID, "task worktree unavailable")
	}
	if manager.Credentials == nil {
		return markPiFailure(manager, taskID, executionID, "credential manifest unavailable")
	}
	credentials, err := manager.Credentials()
	if err != nil {
		return markPiFailure(manager, taskID, executionID, "credential manifest unavailable")
	}
	environment, err := buildEnvironment(credentials, taskRequirements(manager, taskID))
	if err != nil {
		return markPiFailure(manager, taskID, executionID, "requested credential unavailable")
	}
	environment = append(environment, "AKAGENT_TASK_ID="+taskID, "AKAGENT_EXECUTION_ID="+executionID)
	if context != "" {
		environment = append(environment, "AKAGENT_WORKING_CONTEXT="+context)
	}
	if stderr != nil {
		_, _ = fmt.Fprintf(stderr, "akagent: starting managed Pi execution %s\n", executionID)
	}
	if execAgent == nil {
		execAgent = syscall.Exec
	}
	piArgs := []string{command}
	if prompt != "" {
		piArgs = append(piArgs, "@"+prompt)
	}
	if err := execAgent(command, piArgs, environment); err != nil {
		return markPiFailure(manager, taskID, executionID, "agent process could not be started")
	}
	return nil
}

func taskRequirements(manager *lifecycle.Manager, taskID string) []string {
	manifest, err := manager.Inspect(taskID)
	if err != nil || manifest.Requirements == "" {
		return nil
	}
	return strings.Split(manifest.Requirements, ",")
}

func buildEnvironment(manifest *credential.Manifest, requirements []string) ([]string, error) {
	return credential.BuildEnvironment(manifest, requirements, os.Environ())
}

func parsePiArguments(args []string) (string, string, error) {
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		return "", "", nil
	}
	prompt := ""
	context := ""
	for index := 0; index < len(args); index++ {
		switch {
		case strings.HasPrefix(args[index], "@") && prompt == "":
			prompt = strings.TrimPrefix(args[index], "@")
		case args[index] == "--context" && index+1 < len(args):
			context = args[index+1]
			index++
		default:
			return "", "", errors.New("invalid Pi integration arguments")
		}
	}
	return prompt, context, nil
}

func markPiFailure(manager *lifecycle.Manager, taskID, executionID, detail string) error {
	_, updateErr := manager.Store.UpdateExecution(taskID, executionID, func(execution *store.Execution) error {
		execution.Lifecycle = "starting"
		execution.Observation = lifecycle.ObservationMissing
		execution.ObservedPID, execution.ObservedStartTime = 0, 0
		execution.ProcessPID, execution.ProcessStartTime = 0, 0
		if execution.RecoveryDebt == "" {
			execution.RecoveryDebt = "launch_failed"
		} else if !strings.Contains(";"+execution.RecoveryDebt+";", ";launch_failed;") {
			execution.RecoveryDebt += ";launch_failed"
		}
		return nil
	})
	if updateErr != nil {
		return errors.New("managed Pi launch failed and could not be recorded")
	}
	_, _ = manager.Store.AppendExecutionEvent(taskID, executionID, store.Event{Operation: "launch", Outcome: "failed", Detail: detail})
	return errors.New("managed Pi launch failed; retry the execution")
}
