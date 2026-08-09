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

## Direction

Near-term work is local only:

1. Validate protocol output and task identifiers.
2. Add worker-local task records and atomic lifecycle operations.
3. Add tmux task startup, inspection, state publication, stop, finish, and reconciliation.
4. Add repository policy and Git worktree ownership.
5. Add optional LLM hooks and plugins as thin CLI consumers.
6. Add explicitly selected remote EC2 workers after the local protocol is stable.

Containers, automatic scheduling, and a central service are deferred.
