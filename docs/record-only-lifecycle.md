# State-Only Records

`akagent task record` is the state-only boundary for work managed by a caller outside `akagent`.

These commands write durable task, resource, execution, observation, completion, and archive records without invoking Git, tmux, providers, credentials, transcript readers, or process inspection.

## Contract

Adopt an empty task identity when no resource or execution exists:

```text
akagent task record task <task-id> \
  --caller-id <stable-caller-id> \
  --operation-id <idempotency-key>
```

Adopt an external repository binding with caller-declared identity:

```text
akagent task record adopt <task-id> \
  --resource-id <resource-id> \
  --caller-id <stable-caller-id> \
  --operation-id <idempotency-key> \
  --repository <identity> \
  --branch <branch> \
  --base <revision> \
  --head <revision> \
  --worktree <absolute-reference> \
  [--expected-revision <revision>] \
  [--metadata <key=value>]
```

The worktree reference is recorded even when it is missing or offline.

The command never resolves repository names, checks revisions, reads the path, or changes the filesystem outside the worker-local state store.

Record an external execution attempt:

```text
akagent task record execution <task-id> \
  --execution-id <execution-id> \
  --resource-id <resource-id> \
  --caller-id <stable-caller-id> \
  --operation-id <idempotency-key> \
  [--predecessor <execution-id>] \
  [--tool <tool> --session-id <session-id>]
```

Session references are provider-neutral declarations.

The core does not open, parse, or validate provider-owned session files.

Record an observation with explicit provenance:

```text
akagent task record observe <task-id> <execution-id> \
  --caller-id <stable-caller-id> \
  --operation-id <idempotency-key> \
  --expected-revision <revision> \
  --source <source> \
  --observed-at <RFC3339> \
  --host-id <host-id> \
  --boot-id <boot-id> \
  [--process-state <state>] \
  [--result <result>] \
  [--detail <redacted-detail>]
```

Observations are historical evidence only.

A previous-boot, stale, missing, or caller-declared observation never proves present liveness, success, or local ownership.

## Completion and archive

Completion is an explicit caller declaration against a named contract.

It is never inferred from a missing process, a missing checkout, a stale observation, or a provider session state.

Complete a task with no resources or executions:

```text
akagent task record complete <task-id> \
  --caller-id <stable-caller-id> \
  --operation-id <idempotency-key> \
  --expected-revision <revision> \
  --contract <contract> \
  --result <result>
```

Complete one external execution by adding `--execution-id <execution-id>`.

Archive the completed task, resource, or execution with `task record archive`.

State-only archives contain durable manifests, caller observations, and append-only events.

They do not capture terminal output, inspect processes, run Git, or clean worktrees and credentials.

## Concurrency and provenance

The stable caller ID identifies the external owner declaration.

The operation ID is a separate per-operation idempotency key.

Every mutable state-only record carries a revision and a bounded receipt history.

Updates require the current expected revision, while a repeated operation ID with the same inputs returns its durable receipt.

An operation ID reused with different inputs is rejected.

Predecessor execution IDs must belong to the same task, and lineage cycles are rejected before persistence.

Legacy managed resources and executions remain distinct from external records.

Legacy archive, stop, reconcile, and cleanup paths reject external provenance rather than trusting caller-declared ownership.

This guard preserves legacy cleanup safety while allowing later removal of host-side orchestration code.
