package store

import (
	"fmt"
	"os"
	"strings"
)

// maxHandoffTaskHints bounds the read-only cross-task lookup used only to
// explain a refused handoff.
const maxHandoffTaskHints = 3

// handoffRefusals collects every failed handoff precondition so one refusal
// names them all, each paired with its recovery.
type handoffRefusals struct {
	messages   []string
	recoveries []string
}

func (r *handoffRefusals) add(message, recovery string) {
	r.messages = append(r.messages, message)
	r.recoveries = append(r.recoveries, recovery)
}

func (r *handoffRefusals) empty() bool {
	return len(r.messages) == 0
}

func (r *handoffRefusals) err(executionID string) error {
	if r.empty() {
		return nil
	}
	return newError(KindConflict,
		fmt.Sprintf("Handoff of execution %s refused: %s", executionID, numberedClauses(r.messages)),
		numberedClauses(r.recoveries))
}

func numberedClauses(values []string) string {
	if len(values) == 1 {
		return values[0]
	}
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = fmt.Sprintf("(%d) %s", i+1, value)
	}
	return strings.Join(parts, "; ")
}

// checkHandoffSuccessorLocked records why the named successor cannot accept a
// handoff on taskID and reports whether it is an execution of that task. It
// returns an error only for unreadable state.
func (s *Store) checkHandoffSuccessorLocked(taskID, successorID string, refusals *handoffRefusals) (bool, error) {
	successor, err := s.ReadExecution(taskID, successorID)
	if IsKind(err, KindNotFound) {
		message := fmt.Sprintf("successor execution %s is not an execution of task %s", successorID, taskID)
		if others := s.executionTaskIDs(successorID, taskID); len(others) > 0 {
			message += fmt.Sprintf(" (it is recorded on task %s)", strings.Join(others, ", "))
		} else {
			message += " (no execution with that ID is recorded)"
		}
		refusals.add(message, fmt.Sprintf("A handoff successor must be a new execution of the same task: begin it from the successor's own pane with `akagent task begin %s --title <exact task title> --execution-id <new-id> --command <label>`; a successor on another task cannot take a handoff, so close the predecessor with an operator-directed `akagent task execution finish` after independently verifying it ended", taskID))
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var problems []string
	if successor.Provenance == ProvenanceExternal {
		problems = append(problems, "external provenance")
	}
	if detail := ClosedExecutionDetail(successor); detail != "" {
		problems = append(problems, "already closed: "+detail)
	}
	if successor.Condition != "active" {
		problems = append(problems, "condition "+displayValue(successor.Condition))
	}
	if len(problems) > 0 {
		refusals.add(
			fmt.Sprintf("successor execution %s is not an active managed execution (%s)", successorID, strings.Join(problems, ", ")),
			fmt.Sprintf("Hand off only to a managed execution of task %s that has begun and is published active (`akagent task begin` does both); inspect it with `akagent task execution inspect %s %s`", taskID, taskID, successorID),
		)
	}
	return true, nil
}

// executionTaskIDs lists other tasks that hold an execution with this ID. It
// only probes paths and ignores unreadable entries because it explains a
// refusal; it never selects a record to mutate.
func (s *Store) executionTaskIDs(executionID, exclude string) []string {
	taskIDs, err := s.TaskIDs()
	if err != nil {
		return nil
	}
	var matches []string
	for _, taskID := range taskIDs {
		if taskID == exclude {
			continue
		}
		if _, err := os.Lstat(s.executionManifestPath(taskID, executionID)); err == nil {
			matches = append(matches, taskID)
			if len(matches) == maxHandoffTaskHints {
				break
			}
		}
	}
	return matches
}

// ClosedExecutionDetail describes how an execution was closed, or returns ""
// when it is still open.
func ClosedExecutionDetail(execution Execution) string {
	var parts []string
	switch {
	case execution.HandoffDisposition != nil:
		parts = append(parts, "handed off to successor "+execution.HandoffDisposition.SuccessorExecutionID)
	case execution.ExternalCompletion != nil:
		parts = append(parts, fmt.Sprintf("finished with contract %s, result %s", execution.ExternalCompletion.Contract, execution.ExternalCompletion.Result))
	case execution.Lifecycle == "finished":
		parts = append(parts, "finished, result "+displayValue(execution.Result))
	case execution.Lifecycle == "stopped":
		parts = append(parts, "stopped")
	}
	if execution.ArchiveState == "complete" {
		parts = append(parts, "archived")
	}
	return strings.Join(parts, ", ")
}

func terminalTaskDetail(manifest Manifest) string {
	var parts []string
	if manifest.Lifecycle == "finished" {
		parts = append(parts, "finished, result "+displayValue(manifest.Result))
	}
	if manifest.ArchiveState == "complete" {
		parts = append(parts, "archived")
	}
	return strings.Join(parts, ", ")
}

func displayValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unset"
	}
	return value
}
