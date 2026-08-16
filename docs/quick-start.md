# Quick start

This guide uses only public commands and generic local paths.

`akagent` is a local-first CLI for durable task records, Git worktrees, and tmux-backed human or optional managed local Pi interaction.
It writes protocol data and errors as TOON on stdout.

## Install or update

Build the binary from a public source checkout:

```bash
git clone https://github.com/akofink/akagent-cli.git /path/to/akagent-cli
cd /path/to/akagent-cli
go build -o "$HOME/.local/bin/akagent" ./cmd/akagent
```

The user-local binary directory must be on `PATH`.

Update from a clean checkout on `main` with an explicit source path:

```bash
akagent update --source /path/to/akagent-cli
```

The updater fetches `origin`, fast-forwards the source checkout to `origin/main`, builds in a temporary detached worktree, and atomically replaces the installed binary.
It does not discard source changes.

## Control automated orchestration

Agent orchestration is enabled by default over the stable `akagent` CLI boundary.
Repository implementation work uses the managed `akagent` lifecycle when the integration gate reports enabled.
The agent skill owns automated lifecycle behavior and direct human `akagent` commands remain available.
After a command that may have mutated state fails, inspect the task and run reconciliation before attempting a manual fallback.

`AKAGENT_ENABLED` remains the immediate per-environment disable signal.
At the CLI boundary, automated integrations are enabled unless `AKAGENT_ENABLED` is set to the exact value `0`.

```bash
akagent integration inspect
```

The inspection command is read-only.
Disable automation immediately for the current shell with:

```bash
export AKAGENT_ENABLED=0
```

Re-enable automation when the integration is needed again:

```bash
unset AKAGENT_ENABLED
```

## Register a repository

Register the root of an existing Git checkout before starting a task:

```bash
akagent repository register demo /path/to/checkout
akagent repository list
akagent repository inspect demo
```

Registration defaults to the `worktree` policy.
Use `--policy direct` only when the task is intentionally allowed to use the registered checkout itself.

Repository registration records the path and policy but never deletes the checkout or its Git metadata.
The supported repository mutations are:

```text
akagent repository update <name> [--path <path>] [--policy <worktree|direct>] [--worktree-root <absolute-path>]
akagent repository unregister <name>
```

Use `--worktree-root /home/user/dev/worktrees/demo` with the `worktree` policy to keep task worktrees separate from primary clones.
The configured root must be absolute, and every managed worktree must remain beneath it.
Without this option, the root remains `<checkout-parent>/.akagent/worktrees/<name>`.

An update that would invalidate a task reference is rejected.
Unregister removes only the registration and is rejected while tasks still reference it.

## Create, launch, and inspect a task

Create a task with a title and registered repository:

```bash
akagent task create --title "Review the build" --repository demo \
  --branch akofink/review-build
akagent task launch <task-id> --target shell
akagent task list [--all] [--repository <name>] [--worktree <path>]
akagent task inspect <task-id>
```

The command generates a UUIDv7 task ID when `--task-id` is omitted.
It creates a durable manifest, a task branch, and an isolated Git worktree under the registered repository's configured worktree root when the policy is `worktree`.
Worktree-policy tasks require an explicit descriptive branch, conventionally `akofink/<issue-or-ticket>-<2-3-word-description>`.
When `--worktree` is omitted, its directory name is the branch label after the owner prefix beneath the registered root, so `akofink/80-worktree-labels` uses `<worktree-root>/80-worktree-labels`.
Direct-policy tasks deliberately use the registered checkout's current branch when `--branch` is omitted.
The `--branch`, `--base`, and `--worktree` options provide explicit immutable Git inputs.
The default task list shows actionable records only, while `--all` includes archived history.
Actionable records include non-archived tasks and archived tasks with incomplete cleanup or recovery debt.
Use `--repository` and `--worktree` to compose deterministic exact-match filters.
Task creation does not create a tmux resource or start a process.
The explicit shell launch command creates a generic execution in a task-tagged tmux window.
Its display name is derived from the selected resource or task branch without the owner prefix, or supplied with `--label <descriptive-label>` when no branch is available.
The task and execution IDs remain in window metadata for lifecycle verification.
Managed execution windows publish `@agent_state` by matching those metadata IDs.
Active execution clears the option, waiting and blocked publish their values, and completed execution publishes `done`.

