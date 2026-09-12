package app

import (
	"io"

	"github.com/akofink/akagent-cli/internal/integration"
)

type integrationInspectView struct {
	Integration integration.Status `json:"integration"`
}

func integrationCommand(args []string, stdout io.Writer) int {
	if len(args) == 1 && args[0] == "inspect" {
		return write(stdout, integrationInspectView{Integration: integration.Inspect()})
	}
	if len(args) > 0 && args[0] == "launch" {
		return removedCommandError(stdout, "integration launch", "Create a record-only execution and use an external tool for process or deployment work")
	}
	return writeError(stdout, "usage", "Usage: akagent integration inspect", false, "Run `akagent integration inspect`")
}
