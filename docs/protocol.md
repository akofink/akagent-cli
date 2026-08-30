# Protocol

## Scope

The protocol is the stable contract between direct commands, local tasks, and integrations.
It covers identity, records, lifecycle semantics, output, errors, compatibility, and reconciliation.

It does not standardize cloud provisioning or imply that all workers have identical capabilities.
Execution records are tool-neutral.
Optional integrations, including Pi, build on the execution interface rather than task or resource lifecycle behavior.

## Resources

### Worker

A worker is a named machine on which local worker inspection can operate.

```toon
worker:
  id: local
  protocol_version: 1
  architecture: arm64
  operating_system: linux
  features[2]: tmux,git-worktree
```

Capabilities include architecture, operating system, execution features, capacity, protocol version, and image or configuration version where relevant.
The current CLI exposes `akagent worker inspect` for the implicit local worker.

### Task

A task has an immutable ID, operator request, and zero or more optional execution records.
A task may own zero or more Git resources.
Executions and resources are separate records with independent lifecycle and recovery state.
Task creation can therefore record intent without selecting a repository or creating a worktree.

```toon
task:
  id: 019fe8f2-ac67-7406-a6e6-2717b2cd31c6
  title: Inspect local reconciliation
  worker: local
  repository: example
```

Task IDs are generated before dispatch and are safe for filenames, tmux options, and command arguments.
UUIDv7 provides sortable random identity without worker-local coordination.

Titles, branches, tickets, tmux names, and prompts are attributes rather than identity.

### Task execution

An execution is an optional tool-neutral process associated with one task.
Its durable ID is independent from both the task ID and every resource ID.
The record stores a descriptive label, target, command metadata, optional resource attachment metadata, lifecycle state, process identity, tmux observations, heartbeat, archive state, recovery debt, and zero or more session references.
The core execution record has no Pi-specific fields.
A session reference contains only a provider-neutral tool identifier, session ID, and optional absolute local reference path.
The path is a reference to provider-owned state, not session content, and may be absent or no longer present when the execution is inspected.
Phase 0 evidence views derive metadata-only captures from these references without parsing provider artifacts.

```toon
execution:
  id: 019f...
  task_id: 019f...
  label: review-shell
  target: shell
  resource_id: 019f...
  lifecycle: running
  tmux_window: '@3'
  observation: fresh
```

The task ID and execution ID are both written to owned tmux window metadata.
The display label is descriptive and never derived from an internal UUID.
Compatibility launches derive that label from the selected resource or task branch, removing the owner prefix, or require `--label <descriptive-label>` when no branch is available.
The compatibility label must not be `pi`, `shell`, `akagent`, `execution`, or an internal UUID.
A managed execution also receives `AKAGENT_TASK_ID` and `AKAGENT_EXECUTION_ID` as non-secret environment context.
An execution can use those values with the local CLI to create, inspect, and update resources for its owning task.
A resource ID is attachment metadata used to select a working directory and verify durable intent; execution lifecycle operations do not mutate or require the resource's archive or cleanup state.
Integrations record their own session references with the generic execution update surface.
The core CLI never parses Pi or another provider's session files.
The managed execution window also publishes the shared tmux `@agent_state` option using those metadata IDs, never its display label.
Active execution clears the option, while waiting, blocked, and completed execution publish `waiting`, `blocked`, and `done` respectively.

### Task resource

A task resource has immutable repository, branch, base revision, and worktree ownership inputs.
Its provider-neutral metadata and external URLs are mutable delivery observations.
Each resource has its own Git facts, archive state, cleanup state, and recovery debt.
Resource records are independently archived, inspected, reconciled, and cleaned.

```toon
resource:
  id: 019f...
  repository: backend
  branch: akofink/57-backend
  worktree_path: /path/to/.akagent/worktrees/backend/57-backend
  metadata:
    delivery: published
  external_urls[1]: https://forge.example/pull/61
  committed: false
  dirty: false
  untracked: false
```

### Repository

A repository registration resolves a stable name to a local Git checkout and policy.

