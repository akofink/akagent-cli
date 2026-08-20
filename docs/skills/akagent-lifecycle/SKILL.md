---
name: akagent-lifecycle
description: Maintains durable akagent task, resource, and execution state during ordinary coding-agent work.
---

# akagent lifecycle

Use this skill when starting, continuing, recovering, delivering, or finishing coding work.
Call the public `akagent` CLI directly.
Do not require a parent orchestrator, launch adapter, daemon, or provider-specific lifecycle adapter.

## Inputs

Use `AKAGENT_TASK_ID` when the environment supplies an adopted task.
Use `AKAGENT_EXECUTION_ID` when the environment supplies an adopted execution.
Treat these values as identifiers, not credentials.

If a task ID is available, inspect it before editing:

```bash
akagent task inspect "$AKAGENT_TASK_ID"
akagent task resource list "$AKAGENT_TASK_ID"
akagent task execution list "$AKAGENT_TASK_ID"
```

If an execution ID is available, inspect it as well:

```bash
akagent task execution inspect "$AKAGENT_TASK_ID" "$AKAGENT_EXECUTION_ID"
```

If no task ID is available, self-bootstrap from the current checkout:

```bash
akagent repository register example-repo /path/to/checkout \
  --worktree-root /path/to/worktrees/example-repo
akagent task create --title "Describe the work" \
  --repository example-repo --branch agent/work-description
```

Use the returned task, resource, and execution IDs for later commands.
Do not create a duplicate record when an adopted record already exists.

## Establish resources and executions

A task resource owns immutable repository, branch, base, and worktree facts.
Create one for each repository or worktree required by the task:

```bash
akagent task resource create <task-id> \
  --repository example-repo --resource-id example-resource \
  --branch agent/work-description
```

An execution is an optional tool-neutral process associated with the task.
Create it before launch when explicit execution identity or a selected resource is useful:

```bash
akagent task execution create <task-id> \
  --execution-id example-execution --target shell --command /bin/sh \
  --resource example-resource
akagent task execution launch <task-id> example-execution
```

Use `akagent task launch <task-id> --target shell` for the shorter direct shell flow.
Resource and execution lifecycle are independent.

## Publish progress

Publish task and execution state before editing and after meaningful milestones:

```bash
akagent task publish <task-id> \
  --condition active --activity "implementing change"
akagent task execution publish <task-id> <execution-id> \
  --condition active --activity "running checks"
```

Publish `waiting` or `blocked` with a concise reason before yielding for input or an external dependency:

```bash
akagent task publish <task-id> \
  --condition waiting --reason "needs review"
akagent task execution publish <task-id> <execution-id> \
  --condition blocked --reason "waiting for dependency"
```

Never put credentials, tokens, private prompt contents, or sensitive logs in activity, reasons, or metadata.

## Record references

Record provider-neutral session provenance when a stable session ID is available:

```bash
akagent task execution session add <task-id> <execution-id> \
  --tool example-tool --session-id <session-id> \
  --reference-path /path/to/session-record
```

The path is an optional absolute reference to integration-owned state.
`akagent` does not read or parse that state.

After creating or changing a pull request, record its HTTPS URL and non-secret delivery metadata:

```bash
akagent task resource update <task-id> <resource-id> \
  --metadata delivery=pull-request-opened \
  --external-url https://forge.example/pull/123
```

Use the forge's own tooling for pull-request operations.
The `akagent` record is a delivery reference, not a forge integration.

## Recover safely

After a possibly mutating failure, inspect and reconcile before retrying or creating anything new:

```bash
akagent task inspect <task-id>
akagent task reconcile <task-id>
akagent task execution reconcile <task-id>
akagent task inspect <task-id>
```

Reconcile after a disconnect, unexpected process exit, terminal change, or contradictory observation.
Reconciliation repairs safe derived facts but does not delete task state, branches, worktrees, windows, or terminal history.

## Finish and archive

Do not finish a task while its task or execution process is live.
Stop active work, record the result, and archive it:

```bash
akagent task stop <task-id>
akagent task finish <task-id> succeeded "Describe the completed result"
akagent task archive <task-id>
```

Use `failed` instead of `succeeded` for an unsuccessful result.
Inspect the archived Git and recovery facts before separately authorized cleanup.

## Durable boundary

Use `akagent task inspect` as the durable work-state view.
Treat tmux, terminal output, provider sessions, and pull-request pages as observations or references rather than the lifecycle database.
Use TOON stdout as the machine-readable protocol boundary.
Keep optional provider integrations replaceable and preserve ordinary shell and Git recovery paths.
