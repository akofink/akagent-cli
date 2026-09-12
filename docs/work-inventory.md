# Work inventory

`akagent` separates accepted work intent from process liveness, lifecycle state, and cleanup observations.
This keeps a reboot or an interrupted terminal from turning unfinished commitments into history merely because no process is currently visible.

## Disposition

Each task can have one of three work dispositions:

- `in-flight` means the work remains an accepted unfinished commitment.
- `deferred` means the work is intentionally paused and should not appear in the default in-flight inventory.
- `terminal` means the work is complete or otherwise explicitly closed as work.

Disposition is record-only metadata.
`akagent task disposition <task-id> <in-flight|deferred|terminal> --reason <reason>` changes neither a process, a terminal, a Git worktree, nor archive or cleanup state.
The reason is required for auditability.
Use `--expected-revision <revision>` for a compare-and-set transition when multiple writers may update the same task.
The transition is idempotent when disposition and reason already match.
A normal `task finish` records a terminal disposition so completed work cannot remain in-flight accidentally.

The command returns a conflict rather than overwriting a newer disposition when the expected revision does not match.
Disposition audit events carry the resulting disposition revision, and retries repair a missing event only for that exact revision while the task lock prevents duplicate repairs.
The revision starts at zero for legacy records and increments only when an explicit disposition or reason changes.

Legacy records without disposition remain readable.
A legacy task is inferred as `terminal` only when it has an explicit finished outcome.
All other legacy records are conservatively treated as `in-flight`, including waiting, blocked, interrupted, and stopped records without a completion outcome.

## Inventory views

`akagent task list` defaults to the `in-flight` view.
`--all` retains the compatibility behavior of including every record.
New bounded views are selected with `--view`:

```text
akagent task list --view in-flight
akagent task list --view attention
akagent task list --view maintenance
akagent task list --view deferred
akagent task list --view history
```

- `in-flight` lists accepted unfinished work, including waiting and blocked commitments.
- `attention` lists in-flight work whose condition, stale or unknown observation, or recovery debt needs operator attention.
- `maintenance` lists tasks with cleanup, archive, recovery, or resource debt regardless of their work disposition.
- `deferred` lists explicitly deferred work.
- `history` lists explicitly or conservatively inferred terminal work without treating age as completion evidence.

Terminal cleanup debt is shown by `maintenance` and is not included in the in-flight view or the default inventory.
The views are read-only sweeps.
Age, process absence, archive state, cleanup state, and reconciliation never implicitly transition a task to terminal.

All list output remains deterministic TOON by default.
`--format human` remains available for direct terminal presentation.
