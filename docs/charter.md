# Durable registry charter

**Status:** Shipped record-only protocol-v2 boundary after issue [#140](https://github.com/akofink/akagent-cli/issues/140).

## Recommendation

`akagent` is a narrow, local-first protocol for durable task, resource, and execution records.
It records intent, typed observations, conditions, recovery debt, checkpoints, dispositions, archives, and delivery references through the CLI.

The core does not own process orchestration.
External tools and skills own process launch and stop, tmux interaction, Git and worktree mutation, provider sessions, credential handling, cleanup, deployment, and forge operations when those capabilities remain needed.
Capabilities may be intentionally retired instead of receiving a feature-parity adapter.

## Shipped boundary

| Surface | Boundary |
| --- | --- |
| Task, resource, execution, repository, event, archive, checkpoint, disposition, observation, recovery, and delivery records | Retained in the core |
| Provider-neutral session references and metadata-only evidence | Retained in the core; provider interpretation stays external |
| Inspection, list, publication, finish, archive, reconciliation, and migration | Record-only and offline-safe |
| Git, worktree, process, tmux, provider, credential, deployment, cleanup, and forge side effects | External tools and skills |
| Launch, attach, stop, deploy, clean, credential, integration, provider orchestration, and `task record` command families | Removed; structured usage refusal before store access |
| Worker inspection | Retained declarative protocol version `2` and capabilities |

The CLI never treats a missing process, terminal, checkout, provider session, credential, or network as proof of completion.
Legacy manifests, archives, event histories, credential metadata, and storage schema version `1` remain readable.

## Ownership rules

The stable boundary is the CLI and its typed record schemas, not an exported Go package graph.

- The protocol owns IDs, records, transitions, conditions, observations, recovery debt, references, archives, and structured errors.
- The store owns restrictive paths, envelopes, atomic manifest replacement, append-only events, locks, archives, and store recovery.
- External tools and skills own host observations, process lifecycle, Git and worktree operations, credentials, provider interpretation, cleanup, and delivery.
- The core accepts redaction-safe caller facts and preserves uncertainty without inferring success.
- Credential values, prompt content, provider session content, terminal output, and secret arguments never enter records, output, logs, or diagnostics.

## Recovery and compatibility

Record creation precedes any external side effect when a workflow uses one.
The core itself performs no side effect and no recovery replay.
Checkpoints use expected revisions and idempotency keys.
Dispositions are record-only and revision-scoped.
Legacy unfinished and stopped work requires explicit store-only adoption or migration.
Recovery never creates duplicate tasks, resources, executions, branches, or worktrees.

Manifest replacement and audit append are separate writes.
The durability claim is bounded and does not promise crash-atomicity across both files.

Protocol version `2` covers the breaking removal of orchestration and credential command semantics.
Storage schema version `1` remains readable.
Optional observation metadata may be added compatibly, while lifecycle meaning or record-field removal requires a later protocol version.

## Verification gates

The boundary is verified by offline core tests, subprocess canaries for Git, tmux, providers, Pi, and deployment tools, concurrency and race tests, protocol output tests, legacy readability tests, and structured refusal tests.

The core-boundary exit criteria are met.
Before installed-binary changes, verify the integrated CLI, lifecycle skill, repository instructions, and active-agent workflow against `main`.
