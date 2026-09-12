---
name: akagent-lifecycle
description: Maintains durable akagent task, resource, and execution records during coding-agent work.
---

# akagent lifecycle

Use this skill when starting, continuing, recovering, delivering, or finishing coding work.
Call the public `akagent` CLI directly.
The CLI is record-only and does not require a parent orchestrator, launch adapter, daemon, provider, Git, or tmux.

## Adopt or create records

Use `AKAGENT_TASK_ID` and `AKAGENT_EXECUTION_ID` when supplied.
Treat them as identifiers, not credentials.
Inspect an adopted task before editing:

```bash
akagent task inspect "$AKAGENT_TASK_ID"
akagent task resource list "$AKAGENT_TASK_ID"
akagent task execution list "$AKAGENT_TASK_ID"
```

If no task ID is available, record intent from the current checkout without asking `akagent` to inspect it:

```bash
akagent task create --title "Describe the work" --task-id work-description
akagent task resource create work-description \
  --resource-id source --repository example-repo \
  --branch agent/work-description --worktree /path/to/worktree
```

Use the returned IDs for later commands.
Do not create a duplicate record when an adopted record already exists.
Repository registration is optional metadata and does not validate or mutate a checkout.

## Establish executions

An execution is an optional tool-neutral attempt.
Create it without launching a process:

```bash
akagent task execution create <task-id> \
  --execution-id example-execution --target external \
  --command /path/to/tool --resource source
```

External tools own process launch, attachment, stop, and cleanup.
Do not use removed `launch`, `attach`, `stop`, `deploy`, or `clean` commands.

## Publish progress

Publish task and execution state after meaningful milestones:

```bash
akagent task publish <task-id> --condition active --activity "implementing change"
akagent task execution publish <task-id> <execution-id> \
  --condition waiting --reason "needs review"
```

Publication changes durable records only.
Never put credentials, tokens, private prompt contents, or sensitive logs in activity, reasons, metadata, URLs, or references.

## Record references

Record provider-neutral session provenance when a stable session ID is available:

```bash
akagent task execution session add <task-id> <execution-id> \
  --tool example-tool --session-id <session-id> \
  --reference-path /path/to/session-record
```

The core stores the reference but never opens or parses provider-owned state.
Record delivery URLs and metadata after using external forge tooling:

```bash
akagent task resource update <task-id> <resource-id> \
  --metadata delivery=pull-request-opened \
  --external-url https://forge.example/pull/123
```

## Recover safely

After a possibly mutating failure, inspect and reconcile before retrying or creating anything new:

```bash
akagent task inspect <task-id>
akagent task reconcile <task-id>
akagent task execution reconcile <task-id>
akagent task inspect <task-id>
```

Reconciliation is store-only.
It preserves missing, stale, contradictory, and unavailable observations.
It never launches a replacement process, touches Git or worktrees, deletes state, or infers completion.

## Finish and archive

Record explicit completion only when the caller's completion contract is satisfied:

```bash
akagent task finish <task-id> succeeded "Describe the completed result"
akagent task archive <task-id>
```

Use `failed` for an unsuccessful result.
A missing process, terminal, provider session, checkout, or credential never proves success.

## Durable boundary

Use `akagent task inspect` as the durable work-state view.
Use `task list --view in-flight`, `attention`, `maintenance`, `deferred`, or `history` for inventory.
Use checkpoints for bounded handoffs with expected revisions and idempotency keys.
Legacy unfinished work requires explicit store-only adoption and must not create duplicate tasks or worktrees.
TOON stdout is the machine-readable protocol boundary.
Worker protocol version `2` reports declarative capabilities, while storage schema version `1` remains readable.
