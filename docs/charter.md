# Proposed charter boundary

**Status:** design proposal for issue [#122](https://github.com/akofink/akagent-cli/issues/122).

## Recommendation

`akagent` should become a narrow, local-first protocol for durable task, resource, and execution records, not the owner of process orchestration.

The core should record intent, facts, conditions, recovery debt, and delivery references through the stable CLI.
Local host adapters should own Git worktree mutation, process launch, tmux interaction, credential injection, and provider-specific session behavior.

This is a boundary proposal, not a behavioral removal or broad refactor.
Existing commands and records remain compatibility surfaces while the seams are introduced incrementally.

The distinction is important: an execution record remains useful when a launcher, terminal, provider, or orchestrator disappears, but the record does not require `akagent` to have launched or killed the process.

## Alternatives

### Retain orchestration in the core

This keeps the current turnkey experience and requires the least migration.
The same command can create a record, create a worktree, launch a process, attach to tmux, stop it, and reconcile it.

The cost is a growing authority boundary around one `lifecycle.Manager`.
It currently combines task creation, repository and worktree policy, credential checks, process lifecycle, tmux observation, reconciliation, archive, cleanup, and legacy compatibility in `internal/lifecycle/lifecycle.go`.
Execution-specific lifecycle and tmux operations continue in `internal/lifecycle/execution.go`, while Pi launch policy and process replacement live in `internal/integration/pi.go`.

The recent tmux failures show why this coupling is risky.
[Issue #118](https://github.com/akofink/akagent-cli/issues/118) fixed metadata initialization that used the active client context and could tag the operator window instead of the new execution window.
[Issue #120](https://github.com/akofink/akagent-cli/issues/120) identifies the corresponding reconciliation hazard: a live window can be stopped while its durable execution is still `created` or `starting`, and an incorrectly tagged operator window can be mistaken for the managed execution.
Each fix increases verification rules and recovery paths inside the same authority boundary.

This option is acceptable only if `akagent` is explicitly a local tmux orchestrator.
It is not the recommended long-term charter because tmux and process control are interaction concerns with inherently racy observations, not durable task identity.

### Deprecate direct launch but retain recoverable execution records

This option keeps `task execution create`, inspection, publication, session references, archive, and reconciliation records in the core.
Direct launch commands become compatibility wrappers that warn and delegate to a local adapter.

It offers a low-risk migration and preserves the current user experience while new orchestrators can create and update records directly.
It also separates the durable record model from provider session files without forcing an immediate command break.

The limitation is that the core still needs launch and tmux contracts for the compatibility path.
That leaves the most failure-prone authority in the core for as long as the compatibility commands remain active.
This is the right transition phase, but not the final charter.

### Remove orchestration from the core

The core retains task, resource, and execution identity; typed state transitions; append-only events; local durable storage; conditions and recovery debt; session-reference declarations; delivery metadata; archive; and protocol output.
An adapter creates an execution record before starting work, then submits process and environment observations through explicit interfaces.

The core does not call tmux, inspect PIDs, run Git, create or remove worktrees, resolve credential values, parse provider sessions, or call GitHub.
A local adapter may still provide all of those features, but it is replaceable and can fail without making the durable protocol unavailable.

This is the recommended direction because it preserves the most valuable behavior while making authority explicit.
`akagent` remains useful to direct agents and independent orchestrators without requiring either one to adopt a particular launcher.

## Component boundaries

The stable boundary is the CLI and its typed record schemas, not an exported Go package graph.
The following ownership rules should guide future changes.

| Component | Owns | Does not own |
| --- | --- | --- |
| Protocol and domain | IDs, task/resource/execution records, lifecycle transitions, conditions, recovery debt, session-reference shape, delivery metadata, and compatibility rules | Processes, tmux, Git, credentials, provider files, or forge APIs |
| Local store | Restrictive worker-local paths, envelopes, atomic replacement, append-only events, locks, archives, and recovery of store artifacts | Lifecycle policy, command parsing, tmux, Git, or credential values |
| Local host adapter | Repository registration, Git facts, branch and worktree operations, process observations, and cleanup hooks | Provider session interpretation or durable record file access outside the store interface |
| Interaction adapter | Launch, attach, stop, and observe for tmux or another terminal surface | Durable task truth or ambiguous window ownership decisions |
| Provider adapter | Provider command construction, session discovery, provider policy, and provider-specific environment setup | Core record schema, forge delivery, or unrelated credentials |
| CLI layer | Narrow argument parsing, command dispatch, protocol encoding, and structured errors | Business policy hidden in command branches or direct store-file manipulation |
| External delivery tooling | GitHub, Bitbucket, or other forge operations | Core task state and provider credentials in records or output |

An adapter may report `unavailable`, `stale`, or `contradictory` observations.
The core must preserve those facts and avoid inferring successful completion from a missing observation.
Destructive actions such as killing a window or removing a worktree require an adapter-specific ownership proof and an explicit approval path.

Tmux metadata is an observation and routing hint, not an authorization token.
A tmux adapter must resolve a new window from its pane-local identity, verify task and execution metadata immediately before attachment or cleanup, compare fresh process identity including start time, and refuse ambiguous matches.
The core should not reproduce those rules or pretend that a window name is durable state.

## Package layout principles

The code should grow by cohesive, small packages with one reason to change.
Package names should describe a boundary such as `store`, `protocol`, `worktree`, `tmux`, or `provider`, rather than a feature bucket that accumulates unrelated orchestration.

- Keep durable records, validation, and state transitions independent from operating-system adapters.
- Keep `internal/store` isolated from tmux, Git, credentials, and CLI parsing, as it is today.
- Define narrow interfaces at system boundaries for observations, launch, attachment, cleanup, Git, credential resolution, and clock or process identity.
- Keep command parsing shallow in `internal/app` and translate parsed values into service requests instead of embedding lifecycle policy in switch branches.
- Keep local store access behind a store service or repository interface so adapters cannot mutate record files directly.
- Keep provider policy and session discovery in provider packages that submit non-secret references and observations through the CLI or service boundary.
- Keep protocol encoding at the output boundary and keep typed domain values separate from TOON views.

The current code provides incremental seams rather than requiring a rewrite.
`internal/store` already isolates file durability and security.
`internal/credential` already separates named capability readiness from secret values, and `internal/pi` already isolates Pi launch policy.
The next seams can extract the concrete `commandTmux` implementation from `internal/lifecycle/lifecycle.go`, move Git and worktree operations behind a local-host package, and leave lifecycle code responsible for applying recorded facts.

`internal/lifecycle/lifecycle.go` is the first extraction target because its `Manager` and concrete adapters currently span task creation, Git, tmux, Pi startup, cleanup, and reconciliation.
`internal/lifecycle/execution.go` is a second seam for separating execution record transitions from launch, attachment, and stop operations.
`internal/app/task.go` combines parsing, dispatch, and output views for task, resource, and execution commands, so command-family helpers can be split without changing the CLI contract.
These are focused extractions around existing interfaces, not permission to redesign the storage schema or rewrite the command surface.

## Migration and compatibility

1. Document the protocol-only charter and add an explicit adapter interface while preserving the current implementation behind it.
2. Make execution creation and observation submission the primary path.
Keep launch-before-record behavior unsupported, so a failed launch remains recoverable.
3. Move tmux, Git/worktree, and Pi behavior behind local or provider adapters.
The existing adapters may initially call the same lifecycle methods while ownership moves out of the core.
4. Keep `task launch`, `task execution launch`, and `integration launch` as compatibility commands that delegate to an installed local adapter and emit deprecation diagnostics on stderr.
Their stdout schemas and existing durable records remain stable during the migration window.
5. Add an explicit adapter capability or command discovery path for environments without tmux, Git worktrees, or a provider.
Core task and record operations must continue to work in those environments.
6. After adapter coverage and recovery tooling are established, remove direct launch behavior from the core and retain a separately versioned compatibility adapter for users who need the old commands.

Existing manifests, archives, and event histories must remain readable.
Adding optional observation or adapter metadata is compatible; changing lifecycle meanings or removing record fields requires a protocol version change.
A provider or orchestrator that is replaced can reattach by recording a session reference and fresh observation instead of recreating the task.

## Security and operations

The core must never persist credential values, prompt contents, provider session contents, or unrelated inherited environment values.
Adapters resolve named credentials immediately before use and pass only the requested values to a child process.
External URLs and session references remain non-secret declarations and must be validated without opening provider files.

The protocol should remain useful when tmux is absent, the operator terminal disconnects, the provider exits, or the network is unavailable.
Launchers should create an execution record before side effects and use stable IDs for retries.
Adapters should report uncertain outcomes rather than guessing, and reconciliation should be scoped to the execution and resource that the adapter owns.
The core can then preserve recovery debt and expose actionable state without attempting a dangerous cleanup itself.

Local-first does not mean authority-free.
The store continues to enforce restrictive permissions, descriptor-safe traversal, atomic writes, and per-task locking.
Adapter commands must validate paths and ownership at the point of mutation, and no adapter may treat a display label, active tmux client, PID alone, or provider filename as proof of identity.

## Verification strategy

Tests should model the boundary and failure modes rather than only the happy path.
Core tests should cover idempotent creates, concurrent event appends, malformed or partially written records, state transitions, archive recovery, observation freshness, and preservation of unknown or contradictory facts.

Adapter tests should cover tmux pane-local targeting, active-window mismatch, stale and reused PIDs, duplicate metadata, a window disappearing between observe and stop, verification failure after kill, and commands that exit before metadata initialization.
Git tests should cover branch and worktree collisions, wrong common directories, dirty and untracked work, direct-checkout protection, and cleanup refusal.
Provider tests should cover unavailable executables, unsafe prompt references, readiness failures, process replacement, and the absence of secret values from arguments, logs, and records.
CLI tests should keep parser behavior, structured errors, TOON determinism, and direct core operation without optional providers explicit.

The existing `go test ./...`, `go test -race ./...`, `go vet ./...`, and `git diff --check` checks remain appropriate for this documentation-led transition.
No behavioral removal or broad refactoring is part of this proposal.