The `worktree` policy creates an isolated task branch and worktree under the registration's worktree root.
When `--worktree` is omitted, the worktree directory name is the branch label after the owner prefix, such as `80-worktree-labels` for `akofink/80-worktree-labels`.
Registrations may set that root with an absolute `--worktree-root` value, such as `~/dev/worktrees/backend` after shell expansion.
When no value is configured, the root remains `<checkout-parent>/.akagent/worktrees/<name>`.
Task and resource creation reject worktree paths outside the root.
Cleanup ownership checks and reconciliation apply the same boundary.
The `direct` policy uses the registered checkout and requires its base revision to match.
A worktree root is valid only for the `worktree` policy.

### Credential capability

A credential capability is a named permission needed by a task.
The task record stores the capability ID and readiness information without storing the value.

### Execution launch

An execution launch persists the resolved command, arguments, working directory, and optional resource attachment before tmux starts.
The launcher keeps standard input attached to the tmux terminal for interactive tools.
Credential values and prompt contents are never copied into process arguments, task events, or protocol output.
Optional integrations map their provider configuration onto tool-neutral execution records.
Pi-specific launch behavior is implemented outside the core task, resource, and execution records.

An optional managed integration may receive a minimal safe runtime environment, `AKAGENT_TASK_ID`, and only requested environment credentials that passed readiness checks.
Managed executions additionally receive `AKAGENT_EXECUTION_ID`.
Optional requirements are recorded as non-secret warnings and are not injected.
File credentials can satisfy readiness requirements but cannot be injected into the managed environment.

External delivery is outside the core lifecycle.
Agents use provider tooling such as `gh` or Bitbucket tooling to create and manage pull requests, then optionally record a provider-neutral URL with `task resource update`.
The core CLI has no GitHub, Bitbucket, or Pi-specific delivery API.

## State model

Lifecycle phase:

```text
created | starting | running | stopped | finished
```

Agent-declared condition:

```text
active | waiting | blocked | failed | none
```

Observed facts include execution tmux window existence, process identity and start time, heartbeat, Git status, HEAD, archive status, cleanup status, and credential readiness.

The compact `status` field is a computed view:

1. `created` after durable task creation and any requested Git resource creation, before an execution is selected.
2. `starting` while recoverable execution startup is incomplete.
3. `failed` when the condition or finish outcome records failure.
4. `waiting` or `blocked` when a fresh process observation has that condition.
5. `active` when a fresh task process exists without a stronger condition.
6. `finished` when finish recorded an outcome and no process remains.
7. `stopped` when stop completed without a finish outcome.
8. `unknown` when required observations are unavailable, stale, or contradictory.

`committed`, `dirty`, and `untracked` are Git facts.
Archive and cleanup state are independent recovery facts.

## Current local commands

The current executable exposes:

```text
akagent
akagent credential <list|inspect|doctor|clean>
akagent integration <inspect|launch>
akagent id generate
akagent repository <register|list|inspect|update|unregister>
akagent task <create|deploy|resource|execution|credential|launch|list|inspect|attach|publish|finish|stop|archive|clean|reconcile>
akagent task list [keyword] [--all] [--repository <name>] [--worktree <path>] [--format <toon|human>]
akagent task inspect <task-id|keyword> [--format <toon|human>]
akagent task resource <create|list|inspect|update|archive|clean>
akagent task execution <create|launch|list|inspect|session|evidence|publish|attach|stop|archive|reconcile>
akagent update [--source <path>]
akagent worker inspect
```

Task creation validates credential requirements and persists a task manifest without creating a tmux window or starting a process.
When `--repository` is supplied for compatibility, it also creates the initial legacy resource.
A task with no repository starts with zero resources.
`task resource create` adds each immutable repository, branch, base, and worktree association and creates its Git worktree when needed.
Worktree-policy tasks require an explicit descriptive branch, conventionally `akofink/<issue-or-ticket>-<2-3-word-description>`.
Direct-policy tasks deliberately use the registered checkout's current branch when no branch is provided.

