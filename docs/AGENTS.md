# Generic agent guidance

Use `akagent` directly to keep task state durable during ordinary coding work.
The CLI is the source of truth for task, resource, execution, status, recovery, and delivery facts.
Tmux and provider sessions are optional interaction surfaces, not durable state.

## Start or adopt work

If the environment provides `AKAGENT_TASK_ID`, adopt the existing task instead of creating a duplicate.
Inspect the task and its resources and executions before editing:

```bash
akagent task inspect "$AKAGENT_TASK_ID"
akagent task resource list "$AKAGENT_TASK_ID"
akagent task execution list "$AKAGENT_TASK_ID"
```

If the environment also provides `AKAGENT_EXECUTION_ID`, inspect that execution before using it.
If no task ID is available, bootstrap one for the current checkout:

```bash
akagent repository register example-repo /path/to/checkout \
  --worktree-root /path/to/worktrees/example-repo
akagent task create --title "Describe the work" \
  --repository example-repo --branch agent/work-description
```

Use the task ID and resource ID returned by the CLI for subsequent commands.
Task creation establishes durable intent and a resource; it does not require an interactive process.
Create an execution only when a managed process or session is useful:

```bash
akagent task execution create <task-id> \
  --execution-id <execution-id> --target shell --command /bin/sh \
  --resource <resource-id>
akagent task execution launch <task-id> <execution-id>
```

Do not recreate a task, resource, execution, branch, or worktree when an adopted record already exists.

## During work

Publish durable activity when work starts, changes condition, waits, or becomes blocked:

```bash
akagent task publish <task-id> --condition active --activity "implementing change"
akagent task execution publish <task-id> <execution-id> \
  --condition active --activity "running checks"
akagent task publish <task-id> --condition waiting --reason "needs review"
```

Use `active`, `waiting`, `blocked`, `failed`, and `none` as appropriate.
Keep activity and reasons concise and never put credentials or sensitive prompt content in them.

Record non-secret session provenance when a tool provides a stable session ID.
Record a reference path only when it is an absolute local path that the integration owns:

```bash
akagent task execution session add <task-id> <execution-id> \
  --tool example-tool --session-id <session-id> \
  --reference-path /path/to/session-record
```

After creating or changing a pull request or other delivery, record its HTTPS URL and non-secret metadata on the resource:

```bash
akagent task resource update <task-id> <resource-id> \
  --metadata delivery=pull-request-opened \
  --external-url https://forge.example/pull/123
```

The CLI stores references but does not parse provider session files or operate the forge.
Use the provider's normal tooling for delivery and keep its credentials out of commands, output, logs, and task records.

## Recover and finish

After a command may have changed durable state but reports an error, inspect before retrying:

```bash
akagent task inspect <task-id>
akagent task reconcile <task-id>
akagent task execution reconcile <task-id>
```

Reconciliation repairs safe derived observations without deleting task state, Git branches, worktrees, windows, or terminal history.
Do not create replacement lifecycle records until inspection and reconciliation establish that the original operation did not succeed.

Stop live work before recording an outcome or archiving it:

```bash
akagent task stop <task-id>
akagent task finish <task-id> succeeded "Describe the completed result"
akagent task archive <task-id>
```

Use `failed` instead of `succeeded` when the result is unsuccessful.
Archive only after the task process has exited and review the archived Git and recovery facts before any separately authorized cleanup.

For the full workflow and a reusable skill, see the [agent integration guide](agent-integration.md) and [lifecycle skill](skills/akagent-lifecycle/SKILL.md).
