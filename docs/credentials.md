# Credential model

## Current local behavior

The machine invoking `akagent` is the initial source of credential readiness information.
The current local CLI validates named requirements and propagates no credential values through task output or command arguments.

Missing optional credentials produce warnings.
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

The current local task implementation records named requirements and readiness warnings in the task manifest.
It does not implement remote credential transfer or task-scoped credential installation.

## Future propagation

Remote propagation is deferred until authenticated worker execution exists.
Future implementations must:

- Select capabilities per task or worker instead of copying the full local collection.
- Transfer through stdin or a protected file channel, never command arguments.
- Install atomically into restrictive directories and files.
- Track non-secret source identity, scope, fingerprint, expiration, and cleanup state.
- Remove or refresh credentials according to explicit lifetime policy.
- Preserve cleanup debt when transfer succeeds but launch or removal fails.

Dedicated agent SSH, GitHub, and LLM credentials are preferred over copied primary human credentials.
Git signing should use a scoped signing subkey rather than a primary secret key.

## Deferred classes

Work-specific credentials, browser sessions, remote providers, cloud roles, deployment credentials, and shared worker credential reference counting require a separate design.

## Failure modes

The protocol must remain understandable when a required capability is missing, a token expires, a source changes, cleanup is retried, or an integration attempts to use an unavailable provider.

The integration gate does not grant credentials.
It only controls whether automated integrations may invoke automated `akagent` behavior.
