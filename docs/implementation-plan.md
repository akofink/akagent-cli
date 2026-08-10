# Implementation plan

## GitHub workflow

GitHub issues are the source of truth for implementation scope and completion.
Each implementation issue receives one dedicated branch, worktree, agent session, signed commit, and pull request.

Commit messages and pull-request bodies use `Fixes #<issue>` so GitHub closes the issue when the pull request merges.
Do not manually close an implementation issue merely because code was committed or a pull request was opened.

The default branch remains protected operationally by workflow rather than direct implementation commits:

1. Fast-forward the primary clone's `main` to `origin/main`.
2. Create `akofink/<issue>-<slug>` in a dedicated worktree.
3. Implement only the issue's stated scope.
4. Run `go test ./...`, `go test -race ./...`, `go vet ./...`, and focused checks.
5. Create one signed Conventional Commit containing `Fixes #<issue>`.
6. Rebase or recreate the commit on current `origin/main` before publishing when necessary.
7. Push the issue branch and open a pull request with the same closure reference.
8. Wait for CI and review; merging remains an explicit operator action.

## Phase 1 work graph

Parent issue: [#1 Phase 1: establish local protocol foundations](https://github.com/akofink/akagent-cli/issues/1)

The first wave contains three parallel child issues:

| Issue | Scope | Primary ownership | Shared-file risk |
| --- | --- | --- | --- |
| [#2 Validate and pin TOON output contract](https://github.com/akofink/akagent-cli/issues/2) | Specification, encoder conformance, output fixtures, token measurements | `internal/output/` and focused TOON docs | Low; avoid command registration |
| [#3 Implement secure worker-local state store](https://github.com/akofink/akagent-cli/issues/3) | Typed envelopes, XDG state root, atomic writes, locks, recovery | New `internal/store/` package and storage docs | Low; avoid CLI commands and credentials |
| [#4 Add local credential manifest and doctor](https://github.com/akofink/akagent-cli/issues/4) | Manifest, `file:` and `env:` readiness, permissions, list/inspect/doctor | New `internal/credential/`, command registration, credential docs | Moderate; owns its necessary `internal/app/` changes |

The boundaries deliberately keep #2 and #3 away from `internal/app/` while #4 owns the first new command integration.
Agents must not opportunistically refactor shared command dispatch, output schemas, or unrelated documentation.

## Parallel execution

Launch one Pi session per child issue from a stable directory.
Each session receives its exact issue URL, branch, worktree, verification commands, commit requirement, and instruction to leave its window available after completion.

Each agent must:

- Read `AGENTS.md`, the issue, `docs/handoff.md`, and relevant design documents.
- Treat issue acceptance criteria as the scope boundary.
- Keep its `WORKING_STATE.md` entry current.
- Commit with `Fixes #N` and a signed commit.
- Prepare and push a branch and pull request only when explicitly included in its launch instruction.
- Publish tmux `@agent_state` as `waiting`, `blocked`, or `done` when appropriate.
- Report blockers without editing another issue's worktree.

## Integration order

The issues are implementation-independent but semantic dependencies remain:

1. Review #2 first when another branch needs to emit new TOON schemas.
2. Review #3 before defining task lifecycle commands because it establishes persistence and locking behavior.
3. Review #4 after confirming its output conforms to #2 or rebase it onto #2 if required.
4. Re-run the full race-enabled suite after every merge.
5. Close parent #1 only after all children are merged, documentation is reconciled, and a follow-up task-lifecycle issue is refined.

Merge order may change when branches remain independent.
The orchestrator should prefer the least-conflicting order supported by current diffs and CI rather than imposing artificial sequencing.

## Foundation completion

Issues [#2](https://github.com/akofink/akagent-cli/issues/2), [#3](https://github.com/akofink/akagent-cli/issues/3), and [#4](https://github.com/akofink/akagent-cli/issues/4) are merged into `main`.
The output boundary is pinned to TOON 4.1 with a documented supported subset and conformance floors.
The state store uses typed JSON envelopes, descriptor-safe durable mutation, per-task locking, and recovery.
The credential package provides non-secret manifest discovery, metadata-only readiness, and structured list, inspect, and doctor commands.

The next implementation issue is [#8 Implement local task lifecycle commands](https://github.com/akofink/akagent-cli/issues/8).
It owns the first integration surface across these foundations and must keep repository policy, task lifecycle, tmux observation, credential capabilities, and protocol output behind the stable CLI boundary.

## Orchestrator role

An OpenCode orchestrator remains idle while implementation agents work.
It may inspect tmux state, GitHub issues, pull requests, commits, diffs, CI, and the working-state board.

It must not edit another agent's worktree, post or mutate pull requests, merge branches, or redirect a live agent without explicit operator instruction.
When agents finish, it should report verified evidence, conflicts, recommended review order, and the next decision needed from the operator.

## Next waves

After #2, #3, and #4 are integrated:

1. Define repository registration and policy discovery.
2. Implement local task creation using the store and credential requirements.
3. Add tmux window identity, attachment, state publication, finish, and stop.
4. Add Git worktree ownership and repository locking.
5. Add reconciliation, archival, cleanup, and preservation checks.
6. Add opt-in LLM hooks and Pi integration over the stable CLI.
7. Begin one named remote EC2 worker only after the local lifecycle is reliable.

Remote discovery, the stale-aware local cache, containers, schedulers, and a central service remain later work.
