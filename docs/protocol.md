# Protocol

## Scope

The protocol is the stable contract between direct commands, local tasks, and integrations.
It covers identity, records, lifecycle semantics, output, errors, compatibility, and reconciliation.

It does not standardize cloud provisioning or imply that all workers have identical capabilities.
Managed local Pi launch is part of the local task contract described here.

## Resources

### Worker

A worker is a named machine on which local worker inspection can operate.

```toon
worker:
  id: local
  protocol_version: 1
  architecture: arm64
  operating_system: linux
  features[2]: tmux,git-worktree
```

Capabilities include architecture, operating system, execution features, capacity, protocol version, and image or configuration version where relevant.
The current CLI exposes `akagent worker inspect` for the implicit local worker.

### Task

A task has an immutable ID, operator request, and mutable execution record.

```toon
task:
  id: 019fe8f2-ac67-7406-a6e6-2717b2cd31c6
  title: Inspect local reconciliation
  worker: local
  repository: example
```

Task IDs are generated before dispatch and are safe for filenames, tmux options, and command arguments.
UUIDv7 provides sortable random identity without worker-local coordination.

Titles, branches, tickets, tmux names, and prompts are attributes rather than identity.

### Repository

A repository registration resolves a stable name to a local Git checkout and policy.

The `worktree` policy creates an isolated task branch and worktree under the registration's worktree root.
The `direct` policy uses the registered checkout and requires its base revision to match.

### Credential capability

A credential capability is a named permission needed by a task.
The task record stores the capability ID and readiness information without storing the value.

### Managed launch

A managed launch selects the local `pi` executable and persists its resolved command, task worktree, optional prompt-file reference, and optional non-secret working context.
The prompt reference identifies a regular local file whose path is passed to Pi as a file reference.
The launcher keeps standard input attached to the tmux terminal so Pi remains interactive.
The prompt contents are not copied into process arguments, task events, or protocol output.

The managed process receives a minimal safe runtime environment, `AKAGENT_TASK_ID`, and only requested environment credentials that passed readiness checks.
Optional requirements are recorded as non-secret warnings and are not injected.
File credentials can satisfy readiness requirements but cannot be injected into the managed environment.

## State model

Lifecycle phase:

```text
starting | running | stopped | finished
```

Agent-declared condition:

```text
active | waiting | blocked | failed | none
```

Observed facts include tmux window existence, process identity and start time, heartbeat, Git status, HEAD, archive status, cleanup status, and credential readiness.

The compact `status` field is a computed view:

1. `starting` while recoverable startup is incomplete.
2. `failed` when the condition or finish outcome records failure.
3. `waiting` or `blocked` when a fresh process observation has that condition.
4. `active` when a fresh task process exists without a stronger condition.
5. `finished` when finish recorded an outcome and no process remains.
6. `stopped` when stop completed without a finish outcome.
7. `unknown` when required observations are unavailable, stale, or contradictory.

`committed`, `dirty`, and `untracked` are Git facts.
Archive and cleanup state are independent recovery facts.

## Current local commands

The current executable exposes:

```text
akagent
akagent credential <list|inspect|doctor>
akagent integration inspect
akagent id generate
akagent repository <register|list|inspect|update|unregister>
akagent task <start|list|inspect|attach|publish|finish|stop|archive|clean|reconcile>
akagent update [--source <path>]
akagent worker inspect
```

The local task start operation validates the repository and credential requirements, persists a manifest, creates the requested branch and Git worktree when needed, and starts either a detached shell or the selected managed Pi process in a task-tagged tmux window.
For managed launch, the configuration is persisted before tmux starts and the launcher replaces itself with Pi so the durable process identity refers to the managed process.
The launcher prints a non-secret startup line in the owned pane, and Pi's interactive status and tool views remain visible during execution.
A launch failure prints a safe recovery message in the pane, remains in recoverable `starting` state, and records recovery debt.

## Lifecycle operations

### Start

The operator surface generates a task ID when omitted.
A start is recoverable and records these steps:

1. Resolve and lock the repository.
2. Validate policy and requested base.
3. Create or validate the branch and worktree.
4. Check named required and optional credential capabilities.
5. Persist the task manifest and start event.
6. Create a detached task-tagged tmux shell or managed Pi launch.
7. Record the tmux window, pane, process identity, and successful start.

