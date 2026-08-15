package app

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/akofink/akagent-cli/internal/lifecycle"
	"github.com/akofink/akagent-cli/internal/output"
	"github.com/akofink/akagent-cli/internal/store"
	updatecmd "github.com/akofink/akagent-cli/internal/update"
	"github.com/google/uuid"
)

const description = "Orchestrate local coding agents through tmux and Git worktrees"

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
				"credential <list|inspect|doctor>",
				"integration inspect",
				"id generate",
				"repository register <name> <path> [--policy <worktree|direct>]",
				"repository <list|inspect|update|unregister>",
				"task <start|list|inspect|attach|publish|finish|stop|archive|clean|reconcile>",
				"update [--source <path>]",
				"worker inspect",
			},
		})
	}

	switch args[0] {
	case "credential":
		return credentialCommand(args[1:], stdout)
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
		if len(args) == 3 && args[1] == "launch" {
			state, err := store.Open()
			if err != nil {
				return 1
			}
			if err := lifecycle.New(state).Launch(args[2]); err != nil {
				return 1
			}
			return 0
		}
		if len(args) == 2 && args[1] == "inspect" {
			return write(stdout, workerView{Worker: inspectWorker(exec.LookPath)})
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

func inspectWorker(lookPath func(string) (string, error)) worker {
	features := make([]string, 0, 2)
	if _, err := lookPath("tmux"); err == nil {
		features = append(features, "tmux")
	}
	if _, err := lookPath("git"); err == nil {
		features = append(features, "git-worktree")
	}

	return worker{
		ID:              "local",
		ProtocolVersion: 1,
		Architecture:    runtime.GOARCH,
		OperatingSystem: runtime.GOOS,
		Features:        features,
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
			"Run `akagent credential doctor` to check local credential readiness",
			"Run `akagent id generate` to create a task ID",
			"Run `akagent update` to update from the local source checkout",
			"Run `akagent worker inspect` to inspect the local worker",
		},
	}
}

func formatArgs(args []string) string {
	if len(args) == 0 {
		return "<none>"
	}
	return fmt.Sprintf("%q", args)
}

func write(stdout io.Writer, value any) int {
	if err := output.Write(stdout, value); err != nil {
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
