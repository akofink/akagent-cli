# Credential model

## Current local behavior

The machine invoking `akagent` is the initial source of credential readiness information.
The current local CLI validates named requirements and propagates no credential values through task output, task records, or command arguments.
The optional Pi execution integration may inject a requested `env:` credential into its managed process after readiness checks pass.
File credentials are readiness-only and cannot be injected into the managed environment.

Missing optional credentials produce non-secret warnings and are not injected.
Missing required credentials prevent a task that requests them from starting.

A deployment can declare work-scoped credential IDs independently from the task requirements.
The IDs and readiness state are durable metadata, while secret values remain process-local.
Deployment requirements are checked again immediately before launch so rotation or removal is observed safely.
Only ready `env:` entries can be injected into a local deployment command; file entries remain readiness-only.

The current supported inspection commands are:

```text
akagent credential list
akagent credential inspect <id>
akagent credential doctor
```

Task requirements use named IDs:

```text
akagent task create --title <title> --repository <name> --require <credential>
akagent task create --title <title> --repository <name> --optional <credential>
```

## Local layout

Use XDG paths rather than one opaque home-directory file:

```text
$XDG_CONFIG_HOME/akagent/credentials.toon
$XDG_DATA_HOME/akagent/credentials/
```

When the XDG variables are unset, the platform's standard user directories are used.
The manifest contains source references and policy, never secret values.

```toon
credentials[3]{id,type,source,required_for}:
  git-ssh,ssh_key,file:<path>,git
  github,api_token,env:GITHUB_TOKEN,github
  llm,api_token,env:OPENAI_API_KEY,
```

A blank `required_for` marks an optional credential.
The supported manifest schema version is `1`.
Duplicate IDs and newer versions are rejected.

## Source checks

The current providers are metadata-only `file:` and presence-only `env:` checks.

A `file:` source is checked for existence, ownership, and mode without opening the file.
Credential files must be exactly mode `0600` in an exactly mode `0700` directory owned by the current user on platforms with Unix permission semantics.
Symlinks and unsafe special permission bits are rejected.

An `env:` source is checked for presence without persisting or printing its value.

Set `AKAGENT_CREDENTIALS` to use a non-default manifest path.

## Output and redaction

Credential output reports identity, source kind, readiness, and non-secret warnings.
It never reports credential values.

Credential values must not appear in TOON output, errors, logs, events, prompts, tmux commands, process arguments, or diagnostics.
Credential cleanup is task-scoped and independent from Git worktree cleanup.
It requires explicit `--allow-credentials` authorization through `akagent credential clean <task-id>`, `akagent task credential clean <task-id>`, or `task clean`.
The cleanup lifecycle records `pending`, `blocked`, `partial`, and `complete` state in the durable task manifest and records redaction-safe events for refusal, retry, and success.
A failed hook leaves the task and credential manifest intact and can be retried without repeating worktree cleanup.
The core credential package does not read credential values or define provider-specific deletion behavior.

The current task implementation records named requirements and readiness warnings in the task manifest.
For the optional Pi integration, the integration worker constructs a minimal environment from safe runtime variables, adds the non-secret `AKAGENT_TASK_ID` and `AKAGENT_EXECUTION_ID` context, and injects only requested, ready `env:` credentials.
It excludes ambient variables outside the safe runtime allowlist and filters credential-like names unless they were explicitly requested.
Optional credentials are never injected.

Dedicated agent SSH, GitHub, and LLM credentials are preferred over copied primary human credentials.
Git signing should use a scoped signing subkey rather than a primary secret key.

## Failure modes

The protocol must remain understandable when a required capability is missing, a token expires, a source changes, cleanup is retried, or an integration attempts to use an unavailable provider.

The integration gate does not grant credentials.
Credential cleanup authorization does not grant, print, or rotate credentials.
It only controls whether automated integrations may invoke automated `akagent` behavior.

Local deployments use a worker command that receives only task and execution IDs.
The worker resolves the deployment record, builds the minimal environment, and never places credential values in tmux metadata, command arguments, durable records, or protocol output.
Deployment commands are responsible for avoiding secret values in their own output.
