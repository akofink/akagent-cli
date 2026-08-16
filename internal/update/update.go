package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gofrs/flock"
)

type Result struct {
	Source        string `json:"source"`
	Installed     string `json:"installed"`
	SourceBefore  string `json:"source_before"`
	SourceAfter   string `json:"source_after"`
	SourceChanged bool   `json:"source_changed"`
	Reinstalled   bool   `json:"reinstalled"`
}

type Error struct {
	Category  string
	Message   string
	Retryable bool
	Recovery  string
}

type commandRunner func(dir string, env []string, name string, args ...string) ([]byte, error)

func Run(sourceDir, executable string) (Result, *Error) {
	return run(sourceDir, executable, execute)
}

func run(sourceDir, executable string, runner commandRunner) (Result, *Error) {
	if runtime.GOOS == "windows" {
		return Result{}, &Error{
			Category: "capability",
			Message:  "Source-managed self-update is not supported on Windows",
			Recovery: "Build the latest checkout and replace akagent after the current process exits",
		}
	}

	sourceDir, err := filepath.Abs(sourceDir)
	if err != nil {
		return Result{}, internalError("Resolve the source checkout", "Retry `akagent update --source <path>`")
	}
	if _, err := os.Stat(filepath.Join(sourceDir, ".git")); err != nil {
		return Result{}, &Error{
			Category: "not_found",
			Message:  fmt.Sprintf("Akagent source checkout not found at %s", sourceDir),
			Recovery: fmt.Sprintf("Clone `https://github.com/akofink/akagent-cli` to `%s` or pass `--source <path>`", sourceDir),
		}
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return Result{}, internalError("Resolve the installed binary", "Reinstall akagent through machine setup")
	}
	updateLock := flock.New(executable + ".update.lock")
	locked, err := updateLock.TryLock()
	if err != nil {
		return Result{}, internalError("Lock the installed binary", "Retry `akagent update`")
	}
	if !locked {
		return Result{}, &Error{
			Category:  "retryable",
			Message:   "Another akagent update is already running",
			Retryable: true,
			Recovery:  "Wait for the active update, then retry `akagent update`",
		}
	}
	defer updateLock.Unlock()

	status, commandErr := runner(sourceDir, nil, "git", "status", "--porcelain")
	if commandErr != nil {
		return Result{}, internalError("Inspect the source checkout", fmt.Sprintf("Run `git -C %q status`", sourceDir))
	}
	if len(strings.TrimSpace(string(status))) > 0 {
		return Result{}, &Error{
			Category: "conflict",
			Message:  "Akagent source checkout has uncommitted changes",
			Recovery: fmt.Sprintf("Clean or commit changes in `%s`, then retry `akagent update`", sourceDir),
		}
	}

	branch, commandErr := runner(sourceDir, nil, "git", "branch", "--show-current")
	if commandErr != nil {
		return Result{}, internalError("Inspect the source branch", fmt.Sprintf("Run `git -C %q branch --show-current`", sourceDir))
	}
	if strings.TrimSpace(string(branch)) != "main" {
		return Result{}, &Error{
			Category: "conflict",
			Message:  "Akagent source checkout is not on main",
			Recovery: fmt.Sprintf("Switch `%s` to main, then retry `akagent update`", sourceDir),
		}
	}

	before, commandErr := revision(sourceDir, runner)
	if commandErr != nil {
		return Result{}, internalError("Read the installed source revision", fmt.Sprintf("Run `git -C %q rev-parse HEAD`", sourceDir))
	}
	if _, commandErr := runner(sourceDir, nil, "git", "fetch", "origin"); commandErr != nil {
		return Result{}, &Error{
			Category:  "retryable",
			Message:   "Failed to fetch akagent updates",
			Retryable: true,
			Recovery:  "Check network and GitHub access, then retry `akagent update`",
		}
	}
	if _, commandErr := runner(sourceDir, nil, "git", "merge", "--ff-only", "origin/main"); commandErr != nil {
		return Result{}, &Error{
			Category: "conflict",
			Message:  "Local akagent main cannot fast-forward to origin/main",
			Recovery: fmt.Sprintf("Inspect `git -C %q status` and reconcile main without discarding work", sourceDir),
		}
	}
	after, commandErr := revision(sourceDir, runner)
	if commandErr != nil {
		return Result{}, internalError("Read the updated source revision", fmt.Sprintf("Run `git -C %q rev-parse HEAD`", sourceDir))
	}

	installDir := filepath.Dir(executable)
	temporary, err := os.CreateTemp(installDir, ".akagent-update-*")
	if err != nil {
		return Result{}, &Error{
			Category: "capability",
			Message:  fmt.Sprintf("Cannot write an update beside %s", executable),
			Recovery: "Reinstall akagent to a user-writable directory such as `~/.local/bin`",
		}
	}
	temporaryPath := temporary.Name()
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return Result{}, internalError("Prepare the updated binary", "Retry `akagent update`")
	}
	defer os.Remove(temporaryPath)

	worktreeParent, err := os.MkdirTemp("", "akagent-update-source-*")
	if err != nil {
		return Result{}, internalError("Create an isolated source directory", "Retry `akagent update`")
	}
	defer os.RemoveAll(worktreeParent)
	worktreeDir := filepath.Join(worktreeParent, "checkout")
	if _, commandErr := runner(sourceDir, nil, "git", "worktree", "add", "--detach", worktreeDir, after); commandErr != nil {
		return Result{}, internalError("Create an isolated source checkout", fmt.Sprintf("Run `git -C %q worktree prune`, then retry `akagent update`", sourceDir))
	}
	worktreeAdded := true
	removeWorktree := func() error {
		if !worktreeAdded {
			return nil
		}
		_, removeErr := runner(sourceDir, nil, "git", "worktree", "remove", "--force", worktreeDir)
		if removeErr == nil {
			worktreeAdded = false
		}
		return removeErr
	}
	defer removeWorktree()

	if _, commandErr := runner(worktreeDir, sanitizedGoEnvironment(), "go", "build", "-o", temporaryPath, "./cmd/akagent"); commandErr != nil {
		return Result{}, &Error{
			Category: "internal",
			Message:  "Failed to build the updated akagent binary",
			Recovery: fmt.Sprintf("Run `cd %q && go test ./... && go build ./cmd/akagent`", sourceDir),
		}
	}
	if err := removeWorktree(); err != nil {
		return Result{}, &Error{
			Category: "partial",
			Message:  "Built akagent but could not remove its temporary source worktree",
			Recovery: fmt.Sprintf("Run `git -C %q worktree prune`, then retry `akagent update`", sourceDir),
		}
	}
	if err := os.Chmod(temporaryPath, 0o755); err != nil {
		return Result{}, internalError("Make the updated binary executable", "Retry `akagent update`")
	}
	if err := os.Rename(temporaryPath, executable); err != nil {
		return Result{}, &Error{
			Category: "capability",
			Message:  fmt.Sprintf("Cannot replace the installed binary at %s", executable),
			Recovery: "Reinstall akagent to a user-writable directory such as `~/.local/bin`",
		}
	}

	return Result{
		Source:        sourceDir,
		Installed:     executable,
		SourceBefore:  before,
		SourceAfter:   after,
		SourceChanged: before != after,
		Reinstalled:   true,
	}, nil
}

func execute(dir string, env []string, name string, args ...string) ([]byte, error) {
	command := exec.Command(name, args...)
	command.Dir = dir
	if env != nil {
		command.Env = env
	}
	return command.CombinedOutput()
}

func sanitizedGoEnvironment() []string {
	environment := os.Environ()
	sanitized := make([]string, 0, len(environment)+2)
	for _, entry := range environment {
		key, _, _ := strings.Cut(entry, "=")
		switch key {
		case "GOROOT", "GOTOOLDIR", "GOTOOLCHAIN", "GOENV":
			continue
		default:
			sanitized = append(sanitized, entry)
		}
	}
	return append(sanitized, "GOENV=off", "GOTOOLCHAIN=local")
}

func revision(sourceDir string, runner commandRunner) (string, error) {
	revision, err := runner(sourceDir, nil, "git", "rev-parse", "HEAD")
	return strings.TrimSpace(string(revision)), err
}

func internalError(action, recovery string) *Error {
	return &Error{
		Category: "internal",
		Message:  fmt.Sprintf("Failed to %s", strings.ToLower(action)),
		Recovery: recovery,
	}
}
