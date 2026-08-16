---
name: developing-akagent
description: Implements and reviews akagent-cli GitHub issues. Use when changing the Go CLI, TOON protocol output, local state, credentials, tmux or worktree lifecycle, integrations, or worker behavior in this repository.
---

# Developing akagent

## Orient

1. Read the assigned GitHub issue, `AGENTS.md`, and `docs/handoff.md`.
2. Read only the relevant design document: `docs/protocol.md`, `docs/credentials.md`, `docs/architecture.md`, or `docs/implementation-plan.md`.
3. Confirm the issue's owned files and out-of-scope boundaries before editing.

## Preserve the protocol

- Keep typed domain values separate from TOON encoding.
- Keep default output schemas minimal and deterministic.
- Write structured errors to stdout with actionable recovery and conventional exit codes.
- Keep diagnostics on stderr and opt-in.
- Make mutations idempotent and recoverable.
- Treat tmux as an interaction surface, not durable state.
- Use generic execution records for process launches; optional provider integrations must remain outside task and resource lifecycle behavior.
- Keep direct human shell execution explicitly available without requiring an external provider.
- Never expose secret values or inherit unrelated credentials into managed processes.

## Verify

Run:

```bash
gofmt -w <changed-go-paths>
go test ./...
go test -race ./...
go vet ./...
git diff --check
```

Add focused failure and concurrency tests for stateful behavior.
Include dogfood coverage for one execution coordinating multiple resources and for core commands without optional providers.
Show verification evidence rather than only stating that checks pass.

## Deliver

1. Keep one signed Conventional Commit for the issue.
2. Include `Fixes #N` in the commit message and pull-request body.
3. Rebase on current `origin/main` before publishing when the base changed.
4. Push the issue branch and open a pull request only when requested.
5. Do not merge or mutate another agent's issue, worktree, branch, or pull request.
