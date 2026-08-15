# Architecture

## Thesis

Standardize task identity, lifecycle operations, durable records, and recovery behavior.
Do not make unlike execution environments appear identical.

The current system is a local task coordinator with a stable CLI protocol.
Future remote execution can transport the same worker operations to a named host.

## One executable and current surfaces

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

The current task start operation creates a detached tmux shell and a Git worktree when repository policy requires one.
It does not launch a managed coding-agent executable.

## Components

### CLI layer

The CLI:

- Generates stable task IDs when the caller does not provide one.
- Resolves a named local repository and policy.
- Validates named credential requirements.
- Invokes local lifecycle operations.
- Emits concise TOON data and structured errors.
- Provides read-only integration-gate inspection.

The first release has one implicit local worker and no scheduler or remote transport.

### Lifecycle layer

The lifecycle layer owns:

- Task records and event history.
- Repository registration and locks.
- Branch and Git worktree creation.
- Tmux window identity, observation, attachment, and stop.
- Durable condition publication and heartbeat refresh.
- Process and Git reconciliation.
- Archive capture and cleanup-preservation policy.

The default local cleanup hooks do not remove worktrees or credentials.
The lifecycle records independent cleanup states and recovery debt so destructive behavior can be added or retried without losing evidence.

### Tmux

Tmux is the human interaction and recovery surface, not the database.

Each task window has a stable task ID in a window-scoped option.
The task record stores the window, pane, process ID, and process start time.

Attachment requires a fresh observation and exact process identity before it runs `tmux attach-session`.
It never trusts a similarly named window and never creates or mutates a target as part of attachment.

### Git repositories and worktrees

The worker host owns repositories and worktrees.
Repository policy is explicit because some repositories use isolated task worktrees while others intentionally use a direct checkout.

Per-repository locking protects shared Git administrative state.
Startup validates branch, base revision, worktree location, and existing worktree identity.

### Durable records

The worker-local store is file based:

```text
$XDG_STATE_HOME/akagent/
  repositories/<name>.json
  tasks/<task-id>/manifest.json
  tasks/<task-id>/events/<sequence>.json
  tasks/<task-id>/archive.json
  locks/
```

The store uses typed JSON envelopes, atomic manifest replacement, append-only events, descriptor-safe traversal, and per-task locks.
TOON remains the agent-facing output encoding.

### Integrations

Shell helpers, LLM hooks, plugins, and installable skills are optional adapters.
They must invoke the CLI, request small field sets, preserve structured errors, and never write the task store directly.

Automated integrations are disabled unless `AKAGENT_ENABLED=1` is present.
The gate is checked by the integration before automated behavior, while direct human commands remain available.

## Future remote extension

Remote support should add a transport around worker operations rather than a second orchestration implementation.

The minimum future transport contract is:

```text
execute(worker, argv, stdin) -> stdout, stderr, exit status
attach(worker, tmux target)
copy_to(worker, local source, remote destination)
copy_from(worker, remote source, local destination)
```

The operator sends an already generated task ID with every mutation.
A lost response can therefore be retried without launching a duplicate task.

Workers should remain independently operable when the operator CLI or transport is unavailable.
Infrastructure provisioning, instance startup, and DNS remain separate concerns.

## Discovery and local cache

After named remote execution works, the operator CLI may discover tasks across known workers and cache compact observations locally.
The cache will be non-authoritative and mutations will revalidate live state before acting.

Candidate future commands are:

```text
akagent discover refresh
akagent discover list
akagent discover inspect <task-id>
```

## Failure assumptions

Task records should survive loss of the operator process and terminal attachment when the worker filesystem survives.
Uncommitted work should survive process failure and ordinary stop operations.
Worker replacement preserves nothing unless Git state and declared artifacts have been copied to durable external storage.

Control-channel loss, stale PIDs, PID reuse, split-brain starts, partial setup, disk exhaustion, credential expiration, cleanup races, and false idle detection are expected conditions.

## Explicit non-goals for the current system

- Managed coding-agent launch.
- Automatic worker placement.
- A central daemon or database.
- A distributed queue.
- Container execution.
- Kubernetes, ECS, or another scheduler.
- Transparent task migration.
- Cloud infrastructure provisioning.
- Multi-tenant security isolation.
- A web dashboard.
