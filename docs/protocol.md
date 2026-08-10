# Protocol

## Scope

The protocol is the stable contract between operator commands, worker commands, agents, integrations, and future transports.
It covers command behavior, identity, records, lifecycle semantics, output, errors, compatibility, and reconciliation.

It does not standardize cloud provisioning or imply that all workers have identical capabilities.

## Resources

### Worker

A worker is a named machine on which `akagent worker` can operate.

```toon
worker:
  id: local
  protocol_version: 1
  architecture: arm64
  operating_system: linux
  features[2]: tmux,git-worktree
  max_tasks: 4
```

Capabilities include architecture, operating system, execution features, capacity, protocol version, and image or configuration version where relevant.

### Task

A task has an immutable ID, operator request, and mutable execution record.

```toon
task:
  id: 019fe8f2-ac67-7406-a6e6-2717b2cd31c6
  title: Implement local reconciliation
  worker: local
  repository: example
```

Task IDs are generated before dispatch and are safe for filenames, tmux options, and command arguments.
UUIDv7 provides sortable random identity without worker-local coordination.

Titles, branches, tickets, tmux names, and prompts are attributes rather than identity.

### Repository

A repository registration resolves a stable name to a clone and policy.

Policy covers worktree requirements, branch naming, direct-branch exceptions, default base, commit expectations, and repository instruction discovery.

### Credential capability

A credential capability is a named permission needed by a worker or task.
The task record stores the capability ID, source type, scope, fingerprint, expiration, and installation status without storing the value.

## State model

Lifecycle phase:

```text
starting | running | stopped | finished
```

Agent-declared condition:

```text
active | waiting | blocked | failed | none
```

Observed facts include tmux window existence, process identity and start time, heartbeat, exit status, Git status, HEAD, archive status, cleanup status, and credential expiration.

The compact `status` field is a computed view:

1. `unknown` when required observations are unavailable or contradictory.
2. `failed` when launch or finish recorded failure.
3. `waiting` or `blocked` when the process exists and a matching declaration has a fresh heartbeat.
4. `active` when the process exists and no stronger condition applies.
5. `finished` when finish recorded an outcome and no process remains.
6. `stopped` when stop completed without a finish outcome.
7. `starting` while recoverable startup is incomplete.

Verification is current activity rather than lifecycle state.
Committed and dirty are Git facts.
Abandonment is an operator disposition.

## Lifecycle operations

### Start

The operator surface generates a task ID when omitted.
Direct `akagent worker task start` requires `--task-id`, and `akagent id generate` exposes the same generation algorithm.

Startup recoverably records these steps:

1. Reserve the task ID.
2. Resolve and lock the repository.
3. Validate policy and requested base.
4. Create or validate branch and worktree.
5. Resolve and validate credential requirements.
6. Persist prompt and launch configuration.
7. Install task-scoped credentials when required.
8. Create the tmux window.
9. Launch the agent process.
10. Record process identity and success.

Repeated equivalent starts return the existing task with exit code `0`.
Conflicting immutable inputs return a structured conflict.

### Publish state

Agents and integrations may publish condition, concise reason, current activity, requested operator action, and heartbeat.
Repeating the current value is a successful no-op.

Publication updates the durable task record and mirrors immediate condition into tmux when available.

### Inspect and list

List views default to three or four decision-relevant fields:

```toon
tasks[2]{id,title,status,worker}:
  019f...,Fix bootstrap,active,local
  01a0...,Review cleanup,waiting,local
```

Lists report a definitive total and support `--fields`.
Detail views preview large fields and offer `--full` only when content is truncated.

### Attach

Attachment resolves a task ID to a verified tmux target.
It checks the window task-ID option instead of trusting the window name.

### Stop

Stop verifies process identity using more than a PID, requests graceful termination, waits for a bounded interval, and records the outcome.
It preserves worktree, task record, logs, credentials needed for later recovery, and tmux history according to policy.
Stopping an already stopped task is a successful no-op.

### Finish

Finish records `succeeded` or `failed`, a concise result, and artifact declarations.
It moves lifecycle to `finished` after process exit.

An agent declaration does not prove verification passed, changes were committed, or the worktree is clean.

### Archive

Archive captures manifest, events, prompt, result, terminal output, Git facts, artifacts, and tool and worker versions.
It reports artifacts that could not be captured and does not claim worker-replacement durability unless stored elsewhere.

### Clean

Clean refuses destructive action while the verified process runs.
It captures Git facts and refuses to discard uncommitted or untracked work without explicit authorization.
Credential cleanup is recorded independently so partial cleanup can be retried safely.

### Reconcile

Reconciliation compares declarations with tmux, process, Git, filesystem, credential, and artifact facts.
It repairs safe derived metadata and reports ambiguous conditions.

It detects missing windows, replaced processes, PID reuse, stale heartbeats, worktree mismatches, orphaned resources, partial operations, expired credentials, and dangerous disk pressure.
It never deletes resources merely because they appear stale.

## Output

Agent-consumed stdout uses conforming TOON by default.
Internal logic uses typed records and encodes TOON at the boundary.

The implementation pins an exact supported TOON specification version and validates output against a conforming implementation or official tests.
Protocol compatibility is defined by `akagent` schemas and semantics, not by TOON alone.

Token-efficiency rules:

- Keep default list schemas small.
- Use tabular arrays for uniform records.
- Preview rather than silently omit large text.
- Offer `--fields` and `--full` escape hatches.
- Make no-argument output compact live content rather than a manual.
- Include contextual help only when it avoids a likely discovery call.
- Give session hooks a stricter budget than interactive commands.

## Errors

Structured successes and errors go to stdout.
Opt-in diagnostics and progress go to stderr.

```toon
error:
  category: conflict
  message: Task inputs conflict with the existing task
  retryable: false
  recovery: akagent task inspect <task-id> --full
```

Categories begin with `usage`, `not_found`, `conflict`, `retryable`, `partial`, `preservation_required`, `capability`, and `internal`.

Exit codes are `0` for success and no-op, `1` when intent cannot be satisfied, and `2` for usage errors.
Unknown flags fail before side effects.

## Concurrency and consistency

- Task mutation uses a per-task lock.
- Repository mutation uses a per-repository lock.
- Manifest replacement uses atomic rename and appropriate synchronization.
- Task ID and operation name form the default idempotency identity.
- Remote retries do not assume a lost response means failure.
- Reads identify observation time and tolerate concurrent change.

No distributed lock is required while one worker owns its filesystem and repositories.

## Compatibility

Cross-worker responses include protocol version metadata.
Adding optional fields is compatible.
Removing fields, changing meanings, or changing lifecycle semantics requires a protocol version change.
Human-readable text is not a stable parsing interface.
