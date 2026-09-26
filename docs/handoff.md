# Current handoff

## Goal

`akagent` is a local-first durable registry protocol and CLI for coding agents.
The core records task, repository, resource, execution, observation, checkpoint, disposition, archive, recovery, and delivery facts.
External tools and skills own process, tmux, Git, worktree, credential, provider, cleanup, deployment, and forge side effects.

## Shipped behavior

See [Roadmap](roadmap.md) for the authoritative shipped-capabilities list and prioritized follow-up work.

## Current workflow

Adopt an existing task with `AKAGENT_TASK_ID` and inspect it before editing.
Otherwise create task intent and caller-declared resources directly through the CLI.
Create an execution only when an external tool needs explicit identity.
Use `task external` when the caller needs owned provenance, operation IDs, and revision-checked completion.
A managed `task execution create --target external` records metadata only and does not create an external execution.
`task execution finish` can close that managed execution with its displayed revision, including `0`, before or after the parent task is terminal, without relabeling it.
Reconcile does not infer that completion.
Publish conditions and activity as durable records.
Record provider session references and delivery URLs without provider content.
Run `task check` to compare cached records with live Git and GitHub state instead of re-recording those facts.
After an uncertain failure, inspect and reconcile before retrying.
Finish explicitly against the caller's completion contract, then archive.

The core never launches or stops a process, inspects tmux or PIDs, runs Git, mutates a worktree, resolves credential values, reads a provider file, captures terminal output, or calls a forge.
The opt-in `task check` command is the one exception: it reads Git and GitHub through their own CLIs and never writes to the store, a checkout, or the forge.
A missing process, checkout, provider session, credential, or network never proves completion.

## Recovery and migration

Reconciliation is store-only and offline-safe.
It preserves missing, stale, unavailable, and contradictory observations.
It never creates duplicate tasks, resources, executions, branches, or worktrees.
Legacy unfinished and stopped work requires explicit store-only adoption or migration.
Legacy IDs, manifests, archives, event history, credential metadata, and storage schema version `1` remain readable.
Manifest replacement and audit append remain separate writes, so durability is bounded rather than crash-atomic across both files.

## Removed commands

Launch, attach, stop, deployment, cleanup, credential, integration, provider orchestration, and `task record` commands return structured usage errors with exit code `2` before store access or mutation.
Migration guidance names the command family and points callers to durable record operations and external tools or skills.
It never echoes sensitive input.

## Next public work

See [Roadmap](roadmap.md) for the authoritative prioritized follow-up work.

## Verification and compatibility gate

Run:

```bash
go test ./...
go test -race ./...
go vet ./...
git diff --check
```
Do not install the binary until the integrated CLI, lifecycle skill, repository instructions, and active-agent workflow are verified against `main`.
