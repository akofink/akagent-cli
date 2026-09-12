# Roadmap

This roadmap separates shipped compatibility behavior from the accepted durable registry direction.
The order prioritizes unfinished work that survives an agent, terminal, provider, or network failure.

## Phase 0: protocol foundation - complete

The foundation includes:

- Go module and `akagent` executable.
- UUIDv7 task-ID generation.
- TOON output boundary and structured errors.
- Direct local worker inspection.
- One implicit local worker.
- Source-managed self-update.
- Conforming TOON 4.1 output with fixtures and token measurements.
- Worker-local JSON state with typed envelopes, atomic replacement, locking, and recovery.
- Local credential manifest discovery and metadata-only readiness checks.
- Unit tests, vet, race coverage, and CI.

## Phase 1: local lifecycle - shipped, transitional

The current local lifecycle provides:

- Repository registration with `worktree` and `direct` policies.
- Durable task, resource, and execution records with list, inspect, publication, finish, stop, archive, clean, and reconcile behavior.
- Git branch and worktree creation under the worktree policy.
- Task- and execution-tagged tmux windows with verified attachment and process observations.
- Optional local Pi execution and direct deployment execution.
- Provider-neutral session references, metadata-only evidence, and delivery references.
- Archive, cleanup-preservation, and recovery-debt state.

These commands and side effects are shipped compatibility behavior.
They are not the target ownership model and remain documented at their current syntax until implementation work changes them.

## Phase 2: record-only adoption - next

Make the durable CLI the normal path for agents and independent tools without requiring a parent orchestrator.

Milestones:

- Create task, resource, and execution intent before any external side effect.
- Publish typed conditions, observations, activity, recovery debt, session references, and delivery metadata through the stable CLI.
- Keep record inspection, archive, and reconciliation available when tmux, providers, credentials, deployment tools, Git mutation, or network access are unavailable.
- Preserve unknown, stale, and contradictory observations instead of inferring success or triggering destructive cleanup.
- Provide bounded, deterministic output suitable for offline agent decisions.

## Phase 3: independent work views - planned

Expose durable views that separate unfinished work from maintenance and recovery work.

Milestones:

- Show active and unfinished task, resource, and execution records without requiring a live process.
- Show recovery debt, cleanup debt, checkpoint references, last observations, and adapter availability as separate decision inputs.
- Make views useful after terminal disconnect and before provider or tmux recovery is attempted.
- Test one execution coordinating multiple resources without duplicating resource state.

The view names and command syntax are not specified here until an implementation issue owns them.

## Phase 4: crash-safe checkpoints and reboot recovery - planned

Make same-machine reboot recovery a first-class durable workflow before removing orchestration.

Milestones:

- Store typed checkpoint references and verification observations atomically with append-only history.
- Make concurrent checkpoint publication idempotent and safe across interruption or partial writes.
- Recover accepted unfinished work from the surviving worker filesystem after reboot without requiring tmux or a provider.
- Distinguish safe partial recovery from exact process restoration.
- Preserve recovery debt for missing, stale, unavailable, and contradictory process or provider observations.
- Verify offline inspection and a safe partial recovery path in failure-oriented tests.

The worker-local store does not protect against disk loss.
Cross-machine synchronization requires an external backup or synchronization system.
Provider session recovery remains best effort, and a replacement process requires fresh adapter verification.

## Phase 5: orchestration exit - planned

Move side effects outside the core and remove direct orchestration after the finite gates in [`charter.md`](charter.md) pass.
Direct tools and skills are valid replacements for capabilities that remain needed.
Optional local or provider adapters are convenience integrations, not required feature-parity deliverables.

Milestones:

- External tools and skills own Git/worktree mutation, process launch and stop, tmux attachment, credential injection and cleanup, and deployment execution when those capabilities remain needed.
- Optional provider adapters may provide provider policy and session discovery.
- Core commands accept and preserve typed observations without hidden side effects or a dependency on any adapter.
- Existing orchestration command families emit deprecation diagnostics during the migration window.
- Removed commands return structured migration guidance to the record and the direct tool, skill, or optional adapter workflow when one exists.
- Deployment and credential capabilities may be intentionally retired instead of recreated in separate adapters.
- Existing manifests, archives, and events remain readable after removal.

Ownership verification for destructive actions remains with the external tool or skill performing the action.
No new command syntax is promised by this roadmap.

## Phase 6: skill rollout - planned

Update reusable agent guidance after the record-only and reboot-recovery paths are proven.

Milestones:

- Teach agents to adopt or create durable task state without duplicate resources or executions.
- Teach inspect, publish, checkpoint, reconcile, and archive behavior after disconnects and failed mutations.
- Keep direct human shell, Git, and tmux recovery available without requiring an adapter.
- Measure recovery time, duplicate or orphaned records, manual interventions, and safe retries.

## Metrics that can change decisions

- Task startup time and the commands and tokens needed to discover and update state.
- Duplicate and orphaned resources.
- Reconciliation findings and time to attach or recover after disconnect.
- Disk use by worktrees, caches, logs, and archives.
- Manual interventions per task.
- Safe retries for task launch and lifecycle operations.
- Credential warnings, expiration, rotation, and cleanup debt.

## Explicit non-goals

- A resident daemon or remote scheduler.
- Cross-machine synchronization in the local store.
- Exact restoration of a process after reboot.
- Provider transcript indexing or private context persistence.
- Deployment expansion before recovery hardening demonstrates a concrete need.