Use `--target shell` for direct human or shell-driven work.
Use `task execution create` and `task execution launch` when the caller needs explicit generic execution identity.
A task can create multiple resources and coordinate them through one execution by selecting one resource as its working directory.
For example, keep the primary worktree under a configured root while attaching a direct checkout for related notes:

```bash
akagent repository register app /path/to/app --worktree-root /home/user/dev/worktrees/app
akagent repository register notes /path/to/notes --policy direct
akagent task create --title "Coordinate the release docs"
akagent task resource create <task-id> --repository app --resource-id app-resource \
  --branch akofink/release-docs
akagent task resource create <task-id> --repository notes --resource-id notes-resource
akagent task execution create <task-id> --execution-id coordinator \
  --target shell --command /bin/sh --resource app-resource
akagent task execution launch <task-id> coordinator
```

The execution can record provider-neutral session provenance and delivery metadata without importing provider state:

```bash
akagent task execution session add <task-id> coordinator \
  --tool example-tool --session-id <session-id> --reference-path /path/to/session-record
akagent task resource update <task-id> app-resource \
  --metadata delivery=published --external-url https://forge.example/pull/78
akagent task inspect <task-id>
akagent task execution inspect <task-id> coordinator
```

Task, resource, and execution archives preserve these references.

Automated local workflows can use the provider-neutral integration entry point:

```bash
akagent integration launch <task-id> --execution-id workflow-run \
  --command /path/to/workflow --arg --mode --arg check --resource app-resource \
  --label workflow-check
```

The caller supplies a stable execution ID so retries are idempotent.
The command and arguments are opaque non-secret local process inputs.
The integration records a generic `workflow` execution before starting tmux.
If `AKAGENT_ENABLED=0` is set, the command returns a skipped success without creating task, resource, execution, or tmux state.
After an enabled launch failure, inspect and reconcile the execution before retrying:

```bash
akagent task execution inspect <task-id> workflow-run
akagent task execution reconcile <task-id>
```

Use `--target pi` only to select the optional Pi integration.
Pi must be installed and available as `pi` on `PATH` when that integration is selected.

A minimal managed-launch example uses only placeholder task data and a local prompt-file reference:

```bash
akagent task create --title "Review the build" --repository demo \
  --branch akofink/review-build
akagent task launch <task-id> --target pi --prompt /path/to/prompt.txt --context "example"
```

The prompt file must be a regular local file.
Only the validated prompt-file reference is passed to Pi, and standard input remains attached to the tmux terminal.
This keeps Pi interactive while prompt contents stay out of process arguments, tmux commands, task events, and TOON output.
The optional integration creates a generic execution with the selected resource worktree and non-secret context before tmux starts.
The owned pane shows a non-secret startup line before Pi initializes.
Pi's interactive status and tool views remain visible while the managed task works.
A failed launch leaves the generic execution recoverable and does not change resource state.
The managed process receives a minimal safe environment plus the non-secret `AKAGENT_TASK_ID` and `AKAGENT_EXECUTION_ID` context, along with requested environment credentials that passed readiness checks.
An execution can use that context with `akagent task resource create`, `task resource list`, and `task resource update` without parent-orchestrator intervention.
Resource metadata and external URLs are provider-neutral delivery records.
Agents use provider tooling such as `gh` or Bitbucket tooling for pull request operations, then optionally record the resulting URL with `akagent`.
Optional credentials produce non-secret warnings and are not injected.
File credentials can be checked for readiness but cannot be injected into the managed environment.
The core CLI does not provide GitHub, Bitbucket, or Pi delivery commands.

Task status is computed from lifecycle records and observations.
Lifecycle values are `created`, `starting`, `running`, `stopped`, and `finished`.
Computed statuses include `active`, `waiting`, `blocked`, `failed`, `stopped`, `finished`, and `unknown`.

