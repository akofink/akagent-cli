# Task CLI contract

The task command is the stable command boundary for local task lifecycle operations.

All protocol data and errors are written to stdout as TOON.
Diagnostics are not mixed into protocol output.

Exit code `0` means success or an idempotent no-op.
Exit code `1` means the requested operation could not be completed.
Exit code `2` means the command or its arguments are invalid.

## Repository registration

```text
akagent repository register <name> <path> [--policy <worktree|direct>]
akagent repository list
akagent repository inspect <name>
akagent repository update <name> [--path <path>] [--policy <worktree|direct>]
akagent repository unregister <name>
```

The path must name the root of an existing Git worktree.

When policy is omitted during registration, `worktree` is selected for a Git checkout.

The `worktree` policy creates task worktrees under `<checkout-parent>/.akagent/worktrees/<name>`.
The `direct` policy uses the registered checkout and requires the task base to match its current revision.

Registering the same name with the same fields is an idempotent success.
Registering the name with different fields returns a `conflict` error.

The list command emits the name, path, policy, and definitive total by default.
Inspect and mutation commands emit the durable registration detail.

Updating equivalent fields is a successful no-op.
An update that changes the path of a repository referenced by tasks returns a structured `conflict` error and leaves the record unchanged.

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
akagent task start --title <title> --repository <name> [--task-id <id>] [--branch <branch>] [--base <revision>] [--worktree <path>] [--agent pi --prompt <path>] [--context <value>] [--require <credential>] [--optional <credential>]
akagent task list
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

The default start creates a branch named `akagent/<task-id>` and an isolated worktree under the registered repository's worktree root.
Explicit `--branch`, `--base`, and `--worktree` values are immutable task inputs.
The start operation creates a task-tagged tmux resource.
Without managed-launch options, that resource runs the user's shell for direct work.

`--agent pi` selects the supported managed local Pi target.
The `pi` executable must be available on `PATH`, and its resolved command path is stored in the task launch configuration.

`--prompt` stores a reference to a regular local prompt file.
The launcher passes only the validated file reference to Pi and leaves standard input attached to the tmux terminal.
This preserves Pi's interactive mode while keeping prompt content out of process arguments, tmux commands, events, and protocol output.

`--context` stores one non-secret, single-line working-context value and exposes it to the managed process as `AKAGENT_WORKING_CONTEXT`.

The selected launch target, command path, task worktree, prompt reference, and working context are persisted before tmux starts.
The owned pane shows a non-secret startup line before Pi initializes, and Pi's interactive status and tool views remain visible while work runs.
A failed managed launch shows safe recovery guidance in the pane, leaves the task in recoverable `starting` state, records recovery debt, and can be retried with the same immutable inputs.

Equivalent repeated starts, publications, finishes, stops, archives, and completed cleans are successful no-ops.
A start with different immutable inputs returns a `conflict` error.

The accepted published conditions are `active`, `waiting`, `blocked`, `failed`, and `none`.
Publication updates the durable record and heartbeat.

A finish while the task process is running fails without changing the task outcome.
Stop preserves the durable record and worktree but ends the task's tagged tmux window.

Archive requires a stopped or finished task and captures the manifest, events, Git facts, and available terminal history.
Clean archives first and refuses a live task.
It requires explicit authorization for each committed, dirty, or untracked category before destructive cleanup.
For a registered `worktree` repository, `--allow-worktree` is a separate explicit approval that enables the destructive worktree cleanup hook.
The hook validates durable ownership, removes only the task worktree, preserves the task branch, and records the pre-cleanup Git facts in the archive.
Direct repository tasks never remove their registered checkout.
Without worktree approval, cleanup records preservation debt and leaves the worktree available for direct human recovery.
Credential cleanup remains independent and retryable.

Reconciliation repairs derived observations and Git facts.
It never deletes task state, branches, worktrees, windows, or terminal history.

## Attachment

Attachment requires a running shell or managed Pi task with a fresh heartbeat and process observation.
It verifies exactly one process, the durable process identity, the task-tagged tmux window, and the window option immediately before attaching.
For managed launch, the launcher replaces itself with Pi so the recorded PID and process start time identify Pi rather than a wrapper process.

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
  branch: akagent/019fe8f2-ac67-7406-a6e6-2717b2cd31c6
  worktree_path: /path/to/.akagent/worktrees/demo/019fe8f2-ac67-7406-a6e6-2717b2cd31c6
  condition: none
  committed: false
  dirty: false
  untracked: false
```

Managed task detail and list views include `agent`, `agent_command`, `prompt_reference`, and `working_context` when configured.
These fields contain launch metadata only; prompt contents and credential values are never emitted.

The list schema uses a compact tabular array and includes the definitive total.

```toon
tasks[1]{id,title,status,worker,branch,base_revision,worktree_path,condition,committed,dirty,untracked}:
  019fe8f2-ac67-7406-a6e6-2717b2cd31c6,Inspect local reconciliation,active,local,akagent/019fe8f2-ac67-7406-a6e6-2717b2cd31c6,"0000000000000000000000000000000000000001",/path/to/.akagent/worktrees/demo/019fe8f2-ac67-7406-a6e6-2717b2cd31c6,none,false,false,false
total: 1
```

Optional fields such as `reason`, `activity`, `result`, `recovery_debt`, `warnings`, archive state, cleanup state, and cleanup debt are emitted only when present.

## Orchestration and integration signal

Agent orchestration is enabled by default over the stable task CLI boundary.
The agent skill owns automated lifecycle behavior and direct human commands remain available independently.
After a command that may have mutated state fails, inspect the task and run reconciliation before attempting a manual fallback.

`AKAGENT_ENABLED` remains the immediate per-environment disable signal for automated integrations.
Automation is enabled at the CLI boundary unless `AKAGENT_ENABLED` is set to the exact value `0`.
`akagent integration inspect` reports the read-only state.
Direct human commands, including explicit managed Pi task starts, are unaffected.

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
