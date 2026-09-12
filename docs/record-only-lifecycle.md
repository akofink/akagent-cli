# Record-only lifecycle

The normal task, repository, resource, and execution commands are the record-only boundary.
The transitional `akagent task record` command family is removed.

## Contract

Create durable task intent:

```text
akagent task create --task-id <task-id> --title <title> [--repository <name>] [--branch <branch>] [--base <revision>] [--worktree <path>]
```

Record a caller-declared resource:

```text
akagent task resource create <task-id> \
  --resource-id <resource-id> --repository <identity> \
  --branch <branch> --base <revision> --head <revision> \
  --worktree <absolute-reference> [--metadata <key=value>]
```

Record an external execution attempt:

```text
akagent task execution create <task-id> \
  --execution-id <execution-id> --target external \
  --command <opaque-command> --resource <resource-id>
```

These commands persist caller-declared facts without resolving repository names, checking revisions, reading paths, starting processes, invoking Git, inspecting tmux, or reading provider files.

## Observations and references

Publish record conditions and activity:

```text
akagent task publish <task-id> --condition <active|waiting|blocked|failed|none> [--reason <reason>] [--activity <activity>]
akagent task execution publish <task-id> <execution-id> --condition <active|waiting|blocked|failed|none> [--reason <reason>] [--activity <activity>]
```

Record provider-neutral session provenance:

```text
akagent task execution session add <task-id> <execution-id> \
  --tool <tool> --session-id <session-id> --reference-path <absolute-reference>
```

References are metadata only.
The core never opens, parses, or interprets provider-owned content.
Observations remain historical evidence and never prove present liveness, success, or ownership.

Record an external execution observation with its provenance and current revision:

```text
akagent task execution observe <task-id> <execution-id> \
  --caller-id <id> --operation-id <id> --expected-revision <revision> \
  --source <source> --observed-at <RFC3339> --host-id <id> --boot-id <id> \
  [--process-state <state>] [--result <result>] [--detail <text>]
```

Finish an external execution only with its owning caller, current revision, operation ID, and named completion contract:

```text
akagent task execution finish <task-id> <execution-id> \
  --caller-id <id> --operation-id <id> --expected-revision <revision> \
  --contract <name> --result <result>
```

Equivalent retries are successful no-ops.
Stale revisions, changed operation inputs, wrong callers, and terminal mutations return conflicts.

## Completion and archive

Completion is explicit and is checked against the caller's contract outside the core:

```text
akagent task finish <task-id> <succeeded|failed> <result>
akagent task archive <task-id>
```

A missing process, checkout, stale observation, provider session, or credential never proves completion.
Archives contain durable manifests, references, checkpoints, and append-only events without terminal capture or host inspection.

## Concurrency and migration

Task, resource, execution, checkpoint, disposition, and event updates use durable revisions, locks, and idempotency keys where defined by their command contract.
Revision-scoped audit identity prevents an earlier event from satisfying a later repair after an A to B to A transition.
Manifest replacement and audit append remain separate writes, so durability is bounded rather than crash-atomic across both files.

Legacy records remain readable and preserve IDs, historical process and Git facts, session references, credential metadata, and recovery debt.
Legacy unfinished and stopped work requires explicit store-only adoption or migration.
The core never creates duplicate tasks, executions, branches, or worktrees and never implicitly reactivates work.
