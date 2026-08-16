# akagent documentation

`akagent` is a local-first orchestration protocol and CLI for coding tasks.
It preserves direct Git worktree and tmux operation while adding stable identity, structured state, recovery, and optional integrations.

Use the [quick start](quick-start.md) for installation and the public task lifecycle.
This page indexes the design documentation and current decisions.

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

1. Use one executable with direct human commands and a directly inspectable local worker.
2. Keep the CLI as the permanent boundary for humans, agents, integrations, plugins, and skills.
3. Prove local task lifecycle, tmux integration, Git worktrees, generic executions, optional Pi integration, status, reconciliation, archive, and safe cleanup.
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
- Keep automated integrations enabled by default, with `AKAGENT_ENABLED=0` as the immediate disable signal.
- Never expose credential values in commands, output, logs, task records, or tmux metadata.

## Current local boundary

The current CLI registers local Git repositories, records state-only task intent, creates independently recoverable resources and isolated worktrees under the `worktree` policy, and keeps resource creation separate from execution.

A task can use an explicit detached shell execution for direct work or the optional Pi integration selected with `--target pi`.
Both paths use the generic execution primitives, while task and resource creation remain independent of Pi availability.
One execution can coordinate multiple task resources by selecting a resource worktree.

It supports inspection, durable condition publication, safe verified attachment, stop, finish, reconciliation, archive, and cleanup-state tracking.
Worktree cleanup requires explicit approval and validates durable ownership before removal.
Credential cleanup is an independent approval-gated hook with durable retry state.

## Rejected initial approaches

### Tmux as the database

Tmux is an excellent human interaction and recovery surface.
Window names, process inspection, and scrollback are not sufficient durable orchestration state.

### One opaque secret bundle

Copying every credential to every worker creates unnecessary exposure and makes rotation and cleanup ambiguous.
The credential model uses named capabilities, source references, explicit requirements, and bounded lifetimes.
