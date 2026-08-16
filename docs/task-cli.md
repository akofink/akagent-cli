# Task CLI contract

The task command is the stable command boundary for local task lifecycle operations.

All protocol data and errors are written to stdout as TOON.
Diagnostics are not mixed into protocol output.

Exit code `0` means success or an idempotent no-op.
Exit code `1` means the requested operation could not be completed.
Exit code `2` means the command or its arguments are invalid.

## Repository registration

```text
akagent repository register <name> <path> [--policy <worktree|direct>] [--worktree-root <absolute-path>]
akagent repository list
akagent repository inspect <name>
akagent repository update <name> [--path <path>] [--policy <worktree|direct>] [--worktree-root <absolute-path>]
akagent repository unregister <name>
```

The path must name the root of an existing Git worktree.

When policy is omitted during registration, `worktree` is selected for a Git checkout.

The `worktree` policy creates task worktrees under the configured `--worktree-root` when one is provided.
The root must be absolute and task worktrees must remain contained beneath it.
When the option is omitted, the existing default is `<checkout-parent>/.akagent/worktrees/<name>`.
The `direct` policy uses the registered checkout and requires the task base to match its current revision.
A worktree root is valid only with the `worktree` policy.

Registering the same name with the same fields is an idempotent success.
Registering the name with different fields returns a `conflict` error.

The list command emits the name, path, policy, configured worktree root when present, and definitive total by default.
Inspect and mutation commands emit the durable registration detail.

Updating equivalent fields is a successful no-op.
An update that changes the path of a repository referenced by tasks returns a structured `conflict` error and leaves the record unchanged.
Updating the worktree root changes the containment boundary used by later task creation, cleanup ownership checks, and reconciliation.

Unregister removes only the registration record.
It never removes the checkout, Git metadata, worktrees, or task files.

Unregister fails with a structured `conflict` error while any task references the repository and leaves the record intact.

```toon
repositories[1]{name,path,policy}:
  demo,/path/to/checkout,worktree
total: 1
```

```toon
repository:
  name: demo
  path: /path/to/checkout
  policy: worktree
  worktree_root: /path/to/.akagent/worktrees/demo
```

## Task commands

```text
akagent task create --title <title> [--task-id <id>] [--repository <name>] [--require <credential>] [--optional <credential>]
akagent task resource <create|list|inspect|update|archive|clean> ...
akagent task execution <create|launch|list|inspect|publish|attach|stop|archive|reconcile> ...
akagent task launch <task-id> --target <shell|pi> [--resource <resource-id>] [--label <descriptive-label>] [--prompt <path>] [--context <value>]
akagent task list [--all] [--repository <name>] [--worktree <path>]
akagent task inspect <task-id>
akagent task attach <task-id>
akagent task publish <task-id> --condition <condition> [--reason <reason>] [--activity <activity>]
akagent task finish <task-id> <succeeded|failed> <result>
akagent task stop <task-id>
akagent task archive <task-id>
akagent task clean <task-id> [--allow-committed] [--allow-dirty] [--allow-untracked] [--allow-worktree]
akagent task reconcile
```

A task ID is generated when `--task-id` is omitted.

The default task list shows actionable records only.
It includes every non-archived task and every archived task with incomplete cleanup or recovery debt.
A fully archived, fully cleaned, debt-free task is hidden from the default list.
Use `--all` to include all durable task records.
`--repository <name>` filters by registered repository name, and `--worktree <path>` filters by exact task worktree path.
Filters compose as an intersection and results remain sorted by task ID.

Task creation persists task intent and can create zero resources.
The compatibility `--repository` form creates one initial resource without creating a tmux window or starting a process.
Use `akagent task resource create <task-id> --repository <name> [--resource-id <id>] [--branch <branch>] [--base <revision>] [--worktree <path>] [--metadata <key=value>] [--external-url <https-url>]` to add each resource.
Use `akagent task resource update <task-id> <resource-id> [--metadata <key=value>] [--external-url <https-url>]` to record mutable delivery metadata without changing Git ownership inputs.
The `worktree` policy requires an explicit descriptive `--branch` value, conventionally `akofink/<issue-or-ticket>-<2-3-word-description>`, and creates an isolated worktree under the registered repository's worktree root.
Explicit branch, base, and worktree values are immutable resource inputs.
The `direct` policy deliberately permits an omitted branch and uses the registered checkout's current branch.
The task initially has status `created` and can be inspected, archived after stopping, or launched later.

