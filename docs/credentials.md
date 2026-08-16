# Credential model

## Current local behavior

The machine invoking `akagent` is the initial source of credential readiness information.
The current local CLI validates named requirements and propagates no credential values through task output, task records, or command arguments.
The optional Pi execution integration may inject a requested `env:` credential into its managed process after readiness checks pass.
File credentials are readiness-only and cannot be injected into the managed environment.

Missing optional credentials produce non-secret warnings and are not injected.
Missing required credentials prevent a task that requests them from starting.

The current supported inspection commands are:

```text
akagent credential list
akagent credential inspect <id>
akagent credential doctor
```

Task requirements use named IDs:

```text
akagent task start --title <title> --repository <name> --require <credential>
akagent task start --title <title> --repository <name> --optional <credential>
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

The current task implementation records named requirements and readiness warnings in the task manifest.
For the optional Pi integration, the integration worker constructs a minimal environment from safe runtime variables, adds `AKAGENT_TASK_ID`, and injects only requested, ready `env:` credentials.
It excludes ambient variables outside the safe runtime allowlist and filters credential-like names unless they were explicitly requested.
Optional credentials are never injected.

Dedicated agent SSH, GitHub, and LLM credentials are preferred over copied primary human credentials.
Git signing should use a scoped signing subkey rather than a primary secret key.

## Failure modes

The protocol must remain understandable when a required capability is missing, a token expires, a source changes, cleanup is retried, or an integration attempts to use an unavailable provider.

The integration gate does not grant credentials.
It only controls whether automated integrations may invoke automated `akagent` behavior.
