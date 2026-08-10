# Design index

## Purpose

`akagent` is a local-first orchestration protocol and CLI for coding agents.
It preserves direct tmux and Git worktree operation while adding stable identity, structured state, recovery, optional integrations, and eventual remote execution.

The repository is `github.com/akofink/akagent-cli`.
The installed binary is `akagent`.
`aka` is an optional interactive shell alias and is not a second protocol entry point.

## Documents

- [`architecture.md`](architecture.md) defines components, ownership, persistence, integrations, discovery, and explicit non-goals.
- [`protocol.md`](protocol.md) defines worker and task resources, state, lifecycle operations, TOON output, errors, idempotency, and reconciliation.
- [`credentials.md`](credentials.md) defines credential sources, requirements, transfer, installation, lifetime, redaction, and revocation.
- [`technology.md`](technology.md) compares Go, Rust, TypeScript, Python, and shell.
- [`roadmap.md`](roadmap.md) separates the local proof from hooks, remote EC2 support, and later discovery.
- [`implementation-plan.md`](implementation-plan.md) maps GitHub issues, parallel ownership, delivery conventions, and integration order.
- [`handoff.md`](handoff.md) records current implementation state and the next tasks.

## Current decisions

1. Use one executable with operator commands and directly invocable `akagent worker ...` commands.
2. Keep the CLI as the permanent boundary for humans, agents, shell helpers, native hooks, plugins, skills, and remote transports.
3. Prove task lifecycle, tmux integration, worktrees, status, reconciliation, and cleanup locally before adding remote transport.
4. Use explicitly selected EC2 workers for the first off-machine tests.
5. Defer containers, schedulers, automatic placement, and a central service.
6. Use TOON for agent-consumed stdout and treat token use as an interface constraint.
7. Keep worker-local durable task records and derive status through reconciliation.
8. Keep infrastructure provisioning separate from task orchestration.
9. Keep application source and releases in this repository while using dots or another provisioner to install the binary.
10. Add local cross-worker discovery and a stale-aware read cache only after basic remote execution works.
11. Source credentials from the machine invoking `akagent`, validate task requirements, and propagate only explicitly selected capabilities.
12. Use the existing passphrase-free signing subkey as a scoped bearer credential while never distributing the primary secret key.

## Design constraints

- Preserve ordinary shell and tmux recovery paths.
- Preserve repository-specific branch and worktree policies.
- Make repeated mutations idempotent.
- Keep tasks operable when hooks, operator processes, or network connections fail.
- Do not infer completion from terminal output alone.
- Do not trust agent-declared state without checking process, tmux, filesystem, and Git facts.
- Avoid fields and ambient context that do not change the next agent decision.
- Expose worker capability and persistence differences instead of claiming false backend transparency.
- Never expose credential values in commands, output, logs, task records, or tmux metadata.

## Rejected initial approaches

### Full scheduler first

A queue, placement engine, central database, and worker leases would add distributed failure modes before workload measurements justify them.
Explicit local execution and named workers provide enough evidence for the first stages.

### Containers first

Containers improve dependency and resource isolation but complicate interactive attachment, Git worktrees, ownership, browser access, caches, and credentials.
They remain a distant option rather than an initial protocol requirement.

### Tmux as the database

Tmux is an excellent human interaction and recovery surface.
Window names, process inspection, and scrollback are not sufficient durable orchestration state.

### One opaque secret bundle

Copying every credential to every worker creates unnecessary exposure and makes rotation and cleanup ambiguous.
The credential model uses named capabilities, source references, explicit requirements, and bounded lifetimes.
