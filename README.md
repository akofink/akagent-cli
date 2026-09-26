# akagent

`akagent` is a local-first durable registry protocol and CLI for coding agents.
Agents invoke it directly during ordinary coding work to preserve durable task identity, structured observations, reconciliation, and recovery around local work.
External tools, including tmux, Git, and providers, remain interaction surfaces outside the core, while the `akagent` CLI is the durable source of truth.

The installed binary is `akagent`.
`aka` may be configured as an optional shell alias, but it is not a second binary or protocol entry point.

## Status

The repository provides the protocol foundation and the initial local task lifecycle.
Task, resource, and execution manifests and append-only events are durable state.
Coding agents create, inspect, update, reconcile, and archive that state themselves through the CLI.
The core is a record-only boundary that accepts typed observations while external tools and skills own side effects that remain needed.
The core does not launch or stop processes, inspect tmux or PIDs, run Git, mutate worktrees, resolve credentials, deploy commands, parse provider sessions, or capture terminal output.
The opt-in `task check` command is the one exception: it reads Git and GitHub through their own CLIs and never writes to the store, a checkout, or the forge.
Optional local or provider adapters are convenience integrations, not required dependencies or feature-parity replacements.

Current commands:

```text
akagent
akagent id generate
akagent repository <register|list|inspect|update|unregister>
akagent task <create|external|checkpoint|resource|execution|disposition|list|inspect|publish|finish|archive|reconcile>
akagent task external <create|finish|resource|execution|archive>
akagent task resource <create|list|inspect|update|archive>
akagent task execution <create|observe|finish|list|inspect|session|evidence|publish|archive|reconcile>
akagent update [--source <path>]
akagent worker inspect
```

Removed orchestration, credential, and integration command families return structured usage errors with exit code `2` before store access.
Worker inspection reports protocol version `2` and declarative registry, checkpoint, observation, and handoff capabilities.

Stdout carries TOON protocol data and structured errors by default.
Use `akagent task list --format json` or `akagent task inspect <task-id> --format json` for compact JSON of the same typed views.
Use `--format human` for deterministic terminal-oriented views.
JSON does not change the default protocol, and structured errors remain TOON.
The TOON output contract is pinned to specification version 4.1 with a validated encoder and official conformance fixtures.
See [`docs/task-cli.md`](docs/task-cli.md) for the output-format contract and [`docs/toon.md`](docs/toon.md) for the TOON contract.

## Quick start

See [`docs/quick-start.md`](docs/quick-start.md) for public installation, update, repository, task, resource, execution, status, delivery, reconciliation, archive, and recovery examples.

A coding agent can own the normal local flow directly:

```bash
akagent repository register demo /path/to/checkout --worktree-root /path/to/worktrees/demo
akagent task create --title "Review the build" --repository demo \
  --branch akofink/review-build --worktree /path/to/worktree
akagent task execution create <task-id> --execution-id attempt --target external --command /path/to/tool
akagent task inspect <task-id|keyword>
akagent task publish <task-id> --condition active --activity "running tests"
akagent task reconcile <task-id>
```

No parent orchestrator, launch adapter, or daemon is required for the current direct workflow.
The shipped core does not make any of those components a prerequisite.

Repository registration records a repository identity and policy without checking or changing the path.
External tools own checkout validation, branches, and worktrees.
The `--worktree-root` value remains a non-secret reference for callers that use isolated worktrees.

`task create` is state-only task creation: it creates a durable record and records any supplied repository facts without starting tmux, a process, Git, or a worktree.
`task execution create` records an optional tool-neutral managed execution without starting tmux or a process.
The explicit `task external` family creates caller-owned external task, resource, and execution records with stable operation IDs.
External creation does not relabel managed records, and external completion and archive require caller ownership and current revisions.
`task execution session add` records non-secret provider-neutral tool and session provenance without parsing provider files.
`task execution evidence list` and `task execution evidence inspect` provide metadata-only views of those references.
A task can coordinate multiple resources through one execution by selecting a resource during execution creation.
Use `task publish` and `task execution publish` for durable condition and heartbeat updates.

`task reconcile` repairs only durable store artifacts and preserves caller-submitted observations.
`task inspect` is the durable work-state view for resources, executions, activity, results, delivery metadata, and session references.
It accepts an exact task ID or a case-sensitive title or branch keyword when exactly one task matches.
`task list [keyword]` applies the same title and branch matching without requiring uniqueness.
Use `--format json` on `task list` or `task inspect` for compact JSON interchange, and `--format human` when reading task state directly in a terminal.
Agents should call these commands directly when work starts, changes, disconnects, or needs recovery.
They never delete task state, branches, worktrees, windows, or terminal history.

## Optional provider tools

Pi and other providers are external to the core.
They may record a provider-neutral session reference, but the core never launches them or reads their session content.

