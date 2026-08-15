# akagent

`akagent` is a local-first orchestration protocol and CLI for coding tasks.
It preserves Git worktree and tmux workflows while providing durable task identity, structured state, reconciliation, and recovery.

The installed binary is `akagent`.
`aka` may be configured as an optional shell alias, but it is not a second binary or protocol entry point.

## Status

The repository provides the protocol foundation and the initial local task lifecycle.
Task manifests and append-only events are durable state.
Tmux is an observed interaction surface and a human attachment surface.

Current commands:

```text
akagent
akagent credential <list|inspect|doctor>
akagent integration inspect
akagent id generate
akagent repository <register|list|inspect|update|unregister>
akagent task <start|list|inspect|attach|publish|finish|stop|archive|clean|reconcile>
akagent update [--source <path>]
akagent worker inspect
```

Stdout carries TOON protocol data and structured errors.
The TOON output contract is pinned to specification version 4.1 with a validated encoder and official conformance fixtures.
See [`docs/toon.md`](docs/toon.md).

## Quick start

See [`docs/quick-start.md`](docs/quick-start.md) for public installation, update, integration-gate, repository, task, attachment, reconciliation, archive, cleanup, and recovery examples.

A minimal local flow is:

```bash
akagent integration inspect
akagent repository register demo /path/to/checkout
akagent task start --title "Review the build" --repository demo
akagent task list
akagent task inspect <task-id>
```

Registration requires the root of an existing Git checkout.
The default repository policy creates an isolated branch and Git worktree for each task.
The `direct` policy permits the registered checkout itself when that is explicitly selected.

`task start` creates a durable record, validates the repository inputs, creates the required branch and worktree, and starts a task-tagged tmux resource.
The local CLI can start either a detached shell for direct human work or a managed local Pi process.
Use `--agent pi` to select the managed launch, `--prompt` to provide a local prompt file on standard input, and `--context` to provide one non-secret working-context value.
Use `task attach` for verified human attachment and use `task publish` for durable condition and heartbeat updates.

`task reconcile` compares durable records with tmux and Git observations and repairs safe derived facts.
It never deletes task state, branches, worktrees, windows, or terminal history.

## Managed Pi launch

Pi must be installed and available as `pi` on `PATH`.
The managed launch configuration resolves that command, the task worktree, an optional prompt-file reference, and an optional non-secret context before starting tmux.
The prompt file is opened as standard input for Pi and its content is never placed in process arguments, task events, or protocol output.

```bash
akagent task start --title "Review the build" --repository demo \
  --agent pi --prompt /path/to/prompt.txt --context "example"
```

The start response includes the selected agent, resolved command, prompt reference, and working context.
A repeated start with the same immutable inputs is an idempotent no-op, while a failed launch remains retryable through the same task start command.
The managed process receives a minimal safe environment, `AKAGENT_TASK_ID`, and the requested environment credentials that passed readiness checks.
Optional credentials produce non-secret warnings and are not injected; file credentials are readiness-only and cannot be injected into the managed environment.

`task archive` captures a stopped or finished task's manifest, events, non-secret Git facts, and available terminal history.
`task clean` archives first, refuses live tasks, preserves committed, dirty, and untracked work unless each category is explicitly authorized, and records independent cleanup debt.
The default local cleanup hooks do not delete worktrees or credentials.

## Integration gate

Automated integrations are disabled unless `AKAGENT_ENABLED=1` is present in the invoking environment.
A missing value or any value other than `1` is disabled.

```bash
akagent integration inspect
export AKAGENT_ENABLED=1
unset AKAGENT_ENABLED
```

`integration inspect` is read-only.
Direct human CLI commands remain available regardless of the gate.

## Credentials

`akagent` discovers local credentials from a versioned, non-secret manifest under the user's XDG configuration directory.
The manifest holds source references and policy, never secret values.
`akagent` never reads or prints credential values for these inspection commands.

```toon
version: 1
credentials[3]{id,type,source,required_for}:
  git-ssh,ssh_key,file:<path>,git
  github,api_token,env:GITHUB_TOKEN,github
  llm,api_token,env:OPENAI_API_KEY,
```

Each row names a credential ID, non-secret type, source reference, and required capability.
A blank capability marks an optional credential.
Missing required credentials block a task that requests them.
Missing optional credentials produce non-secret warnings.

Supported source kinds are `file:<path>` and `env:<VAR>`.
File readiness is checked from metadata without opening the file.
Environment readiness checks presence without printing the value.

Commands:

```text
akagent credential list
akagent credential inspect <id>
akagent credential doctor
```

Set `AKAGENT_CREDENTIALS` to use a non-default manifest path.
See [`docs/credentials.md`](docs/credentials.md) for the credential model and current limitations.

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

## Direction

Near-term work focuses on the local protocol, managed local Pi launch, and opt-in workflow integrations.
The current CLI remains local-first: it uses the registered checkout, Git worktrees, and tmux on the invoking machine.

## Design documentation

- [`docs/quick-start.md`](docs/quick-start.md) provides the public agent-safe setup and recovery path.
- [`docs/README.md`](docs/README.md) indexes the design and current decisions.
- [`docs/storage.md`](docs/storage.md) defines the worker-local state store layout, schema, permissions, locking, archive, and recovery.
- [`docs/architecture.md`](docs/architecture.md) defines system boundaries and failure assumptions.
- [`docs/protocol.md`](docs/protocol.md) defines resources, state, lifecycle operations, output, and compatibility.
- [`docs/task-cli.md`](docs/task-cli.md) defines the supported repository and task command syntax, output schemas, errors, and exit codes.
- [`docs/credentials.md`](docs/credentials.md) defines credential discovery, validation, and managed-launch environment behavior.
- [`docs/integration-gate.md`](docs/integration-gate.md) defines the default-disabled integration signal.
- [`docs/technology.md`](docs/technology.md) records the implementation-stack evaluation.
- [`docs/roadmap.md`](docs/roadmap.md) stages completed local work and remaining local integration work.
- [`docs/handoff.md`](docs/handoff.md) records current implementation status and limitations.
