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
- Repository registration with `worktree` and `direct` policies, optional absolute worktree roots, and the derived-root default.
- Durable local task creation, explicit execution launch, list, inspect, publish, finish, stop, archive, clean, and reconcile commands.
- Zero or more independently recoverable task resources with separate Git facts, archives, cleanup state, and recovery debt.
- Zero or more optional tool-neutral executions with independent identity, tmux metadata, lifecycle observation, archive, stop, and recovery state.
- Git branch and worktree creation with explicit immutable branch, base, and worktree inputs.
- Task and execution-tagged detached tmux resources, optional Pi execution integration, and verified attachment using fresh process identity and heartbeat observations.
- A per-environment integration signal inspected by `akagent integration inspect`.

## Current workflow

`task create` creates durable task intent and can create zero resources without creating a tmux window or starting a process.
The compatibility `--repository` form creates one initial resource.
`task resource create` adds additional repository, branch, and worktree combinations.
Worktree-policy resources require an explicit descriptive branch, conventionally `akofink/<issue-or-ticket>-<2-3-word-description>`.
Direct-policy tasks deliberately use the registered checkout's current branch when no branch is provided.
`task execution create` records an optional tool-neutral execution without a process side effect.
`task execution launch` starts the selected execution, and a multi-resource task may attach one resource with `--resource` during creation.
Execution stop, archive, attach, and reconcile operate independently from resource state.
The compatibility `task launch --target shell` path creates and launches a generic shell execution.
The optional `task launch --target pi` path delegates to the Pi integration, which creates and launches a generic execution.
Tmux derives its display name from the descriptive execution label and stores task and execution IDs in window metadata for lifecycle verification.
The Pi integration passes a validated prompt-file reference without changing standard input, so Pi remains interactive and a failed launch remains retryable.
The historical `task start` create-and-launch shortcut remains available for direct human recovery.

`task stop` ends the tagged tmux window and preserves the durable task record and Git worktree.
`task finish` records a result only after the task process has exited.
`task archive` captures durable records, Git facts, and terminal history when available.
`task clean` refuses live tasks and unapproved loss of committed, dirty, or untracked work.
Isolated worktree removal additionally requires `--allow-worktree` and validates durable Git ownership before invoking the destructive hook.
The hook preserves the task branch and archive facts, while direct repository tasks never remove their registered checkout.
Credential cleanup remains independent, and cleanup state and recovery debt are durable and independently retryable.

`task resource archive` and `task resource clean` operate on one resource without changing sibling resource state.
`task execution archive` and `task execution stop` operate on one execution without changing resource state.
`task reconcile` repairs safe derived observations and Git facts for tasks and resources, while `task execution reconcile` repairs execution observations.
It never deletes task state, branches, worktrees, windows, or terminal history.
Legacy single-resource manifests migrate lazily to a `legacy` resource when resource operations inspect or extend them.

Agent orchestration is enabled by default over this stable CLI boundary.
The agent skill owns automated lifecycle behavior and preserves direct human CLI use.
After a command that may have mutated state fails, inspect the task and run reconciliation before attempting a manual fallback.

## Integration signal

`AKAGENT_ENABLED` remains the immediate per-environment disable signal for automated integrations.
At the CLI boundary, automation is enabled unless `AKAGENT_ENABLED` is set to the exact value `0`.
`akagent integration inspect` is read-only and reports the current state.
Direct human commands, including explicit shell execution and optional Pi selection, remain available regardless of the signal.

## Current public command surface

```text
akagent
akagent credential <list|inspect|doctor>
akagent integration inspect
akagent id generate
akagent repository <register|list|inspect|update|unregister>
akagent task <create|resource|execution|launch|start|list|inspect|attach|publish|finish|stop|archive|clean|reconcile>
akagent task resource <create|list|inspect|archive|clean>
akagent task execution <create|launch|list|inspect|publish|attach|stop|archive|reconcile>
akagent update [--source <path>]
akagent worker inspect
```

## Tracked follow-ups

- Destructive worktree and credential cleanup hooks.
- Broader workflow integrations beyond the stable CLI boundary.
- Work-specific secrets and deployment behavior.

The detailed public delivery map is in [`implementation-plan.md`](implementation-plan.md).
