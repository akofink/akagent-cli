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
- TOON output through a narrow output package.
- Structured usage errors and shell exit codes.
- Unit tests, vet, and GitHub Actions CI.

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
- Dedicated agent SSH, GPG signing, GitHub, and LLM credentials are preferred over copied primary human credentials.
- Remote transfer, refresh, and cleanup begin with remote execution, not before.

## Next implementation slice

1. Select and pin an exact TOON specification version.
2. Evaluate the current Go encoder against official conformance cases and representative token measurements.
3. Define typed task, event, repository-policy, and credential-manifest schemas.
4. Implement the worker-local state root with secure directory creation, atomic manifest replacement, and per-task locking.
5. Implement `credential doctor` for `file:` and `env:` references without reading or printing values unnecessarily.
6. Add tests for concurrent inspection, partial writes, unsafe permissions, missing credentials, and output redaction.
7. Define and test the managed-process environment allowlist before launching an agent.

Tmux task mutation should follow these storage and credential foundations rather than precede them.

## Open decisions

- Whether TOON is suitable for durable manifests or only output and interchange.
- The exact task and event schema fields.
- The initial repository-registration format and policy discovery rules.
- The first remote transport after local lifecycle completion.
- The operational mechanism for unlocking a dedicated unattended GPG signing subkey.
- Whether GitHub access begins with a fine-grained token or a GitHub App installation.

## Explicitly deferred

- Containers.
- Scheduling and automatic worker placement.
- A central dashboard or service.
- Work-specific credentials.
- Browser session propagation.
