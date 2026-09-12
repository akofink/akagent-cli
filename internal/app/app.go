package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/akofink/akagent-cli/internal/output"
	updatecmd "github.com/akofink/akagent-cli/internal/update"
	"github.com/google/uuid"
)

const description = "Manage local coding-agent tasks with durable state and explicit recovery records"

type homeView struct {
	Bin         string   `json:"bin"`
	Description string   `json:"description"`
	Tasks       []string `json:"tasks"`
	Help        []string `json:"help"`
}

type identifierView struct {
	ID string `json:"id"`
}

type helpView struct {
	Usage    string   `json:"usage"`
	Commands []string `json:"commands"`
}

type workerView struct {
	Worker worker `json:"worker"`
}

type updateView struct {
	Update updatecmd.Result `json:"update"`
}

type worker struct {
	ID              string   `json:"id"`
	ProtocolVersion int      `json:"protocol_version"`
	Architecture    string   `json:"architecture"`
	OperatingSystem string   `json:"operating_system"`
	Features        []string `json:"features"`
}

func Run(args []string, stdout io.Writer) int {
	if len(args) == 0 {
		return write(stdout, home())
	}
	if len(args) == 1 && args[0] == "--help" {
		return write(stdout, helpView{
			Usage: "akagent <command>",
			Commands: []string{
				"integration inspect",
				"id generate",
				"repository register <name> <path> [--policy <worktree|direct>] [--worktree-root <absolute-path>]",
				"repository update <name> [--path <path>] [--policy <worktree|direct>] [--worktree-root <absolute-path>]",
				"repository <list|inspect|unregister>",
				"task <create|checkpoint|resource|execution|list|inspect|publish|finish|archive|reconcile>",
				"task checkpoint <write|inspect> <task-id> ...",
				"task disposition <task-id> <in-flight|deferred|terminal> --reason <reason> [--expected-revision <revision>]",
				"task list [keyword] [--view <in-flight|attention|maintenance|deferred|history>] [--all] [--format <toon|human>]",
				"task execution session add <task-id> <execution-id> --tool <tool> --session-id <id> [--reference-path <path>]",
				"task execution evidence <list|inspect> <task-id> <execution-id> [<capture-id>]",
				"update [--source <path>]",
				"worker inspect",
			},
		})
	}

	switch args[0] {
	case "credential":
		return removedCommandError(stdout, "credential", "Record credential references as historical metadata; external tools own credential readiness and cleanup")
	case "integration":
		return integrationCommand(args[1:], stdout)
	case "id":
		if len(args) == 2 && args[1] == "generate" {
			id, err := uuid.NewV7()
			if err != nil {
				return writeError(stdout, "internal", "Failed to generate a task ID", false, "Retry `akagent id generate`")
			}
			return write(stdout, identifierView{ID: id.String()})
		}
	case "repository":
		return repositoryCommand(args[1:], stdout)
	case "task":
		return taskCommand(args[1:], stdout)
	case "worker":
		if len(args) > 1 && (args[1] == "launch" || args[1] == "launch-pi" || args[1] == "deploy") {
			return removedCommandError(stdout, "worker "+args[1], "Create a record-only execution and use an external tool for process or deployment work")
		}
		if len(args) == 2 && args[1] == "inspect" {
			return write(stdout, workerView{Worker: inspectWorker()})
		}
	case "update":
		sourceDir, valid := updateSource(args)
		if !valid {
			return writeError(stdout, "usage", "Usage: akagent update [--source <path>]", false, "Run `akagent update --source ~/dev/repos/akagent-cli`")
		}
		executable, err := os.Executable()
		if err != nil {
			return writeError(stdout, "internal", "Failed to resolve the installed akagent binary", false, "Reinstall akagent through machine setup")
		}
		result, updateErr := updatecmd.Run(sourceDir, executable)
		if updateErr != nil {
			return writeError(stdout, updateErr.Category, updateErr.Message, updateErr.Retryable, updateErr.Recovery)
		}
		return write(stdout, updateView{Update: result})
	}

	return writeError(stdout, "usage", fmt.Sprintf("Unknown command: %s", formatArgs(args)), false, "Run `akagent --help`")
}

func updateSource(args []string) (string, bool) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	if len(args) == 1 {
		if configured := os.Getenv("AKAGENT_SOURCE_DIR"); configured != "" {
			return configured, true
		}
		return filepath.Join(homeDir, "dev", "repos", "akagent-cli"), true
	}
	if len(args) == 3 && args[1] == "--source" && args[2] != "" {
		return args[2], true
	}
	return "", false
}

func inspectWorker() worker {
	return worker{
		ID:              "local",
		ProtocolVersion: 2,
		Architecture:    runtime.GOARCH,
		OperatingSystem: runtime.GOOS,
		Features:        []string{"registry", "checkpoint", "observation"},
	}
}

func home() homeView {
	bin, err := os.Executable()
	if err != nil {
		bin = "akagent"
	} else if homeDir, homeErr := os.UserHomeDir(); homeErr == nil {
		if relative, relativeErr := filepath.Rel(homeDir, bin); relativeErr == nil && relative != "." && relative != ".." && !filepath.IsAbs(relative) {
			bin = filepath.Join("~", relative)
		}
	}

	return homeView{
		Bin:         bin,
		Description: description,
		Tasks:       []string{},
		Help: []string{
			"Use `akagent task ...` directly for self-service task lifecycle management",
			"Run `akagent integration inspect` only to inspect the optional automation signal",
			"Run `akagent id generate` to create a task ID",
			"Run `akagent update` to update from the local source checkout",
			"Run `akagent worker inspect` to inspect the local worker",
		},
	}
}

func removedCommandError(stdout io.Writer, family, recovery string) int {
	return writeError(stdout, "usage", fmt.Sprintf("The `%s` command family was removed from akagent", family), false, recovery)
}

func formatArgs(args []string) string {
	if len(args) == 0 {
		return "<none>"
	}
	return fmt.Sprintf("%q", args)
}

func write(stdout io.Writer, value any) int {
	if err := output.Write(stdout, value); err != nil {
		if output.WriteError(stdout, "internal", "Failed to serialize protocol output", false, "Retry the command") != nil {
			return 1
		}
		return 1
	}
	return 0
}

func writeError(stdout io.Writer, category, message string, retryable bool, recovery string) int {
	if err := output.WriteError(stdout, category, message, retryable, recovery); err != nil {
		return 1
	}
	if category == "usage" {
		return 2
	}
	return 1
}
