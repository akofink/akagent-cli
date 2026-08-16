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
- Zero or more optional tool-neutral executions with independent identity, tmux metadata, lifecycle observation, archive, stop, recovery state, and multiple non-secret session references.
- Session references contain a provider-neutral tool identifier, session ID, and optional validated absolute local reference path.
- Managed executions receive the owning task ID and execution ID as non-secret `AKAGENT_TASK_ID` and `AKAGENT_EXECUTION_ID` environment context.
- Resources preserve mutable provider-neutral metadata and HTTPS external reference URLs for delivery records.
- Git branch and worktree creation with explicit immutable branch, base, and worktree inputs.
- Task and execution-tagged detached tmux resources, shared `@agent_state` publication by execution metadata, optional Pi execution integration, and verified attachment using fresh process identity and heartbeat observations.
- A per-environment integration signal inspected by `akagent integration inspect`.
- A provider-neutral `akagent integration launch` workflow entry point that records and launches generic executions only when automation is enabled.

## Current workflow

`task create` is state-only task creation: it records durable task intent and can create zero resources without creating a tmux window or starting a process.
Worktree-policy repository registrations accept an absolute `--worktree-root` for task and resource worktrees.
When omitted, the root remains the derived `<checkout-parent>/.akagent/worktrees/<name>` path.
Containment validation, cleanup ownership checks, and reconciliation use the configured root.
The compatibility `--repository` form creates one initial resource.
`task resource create` adds additional repository, branch, and worktree combinations.
Worktree-policy resources require an explicit descriptive branch, conventionally `akofink/<issue-or-ticket>-<2-3-word-description>`.
When `--worktree` is omitted, the worktree directory uses the branch label after the owner prefix beneath the registered root, such as `80-worktree-labels` for `akofink/80-worktree-labels`.
Direct-policy tasks deliberately use the registered checkout's current branch when no branch is provided.
`task execution create` records an optional tool-neutral execution without a process side effect.
The provider-neutral `akagent integration launch` command is the smallest automated workflow adapter: it checks `AKAGENT_ENABLED` before opening the state store, persists a `workflow` execution, and launches it through the same generic lifecycle.
A disabled integration returns a skipped success without creating task, resource, execution, or tmux state.
The command requires a caller-supplied stable execution ID so retries remain idempotent.
`task execution launch` starts the selected execution, and a multi-resource task may attach one resource with `--resource` during execution creation.
`task execution session add` records provider-neutral session provenance without parsing Pi or another provider's session files.
Execution stop, archive, attach, and reconcile operate independently from resource state.
The `task launch --target shell` path creates and launches a generic shell execution.
The optional `task launch --target pi` path delegates to the Pi integration, which creates and launches a generic execution.
Compatibility launches derive their execution and tmux display labels from the selected resource or task branch without the owner prefix, or require an explicit descriptive `--label`.
Tmux stores task and execution IDs in window metadata for lifecycle verification.
Managed execution lifecycle state uses those metadata IDs to clear active state, publish waiting or blocked state, and mark completed execution `done` through `@agent_state`.
The Pi integration passes a validated prompt-file reference without changing standard input, so Pi remains interactive and a failed launch remains retryable.
The historical `task start` create-and-launch shortcut is rejected with structured migration guidance.
Managed execution commands inherit the non-secret `AKAGENT_TASK_ID` and `AKAGENT_EXECUTION_ID` context and can call the local CLI directly to create, inspect, and update resources owned by that task.
Resource metadata is intentionally generic, with `--metadata key=value` and `--external-url https://...` available through `task resource create` and `task resource update`.
The external URL is a delivery reference only.
Agents use provider tooling such as `gh` or Bitbucket tooling to create and manage pull requests, then optionally record the resulting URL in `akagent`.
The core CLI does not call GitHub, Bitbucket, Pi, or another forge delivery API.

`task stop` ends the tagged tmux window and preserves the durable task record and Git worktree.
Execution stop verifies that its tagged window is absent before recording `stopped`; a live or unavailable window returns a structured retryable failure.
`task finish` records a result only after the task and execution processes have exited.
`task archive` captures durable records, Git facts, and terminal history when available.
`task clean` refuses live tasks and unapproved loss of committed, dirty, or untracked work.
Isolated worktree removal additionally requires `--allow-worktree` and validates durable Git ownership before invoking the destructive hook.
The hook preserves the task branch and archive facts, while direct repository tasks never remove their registered checkout.
Credential cleanup remains independent, and cleanup state and recovery debt are durable and independently retryable.
It is exposed through `akagent credential clean <task-id>`, `akagent task credential clean <task-id>`, and the `--allow-credentials` task cleanup approval.
Credential cleanup refusal and hook failures never mutate the credential manifest or unrelated Git resource state.

