# akagent documentation

`akagent` is a local-first durable registry protocol and CLI for coding agents.
An agent invokes the CLI directly during ordinary coding work to create task state, update durable status, and record observations, recovery, and delivery facts.
Git worktrees and tmux are external interaction surfaces.
The CLI is the durable source of truth.

Use the [quick start](quick-start.md) for installation and the record-only task lifecycle.
Use the [agent integration guide](agent-integration.md) for progressive disclosure, a generic `AGENTS.md` template, and a reusable lifecycle skill.
This page indexes the shipped record-only boundary.

The installed binary is `akagent`.
`aka` is an optional interactive shell alias and is not a second protocol entry point.

## Public starting point

- [`quick-start.md`](quick-start.md) provides the agent-safe installation, repository, task, resource, execution, status, delivery, reconciliation, archive, and recovery path.
- [`record-only-lifecycle.md`](record-only-lifecycle.md) defines managed and caller-owned external record commands.
- [`agent-integration.md`](agent-integration.md) progressively introduces the generic `AGENTS.md` template and record-only lifecycle skill.
- [`AGENTS.md`](AGENTS.md) provides concise repository guidance for adopting or self-bootstrapping an `akagent` task.
- [`skills/akagent-lifecycle/SKILL.md`](skills/akagent-lifecycle/SKILL.md) provides reusable lifecycle instructions for coding agents.

## Normal agent workflow

The coding agent owns its durable local records.
It uses `akagent` as a self-service protocol instead of handing state to a parent orchestrator.
The CLI records facts only.
External tools own process, tmux, Git, worktree, credential, provider, cleanup, and forge side effects.

1. Create durable task intent, then record caller-declared repository, branch, and worktree facts.
2. Create or select resources and executions from the task context.
3. Publish conditions, activity, heartbeats, and recovery reasons as work changes.
4. Record provider-neutral session provenance and delivery metadata when they become available.
5. Reconcile observations after disconnects, failures, or terminal changes.
6. Finish and archive only after the caller's completion contract is independently verified.

Tmux may make active work visible, but it is not durable state and the core does not attach to it.
`akagent task inspect` and `akagent task reconcile` remain the durable source of truth when a terminal, provider session, or parent process disappears.

## Documents

- [`architecture.md`](architecture.md) defines current components, ownership, persistence, recovery, and explicit non-goals.
- [`charter.md`](charter.md) defines the shipped durable registry boundary and compatibility rules.
- [`protocol.md`](protocol.md) defines worker and task resources, state, lifecycle, TOON output, errors, compatibility, and reconciliation.
- [`record-only-lifecycle.md`](record-only-lifecycle.md) defines managed and explicit external record contracts.
- [`task-cli.md`](task-cli.md) defines the supported repository and task command syntax, output schemas, errors, and exit codes.
- [`work-inventory.md`](work-inventory.md) defines disposition and the in-flight, attention, maintenance, deferred, and history views.
- [`credentials.md`](credentials.md) defines historical credential metadata and the external ownership boundary.
- [`technology.md`](technology.md) compares the implementation options.
- [`roadmap.md`](roadmap.md) separates shipped local work from prioritized follow-ups.
- [`implementation-plan.md`](implementation-plan.md) records the shipped delivery map and remaining work.
- [`storage.md`](storage.md) defines the worker-local state store layout, schema, permissions, locking, archive, and recovery.
- [`recovery-checkpoints.md`](recovery-checkpoints.md) defines the revision-checked, state-only recovery checkpoint contract and reboot-equivalent drill.
- [`session-retrospection.md`](session-retrospection.md) defines provider-neutral session evidence, shipped Phase 0 views, and later adapter phases.
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
3. Make task, resource, and execution identity, observations, checkpoints, events, archive history, and recovery self-service, durable, inspectable, and recoverable.
4. Use tmux only as an external visibility surface, never as the durable source of truth.
5. Keep one implicit local worker and caller-declared Git facts without core checkout mutation.
6. Keep process, Git, worktree, credential, provider, cleanup, deployment, and forge side effects outside the core.
7. Do not recreate retired launch, attach, stop, credential, or deployment command families in the core.
8. Use TOON for agent-consumed stdout and treat token use as an interface constraint.
9. Keep worker-local durable records and preserve uncertain observations rather than inferring completion.
10. Keep application source and releases in this repository while allowing an external installer to install the binary.
11. Preserve historical credential metadata without resolving or printing values.

## Design constraints

- Keep the CLI useful offline and when external tools are unavailable.
- Make repeated record mutations idempotent.
- Preserve legacy manifests, archives, events, IDs, and storage schema version `1`.
- Preserve unknown, stale, unavailable, and contradictory observations.
- Never infer completion from process absence, terminal output, archive state, or age.
- Keep provider-neutral references and delivery metadata separate from provider content.
- Never expose credential values in commands, output, logs, task records, or diagnostics.
- Keep side effects outside the core and do not require a daemon or feature-parity adapter.

## Current local boundary

The CLI records repository, task, resource, execution, observation, checkpoint, disposition, archive, recovery, and delivery facts.
Repository registration and resource creation record caller-declared paths and Git facts without checking or mutating a checkout.
Execution creation records a tool-neutral attempt without starting a process.
The explicit `task external` family creates caller-owned records with stable operation IDs and never relabels managed records.

The core does not launch or stop processes, inspect tmux or PIDs, run Git, create or remove worktrees, resolve credentials, parse provider sessions, capture terminal output, or call a forge.
External tools and skills own those side effects when a workflow still needs them.
Provider session references and evidence views are metadata-only.

The default inventory is the in-flight view, with explicit attention, maintenance, deferred, and history views.
Removed orchestration, credential, and integration commands return structured usage errors before store access.
Worker protocol version `2` reports declarative capabilities, while storage schema version `1` remains readable.

## Rejected prerequisites and target boundary

### Tmux as the database

Tmux is a useful human visibility surface.
Window names, process inspection, and scrollback are not sufficient durable state.
The CLI records the task, resource, execution, session, delivery, Git, and reconciliation facts that must survive a lost terminal or process.

### A daemon or launch adapter as a prerequisite

A local coding agent should not need a resident daemon, remote scheduler, or launch adapter to use the protocol.
The agent calls `akagent` directly, and optional integrations remain replaceable callers of the same CLI boundary.
The target does not remove optional adapters from the ecosystem.
It removes side-effect authority from the core and does not require an adapter for every retired capability.

### One opaque secret bundle

Copying every credential to every worker creates unnecessary exposure and makes rotation and cleanup ambiguous.
Historical credential IDs and source references may remain in readable records.
External tools own readiness, injection, rotation, and cleanup.
The core never resolves or prints credential values.