Use TOON output as the protocol boundary rather than parsing human-oriented text:

```bash
akagent task inspect <task-id>
akagent task list [--all] [--repository <name>] [--worktree <path>]
```

## Publish, attach, and reconcile

Publish a condition and heartbeat from a trusted local integration, managed workflow, or shell:

```bash
akagent task publish <task-id> --condition active --activity "running tests"
akagent task publish <task-id> --condition waiting --reason "needs review"
```

The accepted conditions are `active`, `waiting`, `blocked`, `failed`, and `none`.
Publication changes durable task state and refreshes its heartbeat.
For managed executions, active, failed, and none clear `@agent_state`.
Reconciliation clears stale waiting or blocked state when process observations are no longer fresh.

Attach a compatibility task execution or a selected generic execution only after inspecting it:

```bash
akagent task attach <task-id>
akagent task execution attach <task-id> <execution-id>
```

Attachment requires a running task or execution, a fresh heartbeat, a fresh process observation, exactly one matching process, and matching task and execution tmux metadata where applicable.
For the optional Pi integration, the integration worker replaces itself with Pi so process identity checks refer to the Pi process.
It verifies the selected window immediately before calling `tmux attach-session`.
It refuses stale, missing, contradictory, stopped, and finished observations.
It never creates, kills, renames, or retargets tmux resources.

Reconcile after a disconnect, a terminal change, or an unexpected process exit:

```bash
akagent task reconcile
akagent task execution reconcile <task-id>
akagent task inspect <task-id>
```

Reconciliation repairs safe derived observations and Git facts.
It does not delete task state, branches, worktrees, windows, or terminal history.

## Stop, finish, archive, and clean

Stop a running task when the work should remain available for recovery:

```bash
akagent task stop <task-id>
```

Stopping ends the tagged task or execution tmux window and preserves the durable task, execution, and Git resource records.
Execution stop is independent from resource state.
It does not mark the task as successfully finished.

Finish only after the task process has exited:

```bash
akagent task finish <task-id> succeeded "Build completed"
akagent task finish <task-id> failed "Tests still fail"
```

Archive a stopped or finished task:

```bash
akagent task archive <task-id>
```

Archive captures the manifest, event history, non-secret Git facts, and terminal history when it is still available.
Unavailable terminal history is reported as a warning.
Archive is idempotent and must complete before cleanup.

Clean only after reviewing the archived Git facts:

```bash
akagent task clean <task-id>
akagent task clean <task-id> --allow-committed --allow-dirty --allow-untracked --allow-worktree --allow-credentials
```

Cleanup refuses to act while the verified task process is live and refuses to discard committed, dirty, or untracked work unless each category is explicitly authorized.
For isolated worktree tasks, `--allow-worktree` is a separate approval that enables the ownership-checked worktree cleanup hook.
The hook removes only the task worktree, preserves the branch and archived Git facts, and never removes a direct registered checkout.
Without that approval, the worktree remains available for direct human recovery and cleanup state records the debt.
Credential cleanup requires the separate `--allow-credentials` approval and can be retried without touching the worktree:

```bash
akagent credential clean <task-id> --allow-credentials
```

## Recovery rules

Start with `task inspect` and `task reconcile` when observations are unclear or a possibly mutating command fails.
Use the agent skill for automated lifecycle behavior and manual fallback only after those checks.

If an optional integration launch fails, retry the generic execution or repeat the same explicit launch command after inspecting and reconciling its state.
Equivalent repeated creates and launches are idempotent; changing immutable task or launch inputs returns a conflict.

Do not attach when the heartbeat or process observation is stale.

Stop the task before archiving or cleaning it.

Review `committed`, `dirty`, `untracked`, `archive_state`, `cleanup_state`, `credential_cleanup_state`, and `cleanup_debt` before authorizing cleanup.

Treat structured errors and their recovery fields as the supported recovery guidance.

Credential requirements use named IDs through `--require` and `--optional`.
Credential values are never printed, stored in task output, or placed in command arguments.
