# Durable registry charter

**Status:** accepted direction for issue [#138](https://github.com/akofink/akagent-cli/issues/138).

This document distinguishes the shipped local implementation from the target boundary.
The current orchestration behavior is compatibility behavior, not the end state.

## Recommendation

`akagent` is a narrow, local-first protocol for durable task, resource, and execution records, not the long-term owner of process orchestration.

The core should record intent, facts, conditions, recovery debt, checkpoint references, and delivery references through the stable CLI.
External tools and skills should own Git worktree mutation, process launch, tmux interaction, credential injection, and provider-specific session behavior.
Optional local or provider adapters may provide those capabilities, but they are not required dependencies of the core or a feature-parity migration deliverable.

This accepted boundary does not itself remove behavior or authorize a broad refactor.
Existing commands and records remain compatibility surfaces while the seams are introduced incrementally.

The distinction is important: an execution record remains useful when a launcher, terminal, provider, or orchestrator disappears, but the record does not require `akagent` to have launched or killed the process.

## Current implementation status

The current CLI ships local Git/worktree mutation, process launch and stop, tmux attachment, credential handling, deployment execution, and reconciliation.
Those paths are explicitly shipped but transitional compatibility behavior.
The record model, protocol output, and direct self-service workflow are the durable product boundary.

## Keep, deprecate, and remove matrix

| Surface | Current status | Accepted direction |
| --- | --- | --- |
| Task, resource, and execution identities, typed state, events, archive history, recovery debt, and protocol output | Shipped | Keep in core |
| Session-reference declarations, evidence metadata, and delivery references | Shipped | Keep in core; integrations own provider interpretation |
| Task/resource/execution inspection, list, publication, finish, archive, and reconciliation records | Shipped | Keep in core; adapters submit observations |
| Repository and resource records | Shipped, with local Git setup in the current implementation | Keep records in core; move Git mutation and ownership checks to external tools or skills, with optional adapters as a convenience |
| `task launch`, `task execution launch`, and `integration launch` | Shipped compatibility behavior | Deprecate, then remove orchestration from core after the exit criteria |
| `task attach`, execution attach, task/execution stop, and local deployment execution | Shipped compatibility behavior | Deprecate, then remove process, tmux, and deployment side effects from core; direct tools or skills may replace only the capabilities still needed |
| Credential injection and cleanup | Shipped compatibility behavior | Move value resolution and cleanup outside core, or retire these capabilities; keep only non-secret observations and recovery facts in core |

The matrix describes existing command families only.
The migration does not require feature-parity adapters for every deprecated capability.
Deployment and credential behavior may be intentionally retired when direct work does not need a replacement.
Current syntax remains unchanged until an implementation issue changes it.

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
An external tool, skill, or optional adapter creates an execution record before starting work, then submits process and environment observations through explicit interfaces.

The core does not call tmux, inspect PIDs, run Git, create or remove worktrees, resolve credential values, parse provider sessions, or call GitHub.
External tools and skills may provide the side effects still needed by a workflow, and optional adapters may make those tools more convenient.
No adapter is required to reproduce every legacy feature, and an absent tool must not make the durable protocol unavailable.

This is the recommended direction because it preserves the most valuable behavior while making authority explicit.
`akagent` remains useful to direct agents and independent orchestrators without requiring either one to adopt a particular launcher.

## Component boundaries

The stable boundary is the CLI and its typed record schemas, not an exported Go package graph.
The following ownership rules should guide future changes.

| Component | Owns | Does not own |
| --- | --- | --- |
| Protocol and domain | IDs, task/resource/execution records, lifecycle transitions, conditions, recovery debt, session-reference shape, delivery metadata, and compatibility rules | Processes, tmux, Git, credentials, provider files, or forge APIs |
| Local store | Restrictive worker-local paths, envelopes, atomic replacement, append-only events, locks, archives, and recovery of store artifacts | Lifecycle policy, command parsing, tmux, Git, or credential values |
| External local tools and skills | Git facts, branch and worktree operations, process observations, cleanup hooks, and any local side effects still required by a workflow | Provider session interpretation or durable record file access outside the store interface |
| Optional interaction adapter | Launch, attach, stop, and observe for tmux or another terminal surface | Durable task truth or ambiguous window ownership decisions |
| Optional provider adapter | Provider command construction, session discovery, provider policy, and provider-specific environment setup | Core record schema, forge delivery, or unrelated credentials |
| CLI layer | Narrow argument parsing, command dispatch, protocol encoding, and structured errors | Business policy hidden in command branches or direct store-file manipulation |
| External delivery tooling | GitHub, Bitbucket, or other forge operations | Core task state and provider credentials in records or output |

An external tool, skill, or optional adapter may report `unavailable`, `stale`, or `contradictory` observations.
The core must preserve those facts and avoid inferring successful completion from a missing observation.
Destructive actions such as killing a window or removing a worktree require an ownership proof from the external tool or skill performing the action and an explicit approval path.

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

The migration has a finite sequence of gates.

1. Adopt record-only task, resource, and execution creation and observation submission as the normal agent workflow.
Launch-before-record behavior remains unsupported so a failed launch remains recoverable.
2. Deliver independent in-flight and maintenance views that work from durable records while offline and do not require tmux, a provider, or a forge.
3. Add crash-safe checkpoint references and same-machine reboot recovery.
Recovery must identify unfinished work, preserve the last durable checkpoint, and allow a safe partial recovery without claiming exact process restoration.
4. Move tmux, Git/worktree, credential, deployment, and provider side effects outside the core.
Direct tools and skills are valid owners, while optional local or provider adapters are convenience integrations rather than required replacements.
Core records must remain usable when any of them is absent.
5. Deprecate the existing orchestration command families with stderr diagnostics while preserving their current syntax and stdout schemas during the migration window.
6. Remove direct orchestration from core when the exit criteria below are met.
Users of removed command families receive structured migration guidance to create or update records through the durable CLI and invoke an external tool, skill, or optional adapter only when the workflow still needs that side effect.
Legacy manifests, archives, and event histories remain readable after removal.

### Orchestration removal exit criteria

Removal is allowed only when all of these are true:

- Record-only create, inspect, publish, archive, and reconciliation flows pass offline without tmux, Git mutation, credentials, deployment executables, providers, or forge access.
- In-flight and maintenance views expose unfinished work, recovery debt, checkpoint references, and adapter availability without provider transcripts or private context.
- Checkpoint writes and concurrent updates are crash-safe and idempotent, and reboot recovery has tested safe partial recovery for missing, stale, and contradictory observations.
- Direct external tools and skills, or optional adapters where useful, can create and submit the observations needed by retained workflows without core-side side effects.
No feature-parity adapter is required for a capability that is intentionally retired.
- Existing records and archives from the compatibility period can be inspected and migrated without data loss.
- The removed command response and migration documentation are covered by protocol tests and identify the replacement record and the direct tool, skill, or optional adapter workflow when one exists.

Same-machine reboot recovery assumes the worker filesystem survives.
Disk loss requires a separately copied archive or other external backup.
Cross-machine synchronization is not provided by the local store.
Exact process restoration is not promised; a new process must be verified by an adapter.
Provider session recovery is best effort and depends on provider-owned references and discovery.

Adding optional observation or adapter metadata is compatible; changing lifecycle meanings or removing record fields requires a protocol version change.
A provider or orchestrator that is replaced can reattach by recording a session reference and fresh observation instead of recreating the task.

## Security and operations

The core must never persist credential values, prompt contents, provider session contents, or unrelated inherited environment values.
Adapters resolve named credentials immediately before use and pass only the requested values to a child process.
External URLs and session references remain non-secret declarations and must be validated without opening provider files.

The protocol should remain useful when tmux is absent, the operator terminal disconnects, the provider exits, or the network is unavailable.
Launchers should create an execution record before side effects and use stable IDs for retries.
External tools, skills, and optional adapters should report uncertain outcomes rather than guessing, and reconciliation should be scoped to the execution and resource that the caller owns.
The core can then preserve recovery debt and expose actionable state without attempting a dangerous cleanup itself.

Local-first does not mean authority-free.
The store continues to enforce restrictive permissions, descriptor-safe traversal, atomic writes, and per-task locking.
External commands must validate paths and ownership at the point of mutation, and no tool, skill, or optional adapter may treat a display label, active tmux client, PID alone, or provider filename as proof of identity.

## Verification strategy

Tests should model the boundary and failure modes rather than only the happy path.
Core tests should cover idempotent creates, concurrent event appends, malformed or partially written records, state transitions, archive recovery, observation freshness, and preservation of unknown or contradictory facts.

Adapter tests should cover tmux pane-local targeting, active-window mismatch, stale and reused PIDs, duplicate metadata, a window disappearing between observe and stop, verification failure after kill, and commands that exit before metadata initialization.
Git tests should cover branch and worktree collisions, wrong common directories, dirty and untracked work, direct-checkout protection, and cleanup refusal.
Provider tests should cover unavailable executables, unsafe prompt references, readiness failures, process replacement, and the absence of secret values from arguments, logs, and records.
CLI tests should keep parser behavior, structured errors, TOON determinism, and direct core operation without optional providers explicit.

The existing `go test ./...`, `go test -race ./...`, `go vet ./...`, and `git diff --check` checks remain appropriate for this documentation-led transition.
No behavioral removal or broad refactoring is part of this proposal.
