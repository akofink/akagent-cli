package app

import (
	"fmt"
	"io"
	"strings"
)

func writeHumanTaskList(writer io.Writer, view taskListView) int {
	var output strings.Builder
	fmt.Fprintf(&output, "Tasks (%d)\n", view.Total)
	output.WriteString("ID | TITLE | STATUS | WORKER | BRANCH | WORKTREE | CONDITION\n")
	for _, task := range view.Tasks {
		fmt.Fprintf(&output, "%s | %s | %s | %s | %s | %s | %s\n",
			humanText(task.ID),
			humanText(task.Title),
			humanText(task.Status),
			humanText(task.Worker),
			humanText(task.Branch),
			humanText(task.WorktreePath),
			humanText(task.Condition),
		)
	}
	fmt.Fprintf(&output, "Total: %d\n", view.Total)
	return writeHuman(writer, output.String())
}

func writeHumanTaskDetail(writer io.Writer, view taskDetailView) int {
	var output strings.Builder
	output.WriteString("Task\n")
	appendTaskFields(&output, "  ", view.Task)

	fmt.Fprintf(&output, "Resources (%d)\n", len(view.Resources))
	if len(view.Resources) == 0 {
		output.WriteString("  none\n")
	} else {
		for index, resource := range view.Resources {
			fmt.Fprintf(&output, "  Resource %d\n", index+1)
			appendResourceFields(&output, "    ", resource)
		}
	}

	fmt.Fprintf(&output, "Executions (%d)\n", len(view.Executions))
	if len(view.Executions) == 0 {
		output.WriteString("  none\n")
	} else {
		for index, execution := range view.Executions {
			fmt.Fprintf(&output, "  Execution %d\n", index+1)
			appendExecutionFields(&output, "    ", execution)
		}
	}
	return writeHuman(writer, output.String())
}

func writeHuman(writer io.Writer, text string) int {
	if _, err := io.WriteString(writer, text); err != nil {
		return 1
	}
	return 0
}

func appendTaskFields(output *strings.Builder, indent string, task taskView) {
	appendTextField(output, indent, "id", task.ID)
	appendTextField(output, indent, "title", task.Title)
	appendTextField(output, indent, "status", task.Status)
	appendTextField(output, indent, "worker", task.Worker)
	appendTextField(output, indent, "branch", task.Branch)
	appendTextField(output, indent, "base_revision", task.BaseRevision)
	appendTextField(output, indent, "worktree_path", task.WorktreePath)
	appendTextField(output, indent, "condition", task.Condition)
	appendTextField(output, indent, "reason", task.Reason)
	appendTextField(output, indent, "activity", task.Activity)
	appendTextField(output, indent, "result", task.Result)
	appendBoolField(output, indent, "committed", task.Committed)
	appendBoolField(output, indent, "dirty", task.Dirty)
	appendBoolField(output, indent, "untracked", task.Untracked)
	appendTextField(output, indent, "recovery_debt", task.RecoveryDebt)
	appendTextField(output, indent, "warnings", task.Warnings)
	appendTextField(output, indent, "archive_state", task.ArchiveState)
	appendTextField(output, indent, "cleanup_state", task.CleanupState)
	appendTextField(output, indent, "worktree_cleanup_state", task.WorktreeCleanupState)
	appendTextField(output, indent, "credential_cleanup_state", task.CredentialCleanupState)
	appendBoolField(output, indent, "cleanup_debt", task.CleanupDebt)
	appendTextField(output, indent, "agent", task.Agent)
	appendTextField(output, indent, "agent_command", task.AgentCommand)
	appendTextField(output, indent, "prompt_reference", task.PromptReference)
	appendTextField(output, indent, "working_context", task.WorkingContext)
	appendTextField(output, indent, "execution", task.Execution)
}

func appendResourceFields(output *strings.Builder, indent string, resource resourceListItem) {
	appendTextField(output, indent, "id", resource.ID)
	appendTextField(output, indent, "repository", resource.Repository)
	appendTextField(output, indent, "branch", resource.Branch)
	appendTextField(output, indent, "base_revision", resource.BaseRevision)
	appendTextField(output, indent, "worktree_path", resource.WorktreePath)
	appendTextField(output, indent, "head", resource.Head)
	appendBoolField(output, indent, "committed", resource.Committed)
	appendBoolField(output, indent, "dirty", resource.Dirty)
	appendBoolField(output, indent, "untracked", resource.Untracked)
	appendTextField(output, indent, "recovery_debt", resource.RecoveryDebt)
	appendTextField(output, indent, "archive_state", resource.ArchiveState)
	appendTextField(output, indent, "cleanup_state", resource.CleanupState)
	appendTextField(output, indent, "worktree_cleanup_state", resource.WorktreeCleanupState)
	appendTextField(output, indent, "credential_cleanup_state", resource.CredentialCleanupState)
	appendBoolField(output, indent, "cleanup_debt", resource.CleanupDebt)
	appendTextField(output, indent, "metadata", resource.Metadata)
	appendTextField(output, indent, "external_urls", resource.ExternalURLs)
}

func appendExecutionFields(output *strings.Builder, indent string, execution executionView) {
	appendTextField(output, indent, "id", execution.ID)
	appendTextField(output, indent, "task_id", execution.TaskID)
	appendTextField(output, indent, "label", execution.Label)
	appendTextField(output, indent, "target", execution.Target)
	appendTextField(output, indent, "command", execution.Command)
	appendTextField(output, indent, "requirements", execution.Requirements)
	appendTextField(output, indent, "resource_id", execution.ResourceID)
	appendTextField(output, indent, "working_directory", execution.WorkingDirectory)
	appendTextField(output, indent, "status", execution.Status)
	appendTextField(output, indent, "condition", execution.Condition)
	appendTextField(output, indent, "reason", execution.Reason)
	appendTextField(output, indent, "activity", execution.Activity)
	appendTextField(output, indent, "result", execution.Result)
	appendTextField(output, indent, "tmux_window", execution.TmuxWindow)
	if execution.ProcessPID == 0 {
		appendTextField(output, indent, "process_pid", "")
	} else {
		appendTextField(output, indent, "process_pid", fmt.Sprintf("%d", execution.ProcessPID))
	}
	appendTextField(output, indent, "observation", execution.Observation)
	appendTextField(output, indent, "recovery_debt", execution.RecoveryDebt)
	appendTextField(output, indent, "archive_state", execution.ArchiveState)
	appendTextField(output, indent, "session_references", execution.SessionReferences)
}

func appendTextField(output *strings.Builder, indent, name, value string) {
	fmt.Fprintf(output, "%s%s: %s\n", indent, name, humanText(value))
}

func appendBoolField(output *strings.Builder, indent, name string, value bool) {
	fmt.Fprintf(output, "%s%s: %t\n", indent, name, value)
}

func humanText(value string) string {
	if value == "" {
		return "-"
	}
	return strings.NewReplacer(
		`\`, `\\`,
		"\n", `\n`,
		"\r", `\r`,
		"\t", `\t`,
		"|", `\|`,
	).Replace(value)
}
