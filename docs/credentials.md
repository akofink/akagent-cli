# Credential model

## Goals

The machine invoking `akagent` is the initial source of credentials.
`akagent` validates requested capabilities and propagates only what the selected task or worker needs.

Missing optional credentials produce warnings.
Missing required credentials prevent startup unless the caller explicitly changes the requirement.

Managed agent processes start from a minimal environment allowlist.
They do not inherit the invoking process's complete environment, and a secret environment variable is injected only when the task requested its mapped capability.

The design must not require a central secret service for local operation.
External providers can be added without changing task capability semantics.

## Local layout

Use XDG paths rather than one opaque home-directory file:

```text
~/.config/akagent/credentials.toon
~/.local/share/akagent/credentials/
```

`credentials.toon` contains source references and policy, never secret values.
The data directory contains dedicated files only when a credential manager or external provider is not used.

```toon
credentials[4]{id,type,source,required_for}:
  git-ssh,ssh_key,file:~/.local/share/akagent/credentials/git_ed25519,git
  git-signing,gpg_key,gpg:AGENT_SUBKEY_ID,commit
  github,api_token,file:~/.local/share/akagent/credentials/github,github
  openai,api_token,env:OPENAI_API_KEY,llm
```

The implementation must validate directory ownership and mode `0700` and secret-file mode `0600` on Unix-like systems.

## Source providers

Initial providers:

- `file:` reads a dedicated local credential file.
- `env:` reads an existing environment variable without persisting its value.
- `gpg:` exports a designated secret signing subkey.
- `command:` invokes a configured credential-manager command with no shell interpolation.

Later providers may include AWS Secrets Manager, Systems Manager Parameter Store, 1Password, or another broker.
Provider-specific behavior must not leak into task definitions.

## Commands

Candidate commands are:

```text
akagent credential list
akagent credential inspect <id>
akagent credential doctor
akagent credential install <id> --worker <worker>
akagent credential remove <id> --worker <worker>
```

Default output reports identity, source type, readiness, scope, and expiration without values:

```toon
credentials[3]{id,status,source}:
  git-ssh,ready,file
  git-signing,ready,gpg
  github,missing,file
warnings[1]: GitHub mutation commands will be unavailable
```

## Task requirements

Tasks request named capabilities rather than paths or environment variable names:

```text
git-read
git-write
git-sign
github-read
github-mutate
llm-openai
llm-anthropic
```

Worker policy resolves a capability to a credential ID and installation method.
Read and mutation permissions remain separate where providers support that distinction.

## Propagation

Remote propagation happens only after an authenticated worker connection and requirement validation.

Secrets:

- Travel through stdin or a protected file-transfer channel, never command arguments.
- Are selected per task or worker instead of copying the entire local collection.
- Are installed atomically into mode-`0700` directories and mode-`0600` files.
- Never appear in TOON output, errors, logs, events, prompts, tmux commands, process arguments, or diagnostics.
- Carry non-secret source identity, scope, fingerprint, expiration, and ownership metadata.
- Are removed or refreshed according to explicit lifetime policy.

If transfer succeeds and launch fails, reconciliation must identify and remove or retain the credential according to policy.

Environment construction is part of propagation even for local tasks.
Safe runtime variables such as locale, terminal, home, and selected tool paths are copied from an explicit allowlist.
Credential-shaped variables and all variables mapped by the credential manifest are removed before requested capabilities are injected.
An integration cannot bypass this filtering by launching the managed agent through an intermediate shell.

## Lifetimes

Every installed capability has one lifetime:

- `task` is installed for one task and removed during cleanup.
- `session` is available only while an operator connection remains active.
- `worker` is deliberately retained on a named durable worker.
- `external` is never copied because the worker obtains it from an instance role or broker.

The default is `task`.
Durable worker retention requires explicit policy.

Cleanup ownership must prevent one task from removing a shared credential still used by another task.
This can use exclusive destinations initially and reference counting only when shared worker credentials become necessary.

## SSH

Use dedicated agent SSH keys rather than the operator's main private key.
Separate keys by trust domain, register recognizable names, inventory fingerprints, provision host trust explicitly, and document revocation.