Repeated equivalent starts return the existing task with exit code `0`.
Conflicting immutable inputs return a structured conflict.

### Publish state

A trusted local integration or shell may publish `active`, `waiting`, `blocked`, `failed`, or `none` with a concise reason and activity.
Publication updates the durable task record and heartbeat.
It does not make a declaration authoritative over contradictory process or Git observations.

### Inspect and list

List views default to compact decision-relevant fields and include a definitive total.
Detail views include task identity, computed status, branch and worktree facts, conditions, results, and recovery fields when present.

```toon
tasks[1]{id,title,status,worker,condition}:
  019f...,Inspect reconciliation,active,local,none
total: 1
```

### Attach

Attachment first resolves the durable task record by task ID.
It proceeds only for a running shell or managed Pi task with a fresh heartbeat, a fresh process observation, exactly one matching process, and a matching `@akagent_task_id` tmux window.

The command rechecks the task option on the selected window immediately before running `tmux attach-session` against that verified window ID.

Missing, stale, contradictory, stopped, and finished observations are rejected with structured recovery guidance.
Attachment does not write durable state and never creates, kills, renames, or retargets tmux resources.

### Stop

Stop verifies the task through the tagged tmux resource and terminates its task window.
It preserves the durable task record and Git worktree, then records `stopped` without claiming a successful outcome.
Terminal history is best-effort: archive runs only after the task is stopped or finished, and stopping ends the tagged window, so the archive may report terminal history as unavailable.
Stopping an already stopped or finished task is a successful no-op.

### Finish

Finish accepts `succeeded` or `failed` and a concise result after the task process has exited.
It moves lifecycle to `finished` and refreshes Git facts.

The current command records the result string only.
It does not infer verification success, commit state, or worktree cleanliness from the result.

### Archive

Archive accepts stopped or finished tasks after confirming no task process is live.
It captures the manifest, event history, non-secret Git facts, and terminal history when available.
Unavailable terminal history is recorded as a warning.
Partial archive attempts remain retryable, and equivalent archives are idempotent.

### Clean

Clean archives first and refuses destructive action while a verified task process runs.
It preserves committed, dirty, and untracked Git facts unless the operator explicitly supplies `--allow-committed`, `--allow-dirty`, and `--allow-untracked` for the corresponding categories.

Worktree and credential cleanup state are recorded independently so partial cleanup can be retried.
The default local cleanup hooks do not remove worktrees or credentials.

### Reconcile

Reconciliation compares durable records with tmux, process, Git, and store observations.
It repairs safe derived metadata, records changes, and reports stale, missing, replaced, or contradictory observations.
It never deletes task state, branches, worktrees, windows, or terminal history.

## Output

Agent-consumed stdout uses conforming TOON by default.
Internal logic uses typed records and encodes TOON at the boundary.

The implementation pins an exact supported TOON specification version and a constrained output subset.
See [`toon.md`](toon.md) for the pinned version, supported forms, known deviations, encoder decision, and token measurements.
Protocol compatibility is defined by `akagent` schemas and semantics, not by TOON alone.

## Errors

Structured successes and errors go to stdout.
Diagnostics and progress do not mix with protocol output.

```toon
error:
  category: conflict
  message: Task inputs conflict with the existing task
  retryable: false
  recovery: akagent task inspect <task-id>
```

Categories include `usage`, `not_found`, `conflict`, `retryable`, `partial`, `preservation_required`, `capability`, and `internal`.
Exit code `0` means success or no-op.
Exit code `1` means the requested operation could not be completed.
Exit code `2` means the command or its arguments are invalid.

## Integration gate

Automated integrations must treat a missing `AKAGENT_ENABLED` signal or any value other than `1` as disabled.
`akagent integration inspect` reports the read-only state.
The gate controls automated invocation only; direct human commands, including an explicit managed Pi task start, remain available regardless of the gate.

## Concurrency and consistency

Task mutation uses a per-task lock.
Repository mutation uses a per-repository lock.
Manifest replacement uses atomic rename and synchronization.
Task ID and operation name form the default idempotency identity.
Reads identify observation time and tolerate concurrent change.

## Compatibility

Protocol responses include version metadata where required by the command contract.
Adding optional fields is compatible.
Removing fields, changing meanings, or changing lifecycle semantics requires a protocol version change.
Human-readable text is not a stable parsing interface.
