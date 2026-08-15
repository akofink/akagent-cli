# `akagent` integration signal

Agent orchestration is ready for default enablement over the stable `akagent` CLI boundary.
Automated lifecycle behavior belongs to the agent skill, while direct human `akagent` commands remain available independently.

`AKAGENT_ENABLED` remains the immediate per-environment disable signal for automated integrations.
At the CLI boundary, only the exact value `1` enables automation.
An unset signal, an empty value, or any other value disables automation.

Inspect the current state without changing it:

```bash
akagent integration inspect
```

Enable the signal for the current shell when an approved automated integration needs it:

```bash
export AKAGENT_ENABLED=1
```

Disable it immediately for the current shell with:

```bash
unset AKAGENT_ENABLED
```

After a command that may have mutated task state fails, inspect the task and run reconciliation before attempting a manual fallback.
The signal does not grant credentials or launch tasks by itself.
An explicit direct CLI managed Pi start remains available regardless of the signal.
