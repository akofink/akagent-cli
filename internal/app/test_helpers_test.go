package app

import (
	"bytes"
	"testing"
)

type commandResult struct {
	code   int
	stdout string
}

func runCommand(t *testing.T, args []string) commandResult {
	t.Helper()
	var stdout bytes.Buffer
	return commandResult{code: Run(args, &stdout), stdout: stdout.String()}
}

func setupTaskCommandTest(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_STATE_HOME", root+"/state")
	t.Setenv("HOME", root+"/home")
}
