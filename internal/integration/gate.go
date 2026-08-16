// Package integration defines the default-on gate for automated akagent integrations.
package integration

import (
	"os"

	"github.com/akofink/akagent-cli/internal/lifecycle"
	"github.com/akofink/akagent-cli/internal/store"
)

const EnableEnv = "AKAGENT_ENABLED"

type Status struct {
	Enabled bool   `json:"enabled"`
	Signal  string `json:"signal"`
	Reason  string `json:"reason"`
}

func Inspect() Status {
	value, set := os.LookupEnv(EnableEnv)
	return InspectValue(value, set)
}

func InspectValue(value string, set bool) Status {
	status := Status{Signal: EnableEnv}
	switch {
	case set && value == "0":
		status.Reason = EnableEnv + " is set to 0"
	case !set:
		status.Enabled = true
		status.Reason = EnableEnv + " is unset"
	default:
		status.Enabled = true
		status.Reason = EnableEnv + " is not set to 0"
	}
	return status
}

func Enabled() bool {
	return Inspect().Enabled
}

// RecordSessionReference lets an optional provider integration publish its own
// session provenance without making the core CLI parse provider state files.
func RecordSessionReference(manager *lifecycle.Manager, taskID, executionID string, reference store.SessionReference) (store.Execution, error) {
	return manager.AddExecutionSessionReference(taskID, executionID, reference)
}
