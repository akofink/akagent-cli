# Implementation plan

## Public delivery workflow

GitHub issues are the source of truth for implementation scope and completion.
Each implementation issue should receive a focused branch, reviewable changes, verification, and a pull request.

Commit messages and pull-request bodies should use `Fixes #<issue>` when the change is intended to close an issue.

The public workflow is:

1. Start from the current `origin/main`.
2. Implement only the issue's stated scope.
3. Run the repository's focused checks and full Go verification.
4. Reconcile documentation with the shipped command surface.
5. Open a pull request with one reviewable change.
6. Wait for required CI to pass, then merge; this repository does not require another approval step.

## Accepted direction

The accepted target is a local-first, record-only core.
Core owns task, resource, and execution identities, typed state transitions, structured observations, checkpoint references, append-only events, archive history, recovery debt, delivery metadata, and protocol output.
External tools and skills own process launch, stop, and attach; tmux control; Git and worktree mutation; credential injection and cleanup; deployment execution; and provider-native session recovery when those capabilities remain needed.
Optional local and provider adapters may provide convenience integrations, but they are not required dependencies or feature-parity deliverables.

The current local lifecycle remains shipped but transitional compatibility behavior.
Current command syntax remains documented as shipped until a focused implementation issue changes it.
The migration has a finite boundary: record-only adoption, independent in-flight and maintenance views, crash-safe checkpoints and same-machine reboot recovery, adapter migration, then orchestration removal.
See [`charter.md`](charter.md) for the keep, deprecate, and remove matrix and exit criteria.

## Completed foundation

The protocol foundation issues established:

- TOON 4.1 output and conformance checks.
- Secure worker-local state with typed JSON envelopes, atomic writes, locks, and recovery.
- Local credential manifest discovery, metadata-only readiness, and structured inspection commands.

## Shipped local lifecycle - transitional

The local lifecycle implementation established:

- Repository registration and `worktree` or `direct` policy, with optional absolute worktree roots and a derived-root default.
- Durable task and resource manifests with append-only events.
- Branch and Git worktree creation.
- Detached task-tagged tmux shells.
- Verified attachment using process identity, heartbeat, and task-window metadata.
- Durable condition publication, finish, stop, and reconciliation.
- Archive capture and cleanup-preservation policy.
- Independent archive, worktree-cleanup, credential-cleanup, and recovery-debt state.
- An optional automated integration compatibility signal.
- Generic execution primitives with optional provider integrations, including managed local Pi.

Task and resource creation, cleanup ownership checks, and reconciliation use the registered worktree root.
The current CLI creates tasks and Git resources without execution side effects.
Explicit `task launch --target shell` starts a generic execution for direct human or shell-driven work.
The optional `task launch --target pi` shortcut delegates to the Pi execution integration.
The removed `task start` shortcut is rejected with migration guidance.
Approval-gated worktree and credential cleanup hooks are implemented for task and resource cleanup.
Credential cleanup is task-scoped, independently approved, durable, and retryable without repeating Git cleanup.

## Staged work

### 1. Record-only adoption - next

Make direct agent use of task, resource, and execution records the normal workflow.
Ensure intent is recorded before side effects, and ensure inspection, publication, archive, and reconciliation work without optional providers, tmux, or network access.
Preserve unknown, stale, and contradictory observations as recovery facts.

### 2. Independent in-flight and maintenance views - planned

Provide durable views for unfinished work, maintenance debt, recovery debt, checkpoint references, last observations, and adapter availability.
Keep these views independent from live tmux windows, provider transcripts, and forge APIs.
The view command names and syntax will be defined by the issue that implements them.

### 3. Crash-safe checkpoints and reboot recovery - planned

Add typed checkpoint references with atomic publication and append-only history.
Recover accepted unfinished work after a same-machine reboot when the worker filesystem survives.
Support safe partial recovery for missing, stale, unavailable, and contradictory observations without claiming exact process restoration.
Verify offline inspection, interrupted writes, concurrent updates, and idempotent recovery paths.

Disk loss, cross-machine synchronization, and exact process restoration require capabilities outside the local store.
Provider session recovery remains best effort and depends on provider-owned references.

### 4. Side-effect migration and orchestration removal - planned

Move side effects outside core to direct tools and skills, with optional local and provider adapters for convenience.
During the migration window, existing launch, attach, stop, credential, deployment, Git, and worktree command families remain compatibility surfaces with deprecation diagnostics.
After the charter exit criteria pass, remove direct orchestration from core and return structured migration guidance for removed commands.
Capabilities such as deployment or credential injection may be intentionally retired instead of recreated in separate adapters.
Ownership verification for destructive actions remains with the external tool or skill performing the action.
Legacy manifests, archives, and events remain readable.

### 5. Skill rollout - planned

Update reusable agent guidance after record-only adoption and reboot recovery have proven their failure paths.
Teach agents to recover accepted unfinished work without duplicate tasks, resources, executions, provider assumptions, or adapter dependencies.

## Current command surface

This list records shipped syntax and is not a promise that orchestration remains in the target core.

```text
akagent
akagent credential <list|inspect|doctor|clean>
akagent integration <inspect|launch>
akagent id generate
akagent repository <register|list|inspect|update|unregister>
akagent task <create|deploy|resource|execution|credential|launch|list|inspect|attach|publish|finish|stop|archive|clean|reconcile>
akagent task resource <create|list|inspect|update|archive|clean>
akagent task execution <create|launch|list|inspect|session|evidence|publish|attach|stop|archive|reconcile>
akagent update [--source <path>]
akagent worker inspect
```

The existing syntax remains the source of truth until its owning implementation issue changes it.
New future commands are intentionally not specified by this document.

## Verification expectations

Each staged change must preserve `go test ./...`, `go test -race ./...`, `go vet ./...`, and `git diff --check`.
Documentation changes must verify relative links and must not claim behavior that is not shipped.
Recovery work must include offline, interruption, concurrency, stale-observation, and safe-partial-recovery coverage before orchestration removal proceeds.
