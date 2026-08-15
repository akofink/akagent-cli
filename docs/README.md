# Design index

`akagent` is a local-first orchestration protocol and CLI for coding tasks.
It preserves direct Git worktree and tmux operation while adding stable identity, structured state, recovery, and optional integrations.

The installed binary is `akagent`.
`aka` is an optional interactive shell alias and is not a second protocol entry point.

## Public starting point

- [`quick-start.md`](quick-start.md) provides the agent-safe installation, integration-gate, repository, task, attachment, reconciliation, archive, cleanup, and recovery path.

## Documents

- [`architecture.md`](architecture.md) defines current components, ownership, persistence, integrations, discovery, and explicit non-goals.
- [`protocol.md`](protocol.md) defines worker and task resources, state, lifecycle, TOON output, errors, compatibility, and reconciliation.
- [`task-cli.md`](task-cli.md) defines the supported repository and task command syntax, output schemas, errors, and exit codes.
- [`credentials.md`](credentials.md) defines local credential sources, requirements, validation, and current limitations.
- [`integration-gate.md`](integration-gate.md) defines the integration signal and immediate disable path.
- [`technology.md`](technology.md) compares the implementation options.
- [`roadmap.md`](roadmap.md) separates shipped local work from tracked follow-ups.
- [`implementation-plan.md`](implementation-plan.md) records the public issue and delivery map.
- [`storage.md`](storage.md) defines the worker-local state store layout, schema, permissions, locking, archive, and recovery.
- [`handoff.md`](handoff.md) records current implementation status and the next public work.

## Current decisions

1. Use one executable with direct human commands and a directly inspectable local worker.
2. Keep the CLI as the permanent boundary for humans, agents, integrations, plugins, and skills.
3. Prove local task lifecycle, tmux integration, Git worktrees, managed Pi launch, status, reconciliation, archive, and safe cleanup.
4. Use one implicit local worker.
5. Keep infrastructure provisioning separate from task orchestration.
6. Use TOON for agent-consumed stdout and treat token use as an interface constraint.
7. Keep worker-local durable task records and derive status from reconciled observations.
8. Keep application source and releases in this repository while allowing an external installer to install the binary.
9. Source credentials locally, validate named requirements, and never expose credential values.

## Design constraints

- Preserve ordinary shell, tmux, and Git recovery paths.
- Preserve repository-specific branch and worktree policies.
- Make repeated mutations idempotent.
- Keep tasks operable when integrations, operator processes, or network connections fail.
- Do not infer completion from terminal output alone.
- Do not trust a declared condition without checking process, tmux, filesystem, and Git facts where required.
- Avoid fields and ambient context that do not change the next agent decision.
- Expose worker capability and persistence differences instead of claiming false backend transparency.
- Keep automated integrations disabled by default.
- Never expose credential values in commands, output, logs, task records, or tmux metadata.

## Current local boundary

The current CLI registers local Git repositories, creates durable task records, creates isolated worktrees under the `worktree` policy, and starts task-tagged tmux resources.

A task can use a detached shell for direct work or a managed local Pi launch selected with `--agent pi`.
Managed launch persists the resolved command, task worktree, optional prompt-file reference, and optional non-secret working context before tmux starts.

It supports inspection, durable condition publication, safe verified attachment, stop, finish, reconciliation, archive, and cleanup-state tracking.
The default cleanup hooks do not delete worktrees or credentials.

## Rejected initial approaches

### Tmux as the database

Tmux is an excellent human interaction and recovery surface.
Window names, process inspection, and scrollback are not sufficient durable orchestration state.

### One opaque secret bundle

Copying every credential to every worker creates unnecessary exposure and makes rotation and cleanup ambiguous.
The credential model uses named capabilities, source references, explicit requirements, and bounded lifetimes.