The execution operation creates an optional tool-neutral record without a tmux or process side effect.
`task execution launch` starts that record in a task-tagged tmux window and records the process identity.
The display label is descriptive, while the task and execution IDs remain in tmux metadata used for lifecycle verification.
Launch clears any stale `@agent_state` value for an active execution.
A launch failure leaves the execution in recoverable `starting` state and records recovery debt.
The `task launch --target shell` command creates and launches a generic execution record.
The `task launch --target pi` target delegates to the Pi integration, which creates and launches a generic execution record.
Managed Pi launches default to the validated non-secret policy `--provider openai-codex --model gpt-5.6-luna --thinking high`.
Callers may override each policy field with `--provider`, `--model`, or `--thinking`; credentials are resolved separately and never persisted in launch policy.
Both compatibility targets use a descriptive branch-derived execution and tmux label, or an explicit `--label` value.

## Lifecycle operations

### Create and launch

The operator surface generates a task ID when omitted.
Task creation is recoverable and records these steps:

1. Check named required and optional credential capabilities.
2. Persist the task manifest and create event.
3. For a compatibility `--repository` input, resolve and lock the repository.
4. For a compatibility `--repository` input, validate policy, branch, base, and worktree.

Resource creation separately resolves and locks its repository, creates or validates its branch and worktree, and records a resource event.
It may also record non-secret `--metadata key=value` values and provider-neutral `--external-url https://...` references.
`task resource update` changes only those mutable metadata fields and records a resource event without changing Git ownership inputs.
Neither operation has a tmux or process side effect.
Repeated equivalent creates return the existing task with exit code `0`.
Conflicting immutable inputs return a structured conflict.

Resource creation and execution creation remain separate durable operations.
Execution creation persists target-neutral immutable inputs and never creates a tmux window.
Execution launch selects an existing execution and records the observed process identity.
A resource attachment is optional and only supplies verified working-directory metadata.
Repeated equivalent creates and launches are successful no-ops.
A failed launch remains retryable without recreating the task or selected resource.
A task can own zero or more executions, and each execution can be inspected, attached, stopped, archived, and reconciled independently.
`task execution session add` appends an idempotent provider-neutral session reference without provider-specific parsing.
`task execution evidence list` and `task execution evidence inspect` provide read-only metadata evidence derived from those references.

### Publish state

A trusted local integration or shell may publish `active`, `waiting`, `blocked`, `failed`, or `none` with a concise reason and activity.
Publication updates the durable task record and heartbeat.
For managed executions, active, failed, and none clear `@agent_state`, while waiting and blocked publish their matching values.
Finish and archive preserve `done`, stop clears the option, and reconciliation clears stale state when the execution is no longer actively waiting or blocked.
It does not make a declaration authoritative over contradictory process or Git observations.

### Inspect and list

List views default to compact decision-relevant fields and include a definitive total.
Task resource and execution operations use the same compact TOON boundary by default.
`task list` and `task inspect` accept `--format human` for a deterministic terminal-oriented presentation; errors remain TOON.
`task resource list` returns `resources[]` and `total`, while inspect returns one `resource` object.
Metadata and external URLs are emitted when present and are preserved in resource archives, task archive resource snapshots, and reconciliation.
`task execution list` returns `executions[]` and `total`, while inspect returns one `execution` object.
The default list includes actionable records: non-archived tasks and archived tasks with incomplete task or resource cleanup, cleanup debt, or recovery debt.
Only fully archived, fully cleaned, debt-free records are hidden by default.
`task list --all` includes all durable task records, including historical records that are complete.
`task list [keyword]` filters by case-sensitive substring matches against task titles and task or resource branches only.
It does not match task IDs, repository names, or worktree paths.
`--repository <name>` and `--worktree <path>` apply deterministic exact-match filters that compose with `--all` and keyword matching.
`task inspect <task-id|keyword>` accepts an exact task ID or a keyword that must match exactly one task by title or branch.
Detail views include task identity, computed status, branch and worktree facts, conditions, results, recovery fields, all task resources, and all task executions when present.
The human task list is a fixed-column pipe-delimited table, and the human task inspect view uses labeled fields with numbered resource and execution sections.
Human output does not depend on terminal width or color and escapes control characters and pipe delimiters in values.
Execution detail includes full session references, while execution list output includes a compact `tool:session-id` summary.
Execution evidence list output includes an `evidence` summary and `captures[<n>]` rows derived from existing session references.
A capture with an existing regular local artifact is `available` with evidence class `observed`; a capture without a path is `unknown` with evidence class `recorded`; and a capture whose path is missing, unreadable, a symlink, a directory, or another non-regular file is `unavailable`.
An execution with no session references is reported distinctly as `unavailable` with reason `no_session_references` and zero captures.

