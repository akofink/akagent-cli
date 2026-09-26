# Implementation plan

## Scope

The shipped core is a protocol-v2 record-only boundary for task, resource, execution, observation, checkpoint, disposition, archive, recovery, and delivery metadata.
Issue [#140](https://github.com/akofink/akagent-cli/issues/140) removed direct orchestration from the durable CLI.
Later issues added external records, inventory views, checkpoints, human terminal output, and metadata-only session evidence.

The core does not launch processes, inspect tmux or PIDs, run Git, mutate worktrees, resolve credentials, parse provider sessions, capture terminal output, or call a forge.
External tools and skills own side effects that remain needed.
Capabilities such as deployment and credential injection stay retired rather than recreated.
Legacy records and storage schema version `1` remain readable.

## Shipped status

See [Roadmap](roadmap.md) for the authoritative shipped-capabilities list and prioritized follow-up work.

## Command boundary

Retain repository registration, task/resource/execution records, inspection, publication, finish, archive, reconciliation, checkpoints, dispositions, inventory, update, ID generation, and worker inspection.

Remove launch, attach, stop, deployment, cleanup, credential, integration, provider orchestration, and transitional `task record` commands.
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

## Remaining work

See [Roadmap](roadmap.md) for the authoritative prioritized follow-up work.
The expected near-future work is compatibility and workflow clarity, not a return of core orchestration.

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
