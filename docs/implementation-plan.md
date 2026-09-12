# Implementation plan

## Scope

Issue #140 removes direct orchestration from the durable CLI.
The shipped core is a record-only boundary for task, resource, execution, observation, checkpoint, disposition, archive, recovery, and delivery metadata.
It does not launch processes, inspect tmux or PIDs, run Git, mutate worktrees, resolve credentials, parse provider sessions, capture terminal output, or call a forge.

External tools and skills own side effects that remain needed.
Capabilities such as deployment and credential injection may be retired rather than recreated.
Legacy records and storage schema version `1` remain readable.
The breaking CLI and lifecycle boundary is protocol version `2`.

## Shipped implementation

- Record-only task, repository, resource, and execution creation and updates.
- Caller-declared Git, branch, base, head, and worktree facts.
- Durable conditions, heartbeats, finish results, events, archives, recovery debt, and delivery metadata.
- Provider-neutral session references and metadata-only evidence views.
- Revision-scoped dispositions and bounded recovery checkpoints.
- Store-only reconciliation and explicit legacy migration.
- Deterministic in-flight, attention, maintenance, deferred, and history inventory views.
- Declarative worker capabilities and protocol version `2`.
- Pre-store structured refusal for removed orchestration, deployment, credential, and record command families.
- Subprocess canaries proving retained commands do not invoke Git, tmux, Pi, providers, or deployment tools.
- Migration documentation and updated agent lifecycle guidance.

## Command boundary

Retain repository registration, task/resource/execution records, inspection, publication, finish, archive, reconciliation, checkpoints, dispositions, inventory, integration inspection, update, ID generation, and worker inspection.

Remove launch, attach, stop, deployment, cleanup, credential, provider orchestration, and transitional `task record` commands.
Removed commands return the existing structured usage error contract with exit code `2` before store access or mutation.
Guidance names only the command family and never echoes sensitive arguments.

## Recovery contract

Recovery operates offline over durable records only.
It never creates duplicate tasks, resources, executions, branches, or worktrees.
It does not infer completion or reactivate work from age, archive state, process absence, or unavailable provider data.
Legacy unfinished and stopped records require explicit store-only adoption or migration.
Any uncertain external operation remains recovery debt until an external tool verifies it.

Manifest replacement and audit append remain separate writes.
The implementation makes no crash-atomicity claim across both files.

## Verification

Run the following before delivery:

```bash
go test ./...
go test -race ./...
go vet ./...
git diff --check
git diff --stat
```

Review the final diff for credential values, provider content, process arguments, implicit host inspection, and accidental command-family compatibility.
Do not install the binary until the integrated CLI, skills, and active-agent compatibility gate is complete on `main`.
