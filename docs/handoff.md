# Current handoff

## Goal

`akagent` is a local-first durable registry protocol and CLI for coding agents.
The core records task, repository, resource, execution, observation, checkpoint, disposition, archive, recovery, and delivery facts.
External tools and skills own process, tmux, Git, worktree, credential, provider, cleanup, deployment, and forge side effects.

## Shipped behavior

- UUIDv7 task IDs and structured TOON protocol output.
- Worker protocol version `2` with declarative `registry`, `checkpoint`, and `observation` capabilities.
- Storage schema version `1` compatibility and secure local manifests, events, checkpoints, archives, and locks.
- Record-only repository, task, resource, and execution operations.
- Caller-declared branch, revision, head, worktree, observation, session, and delivery metadata.
- Provider-neutral session references and metadata-only evidence views.
- Record-only conditions, finish results, archives, reconciliation, revision-scoped dispositions, and bounded checkpoints.
- Deterministic in-flight, attention, maintenance, deferred, and history inventory views.
- Pre-store structured refusal for removed orchestration, deployment, cleanup, credential, and transitional record commands.
- Subprocess canaries proving retained commands do not invoke Git, tmux, Pi, providers, or deployment tools.
- Read-only `AKAGENT_ENABLED` integration inspection.
- Public quick-start, protocol, architecture, migration, agent guidance, and lifecycle skill documentation.

## Current workflow

Adopt an existing task with `AKAGENT_TASK_ID` and inspect it before editing.
Otherwise create task intent and caller-declared resources directly through the CLI.
Create an execution only when an external tool needs explicit identity.
Publish conditions and activity as durable records.
Record provider session references and delivery URLs without provider content.
After an uncertain failure, inspect and reconcile before retrying.
Finish explicitly against the caller's completion contract, then archive.

The core never launches or stops a process, inspects tmux or PIDs, runs Git, mutates a worktree, resolves credential values, reads a provider file, captures terminal output, or calls a forge.
A missing process, checkout, provider session, credential, or network never proves completion.

## Recovery and migration

Reconciliation is store-only and offline-safe.
It preserves missing, stale, unavailable, and contradictory observations.
It never creates duplicate tasks, resources, executions, branches, or worktrees.
Legacy unfinished and stopped work requires explicit store-only adoption or migration.
Legacy IDs, manifests, archives, event history, credential metadata, and storage schema version `1` remain readable.
Manifest replacement and audit append remain separate writes, so durability is bounded rather than crash-atomic across both files.

## Removed commands

Launch, attach, stop, deployment, cleanup, credential, provider orchestration, and `task record` commands return structured usage errors with exit code `2` before store access or mutation.
Migration guidance names the command family and points callers to durable record operations and external tools or skills.
It never echoes sensitive input.

## Verification and compatibility gate

Run:

```bash
env GOROOT=/usr/local/go go test ./...
env GOROOT=/usr/local/go go test -race ./...
env GOROOT=/usr/local/go go vet ./...
git diff --check
```

The local Go installation may require `GOROOT=/usr/local/go` when the selected tool reports a standard-library version mismatch.
Do not install the binary until the integrated CLI, lifecycle skill, repository instructions, and active-agent workflow are verified against `main`.
