# Agent integration guide

This guide is a progressive disclosure path for adding `akagent` to a coding agent's ordinary workflow.
It uses only public commands, generic names, and provider-neutral metadata.

## Choose the smallest useful integration

Start with the [generic `AGENTS.md` template](AGENTS.md).
Copy or adapt its lifecycle rules into the repository instructions that your coding agent already reads.
The template covers adoption, self-bootstrap, durable status, recovery, delivery metadata, finish, and archive.

Add the [reusable lifecycle skill](skills/akagent-lifecycle/SKILL.md) when the agent supports skills or prompt modules.
It turns the same rules into a focused procedure that can be invoked for implementation, recovery, or delivery work.

Read the [quick start](quick-start.md) when you need installation, repository registration, or complete command examples.
Use the [task CLI contract](task-cli.md) when an integration needs exact syntax, output schemas, or exit codes.

## Lifecycle boundary

The coding agent owns its task lifecycle through direct `akagent` commands.
A parent process, launch adapter, daemon, provider session, or forge integration is not required.

A task records durable intent and owns zero or more resources and executions.
A resource records immutable repository, branch, base, and worktree facts plus mutable delivery metadata.
An execution records an optional tool-neutral process and session references.
Resource and execution lifecycle remain independently recoverable.

Tmux and provider sessions may improve visibility, but `akagent task inspect` is the durable work-state view.
The CLI remains useful when a terminal, provider session, or network connection is unavailable.

## Progressive workflow

### 1. Adopt or self-bootstrap

When a managed environment supplies `AKAGENT_TASK_ID`, inspect and adopt that task.
Do not create a duplicate task or resource.

```bash
akagent task inspect "$AKAGENT_TASK_ID"
akagent task resource list "$AKAGENT_TASK_ID"
akagent task execution list "$AKAGENT_TASK_ID"
```

When no task exists, register the checkout and create task intent.
The compatibility `--repository` form creates an initial resource and isolated worktree under the configured root:

```bash
akagent repository register example-repo /path/to/checkout \
  --worktree-root /path/to/worktrees/example-repo
akagent task create --title "Describe the work" \
  --repository example-repo --branch agent/work-description
```

A task can also start without a resource and add one explicitly with `task resource create`.
Use the IDs returned by the CLI rather than guessing identifiers.

### 2. Select resources and executions

Create a resource for each repository or worktree that the task needs.
Resource Git ownership inputs are immutable after creation.

```bash
akagent task resource create <task-id> \
  --repository example-repo --resource-id example-resource \
  --branch agent/work-description
```

Create an execution when an interactive or managed process is useful.
Execution creation records intent without starting a process.
Launch it separately so a failed launch can be inspected and retried safely:

```bash
akagent task execution create <task-id> \
  --execution-id example-execution --target shell --command /bin/sh \
  --resource example-resource
akagent task execution launch <task-id> example-execution
```

The shorter shell flow is also valid:

```bash
akagent task launch <task-id> --target shell --resource example-resource
```

A task may have multiple resources and executions.
One execution can coordinate multiple resources while using one selected resource as its working directory.

### 3. Publish status

Publish task and execution conditions at meaningful boundaries.
Use `active` while making progress, `waiting` when work is awaiting an input, `blocked` for an external dependency, and `failed` for an unrecoverable failure:

```bash
akagent task publish <task-id> \
  --condition active --activity "implementing change"
akagent task execution publish <task-id> <execution-id> \
  --condition active --activity "running checks"
akagent task publish <task-id> \
  --condition waiting --reason "needs review"
```

Publication updates durable activity and the heartbeat.
Use `akagent task inspect <task-id>` to review the combined task, resource, execution, Git, session, delivery, and recovery state.

### 4. Record session and delivery metadata

Record a stable, provider-neutral session reference when one becomes available:

```bash
akagent task execution session add <task-id> <execution-id> \
  --tool example-tool --session-id <session-id> \
  --reference-path /path/to/session-record
```

The optional path is a reference only.
`akagent` validates its shape but does not read or parse provider-owned session state.

Use the forge's normal tooling to create or update a pull request.
Then record the resulting HTTPS URL and non-secret delivery metadata on the resource:

```bash
akagent task resource update <task-id> <resource-id> \
  --metadata delivery=pull-request-opened \
  --external-url https://forge.example/pull/123
```

These records are provider-neutral references, not a forge adapter.
Never place credential values, tokens, private prompt content, or sensitive logs in metadata, activity, reasons, or URLs.

### 5. Reconcile before recovery retries

If a command may have mutated state and then fails, do not immediately create a replacement.
Inspect and reconcile the affected records first:

```bash
akagent task inspect <task-id>
akagent task reconcile <task-id>
akagent task execution reconcile <task-id>
akagent task inspect <task-id>
```

Reconciliation repairs safe derived observations and Git facts.
It does not delete task state, branches, worktrees, windows, or terminal history.
After a disconnect or unexpected process exit, reconcile before resuming or creating another execution.

### 6. Finish and archive

Stop live work before recording an outcome.
Finish the task with a concise result, then archive the durable records:

```bash
akagent task stop <task-id>
akagent task finish <task-id> succeeded "Describe the completed result"
akagent task archive <task-id>
```

Use `failed` when the work did not complete successfully.
Archive captures the manifest, events, Git facts, resource and execution snapshots, session references, delivery metadata, and available terminal history.
Review archive and cleanup facts before separately authorizing destructive cleanup.

## Integration rules

Keep task, resource, and execution state in `akagent` rather than a shared text board or terminal scrollback.
Use TOON on stdout as the protocol boundary and keep diagnostics on stderr when an integration adds diagnostics.
Treat structured errors as recovery guidance.
Prefer idempotent commands and preserve the direct shell and Git recovery path.

The `akagent integration inspect` signal is relevant to optional automated local workflow integrations.
It is not a prerequisite for direct task, resource, execution, status, reconciliation, or archive commands.
