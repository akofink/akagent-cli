# Current handoff

## Goal

Build a local-first task protocol that preserves ordinary Git worktree and tmux recovery paths.

## Shipped behavior

- Compact `akagent` home view.
- `akagent id generate` using UUIDv7.
- `akagent worker inspect` with local capabilities.
- TOON 4.1 output through a narrow, conformance-tested output package.
- Structured usage errors and shell exit codes.
- Source-managed `akagent update` with clean-main validation, fast-forward-only Git updates, and atomic binary replacement.
- A secure worker-local state store for versioned manifests, append-only events, atomic replacement, locking, archives, and recovery.
- A local credential manifest with `file:` and `env:` readiness checks plus `credential list`, `inspect`, and `doctor`.
- Repository registration with `worktree` and `direct` policies.
- Durable local task start, list, inspect, publish, finish, stop, archive, clean, and reconcile commands.
- Git branch and worktree creation with explicit immutable branch, base, and worktree inputs.
- Task-tagged detached tmux resources, managed local Pi launch, and verified attachment using fresh process identity and heartbeat observations.
- A per-environment integration signal inspected by `akagent integration inspect`.

## Current workflow

`task start` creates a durable record, creates or validates the task Git worktree, and starts either a detached shell or a managed local Pi process in a task-tagged tmux window.
Managed launch persists the resolved `pi` command, worktree, optional prompt-file reference, and optional non-secret working context before tmux starts.
The prompt-file reference is passed to Pi without changing standard input, so Pi remains interactive and a failed launch remains retryable with the same immutable inputs.

`task stop` ends the tagged tmux window and preserves the durable task record and Git worktree.
`task finish` records a result only after the task process has exited.
`task archive` captures durable records, Git facts, and terminal history when available.
`task clean` refuses live tasks and unapproved loss of committed, dirty, or untracked work.
The default local cleanup hooks do not remove worktrees or credentials, but cleanup state and recovery debt are durable and independently retryable.

`task reconcile` repairs safe derived observations and Git facts.
It never deletes task state, branches, worktrees, windows, or terminal history.

Agent orchestration is enabled by default over this stable CLI boundary.
The agent skill owns automated lifecycle behavior and preserves direct human CLI use.
After a command that may have mutated state fails, inspect the task and run reconciliation before attempting a manual fallback.

## Integration signal

`AKAGENT_ENABLED` remains the immediate per-environment disable signal for automated integrations.
At the CLI boundary, automation is enabled unless `AKAGENT_ENABLED` is set to the exact value `0`.
`akagent integration inspect` is read-only and reports the current state.
Direct human commands, including managed Pi task starts, remain available regardless of the signal.

## Current public command surface

```text
akagent
akagent credential <list|inspect|doctor>
akagent integration inspect
akagent id generate
akagent repository <register|list|inspect|update|unregister>
akagent task <start|list|inspect|attach|publish|finish|stop|archive|clean|reconcile>
akagent update [--source <path>]
akagent worker inspect
```

## Tracked follow-ups

- Destructive worktree and credential cleanup hooks.
- Broader workflow integrations beyond the stable CLI boundary.
- Work-specific secrets and deployment behavior.

The detailed public delivery map is in [`implementation-plan.md`](implementation-plan.md).
