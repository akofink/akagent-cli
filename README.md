# akagent

`akagent` is a local-first orchestration protocol and CLI for coding agents.
It preserves tmux and Git worktree workflows while providing stable task identity, structured state, reconciliation, and eventual remote-worker support.

The repository is named `akagent-cli`.
The installed binary is `akagent`, and `aka` may be configured as a short shell alias.

## Status

This repository provides the protocol foundation and initial local task lifecycle.
Task manifests and append-only events are durable state; tmux is an observed interaction surface.

Implemented commands:

```text
akagent
akagent credential <list|inspect|doctor>
akagent id generate
akagent repository <register|list|inspect|update|unregister>
akagent task <start|list|inspect|publish|finish|stop|archive|clean|reconcile>
akagent update [--source <path>]
akagent worker inspect
```

Repository registration requires a Git worktree and records applicable `AGENTS.md` instructions.

Stdout uses TOON because coding agents are the primary machine consumers.
The TOON output contract is pinned to specification version 4.1 with a validated encoder and official conformance fixtures; see [`docs/toon.md`](docs/toon.md).

## Local tasks

Register a local repository before starting a task.

```bash
akagent repository register akagent-cli ~/dev/repos/akagent-cli
akagent task start --title "Implement lifecycle" --repository akagent-cli
```

`task start` creates a durable task record, an isolated Git branch and worktree when repository policy requires it, and a detached tmux window.
Use `--branch`, `--base`, and `--worktree` to provide explicit immutable Git inputs.
Repeated equivalent starts are no-ops, while conflicting task inputs are rejected.
`task publish` persists an agent condition, reason, activity, and heartbeat without exposing credential values.
`task reconcile` compares durable task records with tmux and Git observations, reports dirty or untracked work and recovery debt, and never deletes task state, worktrees, branches, or terminal history.
Required credential IDs passed with `--require` must be ready; unavailable `--optional` IDs are reported as non-secret warnings.

## Credentials

`akagent` discovers local credentials from a versioned, non-secret manifest at `~/.config/akagent/credentials.toon`.
The manifest holds only source references and policy, never secret values; `akagent` never reads or prints the underlying secrets.

```toon
version: 1
credentials[3]{id,type,source,required_for}:
  git-ssh,ssh_key,file:~/.local/share/akagent/credentials/git_ed25519,git
  github,api_token,env:GITHUB_TOKEN,github
  llm,api_token,env:OPENAI_API_KEY,
```

Each row names a credential `id`, a non-secret `type`, a `source` reference, and a `required_for` capability.
A trailing empty `required_for` marks an optional credential, which only warns when its source is unavailable.
The supported manifest schema version is `1`; newer versions and duplicate IDs are rejected.
TOON-quoted fields may contain commas, whitespace, quotes, and escaped characters.
Missing or misconfigured required credentials block readiness.

Supported source kinds are `file:<path>` and `env:<VAR>`.
A leading `~/` in a `file:` path expands to the current user's home directory.
`file:` sources are validated by metadata only (existence, ownership, and mode) and are never opened for reading.
Credential files must be exactly mode `0600` in an exactly mode `0700` directory owned by the current user; symlinks and unsafe special permission bits are rejected.
On platforms without Unix ownership and mode semantics, file readiness is reported as `unsupported` rather than inferred from synthesized permission bits.
`env:` sources are checked for presence without persisting or printing their values.

Commands:

```text
akagent credential list
akagent credential inspect <id>
akagent credential doctor
```

`list` reports each credential identity, status, and source kind.
`inspect <id>` shows one credential's non-secret detail.
`doctor` runs all readiness checks and exits nonzero when a required credential is not ready.
Set `AKAGENT_CREDENTIALS` to use a non-default manifest path.

See [`docs/credentials.md`](docs/credentials.md) for the full credential model.

## Development

```bash
go test ./...
go vet ./...
go run ./cmd/akagent
```

## Installation and updates

The current installation model uses a local source checkout and a user-local binary:

```bash
git clone https://github.com/akofink/akagent-cli ~/dev/repos/akagent-cli
cd ~/dev/repos/akagent-cli
go build -o ~/.local/bin/akagent ./cmd/akagent
```

Machine setup should perform the same build through a temporary file and atomic rename rather than writing directly over the installed binary.

Update an installed binary with:

```bash
akagent update
```

The updater expects a clean `~/dev/repos/akagent-cli` checkout on `main`, locks the installed binary, fetches `origin`, and fast-forwards to `origin/main`.
It builds from a temporary detached worktree at that exact commit, then atomically replaces the installed executable.
Use `AKAGENT_SOURCE_DIR` or `--source <path>` for a non-default checkout.

Source-managed self-update currently supports macOS and Linux.

Automatic update on every invocation is intentionally deferred because ordinary commands should not unexpectedly require network access, mutate source, or add startup latency.
A later rate-limited background check can call the same explicit updater if measured use justifies it.

## Direction

Near-term work is local only:

1. Validate protocol output and task identifiers.
2. Add worker-local task records and atomic lifecycle operations.
3. Add tmux task startup, inspection, state publication, stop, finish, and reconciliation.
4. Add repository policy and Git worktree ownership.
5. Add optional LLM hooks and plugins as thin CLI consumers.
6. Add explicitly selected remote EC2 workers after the local protocol is stable.

Containers, automatic scheduling, and a central service are deferred.

## Design documentation

- [`docs/README.md`](docs/README.md) indexes the design and current decisions.
- [`docs/storage.md`](docs/storage.md) defines the worker-local state store layout, schema, permissions, locking, and recovery.
- [`docs/architecture.md`](docs/architecture.md) defines system boundaries and failure assumptions.
- [`docs/protocol.md`](docs/protocol.md) defines resources, state, lifecycle operations, output, and compatibility.
- [`docs/credentials.md`](docs/credentials.md) defines credential discovery, validation, propagation, and cleanup.
- [`docs/technology.md`](docs/technology.md) records the implementation-stack evaluation.
- [`docs/roadmap.md`](docs/roadmap.md) stages local, integration, remote, and discovery work.
- [`docs/handoff.md`](docs/handoff.md) records implementation status and the next concrete work.
