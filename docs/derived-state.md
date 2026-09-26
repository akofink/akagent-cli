# Derived state and deterministic checks

This document defines how `akagent` stops asking agents to hand-record facts that drift from their real source of truth.
It decides which record fields stay, which become derived, and which are retired, and it defines the adapter contract and the `task check` command.

## Problem

The record-only registry asks agents to publish conditions, observations, Git facts, and delivery metadata that describe systems akagent does not own.
Those copies drift after a rebase, merge, branch deletion, terminal restart, provider quota stop, or lost machine.
In daily use, stores accumulated dozens of executions still `active`, `waiting`, or `created` under tasks that had already finished, and no inventory view surfaced them.
Resource views printed `committed: false`, `dirty: false`, and `untracked: false` for every resource even though no current command ever observes those facts.
Operators then reconciled records by hand against Git, GitHub, and terminal panes, which is the work akagent should do deterministically.

## Principles

- The real system is the authority for its own state: Git for checkouts and branches, the forge for pull requests and checks, the terminal host for panes, and the provider for sessions.
- akagent keeps only identity, intent, bindings, lineage, and explicit decisions, and derives everything else at read time.
- The local akagent store is a cache.
  The operator's durable work record lives outside akagent, and every kept field must be reconstructable from it or from live sources.
  Losing the store must never lose the only copy of a decision.
- Derived state is observed, never written back.
  `task check` does not mutate the store, a checkout, the forge, a pane, or a provider session.
- `task inspect`, `task list`, reconciliation, and every write path stay offline and never call an adapter.
  Only `task check` reads live systems, and only when asked.
- `unknown` is not `missing`, `missing` is not completion, and a green check is not delivery.
  Completion stays an explicit, guarded declaration against a named contract.
- Output is deterministic for the same store snapshot and the same adapter responses.
  It contains no timestamps, credentials, command output, file paths from provider state, or provider content.

## Sources of truth

| Fact | Authority | How akagent gets it |
| --- | --- | --- |
| Task identity, title, intent, disposition, completion | Operator work record and explicit akagent commands | Kept (cache) |
| Repository, branch, and worktree binding | Agent declaration at creation | Kept (cache) |
| Worktree existence, registration, branch, dirty and untracked state | Git | Git adapter |
| Local branch existence and head | Git | Git adapter |
| Pull request identity, state, and head | Forge | GitHub adapter |
| Check runs and commit statuses on the PR head | Forge | GitHub adapter |
| Execution identity, lineage, command label, session references | Agent declaration | Kept (cache) |
| Whether an execution's agent is still running, idle, or done | Terminal host | Terminal adapter (designed below, not shipped) |
| Whether a provider session record still exists | Provider | Provider adapter (designed below, not shipped) |
| Consistency between kept records | akagent store | Record-consistency rules (offline) |

## Record disposition

"Keep" means akagent stores it as a cache that the operator record can restore.
"Derive" means `task check` computes it live and akagent stops asking agents to write it.
"Legacy" means the field stays readable for compatibility and is no longer presented or recommended; removal waits for a versioned protocol migration.

### Tasks

| Field | Decision | Notes |
| --- | --- | --- |
| ID, title, provenance, caller, revision | Keep | Identity and guarded-write anchors. |
| Disposition (`in-flight`, `deferred`, `terminal`) | Keep | Operator decision, not derivable. |
| Condition, reason, activity | Keep as a claim | Agent-declared; `task check` flags claims contradicted by records or live state. |
| Finish result, external completion, archive | Keep | Explicit completion is never derived. |
| Checkpoints, events | Keep | Bounded handoff and audit history. |
| Task-level branch, base, worktree | Legacy | Superseded by resources. |
| Task-level `committed`, `dirty`, `untracked` | Legacy | Already omitted from views unless a legacy record set them. |
| Cleanup, worktree cleanup, credential cleanup states, cleanup debt | Legacy | Cleanup is derived from worktree and branch existence after delivery. |

### Resources

| Field | Decision | Notes |
| --- | --- | --- |
| ID, repository, branch, worktree path, base revision | Keep | The binding every adapter starts from. |
| External URLs | Keep as a hint | The GitHub adapter verifies a linked PR against the repository and branch; a URL alone is never proof. |
| `head` (`--head` at creation) | Legacy | A creation-time snapshot; the Git adapter reports the live head. |
| `committed`, `dirty`, `untracked` | Derive | No v2 command sets them; views omit them unless a legacy record set them. |
| Delivery metadata such as `delivery=pull-request-opened` | Derive | The GitHub adapter reports PR state; stop recording it by hand. |
| Observation history | Keep as history | Never displayed as present truth. |
| Cleanup states and debt | Legacy | As for tasks. |

### Executions

