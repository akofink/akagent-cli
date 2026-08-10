# Architecture

## Thesis

Standardize task identity, lifecycle operations, durable records, and recovery behavior.
Do not attempt to make unlike execution environments appear identical.

The first system is a local process orchestrator with a stable worker protocol.
Remote execution later transports the same worker commands to a named host.

## One executable, two surfaces

The operator surface is the normal interface:

```text
akagent
akagent task start
akagent task list
akagent task inspect
akagent task attach
akagent task state
akagent task finish
akagent task stop
akagent task archive
akagent task clean
akagent worker list
akagent worker inspect
```

The worker surface owns machine-local mutations and remains directly invocable:

```text
akagent worker task start
akagent worker task inspect
akagent worker task state
akagent worker task finish
akagent worker task stop
akagent worker task archive
akagent worker task clean
akagent worker reconcile
```

For a local worker, the operator surface may call worker functions in-process.
For debugging, tests, recovery, and remote execution, the same operation is exposed through the worker command group.

## Components

### Operator layer

The operator layer:

- Generates stable task IDs before dispatch.
- Resolves an explicitly selected worker.
- Validates requirements against worker and credential capabilities.
- Invokes worker operations locally or through a transport.
- Aggregates concise read-only status when requested.
- Resolves attachment and artifact retrieval.
- Does not edit a remote task record, worktree, or tmux server directly.

The first release has one implicit local worker and no scheduler.

### Worker layer

The worker layer is the sole mutation boundary for:

- Task records and event history.
- Repository registration and locks.
- Branch and worktree creation.
- Tmux windows and options.
- Agent process launch and termination.
- Credential installation and cleanup on that worker.
- State publication and reconciliation.
- Archival and cleanup.

Worker operations acquire task or repository locks and record recoverable progress after each side effect.

### Tmux

Tmux remains the human interaction and recovery surface, not the database.

Each interactive task has:

- A stable task ID in a window-scoped option.
- A concise mutable window name.
- An immediate `@agent_state` option for human navigation.
- A durable task record that remains authoritative across tmux renames and process exits.

Manual status updates and future hooks call the same CLI operation.

### Git repositories and worktrees

The worker host owns repositories and worktrees.
Repository policy is explicit because some repositories require worktrees while others require direct work on a designated branch.

Per-repository locking protects shared Git administrative state.
Startup detects branch collisions, stale worktree records, unexpected bases, and branches already checked out elsewhere.

### Durable records

The initial store is worker-local and file based:

```text
~/.local/state/akagent/
  worker.<encoding>
  repositories.<encoding>
  tasks/<task-id>/manifest.<encoding>
  tasks/<task-id>/events/<sequence>.<encoding>
  tasks/<task-id>/prompt.md
  tasks/<task-id>/result.md
  tasks/<task-id>/terminal.log
  locks/
```

Manifest replacement must be atomic.
The event history records intent and outcome for recovery and debugging.
The TOON evaluation must decide whether strict TOON is appropriate for durable mutable records or should remain an output encoding.

### Integrations

Shell helpers, LLM hooks, Pi plugins, and installable skills are optional adapters.
They invoke the CLI, request small field sets, preserve structured errors, and never write the task store directly.

Integrations are opt-in and idempotently installed.
Their failure cannot prevent direct CLI or tmux use.
Ambient session context is directory scoped and aggressively token limited.

## Remote extension

Remote support adds a transport around worker commands, not a second orchestration implementation.

The minimum transport contract is:

```text
execute(worker, argv, stdin) -> stdout, stderr, exit status
attach(worker, tmux target)
copy_to(worker, local source, remote destination)
copy_from(worker, remote source, local destination)
```

The operator sends an already generated task ID with every mutation.
A lost response can therefore be retried without launching a duplicate task.

Workers remain independently operable when the operator CLI or transport is unavailable.
Infrastructure provisioning, instance startup, and DNS remain separate concerns initially.

Private networking and outbound access are preferable to public worker addresses.
SSH over a private overlay and Systems Manager Session Manager should be evaluated by measured tmux latency, terminal fidelity, file transfer, port forwarding, recovery access, and credential lifetime.

## Discovery and local cache

After named remote execution works, the operator CLI should discover tasks across known workers and cache compact observations locally.
This provides a cross-platform overview without an always-available dashboard or central service.

Candidate commands are:

```text
akagent discover refresh
akagent discover list
akagent discover inspect <task-id>
```

Each cached observation records source worker and platform, task and agent identity, computed status, source observation time, local fetch time, expiration, and the most recent refresh error.

The cache is non-authoritative.
Mutations resolve the owning worker and revalidate live state before acting.
Unreachable workers retain visibly stale entries so network loss does not make agents disappear.
Conflicting observations remain attributed to their sources rather than being silently merged.

Refresh is bounded, cancellable, and tolerant of slow or unavailable workers.
Explicit refresh and optional shell-hook refresh are sufficient without a daemon.

## Provisioning boundary

This repository owns source, tests, schemas, and releases.
A dotfiles or machine-bootstrap repository may install `akagent`, configure the `aka` alias, and provision public prerequisites.
It must not become the owner of application state or protocol behavior.

Operator-specific private context is an optional provisioning layer.
It must be separable from public worker setup and from unrelated machine classifications such as personal versus work.

## Failure assumptions

Task execution must survive loss of the operator process and terminal attachment.
Task records must survive loss of tmux state when the worker filesystem survives.
Uncommitted work must survive agent process failure and ordinary stop operations.
Worker replacement preserves nothing unless Git state and declared artifacts have been copied to durable external storage.

Control-channel loss, stale PIDs, PID reuse, split-brain starts, partial setup, disk exhaustion, credential expiration, cleanup races, and false idle detection are expected conditions rather than exceptional surprises.

## Explicit non-goals for the first system

- Automatic worker placement.
- A central daemon or database.
- A distributed queue.
- Container execution.
- Kubernetes, ECS, or another scheduler.
- Transparent task migration.
- Cloud infrastructure provisioning.
- Multi-tenant security isolation.
- A web dashboard.
