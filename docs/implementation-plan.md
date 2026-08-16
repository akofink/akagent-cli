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

## Completed foundation

The protocol foundation issues established:

- TOON 4.1 output and conformance checks.
- Secure worker-local state with typed JSON envelopes, atomic writes, locks, and recovery.
- Local credential manifest discovery, metadata-only readiness, and structured inspection commands.

## Completed local lifecycle

The local lifecycle implementation established:

- Repository registration and `worktree` or `direct` policy, with optional absolute worktree roots and a derived-root default.
- Durable task manifests and append-only events.
- Branch and Git worktree creation.
- Detached task-tagged tmux shells.
- Verified attachment using process identity, heartbeat, and task-window metadata.
- Durable condition publication, finish, stop, and reconciliation.
- Archive capture and cleanup-preservation policy.
- Independent archive, worktree-cleanup, credential-cleanup, and recovery-debt state.
- Default-on automated integration gating with an immediate per-environment disable path.
- Generic execution primitives with optional provider integrations, including managed local Pi, layered on top of task and resource state.

Task and resource creation, cleanup ownership checks, and reconciliation use the registered worktree root.
The current CLI creates tasks and Git resources without execution side effects.
Explicit `task launch --target shell` starts a generic execution for direct human or shell-driven work.
The optional `task launch --target pi` shortcut delegates to the Pi execution integration.
The removed `task start` shortcut is rejected with migration guidance.
Approval-gated worktree and credential cleanup hooks are implemented for task and resource cleanup.
Credential cleanup is task-scoped, independently approval-gated, durable, and retryable without repeating Git cleanup.

## Current orchestration boundary

Agent orchestration is enabled by default over the stable CLI boundary.
The agent skill owns automated lifecycle behavior, while direct human CLI commands remain available.
After a command that may have mutated state fails, inspect the task and run reconciliation before attempting a manual fallback.

`AKAGENT_ENABLED` remains the immediate per-environment disable signal for automated integrations.
At the CLI boundary, automation is enabled unless `AKAGENT_ENABLED` is set to the exact value `0`.
The provider-neutral `integration launch` command is the first broader workflow adapter.
It requires a stable execution ID, persists a generic execution, and delegates process startup to the existing lifecycle.

## Current command surface

```text
akagent
akagent credential <list|inspect|doctor|clean>
akagent integration <inspect|launch>
akagent id generate
akagent repository <register|list|inspect|update|unregister>
akagent task <create|deploy|resource|execution|credential|launch|list|inspect|attach|publish|finish|stop|archive|clean|reconcile>
akagent task resource <create|list|inspect|update|archive|clean>
akagent task execution <create|launch|list|inspect|session|publish|attach|stop|archive|reconcile>
akagent update [--source <path>]
akagent worker inspect
```

## Tracked follow-ups

1. Keep direct local commands stable and protocol output compatible.
2. Extend local lifecycle coverage for provider-specific credential cleanup behavior beyond the stable hook boundary.
3. Add work-specific secrets and deployment behavior only when a concrete requirement exists.