```toon
tasks[1]{id,title,status,worker,condition}:
  019f...,Inspect reconciliation,active,local,none
total: 1
```

### Attach

Execution attachment first resolves the durable task and execution records.
It proceeds only for a running execution with a fresh heartbeat, a fresh process observation, exactly one matching process, and a tmux window containing matching `@akagent_task_id` and `@akagent_execution_id` metadata.

The command rechecks both task and execution options on the selected window immediately before running `tmux attach-session` against that verified window ID.

Missing, stale, contradictory, stopped, and finished observations are rejected with structured recovery guidance.
Attachment does not write durable state and never creates, kills, renames, or retargets tmux resources.

### Stop

Execution stop verifies the execution's tagged tmux metadata, terminates only that execution window, and re-observes it before changing durable state.
If the tagged window cannot be observed or remains live, stop returns a structured retryable error and leaves the execution lifecycle unchanged.
It preserves the task record, execution record, and Git resources, then records `stopped` without claiming a successful outcome.
Stopping an already stopped or finished execution verifies that no tagged process remains before returning a successful no-op.
The compatibility task stop operation retains its historical task-level behavior.

### Finish

Finish accepts `succeeded` or `failed` and a concise result after the task process has exited.
It moves lifecycle to `finished` and refreshes Git facts.

The current command records the result string only.
It does not infer verification success, commit state, or worktree cleanliness from the result.

### Archive

Execution archive accepts a stopped or finished execution after confirming no execution process is live.
It captures the execution manifest, execution event history, and terminal history when available.
Execution archive does not require a resource to be archived or cleaned and never changes resource state.
Unavailable terminal history is recorded as a warning.
Partial archive attempts remain retryable, and equivalent archives are idempotent.
Task archives include the durable resource and execution snapshots, including session references.
The compatibility task and resource archive operations retain their existing scope.

### Clean

Clean archives first and refuses destructive action while a verified task process runs.
`task resource clean` applies the same explicit preservation approvals to one resource only.
A blocked or partial resource cleanup leaves that resource's debt durable and does not mark sibling resources cleaned.
It preserves committed, dirty, and untracked Git facts unless the operator explicitly supplies `--allow-committed`, `--allow-dirty`, and `--allow-untracked` for the corresponding categories.

For a registered `worktree` repository, removing the task worktree also requires the separate `--allow-worktree` approval.
The cleanup hook validates the durable repository, path, branch, and Git common directory before removing only the task worktree with Git.
It preserves the task branch and the archive's pre-cleanup Git facts.
Direct repository tasks never remove their registered checkout.
Worktree and credential cleanup state are recorded independently so partial cleanup can be retried.
Without worktree approval, the worktree remains available for direct human recovery and cleanup debt is durable.
Credential cleanup requires the separate `--allow-credentials` approval.
`akagent credential clean <task-id>` and `akagent task credential clean <task-id>` run only the credential hook, while `task clean` may retry it alongside remaining Git cleanup.
Refused and failed hooks preserve the durable task and credential manifest state and emit only redaction-safe structured errors and events.

### Reconcile

Execution reconciliation compares each durable execution with its task-tagged tmux and process observations.
It repairs safe derived metadata, republishes waiting or blocked state only with fresh observations, and clears stale `@agent_state` values.
For a non-running execution, a live window with matching task and execution metadata is safely stopped and verified absent.
If that cleanup cannot converge, reconciliation returns a structured retryable error without claiming the execution is stopped.
It never deletes task state, executions, branches, worktrees, or terminal history, and never removes an unverified window.
Task reconciliation continues to repair legacy task and resource observations.
It preserves session references as declared integration metadata and does not inspect provider state.

