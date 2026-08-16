# Worker-local state store

## Purpose

`internal/store` provides the secure, worker-local durable storage primitives that task lifecycle commands will build on.
It stores typed, versioned task manifests and an append-only event history, resolves the XDG state root, and provides atomic writes, per-task locking, and recovery.

The package is deliberately independent of tmux, Git worktrees, credentials, and CLI command parsing.
Those surfaces read and mutate through the store rather than touching its files directly.

## Layout

The store lives under the XDG state root for the `akagent` application:

| Context | Root |
| --- | --- |
| `$XDG_STATE_HOME` set | `$XDG_STATE_HOME/akagent` |
| otherwise | `$HOME/.local/state/akagent` |

```text
<root>/
  repositories/<name>.json
  tasks/<task-id>/manifest.json
  tasks/<task-id>/events/000001.json
  tasks/<task-id>/events/000002.json
  tasks/<task-id>/resources/<resource-id>/manifest.json
  tasks/<task-id>/resources/<resource-id>/events/000001.json
  tasks/<task-id>/resources/<resource-id>/archive.json
  tasks/<task-id>/executions/<execution-id>/manifest.json
  tasks/<task-id>/executions/<execution-id>/events/000001.json
  tasks/<task-id>/executions/<execution-id>/archive.json
  tasks/<task-id>/archive.json
  locks/<task-id>.lock
```

- `manifest.json` is the mutable task manifest, atomically replaced.
- `events/<sequence>.json` is an immutable task event record; sequences are 1-based and zero-padded.
- Each resource has its own mutable manifest, event history, and archive under `resources/<resource-id>`.
- Each execution has its own mutable manifest, event history, and archive under `executions/<execution-id>`.
- Execution records contain tool-neutral command and observation metadata, not resource Git state.
- `archive.json` is an atomically replaced snapshot of the corresponding task, resource, or execution manifest and event history.
Task archives include resource snapshots so resource metadata and external delivery URLs remain available with the task record.
- `locks/<task-id>.lock` is the per-task advisory lock file, opened and locked by descriptor rather than by path.

## Permissions

The store is restrictive because event history and manifests can carry sensitive context:

- Directories are created `0700`.
- Record files and lock files are created `0600`.
- Existing directories and files with any group or other access (`mode & 0077 != 0`) are rejected with a typed `unsafe_permissions` error whose recovery suggests the exact `chmod` command.
- Symbolic links anywhere in the store tree and record paths that are not regular files are rejected with a typed `unsafe_path` error, so a task directory or record symlink cannot redirect reads or writes outside the configured state root.
- Reads and event-directory listings use descriptor-relative `openat` traversal with `O_NOFOLLOW` on every component, avoiding intermediate symlink traversal and the separate `Lstat`/read check/use race.
- `Open`/`OpenAt` validate the root, `tasks`, and `locks` directories; reads also validate task and events directories and record files.

Permissions are restored on creation even when a restrictive `umask` is not set.
The strict check applies to paths the store owns, not to ancestors such as `$HOME`.

## Envelope and schema version

Every record is a typed, versioned envelope serialized as JSON:

```json
{
  "schema_version": 1,
  "kind": "manifest",
  "task_id": "019fe8f2-ac67-7406-a6e6-2717b2cd31c6",
  "resource_id": "019f-resource",
  "observed_at": "2026-08-09T21:59:00Z",
  "data": { ... }
}
```

The envelope carries the schema version, record kind, task ID, optional resource or execution ID, and observation time (`observed_at`, UTC).
`internal/store.SchemaVersion` is `1`.
Envelopes without an observation time are rejected as malformed.

### Version behavior

- Readers reject records whose `schema_version` is anything other than the current version instead of guessing at field meanings.
- Adding optional fields is backward compatible and does not require a version bump.
- Removing fields, changing meanings, or changing the kind set requires a version bump.
- A higher-than-current version means the record came from a newer `akagent`; the operator is told to upgrade or repair.

The durable encoding is JSON.
Whether strict TOON is also safe for durable mutable records remains an open decision tracked by the TOON issue; TOON stays an output and interchange encoding until then.

## Resource semantics

A resource manifest is keyed by its owning task ID and immutable resource ID.
Resource mutations use the owning task lock, while Git setup also uses the repository lock.
Git ownership inputs remain immutable, while non-secret provider-neutral metadata and HTTPS external URLs are mutable through the lifecycle package.
Resource archive, cleanup, and recovery fields are never inferred from sibling resources.