| Field | Decision | Notes |
| --- | --- | --- |
| ID, predecessor, command label, target, resource, working directory | Keep | Identity and lineage for provider or model switches. |
| Session references | Keep | The binding the terminal and provider adapters start from. |
| Lifecycle and condition | Keep as a claim | Compared with task state offline and with live adapters when they ship. |
| Typed observations (`--process-state`, `--result`) | Keep as history | Still useful on hosts without a terminal adapter; stop recommending them where an adapter covers the host. |
| Guarded finish, external completion, handoff disposition | Keep | Explicit completion is never derived. |

### Repositories

Repository registrations stay as cached local paths and policies.
They let the Git adapter find the primary clone for worktree registration checks.

## Findings

Each check produces findings with a fixed shape:

```text
surface  what was checked: task, worktree, branch, pull_request, checks, or execution:<id>
state    current | stale | missing | unknown
code     stable machine-readable reason
detail   optional redaction-safe value such as a PR URL, head commit, or check names
action   deterministic next step for an agent or operator
```

`current` means the live source agrees with the record, or the record needs nothing.
`stale` means a record, claim, or live state disagrees and needs a decision.
`missing` means the bound thing does not exist where it should.
`unknown` means the adapter could not observe the surface: unsupported remote, unreachable host, absent credentials, network failure, ambiguous match, or no adapter yet.
A finding needs action when its state is `stale` or `missing`.
`unknown` findings are shown but never counted as action or as success.

Codes are part of the public protocol.
Adding a code is compatible; renaming or repurposing one is a breaking change.

### Record-consistency rules

These rules read only the store and run even with `--offline`.

| Surface | Condition | State and code |
| --- | --- | --- |
| `execution:<id>` | Task is terminal and the execution is not finished, stopped, completed, or handed off | `stale` `task_terminal` |
| `execution:<id>` | Execution is finished, stopped, completed, or handed off | `current` `closed` |
| `execution:<id>` | Task and execution are open and no liveness adapter applies | `unknown` `adapter_unavailable` |
| `task` | Task condition is `active`, it has executions, and every execution is closed | `stale` `no_open_execution` |
| `task` | `--all` could not load the task's records | `stale` `record_unreadable` |

A task is terminal when it is finished or stopped, externally completed, or archived.

### Live rules

| Surface | Condition | State and code |
| --- | --- | --- |
| `worktree` | Bound path is registered with the repository, on the bound branch, and clean | `current` `registered` |
| `worktree` | Registered but has tracked or untracked changes | `stale` `dirty` |
| `worktree` | Registered but on another branch | `stale` `branch_mismatch` |
| `worktree` | Still registered after the task is terminal | `stale` `retained_after_finish` |
| `worktree` | Not in `git worktree list` or absent from disk | `missing` `not_registered` or `path_missing` |
| `branch` | Local ref exists | `current` `local_ref`, detail is the live head |
| `branch` | Local ref is gone | `missing` `local_ref_missing` |
| `pull_request` | Exactly one PR on the bound repository and head branch | `current` `open` or `merged`, detail is the PR URL |
| `pull_request` | That PR is closed without merge | `stale` `closed` |
| `pull_request` | No PR | `missing` `not_found` |
| `pull_request` | No PR on the branch while a PR URL is recorded | `stale` `link_mismatch` |
| `checks` | Every check run and commit status on the PR head succeeded | `current` `reported_success` |
| `checks` | Any is pending or failed | `stale` `pending` or `failed`, detail lists check names |
| `checks` | None reported | `missing` `none_reported` |
| `checks` | The local branch head differs from the PR head | `stale` `head_mismatch` |
| `task` | Task is open, it has resources, and every resource has a merged PR | `stale` `delivered_unfinished` |

Adapter failures, ambiguous matches, truncated results, and unsupported remotes are `unknown` with a specific code.
One adapter's failure never hides another adapter's findings.
`reported_success` does not name the repository's required check: the agent still verifies the named delivery gate.

## The `task check` command

```text
akagent task check <task-id|keyword> [--offline] [--format <toon|json>]
akagent task check --all [--offline] [--format <toon|json>]
```

Single-task mode reports every finding for the task, including `current` ones.
`--all` checks every task in the store, including archived ones.
It runs live adapters only for tasks that are not terminal, runs record-consistency rules for all of them, and prints only tasks and findings that need action.
Its summary counts every finding it evaluated.
`--offline` skips live adapters and reports their surfaces as `unknown` `offline` in single-task mode.

Exit code `0` means the check ran, even when findings need action.
Exit code `1` means the store could not be read or the task does not exist.
Exit code `2` means invalid arguments.
Callers gate on `summary.needs_action`, not on the exit code.

## Adapters

### Contract

An adapter is an in-process, read-only Go function in `internal/derived` that takes a record snapshot and a `Runner` and returns typed findings.
`Runner` executes one argument-vector command with a bounded context; it never uses a shell and never receives a secret as an argument.
Adapters authenticate only through the external tool's own credential store, such as the `gh` login.
Failures become `unknown` findings with stable codes; raw command output and error text are never copied into findings.
Adapters run in a fixed order, and findings are sorted by resource ID and execution ID.

