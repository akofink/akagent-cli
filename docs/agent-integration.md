# Agent integration guide

This guide adds `akagent` to a coding agent's ordinary workflow.
It uses public commands, generic names, and provider-neutral metadata.

## Choose the smallest integration

Start with the [generic `AGENTS.md` template](AGENTS.md).
Add the [lifecycle skill](skills/akagent-lifecycle/SKILL.md) when the agent supports skills.
Read the [quick start](quick-start.md) for examples and the [task CLI contract](task-cli.md) for exact syntax and protocol behavior.

## Lifecycle boundary

The coding agent owns its durable lifecycle through direct `akagent` commands.
A parent process, launch adapter, daemon, provider, Git checkout, or forge integration is not required.

A task records intent and owns zero or more resources and executions.
A resource records caller-declared repository, branch, revision, and worktree facts plus delivery metadata.
An execution records an optional tool-neutral attempt and provider-neutral session references.
The core never starts or stops the attempt.

External tools own process, tmux, Git, worktree, provider, credential, and forge side effects.
They submit redaction-safe observations and references through the CLI.
The CLI remains useful when any external tool or network is unavailable.

## Progressive workflow

### 1. Adopt or bootstrap

When `AKAGENT_TASK_ID` is supplied, inspect and adopt it instead of creating a duplicate:

```bash
akagent task inspect "$AKAGENT_TASK_ID"
akagent task resource list "$AKAGENT_TASK_ID"
akagent task execution list "$AKAGENT_TASK_ID"
```

When no task exists, record intent and declared facts:

```bash
akagent task create --title "Describe the work" --task-id work-description
akagent task resource create work-description --resource-id source \
  --repository example-repo --branch agent/work-description \
  --worktree /path/to/worktree
```

The commands do not inspect or mutate the checkout.
Use returned IDs rather than guessing identifiers.

### 2. Record an execution

Create an execution when an external tool needs explicit identity:

```bash
akagent task execution create <task-id> \
  --execution-id example-execution --target external \
  --command /path/to/tool --resource source
```

Creation records intent without starting a process.
`--target external` here is managed metadata and does not create a caller-owned external execution.
Use `task external` when the caller needs owned provenance and revision-checked completion.
The external tool owns startup and reports observations separately.

### 3. Publish status

```bash
akagent task publish <task-id> --condition active --activity "implementing change"
akagent task execution publish <task-id> <execution-id> \
  --condition waiting --reason "needs review"
```

Publication changes durable records only.
Use `active`, `waiting`, `blocked`, `failed`, and `none` as appropriate.
Never place credentials, private prompts, or sensitive logs in activity, reasons, metadata, URLs, or references.

### 4. Record session and delivery metadata

```bash
akagent task execution session add <task-id> <execution-id> \
  --tool example-tool --session-id <session-id> \
  --reference-path /path/to/session-record
akagent task resource update <task-id> <resource-id> \
  --external-url https://forge.example/pull/123
```

The core stores references but never opens provider files or operates a forge.
Record the PR URL as a hint only; `task check` derives PR and check state, so do not record delivery state as metadata.
Use external forge tooling for delivery.

### 5. Recover safely

After a possibly mutating failure, inspect and reconcile before retrying:

```bash
akagent task inspect <task-id>
akagent task reconcile <task-id>
akagent task execution reconcile <task-id>
akagent task inspect <task-id>
```

Reconciliation is offline and store-only.
It preserves missing, stale, unavailable, and contradictory observations.
It never launches a replacement process, changes Git or worktrees, deletes state, or infers completion.

### 6. Finish and archive

```bash
akagent task finish <task-id> succeeded "Describe the completed result"
akagent task archive <task-id>
```

Use `failed` when work did not complete successfully.
A missing process, checkout, provider session, or credential never proves success.

## Integration rules

Keep lifecycle state in `akagent`, not in terminal scrollback or a shared text board.
Use TOON stdout as the protocol boundary.
Treat structured errors as recovery guidance.
Prefer idempotent commands and stable IDs.

Worker protocol version `2` reports declarative capabilities.
Storage schema version `1` and legacy records remain readable.

Launch, attach, stop, deployment, cleanup, credential, integration, provider orchestration, and transitional `task record` commands are removed.
They return structured usage errors with exit code `2` before store access.
