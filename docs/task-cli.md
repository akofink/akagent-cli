# Task CLI contract

The task CLI is a local-first durable registry boundary.
Task, resource, execution, repository, checkpoint, disposition, observation, archive, and delivery operations are record-only.
The record lifecycle never invokes Git, tmux, a provider, a credential resolver, a deployment executable, or a terminal reader.
The opt-in `task check` command invokes read-only Git and GitHub adapters outside the record lifecycle and never writes to the store.

Protocol data and structured errors are written to stdout as TOON by default.
`task list` and `task inspect` also accept `--format json` for compact JSON of the same typed views and `--format human` for deterministic terminal reading.
JSON is an explicit interchange option and does not change the default protocol.
Structured errors remain TOON even when JSON is requested.
Exit code `0` means success or an idempotent no-op.
Exit code `1` means the requested operation could not be completed.
Exit code `2` means the command or its arguments are invalid.

## Public commands

```text
akagent repository <register|list|inspect|update|unregister>
akagent task create --title <title> [--task-id <id>] [--repository <name>] [--branch <branch>] [--base <revision>] [--worktree <path>] [--require <credential>] [--optional <credential>]
akagent task external create <task-id> --title <title> --caller-id <id> --operation-id <id>
akagent task external finish <task-id> --caller-id <id> --operation-id <id> --expected-revision <revision> --contract <name> --result <result>
akagent task external archive <task-id> --caller-id <id> --operation-id <id> --expected-revision <revision>
akagent task checkpoint <write|inspect> <task-id> ...
akagent task disposition <task-id> <in-flight|deferred|terminal> --reason <reason> [--expected-revision <revision>]
akagent task list [keyword] [--view <in-flight|attention|maintenance|deferred|history>] [--all] [--repository <name>] [--worktree <path>] [--format <toon|human|json>]
akagent task inspect <task-id|keyword> [--format <toon|human|json>]
akagent task check <task-id|keyword|--all> [--offline] [--format <toon|json>]
akagent task publish <task-id> --condition <condition> [--reason <reason>] [--activity <activity>]
akagent task finish <task-id> <succeeded|failed> <result>
akagent task archive <task-id>
akagent task reconcile [<task-id>]
akagent task resource <create|list|inspect|update|archive> ...
akagent task external resource create <task-id> --resource-id <id> --repository <name> --branch <branch> --base <revision> --head <revision> --worktree <absolute-path> --caller-id <id> --operation-id <id>
akagent task external resource archive <task-id> <resource-id> --caller-id <id> --operation-id <id> --expected-revision <revision>
akagent task execution <create|observe|finish|handoff|list|inspect|session|evidence|publish|archive|reconcile> ...
akagent task external execution create <task-id> --execution-id <id> --resource <resource-id> --caller-id <id> --operation-id <id> [--predecessor <execution-id>] [--tool <tool> --session-id <id> [--reference-path <absolute-path>]]
akagent task external execution archive <task-id> <execution-id> --caller-id <id> --operation-id <id> --expected-revision <revision>
akagent update [--source <path>]
akagent worker inspect
```

The former launch, attach, stop, deploy, clean, credential, integration, and `task record` command families are removed.
Recognized removed commands return a structured usage error with exit code `2` before opening or mutating the state store.
The error names only the removed command family and provides safe migration guidance.

## Derived checks

```text
akagent task check <task-id|keyword> [--offline] [--format <toon|json>]
akagent task check --all [--offline] [--format <toon|json>]
```

`task check` compares cached records with live Git and GitHub state and reports findings; it never writes to the store, a checkout, or the forge.
Each finding has a `surface`, a `state` of `current`, `stale`, `missing`, or `unknown`, a stable `code`, an optional redaction-safe `detail`, and a deterministic `action`.
Single-task output has `task_id`, a `summary` with counts per state and `needs_action`, then `task`, `resources`, and `executions` findings.
Resource findings cover `worktree`, `branch`, `pull_request`, and `checks` in that order; resources and executions are sorted by ID.
Record-consistency findings, such as an open execution under a terminal task, need only the store.
`--offline` skips Git and GitHub and reports those surfaces as `unknown` `offline`.
`--all` checks every task, including archived tasks, runs live adapters only for tasks that are not terminal, and lists only tasks and findings that need action under `checked`, `summary`, and `tasks`.
A finding needs action when its state is `stale` or `missing`.
Exit code `0` means the check ran even when findings need action; gate on `summary.needs_action`.
A missing task returns exit code `1`, and invalid arguments return exit code `2`.
A `current` finding is not a delivery verdict, and explicit completion remains required.
See [Derived state](derived-state.md) for every code and the adapter contract.

## Repository records

```text
akagent repository register <name> <path> [--policy <worktree|direct>] [--worktree-root <absolute-path>]
akagent repository list
akagent repository inspect <name>
akagent repository update <name> [--path <path>] [--policy <worktree|direct>] [--worktree-root <absolute-path>]
akagent repository unregister <name>
```

