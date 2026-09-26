// Package derived performs opt-in read-only checks against live external resources.
package derived

import (
	"context"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/akofink/akagent-cli/internal/store"
)

// Runner makes external reads replaceable in tests. Never pass secrets or a shell command.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type CommandRunner struct{}

func (CommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

const (
	StateCurrent = "current"
	StateStale   = "stale"
	StateMissing = "missing"
	StateUnknown = "unknown"
)

const resourceTimeout = 20 * time.Second

type Finding struct {
	Surface string `json:"surface"`
	State   string `json:"state"`
	Code    string `json:"code"`
	Detail  string `json:"detail,omitempty"`
	Action  string `json:"action"`
}

// NeedsAction reports whether the finding asks an agent or operator to decide something.
func (f Finding) NeedsAction() bool { return f.State == StateStale || f.State == StateMissing }

type Resource struct {
	ID       string    `json:"id"`
	Findings []Finding `json:"findings"`
}

type Summary struct {
	Current     int `json:"current"`
	Stale       int `json:"stale"`
	Missing     int `json:"missing"`
	Unknown     int `json:"unknown"`
	NeedsAction int `json:"needs_action"`
}

func (s *Summary) add(f Finding) {
	switch f.State {
	case StateCurrent:
		s.Current++
	case StateStale:
		s.Stale++
	case StateMissing:
		s.Missing++
	default:
		s.Unknown++
	}
	if f.NeedsAction() {
		s.NeedsAction++
	}
}

func (s *Summary) merge(other Summary) {
	s.Current += other.Current
	s.Stale += other.Stale
	s.Missing += other.Missing
	s.Unknown += other.Unknown
	s.NeedsAction += other.NeedsAction
}

type Report struct {
	TaskID     string     `json:"task_id"`
	Summary    Summary    `json:"summary"`
	Task       []Finding  `json:"task,omitempty"`
	Resources  []Resource `json:"resources,omitempty"`
	Executions []Finding  `json:"executions,omitempty"`
}

type StoreReport struct {
	Checked int      `json:"checked"`
	Summary Summary  `json:"summary"`
	Tasks   []Report `json:"tasks,omitempty"`
}

// Snapshot is the cached record state a check starts from. It is authoritative only for bindings and claims.
type Snapshot struct {
	TaskID         string
	Task           store.Manifest
	Resources      []store.Resource
	Executions     []store.Execution
	RepositoryPath func(name string) string
	// Unreadable marks a task whose cached records could not be loaded.
	Unreadable bool
}

type Options struct {
	Offline bool
}

// Check never mutates records, checkouts, the forge, panes, or provider state.
func Check(snapshot Snapshot, runner Runner, options Options) Report {
	terminal := TaskTerminal(snapshot.Task)
	report := Report{TaskID: snapshot.TaskID}
	if snapshot.Unreadable {
		report.Task = []Finding{finding("task", StateStale, "record_unreadable", "Inspect and reconcile the task records")}
		report.Summary.add(report.Task[0])
		return report
	}
	resources := append([]store.Resource(nil), snapshot.Resources...)
	sort.Slice(resources, func(i, j int) bool { return resources[i].ID < resources[j].ID })
	merged := 0
	for _, resource := range resources {
		if options.Offline {
			report.Resources = append(report.Resources, Resource{ID: resource.ID, Findings: offlineFindings()})
			continue
		}
		path := ""
		if snapshot.RepositoryPath != nil {
			path = snapshot.RepositoryPath(resource.Repository)
		}
		ctx, cancel := context.WithTimeout(context.Background(), resourceTimeout)
		git := checkGit(ctx, runner, resource, path, terminal)
		github := checkGitHub(ctx, runner, resource, git)
		cancel()
		if github.PR.Code == "merged" {
			merged++
		}
		report.Resources = append(report.Resources, Resource{ID: resource.ID, Findings: []Finding{git.Worktree, git.Branch, github.PR, github.Checks}})
	}
	if !terminal && len(resources) > 0 && merged == len(resources) {
		report.Task = append(report.Task, finding("task", StateStale, "delivered_unfinished", "Verify the delivery contract, then finish the task"))
	}
	executions := append([]store.Execution(nil), snapshot.Executions...)
	sort.Slice(executions, func(i, j int) bool { return executions[i].ID < executions[j].ID })
	open := 0
	for _, execution := range executions {
		if !ExecutionClosed(execution) {
			open++
		}
		report.Executions = append(report.Executions, checkExecution(execution, terminal))
	}
	if !terminal && snapshot.Task.Condition == "active" && len(executions) > 0 && open == 0 {
		report.Task = append(report.Task, finding("task", StateStale, "no_open_execution", "Resume with a new execution or finish the task against its contract"))
	}
	for _, f := range report.Task {
		report.Summary.add(f)
	}
	for _, resource := range report.Resources {
		for _, f := range resource.Findings {
			report.Summary.add(f)
		}
	}
	for _, f := range report.Executions {
		report.Summary.add(f)
	}
	return report
}

// CheckStore runs record-consistency rules for every snapshot and live adapters only for open tasks.
// It keeps only tasks and findings that need action; the summary still counts every finding.
func CheckStore(snapshots []Snapshot, runner Runner, options Options) StoreReport {
	ordered := append([]Snapshot(nil), snapshots...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].TaskID < ordered[j].TaskID })
	result := StoreReport{Checked: len(ordered)}
	for _, snapshot := range ordered {
		taskOptions := options
		if TaskTerminal(snapshot.Task) {
			taskOptions.Offline = true
			snapshot.Resources = nil
		}
		report := Check(snapshot, runner, taskOptions)
		result.Summary.merge(report.Summary)
		if filtered, ok := actionable(report); ok {
			result.Tasks = append(result.Tasks, filtered)
		}
	}
	return result
}