SSH agent forwarding is an explicit interactive mode rather than the autonomous default.
It avoids copying a key but depends on a live connection and allows a compromised worker to use the forwarded agent while connected.

## Git signing

Use the existing signing subkey attached to the operator's identity for both local and managed agent development.
Export and import only that secret signing subkey and the required public key material.
Workers never receive the primary secret key or unrelated encryption and authentication subkeys.

The exported signing subkey intentionally has no passphrase.
This keeps Git and other DVCS signing behavior consistent across local and remote environments without per-instance pinentry or signing-key changes.

The passphrase-free secret subkey is a bearer credential.
Possession is sufficient to create signatures under that subkey, so filesystem permissions, propagation scope, worker trust, inventory, revocation, rotation, snapshot policy, and cleanup are the protection boundary.

`akagent` should record the subkey fingerprint and installation state without recording exported key material.
Import must verify the expected primary-key and signing-subkey fingerprints before reporting readiness.
Removal must target the propagated secret subkey without deleting unrelated local keys.

This decision removes passphrase and pinentry availability from task startup, but signing failures must still fail quickly and produce a structured capability error.

## GitHub and APIs

Prefer a dedicated fine-grained token or GitHub App credential over a broad human token.
Separate repository read, Git push, issue and pull-request mutation, Actions, and administration capabilities.

Apply the same model to other API providers.
Work-specific credentials are deferred but must eventually use separate trust domains and policies.

## Additional credential classes

- LLM provider keys and project identifiers.
- Package registries such as npm, RubyGems, PyPI, and private Go modules.
- Cloud access, preferably obtained from instance or task roles rather than propagation.
- Artifact and object storage.
- Deployment providers.
- VPN or private-overlay enrollment.
- Encryption identities such as age or SOPS.
- Database access, deferred with work-specific credentials.
- Container registries, deferred with container execution.

Git author name and email, host fingerprints, and CA trust are identity configuration rather than secrets but belong in the same readiness checks.

Browser cookies, profiles, and OAuth refresh tokens are broad and difficult to scope.
They are not propagated by default and require a separate interactive-authentication design.

## Redaction and diagnostics

The implementation must avoid reading a secret until an operation requires it.
Once read, it should minimize copies and lifetime in memory.

Diagnostics report credential ID, provider, fingerprint, scope, status, and expiration only.
Known values and common encoded forms should be redacted defensively, but redaction is not a substitute for never logging values.

Shell tracing must be disabled around credential operations.
Dependency errors must be translated before output because raw errors may include paths, commands, or values.

## Rotation and revocation

Source changes do not automatically prove remote copies were removed.
The local inventory records installed fingerprints and last successful worker observation.

Rotation should install the replacement, verify readiness, update task ownership, and then remove the old credential.
Revocation should mark known remote copies as cleanup debt until each worker confirms removal.

## Failure modes

- A task starts with read access and later needs mutation access.
- A token expires during autonomous work.
- Transfer succeeds but launch fails.
- Cleanup races another task using the same credential.
- Two credentials target one destination.
- GPG waits indefinitely for pinentry.
- Debug output or shell tracing exposes values.
- A worker snapshot captures retained secrets.
- Rotation leaves stale remote copies.
- Values containing newlines or binary data break naive environment handling.
- A compromised agent reads a credential and sends it to an LLM or external service.
- An unrequested secret is inherited from the operator process environment.

The last risk cannot be solved by file permissions once the task legitimately receives the credential.
Least privilege, short lifetime, provider-side scope, network controls, and independent agent credentials reduce the blast radius.

## Initial implementation scope

1. Define the credential manifest schema.
2. Implement `credential doctor`, `list`, and `inspect` locally.
3. Support `file:` and `env:` sources.
4. Validate ownership, permissions, and non-secret identity metadata.
5. Add task capability requirements and startup warnings or failures.
6. Guarantee that output and errors never include values.
7. Add dedicated SSH, signing, and GitHub credentials operationally.
8. Add transfer and remote cleanup only with remote task execution.
