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

When the policy is omitted during registration, `worktree` is selected for a Git checkout.

Registering the same name with the same fields is an idempotent success.

Registering the name with different fields returns a `conflict` error.

The list command emits only the name, path, policy, and definitive total by default.

Inspect and mutation commands emit the durable registration detail.

Updating equivalent fields is a successful no-op.

An update that changes the path of a repository referenced by tasks returns a structured `conflict` error and leaves the record unchanged.

Unregister removes only the registration record.

It never removes the checkout, Git metadata, worktrees, or task files.

Unregister fails with a structured `conflict` error while any task references the repository and leaves the record intact.

```toon
repositories[1]{name,path,policy}:
  demo,/work/demo,worktree
total: 1
```

```toon
repository:
  name: demo
  path: /work/demo
  policy: worktree
  worktree_root: /work/.akagent/worktrees/demo
```

## Task commands

```text
akagent task start --title <title> --repository <name> [--task-id <id>] [--require <credential>] [--optional <credential>]
akagent task list
akagent task inspect <task-id>
akagent task publish <task-id> --condition <condition> [--reason <reason>] [--activity <activity>]
akagent task finish <task-id> <succeeded|failed> <result>
akagent task stop <task-id>
akagent task reconcile
```

A task ID is generated when `--task-id` is omitted.

The accepted published conditions are `active`, `waiting`, `blocked`, `failed`, and `none`.

Equivalent repeated starts, publications, finishes, and stops are successful no-ops.

A start with different immutable inputs returns a `conflict` error.

A finish while the task process is running fails without changing the task outcome.

Reconciliation repairs derived lifecycle state when a running task no longer has its tmux window.

The default detail schema is a single `task` object.

```toon
task:
  id: 019fe8f2-ac67-7406-a6e6-2717b2cd31c6
  title: Implement local reconciliation
  status: running
  worker: local
  condition: none
```

The list schema uses a compact tabular array and includes the definitive total.

```toon
tasks[1]{id,title,status,worker,condition}:
  019fe8f2-ac67-7406-a6e6-2717b2cd31c6,Implement local reconciliation,running,local,none
total: 1
```

Optional fields such as `reason`, `activity`, `result`, and `warnings` are emitted only when present.

## Errors

Errors use the same structured TOON envelope for every lifecycle command.

```toon
error:
  category: not_found
  message: No manifest found for task missing
  retryable: false
  recovery: Inspect the task state and retry
```

The main categories are `usage`, `not_found`, `conflict`, `capability`, `retryable`, and `internal`.

Unknown flags and malformed argument combinations are rejected with category `usage` and exit code `2` before lifecycle mutation.

A retryable store lock reports category `retryable` and `retryable: true`.

Credential failures identify only the named capability and its readiness state.

Credential values are never read into protocol output, errors, fixtures, or diagnostics.
