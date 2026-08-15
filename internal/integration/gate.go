// Package integration defines the opt-in gate for automated akagent integrations.
package integration

import "os"

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
	case !set:
		status.Reason = EnableEnv + " is unset"
	case value != "1":
		status.Reason = EnableEnv + " is set to a value other than 1"
	default:
		status.Enabled = true
		status.Reason = EnableEnv + " is set to 1"
	}
	return status
}

func Enabled() bool {
	return Inspect().Enabled
}
