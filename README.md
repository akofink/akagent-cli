# akagent

`akagent` is a local-first orchestration protocol and CLI for coding agents.
It preserves tmux and Git worktree workflows while providing stable task identity, structured state, reconciliation, and eventual remote-worker support.

The repository is named `akagent-cli`.
The installed binary is `akagent`, and `aka` may be configured as a short shell alias.

## Status

This repository currently contains the protocol foundation spike.
It intentionally does not yet create tasks or mutate tmux and Git worktrees.

Implemented commands:

```text
akagent
akagent id generate
akagent update [--source <path>]
akagent worker inspect
```

Stdout uses TOON because coding agents are the primary machine consumers.
The initial encoder dependency is under evaluation against the current TOON specification and is not yet a durable storage decision.

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
- [`docs/architecture.md`](docs/architecture.md) defines system boundaries and failure assumptions.
- [`docs/protocol.md`](docs/protocol.md) defines resources, state, lifecycle operations, output, and compatibility.
- [`docs/credentials.md`](docs/credentials.md) defines credential discovery, validation, propagation, and cleanup.
- [`docs/technology.md`](docs/technology.md) records the implementation-stack evaluation.
- [`docs/roadmap.md`](docs/roadmap.md) stages local, integration, remote, and discovery work.
- [`docs/handoff.md`](docs/handoff.md) records implementation status and the next concrete work.
