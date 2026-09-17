# Generic agent guidance

Use `akagent` directly to keep task state durable during ordinary coding work.
The CLI is the source of truth for task, resource, execution, status, checkpoint, recovery, disposition, and delivery facts.
Tmux, Git, worktrees, providers, credentials, and forge clients are external surfaces, not durable state.

## Start or adopt work

If `AKAGENT_TASK_ID` is supplied, adopt that task instead of creating a duplicate.
Inspect it before editing:

```bash
akagent task inspect "$AKAGENT_TASK_ID"
akagent task resource list "$AKAGENT_TASK_ID"
akagent task execution list "$AKAGENT_TASK_ID"
```

If `AKAGENT_EXECUTION_ID` is supplied, inspect that execution too.
If no task ID is available, record intent without requiring repository or process access:

```bash
akagent task create --title "Describe the work" --task-id work-description
akagent task resource create work-description --resource-id source \
  --repository example-repo --branch agent/work-description \
  --worktree /path/to/worktree
```

Use returned IDs for subsequent commands.
Do not recreate a task, resource, execution, branch, or worktree when an adopted record exists.
Use `task external` only when the caller needs owned provenance and revision-checked completion.

## During work

Publish durable activity when work starts, changes condition, waits, or becomes blocked:

```bash
akagent task publish <task-id> --condition active --activity "implementing change"
akagent task execution publish <task-id> <execution-id> \
  --condition waiting --reason "needs review"
```

Publication is record-only.
Keep activity and reasons concise and never put credentials, sensitive prompt content, or private logs in them.

Record non-secret provider session provenance and delivery metadata through the generic commands:

```bash
akagent task execution session add <task-id> <execution-id> \
  --tool example-tool --session-id <session-id> \
  --reference-path /path/to/session-record
akagent task resource update <task-id> <resource-id> \
  --metadata delivery=pull-request-opened \
  --external-url https://forge.example/pull/123
```

The core stores references but never parses provider files or operates a forge.
External tools own process, Git, worktree, credential, provider, and delivery side effects.

## Recover and finish

After a command may have changed durable state but reports an error, inspect before retrying:

```bash
akagent task inspect <task-id>
akagent task reconcile <task-id>
akagent task execution reconcile <task-id>
```

Reconciliation is offline and store-only.
It preserves uncertainty and never launches a replacement process, mutates Git or worktrees, deletes state, or infers completion.

Record an outcome only when the caller's completion contract is satisfied:

```bash
akagent task finish <task-id> succeeded "Describe the completed result"
akagent task archive <task-id>
```

Use `failed` for an unsuccessful result.
A missing process, checkout, provider session, or credential never proves success.

Removed launch, attach, stop, deployment, cleanup, credential, and `task record` commands return structured usage errors with exit code `2`.
For the full workflow, see [agent-integration.md](agent-integration.md) and [skills/akagent-lifecycle/SKILL.md](skills/akagent-lifecycle/SKILL.md).
