# Architecture

## Thesis

Standardize task identity, durable records, typed observations, and recovery behavior without making unlike execution environments appear identical.
Issue [#140](https://github.com/akofink/akagent-cli/issues/140) shipped a local-first, record-only task boundary with a stable CLI protocol.
External tools and skills own side effects that remain necessary.

## Public surfaces

```text
akagent
akagent integration inspect
akagent id generate
akagent repository <register|list|inspect|update|unregister>
akagent task <create|external|checkpoint|disposition|list|inspect|publish|finish|archive|reconcile|resource|execution>
akagent update [--source <path>]
akagent worker inspect
```

The removed launch, attach, stop, deployment, cleanup, credential, provider, and transitional record commands fail with structured usage errors before store access.

## Components

### CLI layer

The CLI records repository, task, resource, execution, observation, checkpoint, disposition, archive, recovery, and delivery facts.
It emits concise TOON data and structured errors.
It provides only the read-only `AKAGENT_ENABLED` integration inspection signal and worker capability inspection.

### Record-only lifecycle

The lifecycle owns durable records, append-only events, revision checks, idempotency, locks, archives, recovery debt, and protocol views.
It accepts caller-declared observations and provider-neutral references.
It never launches or stops processes, inspects tmux or PIDs, runs Git, creates or removes worktrees, resolves credential values, parses provider sessions, captures terminal output, or calls a forge.

Repository registration stores an absolute path, policy, and optional worktree-root reference without checking or mutating the checkout.
Resource creation stores repository identity, branch, revisions, worktree references, and delivery metadata without host inspection.
Execution creation stores a tool-neutral attempt without starting it.
Publication, finish, archive, disposition, and reconciliation change records only.

### External interaction surfaces

Tmux, Git, process launchers, deployment tools, credential managers, providers, and forge clients are outside the core.
They may submit redaction-safe observations, session references, and delivery URLs through the CLI.
Their absence, a missing process, or a missing reference never proves completion.

### Durable records

The worker-local store is file based:

```text
$XDG_STATE_HOME/akagent/
  repositories/<name>.json
  tasks/<task-id>/manifest.json
  tasks/<task-id>/events/<sequence>.json
  tasks/<task-id>/resources/<resource-id>/manifest.json
  tasks/<task-id>/resources/<resource-id>/events/<sequence>.json
  tasks/<task-id>/executions/<execution-id>/manifest.json
  tasks/<task-id>/executions/<execution-id>/events/<sequence>.json
  tasks/<task-id>/archive.json
  locks/
```

The store uses typed JSON envelopes, atomic manifest replacement, append-only events, descriptor-safe traversal, and per-task locks.
Manifest replacement and audit append are separate writes, so the durability guarantee is bounded rather than crash-atomic across both files.
TOON remains the agent-facing output encoding.
Storage schema version `1` remains readable.

### Checkpoints and recovery

Checkpoints are bounded, provider-neutral handoffs with expected revisions and idempotency keys.
They preserve purpose-tagged context references, resource and session references, verification facts, uncertain external operations, and the next action.
Recovery is store-only and offline-safe.
External tools verify uncertain process, Git, worktree, provider, or forge operations before replaying them.
No recovery path launches a replacement process or creates a duplicate task or worktree.

## Failure assumptions

Task records survive loss of the operator process and terminal attachment when the worker filesystem survives.
Uncommitted work survives ordinary record operations because the core does not touch checkouts.
A same-machine reboot can recover durable intent, observations, and checkpoint references, but an external tool must verify any replacement process.
Disk loss and cross-machine synchronization are outside the local store.
Provider session recovery depends on provider-owned references.

Terminal disconnects, stale PIDs, PID reuse, duplicate starts, partial setup, disk exhaustion, credential expiration, cleanup races, and contradictory observations are expected conditions.
The core preserves uncertainty and recovery debt rather than inferring success.

## Non-goals

- Automatic worker placement.
- Multi-tenant security isolation.
- A web dashboard.
- Cross-machine synchronization in the local store.
- Exact process restoration after reboot.
- Provider transcript indexing or private context persistence.
- A launch, cleanup, credential, or provider-parity adapter in the core.