A task can own zero or more optional tool-neutral execution records.
Use `task execution create` to persist an execution without starting tmux, then `task execution launch` to start it.
Execution attachment, stop, archive, and reconcile operate on one execution and do not change resource state.
The task-tagged tmux window has a descriptive display label and stores task and execution IDs in window metadata.
Managed execution windows publish the shared tmux `@agent_state` option by matching those metadata IDs rather than the display label.
Active execution clears the option, waiting and blocked publish their values, and completed execution publishes `done`.
The managed process also receives those IDs as the non-secret `AKAGENT_TASK_ID` and `AKAGENT_EXECUTION_ID` environment variables.
An execution can use them directly with the local CLI:

```bash
akagent task resource create "$AKAGENT_TASK_ID" --repository backend --resource-id backend-resource --branch akofink/61-backend
akagent task resource list "$AKAGENT_TASK_ID"
akagent task resource update "$AKAGENT_TASK_ID" backend-resource --metadata delivery=published --external-url https://forge.example/pull/61
```
The `task launch --target shell` command creates and launches a generic shell execution.
Compatibility shell and Pi launches derive the execution and tmux display label from the selected resource or task branch, without the owner prefix.
Use `--label <descriptive-label>` when no descriptive branch is available.
Labels must not be `pi`, `shell`, `akagent`, `execution`, or an internal UUID.
New integrations should use `task create`, optional resource creation, and explicit execution create and launch operations.
One execution can coordinate multiple resources by selecting one resource as its working directory and using the owning task ID for further resource operations.
Resource lifecycle remains independent from execution lifecycle.
For example, one execution can coordinate two resources while selecting one as its working directory:

```bash
akagent task resource create <task-id> --repository frontend --resource-id frontend-resource --branch akofink/feature
akagent task resource create <task-id> --repository backend --resource-id backend-resource --branch akofink/feature
akagent task execution create <task-id> --execution-id coordinator --target shell --command /bin/sh --resource frontend-resource
akagent task execution launch <task-id> coordinator
```

`--target pi` is an optional integration target.
It checks for `pi` only when selected, then creates a generic execution whose worker integration starts Pi.
Core task, resource, and generic execution commands do not require Pi to be installed.

`--prompt` stores a reference to a regular local prompt file.
The Pi integration passes only the validated file reference to Pi and leaves standard input attached to the tmux terminal.
This preserves Pi's interactive mode while keeping prompt content out of durable events and protocol output.

`--context` stores one non-secret, single-line working-context value and exposes it to the managed process as `AKAGENT_WORKING_CONTEXT`.
Resource metadata and external URLs are provider-neutral and are preserved in resource archives, task archive resource snapshots, and reconciliation.
Agents use provider tooling such as `gh` or Bitbucket tooling to create and manage pull requests, then optionally record the resulting URL with `akagent`.
The core CLI does not provide GitHub, Bitbucket, or Pi delivery commands.

The selected execution target, command, selected resource worktree, and non-secret arguments are persisted before tmux starts.
Create and launch are separate durable operations, so a failed launch can be retried without recreating the task or Git resource.
A failed optional integration leaves its generic execution recoverable and does not change resource state.

Equivalent repeated creates, launches, publications, finishes, stops, archives, and completed cleans are successful no-ops.
A create or launch with different immutable inputs returns a `conflict` error.

The accepted published conditions are `active`, `waiting`, `blocked`, `failed`, and `none`.
Publication updates the durable record and heartbeat.
Managed execution publication maps active, failed, and none to a cleared `@agent_state` option.
Stop clears the option, archive preserves `done`, and reconciliation republishes waiting or blocked only when observations remain fresh.

A finish while the task process is running fails without changing the task outcome.
Stop preserves the durable record and worktree but ends the task's tagged tmux window.

