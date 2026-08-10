# Roadmap

## Phase 0: protocol foundation

Goal: retire high-risk assumptions without creating production orchestration structure.

Completed foundation:

- Go module and `akagent` executable.
- UUIDv7 task-ID generation.
- TOON output boundary and structured errors.
- Direct local worker inspection.
- Explicit source-managed self-update.
- Conforming TOON 4.1 output with a constrained supported subset and official fixtures.
- Worker-local JSON state with typed envelopes, atomic replacement, locking, and recovery.
- Local credential manifest discovery and metadata-only readiness checks.
- Unit tests, vet, and CI.

Completion evidence:

- Child issues [#2](https://github.com/akofink/akagent-cli/issues/2), [#3](https://github.com/akofink/akagent-cli/issues/3), and [#4](https://github.com/akofink/akagent-cli/issues/4) are merged.
- The durable store encoding is JSON; TOON 4.1 is pinned for agent-facing output and interchange.
- Conformance, token measurement, failure, race, recovery, and redaction tests are present in `main`.

The next task is [#8](https://github.com/akofink/akagent-cli/issues/8), which refines the first local task-lifecycle implementation from these APIs.

## Phase 1: local task lifecycle

Goal: replace error-prone manual orchestration while preserving ordinary tmux use.

Entry issue: [#8 Implement local task lifecycle commands](https://github.com/akofink/akagent-cli/issues/8).
The issue is deliberately concrete about repository policy, durable task records, tmux observations, credential capabilities, idempotency, recovery, and protocol output.

Deliverables:

- One implicit local worker.
- Repository registration and policy.
- Start, list, inspect, attach, state, finish, stop, archive, clean, and reconcile.
- Stable task IDs and tmux metadata.
- Repository and task locks.
- Durable manifests and events.
- Git and process fact collection.
- Compact no-argument live view.
- Local credential manifest, doctor, requirements, and warnings.

Exit criteria:

- Existing tmux and worktree workflows remain directly recoverable.
- Manual tmux state updates work through `akagent`.
- Waiting, crashes, finish outcomes, operator stops, and process loss remain distinguishable.
- Partial startup and cleanup can be retried safely.
- Uncommitted work and credentials are not silently discarded or retained.

## Phase 2: workflow integrations

Goal: reduce manual reporting without making an LLM tool authoritative.

Candidates include shell helpers, native lifecycle hooks, a Pi plugin, an installable agent skill, directory-scoped session-start context, and session-end result capture.

All integrations are opt-in, idempotent, independently removable, token-budgeted, and limited to CLI operations.

## Phase 3: named remote EC2 worker

Goal: prove the local protocol off-machine without scheduling.

Deliverables:

- Static named-worker configuration.
- One remote command and attachment transport.
- Artifact retrieval.
- Capability and protocol negotiation.
- Credential selection, protected transfer, installation, refresh, and cleanup.
- Reconnection and lost-response tests.
- Worker-local operation during operator disconnection.

Evaluate SSH over a private overlay and Systems Manager Session Manager by measured behavior rather than assumed security or convenience.

Exit criteria:

- Local and remote tasks share lifecycle semantics and schemas.
- Direct `akagent worker ...` diagnosis works on the host.
- Retries cannot duplicate tasks.
- Host stop, restart, and process loss reconcile understandably.
- No broad copied human cloud credential is required.
- Secrets do not appear in argv, output, logs, or task records.

## Phase 4: discovery across workers and platforms

Goal: provide a local overview and learn whether explicit placement remains sufficient.

Deliverables:

- Concurrent bounded status queries across named workers.
- Local cache of compact worker, task, and agent observations.
- Explicit refresh with stale and unreachable reporting.
- Source attribution and live revalidation before mutation.
- Capability-aware validation.
- Idle, retention, credential-expiration, and cleanup-debt reporting.

The cache remains non-authoritative and useful during temporary network loss.
Only measured workload should justify automatic placement, a central index, or a queue.

## Deferred

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
- Worker contention under realistic load.

Structured events and periodic analysis are enough before building a metrics pipeline.
