# Recovery checkpoints

A recovery checkpoint is the durable handoff for an accepted unfinished task.
It preserves the next safe action and the evidence needed to choose that action after a process restart or reboot-equivalent interruption.
It does not restore a terminal, process memory, tmux window, provider transcript, dirty-file contents, or an external operation.

## Record contract

Checkpoints are stored as versioned envelopes at `tasks/<task-id>/checkpoint.json` inside the secure worker-local store.
The checkpoint is written atomically under the task lock and is authoritative even when recording its accompanying event is interrupted.
A failed event append returns a retryable partial error and persists audit debt identified by the checkpoint revision and operation receipt.
A retry repairs the missing event under the same task lock and clears that debt without creating another revision.
This also applies when later checkpoint revisions were acknowledged first.

Each checkpoint contains:

- `revision`, `idempotency_key`, `task_id`, and `acknowledged_at`.
- Durable per-operation receipts containing key and payload digests, original revision, and acknowledgement time.
- Audit debt identifying acknowledged revisions whose audit event still needs repair.
- `task_kind`, `completion_contract`, and a bounded `next_action`.
- Purpose-tagged context references that point to documents without embedding their contents.
- Resource references with stable resource IDs and optional non-secret Git observations.
- Provider-neutral session references and previous-attempt references.
- Optional verification scope, revision, result, and time.
- Uncertain external operations with a reference key and an explicit verify-before-replay instruction.

References are declarations.
A missing document, worktree, provider artifact, network, or external service does not turn a historical reference into an invented recovery instruction.
Core inspection never opens provider references or executes operation text.

## CLI contract

Write a checkpoint with an explicit optimistic revision and caller idempotency key:

```text
akagent task checkpoint write <task-id> \
  --idempotency-key <key> \
  --expected-revision <revision> \
  --task-kind <kind> \
  --completion-contract <contract> \
  --next-action <action> \
  [--context <purpose=reference>] \
  [--resource-reference <resource-id>] \
  [--session-reference <execution-id=tool:session-id>] \
  [--previous-attempt-reference <reference>] \
  [--verification-scope <scope>] \
  [--verification-revision <revision>] \
  [--verification-result <result>] \
  [--verification-time <RFC3339>] \
  [--uncertain-operation <reference-key=operation>]
```

A first write uses `--expected-revision 0`.
A successful write returns an acknowledged checkpoint with revision `1`.
A later write must use the currently acknowledged revision.
A repeated identical write with the same idempotency key is an idempotent success.
After later revisions, an identical retry returns the original acknowledged revision from its receipt and repairs any audit debt.
A changed payload with an existing key or a stale expected revision returns a structured conflict and leaves the authoritative checkpoint unchanged.

Inspect a checkpoint without requiring tmux, Git, provider binaries, network, or credentials:

```text
akagent task checkpoint inspect <task-id>
```

A task created before checkpoints were introduced returns `available: false` and `state: unavailable` with guidance to create a checkpoint.
It does not fabricate a next action from task status, branch names, process IDs, or terminal history.

## Reboot-equivalent recovery drill

This drill validates a fresh process against persistent state, not a physical reboot.

1. Create a temporary state root and a state-only task.
2. Write a checkpoint with `--expected-revision 0` and a unique idempotency key.
3. End the first CLI process.
4. Start a second CLI process with the same state root and no tmux, Git, provider, or network dependency available.
5. Run `akagent task checkpoint inspect <task-id>` and confirm the acknowledged revision, next action, context references, and verify-only external operation are unchanged.
6. Repeat the original write and confirm it returns the same acknowledged revision.
7. Attempt a stale write with a different key and confirm it is rejected without changing the checkpoint.

This drill does not validate kernel recovery, filesystem durability across power loss, disk-loss backup restoration, native provider-session resumption, or a real machine reboot.
Those require separate disposable-machine and backup-recovery validation.
