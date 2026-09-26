# Protocol

## Scope

The protocol is the stable contract between direct commands and durable local records.
It covers identity, record-only lifecycle semantics, typed observations, output, errors, compatibility, inventory, and recovery.
It does not standardize process execution, Git, tmux, credentials, provider sessions, or forge behavior.

## Worker

```toon
worker:
  id: local
  protocol_version: 2
  architecture: arm64
  operating_system: linux
  features[4]: registry,checkpoint,observation,handoff
```

Worker inspection is declarative.
It does not scan for Git, tmux, providers, credentials, worktrees, or executables.
Capabilities describe retained record operations, not host-side orchestration.

## Records

A task has an immutable ID, title, durable condition, result, events, observations, checkpoints, dispositions, and zero or more resources and executions.
A resource stores repository identity, branch, base and head revisions, worktree reference, Git facts, recovery debt, archive state, delivery metadata, and historical credential metadata.
An execution stores a tool-neutral target, command metadata, optional resource ID, lifecycle facts, observations, recovery state, and provider-neutral session references.

All paths, process facts, Git facts, provider references, and delivery URLs are caller-declared or historical facts.
The core does not inspect their targets.
A session reference contains a tool identifier, session ID, and optional absolute reference path.
The path is provider-owned state and is never opened or parsed.

Task and resource IDs remain stable across migration.
UUIDv7 is used when the caller does not provide an ID.
Titles, branches, labels, paths, and session IDs are attributes, not identity.

## State model

Lifecycle phases remain readable as:

```text
created | starting | running | stopped | finished
```

New record-only work is created in `created` and completed as `finished`.
`starting`, `running`, and `stopped` remain readable historical values from legacy orchestration records.
Derived `status` reports `active` for record-only work whose lifecycle is `created` and whose published condition is `active`.
Task-level `committed`, `dirty`, and `untracked` are legacy facts with no v2 setter and are omitted from task views unless set; resource views keep their Git facts.

Agent conditions are:

```text
active | waiting | blocked | failed | none
```

The core records conditions and observations independently.
It does not derive completion from process absence, archive state, age, or provider data.
Unknown, stale, unavailable, and contradictory observations remain visible.
A caller must explicitly record a finish result.
External execution observations are historical and require the owning caller and current execution revision.
External execution completion requires the owning caller, a unique operation ID, a named completion contract, and the current execution revision.
Inspection always includes that revision, including `0`.
A managed execution created with `task execution create` can use the same finish command before or after its parent task is terminal.
That close preserves managed provenance, records the named contract, and does not infer completion from process absence.
Observation remains limited to externally declared records.
A separate successor-authorized handoff disposition applies only to managed executions published `waiting` / `handed off`.
It requires a distinct active managed successor on the same nonterminal task, the current predecessor revision, a unique operation ID, and caller attestations of independently verified takeover and predecessor closure.
The core records the successor ID and `handed_off` result without changing provenance or claiming process success.
The core cannot verify the external attestations or inspect tmux.
Repeated equivalent operations are idempotent, while stale revisions, changed operation inputs, and terminal mutations are conflicts.

## Record-only commands

```text
akagent id generate
akagent repository <register|list|inspect|update|unregister>
akagent task create ...
akagent task external <create|finish|resource|execution|archive> ...
akagent task checkpoint <write|inspect> ...
akagent task disposition ...
akagent task list ...
akagent task inspect ...
akagent task publish ...
akagent task finish ...
akagent task archive ...
akagent task reconcile ...
akagent task resource <create|list|inspect|update|archive> ...
akagent task execution <create|observe|finish|handoff|list|inspect|session|evidence|publish|archive|reconcile> ...
akagent update [--source <path>]
akagent worker inspect
```

These commands record durable facts only.
They never invoke Git, tmux, a provider, a credential resolver, a deployment executable, or a terminal reader.
Create operations are idempotent for equivalent inputs and return a conflict for different immutable inputs.
The explicit external creation family validates caller ownership and immutable inputs before mutation.
Managed creation commands retain managed provenance, including a normal execution created with `--target external`.
Finishing that execution after the parent task is terminal does not relabel it.

The former launch, attach, stop, deploy, clean, credential, integration, provider orchestration, and transitional `task record` commands are removed.
Recognized removed forms return the structured usage error contract with exit code `2` before opening or mutating the state store.
Migration guidance names only the command family and never echoes sensitive input.

## Checkpoints and dispositions

A checkpoint is a bounded, provider-neutral recovery handoff.
It includes an idempotency key, expected task revision, task kind, completion contract, next action, purpose-tagged context references, resource and session references, verification facts, and uncertain external operations.
Checkpoint writes are revision checked and idempotent.
They do not execute the next action.

A disposition records whether accepted work is `in-flight`, `deferred`, or `terminal`, with a required reason and optional expected revision.
Disposition transitions change records only.
They never alter process, Git, worktree, archive, cleanup, or provider state.
Revision-scoped audit identity prevents an earlier event from satisfying a later repair after an A to B to A transition.

## Inventory

The default list view is `in-flight` and includes accepted unfinished work.
Explicit views include `attention`, `maintenance`, `deferred`, and `history`.
`--all` includes every durable record.
Keyword matching is limited to task titles and task or resource branches.
Repository and worktree filters are exact matches.
Results are deterministic and include a definitive total.
Archive, cleanup state, process absence, and reconciliation never hide accepted unfinished work from the in-flight view.

## Recovery and compatibility

Reconciliation is store-only and offline-safe.
It repairs durable artifacts and preserves caller-submitted observations.
It never launches a process, creates a duplicate task or worktree, inspects a provider file, changes Git, or infers completion.

Legacy manifests, archives, events, credential metadata, recovery debt, and storage schema version `1` remain readable.
Legacy unfinished and stopped work requires explicit store-only adoption or migration.
The migration preserves historical IDs and Git and session facts without implicit reactivation.
Missing processes never reactivate or complete work.

Adding optional observation metadata is compatible.
External records use the existing schema-1 readable envelopes and add no host-side effects.
Changing lifecycle meanings or removing record fields requires a later breaking protocol version.

## Output and security

Protocol data and errors are TOON on stdout.
`task list` and `task inspect` also accept `--format json` for compact JSON of the same typed views and `--format human` for terminal presentation.
JSON is an explicit interchange option and does not change the default protocol.
Structured errors remain TOON even when JSON is requested.
Human task views are an explicitly selected presentation and are not a parsing interface.
Diagnostics are not mixed into protocol output.

Credential values, prompt contents, provider session contents, environment values, terminal output, and secret command arguments never enter records, archives, output, errors, logs, references, or diagnostics.
External URLs and session references are non-secret metadata.
