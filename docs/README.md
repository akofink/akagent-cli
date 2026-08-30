# akagent documentation

`akagent` is a local-first orchestration protocol and CLI for coding agents.
An agent invokes the CLI directly during ordinary coding work to create task state, update durable status, and record recovery and delivery facts.
Git worktrees remain the implementation boundary, while tmux provides visibility and recovery rather than durable state.

Use the [quick start](quick-start.md) for installation and the self-service task lifecycle.
Use the [agent integration guide](agent-integration.md) for progressive disclosure, a generic `AGENTS.md` template, and a reusable lifecycle skill.
This page is the project charter and indexes the design documentation and current decisions.

The installed binary is `akagent`.
`aka` is an optional interactive shell alias and is not a second protocol entry point.

## Public starting point

- [`quick-start.md`](quick-start.md) provides the agent-safe installation, repository, task, resource, execution, status, delivery, reconciliation, archive, cleanup, and recovery path.
- [`agent-integration.md`](agent-integration.md) progressively introduces the generic `AGENTS.md` template and lifecycle skill.
- [`AGENTS.md`](AGENTS.md) provides concise repository guidance for adopting or self-bootstrapping an `akagent` task.
- [`skills/akagent-lifecycle/SKILL.md`](skills/akagent-lifecycle/SKILL.md) provides reusable lifecycle instructions for coding agents.

## Normal agent workflow

The coding agent is the owner of its local lifecycle.
It uses `akagent` as a self-service protocol instead of handing state to a parent orchestrator.

1. Register the checkout once and create a durable task with the requested branch and worktree facts.
2. Create or select resources and executions from the task context.
3. Publish conditions, activity, heartbeats, and recovery reasons as work changes.
4. Record provider-neutral session provenance and pull-request metadata when they become available.
5. Reconcile observations after disconnects, failures, or terminal changes.
6. Finish, archive, and clean only after reviewing the durable Git and recovery facts.

Tmux windows and process inspection make active work visible and attachable.
`akagent task inspect` and `akagent task reconcile` remain the durable source of truth when a terminal, provider session, or parent process disappears.

## Documents

- [`architecture.md`](architecture.md) defines current components, ownership, persistence, integrations, discovery, and explicit non-goals.
- [`charter.md`](charter.md) proposes a narrower durable protocol boundary and an incremental migration away from core orchestration.
- [`protocol.md`](protocol.md) defines worker and task resources, state, lifecycle, TOON output, errors, compatibility, and reconciliation.
- [`task-cli.md`](task-cli.md) defines the supported repository and task command syntax, output schemas, errors, and exit codes.
- [`credentials.md`](credentials.md) defines local credential sources, requirements, validation, and current limitations.
- [`integration-gate.md`](integration-gate.md) defines the integration signal and immediate disable path.
- [`technology.md`](technology.md) compares the implementation options.
- [`roadmap.md`](roadmap.md) separates shipped local work from tracked follow-ups.
- [`implementation-plan.md`](implementation-plan.md) records the public issue and delivery map.
- [`storage.md`](storage.md) defines the worker-local state store layout, schema, permissions, locking, archive, and recovery.
- [`handoff.md`](handoff.md) records current implementation status and the next public work.

## Preview the documentation site

The site is built with [MkDocs](https://www.mkdocs.org/) from the existing Markdown files in this directory.
Install MkDocs in a virtual environment, then run the following commands from the repository root:

```bash
python3 -m venv .venv-docs
. .venv-docs/bin/activate
python -m pip install mkdocs==1.6.1
mkdocs serve
```

Open the local URL printed by `mkdocs serve` to preview changes.
Build the same site artifact used by CI with:

```bash
mkdocs build --strict --site-dir _site
```

The generated `_site/` directory is disposable and should not be committed.

## Current decisions

1. Use one executable that coding agents invoke directly during ordinary work.
2. Keep the CLI as the permanent boundary for humans, agents, skills, and optional provider integrations.
3. Make task, resource, and execution lifecycle self-service, durable, inspectable, and recoverable.
4. Use tmux for interactive visibility and recovery, never as the durable source of truth.
5. Keep one implicit local worker and local Git worktree boundaries.
6. Keep infrastructure provisioning, launch adapters, and daemon processes outside the protocol.
7. Use TOON for agent-consumed stdout and treat token use as an interface constraint.
8. Keep worker-local durable task records and derive status from reconciled observations.
9. Keep application source and releases in this repository while allowing an external installer to install the binary.
10. Source credentials locally, validate named requirements, and never expose credential values.

## Design constraints

- Preserve ordinary shell, tmux, and Git recovery paths.
- Preserve repository-specific branch and worktree policies.
- Make repeated mutations idempotent.
- Keep tasks operable when integrations, operator processes, or network connections fail.
- Do not infer completion from terminal output alone.
- Do not trust a declared condition without checking process, tmux, filesystem, and Git facts where required.
- Avoid fields and ambient context that do not change the next agent decision.
- Expose worker capability and persistence differences instead of claiming false backend transparency.
- Keep optional integrations replaceable and never require a launch adapter or daemon for the normal local workflow.
- Never expose credential values in commands, output, logs, task records, or tmux metadata.

## Current local boundary

The current CLI registers local Git repositories, records state-only task intent, creates independently recoverable resources and isolated worktrees under the `worktree` policy, and keeps resource creation separate from execution.

A task can use an explicit detached shell execution for direct work or the optional Pi integration selected with `--target pi`.
Both paths use the generic execution primitives, while task and resource creation remain independent of Pi availability.
One execution can coordinate multiple task resources by selecting a resource worktree.
No launch adapter or daemon is required to use these primitives.

It supports inspection, durable condition publication, safe verified attachment, stop, finish, reconciliation, archive, and cleanup-state tracking.
Worktree cleanup requires explicit approval and validates durable ownership before removal.
Credential cleanup is an independent approval-gated hook with durable retry state.

## Rejected initial approaches

### Tmux as the database

Tmux is an excellent human visibility and recovery surface.
Window names, process inspection, and scrollback are not sufficient durable orchestration state.
The CLI records the task, resource, execution, session, delivery, Git, and reconciliation facts that must survive a lost terminal or process.

### A daemon or launch adapter as a prerequisite

A local coding agent should not need a resident daemon, remote scheduler, or launch adapter to use the protocol.
The agent calls `akagent` directly, and optional integrations remain replaceable callers of the same CLI boundary.

### One opaque secret bundle

Copying every credential to every worker creates unnecessary exposure and makes rotation and cleanup ambiguous.
The credential model uses named capabilities, source references, explicit requirements, and bounded lifetimes.
