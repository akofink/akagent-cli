# Roadmap

## Phase 0: protocol foundation - complete

The foundation includes:

- Go module and `akagent` executable.
- UUIDv7 task-ID generation.
- TOON output boundary and structured errors.
- Direct local worker inspection.
- Explicit source-managed self-update.
- Conforming TOON 4.1 output with fixtures and token measurements.
- Worker-local JSON state with typed envelopes, atomic replacement, locking, and recovery.
- Local credential manifest discovery and metadata-only readiness checks.
- Unit tests, vet, race coverage, and CI.

## Phase 1: local task lifecycle - implemented

The local lifecycle currently provides:

- One implicit local worker.
- Repository registration with `worktree` and `direct` policies.
- Durable task start, list, inspect, condition publication, finish, stop, archive, clean, and reconcile commands.
- Stable task IDs and task-tagged tmux windows.
- Verified tmux attachment using fresh process identity and heartbeat observations.
- Git branch and worktree ownership under the `worktree` policy.
- Durable manifests, append-only events, repository and task locks.
- Git and process fact collection.
- Compact TOON output and structured recovery errors.
- Local credential requirements and non-secret warnings.
- Archive and cleanup-preservation state with independent recovery debt.

The current start operation creates a detached tmux shell.
It does not launch a managed coding-agent executable.
The default cleanup hooks do not delete worktrees or credentials.

## Phase 2: workflow integrations - gated foundation

Automated integrations have a default-disabled gate.
They must require exactly `AKAGENT_ENABLED=1` and must continue without the integration otherwise.

Future integrations may include shell helpers, native lifecycle hooks, a plugin, an installable skill, directory-scoped session context, and session-end result capture.
Each integration must be opt-in, idempotent, independently removable, token-budgeted, and limited to CLI operations.

## Phase 3: named remote worker - future

The first remote proof should use one explicitly selected worker without scheduling.

Potential deliverables include:

- Static named-worker configuration.
- One remote command and attachment transport.
- Artifact retrieval.
- Capability and protocol negotiation.
- Credential selection, protected transfer, installation, refresh, and cleanup.
- Reconnection and lost-response tests.
- Worker-local operation during operator disconnection.

## Phase 4: discovery across workers and platforms - future

A later local cache may provide compact observations across named workers.
It should retain source attribution, stale and unreachable status, and live revalidation before mutation.

## Deferred

- Managed coding-agent launch.
- Agent containers and container schedulers.
- Automatic horizontal scaling.
- Transparent task migration.
- A central orchestration service.
- A web dashboard.
- Multi-tenant isolation.
- General-purpose plugins inside the core executable.
- Work-specific secrets and deployment behavior.

## Metrics that can change decisions

- Task startup time.
- Commands and tokens needed to discover and update state.
- Duplicate and orphaned resources.
- Reconciliation findings.
- Time to attach and recover after disconnect.
- Disk use by worktrees, caches, logs, and archives.
- Manual interventions per task.
- Remote transport failures and safe retries.
- Credential warnings, expiration, rotation, and cleanup debt.