Registration records the repository path, policy, and optional worktree root without checking or changing the checkout.
The path is stored as an absolute reference.
External tools own checkout validation, branch creation, worktree creation, and cleanup.
Unregister removes only the registration record and refuses while durable tasks reference the repository.

## Task and resource records

`task create` records task intent without Git, filesystem, process, or tmux access.
The optional repository, branch, base, and worktree values are recorded as declared facts.
A repository value also creates a durable `legacy` resource snapshot without inspecting the path.
`--require` and `--optional` store opaque historical credential IDs and never resolve, validate, or print credential values.

```text
akagent task create --title <title> [--task-id <id>] [--repository <name>] [--branch <branch>] [--base <revision>] [--worktree <path>]
akagent task resource create <task-id> --repository <name> [--resource-id <id>] [--branch <branch>] [--base <revision>] [--head <revision>] [--worktree <path>] [--metadata <key=value>] [--external-url <https-url>]
akagent task resource list <task-id>
akagent task resource inspect <task-id> [<resource-id>]
akagent task resource update <task-id> <resource-id> [--metadata <key=value>] [--external-url <https-url>]
akagent task resource archive <task-id> <resource-id>
```

Resource creation records repository identity, branch, base, head, worktree references, delivery metadata, and recovery facts.
It does not resolve a repository registration or change the filesystem.
Resource Git facts are caller-declared or historical.
A resource archive contains the durable resource and event history without terminal capture or Git inspection.

## Explicit external records

The `task external` family is the public creation boundary for caller-owned records.
It creates external provenance directly and never relabels a managed task, resource, or execution.
The task ID, resource ID, execution ID, caller ID, and operation ID are bounded non-secret identifiers.
Resource creation additionally requires repository, branch, base, head, and an absolute worktree reference.
The referenced worktree does not need to exist and is never inspected.
External execution creation requires an external resource belonging to the same caller.

Equivalent create requests are idempotent and return the acknowledged record.
Reusing an operation ID with different inputs, using another caller, targeting a managed record, or creating beneath a terminal record returns a structured conflict.
Malformed input is rejected with exit code `2` before durable record creation.
Successful commands return the normal task, resource, or execution detail view with provenance and revision fields.

The external finish commands require the owning caller, a new operation ID, the current revision, a named completion contract, and a result.
Equivalent finish retries are idempotent.
Stale revisions, changed operation inputs, terminal mutations, and managed executions are rejected without changing the record.
That rejection applies to the `task external` finish and archive commands.
`task execution finish` is the separate close path for a managed execution created by `task execution create`, including `--target external`.
External archive commands require explicit completion for tasks and explicit completion for executions.
Resource archives require only the owning caller and current resource revision.
Archive retries preserve the existing archive and never inspect a process, terminal, provider, Git checkout, credential, or deployment.

The normal `task create`, `task resource create`, and `task execution create` commands retain managed provenance semantics.
In particular, `--target external` on the normal execution command does not create an external execution.
The removed `task record` family remains a migration error and is not an alias for `task external`.

## Execution records

```text
akagent task execution create <task-id> --target <target> [--execution-id <id>] [--label <label>] [--command <command>] [--resource <resource-id>] [--worktree <path>]
akagent task execution observe <task-id> <execution-id> --caller-id <id> --operation-id <id> --expected-revision <revision> --source <source> --observed-at <RFC3339> --host-id <id> --boot-id <id> [--process-state <state>] [--result <result>] [--detail <text>]
akagent task execution finish <task-id> <execution-id> --caller-id <id> --operation-id <id> --expected-revision <revision> --contract <name> --result <result>
akagent task execution handoff <task-id> <predecessor-id> --successor-execution <id> --operation-id <id> --expected-revision <revision> --takeover-verified --predecessor-closed
akagent task execution list <task-id>
akagent task execution inspect <task-id> [<execution-id>]
akagent task execution session add <task-id> <execution-id> --tool <tool> --session-id <id> [--reference-path <path>]
akagent task execution evidence <list|inspect> <task-id> <execution-id> [<capture-id>]
akagent task execution publish <task-id> <execution-id> --condition <condition> [--reason <reason>] [--activity <activity>]
akagent task execution archive <task-id> <execution-id>
akagent task execution reconcile <task-id>
```

