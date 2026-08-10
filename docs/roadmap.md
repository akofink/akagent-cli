# Roadmap

## Phase 0: protocol foundation

Goal: retire high-risk assumptions without creating production orchestration structure.

Current foundation:

- Go module and `akagent` executable.
- UUIDv7 task-ID generation.
- TOON output boundary and structured errors.
- Direct local worker inspection.
- Explicit source-managed self-update.
- Unit tests, vet, and CI.

Remaining work:

- Pin and validate the TOON specification and library.
- Define manifest, event, repository-policy, and credential schemas.
- Decide the durable storage encoding.
- Add token measurements and failure tests.
- Evaluate a rate-limited background update check only after explicit updates prove reliable.

## Phase 1: local task lifecycle

Goal: replace error-prone manual orchestration while preserving ordinary tmux use.

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
