# Implementation plan

## Scope

The shipped core is a protocol-v2 record-only boundary for task, resource, execution, observation, checkpoint, disposition, archive, recovery, and delivery metadata.
Issue [#140](https://github.com/akofink/akagent-cli/issues/140) removed direct orchestration from the durable CLI.
Later issues added external records, inventory views, checkpoints, human terminal output, and metadata-only session evidence.

The core does not launch processes, inspect tmux or PIDs, run Git, mutate worktrees, resolve credentials, parse provider sessions, capture terminal output, or call a forge.
External tools and skills own side effects that remain needed.
Capabilities such as deployment and credential injection stay retired rather than recreated.
Legacy records and storage schema version `1` remain readable.

## Shipped implementation

- Record-only task, repository, resource, and execution creation and updates.
- Caller-owned `task external` create, finish, and archive commands that never relabel managed records.
- Caller-declared Git, branch, base, head, and worktree facts.
- Durable conditions, heartbeats, finish results, events, archives, recovery debt, and delivery metadata.
- Provider-neutral session references and metadata-only evidence views.
- Revision-scoped dispositions and bounded recovery checkpoints.
- Store-only reconciliation and explicit legacy migration.
- Deterministic in-flight, attention, maintenance, deferred, and history inventory views.
- Human presentation for `task list` and `task inspect`.
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

## Remaining work

The expected near-future work is compatibility and workflow clarity, not a return of core orchestration.

1. Complete the integrated CLI, skill, and active-agent compatibility gate before any installed-binary change.
2. Keep agent guidance aligned with managed versus external provenance so callers use `task external` only when they need caller-owned records.
3. Add later session-evidence adapter phases only when a workflow needs native discovery beyond Phase 0 metadata views.
4. Add archive and checkpoint backup procedures without a daemon or central store.
5. Keep additional machine-readable presentation formats as optional follow-ups.
   TOON remains the protocol output and human remains the explicit terminal view.

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
