# Task CLI contract

The task CLI is a local-first durable registry boundary.
Task, resource, execution, repository, checkpoint, disposition, observation, archive, and delivery operations are record-only.
The core never invokes Git, tmux, a provider, a credential resolver, a deployment executable, or a terminal reader.

Protocol data and structured errors are written to stdout as TOON by default.
`task list` and `task inspect` also accept `--format human` for deterministic terminal reading.
Exit code `0` means success or an idempotent no-op.
Exit code `1` means the requested operation could not be completed.
Exit code `2` means the command or its arguments are invalid.

## Public commands

```text
akagent integration inspect
akagent repository <register|list|inspect|update|unregister>
akagent task create --title <title> [--task-id <id>] [--repository <name>] [--branch <branch>] [--base <revision>] [--worktree <path>] [--require <credential>] [--optional <credential>]
akagent task checkpoint <write|inspect> <task-id> ...
akagent task disposition <task-id> <in-flight|deferred|terminal> --reason <reason> [--expected-revision <revision>]
akagent task list [keyword] [--view <in-flight|attention|maintenance|deferred|history>] [--all] [--repository <name>] [--worktree <path>] [--format <toon|human>]
akagent task inspect <task-id|keyword> [--format <toon|human>]
akagent task publish <task-id> --condition <condition> [--reason <reason>] [--activity <activity>]
akagent task finish <task-id> <succeeded|failed> <result>
akagent task archive <task-id>
akagent task reconcile [<task-id>]
akagent task resource <create|list|inspect|update|archive> ...
akagent task execution <create|list|inspect|session|evidence|publish|archive|reconcile> ...
akagent update [--source <path>]
akagent worker inspect
```

The former launch, attach, stop, deploy, clean, credential, and `task record` command families are removed.
Recognized removed commands return a structured usage error with exit code `2` before opening or mutating the state store.
The error names only the removed command family and provides safe migration guidance.

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

`task create` records task intent without credentials, Git, filesystem, process, or tmux access.
The optional repository, branch, base, and worktree values are recorded as declared facts.
A repository value also creates a durable `legacy` resource snapshot without inspecting the path.

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

## Execution records

```text
akagent task execution create <task-id> --target <target> [--execution-id <id>] [--label <label>] [--command <command>] [--resource <resource-id>] [--worktree <path>]
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
An execution can select one resource while coordinating other resources through the task ID.
Execution archive contains the durable execution and event history without terminal capture or process inspection.

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

`worker inspect` reports worker protocol version `2` and declarative capabilities for `registry`, `checkpoint`, and `observation`.
It does not scan for Git, tmux, providers, credentials, or worktrees as prerequisites.
Storage schema version `1` remains readable.
Removing command families and changing lifecycle meanings is a protocol-breaking change documented by version `2`.
