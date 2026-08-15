# Quick start

This guide uses only public commands and generic local paths.

`akagent` is a local-first CLI for durable task records, Git worktrees, and tmux-backed human or managed local Pi interaction.
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

## Check the integration gate

Automated integrations are disabled unless the invoking environment contains exactly `AKAGENT_ENABLED=1`.
A missing variable, an empty value, or any other value is disabled.

```bash
akagent integration inspect
```

The inspection command is read-only and does not enable anything.
Direct human `akagent` commands, including an explicit managed Pi task start, remain available regardless of the gate.

Enable the gate only for the current shell when an approved integration needs it:

```bash
export AKAGENT_ENABLED=1
```

Disable it immediately when the integration is no longer needed:

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
akagent repository update <name> [--path <path>] [--policy <worktree|direct>]
akagent repository unregister <name>
```

An update that would invalidate a task reference is rejected.
Unregister removes only the registration and is rejected while tasks still reference it.

## Start and inspect a task

Start a task with a title and registered repository:

```bash
akagent task start --title "Review the build" --repository demo
akagent task list
akagent task inspect <task-id>
```

The command generates a UUIDv7 task ID when `--task-id` is omitted.
It creates a durable manifest, a task branch, and an isolated Git worktree under the registered repository's worktree root when the policy is `worktree`.
The `--branch`, `--base`, and `--worktree` options provide explicit immutable Git inputs.
The command also creates a task-tagged tmux resource.

By default, the local CLI starts a shell for direct human or shell-driven work.
Use `--agent pi` to start a managed local Pi process instead.
Pi must be installed and available as `pi` on `PATH`.

A minimal managed-launch example uses only placeholder task data and a local prompt-file reference:

```bash
akagent task start --title "Review the build" --repository demo \
  --agent pi --prompt /path/to/prompt.txt --context "example"
```

The prompt file must be a regular local file.
Its contents are opened as Pi's standard input and are not copied into process arguments, tmux commands, task events, or TOON output.
The task manifest stores the selected target, resolved command path, prompt reference, worktree, and non-secret context before tmux starts.
The managed process receives a minimal safe environment plus `AKAGENT_TASK_ID` and the requested environment credentials that passed readiness checks.
Optional credentials produce non-secret warnings and are not injected.
File credentials can be checked for readiness but cannot be injected into the managed environment.

Task status is computed from lifecycle records and observations.
Lifecycle values are `starting`, `running`, `stopped`, and `finished`.
Computed statuses include `active`, `waiting`, `blocked`, `failed`, `stopped`, `finished`, and `unknown`.

Use TOON output as the protocol boundary rather than parsing human-oriented text:

```bash
akagent task inspect <task-id>
akagent task list
```

## Publish, attach, and reconcile

Publish a condition and heartbeat from a trusted local integration, managed workflow, or shell:

```bash
akagent task publish <task-id> --condition active --activity "running tests"
akagent task publish <task-id> --condition waiting --reason "needs review"
```

The accepted conditions are `active`, `waiting`, `blocked`, `failed`, and `none`.
Publication changes durable task state and refreshes its heartbeat.

Attach a direct shell or managed Pi task only after inspecting it:

```bash
akagent task attach <task-id>
```

Attachment requires a running task, a fresh heartbeat, a fresh process observation, exactly one matching task process, and a matching tmux task ID.
For managed launch, the launcher replaces itself with Pi so process identity checks refer to the Pi process.
It verifies the selected window immediately before calling `tmux attach-session`.
It refuses stale, missing, contradictory, stopped, and finished observations.
It never creates, kills, renames, or retargets tmux resources.

Reconcile after a disconnect, a terminal change, or an unexpected process exit:

```bash
akagent task reconcile
akagent task inspect <task-id>
```

Reconciliation repairs safe derived observations and Git facts.
It does not delete task state, branches, worktrees, windows, or terminal history.

## Stop, finish, archive, and clean

Stop a running task when the work should remain available for recovery:

```bash
akagent task stop <task-id>
```

Stopping ends the tagged tmux window and preserves the durable task record and Git worktree.
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
akagent task clean <task-id> --allow-committed --allow-dirty --allow-untracked
```

Cleanup refuses to act while the verified task process is live and refuses to discard committed, dirty, or untracked work unless each category is explicitly authorized.
The default local cleanup hooks do not delete worktrees or credentials, but cleanup state and any recovery debt are recorded so a later implementation or retry can act safely.

## Recovery rules

Start with `task inspect` and `task reconcile` when observations are unclear.

If a managed launch fails, repeat the same `task start` command to retry the recoverable `starting` task.
Equivalent repeated starts are idempotent; changing immutable launch inputs returns a conflict.

Do not attach when the heartbeat or process observation is stale.

Stop the task before archiving or cleaning it.

Review `committed`, `dirty`, `untracked`, `archive_state`, `cleanup_state`, and `cleanup_debt` before authorizing cleanup.

Treat structured errors and their recovery fields as the supported recovery guidance.

Credential requirements use named IDs through `--require` and `--optional`.
Credential values are never printed, stored in task output, or placed in command arguments.
