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
akagent task start --title <title> --repository <name> [--task-id <id>] [--branch <branch>] [--base <revision>] [--worktree <path>] [--require <credential>] [--optional <credential>]
akagent task list
akagent task inspect <task-id>
akagent task attach <task-id>
akagent task publish <task-id> --condition <condition> [--reason <reason>] [--activity <activity>]
akagent task finish <task-id> <succeeded|failed> <result>
akagent task stop <task-id>
akagent task archive <task-id>
akagent task clean <task-id> [--allow-committed] [--allow-dirty] [--allow-untracked]
akagent task reconcile
```

A task ID is generated when `--task-id` is omitted.

The default start creates a branch named `akagent/<task-id>` and an isolated worktree under the registered repository's worktree root.
Explicit `--branch`, `--base`, and `--worktree` values are immutable task inputs.
The start operation also creates a detached tmux shell tagged with the task ID.
It does not launch a managed coding-agent executable.

Equivalent repeated starts, publications, finishes, stops, archives, and completed cleans are successful no-ops.
A start with different immutable inputs returns a `conflict` error.

The accepted published conditions are `active`, `waiting`, `blocked`, `failed`, and `none`.
Publication updates the durable record and heartbeat.

A finish while the task process is running fails without changing the task outcome.
Stop preserves the durable record and worktree but ends the task's tagged tmux window.

Archive requires a stopped or finished task and captures the manifest, events, Git facts, and available terminal history.
Clean archives first, refuses a live task, and requires explicit authorization for each committed, dirty, or untracked category before destructive cleanup.
The default local cleanup hooks do not delete worktrees or credentials.

Reconciliation repairs derived observations and Git facts.
It never deletes task state, branches, worktrees, windows, or terminal history.

## Attachment

Attachment requires a running task with a fresh heartbeat and process observation.
It verifies exactly one process, the durable process identity, the task-tagged tmux window, and the window option immediately before attaching.

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

The list schema uses a compact tabular array and includes the definitive total.

```toon
tasks[1]{id,title,status,worker,branch,base_revision,worktree_path,condition,committed,dirty,untracked}:
  019fe8f2-ac67-7406-a6e6-2717b2cd31c6,Inspect local reconciliation,active,local,akagent/019fe8f2-ac67-7406-a6e6-2717b2cd31c6,"0000000000000000000000000000000000000001",/path/to/.akagent/worktrees/demo/019fe8f2-ac67-7406-a6e6-2717b2cd31c6,none,false,false,false
total: 1
```

Optional fields such as `reason`, `activity`, `result`, `recovery_debt`, `warnings`, archive state, cleanup state, and cleanup debt are emitted only when present.

## Integration gate

Automated integrations must check `AKAGENT_ENABLED` before invoking automated behavior.
Only the exact value `1` enables the integration.
`akagent integration inspect` reports the read-only state.
Direct human commands are unaffected.

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
