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

## Phase 2: agent self-service adoption - next

Make skill-guided use of the durable CLI the normal coding-agent workflow.
The agent skill should teach agents to create task intent, manage resources and executions, publish status, record session and pull-request metadata, reconcile observations, and archive recoverable history.

The adoption work should prioritize:

- Short, deterministic CLI sequences that fit ordinary implementation and review loops.
- Durable inspection and reconciliation after disconnects, failed mutations, and provider changes.
- Explicit branch, worktree, session, and delivery facts that another agent can recover without tmux scrollback.
- Direct CLI use that remains useful without a provider integration, launch adapter, daemon, or network connection.
- Idempotent operations with structured recovery guidance and bounded output.

Tmux remains an interactive visibility and recovery surface.
The `akagent` CLI remains the durable source of truth.

## Tracked follow-ups

- Skill-guided adoption in ordinary coding workflows.
- Broader documentation for provider-neutral session and delivery records.
- Broader local deployment integrations beyond direct executable commands.

A resident daemon, remote scheduler, and launch-adapter requirement are explicitly out of scope.

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
