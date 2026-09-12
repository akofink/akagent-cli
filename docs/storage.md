# Worker-local state store

## Purpose

`internal/store` provides secure worker-local durable storage primitives for record-only task lifecycle operations.
It stores typed versioned manifests, append-only events, checkpoints, archives, locks, and recovery facts.
It is independent of tmux, Git, worktrees, credentials, providers, and CLI parsing.

## Layout

The store lives under `$XDG_STATE_HOME/akagent`, or `$HOME/.local/state/akagent` when unset:

```text
<root>/
  repositories/<name>.json
  tasks/<task-id>/manifest.json
  tasks/<task-id>/checkpoint.json
  tasks/<task-id>/events/000001.json
  tasks/<task-id>/resources/<resource-id>/manifest.json
  tasks/<task-id>/resources/<resource-id>/events/000001.json
  tasks/<task-id>/resources/<resource-id>/archive.json
  tasks/<task-id>/executions/<execution-id>/manifest.json
  tasks/<task-id>/executions/<execution-id>/events/000001.json
  tasks/<task-id>/executions/<execution-id>/archive.json
  tasks/<task-id>/archive.json
  locks/<task-id>.lock
```

Manifests are atomically replaced.
Checkpoints and archives are durable snapshots.
Events are immutable, append-only records.
Resource and execution archives are independently recoverable.
Session references contain metadata only and never provider content.

## Permissions and safety

Directories are created with mode `0700` and record and lock files with mode `0600`.
Owned paths reject group or other access, symbolic links, and non-regular record files.
Descriptor-relative traversal with no-follow semantics prevents intermediate symlink races.
Per-task locks serialize mutations and kernel locks are released when a writer exits.

## Envelope and schema

Every record uses a typed JSON envelope:

```json
{
  "schema_version": 1,
  "kind": "manifest",
  "task_id": "019fe8f2-ac67-7406-a6e6-2717b2cd31c6",
  "observed_at": "2026-08-09T21:59:00Z",
  "data": {}
}
```

`internal/store.SchemaVersion` is `1`.
Readers reject unsupported versions and malformed envelopes rather than guessing field meanings.
Optional fields may be added without a version bump.
Removing fields, changing meanings, or changing record kinds requires a storage version change.
Legacy schema version `1` remains readable after protocol version `2` orchestration removal.

## Record semantics

Task, resource, execution, repository, checkpoint, disposition, observation, delivery, archive, and recovery records are caller-declared or historical facts.
The store does not inspect paths, processes, Git, tmux, provider files, or credentials.
Legacy manifests and archives retain historical orchestration and credential fields as opaque readable data.
Store-only migration preserves IDs and facts and does not reactivate work or create duplicates.

`WriteManifest` writes a temporary sibling, applies restrictive permissions, syncs the file, renames it atomically, and syncs the directory when supported.
A reader never observes a truncated manifest.
`AppendEvent` writes a six-digit sequence under the task lock and rejects malformed names, gaps, and duplicates.
Manifest replacement and event append remain separate writes, so the durability guarantee is bounded and not crash-atomic across both files.

## Recovery

`Recover` scans valid task directories under their locks.
It removes only interrupted temporary write files, validates manifests, checkpoints, event files, and archives, and reports malformed records without deleting them.
Contended locks are reported as skipped.
Recovery is store-only and offline-safe.
It never launches processes, inspects tmux, runs Git, reads provider files, resolves credentials, or performs cleanup.

## Errors

Store failures are typed `*store.Error` values with a kind, message, retryable flag, and recovery guidance.
Kinds include `usage`, `not_found`, `lock_contention`, `malformed`, `unsafe_permissions`, `unsafe_path`, `partial`, and `internal`.
The CLI translates these into structured TOON errors.

## Boundary

The store does not implement lifecycle policy or host-side side effects.
The lifecycle layer applies record transitions and inventory views through this interface.
External tools submit observations, session references, delivery URLs, and cleanup or recovery facts through the CLI.
Credential values, prompt content, provider session content, terminal output, and secret arguments never enter the store.
