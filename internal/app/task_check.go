package app

import (
	"io"

	"github.com/akofink/akagent-cli/internal/derived"
	"github.com/akofink/akagent-cli/internal/lifecycle"
	"github.com/akofink/akagent-cli/internal/store"
)

const taskCheckUsage = "Usage: akagent task check <task-id|keyword|--all> [--offline] [--format <toon|json>]"

// checkRunner is replaced in tests so check commands never reach Git or the network.
var checkRunner derived.Runner = derived.CommandRunner{}

func taskCheckCommand(args []string, stdout io.Writer, state *store.Store, manager *lifecycle.Manager) int {
	var target, format string
	all, offline := false, false
	for len(args) > 0 {
		switch args[0] {
		case "--all":
			all = true
			args = args[1:]
		case "--offline":
			offline = true
			args = args[1:]
		case "--format":
			if len(args) < 2 || format != "" || (args[1] != "toon" && args[1] != "json") {
				return writeError(stdout, "usage", taskCheckUsage, false, "Use --format toon or --format json")
			}
			format = args[1]
			args = args[2:]
		default:
			if target != "" || len(args[0]) > 1 && args[0][0] == '-' {
				return writeError(stdout, "usage", taskCheckUsage, false, "Inspect the task first if its identity is uncertain")
			}
			target = args[0]
			args = args[1:]
		}
	}
	if all == (target != "") {
		return writeError(stdout, "usage", taskCheckUsage, false, "Pass one task ID or --all")
	}
	options := derived.Options{Offline: offline}
	var result any
	if all {
		ids, err := state.TaskIDs()
		if err != nil {
			return lifecycleError(stdout, err)
		}
		snapshots := make([]derived.Snapshot, 0, len(ids))
		for _, id := range ids {
			snapshot, err := checkSnapshot(manager, id)
			if err != nil {
				snapshot = derived.Snapshot{TaskID: id, Unreadable: true}
			}
			snapshots = append(snapshots, snapshot)
		}
		result = derived.CheckStore(snapshots, checkRunner, options)
	} else {
		taskID, err := resolveTaskID(state, manager, target)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		snapshot, err := checkSnapshot(manager, taskID)
		if err != nil {
			return lifecycleError(stdout, err)
		}
		result = derived.Check(snapshot, checkRunner, options)
	}
	if format == "json" {
		return writeJSON(stdout, result)
	}
	return write(stdout, result)
}

func checkSnapshot(manager *lifecycle.Manager, taskID string) (derived.Snapshot, error) {
	task, err := manager.Inspect(taskID)
	if err != nil {
		return derived.Snapshot{}, err
	}
	resources, err := manager.ListResources(taskID)
	if err != nil {
		return derived.Snapshot{}, err
	}
	executions, err := manager.ListExecutions(taskID)
	if err != nil {
		return derived.Snapshot{}, err
	}
	repositoryPath := func(name string) string {
		repository, err := manager.InspectRepository(name)
		if err != nil {
			return ""
		}
		return repository.Path
	}
	return derived.Snapshot{TaskID: taskID, Task: task, Resources: resources, Executions: executions, RepositoryPath: repositoryPath}, nil
}
