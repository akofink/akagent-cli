# Architecture

## Thesis

Standardize task identity, lifecycle operations, durable records, and recovery behavior.
Do not make unlike execution environments appear identical.

The current system is a local task coordinator with a stable CLI protocol.

## One executable and current surfaces

The current executable exposes:

```text
akagent
akagent credential <list|inspect|doctor|clean>
akagent integration <inspect|launch>
akagent id generate
akagent repository <register|list|inspect|update|unregister>
akagent task <create|resource|execution|credential|launch|list|inspect|attach|publish|finish|stop|archive|clean|reconcile>
akagent task resource <create|list|inspect|update|archive|clean>
akagent task execution <create|launch|list|inspect|session|publish|attach|stop|archive|reconcile>
akagent update [--source <path>]
akagent worker inspect
```

Task creation persists task intent and may own zero resources without creating tmux or starting a process.
The compatibility `--repository` form creates one initial resource.
The resource command creates additional immutable repository, branch, and worktree combinations.
A task may own zero or more optional tool-neutral executions.
Execution creation, launch, observation, attachment, stop, archive, and recovery are independent from resource state.
An execution may contain multiple non-secret session references identified by provider-neutral tool, session ID, and optional absolute local reference path.
The explicit shell launch operation is a direct-human workflow built on generic executions.
The optional Pi integration provides a separate target on the same execution interface.
The removed `task start` shortcut is rejected with migration guidance.

## Components

### CLI layer

The CLI:

- Generates stable task IDs when the caller does not provide one.
- Resolves a named local repository and policy.
- Validates named credential requirements.
- Invokes local lifecycle operations.
- Emits concise TOON data and structured errors.
- Provides read-only integration-gate inspection.

The first release has one implicit local worker.

### Lifecycle layer

The lifecycle layer owns:

- Task records and event history.
- Independently recoverable resource records, Git facts, provider-neutral delivery metadata, archives, cleanup state, and recovery debt.
- Repository registration and locks.
- Branch and Git worktree creation.
- Optional execution records with durable process identity, descriptive tmux labels, non-secret task context, observation, attachment, stop, archive, and recovery state.
- Integration-owned session references that the lifecycle validates and persists without parsing provider session files.
- Tmux window identity, observation, attachment, and stop.
- Durable condition publication and heartbeat refresh.
- Process and Git reconciliation.
- Archive capture and cleanup-preservation policy.

The lifecycle exposes approval-gated worktree and credential cleanup hooks.
The worktree hook validates ownership before removal, direct repositories are never removed, and credential cleanup never rewrites the credential manifest.
The lifecycle records independent cleanup states and recovery debt so refused or failed destructive behavior can be retried without losing evidence.

### Tmux

Tmux is the human interaction and recovery surface, not the database.

Each managed execution window has stable task and execution IDs in window-scoped options.
The execution record stores the window, pane, process ID, process start time, and shared `@agent_state` publication state.
Compatibility task windows retain task-level metadata while new execution operations verify both IDs.

Attachment requires a fresh observation and exact process identity before it runs `tmux attach-session`.
It never trusts a similarly named window and never creates or mutates a target as part of attachment.
Execution stop re-observes the verified tagged window after killing it, and reports a retryable error rather than recording stopped if the window remains live.

### Git repositories and worktrees

The worker host owns repositories and worktrees.
Repository policy is explicit because some repositories use isolated task worktrees while others intentionally use a direct checkout.
Worktree-policy registrations may configure an absolute worktree root independent of the primary clone location.
The existing derived root remains the default when no root is configured.

Per-repository locking protects shared Git administrative state.
Startup validates branch, base revision, worktree containment, and existing worktree identity.
Cleanup ownership checks and reconciliation use the registered root.

### Durable records

The worker-local store is file based:

```text
$XDG_STATE_HOME/akagent/
  repositories/<name>.json
  tasks/<task-id>/manifest.json
  tasks/<task-id>/events/<sequence>.json
  tasks/<task-id>/resources/<resource-id>/manifest.json
  tasks/<task-id>/resources/<resource-id>/events/<sequence>.json
  tasks/<task-id>/resources/<resource-id>/archive.json
  tasks/<task-id>/executions/<execution-id>/manifest.json
  tasks/<task-id>/executions/<execution-id>/events/<sequence>.json
  tasks/<task-id>/executions/<execution-id>/archive.json
  tasks/<task-id>/archive.json
  locks/
```

The store uses typed JSON envelopes, atomic manifest replacement, append-only events, descriptor-safe traversal, and per-task locks.
TOON remains the agent-facing output encoding.

### Integrations

Shell helpers, LLM hooks, plugins, and installable skills are optional adapters.
They must invoke the CLI, request small field sets, preserve structured errors, and never write the task store directly.

Automated integrations are enabled unless `AKAGENT_ENABLED=0` is present.
The provider-neutral `integration launch` adapter checks the gate before opening the state store, records a generic workflow execution, and delegates startup to the lifecycle.
A disabled adapter has no lifecycle side effects, while direct human commands, including explicit shell execution and optional Pi selection, remain available.
Forge and provider-specific behavior remains outside the core lifecycle.

## Failure assumptions

Task records should survive loss of the operator process and terminal attachment when the worker filesystem survives.
Uncommitted work should survive process failure and ordinary stop operations.
Worker replacement preserves nothing unless Git state and declared artifacts have been copied to durable external storage.

Terminal disconnects, stale PIDs, PID reuse, duplicate starts, partial setup, disk exhaustion, credential expiration, cleanup races, and false idle detection are expected conditions.

## Explicit non-goals for the current system

- Automatic worker placement.
- Multi-tenant security isolation.
- A web dashboard.
