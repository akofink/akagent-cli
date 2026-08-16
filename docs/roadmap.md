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
- Durable task create, launch, list, inspect, condition publication, finish, stop, archive, clean, and reconcile commands.
- Stable task IDs and task-tagged tmux windows.
- Verified tmux attachment using fresh process identity and heartbeat observations.
- Git branch and worktree ownership under the `worktree` policy.
- Durable manifests, append-only events, repository and task locks.
- Optional managed local Pi execution integration with interactive prompt-file references, safe environment construction, and requested-credential handling.
- Local deployment executions with work-scoped credential readiness and durable completion results.
- Git and process fact collection.
- Compact TOON output and structured recovery errors.
- Local credential requirements and non-secret warnings.
- Archive and cleanup-preservation state with independent recovery debt.

Task creation records durable intent and can create zero resources without a process side effect.
Resource creation independently provisions or validates Git state, while generic execution creation and launch manage task-tagged tmux processes.
The compatibility shell launch and optional managed local Pi target both use generic executions.
Approved worktree cleanup validates ownership and preserves archive facts and the task branch.
Credential cleanup is an independent approval-gated hook with durable retry state.

## Phase 2: workflow integrations - default-enabled integration

Agent orchestration is enabled by default over the stable CLI boundary.
The agent skill owns automated lifecycle behavior and should inspect and reconcile before a manual fallback after a possibly mutating failure.

`AKAGENT_ENABLED` remains the immediate per-environment disable signal.
At the CLI boundary, automated integrations are enabled unless `AKAGENT_ENABLED` is set to the exact value `0`.
Direct CLI commands, including explicit shell execution and optional Pi launch, remain available when the signal disables automation.
The provider-neutral `akagent integration launch` adapter is the first broader workflow integration.
It requires a stable execution ID, persists generic execution state, and delegates startup to the existing lifecycle.
Disabled automation returns a skipped success without lifecycle side effects.
Integrations must be idempotent, independently removable, token-budgeted, and limited to CLI operations.

## Tracked follow-ups

- Broader workflow integrations beyond the stable CLI boundary.
- Broader local deployment integrations beyond direct executable commands.

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
