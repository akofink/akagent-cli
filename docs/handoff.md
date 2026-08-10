# Current handoff

## Goal

Build a local-first agent orchestration protocol that preserves tmux and Git worktree ergonomics and later supports named remote workers.

## Repository and installation

- Source repository: `github.com/akofink/akagent-cli`.
- Binary: `akagent`.
- Optional interactive alias: `aka`.
- Go is the selected implementation language.
- Machine provisioning may install the Go command into `~/.local/bin`.
- Public worker bootstrap must not require private operator context.

## Implemented

- `akagent` compact home view.
- `akagent id generate` using UUIDv7.
- `akagent worker inspect` with local capabilities.
- TOON 4.1 output through a narrow, conformance-tested output package.
- Structured usage errors and shell exit codes.
- Unit tests, vet, and GitHub Actions CI.
- Source-managed `akagent update` with clean-main validation, fast-forward-only Git updates, and atomic binary replacement.
- A secure worker-local state store for versioned manifests, append-only events, atomic replacement, locking, and recovery.
- A local credential manifest with `file:` and `env:` readiness checks plus `credential list`, `inspect`, and `doctor`.

## Design evidence from the remote-workstation prototype

- A conventional remote Linux workstation can reproduce the local shell, tmux, Git, runtime, and LLM-tool workflow.
- Bootstrap portability across distributions and architectures requires explicit package mappings, disk and temporary-directory policy, noninteractive behavior, and complete installation checks.
- Small workers may run finished tools but are poor places to compile large language runtimes and dependencies.
- Private workers generally do not need public ingress or public IPv4 addresses.
- Host telemetry alone cannot answer task state, waiting reason, cleanup debt, credential identity, or artifact recovery questions.
- Tmux remains the best human interaction surface but cannot be the sole source of orchestration truth.
- Git commits and structured records are better default checkpoints than machine snapshots.

## Current credential direction

- The operator-side invocation resolves credential sources.
- A non-secret manifest lives under XDG configuration.
- Dedicated secret files, when needed, live under XDG data with strict permissions.
- Tasks request named capabilities rather than secret paths.
- Missing optional credentials warn; missing required credentials block startup.
- Dedicated agent SSH, GitHub, and LLM credentials are preferred over copied primary human credentials.
- Git signing uses the existing signing subkey exported without its parent secret key or passphrase and treated as a scoped bearer credential.
- Remote transfer, refresh, and cleanup begin with remote execution, not before.

## Phase 0 completion

Parent issue [#1](https://github.com/akofink/akagent-cli/issues/1) tracked the three parallel protocol foundations:

1. [#2](https://github.com/akofink/akagent-cli/issues/2) pinned the TOON 4.1 output contract.
2. [#3](https://github.com/akofink/akagent-cli/issues/3) implemented the secure worker-local state store.
3. [#4](https://github.com/akofink/akagent-cli/issues/4) added the local credential manifest and doctor commands.

All three child issues are merged into `main`.
The detailed ownership, delivery, integration, and future-wave record is in [`implementation-plan.md`](implementation-plan.md).

The foundation is now ready for local task lifecycle work.
Tmux and Git worktree mutation must build on the store, credential, and output boundaries rather than bypass them.

## Next implementation slice

Issue [#8](https://github.com/akofink/akagent-cli/issues/8) is the refined next task: implement local task lifecycle commands.
It should register repositories and policy, create and inspect task manifests, manage tmux-backed local tasks, publish durable state, reconcile observations, and preserve uncommitted work and credential cleanup debt.
The implementation must use `internal/store` for durable records and locks, `internal/credential` for named capability readiness, and `internal/output` for pinned TOON and structured errors.

## Open decisions

- The exact task and event schema fields.
- The initial repository-registration format and policy discovery rules.
- The first remote transport after local lifecycle completion.
- Whether GitHub access begins with a fine-grained token or a GitHub App installation.

The durable store encoding is JSON.
TOON remains the agent-facing output and interchange encoding until a separate durable-encoding decision is justified.

## Explicitly deferred

- Containers.
- Scheduling and automatic worker placement.
- A central dashboard or service.
- Work-specific credentials.
- Browser session propagation.
