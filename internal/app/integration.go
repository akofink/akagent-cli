package app

import (
	"io"

	"github.com/akofink/akagent-cli/internal/integration"
)

type integrationInspectView struct {
	Integration integration.Status `json:"integration"`
}

func integrationCommand(args []string, stdout io.Writer) int {
	if len(args) != 1 || args[0] != "inspect" {
		return writeError(stdout, "usage", "Usage: akagent integration inspect", false, "Run `akagent integration inspect`")
	}
	return write(stdout, integrationInspectView{Integration: integration.Inspect()})
}