Legacy task manifests with one embedded Git resource are migrated lazily by lifecycle resource operations.
The migration writes a `legacy` resource record and preserves the original task fields for compatibility.
Legacy task manifests with launch or process fields are migrated lazily by execution operations into a `legacy` execution record.
Execution migration preserves the legacy process identity and never starts or stops tmux.

## Manifest semantics

`WriteManifest` fully replaces the task manifest under the per-task lock using atomic replacement:

1. Write a temporary sibling file.
2. Set restrictive permissions and `fsync` it.
3. `rename` it over the target.
4. `fsync` the containing directory for durability.

`fsync` on the containing directory is tolerated to fail with `EINVAL` because some filesystems and platforms do not support directory sync.
Because replacement is atomic, a reader never observes a truncated or partially written manifest, and a failed write leaves the previous valid manifest intact.

## Event semantics

`AppendEvent` writes one immutable event and returns its sequence number.
Task, resource, and execution event sequences are computed under the per-task lock, so concurrent appends yield contiguous, non-overlapping sequences.
Event file names must be zero-padded six-digit sequences (`000001.json`) starting at 1 with no gaps; reads, appends, and recovery report malformed names and gaps rather than silently skipping them.
Hidden entries and directory entries in the events directory are also reported as malformed; only temporary write entries are removed during recovery before validation.
Events are never rewritten in place.

## Concurrency and locking

- Task mutation (`WriteManifest`, `AppendEvent`) acquires the per-task lock via `Lock`/`WithLock`.
- Resource and execution mutation acquire the owning task lock.
- `Lock` waits a short bounded time for a contended lock and returns a typed, retryable `lock_contention` error otherwise, so callers can retry safely.
- `WithLock` returns the callback's error when it fails and also surfaces failures to release the lock.
- Reads (`ReadManifest`, `ReadEvents`) do not take the lock; atomic replacement and append-only files make them safe without it.

Lock files use descriptor-relative `openat` with `O_NOFOLLOW` and direct kernel `flock` operations.
Kernel file locks are released when the owning process exits, so a crashed writer never leaves a held lock; leftover lock files are harmless marker files.
The bounded wait is long enough for durable fsync-backed mutations, while callers still receive a retryable contention error if the bound expires.

## Recovery

`Recover` scans each valid task's directory under its per-task lock and:

- Removes temporary files left behind by an interrupted write (names beginning `.akagent-write-`) with descriptor-relative `unlinkat`, never path-based `WalkDir`/`Remove`.
- Validates the manifest, each event file, and any archive snapshot.
- Reports malformed records in `RecoveryResult.MalformedRecords` without deleting them, so an operator can inspect before acting.
- Reports tasks whose lock is contended in `RecoveryResult.SkippedLocked` and leaves them alone.

## Errors

Store failures are typed `*store.Error` values carrying a kind, message, retryable flag, and recovery guidance:

| Kind | Meaning |
| --- | --- |
| `usage` | Invalid input, such as an unsafe task ID. |
| `not_found` | No record exists for the requested task. |
| `lock_contention` | Another writer holds the per-task lock; retryable. |
| `malformed` | A record is unreadable, lacks an observation time, or has an unsupported schema version. |
| `unsafe_permissions` | A store directory or file is accessible by other users. |
| `unsafe_path` | A store-owned path is a symbolic link or not a regular record file. |
| `partial` | A callback and lock release both failed, or a durable replacement completed without directory synchronization. |
| `internal` | An unexpected storage failure. |

Callers translate these kinds into protocol errors at the command boundary.

## Archive and cleanup semantics

`Archive` writes an idempotent snapshot for stopped or finished tasks and records partial attempts in the task manifest and event history so a later retry can complete the snapshot.

`Clean` never runs while a task's verified tmux identity is live.
It archives first, preserves committed, dirty, or untracked Git facts unless the operator explicitly authorizes each category, and records worktree and credential cleanup debt independently.
Resource cleanup applies the same policy to one resource and records its debt without mutating sibling resources.
Reconciliation does not invoke either destructive operation.

## Out of scope

Starting or stopping executions, task lifecycle commands, credentials, tmux, and Git remain outside this package.
The store has no Pi-specific execution fields and does not interpret command targets.
The store also persists repository registration records, including an optional absolute `worktree_root` for worktree-policy registrations.
Older registrations without that field continue to use the derived root in the lifecycle package without a migration rewrite.
The lifecycle package supplies repository validation and archive and cleanup policy.
Worktree removal is available only through the lifecycle approval-gated hook and preserves the task archive and branch.
Credential cleanup remains an independent local hook.
