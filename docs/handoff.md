# Current handoff

## Goal

Build a local-first task protocol that preserves ordinary Git worktree and tmux recovery paths.

## Implemented

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
- A default-disabled automated integration gate inspected by `akagent integration inspect`.

## Current task behavior

`task start` creates a durable record, creates or validates the task Git worktree, and starts either a detached shell or a managed local Pi process in a task-tagged tmux window.
Managed launch persists the resolved `pi` command, worktree, optional prompt-file reference, and optional non-secret working context before tmux starts.
The prompt-file reference is passed to Pi without changing standard input, so Pi remains interactive in the tmux pane.
The launcher replaces itself with Pi, and the durable process identity therefore identifies Pi.
A failed launch remains in recoverable `starting` state and can be retried with the same immutable inputs.

`task stop` ends the tagged tmux window and preserves the durable task record and Git worktree.
`task finish` records a result only after the task process has exited.
`task archive` captures durable records, Git facts, and terminal history when available.
`task clean` refuses live tasks and unapproved loss of committed, dirty, or untracked work.
The default local cleanup hooks do not remove worktrees or credentials, but cleanup state and recovery debt are durable and independently retryable.

`task reconcile` repairs safe derived observations and Git facts.
It never deletes task state, branches, worktrees, windows, or terminal history.

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

## Next public work

The next work should improve opt-in workflow integrations over the stable CLI boundary while preserving the local task and managed Pi contracts.
The default-disabled integration gate remains separate from direct human CLI commands.

The detailed public design and delivery map is in [`implementation-plan.md`](implementation-plan.md).
