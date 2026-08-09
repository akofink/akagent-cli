package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/akofink/akagent-cli/internal/output"
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
				"id generate",
				"worker inspect",
			},
		})
	}

	switch args[0] {
	case "id":
		if len(args) == 2 && args[1] == "generate" {
			id, err := uuid.NewV7()
			if err != nil {
				return writeError(stdout, "internal", "Failed to generate a task ID", false, "Retry `akagent id generate`")
			}
			return write(stdout, identifierView{ID: id.String()})
		}
	case "worker":
		if len(args) == 2 && args[1] == "inspect" {
			return write(stdout, workerView{Worker: worker{
				ID:              "local",
				ProtocolVersion: 1,
				Architecture:    runtime.GOARCH,
				OperatingSystem: runtime.GOOS,
				Features:        []string{"tmux", "git-worktree"},
			}})
		}
	}

	return writeError(stdout, "usage", fmt.Sprintf("Unknown command: %s", formatArgs(args)), false, "Run `akagent --help`")
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
			"Run `akagent id generate` to create a task ID",
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
	if category == "usage" {
		_ = output.WriteError(stdout, category, message, retryable, recovery)
		return 2
	}
	_ = output.WriteError(stdout, category, message, retryable, recovery)
	return 1
}