Archive requires a stopped or finished task and captures the manifest, events, Git facts, and available terminal history.
Execution archive requires only the selected execution to be stopped or finished and does not require resource archive or cleanup.
`task resource archive` captures one resource and its events independently.
Clean archives first and refuses a live task.
`task resource clean` applies preservation approvals to one resource and leaves sibling resource cleanup state unchanged.
It requires explicit authorization for each committed, dirty, or untracked category before destructive cleanup.
For a registered `worktree` repository, `--allow-worktree` is a separate explicit approval that enables the destructive worktree cleanup hook.
The hook validates durable ownership, removes only the task worktree, preserves the task branch, and records the pre-cleanup Git facts in the archive.
Direct repository tasks never remove their registered checkout.
Without worktree approval, cleanup records preservation debt and leaves the worktree available for direct human recovery.
Credential cleanup remains independent and retryable.

Reconciliation repairs derived observations and Git facts for the task and each resource.
It never deletes task state, branches, worktrees, windows, or terminal history.
Legacy single-resource manifests are migrated lazily to a `legacy` resource when a resource command inspects or extends them.
Legacy task launch fields migrate lazily to a `legacy` execution when an execution command inspects or extends them.
Both migrations preserve durable observations and are idempotent.

## Attachment

Execution attachment requires a running execution with a fresh heartbeat and process observation.
It verifies exactly one process, the durable process identity, and matching task and execution tmux metadata immediately before attaching.
The compatibility task attach command retains its task-level verification behavior.
For the optional Pi integration, the integration worker replaces itself with Pi so the recorded PID and process start time identify Pi rather than a wrapper process.

Missing, stale, contradictory, stopped, and finished observations are rejected with structured recovery guidance.
Attachment never creates, kills, renames, or retargets tmux resources.

## Output schemas

The default detail schema is a single `task` object.

```toon
task:
  id: 019fe8f2-ac67-7406-a6e6-2717b2cd31c6
  title: Inspect local reconciliation
  status: active
  worker: local
  branch: akofink/51-task-labels
  worktree_path: /path/to/.akagent/worktrees/demo/019fe8f2-ac67-7406-a6e6-2717b2cd31c6
  condition: none
  committed: false
  dirty: false
  untracked: false
```

Compatibility task views may include an `agent` target when an optional integration is selected.
Generic execution detail is authoritative for the execution target, command, resource attachment, and recovery state.
Prompt contents and credential values are never emitted.

The list schema uses a compact tabular array and includes the definitive total.

```toon
tasks[1]{id,title,status,worker,branch,base_revision,worktree_path,condition,committed,dirty,untracked}:
  019fe8f2-ac67-7406-a6e6-2717b2cd31c6,Inspect local reconciliation,active,local,akofink/51-task-labels,"0000000000000000000000000000000000000001",/path/to/.akagent/worktrees/demo/019fe8f2-ac67-7406-a6e6-2717b2cd31c6,none,false,false,false
total: 1
```

Optional fields such as `reason`, `activity`, `result`, `recovery_debt`, `warnings`, archive state, cleanup state, and cleanup debt are emitted only when present.
Resource list output uses `resources[<n>]` with `id`, `repository`, `branch`, `base_revision`, `worktree_path`, Git facts, recovery debt, archive state, cleanup state, and optional metadata and external URLs.
Resource inspect and mutation output uses one `resource` object.

## Orchestration and integration signal

Agent orchestration is enabled by default over the stable task CLI boundary.
The agent skill owns automated lifecycle behavior and direct human commands remain available independently.
After a command that may have mutated state fails, inspect the task and run reconciliation before attempting a manual fallback.

`AKAGENT_ENABLED` remains the immediate per-environment disable signal for automated integrations.
Automation is enabled at the CLI boundary unless `AKAGENT_ENABLED` is set to the exact value `0`.
`akagent integration inspect` reports the read-only state.
Direct human commands, including explicit shell execution and optional Pi selection, are unaffected.

## Errors

Errors use the same structured TOON envelope for every lifecycle command.

```toon
error:
  category: preservation_required
  message: Cleanup requires authorization to discard untracked worktree files
  retryable: false
  recovery: Inspect the archived Git facts and retry with explicit cleanup authorization
```

The main categories are `usage`, `not_found`, `conflict`, `capability`, `retryable`, `partial`, `preservation_required`, and `internal`.

Unknown flags and malformed argument combinations are rejected with category `usage` and exit code `2` before lifecycle mutation.

A retryable store lock reports category `retryable` and `retryable: true`.
Credential failures identify only the named capability and its readiness state.
Credential values are never read into protocol output, errors, fixtures, or diagnostics.
