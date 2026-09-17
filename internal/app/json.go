package app

import (
	"io"

	"github.com/akofink/akagent-cli/internal/output"
)

func writeJSON(stdout io.Writer, value any) int {
	if err := output.WriteJSON(stdout, value); err != nil {
		if output.WriteError(stdout, "internal", "Failed to serialize protocol output", false, "Retry the command") != nil {
			return 1
		}
		return 1
	}
	return 0
}