`task resource archive`, `task resource clean`, and `task resource update` operate on one resource without changing sibling resource state.
`task execution archive` and `task execution stop` operate on one execution without changing resource state.
`task reconcile` repairs safe derived observations and Git facts for tasks and resources, while `task execution reconcile` repairs execution observations without changing resource state.
Reconciliation may close a matching tagged window for a non-running execution and verifies it is gone before updating the observation.
It never deletes task state, branches, worktrees, terminal history, or unverified windows.
Legacy single-resource manifests migrate lazily to a `legacy` resource when resource operations inspect or extend them.

Agent orchestration is enabled by default over this stable CLI boundary.
The agent skill owns automated lifecycle behavior and preserves direct human CLI use.
After a command that may have mutated state fails, inspect the task and run reconciliation before attempting a manual fallback.

## Durable work-state model and migration boundary

`akagent task inspect <task-id>` is the durable source of active work state.
Its task detail includes task lifecycle, branch and worktree facts, conditions, activity, result, recovery state, all resources, resource Git facts, generic delivery metadata, all executions, execution tool and process provenance, and provider-neutral session references.
Task and execution archives preserve the same records for recovery after cleanup or provider state changes.
List commands remain concise and include compact session summaries where appropriate.

Session references are declarations from an integration, not provider adapters in the core lifecycle.
An integration discovers its own resumable session state and calls the generic session-reference update surface or `task execution session add`.
`akagent` validates only the non-secret shape and local path reference, then persists the reference without opening or parsing the provider file.
Missing provider files do not invalidate historical records.

This is the migration boundary from the shared `WORKING_STATE.md` board.
Existing task records migrate lazily, with absent session references treated as empty and legacy task launch state detached into generic resources and executions as already documented.
After migration, agents and operators inspect `akagent` task state rather than reading or updating a shared `WORKING_STATE.md`.
`WORKING_STATE.md` is not a durable input to task lifecycle, reconciliation, archive, or cleanup, and the core CLI does not create or interpret it.
The repository coding workflow no longer depends on a shared `WORKING_STATE.md` board after this migration.
Provider-specific session discovery and forge-specific delivery behavior remain outside this repository.

## Task context and delivery metadata

A managed execution receives `AKAGENT_TASK_ID` and `AKAGENT_EXECUTION_ID` in its environment.
These values are task and execution identities, not credentials, and are safe to pass to local `akagent` commands.
An execution can use the task context without a parent orchestrator, for example:

```bash
akagent task resource create "$AKAGENT_TASK_ID" --repository backend --resource-id backend-resource --branch akofink/61-backend
akagent task resource list "$AKAGENT_TASK_ID"
akagent task resource update "$AKAGENT_TASK_ID" backend-resource --metadata delivery=published --external-url https://forge.example/pull/61
```

Resource Git ownership inputs remain immutable after creation.
Resource metadata and external URLs are provider-neutral and are preserved by resource and task archives and reconciliation.
The external URL boundary is deliberate.
Agents perform provider-specific pull request operations with tools such as `gh` or Bitbucket tooling, then optionally record a resulting URL with `task resource update`.
`akagent` does not provide GitHub, Bitbucket, or Pi delivery commands.

## Integration signal

`AKAGENT_ENABLED` remains the immediate per-environment disable signal for automated integrations.
At the CLI boundary, automation is enabled unless `AKAGENT_ENABLED` is set to the exact value `0`.
`akagent integration inspect` is read-only and reports the current state.
The provider-neutral `integration launch` command is the automated workflow entry point and checks the signal before opening the state store or creating an execution.
When disabled, it returns a skipped success without lifecycle side effects.
When enabled, it records and launches a generic `workflow` execution through the normal task lifecycle.
Direct human commands, including explicit shell execution and optional Pi selection, remain available regardless of the signal.

## Current public command surface

```text
akagent
akagent credential <list|inspect|doctor|clean>
akagent integration <inspect|launch>
akagent id generate
akagent repository <register|list|inspect|update|unregister>
akagent task <create|resource|execution|credential|launch|list|inspect|attach|publish|finish|stop|archive|clean|reconcile>
akagent task resource <create|list|inspect|update|archive|clean>
akagent task execution <create|launch|list|inspect|session|publish|attach|stop|archive|reconcile>
akagent update [--source <path>]
akagent worker inspect
```

## Tracked follow-ups

- Work-specific secrets and deployment behavior.

The detailed public delivery map is in [`implementation-plan.md`](implementation-plan.md).