## Output

Agent-consumed stdout uses conforming TOON by default.
`task list` and `task inspect` are the only commands with the current explicit human-readable override, selected with `--format human`.
Internal logic uses typed records and encodes TOON at the boundary, while human output is rendered from the same typed task views.

The implementation pins an exact supported TOON specification version and a constrained output subset.
See [`toon.md`](toon.md) for the pinned version, supported forms, known deviations, encoder decision, and token measurements.
Protocol compatibility is defined by `akagent` schemas and semantics, not by TOON alone.

## Errors

Structured successes and errors go to stdout.
Diagnostics and progress do not mix with protocol output.

```toon
error:
  category: conflict
  message: Task inputs conflict with the existing task
  retryable: false
  recovery: akagent task inspect <task-id>
```

Categories include `usage`, `not_found`, `conflict`, `retryable`, `partial`, `preservation_required`, `capability`, and `internal`.
Exit code `0` means success or no-op.
Exit code `1` means the requested operation could not be completed.
Exit code `2` means the command or its arguments are invalid.

## Self-service and optional integration signal

Agent self-service is the normal workflow over this stable CLI protocol.
Coding agents create, inspect, update, reconcile, and archive their own task, resource, and execution state through direct CLI commands.
Optional integrations may use the same lifecycle, while direct human commands remain available independently.
After a command that may have mutated state fails, inspect the task and run reconciliation before attempting a manual fallback.

`AKAGENT_ENABLED` is the immediate per-environment compatibility signal for optional automated integrations.
At the CLI boundary, optional automation is enabled unless `AKAGENT_ENABLED` is set to the exact value `0`.
`akagent integration inspect` reports the read-only compatibility state and is not a prerequisite for direct CLI use.
`akagent integration launch` is an optional provider-neutral automated workflow entry point.
It requires a stable execution ID, checks the signal before opening the state store, and uses the generic execution lifecycle with target `workflow`.
When disabled, it returns a skipped success and creates no lifecycle state.
The signal controls optional automated integration invocation only.
Direct agent and human commands, including explicit shell execution and optional Pi selection, remain available regardless of the signal.

## Concurrency and consistency

Task and execution mutation use the owning task lock.
Resource mutation uses the owning task lock and a per-repository lock for Git setup.
Repository mutation uses a per-repository lock.
Manifest replacement uses atomic rename and synchronization.
Execution event sequences are computed while holding the task lock, so concurrent execution appends cannot overlap or create gaps.
Task ID, execution ID, and operation name form the default idempotency identity.
Reads identify observation time and tolerate concurrent change.

## Compatibility

Protocol responses include version metadata where required by the command contract.
Adding optional fields is compatible.
Removing fields, changing meanings, or changing lifecycle semantics requires a protocol version change.
Interrupted legacy `starting` manifests without a recorded process identity migrate to `created` when the task is created again.
Legacy manifests with a recorded process remain attached to their observed execution and are not relaunched by migration.
Legacy repository registrations without `worktree_root` continue to use the derived root lazily, without rewriting the registration.
Legacy manifests with one repository, branch, and worktree are migrated lazily when resource operations inspect or extend them.
Legacy manifests with launch, tmux, or process fields are migrated lazily when execution operations inspect or extend them.
Older execution records without session references migrate by treating the field as empty.
The migration boundary ends at the provider-neutral session reference: integrations may discover provider state and record a non-secret reference, but `akagent` does not import or interpret provider session files.
The migration creates a `legacy` execution and preserves the existing task execution fields, target metadata, process identity, tmux window, observation, archive state, cleanup state, and recovery debt.
The migration is idempotent and does not start, stop, rename, or archive a process.
Resource migration and execution migration are independent, so either can be recovered without changing the other record.
Human-readable text is not a stable parsing interface.
The human format is a presentation contract for direct terminal use, not a replacement for the default TOON protocol.
