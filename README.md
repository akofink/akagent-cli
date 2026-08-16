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
akagent task <create|resource|execution|launch|list|inspect|attach|publish|finish|stop|archive|clean|reconcile>
akagent task execution <create|launch|list|inspect|session|publish|attach|stop|archive|reconcile>
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
akagent repository register demo /path/to/checkout --worktree-root /path/to/worktrees/demo
akagent task create --title "Review the build" --repository demo \
  --branch akofink/review-build
akagent task launch <task-id> --target shell
akagent task list
akagent task inspect <task-id>
```

Registration requires the root of an existing Git checkout.
The default repository policy creates an isolated branch and Git worktree for each task.
Use `--worktree-root <absolute-path>` with the `worktree` policy to separate primary clones from managed worktrees.
Without that option, the existing derived root remains `<checkout-parent>/.akagent/worktrees/<name>`.
The `direct` policy permits the registered checkout itself when that is explicitly selected.

`task create` creates a durable record, validates the repository inputs, and creates the required branch and worktree without starting tmux or a process.
`task execution create` records an optional tool-neutral execution without starting tmux, and `task execution launch` starts it with a descriptive display label.
`task execution session add` records non-secret provider-neutral tool and session provenance without parsing provider files.
Managed execution windows publish the shared tmux `@agent_state` option by matching task and execution metadata, not display names.
Active execution clears the option, waiting and blocked publish their values, and completed execution publishes `done`.
`task launch --target shell` is the explicit direct human shell workflow built on those generic execution commands.
Compatibility shell and Pi launches derive their execution and tmux display label from the selected resource or task branch, without the owner prefix.
Pass `--label <descriptive-label>` when no descriptive branch is available.
Execution stop, archive, attachment, and reconciliation are independent from resource state.
A task can coordinate multiple resources through one execution by selecting a resource during execution creation.
Worktree-policy tasks require an explicit descriptive branch such as `akofink/51-task-labels`.
Direct-policy tasks deliberately use the registered checkout's current branch when `--branch` is omitted.
Use `task attach` for verified human attachment and use `task publish` for durable condition and heartbeat updates.

`task reconcile` compares durable records with tmux and Git observations and repairs safe derived facts.
`task inspect` is the durable work-state view for resources, executions, activity, results, delivery metadata, and session references.
It never deletes task state, branches, worktrees, windows, or terminal history.

## Optional Pi integration

Pi is an optional execution integration and must be installed as `pi` on `PATH` only when selected.
Core task, resource, and generic execution creation do not inspect or require Pi.
The `task launch --target pi` target creates a generic execution and delegates process setup to the Pi integration.
The integration passes the owning task ID as `AKAGENT_TASK_ID`, preserves interactive standard input, and keeps prompt content out of durable records and protocol output.
Use the direct shell target when a human wants a shell without any provider integration.

```bash
akagent task create --title "Review the build" --repository demo \
  --branch akofink/review-build
akagent task launch <task-id> --target pi --prompt /path/to/prompt.txt --context "example"
```

A Pi launch failure leaves the generic execution recoverable and does not change resource state.
The managed process receives a minimal safe environment and only requested environment credentials that pass readiness checks.
Optional credentials produce non-secret warnings; file credentials are readiness-only and cannot be injected.

`task archive` captures a stopped or finished task's manifest, events, non-secret Git facts, and available terminal history.
`task clean` archives first, refuses live tasks, preserves committed, dirty, and untracked work unless each category is explicitly authorized, and records independent cleanup debt.
For isolated worktree tasks, `--allow-worktree` is a separate approval for the destructive cleanup hook.
The hook validates ownership, removes only the task worktree, preserves its branch and archive facts, and never removes a direct registered checkout.

## Orchestration workflow

Agent orchestration is enabled by default over the stable `akagent` CLI boundary.
Repository implementation work uses the managed `akagent` lifecycle when the integration gate reports enabled.
The agent skill owns automated lifecycle behavior instead of bypassing the CLI.
After a command that may have mutated state fails, inspect the task and run reconciliation before attempting a manual fallback.
Direct human `akagent` commands remain available regardless of the integration signal.

`AKAGENT_ENABLED` remains the immediate per-environment disable signal for automated integrations.
At the CLI boundary, automation is enabled unless `AKAGENT_ENABLED` is set to the exact value `0`.

```bash
akagent integration inspect
export AKAGENT_ENABLED=0
unset AKAGENT_ENABLED # Re-enable automation
```

`integration inspect` is read-only.

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

The current CLI remains local-first: it uses the registered checkout, Git worktrees, and tmux on the invoking machine.
The tracked follow-ups are broader workflow integrations beyond the stable CLI boundary and work-specific secrets or deployment behavior.

## Design documentation

- [`docs/quick-start.md`](docs/quick-start.md) provides the public agent-safe setup and recovery path.
- [`docs/README.md`](docs/README.md) indexes the design and current decisions.
- [`docs/storage.md`](docs/storage.md) defines the worker-local state store layout, schema, permissions, locking, archive, and recovery.
- [`docs/architecture.md`](docs/architecture.md) defines system boundaries and failure assumptions.
- [`docs/protocol.md`](docs/protocol.md) defines resources, state, lifecycle operations, output, and compatibility.
- [`docs/task-cli.md`](docs/task-cli.md) defines the supported repository and task command syntax, output schemas, errors, and exit codes.
- [`docs/credentials.md`](docs/credentials.md) defines credential discovery, validation, and managed-launch environment behavior.
- [`docs/integration-gate.md`](docs/integration-gate.md) defines the integration signal and immediate disable path.
- [`docs/technology.md`](docs/technology.md) records the implementation-stack evaluation.
- [`docs/roadmap.md`](docs/roadmap.md) stages shipped local work and tracked follow-ups.
- [`docs/handoff.md`](docs/handoff.md) records current implementation status and limitations.
