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
- Managed local Pi launch with durable target configuration, interactive prompt-file references, safe environment construction, and requested-credential handling.
- Git and process fact collection.
- Compact TOON output and structured recovery errors.
- Local credential requirements and non-secret warnings.
- Archive and cleanup-preservation state with independent recovery debt.

The current start operation creates a task-tagged tmux resource.
It starts a shell by default or a managed local Pi process when requested.
The default cleanup hooks do not delete worktrees or credentials.

## Phase 2: workflow integrations - default-enable readiness

Agent orchestration is ready for default enablement over the stable CLI boundary.
The agent skill owns automated lifecycle behavior and should inspect and reconcile before a manual fallback after a possibly mutating failure.

`AKAGENT_ENABLED` remains the immediate per-environment disable signal.
At the CLI boundary, only the exact value `1` enables automated integrations; an unset, empty, or other value disables them.
Direct CLI commands, including managed Pi launch, remain available when the signal disables automation.
Integrations must be idempotent, independently removable, token-budgeted, and limited to CLI operations.

## Tracked follow-ups

- Destructive worktree and credential cleanup hooks.
- Broader workflow integrations beyond the stable CLI boundary.
- Work-specific secrets and deployment behavior.

## Metrics that can change decisions

- Task startup time.
- Commands and tokens needed to discover and update state.
- Duplicate and orphaned resources.
- Reconciliation findings.
- Time to attach and recover after disconnect.
- Disk use by worktrees, caches, logs, and archives.
- Manual interventions per task.
- Safe retries for task launch and lifecycle operations.
- Credential warnings, expiration, rotation, and cleanup debt.