Execution creation records an optional tool-neutral attempt without starting a process.
The command and target are durable metadata, not instructions to execute a program.
Use `task inspect`, `task execution inspect`, or `task execution list` to read an execution's current `revision` before guarded observation or completion.
Those reads include revision `0`.
Human `task inspect` output includes the same revision.
`--target external` on this managed command does not create an external execution.
An execution can select one resource while coordinating other resources through the task ID.
External callers can append typed provenance with `task execution observe`.
Observation writes require `--caller-id`, `--operation-id`, and the current `--expected-revision`, plus source, timestamp, host, and boot provenance.
Observations remain historical and do not change execution lifecycle state.
External callers can finish an attempt with `task execution finish` by naming the completion contract and supplying the current revision.
The same command closes a managed execution, including one whose revision is `0`, before or after the parent task is terminal.
That close preserves managed provenance and does not infer completion.
Handoff remains the successor-takeover path and is not required before finish.
`task execution reconcile` does not close executions.
Equivalent operation retries return the existing record without changing its revision.
A reused operation ID with different inputs, a stale revision, a different caller, or a terminal mutation returns a structured conflict.
Execution archive contains the durable execution and event history without terminal capture or process inspection.

`task execution handoff` is a separate successor-authorized disposition for managed executions.
The predecessor must remain managed, be published with condition `waiting` and activity `handed off`, and match the supplied revision.
The named successor must be a distinct active managed execution on the same nonterminal task.
The caller must explicitly affirm both independently verified takeover and predecessor closure.
The record-only core does not inspect tmux or prove those external facts, so do not use the flags without an independently verified receipt.
The operation preserves managed provenance, records the successor execution ID, increments the predecessor revision, and terminalizes its record with result `handed_off`.
It does not claim that the predecessor process exited successfully.
Operation IDs provide idempotent retries, and stale revisions, inactive successors, incomplete attestations, external executions, and changed terminal mutations are rejected.
Managed executions may have revision `0`; supply the current value returned by inspection.

A provider or external tool may record non-secret session provenance.
The core validates only the reference shape and never opens or parses the provider file.
Evidence commands inspect only local path metadata and never read provider content.
Missing references remain historical evidence and do not prove completion.

## State and completion

```text
akagent task publish <task-id> --condition <active|waiting|blocked|failed|none> [--reason <reason>] [--activity <activity>]
akagent task execution publish <task-id> <execution-id> --condition <active|waiting|blocked|failed|none> [--reason <reason>] [--activity <activity>]
akagent task finish <task-id> <succeeded|failed> <result>
akagent task disposition <task-id> <in-flight|deferred|terminal> --reason <reason> [--expected-revision <revision>]
```

Publication updates durable conditions and heartbeats only.
It does not synchronize process, tmux, Git, credential, or provider state.
Finish records explicit task completion and marks the task terminal without inferring success from a missing process.
A repeated equivalent legacy finish is a successful no-op, but a different terminal result cannot overwrite the existing record without the revision-checked external completion contract.
The record preserves unknown, stale, missing, and contradictory observations.
A missing process never triggers completion or implicit reactivation.

## Checkpoints and recovery

```text
akagent task checkpoint write <task-id> --idempotency-key <key> --expected-revision <revision> --task-kind <kind> --completion-contract <contract> --next-action <action> ...
akagent task checkpoint inspect <task-id>
akagent task reconcile [<task-id>]
```

Checkpoints are bounded, provider-neutral handoffs with revision guards and idempotency keys.
They preserve purpose-tagged context references, resource and session references, verification facts, and uncertain external operations.
Recovery is store-only and offline-safe.
External tools must verify uncertain process, Git, worktree, provider, or forge operations before replaying them.
See [recovery-checkpoints.md](recovery-checkpoints.md) for the complete checkpoint contract.

Legacy unfinished and stopped tasks use explicit store-only migration or adoption.
Task and resource IDs and historical Git and session facts are preserved.
The core does not create duplicate tasks or worktrees and does not reactivate work implicitly.
Legacy manifests, archives, events, credential metadata, and storage schema version `1` remain readable.

## Inventory and delivery

The default `task list` view is `in-flight`.
Use `--view attention`, `--view maintenance`, `--view deferred`, or `--view history` for deterministic inventory slices.
Use `--all` to include every durable record.
Keyword matching applies only to task titles and task or resource branches.
Repository and worktree filters are exact matches and compose with keyword and view filters.
Results are sorted by task ID and include a definitive total.

Agents use external Git and forge tools for commits, pull requests, and merges.
They may record provider-neutral delivery URLs with `task resource update`.
The core does not call GitHub, Bitbucket, or another forge.

## Worker and compatibility protocol

`worker inspect` reports worker protocol version `2`, the binary's `build_revision`, and declarative capabilities for `registry`, `checkpoint`, `observation`, and `handoff`.
The revision is the full Git commit used to build the binary, or `unknown` for builds without revision metadata.
Build with `go build -ldflags "-X=github.com/akofink/akagent-cli/internal/app.buildRevision=$(git rev-parse HEAD)" -o akagent ./cmd/akagent` to embed the current commit.
`akagent update` applies the same build flag automatically and bounds Git and Go build commands to five minutes.
A fetch timeout is reported as retryable.
It does not scan for Git, tmux, providers, credentials, or worktrees as prerequisites.
Storage schema version `1` remains readable.
Removing command families and changing lifecycle meanings is a protocol-breaking change documented by version `2`.