To add an adapter:

1. Name the surface and its binding: which kept fields it starts from, and which host it must run on.
2. Define its states and codes in this document before shipping.
3. Implement it against `Runner` so tests use fixtures, not the network or a real terminal.
4. Cover current, stale, missing, unavailable, ambiguous, and truncated responses, plus redaction of command failure text.
5. Wire it into the check projection without changing any write path.

External plugin executables are out of scope.
They would reintroduce an orchestration surface and make output depend on unknown code.

### Git

The Git adapter resolves the repository's registered path, falling back to the bound worktree path.
It verifies that the worktree is listed by `git worktree list --porcelain`, that it exists, that its current branch equals the bound branch, and whether `git status --porcelain` reports changes.
It resolves `refs/heads/<branch>` exactly, so a branch name cannot be interpreted as an option.
It never fetches, so remote-tracking refs are not consulted.
A worktree or branch on another host is `unknown`, not `missing`, because the path cannot be resolved locally.

### GitHub

The GitHub adapter derives the owner and repository from the checkout's `origin` URL, accepting only exact `github.com` HTTPS, SSH, and SCP-style remotes without embedded credentials.
It lists PRs for `owner:branch` in every state through `gh api` and filters to the same head repository.
A recorded PR URL only chooses among several PRs on the branch; a single PR on the branch wins over an unrelated URL.
It then reads check runs and combined commit status for the PR head commit.
A PR head that differs from the local branch head makes the checks `stale`, because checks on another commit say nothing about local work.

### Terminal (designed, not shipped)

The terminal adapter reports whether an open execution's agent is still present.
Its binding is the execution's session references, matched exactly against what the terminal host reports.

- Herdr: `herdr agent list` returns each agent's session identifier and status.
  A session reference whose ID or reference path equals an agent's session value maps `working` and `blocked` to `current` `live`, `idle` to `current` `idle`, and `done` to `stale` `agent_done`, which prompts verification and a guarded finish.
- tmux: the provider hooks being added to the tmux workflow publish the agent session and state as pane user options.
  The adapter reads them with `tmux list-panes -a -F` and applies the same mapping.
- No matching pane on a reachable local host is `missing` `no_live_session`; it is never completion.
- A session owned by another host is `unknown` `remote_host` until executions record a host identity and the operator configures a read-only transport such as a saved Herdr machine.
  The adapter never substitutes this machine's panes for another host's.

The adapter reads state only.
It never sends keys, prompts, or commands to a pane.

### Provider sessions (designed, not shipped)

The provider adapter checks that a session reference path still exists and is a regular file owned by the current user.
It never opens, parses, or indexes session content, and it reports no modification times, because age thresholds are not deterministic.
An absent session file is `missing` `session_record_missing`, which is evidence for the operator, not completion.

## Durability

The akagent store is disposable.
Every kept field must also live in the operator's durable work record or in the live system it describes.
Today `task check` needs the cached task and resource bindings, so a lost store cannot yet be checked.
A later importer will rehydrate the cache from a versioned, committed binding block in the work record.
It must be idempotent, leave conflicting identities unresolved for operator review, and never overwrite live truth.
Until it ships, do not claim that akagent survives store loss.

## Trims

Safe in this rollout:

- Resource views omit `committed`, `dirty`, and `untracked` unless a legacy record set them, matching task views.
- Guidance stops recommending `--metadata delivery=...` and `--head`; agents record the binding and a PR URL hint, then run `task check`.
- Guidance recommends `task check` before completion and during recovery instead of hand triage.

Deferred to a versioned protocol migration, after active clients prove compatible:

- Removing legacy task Git fields, resource `head` and Git flags, and cleanup state fields from storage.
- Rejecting `--head` on resource creation.
- Retiring liveness observations on hosts where the terminal adapter is shipped.

Never removed: explicit task finish, guarded execution finish, external completion, and archive.
Archival is retention, not cleanup authorization.

## Rollout

1. This design, the Git and GitHub adapters, the record-consistency rules, and `task check`.
2. The safe trims above.
3. The terminal adapter for Herdr and tmux on the local host, after the tmux provider hooks ship.
4. Execution host identity and a read-only remote transport.
5. The provider session adapter.
6. The notes-backed importer, then the versioned migration that removes legacy fields.

Each phase is opt-in and read-only, so no feature flag is needed; existing commands and storage keep working.

## Non-goals

- Writing derived facts back into records, or auto-finishing work from a green check, merged PR, or missing pane.
- A daemon, poller, or background cache refresher.
- Reading provider transcripts or terminal scrollback.
- Adapters that mutate Git, the forge, a terminal, or a provider.