func actionable(report Report) (Report, bool) {
	filtered := Report{TaskID: report.TaskID, Summary: report.Summary}
	filtered.Task = keepActionable(report.Task)
	for _, resource := range report.Resources {
		if findings := keepActionable(resource.Findings); len(findings) > 0 {
			filtered.Resources = append(filtered.Resources, Resource{ID: resource.ID, Findings: findings})
		}
	}
	filtered.Executions = keepActionable(report.Executions)
	return filtered, report.Summary.NeedsAction > 0
}

func keepActionable(findings []Finding) []Finding {
	var kept []Finding
	for _, f := range findings {
		if f.NeedsAction() {
			kept = append(kept, f)
		}
	}
	return kept
}

// TaskTerminal reports whether a task has an explicit terminal record.
func TaskTerminal(task store.Manifest) bool {
	return task.Lifecycle == "finished" || task.Lifecycle == "stopped" || task.ExternalCompletion != nil || task.ArchiveState == "complete"
}

// ExecutionClosed reports whether an execution has an explicit closing record.
func ExecutionClosed(execution store.Execution) bool {
	return execution.Lifecycle == "finished" || execution.Lifecycle == "stopped" || execution.ExternalCompletion != nil || execution.HandoffDisposition != nil || execution.ArchiveState == "complete"
}

func checkExecution(execution store.Execution, taskTerminal bool) Finding {
	surface := "execution:" + execution.ID
	switch {
	case ExecutionClosed(execution):
		return finding(surface, StateCurrent, "closed", "No execution action needed")
	case taskTerminal:
		return finding(surface, StateStale, "task_terminal", "Verify the attempt has ended, then run `task execution finish` with revision "+strconv.FormatUint(execution.Revision, 10))
	}
	result := finding(surface, StateUnknown, "adapter_unavailable", "Verify the agent and its session on the owning host")
	result.Detail = sessionTools(execution.SessionReferences)
	return result
}

func sessionTools(references []store.SessionReference) string {
	seen := map[string]bool{}
	var tools []string
	for _, reference := range references {
		if reference.Tool != "" && !seen[reference.Tool] {
			seen[reference.Tool] = true
			tools = append(tools, reference.Tool)
		}
	}
	sort.Strings(tools)
	return strings.Join(tools, ",")
}

func offlineFindings() []Finding {
	findings := make([]Finding, 0, 4)
	for _, surface := range []string{"worktree", "branch", "pull_request", "checks"} {
		findings = append(findings, finding(surface, StateUnknown, "offline", "Run without --offline to observe live state"))
	}
	return findings
}

func finding(surface, state, code, action string) Finding {
	return Finding{Surface: surface, State: state, Code: code, Action: action}
}

func trimmed(value []byte) string { return strings.TrimSpace(string(value)) }