```bash
akagent task execution session add <task-id> <execution-id> \
  --tool pi --session-id <session-id> --reference-path /path/to/session-record
```

The reference is non-secret metadata.
The core never reads the provider file or resolves credentials.

`task archive` captures durable manifests, events, checkpoint references, and caller-submitted facts without terminal capture or host inspection.
The external archive commands apply the same record-only rule and never inspect processes, Git, worktrees, providers, credentials, deployments, or terminal output.
Worktree and credential cleanup are removed from the core.
External tools own cleanup and must preserve durable recovery debt and historical facts.

## Self-service workflow

The coding agent owns the task, resource, and execution lifecycle through the stable CLI boundary.
The normal workflow is to create durable intent, select or create a resource, record an execution when external work is useful, and publish state as work progresses.

Agents can record and inspect non-secret session provenance and delivery metadata without requiring a provider-specific parent process:

```bash
akagent task execution session add <task-id> <execution-id> \
  --tool example-tool --session-id <session-id> \
  --reference-path /path/to/session-record
akagent task execution evidence list <task-id> <execution-id>
akagent task resource update <task-id> <resource-id> \
  --metadata pull-request=opened \
  --external-url https://forge.example/pull/78
```

After a possibly mutating failure, agents inspect the task and reconcile observations before retrying.
Optional integrations may call the same CLI, but the protocol does not require a launch adapter or daemon.

## Credentials

The core does not provide a credential command family or value-resolution subsystem.
Legacy credential-reference and debt fields remain historical metadata in readable records and are never resolved or erased.
External tools and skills own credential readiness, injection, rotation, and cleanup.
See [`docs/credentials.md`](docs/credentials.md) for the migration boundary.

## Installation and updates

Build the binary from a public source checkout:

```bash
git clone https://github.com/akofink/akagent-cli.git /path/to/akagent-cli
cd /path/to/akagent-cli
go build -o "$HOME/.local/bin/akagent" ./cmd/akagent
```

Update from a clean checkout on `main` with an explicit source path:

```bash
akagent update --source /path/to/akagent-cli
```

The updater fetches `origin`, fast-forwards to `origin/main`, builds from a temporary detached worktree at the selected commit, and atomically replaces the installed executable.
It supports macOS and Linux.

Automatic update on every invocation remains intentionally deferred because ordinary commands should not unexpectedly require network access or mutate source.

## Direction and migration

The accepted direction is now the shipped durable record-only core for task, resource, and execution identity, typed observations, checkpoint references, events, archive history, recovery debt, delivery metadata, and protocol output.
Direct launch, stop, attach, Git/worktree, credential, tmux, provider, and deployment orchestration paths have been removed from the core.
Optional adapters can provide convenience, but migration does not require recreating every deprecated capability.
The migration is complete for the core boundary: record-only adoption, independent inventory views, checkpoints, legacy readability, and orchestration removal are shipped.
Legacy manifests and archives remain readable throughout the migration.
Removed orchestration and credential commands return structured migration guidance rather than retaining compatibility wrappers in core.
The guidance points to direct tools or skills, or to an optional adapter when one exists.
The next priority is integrated CLI, skill, and active-agent compatibility verification before any installed-binary change.
Forge and provider-specific delivery behavior remains outside the core lifecycle and is recorded only through provider-neutral metadata and session references.
A daemon, remote scheduler, or launch adapter is not required for the current workflow or the target core.
Direct tools and skills remain valid replacements, and optional adapters are not a required migration deliverable.

## Design documentation

- [`docs/quick-start.md`](docs/quick-start.md) provides the public agent-safe setup and recovery path.
- [`docs/README.md`](docs/README.md) indexes the design and current decisions.
- [`docs/record-only-lifecycle.md`](docs/record-only-lifecycle.md) defines managed and caller-owned external record contracts.
- [`docs/storage.md`](docs/storage.md) defines the worker-local state store layout, schema, permissions, locking, archive, and recovery.
- [`docs/architecture.md`](docs/architecture.md) defines system boundaries and failure assumptions.
- [`docs/derived-state.md`](docs/derived-state.md) defines derived resource state, the adapter contract, and the `task check` command.
- [`docs/protocol.md`](docs/protocol.md) defines resources, state, lifecycle operations, output, and compatibility.
- [`docs/task-cli.md`](docs/task-cli.md) defines the supported repository and task command syntax, output schemas, errors, and exit codes.
- [`docs/work-inventory.md`](docs/work-inventory.md) defines disposition and inventory views.
- [`docs/credentials.md`](docs/credentials.md) defines the historical credential metadata and external ownership boundary.
- [`docs/technology.md`](docs/technology.md) records the implementation-stack evaluation.
- [`docs/roadmap.md`](docs/roadmap.md) stages shipped local work and prioritized follow-ups.
- [`docs/handoff.md`](docs/handoff.md) records current implementation status and the next public work.
